package agentreceiver

import (
	"encoding/json"
	"time"
)

const (
	CurrentSchemaVersion  = 1
	DefaultMaxBodyBytes   = 2 * 1024 * 1024 // 2 MB
	DefaultMaxRecords     = 100
	DefaultMaxRecordBytes = 1024 * 1024 // 1 MB
)

// Standard stable error codes for agent receiver
const (
	ErrCodeEmptyBody              = "ERR_EMPTY_BODY"
	ErrCodeMalformedJSON          = "ERR_MALFORMED_JSON"
	ErrCodeRequestTooLarge        = "ERR_REQUEST_TOO_LARGE"
	ErrCodeAuthRequired           = "ERR_AUTH_CERTIFICATE_REQUIRED"
	ErrCodeAuthInvalid            = "ERR_AUTH_CERTIFICATE_INVALID"
	ErrCodeAuthIdentityMismatch   = "ERR_AUTH_IDENTITY_MISMATCH"
	ErrCodeAgentNotAssigned       = "ERR_AGENT_NOT_ASSIGNED"
	ErrCodeRoleIneligible         = "ERR_ROLE_INELIGIBLE"
	ErrCodeUnsupportedSchema      = "ERR_UNSUPPORTED_SCHEMA_VERSION"
	ErrCodeEmptyBatch             = "ERR_EMPTY_BATCH"
	ErrCodeBatchTooLarge          = "ERR_BATCH_TOO_LARGE"
	ErrCodeSequenceMismatch       = "ERR_SEQUENCE_MISMATCH"
	ErrCodeSequenceGap            = "ERR_SEQUENCE_GAP"
	ErrCodeRecordChecksumMismatch = "ERR_RECORD_CHECKSUM_MISMATCH"
	ErrCodeInvalidField           = "ERR_INVALID_FIELD"
	ErrCodeRecordTooLarge         = "ERR_RECORD_TOO_LARGE"
	ErrCodeReplayConflict         = "ERR_REPLAY_CONFLICT"
	ErrCodeSinkFailed             = "ERR_ACCEPTANCE_SINK_FAILED"
	ErrCodeInternalError          = "ERR_INTERNAL_ERROR"
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
