package agentreceiver

import (
	"encoding/json"
	"time"
)

const (
	// CurrentSchemaVersion documents the modern Agent→Sensor wire schema (v2).
	// The validator must accept both transitional v1 and current v2 explicitly;
	// do not equality-check solely against this constant.
	CurrentSchemaVersion = 2
	SchemaVersionV1      = 1
	SchemaVersionV2      = 2

	// MAX_SAFE_SEQUENCE is the JSON-safe delivery/replay sequence ceiling (2^53 - 1).
	MAX_SAFE_SEQUENCE = 9007199254740991

	// DefaultMaxBodyBytes matches Core AgentIngestionValidator.MAX_RECORD_BYTES.
	// One Agent envelope is persisted as one Sensor Agent-spool record and drained
	// as one Core agent-ingress record. Accepting a larger body would ACK data
	// Core rejects with ERR_RECORD_TOO_LARGE (HTTP 413).
	DefaultMaxBodyBytes   = 512 * 1024
	DefaultMaxRecords     = 100
	DefaultMaxRecordBytes = 512 * 1024

	lineageOwnerVersion    = 1
	legacyTombstoneVersion = 1
	modernDurableVersion   = 2
)

// Standard stable error codes for agent receiver
const (
	ErrCodeEmptyBody                     = "ERR_EMPTY_BODY"
	ErrCodeMalformedJSON                 = "ERR_MALFORMED_JSON"
	ErrCodeRequestTooLarge               = "ERR_REQUEST_TOO_LARGE"
	ErrCodeAuthRequired                  = "ERR_AUTH_CERTIFICATE_REQUIRED"
	ErrCodeAuthInvalid                   = "ERR_AUTH_CERTIFICATE_INVALID"
	ErrCodeAuthIdentityMismatch          = "ERR_AUTH_IDENTITY_MISMATCH"
	ErrCodeAgentNotAssigned              = "ERR_AGENT_NOT_ASSIGNED"
	ErrCodeAgentAuthorityUnavailable     = "ERR_AGENT_AUTHORITY_UNAVAILABLE"
	ErrCodeAgentAssignmentSensorMismatch = "ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH"
	ErrCodeRoleIneligible                = "ERR_ROLE_INELIGIBLE"
	ErrCodeUnsupportedSchema             = "ERR_UNSUPPORTED_SCHEMA_VERSION"
	ErrCodeEmptyBatch                    = "ERR_EMPTY_BATCH"
	ErrCodeBatchTooLarge                 = "ERR_BATCH_TOO_LARGE"
	ErrCodeSequenceMismatch              = "ERR_SEQUENCE_MISMATCH"
	ErrCodeSequenceGap                   = "ERR_SEQUENCE_GAP"
	ErrCodeRecordChecksumMismatch        = "ERR_RECORD_CHECKSUM_MISMATCH"
	ErrCodeInvalidField                  = "ERR_INVALID_FIELD"
	ErrCodeRecordTooLarge                = "ERR_RECORD_TOO_LARGE"
	ErrCodeReplayConflict                = "ERR_REPLAY_CONFLICT"
	ErrCodeSinkFailed                    = "ERR_ACCEPTANCE_SINK_FAILED"
	ErrCodeInternalError                 = "ERR_INTERNAL_ERROR"
	ErrCodeStreamStaleOrFenced           = "ERR_STREAM_STALE_OR_FENCED"
	ErrCodeLegacyStreamClosed            = "ERR_LEGACY_STREAM_CLOSED"
	ErrCodeStreamOwnerCorrupt            = "ERR_STREAM_OWNER_CORRUPT"
)

// DeliveryRecord represents an individual spooled record inside an agent delivery batch.
type DeliveryRecord struct {
	Sequence  uint64          `json:"sequence"`
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"createdAt"`
	Checksum  string          `json:"checksum"`
	Payload   json.RawMessage `json:"payload"`
}

// DeliveryEnvelope is the versioned wire envelope submitted by an Agent to the Infrastructure Sensor.
type DeliveryEnvelope struct {
	SchemaVersion int              `json:"schemaVersion"`
	AgentID       string           `json:"agentId"`
	TenantID      string           `json:"tenantId"`
	SiteID        string           `json:"siteId,omitempty"`
	CorrelationID string           `json:"correlationId"`
	BatchID       string           `json:"batchId"`
	FirstSequence uint64           `json:"firstSequence"`
	LastSequence  uint64           `json:"lastSequence"`
	RecordCount   int              `json:"recordCount"`
	Records       []DeliveryRecord `json:"records"`
	// AgentStreamID is required on schema v2 and forbidden on transitional schema v1.
	AgentStreamID string `json:"agentStreamId,omitempty"`

	// Stamped by Receiver after authoritative assignment; never wire claims.
	ReceivingSensorID    string `json:"-"`
	AssignmentGeneration int64  `json:"-"`
}

// DeliveryAck is the explicit application acknowledgement returned by the Sensor.
// BatchID must echo the submitted envelope batchId for batch-bound deletion safety.
type DeliveryAck struct {
	Status                  string `json:"status"`
	AcceptedThroughSequence uint64 `json:"accepted_through_sequence"`
	AgentID                 string `json:"agent_id"`
	BatchID                 string `json:"batch_id"`
	CorrelationID           string `json:"correlation_id"`
}

// ErrorResponse is the structured error model returned by the Sensor receiver.
type ErrorResponse struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
