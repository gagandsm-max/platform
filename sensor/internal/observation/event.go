package observation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const EventEnvelopeSchemaVersion = 1

type EventEnvelopeErrorCode string

const (
	EventEnvelopeInvalid       EventEnvelopeErrorCode = "EVENT_ENVELOPE_INVALID"
	EventEnvelopeResourceLimit EventEnvelopeErrorCode = "EVENT_ENVELOPE_RESOURCE_LIMIT"
)

type EventEnvelopeError struct {
	Code    EventEnvelopeErrorCode
	Message string
	Cause   error
}

func (e *EventEnvelopeError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *EventEnvelopeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type EventSource struct {
	SensorID      string `json:"sensorId,omitempty"`
	TenantID      string `json:"tenantId,omitempty"`
	EntityID      string `json:"entityId,omitempty"`
	Transport     string `json:"transport,omitempty"`
	SourceAddress string `json:"sourceAddress,omitempty"`
}

type EventEnvelope struct {
	SchemaVersion        int             `json:"schemaVersion"`
	ReceivedAt           time.Time       `json:"receivedAt"`
	EventTime            *time.Time      `json:"eventTime,omitempty"`
	Source               EventSource     `json:"source"`
	Protocol             string          `json:"protocol"`
	Format               string          `json:"format"`
	Quality              string          `json:"quality"`
	StatusCode           string          `json:"statusCode,omitempty"`
	PayloadSchemaVersion int             `json:"payloadSchemaVersion"`
	Payload              json.RawMessage `json:"payload"`
}

func (e EventEnvelope) Validate(maxPayloadBytes int) error {
	if e.SchemaVersion != EventEnvelopeSchemaVersion {
		return &EventEnvelopeError{Code: EventEnvelopeInvalid, Message: "unsupported event envelope schema version"}
	}
	if e.ReceivedAt.IsZero() {
		return &EventEnvelopeError{Code: EventEnvelopeInvalid, Message: "received timestamp is required"}
	}
	if e.Protocol == "" || len(e.Protocol) > 64 || e.Format == "" || len(e.Format) > 64 || e.Quality == "" || len(e.Quality) > 64 {
		return &EventEnvelopeError{Code: EventEnvelopeInvalid, Message: "protocol/format/quality fields are invalid"}
	}
	if len(e.StatusCode) > 128 || len(e.Source.SensorID) > 256 || len(e.Source.TenantID) > 256 || len(e.Source.EntityID) > 256 || len(e.Source.Transport) > 32 || len(e.Source.SourceAddress) > 512 {
		return &EventEnvelopeError{Code: EventEnvelopeInvalid, Message: "event envelope metadata exceeds fixed safety bounds"}
	}
	if e.PayloadSchemaVersion <= 0 {
		return &EventEnvelopeError{Code: EventEnvelopeInvalid, Message: "payload schema version must be positive"}
	}
	if maxPayloadBytes <= 0 {
		return &EventEnvelopeError{Code: EventEnvelopeResourceLimit, Message: "maximum event payload bytes must be positive"}
	}
	if len(e.Payload) == 0 || len(e.Payload) > maxPayloadBytes || !json.Valid(e.Payload) {
		return &EventEnvelopeError{Code: EventEnvelopeResourceLimit, Message: "event payload is invalid or outside configured bound"}
	}
	return nil
}

func EncodeEventEnvelope(envelope EventEnvelope, maxEnvelopeBytes, maxPayloadBytes int) ([]byte, error) {
	if maxEnvelopeBytes <= 0 {
		return nil, &EventEnvelopeError{Code: EventEnvelopeResourceLimit, Message: "maximum encoded event envelope bytes must be positive"}
	}
	if err := envelope.Validate(maxPayloadBytes); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, &EventEnvelopeError{Code: EventEnvelopeInvalid, Message: "event envelope cannot be encoded", Cause: err}
	}
	if len(payload) > maxEnvelopeBytes {
		return nil, &EventEnvelopeError{Code: EventEnvelopeResourceLimit, Message: "encoded event envelope exceeds configured maximum"}
	}
	return payload, nil
}

func DecodeEventEnvelope(payload []byte, maxEnvelopeBytes, maxPayloadBytes int) (EventEnvelope, error) {
	if maxEnvelopeBytes <= 0 || len(payload) == 0 || len(payload) > maxEnvelopeBytes {
		return EventEnvelope{}, &EventEnvelopeError{Code: EventEnvelopeResourceLimit, Message: "encoded event envelope is outside configured bounds"}
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var envelope EventEnvelope
	if err := dec.Decode(&envelope); err != nil {
		return EventEnvelope{}, &EventEnvelopeError{Code: EventEnvelopeInvalid, Message: "event envelope is invalid", Cause: err}
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return EventEnvelope{}, &EventEnvelopeError{Code: EventEnvelopeInvalid, Message: "event envelope contains trailing data", Cause: err}
	}
	if err := envelope.Validate(maxPayloadBytes); err != nil {
		return EventEnvelope{}, err
	}
	return envelope, nil
}
