package agentreceiver

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"platform/sensor/internal/runtime"
)

const (
	EndpointPath     = "/api/v1/agent/telemetry"
	maxSensorIDBytes = 128
)

// Config parameterizes the Agent receiver component.
type Config struct {
	SensorID        string
	SensorRole      runtime.Role
	MaxBodyBytes    int64
	MaxBatchRecords int
	MaxRecordBytes  int
}

// IsRoleEligible determines whether a sensor role is permitted to ingest Agent telemetry.
// Only INFRASTRUCTURE and COMBINED roles are authorized. NETWORK-only sensors are rejected.
func IsRoleEligible(role runtime.Role) bool {
	return role == runtime.RoleInfrastructure || role == runtime.RoleCombined
}

// Receiver handles Agent telemetry ingestion requests over mTLS.
type Receiver struct {
	cfg        Config
	identity   AgentIdentityVerifier
	authorizer AssignmentAuthorizer
	sink       AcceptanceSink
	replay     *ReplayTracker
	metrics    *ReceiverMetrics
	logger     *slog.Logger
}

func NewReceiver(
	cfg Config,
	identity AgentIdentityVerifier,
	authorizer AssignmentAuthorizer,
	sink AcceptanceSink,
	logger *slog.Logger,
) (*Receiver, error) {
	if !IsRoleEligible(cfg.SensorRole) {
		return nil, fmt.Errorf("%s: sensor role %q is not eligible for agent ingress; role must be INFRASTRUCTURE or COMBINED",
			ErrCodeRoleIneligible, cfg.SensorRole)
	}

	sensorID := strings.TrimSpace(cfg.SensorID)
	if sensorID == "" {
		return nil, errors.New("ERR_INVALID_CONFIG: SensorID must not be empty")
	}
	if utf8.RuneCountInString(sensorID) > maxSensorIDBytes || len(sensorID) > maxSensorIDBytes {
		return nil, fmt.Errorf("ERR_INVALID_CONFIG: SensorID length exceeds maximum of %d", maxSensorIDBytes)
	}
	cfg.SensorID = sensorID

	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if cfg.MaxBatchRecords <= 0 {
		cfg.MaxBatchRecords = DefaultMaxRecords
	}
	if cfg.MaxRecordBytes <= 0 {
		cfg.MaxRecordBytes = DefaultMaxRecordBytes
	}
	if identity == nil {
		identity = NewFailClosedIdentityVerifier()
	}
	if authorizer == nil {
		authorizer = NewFailClosedAuthorizer()
	}
	if sink == nil {
		sink = NewFailClosedAcceptanceSink()
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Receiver{
		cfg:        cfg,
		identity:   identity,
		authorizer: authorizer,
		sink:       sink,
		replay:     NewReplayTracker(5000),
		metrics:    NewReceiverMetrics(),
		logger:     logger.With("component", "agentreceiver", "sensor_id", cfg.SensorID, "role", string(cfg.SensorRole)),
	}, nil
}

func (rcv *Receiver) Metrics() *ReceiverMetrics {
	return rcv.metrics
}

func (rcv *Receiver) ReplayTracker() *ReplayTracker {
	return rcv.replay
}

func (rcv *Receiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Path enforcement
	if r.URL.Path != EndpointPath {
		http.NotFound(w, r)
		return
	}

	// 2. Method enforcement
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		rcv.writeJSONError(w, http.StatusMethodNotAllowed, "ERR_METHOD_NOT_ALLOWED", "Method must be POST")
		return
	}

	// 2b. Content-Type must be application/json (charset optional)
	if !isApplicationJSON(r.Header.Get("Content-Type")) {
		rcv.metrics.MalformedBatches.Add(1)
		rcv.writeJSONError(w, http.StatusUnsupportedMediaType, ErrCodeMalformedJSON, "Content-Type must be application/json")
		return
	}

	// 3. Sensor role eligibility (defense-in-depth)
	if !IsRoleEligible(rcv.cfg.SensorRole) {
		rcv.metrics.RoleIneligibleErrors.Add(1)
		rcv.writeJSONError(w, http.StatusForbidden, ErrCodeRoleIneligible, "Sensor role is not eligible for agent ingress")
		return
	}

	// 4. mTLS Client Certificate Authentication + identity verification.
	// Require verified chain material — PeerCertificates alone is insufficient.
	_, verifiedChain, err := requireVerifiedClientLeaf(r.TLS)
	if err != nil {
		rcv.metrics.AuthFailures.Add(1)
		code := ErrCodeAuthRequired
		if strings.Contains(err.Error(), ErrCodeAuthInvalid) {
			code = ErrCodeAuthInvalid
		}
		rcv.writeJSONError(w, http.StatusUnauthorized, code, "Client certificate is required for mutual TLS authentication")
		return
	}

	authAgent, err := rcv.identity.VerifyPeerIdentity(verifiedChain)
	if err != nil || strings.TrimSpace(authAgent.AgentID) == "" {
		rcv.metrics.AuthFailures.Add(1)
		rcv.logger.Warn("Peer identity verification failed",
			"remote_addr", r.RemoteAddr,
		)
		code := ErrCodeAuthInvalid
		if err != nil && strings.Contains(err.Error(), ErrCodeAuthRequired) {
			code = ErrCodeAuthRequired
		}
		rcv.writeJSONError(w, http.StatusUnauthorized, code, "Client certificate identity could not be verified")
		return
	}

	// 5. Bounded Body & Strict JSON Decoding
	limitedBody := http.MaxBytesReader(w, r.Body, rcv.cfg.MaxBodyBytes)
	dec := json.NewDecoder(limitedBody)
	dec.DisallowUnknownFields()

	var env DeliveryEnvelope
	if err := dec.Decode(&env); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			rcv.metrics.MalformedBatches.Add(1)
			rcv.writeJSONError(w, http.StatusRequestEntityTooLarge, ErrCodeRequestTooLarge,
				fmt.Sprintf("Request body exceeds maximum size limit of %d bytes", rcv.cfg.MaxBodyBytes))
			return
		}
		if errors.Is(err, io.EOF) {
			rcv.metrics.MalformedBatches.Add(1)
			rcv.writeJSONError(w, http.StatusBadRequest, ErrCodeEmptyBody, "Request body is empty")
			return
		}
		rcv.metrics.MalformedBatches.Add(1)
		rcv.writeJSONError(w, http.StatusBadRequest, ErrCodeMalformedJSON, "Malformed JSON payload")
		return
	}

	// Reject trailing garbage after the JSON object
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		rcv.metrics.MalformedBatches.Add(1)
		rcv.writeJSONError(w, http.StatusBadRequest, ErrCodeMalformedJSON, "Extraneous data after JSON body")
		return
	}

	// 6. Body AgentID is a claim: must EqualFold match authenticated identity.
	// Never select identity from arbitrary SAN matching.
	if !strings.EqualFold(strings.TrimSpace(env.AgentID), authAgent.AgentID) {
		rcv.metrics.AuthFailures.Add(1)
		rcv.logger.Warn("Agent identity spoofing attempt detected",
			"authenticated_agent_id", authAgent.AgentID,
			"claimed_agent_id", env.AgentID,
			"remote_addr", r.RemoteAddr,
		)
		rcv.writeJSONError(w, http.StatusForbidden, ErrCodeAuthIdentityMismatch,
			"Claimed agentId does not match authenticated client identity")
		return
	}

	// 7. Authorize Agent -> Sensor Assignment using authenticated identity + receiving sensor ONLY
	assignCtx, err := rcv.authorizer.Authorize(r.Context(), authAgent, rcv.cfg.SensorID)
	if err != nil || !assignCtx.Assigned {
		rcv.metrics.AssignmentDenials.Add(1)
		rcv.logger.Warn("Agent delivery denied by assignment authorizer",
			"agent_id", authAgent.AgentID,
			"sensor_id", rcv.cfg.SensorID,
		)
		rcv.writeJSONError(w, http.StatusForbidden, ErrCodeAgentNotAssigned, "Agent is not assigned to this Infrastructure Sensor")
		return
	}

	if err := validateAssignmentContext(assignCtx, authAgent, rcv.cfg.SensorID); err != nil {
		rcv.metrics.AssignmentDenials.Add(1)
		rcv.writeJSONError(w, http.StatusForbidden, ErrCodeAgentNotAssigned, "Assignment context is inconsistent with authenticated delivery")
		return
	}

	// Payload tenant/site are claims; when present they must match authoritative assignment.
	if claim := strings.TrimSpace(env.TenantID); claim != "" && !strings.EqualFold(claim, assignCtx.TenantID) {
		rcv.metrics.AssignmentDenials.Add(1)
		rcv.writeJSONError(w, http.StatusForbidden, ErrCodeAgentNotAssigned, "Payload tenant claim does not match authoritative assignment")
		return
	}
	payloadSite := strings.TrimSpace(env.SiteID)
	authSite := strings.TrimSpace(assignCtx.SiteID)
	if payloadSite != "" && authSite == "" {
		rcv.metrics.AssignmentDenials.Add(1)
		rcv.writeJSONError(w, http.StatusForbidden, ErrCodeAgentNotAssigned, "Payload site claim is not authorized by assignment")
		return
	}
	if payloadSite != "" && authSite != "" && !strings.EqualFold(payloadSite, authSite) {
		rcv.metrics.AssignmentDenials.Add(1)
		rcv.writeJSONError(w, http.StatusForbidden, ErrCodeAgentNotAssigned, "Payload site claim does not match authoritative assignment")
		return
	}

	// 8. Validate Envelope Structure & Record Checksums
	if valErr := ValidateEnvelope(&env, rcv.cfg.MaxBatchRecords, rcv.cfg.MaxRecordBytes); valErr != nil {
		if valErr.Code == ErrCodeRecordChecksumMismatch {
			rcv.metrics.ChecksumFailures.Add(1)
		} else {
			rcv.metrics.MalformedBatches.Add(1)
		}
		rcv.writeJSONError(w, http.StatusBadRequest, valErr.Code, valErr.Message)
		return
	}

	// Normalize envelope to authoritative identity/assignment before durable sink.
	env.AgentID = authAgent.AgentID
	env.TenantID = assignCtx.TenantID
	env.SiteID = assignCtx.SiteID

	// 9. Check Replay Tracker (Exact Replay vs Conflicting Replay)
	// Use authoritative tenant from AssignmentContext + authenticated agent + batchID.
	replayID := ReplayIdentity{
		TenantID: assignCtx.TenantID,
		AgentID:  authAgent.AgentID,
		BatchID:  env.BatchID,
	}
	batchChecksum := ComputeBatchChecksum(env.Records)

	repDecision, cachedAck := rcv.replay.Check(replayID, env.FirstSequence, env.LastSequence, batchChecksum)
	if repDecision == ReplayExact {
		rcv.metrics.ReplayExactHits.Add(1)
		rcv.logger.Info("Exact duplicate replay acknowledged from cache",
			"agent_id", authAgent.AgentID,
			"batch_id", env.BatchID,
			"accepted_through", cachedAck.AcceptedThroughSequence,
		)
		rcv.writeJSON(w, http.StatusOK, cachedAck)
		return
	}
	if repDecision == ReplayConflict {
		rcv.metrics.ReplayConflicts.Add(1)
		rcv.logger.Warn("Conflicting batch replay detected and rejected",
			"agent_id", authAgent.AgentID,
			"batch_id", env.BatchID,
			"first_sequence", env.FirstSequence,
			"last_sequence", env.LastSequence,
		)
		rcv.writeJSONError(w, http.StatusConflict, ErrCodeReplayConflict,
			"Batch ID was previously processed with conflicting contents or sequence range")
		return
	}

	// 10. Downstream Sensor Acceptance Sink
	result, err := rcv.sink.AcceptBatch(r.Context(), env)
	if err != nil {
		rcv.metrics.SinkFailures.Add(1)
		rcv.logger.Error("Acceptance sink refused or failed to process batch",
			"agent_id", authAgent.AgentID,
			"batch_id", env.BatchID,
			"error", err,
		)
		rcv.writeJSONError(w, http.StatusServiceUnavailable, ErrCodeSinkFailed,
			"Sensor acceptance sink unavailable")
		return
	}

	// Sanity-check watermark from sink
	if result.AcceptedThrough < env.FirstSequence || result.AcceptedThrough > env.LastSequence {
		rcv.metrics.SinkFailures.Add(1)
		rcv.logger.Error("Acceptance sink returned corrupt watermark",
			"accepted_through", result.AcceptedThrough,
			"first_sequence", env.FirstSequence,
			"last_sequence", env.LastSequence,
		)
		rcv.writeJSONError(w, http.StatusInternalServerError, ErrCodeInternalError,
			"Acceptance sink returned watermark outside submitted sequence range")
		return
	}

	// 11. Formulate and Cache Authoritative Application ACK (batch-bound)
	ack := DeliveryAck{
		Status:                  "ACCEPTED",
		AcceptedThroughSequence: result.AcceptedThrough,
		AgentID:                 authAgent.AgentID,
		BatchID:                 env.BatchID,
		CorrelationID:           env.CorrelationID,
	}

	rcv.replay.Record(replayID, env.FirstSequence, env.LastSequence, batchChecksum, ack)
	rcv.metrics.AcceptedBatches.Add(1)
	rcv.metrics.AcceptedRecords.Add(int64(len(env.Records)))
	rcv.metrics.LastAckedSequence.Store(result.AcceptedThrough)

	rcv.writeJSON(w, http.StatusOK, ack)
}

func isApplicationJSON(ct string) bool {
	ct = strings.TrimSpace(strings.ToLower(ct))
	if ct == "" {
		return false
	}
	mediaType, _, _ := strings.Cut(ct, ";")
	return strings.TrimSpace(mediaType) == "application/json"
}

// requireVerifiedClientLeaf extracts the leaf and verified chain from TLS state.
// PeerCertificates alone are insufficient; VerifiedChains must be present and non-empty.
func requireVerifiedClientLeaf(tlsState *tls.ConnectionState) (*x509.Certificate, []*x509.Certificate, error) {
	if tlsState == nil {
		return nil, nil, errors.New(ErrCodeAuthRequired + ": TLS connection state is missing")
	}
	if len(tlsState.PeerCertificates) == 0 || tlsState.PeerCertificates[0] == nil {
		return nil, nil, errors.New(ErrCodeAuthRequired + ": peer certificate is missing")
	}
	if len(tlsState.VerifiedChains) == 0 || len(tlsState.VerifiedChains[0]) == 0 || tlsState.VerifiedChains[0][0] == nil {
		return nil, nil, errors.New(ErrCodeAuthInvalid + ": verified client certificate chain is required")
	}
	chain := tlsState.VerifiedChains[0]
	return chain[0], chain, nil
}

func validateAssignmentContext(assignCtx AssignmentContext, authAgent AuthenticatedAgentIdentity, receivingSensorID string) error {
	if !assignCtx.Assigned {
		return errors.New("assignment not authorized")
	}
	if strings.TrimSpace(assignCtx.TenantID) == "" {
		return errors.New("assignment tenant is empty")
	}
	if agent := strings.TrimSpace(assignCtx.AgentID); agent != "" && !strings.EqualFold(agent, authAgent.AgentID) {
		return errors.New("assignment agent does not match authenticated agent")
	}
	if !strings.EqualFold(strings.TrimSpace(assignCtx.SensorID), strings.TrimSpace(receivingSensorID)) {
		return errors.New("assignment sensor does not match receiving sensor")
	}
	return nil
}

func (rcv *Receiver) writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Status:  "ERROR",
		Code:    code,
		Message: message,
	})
}

func (rcv *Receiver) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
