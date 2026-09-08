package agentreceiver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"platform/sensor/internal/runtime"
)

const EndpointPath = "/api/v1/agent/telemetry"

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
	authorizer AssignmentAuthorizer
	sink       AcceptanceSink
	replay     *ReplayTracker
	metrics    *ReceiverMetrics
	logger     *slog.Logger
}

func NewReceiver(
	cfg Config,
	authorizer AssignmentAuthorizer,
	sink AcceptanceSink,
	logger *slog.Logger,
) (*Receiver, error) {
	if !IsRoleEligible(cfg.SensorRole) {
		return nil, fmt.Errorf("%s: sensor role %q is not eligible for agent ingress; role must be INFRASTRUCTURE or COMBINED",
			ErrCodeRoleIneligible, cfg.SensorRole)
	}

	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if cfg.MaxBatchRecords <= 0 {
		cfg.MaxBatchRecords = DefaultMaxRecords
	}
	if cfg.MaxRecordBytes <= 0 {
		cfg.MaxRecordBytes = DefaultMaxRecordBytes
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

	// 3. Sensor role eligibility (defense-in-depth)
	if !IsRoleEligible(rcv.cfg.SensorRole) {
		rcv.metrics.RoleIneligibleErrors.Add(1)
		rcv.writeJSONError(w, http.StatusForbidden, ErrCodeRoleIneligible, "Sensor role is not eligible for agent ingress")
		return
	}

	// 4. mTLS Client Certificate Authentication
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		rcv.metrics.AuthFailures.Add(1)
		rcv.writeJSONError(w, http.StatusUnauthorized, ErrCodeAuthRequired, "Client certificate is required for mutual TLS authentication")
		return
	}

	clientCert := r.TLS.PeerCertificates[0]
	certCN := strings.TrimSpace(clientCert.Subject.CommonName)
	if certCN == "" {
		rcv.metrics.AuthFailures.Add(1)
		rcv.writeJSONError(w, http.StatusUnauthorized, ErrCodeAuthInvalid, "Client certificate Common Name must not be empty")
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
		rcv.writeJSONError(w, http.StatusBadRequest, ErrCodeMalformedJSON, fmt.Sprintf("Malformed JSON payload: %v", err))
		return
	}

	// Reject trailing garbage after the JSON object
	if dec.More() {
		rcv.metrics.MalformedBatches.Add(1)
		rcv.writeJSONError(w, http.StatusBadRequest, ErrCodeMalformedJSON, "Extraneous data after JSON body")
		return
	}

	// 6. Cross-check claimed Agent identity against authoritative client certificate
	if !matchCertIdentity(certCN, clientCert.DNSNames, env.AgentID) {
		rcv.metrics.AuthFailures.Add(1)
		rcv.logger.Warn("Agent identity spoofing attempt detected",
			"cert_cn", certCN,
			"claimed_agent_id", env.AgentID,
			"remote_addr", r.RemoteAddr,
		)
		rcv.writeJSONError(w, http.StatusForbidden, ErrCodeAuthIdentityMismatch,
			fmt.Sprintf("Claimed agentId %q does not match authenticated client certificate CN %q", env.AgentID, certCN))
		return
	}

	// 7. Authorize Agent -> Sensor Assignment
	decision, err := rcv.authorizer.AuthorizeAssignment(r.Context(), env.AgentID, env.TenantID, rcv.cfg.SensorID)
	if err != nil || !decision.Assigned {
		rcv.metrics.AssignmentDenials.Add(1)
		reason := "Agent is not assigned to this Infrastructure Sensor"
		if decision.Reason != "" {
			reason = decision.Reason
		}
		rcv.logger.Warn("Agent delivery denied by assignment authorizer",
			"agent_id", env.AgentID,
			"tenant_id", env.TenantID,
			"sensor_id", rcv.cfg.SensorID,
			"reason", reason,
		)
		rcv.writeJSONError(w, http.StatusForbidden, ErrCodeAgentNotAssigned, reason)
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

	// 9. Check Replay Tracker (Exact Replay vs Conflicting Replay)
	batchKey := fmt.Sprintf("%s:%s", env.AgentID, env.BatchID)
	batchChecksum := ComputeBatchChecksum(env.Records)

	repDecision, cachedAck := rcv.replay.Check(batchKey, env.FirstSequence, env.LastSequence, batchChecksum)
	if repDecision == ReplayExact {
		rcv.metrics.ReplayExactHits.Add(1)
		rcv.logger.Info("Exact duplicate replay acknowledged from cache",
			"batch_key", batchKey,
			"accepted_through", cachedAck.AcceptedThroughSequence,
		)
		rcv.writeJSON(w, http.StatusOK, cachedAck)
		return
	}
	if repDecision == ReplayConflict {
		rcv.metrics.ReplayConflicts.Add(1)
		rcv.logger.Warn("Conflicting batch replay detected and rejected",
			"batch_key", batchKey,
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
			"batch_key", batchKey,
			"error", err,
		)
		rcv.writeJSONError(w, http.StatusServiceUnavailable, ErrCodeSinkFailed,
			fmt.Sprintf("Sensor acceptance sink unavailable: %v", err))
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

	// 11. Formulate and Cache Authoritative Application ACK
	ack := DeliveryAck{
		Status:                  "ACCEPTED",
		AcceptedThroughSequence: result.AcceptedThrough,
		AgentID:                 env.AgentID,
		CorrelationID:           env.CorrelationID,
	}

	rcv.replay.Record(batchKey, env.FirstSequence, env.LastSequence, batchChecksum, ack)
	rcv.metrics.AcceptedBatches.Add(1)
	rcv.metrics.AcceptedRecords.Add(int64(len(env.Records)))
	rcv.metrics.LastAckedSequence.Store(result.AcceptedThrough)

	rcv.writeJSON(w, http.StatusOK, ack)
}

func matchCertIdentity(certCN string, dnsNames []string, claimedAgentID string) bool {
	claimedAgentID = strings.TrimSpace(claimedAgentID)
	if claimedAgentID == "" {
		return false
	}

	// Match 1: Exact CN match
	if strings.EqualFold(certCN, claimedAgentID) {
		return true
	}

	// Match 2: Standard platform suffix (e.g. agent-01.agent.platform.local)
	if strings.HasSuffix(strings.ToLower(certCN), ".agent.platform.local") {
		prefix := strings.TrimSuffix(strings.ToLower(certCN), ".agent.platform.local")
		if strings.EqualFold(prefix, claimedAgentID) {
			return true
		}
	}

	// Match 3: DNS SANs
	for _, dns := range dnsNames {
		if strings.EqualFold(dns, claimedAgentID) {
			return true
		}
		if strings.HasSuffix(strings.ToLower(dns), ".agent.platform.local") {
			prefix := strings.TrimSuffix(strings.ToLower(dns), ".agent.platform.local")
			if strings.EqualFold(prefix, claimedAgentID) {
				return true
			}
		}
	}

	return false
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
