package agentreceiver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// ValidationError represents a typed validation failure.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
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
	if env.SchemaVersion != CurrentSchemaVersion {
		return &ValidationError{
			Code:    ErrCodeUnsupportedSchema,
			Message: fmt.Sprintf("unsupported schema version %d, expected %d", env.SchemaVersion, CurrentSchemaVersion),
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
