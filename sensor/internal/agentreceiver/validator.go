package agentreceiver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var agentStreamIDPattern = regexp.MustCompile(`^stream-[0-9a-f]{32}$`)

// ValidationError represents a typed validation failure.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ValidAgentStreamID reports whether id matches the BUILD-002 stream grammar.
func ValidAgentStreamID(id string) bool {
	return agentStreamIDPattern.MatchString(id)
}

// ComputeRecordChecksum computes the authoritative SHA-256 checksum for an agent record,
// matching the exact semantics established by agent/common/queue.
func ComputeRecordChecksum(seq uint64, id string, createdAt time.Time, payload []byte) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%d\n%s\n%s\n", seq, id, createdAt.UTC().Format(time.RFC3339Nano))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateEnvelope performs strict boundary and integrity checks on an incoming agent batch.
func ValidateEnvelope(env *DeliveryEnvelope, maxRecords int, maxRecordBytes int) *ValidationError {
	if env == nil {
		return &ValidationError{Code: ErrCodeEmptyBody, Message: "envelope is nil"}
	}
	switch env.SchemaVersion {
	case SchemaVersionV1, SchemaVersionV2:
		// supported transitional v1 and current v2
	default:
		return &ValidationError{
			Code:    ErrCodeUnsupportedSchema,
			Message: fmt.Sprintf("unsupported schema version %d; supported versions are %d and %d", env.SchemaVersion, SchemaVersionV1, SchemaVersionV2),
		}
	}
	if err := validateIdentifier("agentId", env.AgentID, 128); err != nil {
		return err
	}
	if err := validateIdentifier("tenantId", env.TenantID, 128); err != nil {
		return err
	}
	if env.SiteID != "" {
		if err := validateIdentifier("siteId", env.SiteID, 128); err != nil {
			return err
		}
	}
	if err := validateIdentifier("correlationId", env.CorrelationID, 128); err != nil {
		return err
	}
	if err := validateIdentifier("batchId", env.BatchID, 256); err != nil {
		return err
	}

	streamID := strings.TrimSpace(env.AgentStreamID)
	switch env.SchemaVersion {
	case SchemaVersionV1:
		if streamID != "" {
			return &ValidationError{
				Code:    ErrCodeInvalidField,
				Message: "agentStreamId must be absent for schemaVersion 1",
			}
		}
		env.AgentStreamID = ""
	case SchemaVersionV2:
		if streamID == "" {
			return &ValidationError{
				Code:    ErrCodeInvalidField,
				Message: "agentStreamId is required for schemaVersion 2",
			}
		}
		if !ValidAgentStreamID(streamID) {
			return &ValidationError{
				Code:    ErrCodeInvalidField,
				Message: "agentStreamId must match ^stream-[0-9a-f]{32}$",
			}
		}
		env.AgentStreamID = streamID
	}

	recordLen := len(env.Records)
	if recordLen == 0 {
		return &ValidationError{Code: ErrCodeEmptyBatch, Message: "batch contains zero records"}
	}
	if env.RecordCount != recordLen {
		return &ValidationError{
			Code:    ErrCodeSequenceMismatch,
			Message: fmt.Sprintf("recordCount header %d does not match records slice length %d", env.RecordCount, recordLen),
		}
	}
	if maxRecords > 0 && recordLen > maxRecords {
		return &ValidationError{
			Code:    ErrCodeBatchTooLarge,
			Message: fmt.Sprintf("batch contains %d records, exceeding maximum of %d", recordLen, maxRecords),
		}
	}
	if env.FirstSequence == 0 {
		return &ValidationError{Code: ErrCodeSequenceMismatch, Message: "firstSequence must be greater than zero"}
	}
	if env.LastSequence < env.FirstSequence {
		return &ValidationError{
			Code:    ErrCodeSequenceMismatch,
			Message: fmt.Sprintf("lastSequence %d cannot be less than firstSequence %d", env.LastSequence, env.FirstSequence),
		}
	}
	if env.FirstSequence > MAX_SAFE_SEQUENCE || env.LastSequence > MAX_SAFE_SEQUENCE {
		return &ValidationError{
			Code:    ErrCodeSequenceMismatch,
			Message: fmt.Sprintf("sequence range exceeds MAX_SAFE_SEQUENCE (%d)", MAX_SAFE_SEQUENCE),
		}
	}
	if env.Records[0].Sequence != env.FirstSequence {
		return &ValidationError{
			Code:    ErrCodeSequenceMismatch,
			Message: fmt.Sprintf("first record sequence %d does not match batch firstSequence %d", env.Records[0].Sequence, env.FirstSequence),
		}
	}
	if env.Records[recordLen-1].Sequence != env.LastSequence {
		return &ValidationError{
			Code:    ErrCodeSequenceMismatch,
			Message: fmt.Sprintf("last record sequence %d does not match batch lastSequence %d", env.Records[recordLen-1].Sequence, env.LastSequence),
		}
	}

	// Verify monotonic sequence ordering and integrity of every record
	for i := 0; i < recordLen; i++ {
		rec := &env.Records[i]
		if i > 0 {
			prevSeq := env.Records[i-1].Sequence
			if rec.Sequence != prevSeq+1 {
				return &ValidationError{
					Code:    ErrCodeSequenceGap,
					Message: fmt.Sprintf("sequence gap or out-of-order at index %d: expected %d, got %d", i, prevSeq+1, rec.Sequence),
				}
			}
		}
		if rec.Sequence > MAX_SAFE_SEQUENCE {
			return &ValidationError{
				Code:    ErrCodeSequenceMismatch,
				Message: fmt.Sprintf("records[%d].sequence exceeds MAX_SAFE_SEQUENCE (%d)", i, MAX_SAFE_SEQUENCE),
			}
		}

		if err := validateIdentifier(fmt.Sprintf("records[%d].id", i), rec.ID, 128); err != nil {
			return err
		}
		if rec.CreatedAt.IsZero() {
			return &ValidationError{
				Code:    ErrCodeInvalidField,
				Message: fmt.Sprintf("records[%d].createdAt must not be zero", i),
			}
		}
		payloadLen := len(rec.Payload)
		if payloadLen == 0 {
			return &ValidationError{
				Code:    ErrCodeInvalidField,
				Message: fmt.Sprintf("records[%d].payload must not be empty", i),
			}
		}
		if maxRecordBytes > 0 && payloadLen > maxRecordBytes {
			return &ValidationError{
				Code:    ErrCodeRecordTooLarge,
				Message: fmt.Sprintf("records[%d].payload size %d exceeds limit of %d bytes", i, payloadLen, maxRecordBytes),
			}
		}
		if len(rec.Checksum) != 64 || !isHex(rec.Checksum) {
			return &ValidationError{
				Code:    ErrCodeRecordChecksumMismatch,
				Message: fmt.Sprintf("records[%d].checksum must be a 64-character hex string", i),
			}
		}

		// Recompute record SHA-256 and verify exact match
		computed := ComputeRecordChecksum(rec.Sequence, rec.ID, rec.CreatedAt, rec.Payload)
		if !strings.EqualFold(computed, rec.Checksum) {
			return &ValidationError{
				Code:    ErrCodeRecordChecksumMismatch,
				Message: fmt.Sprintf("records[%d] checksum verification failed (sequence %d)", i, rec.Sequence),
			}
		}
	}

	return nil
}

func validateIdentifier(field, val string, maxLen int) *ValidationError {
	val = strings.TrimSpace(val)
	if val == "" {
		return &ValidationError{
			Code:    ErrCodeInvalidField,
			Message: fmt.Sprintf("%s must not be empty", field),
		}
	}
	if len(val) > maxLen {
		return &ValidationError{
			Code:    ErrCodeInvalidField,
			Message: fmt.Sprintf("%s length %d exceeds maximum of %d", field, len(val), maxLen),
		}
	}
	for _, r := range val {
		if unicode.IsControl(r) {
			return &ValidationError{
				Code:    ErrCodeInvalidField,
				Message: fmt.Sprintf("%s contains control characters", field),
			}
		}
	}
	return nil
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}
