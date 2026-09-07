package snmpintegration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"platform/sensor/internal/observation"
	protocolsnmp "platform/sensor/internal/protocol/snmp"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
)

const durableRecordType = "SNMP_OBSERVATION_BATCH"

var _ scheduler.Executor = (*Executor)(nil)

type ErrorCode string

const (
	CodeInvalidConfig      ErrorCode = "SNMP_INTEGRATION_INVALID_CONFIG"
	CodeInvalidResult      ErrorCode = "SNMP_INTEGRATION_INVALID_RESULT"
	CodeResourceLimit      ErrorCode = "SNMP_INTEGRATION_RESOURCE_LIMIT"
	CodeStorageProtection  ErrorCode = "SNMP_INTEGRATION_STORAGE_PROTECTION"
	CodeStorageUnavailable ErrorCode = "SNMP_INTEGRATION_STORAGE_UNAVAILABLE"
	CodeStorageCorruption  ErrorCode = "SNMP_INTEGRATION_STORAGE_CORRUPTION"
	CodeCancelled          ErrorCode = "SNMP_INTEGRATION_CANCELLED"
	CodeDeadline           ErrorCode = "SNMP_INTEGRATION_DEADLINE_EXCEEDED"
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

type Config struct {
	SensorID           string
	MaxSNMPResultBytes int
	MaxEnvelopeBytes   int
}

func (c Config) validate() error {
	if len(c.SensorID) > 256 {
		return &Error{Code: CodeInvalidConfig, Message: "sensor ID exceeds configured integration bound"}
	}
	if c.MaxSNMPResultBytes <= 0 || c.MaxEnvelopeBytes <= 0 {
		return &Error{Code: CodeInvalidConfig, Message: "SNMP result and envelope bounds must be explicitly positive"}
	}
	return nil
}

type Executor struct {
	base      scheduler.Executor
	sink      observation.Sink
	cfg       Config
	component *Component
}

func NewExecutor(base scheduler.Executor, sink observation.Sink, cfg Config) (*Executor, error) {
	if base == nil || sink == nil {
		return nil, &Error{Code: CodeInvalidConfig, Message: "low-level SNMP executor and observation sink are required"}
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if sink.MaxPayloadBytes() <= 0 {
		return nil, &Error{Code: CodeInvalidConfig, Message: "observation sink must expose a positive payload bound"}
	}
	exec := &Executor{base: base, sink: sink, cfg: cfg}
	exec.component = newComponent(exec)
	return exec, nil
}

func (e *Executor) Component() *Component {
	if e == nil {
		return nil
	}
	return e.component
}

func (e *Executor) Execute(ctx context.Context, job scheduler.Job) (scheduler.ExecutionResult, error) {
	if e == nil || e.base == nil || e.sink == nil || e.component == nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(&Error{Code: CodeInvalidConfig, Message: "SNMP integration executor is not initialized"})
	}
	if job.Class != scheduler.ClassNetworkPoll || job.Kind != protocolsnmp.SchedulerJobKind {
		err := &Error{Code: CodeInvalidConfig, Message: "SNMP integration requires NETWORK_POLL class and SNMP job kind"}
		return scheduler.ExecutionResult{}, scheduler.Permanent(err)
	}
	e.component.recordExecution()
	if err := contextError(ctx); err != nil {
		e.component.recordFailure(stableErrorCode(err), false)
		return scheduler.ExecutionResult{}, err
	}

	result, err := e.base.Execute(ctx, job)
	if err != nil {
		e.component.recordFailure(stableErrorCode(err), false)
		return scheduler.ExecutionResult{}, err
	}
	if len(result.Payload) == 0 || len(result.Payload) > e.cfg.MaxSNMPResultBytes {
		err := &Error{Code: CodeResourceLimit, Message: "normalized SNMP result is outside configured integration bounds"}
		e.component.recordFailure(string(err.Code), false)
		return scheduler.ExecutionResult{}, scheduler.Permanent(err)
	}
	decoded, err := decodeResult(result.Payload, e.cfg.MaxSNMPResultBytes)
	if err != nil {
		e.component.recordFailure(stableErrorCode(err), false)
		return scheduler.ExecutionResult{}, scheduler.Permanent(err)
	}

	envelope := buildEnvelope(e.cfg.SensorID, job, decoded, result.Payload)
	encodedLimit := e.cfg.MaxEnvelopeBytes
	if sinkLimit := e.sink.MaxPayloadBytes(); sinkLimit < encodedLimit {
		encodedLimit = sinkLimit
	}
	if _, err := observation.EncodeBounded(ctx, envelope, encodedLimit); err != nil {
		mapped := mapObservationError(err)
		e.component.recordFailure(stableErrorCode(mapped), false)
		return scheduler.ExecutionResult{}, mapped
	}

	receipt, err := e.sink.Write(ctx, envelope)
	if err != nil {
		mapped := mapSinkError(err)
		e.component.recordFailure(stableErrorCode(mapped), true)
		return scheduler.ExecutionResult{}, mapped
	}
	e.component.recordSuccess()

	receiptPayload, err := json.Marshal(durableReceipt{
		SchemaVersion:    1,
		BatchID:          envelope.BatchID,
		SpoolSequence:    receipt.Sequence,
		RecordID:         receipt.RecordID,
		ObservationCount: len(decoded.Observations),
	})
	if err != nil {
		// The durable record is already committed. Do not convert durable success
		// into a retry that could create another copy; return a payload-less success.
		return scheduler.ExecutionResult{}, nil
	}
	return scheduler.ExecutionResult{Payload: receiptPayload}, nil
}

type durableReceipt struct {
	SchemaVersion    int    `json:"schemaVersion"`
	BatchID          string `json:"batchId"`
	SpoolSequence    uint64 `json:"spoolSequence"`
	RecordID         string `json:"recordId"`
	ObservationCount int    `json:"observationCount"`
}

type resultWire struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Version       protocolsnmp.Version   `json:"version"`
	Operation     protocolsnmp.Operation `json:"operation"`
	CollectedAt   time.Time              `json:"collectedAt"`
	Observations  []observationWire      `json:"observations"`
}

type observationWire struct {
	OID         string                         `json:"oid"`
	SourceOID   string                         `json:"sourceOid"`
	BERType     uint8                          `json:"berType"`
	BERTypeName string                         `json:"berTypeName"`
	CollectedAt time.Time                      `json:"collectedAt"`
	Quality     protocolsnmp.Quality           `json:"quality"`
	Status      protocolsnmp.ObservationStatus `json:"status"`
	ErrorCode   protocolsnmp.ErrorCode         `json:"errorCode,omitempty"`
	Value       valueWire                      `json:"value"`
}

type valueWire struct {
	Kind      protocolsnmp.ValueKind `json:"kind"`
	Integer   *int64                 `json:"integer,omitempty"`
	Unsigned  *uint64                `json:"unsigned,omitempty"`
	Counter64 *string                `json:"counter64,omitempty"`
	Bytes     []byte                 `json:"bytes,omitempty"`
	Text      *string                `json:"text,omitempty"`
	BitLength *int                   `json:"bitLength,omitempty"`
}

func decodeResult(payload []byte, maxBytes int) (resultWire, error) {
	if maxBytes <= 0 || len(payload) == 0 || len(payload) > maxBytes {
		return resultWire{}, &Error{Code: CodeResourceLimit, Message: "normalized SNMP result exceeds configured maximum"}
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var result resultWire
	if err := dec.Decode(&result); err != nil {
		return resultWire{}, &Error{Code: CodeInvalidResult, Message: "normalized SNMP result is invalid", Cause: err}
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return resultWire{}, &Error{Code: CodeInvalidResult, Message: "normalized SNMP result contains trailing data", Cause: err}
	}
	if result.SchemaVersion != protocolsnmp.ResultSchemaVersion {
		return resultWire{}, &Error{Code: CodeInvalidResult, Message: "unsupported normalized SNMP result schema version"}
	}
	if !validVersion(result.Version) || !validOperation(result.Operation) || result.CollectedAt.IsZero() {
		return resultWire{}, &Error{Code: CodeInvalidResult, Message: "normalized SNMP result has invalid required metadata"}
	}
	for _, obs := range result.Observations {
		if strings.TrimSpace(obs.OID) == "" || strings.TrimSpace(obs.SourceOID) == "" || obs.BERTypeName == "" || obs.CollectedAt.IsZero() || !validQuality(obs.Quality) || !validStatus(obs.Status) || !validValueKind(obs.Value.Kind) {
			return resultWire{}, &Error{Code: CodeInvalidResult, Message: "normalized SNMP observation has invalid required fields"}
		}
		if obs.Value.Counter64 != nil {
			if _, err := strconv.ParseUint(*obs.Value.Counter64, 10, 64); err != nil {
				return resultWire{}, &Error{Code: CodeInvalidResult, Message: "Counter64 value is not an exact unsigned decimal representation", Cause: err}
			}
		}
		if obs.Value.Kind == protocolsnmp.ValueCounter64 && obs.Value.Counter64 == nil {
			return resultWire{}, &Error{Code: CodeInvalidResult, Message: "Counter64 observation is missing exact decimal value"}
		}
	}
	return result, nil
}

func validVersion(v protocolsnmp.Version) bool {
	switch v {
	case protocolsnmp.Version1, protocolsnmp.Version2c, protocolsnmp.Version3:
		return true
	default:
		return false
	}
}

func validOperation(v protocolsnmp.Operation) bool {
	switch v {
	case protocolsnmp.OperationGet, protocolsnmp.OperationGetNext, protocolsnmp.OperationGetBulk, protocolsnmp.OperationWalk, protocolsnmp.OperationBulkWalk:
		return true
	default:
		return false
	}
}

func validQuality(v protocolsnmp.Quality) bool {
	switch v {
	case protocolsnmp.QualityGood, protocolsnmp.QualityDegraded, protocolsnmp.QualityUnknown:
		return true
	default:
		return false
	}
}

func validStatus(v protocolsnmp.ObservationStatus) bool {
	switch v {
	case protocolsnmp.ObservationValue, protocolsnmp.ObservationException:
		return true
	default:
		return false
	}
}

func validValueKind(v protocolsnmp.ValueKind) bool {
	switch v {
	case protocolsnmp.ValueInteger, protocolsnmp.ValueUnsigned, protocolsnmp.ValueCounter64, protocolsnmp.ValueOctets, protocolsnmp.ValueBitString, protocolsnmp.ValueOID, protocolsnmp.ValueIPAddress, protocolsnmp.ValueNull, protocolsnmp.ValueOpaque, protocolsnmp.ValueException:
		return true
	default:
		return false
	}
}

func buildEnvelope(sensorID string, job scheduler.Job, result resultWire, raw []byte) observation.Envelope {
	status := "COLLECTED"
	quality := "GOOD"
	for _, obs := range result.Observations {
		if obs.Status == protocolsnmp.ObservationException {
			status = "COLLECTED_WITH_EXCEPTIONS"
			if quality == "GOOD" {
				quality = "DEGRADED"
			}
		}
		if obs.Quality == protocolsnmp.QualityUnknown {
			quality = "UNKNOWN"
		} else if obs.Quality == protocolsnmp.QualityDegraded && quality == "GOOD" {
			quality = "DEGRADED"
		}
	}
	return observation.Envelope{
		SchemaVersion: observation.SchemaVersion,
		BatchID:       deterministicBatchID(job),
		RecordType:    durableRecordType,
		CollectedAt:   result.CollectedAt.UTC(),
		SensorID:      sensorID,
		TenantID:      string(job.TenantID),
		EntityID:      string(job.EntityID),
		Job: observation.JobCorrelation{
			JobID:             string(job.JobID),
			AssignmentKey:     string(job.AssignmentKey),
			AssignmentVersion: job.AssignmentVersion,
			Attempt:           job.Attempt,
		},
		Protocol:              "SNMP",
		ProtocolVersion:       string(result.Version),
		Operation:             string(result.Operation),
		Status:                status,
		Quality:               quality,
		ProtocolSchemaVersion: result.SchemaVersion,
		ProtocolPayload:       append(json.RawMessage(nil), raw...),
	}
}

func deterministicBatchID(job scheduler.Job) string {
	h := sha256.New()
	parts := []string{"snmp-observation-v1", string(job.TenantID), string(job.EntityID), string(job.AssignmentKey), string(job.JobID), strconv.FormatUint(job.AssignmentVersion, 10)}
	for _, part := range parts {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(part))
	}
	sum := h.Sum(nil)
	return "snmp-" + hex.EncodeToString(sum[:16])
}

func mapObservationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	switch observation.CodeOf(err) {
	case observation.CodeResourceLimit:
		return scheduler.Permanent(&Error{Code: CodeResourceLimit, Message: "observation envelope exceeds configured durable bound", Cause: err})
	case observation.CodeCancelled:
		return context.Canceled
	case observation.CodeDeadline:
		return context.DeadlineExceeded
	default:
		return scheduler.Permanent(&Error{Code: CodeInvalidResult, Message: "observation envelope is invalid", Cause: err})
	}
}

func mapSinkError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	switch spool.CodeOf(err) {
	case spool.CodeProtection:
		return scheduler.Permanent(&Error{Code: CodeStorageProtection, Message: "Sensor storage protection prevents durable SNMP collection", Cause: err})
	case spool.CodeCorruption, spool.CodeSequence, spool.CodeUnsafePath:
		return scheduler.Permanent(&Error{Code: CodeStorageCorruption, Message: "durable Sensor spool is corrupt or unsafe", Cause: err})
	case spool.CodeClosed, spool.CodeInvalidConfig, spool.CodeInvalidRecord, spool.CodeIO:
		// Conservatively permanent/degraded. Storage errors are not retried unless a
		// future platform-owned classifier can prove a specific condition transient.
		return scheduler.Permanent(&Error{Code: CodeStorageUnavailable, Message: "durable Sensor spool is unavailable", Cause: err})
	default:
		if observation.CodeOf(err) == observation.CodeResourceLimit {
			return scheduler.Permanent(&Error{Code: CodeResourceLimit, Message: "observation sink rejected encoded result bound", Cause: err})
		}
		return scheduler.Permanent(&Error{Code: CodeStorageUnavailable, Message: "durable Sensor observation write failed", Cause: err})
	}
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return scheduler.Permanent(&Error{Code: CodeInvalidConfig, Message: "context must not be nil"})
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		return context.Canceled
	}
	return nil
}

func stableErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var integ *Error
	if errors.As(err, &integ) {
		return string(integ.Code)
	}
	var snmpErr *protocolsnmp.Error
	if errors.As(err, &snmpErr) {
		return string(snmpErr.Code)
	}
	var spoolErr *spool.Error
	if errors.As(err, &spoolErr) {
		return string(spoolErr.Code)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return string(CodeDeadline)
	}
	if errors.Is(err, context.Canceled) {
		return string(CodeCancelled)
	}
	if scheduler.IsCode(err, scheduler.CodeTransient) {
		return string(scheduler.CodeTransient)
	}
	if scheduler.IsCode(err, scheduler.CodePermanent) {
		return string(scheduler.CodePermanent)
	}
	return string(CodeInvalidResult)
}
