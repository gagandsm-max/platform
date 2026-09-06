package observation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const SchemaVersion = 1

type ErrorCode string

const (
	CodeInvalidEnvelope ErrorCode = "OBSERVATION_INVALID_ENVELOPE"
	CodeResourceLimit   ErrorCode = "OBSERVATION_RESOURCE_LIMIT"
	CodeCancelled       ErrorCode = "OBSERVATION_CANCELLED"
	CodeDeadline        ErrorCode = "OBSERVATION_DEADLINE_EXCEEDED"
)

type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
func CodeOf(err error) ErrorCode {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return ""
}

type JobCorrelation struct {
	JobID             string `json:"jobId"`
	AssignmentKey     string `json:"assignmentKey"`
	AssignmentVersion uint64 `json:"assignmentVersion"`
	Attempt           uint32 `json:"attempt"`
}

type Envelope struct {
	SchemaVersion         int             `json:"schemaVersion"`
	BatchID               string          `json:"batchId"`
	RecordType            string          `json:"recordType"`
	CollectedAt           time.Time       `json:"collectedAt"`
	SensorID              string          `json:"sensorId,omitempty"`
	TenantID              string          `json:"tenantId"`
	EntityID              string          `json:"entityId,omitempty"`
	Job                   JobCorrelation  `json:"job"`
	Protocol              string          `json:"protocol"`
	ProtocolVersion       string          `json:"protocolVersion"`
	Operation             string          `json:"operation"`
	Status                string          `json:"status"`
	Quality               string          `json:"quality"`
	ProtocolSchemaVersion int             `json:"protocolSchemaVersion"`
	ProtocolPayload       json.RawMessage `json:"protocolPayload"`
}

func (e Envelope) Validate() error {
	if e.SchemaVersion != SchemaVersion {
		return &Error{Code: CodeInvalidEnvelope, Message: "unsupported observation envelope schema version"}
	}
	if e.BatchID == "" || len(e.BatchID) > 128 {
		return &Error{Code: CodeInvalidEnvelope, Message: "batch ID must be 1..128 bytes"}
	}
	if e.RecordType == "" || len(e.RecordType) > 128 {
		return &Error{Code: CodeInvalidEnvelope, Message: "record type must be 1..128 bytes"}
	}
	if e.CollectedAt.IsZero() {
		return &Error{Code: CodeInvalidEnvelope, Message: "collection timestamp is required"}
	}
	if e.TenantID == "" || len(e.TenantID) > 256 {
		return &Error{Code: CodeInvalidEnvelope, Message: "tenant ID must be 1..256 bytes"}
	}
	if len(e.SensorID) > 256 || len(e.EntityID) > 256 {
		return &Error{Code: CodeInvalidEnvelope, Message: "sensor/entity identity exceeds configured envelope bound"}
	}
	if e.Job.JobID == "" || len(e.Job.JobID) > 256 || e.Job.AssignmentKey == "" || len(e.Job.AssignmentKey) > 256 {
		return &Error{Code: CodeInvalidEnvelope, Message: "job correlation identity is invalid"}
	}
	if e.Protocol == "" || len(e.Protocol) > 64 || e.ProtocolVersion == "" || len(e.ProtocolVersion) > 64 || e.Operation == "" || len(e.Operation) > 64 {
		return &Error{Code: CodeInvalidEnvelope, Message: "protocol correlation fields are invalid"}
	}
	if e.Status == "" || len(e.Status) > 64 || e.Quality == "" || len(e.Quality) > 64 {
		return &Error{Code: CodeInvalidEnvelope, Message: "status/quality fields are invalid"}
	}
	if e.ProtocolSchemaVersion <= 0 {
		return &Error{Code: CodeInvalidEnvelope, Message: "protocol schema version must be positive"}
	}
	if len(e.ProtocolPayload) == 0 || !json.Valid(e.ProtocolPayload) {
		return &Error{Code: CodeInvalidEnvelope, Message: "protocol payload must be valid non-empty JSON"}
	}
	return nil
}

func EncodeBounded(ctx context.Context, envelope Envelope, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, &Error{Code: CodeResourceLimit, Message: "maximum encoded observation bytes must be positive"}
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := envelope.Validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, &Error{Code: CodeInvalidEnvelope, Message: "observation envelope cannot be encoded", Cause: err}
	}
	if len(payload) > maxBytes {
		return nil, &Error{Code: CodeResourceLimit, Message: "encoded observation envelope exceeds configured maximum"}
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return payload, nil
}

func DecodeBounded(payload []byte, maxBytes int) (Envelope, error) {
	if maxBytes <= 0 || len(payload) == 0 || len(payload) > maxBytes {
		return Envelope{}, &Error{Code: CodeResourceLimit, Message: "encoded observation envelope is outside configured bounds"}
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var envelope Envelope
	if err := dec.Decode(&envelope); err != nil {
		return Envelope{}, &Error{Code: CodeInvalidEnvelope, Message: "observation envelope is invalid", Cause: err}
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Envelope{}, &Error{Code: CodeInvalidEnvelope, Message: "observation envelope contains trailing data", Cause: err}
	}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return &Error{Code: CodeInvalidEnvelope, Message: "context must not be nil"}
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return &Error{Code: CodeDeadline, Message: "observation operation deadline exceeded", Cause: err}
		}
		return &Error{Code: CodeCancelled, Message: "observation operation cancelled", Cause: err}
	}
	return nil
}
