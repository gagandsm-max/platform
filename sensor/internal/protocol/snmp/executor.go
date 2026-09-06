package snmp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"platform/sensor/internal/scheduler"
)

const (
	PayloadSchemaVersion = 1
	SchedulerJobKind     = scheduler.JobKind("SNMP")
)

type Executor struct {
	collector       *Collector
	maxPayloadBytes int
}

var _ scheduler.Executor = (*Executor)(nil)

func NewExecutor(collector *Collector, maxPayloadBytes int) (*Executor, error) {
	if collector == nil {
		return nil, newError(CodeInvalidRequest, "collector must not be nil", nil, false)
	}
	if maxPayloadBytes <= 0 {
		return nil, newError(CodeInvalidRequest, "maximum scheduler payload bytes must be greater than zero", nil, false)
	}
	return &Executor{collector: collector, maxPayloadBytes: maxPayloadBytes}, nil
}

func (e *Executor) Execute(ctx context.Context, job scheduler.Job) (scheduler.ExecutionResult, error) {
	if e == nil || e.collector == nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "SNMP executor is not initialized", nil, false))
	}
	if job.Class != scheduler.ClassNetworkPoll || job.Kind != SchedulerJobKind {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "scheduler routing is not valid for the SNMP executor", nil, false))
	}
	request, err := DecodeSchedulerPayload(job.Payload, e.maxPayloadBytes)
	if err != nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(err)
	}
	result, err := e.collector.Collect(ctx, request)
	if err != nil {
		return scheduler.ExecutionResult{}, mapSchedulerError(err)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeMalformedResponse, "normalized SNMP result cannot be encoded", err, false))
	}
	if len(payload) > request.Limits.MaxResultBytes {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeResourceLimit, "encoded SNMP result exceeds configured maximum", nil, false))
	}
	return scheduler.ExecutionResult{Payload: payload}, nil
}

func mapSchedulerError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if IsTransient(err) {
		return scheduler.Transient(err)
	}
	return scheduler.Permanent(err)
}

type schedulerPayload struct {
	SchemaVersion          int                `json:"schemaVersion"`
	Target                 string             `json:"target"`
	Port                   int                `json:"port"`
	Version                Version            `json:"version"`
	Operation              Operation          `json:"operation"`
	OIDs                   []string           `json:"oids"`
	TimeoutMilliseconds    int64              `json:"timeoutMilliseconds"`
	ExecutionTimeoutMillis int64              `json:"executionTimeoutMilliseconds"`
	Retry                  retryPayload       `json:"retry"`
	Limits                 limitsPayload      `json:"limits"`
	Credentials            credentialsPayload `json:"credentials"`
	CorrelationID          string             `json:"correlationId,omitempty"`
}

type retryPayload struct {
	MaxAttempts                int     `json:"maxAttempts"`
	InitialBackoffMilliseconds int64   `json:"initialBackoffMilliseconds"`
	MaxBackoffMilliseconds     int64   `json:"maxBackoffMilliseconds"`
	JitterFraction             float64 `json:"jitterFraction"`
}

type limitsPayload struct {
	MaxOIDs            int    `json:"maxOids"`
	MaxWalkEntries     int    `json:"maxWalkEntries"`
	MaxBulkRepetitions uint32 `json:"maxBulkRepetitions"`
	MaxResults         int    `json:"maxResults"`
	MaxResultBytes     int    `json:"maxResultBytes"`
}

type credentialsPayload struct {
	Community string                `json:"community,omitempty"`
	V3        *v3CredentialsPayload `json:"v3,omitempty"`
}

type v3CredentialsPayload struct {
	Username        string          `json:"username"`
	SecurityLevel   SecurityLevel   `json:"securityLevel"`
	AuthProtocol    AuthProtocol    `json:"authProtocol"`
	AuthSecret      string          `json:"authSecret,omitempty"`
	PrivacyProtocol PrivacyProtocol `json:"privacyProtocol"`
	PrivacySecret   string          `json:"privacySecret,omitempty"`
	ContextName     string          `json:"contextName,omitempty"`
}

func DecodeSchedulerPayload(payload []byte, maxBytes int) (Request, error) {
	if maxBytes <= 0 {
		return Request{}, newError(CodeInvalidRequest, "scheduler payload limit must be greater than zero", nil, false)
	}
	if len(payload) == 0 {
		return Request{}, newError(CodeInvalidRequest, "scheduler SNMP payload is empty", nil, false)
	}
	if len(payload) > maxBytes {
		return Request{}, newError(CodeResourceLimit, "scheduler SNMP payload exceeds configured maximum", nil, false)
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var wire schedulerPayload
	if err := decoder.Decode(&wire); err != nil {
		return Request{}, newError(CodeInvalidRequest, "scheduler SNMP payload is invalid", err, false)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Request{}, newError(CodeInvalidRequest, "scheduler SNMP payload contains trailing data", err, false)
	}
	if wire.SchemaVersion != PayloadSchemaVersion {
		return Request{}, newError(CodeInvalidRequest, "unsupported scheduler SNMP payload schema version", nil, false)
	}

	timeout, err := durationFromMilliseconds(wire.TimeoutMilliseconds, "timeoutMilliseconds")
	if err != nil {
		return Request{}, err
	}
	executionTimeout, err := durationFromMilliseconds(wire.ExecutionTimeoutMillis, "executionTimeoutMilliseconds")
	if err != nil {
		return Request{}, err
	}
	initialBackoff, err := optionalDurationFromMilliseconds(wire.Retry.InitialBackoffMilliseconds, "initialBackoffMilliseconds")
	if err != nil {
		return Request{}, err
	}
	maxBackoff, err := optionalDurationFromMilliseconds(wire.Retry.MaxBackoffMilliseconds, "maxBackoffMilliseconds")
	if err != nil {
		return Request{}, err
	}

	credentials := Credentials{}
	if wire.Credentials.Community != "" {
		credentials.Community = &CommunityCredentials{Community: Secret(wire.Credentials.Community)}
	}
	if wire.Credentials.V3 != nil {
		credentials.V3 = &V3Credentials{
			Username:        wire.Credentials.V3.Username,
			SecurityLevel:   wire.Credentials.V3.SecurityLevel,
			AuthProtocol:    wire.Credentials.V3.AuthProtocol,
			AuthSecret:      Secret(wire.Credentials.V3.AuthSecret),
			PrivacyProtocol: wire.Credentials.V3.PrivacyProtocol,
			PrivacySecret:   Secret(wire.Credentials.V3.PrivacySecret),
			ContextName:     wire.Credentials.V3.ContextName,
		}
	}
	request := Request{
		Target:           wire.Target,
		Port:             wire.Port,
		Version:          wire.Version,
		Credentials:      credentials,
		OIDs:             append([]string(nil), wire.OIDs...),
		Operation:        wire.Operation,
		Timeout:          timeout,
		ExecutionTimeout: executionTimeout,
		Retry: RetryPolicy{
			MaxAttempts:    wire.Retry.MaxAttempts,
			InitialBackoff: initialBackoff,
			MaxBackoff:     maxBackoff,
			JitterFraction: wire.Retry.JitterFraction,
		},
		Limits: Limits{
			MaxOIDs:            wire.Limits.MaxOIDs,
			MaxWalkEntries:     wire.Limits.MaxWalkEntries,
			MaxBulkRepetitions: wire.Limits.MaxBulkRepetitions,
			MaxResults:         wire.Limits.MaxResults,
			MaxResultBytes:     wire.Limits.MaxResultBytes,
		},
		CorrelationID: wire.CorrelationID,
	}
	if err := ValidateRequest(request); err != nil {
		return Request{}, err
	}
	return request, nil
}

// EncodeSchedulerPayload is the canonical internal encoder for the opaque
// scheduler boundary. The returned bytes contain credentials and therefore must
// be treated as secret-bearing opaque data and never logged.
func EncodeSchedulerPayload(request Request) ([]byte, error) {
	if err := ValidateRequest(request); err != nil {
		return nil, err
	}
	wire := schedulerPayload{
		SchemaVersion:          PayloadSchemaVersion,
		Target:                 request.Target,
		Port:                   request.Port,
		Version:                request.Version,
		Operation:              request.Operation,
		OIDs:                   append([]string(nil), request.OIDs...),
		TimeoutMilliseconds:    request.Timeout.Milliseconds(),
		ExecutionTimeoutMillis: request.ExecutionTimeout.Milliseconds(),
		Retry: retryPayload{
			MaxAttempts:                request.Retry.MaxAttempts,
			InitialBackoffMilliseconds: request.Retry.InitialBackoff.Milliseconds(),
			MaxBackoffMilliseconds:     request.Retry.MaxBackoff.Milliseconds(),
			JitterFraction:             request.Retry.JitterFraction,
		},
		Limits: limitsPayload{
			MaxOIDs:            request.Limits.MaxOIDs,
			MaxWalkEntries:     request.Limits.MaxWalkEntries,
			MaxBulkRepetitions: request.Limits.MaxBulkRepetitions,
			MaxResults:         request.Limits.MaxResults,
			MaxResultBytes:     request.Limits.MaxResultBytes,
		},
		CorrelationID: request.CorrelationID,
	}
	if request.Credentials.Community != nil {
		wire.Credentials.Community = request.Credentials.Community.Community.value()
	}
	if request.Credentials.V3 != nil {
		v3 := request.Credentials.V3
		wire.Credentials.V3 = &v3CredentialsPayload{
			Username:        v3.Username,
			SecurityLevel:   v3.SecurityLevel,
			AuthProtocol:    v3.AuthProtocol,
			AuthSecret:      v3.AuthSecret.value(),
			PrivacyProtocol: v3.PrivacyProtocol,
			PrivacySecret:   v3.PrivacySecret.value(),
			ContextName:     v3.ContextName,
		}
	}
	return json.Marshal(wire)
}

func optionalDurationFromMilliseconds(value int64, field string) (time.Duration, error) {
	if value < 0 {
		return 0, newError(CodeInvalidRequest, field+" must not be negative", nil, false)
	}
	if value == 0 {
		return 0, nil
	}
	return durationFromMilliseconds(value, field)
}
