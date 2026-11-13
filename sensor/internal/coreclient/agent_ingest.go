package coreclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const agentTelemetryPath = "/api/v1/sensor/agent-telemetry"

// AgentTelemetryBatch is the Sensor→Core agent-ingress drain request (BUILD-003 contract v2).
type AgentTelemetryBatch struct {
	SchemaVersion            int                  `json:"schemaVersion"`
	SensorID                 string               `json:"sensorId"`
	SensorAgentSpoolStreamID string               `json:"sensorAgentSpoolStreamId"`
	BatchID                  string               `json:"batchId"`
	FirstIngressSequence     uint64               `json:"firstIngressSequence"`
	LastIngressSequence      uint64               `json:"lastIngressSequence"`
	RecordCount              int                  `json:"recordCount"`
	Checksum                 string               `json:"checksum"`
	CorrelationID            string               `json:"correlationId"`
	Records                  []AgentIngressRecord `json:"records"`
}

// AgentIngressRecord preserves immutable accepted evidence from the Sensor Agent spool.
type AgentIngressRecord struct {
	SensorIngressSequence uint64                      `json:"sensorIngressSequence"`
	SensorIngressRecordID string                      `json:"sensorIngressRecordId"`
	TenantID              string                      `json:"tenantId"`
	AgentID               string                      `json:"agentId"`
	AcceptingSensorID     string                      `json:"acceptingSensorId"`
	AssignmentGeneration  int64                       `json:"assignmentGeneration"`
	AgentFirstSequence    uint64                      `json:"agentFirstSequence"`
	AgentLastSequence     uint64                      `json:"agentLastSequence"`
	AgentBatchID          string                      `json:"agentBatchId"`
	CorrelationID         string                      `json:"correlationId"`
	AcceptedAt            string                      `json:"acceptedAt"`
	AcceptanceEvidence    *SensorAcceptanceEvidenceV1 `json:"acceptanceEvidence,omitempty"`
	Records               json.RawMessage             `json:"records"`
}

// AgentTelemetryAck is the durable Core acknowledgement for agent-telemetry.
type AgentTelemetryAck struct {
	Status                         string `json:"status"`
	SensorID                       string `json:"sensorId"`
	BatchID                        string `json:"batchId"`
	AcceptedThroughIngressSequence uint64 `json:"acceptedThroughIngressSequence"`
	AcceptedLastIngressSequence    uint64 `json:"acceptedLastIngressSequence,omitempty"`
	CorrelationID                  string `json:"correlationId"`
	FirstIngressSequence           uint64 `json:"firstIngressSequence,omitempty"`
	LastIngressSequence            uint64 `json:"lastIngressSequence,omitempty"`
	Checksum                       string `json:"checksum,omitempty"`
	SettledThroughIngressSequence  uint64 `json:"settledThroughIngressSequence,omitempty"`
	DispositionID                  string `json:"dispositionId,omitempty"`
	ReasonCode                     string `json:"reasonCode,omitempty"`
}

var durableDispositionID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// UnmarshalJSON keeps agent-telemetry ACK strict: unknown fields fail closed.
func (a *AgentTelemetryAck) UnmarshalJSON(data []byte) error {
	type alias AgentTelemetryAck
	var decoded alias
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decoded); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("agent-telemetry acknowledgement contains trailing JSON data")
	}
	if strings.TrimSpace(decoded.Status) == "" {
		return errors.New("agent-telemetry acknowledgement missing status")
	}
	if strings.TrimSpace(decoded.SensorID) == "" {
		return errors.New("agent-telemetry acknowledgement missing sensorId")
	}
	if strings.TrimSpace(decoded.BatchID) == "" {
		return errors.New("agent-telemetry acknowledgement missing batchId")
	}
	if decoded.Status == "ACCEPTED" && decoded.AcceptedThroughIngressSequence == 0 && decoded.AcceptedLastIngressSequence == 0 {
		return errors.New("agent-telemetry acknowledgement missing accepted sequence")
	}
	if strings.TrimSpace(decoded.CorrelationID) == "" {
		return errors.New("agent-telemetry acknowledgement missing correlationId")
	}
	*a = AgentTelemetryAck(decoded)
	return nil
}

// CoreAPIError captures a structured Core error body for terminal classification.
type CoreAPIError struct {
	Status     string
	Code       string
	Message    string
	HTTPStatus int
	transient  bool
}

func (e *CoreAPIError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *CoreAPIError) Transient() bool { return e != nil && e.transient }

func IsCoreAPICode(err error, code string) bool {
	var target *CoreAPIError
	return errors.As(err, &target) && target.Code == code
}

// UploadAgentTelemetry POSTs agent-ingress batches. HTTP 200 alone is insufficient;
// ACK fields must bind to the submitted batch.
func (c *Client) UploadAgentTelemetry(ctx context.Context, batch AgentTelemetryBatch) (AgentTelemetryAck, error) {
	if c == nil {
		return AgentTelemetryAck{}, newError(CodeInvalidConfig, "Core client is nil", nil, false)
	}
	if c.initErr != nil {
		return AgentTelemetryAck{}, c.initErr
	}
	if ctx == nil {
		return AgentTelemetryAck{}, newError(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	if err := contextClassified(ctx); err != nil {
		return AgentTelemetryAck{}, err
	}
	if batch.SchemaVersion != 2 || strings.TrimSpace(batch.SensorID) == "" || strings.TrimSpace(batch.SensorAgentSpoolStreamID) == "" ||
		strings.TrimSpace(batch.BatchID) == "" ||
		batch.FirstIngressSequence == 0 || batch.LastIngressSequence < batch.FirstIngressSequence ||
		batch.FirstIngressSequence > MaxSafeSequence || batch.LastIngressSequence > MaxSafeSequence ||
		batch.RecordCount == 0 || len(batch.Records) == 0 || strings.TrimSpace(batch.Checksum) == "" ||
		strings.TrimSpace(batch.CorrelationID) == "" {
		return AgentTelemetryAck{}, newError(CodeInvalidConfig, "agent-telemetry batch metadata is incomplete or invalid", nil, false)
	}

	payload, err := json.Marshal(batch)
	if err != nil {
		return AgentTelemetryAck{}, newError(CodeProtocol, "failed to encode agent-telemetry request", err, false)
	}

	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(c.baseURL.Path, "/") + agentTelemetryPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return AgentTelemetryAck{}, newError(CodeProtocol, "failed to create agent-telemetry request", err, false)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", batch.CorrelationID)
	req.Header.Set("Idempotency-Key", batch.BatchID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return AgentTelemetryAck{}, classifyTransport(ctx, err)
	}
	defer resp.Body.Close()
	responseBody, err := readBounded(resp.Body, c.maxResponseBytes)
	if err != nil {
		return AgentTelemetryAck{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AgentTelemetryAck{}, classifyAgentTelemetryStatus(resp.StatusCode, responseBody)
	}

	var ack AgentTelemetryAck
	if err := json.Unmarshal(responseBody, &ack); err != nil {
		return AgentTelemetryAck{}, newError(CodeAckContractMissing,
			"Core agent-telemetry response failed strict ACK contract", err, false)
	}
	if err := validateAgentTelemetryAck(batch, ack); err != nil {
		return AgentTelemetryAck{}, err
	}
	return ack, nil
}

func validateAgentTelemetryAck(batch AgentTelemetryBatch, ack AgentTelemetryAck) error {
	if ack.SensorID != batch.SensorID {
		return newError(CodeProtocol, "Core agent-telemetry acknowledgement sensorId does not match submitted batch", nil, false)
	}
	if ack.BatchID != batch.BatchID {
		return newError(CodeProtocol, "Core agent-telemetry acknowledgement batchId does not match submitted batch", nil, false)
	}
	if ack.CorrelationID != batch.CorrelationID {
		return newError(CodeProtocol, "Core agent-telemetry acknowledgement correlationId does not match submitted batch", nil, false)
	}
	switch ack.Status {
	case "ACCEPTED":
		acceptedLast := ack.AcceptedLastIngressSequence
		if acceptedLast == 0 {
			acceptedLast = ack.AcceptedThroughIngressSequence // pre-V020 response
		}
		if acceptedLast != batch.LastIngressSequence || ack.AcceptedThroughIngressSequence > acceptedLast ||
			ack.DispositionID != "" || ack.ReasonCode != "" {
			return newError(CodeProtocol, "Core accepted range does not match submitted batch", nil, false)
		}
	case "TERMINALLY_REJECTED":
		if batch.FirstIngressSequence != batch.LastIngressSequence || ack.FirstIngressSequence != batch.FirstIngressSequence ||
			ack.LastIngressSequence != batch.LastIngressSequence || !strings.EqualFold(ack.Checksum, batch.Checksum) ||
			ack.SettledThroughIngressSequence < batch.LastIngressSequence ||
			ack.ReasonCode != "ERR_AGENT_IDENTITY_CONFLICT" ||
			!durableDispositionID.MatchString(strings.ToLower(ack.DispositionID)) ||
			ack.AcceptedLastIngressSequence != 0 || ack.AcceptedThroughIngressSequence >= batch.LastIngressSequence {
			return newError(CodeProtocol, "Core terminal disposition does not match submitted batch", nil, false)
		}
	default:
		return newError(CodeProtocol, "Core agent-telemetry disposition status is invalid", nil, false)
	}
	return nil
}

// ValidateAgentTelemetryAck lets drain verify a typed result even when a
// different transport implementation is injected through its interface.
func ValidateAgentTelemetryAck(batch AgentTelemetryBatch, ack AgentTelemetryAck) error {
	return validateAgentTelemetryAck(batch, ack)
}

func classifyAgentTelemetryStatus(status int, body []byte) error {
	var apiErr struct {
		Status  string `json:"status"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &apiErr)
	code := strings.TrimSpace(apiErr.Code)
	msg := strings.TrimSpace(apiErr.Message)
	if msg == "" {
		msg = fmt.Sprintf("Core returned HTTP status %d", status)
	}
	transient := status == http.StatusTooManyRequests || status >= 500
	// BUILD-003 publisher-lease / prior-pending conditions are retryable and must retain spool.
	switch code {
	case "ERR_PREVIOUS_BATCH_PENDING", "ERR_BATCH_PUBLICATION_IN_PROGRESS":
		transient = true
	case "ERR_AGENT_LEGACY_PENDING_RECONCILIATION_REQUIRED",
		"ERR_AGENT_CANONICAL_DIGEST_MISMATCH",
		"ERR_AGENT_AUTHORITY_CHECKPOINT_UNAVAILABLE":
		// Retain and retry the frozen head, while exposing a persistent blocked
		// stream in Sensor self-health until governed reconciliation succeeds.
		transient = false
	}
	if code != "" {
		return &CoreAPIError{Status: apiErr.Status, Code: code, Message: msg, HTTPStatus: status, transient: transient}
	}
	return classifyStatus(status)
}

// ClassifyAgentTelemetryFailure maps a Core agent-ingress HTTP failure for drain classification tests.
func ClassifyAgentTelemetryFailure(httpStatus int, body []byte) error {
	return classifyAgentTelemetryStatus(httpStatus, body)
}

// IsAgentIngressSpoolRetainingCode reports BUILD-003 transport/replay/lineage/state
// and authority/assignment failures that must retain durable Agent spool data
// (no poison park / AcknowledgeThrough). Unknown application 4xx also retain by default
// via the explicit poison allowlist in agentdrain — this helper documents the known set.
func IsAgentIngressSpoolRetainingCode(code string) bool {
	switch strings.TrimSpace(code) {
	case "ERR_PREVIOUS_BATCH_PENDING",
		"ERR_BATCH_PUBLICATION_IN_PROGRESS",
		"ERR_SENSOR_STREAM_CHANGED",
		"ERR_SENSOR_AGENT_SPOOL_STREAM_CHANGED",
		"ERR_IDEMPOTENCY_CONFLICT",
		"ERR_SEQUENCE_REGRESSION",
		"ERR_SEQUENCE_GAP",
		"ERR_SEQUENCE_OUT_OF_BOUNDS",
		"ERR_AGENT_STREAM_ID_INVALID",
		"ERR_SENSOR_AUTHORITY_UNAVAILABLE",
		"ERR_AUTH_CLIENT_CERT_REQUIRED",
		"ERR_AUTH_IDENTITY_MISMATCH",
		"ERR_AGENT_ASSIGNMENT_UNKNOWN",
		"ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH",
		"ERR_AGENT_ACCEPTING_SENSOR_MISMATCH",
		"ERR_TENANT_SCOPE_MISMATCH",
		"ERR_AGENT_ENTITY_UNRESOLVED",
		"ERR_AGENT_LEGACY_PENDING_RECONCILIATION_REQUIRED":
		return true
	default:
		return false
	}
}
