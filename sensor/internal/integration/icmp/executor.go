package icmpintegration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"platform/sensor/internal/observation"
	protocolicmp "platform/sensor/internal/protocol/icmp"
	"platform/sensor/internal/scheduler"
)

const RecordType = "icmp_observation_v1"

type Config struct {
	SensorID         string
	MaxResultBytes   int
	MaxEnvelopeBytes int
	MaxSamples       int
}

type Executor struct {
	base      scheduler.Executor
	sink      observation.Sink
	cfg       Config
	component *Component
}

func NewExecutor(base scheduler.Executor, sink observation.Sink, cfg Config) (*Executor, error) {
	if base == nil || sink == nil || strings.TrimSpace(cfg.SensorID) == "" || len(cfg.SensorID) > 256 || cfg.MaxResultBytes <= 0 || cfg.MaxEnvelopeBytes <= 0 || cfg.MaxSamples <= 0 || sink.MaxPayloadBytes() <= 0 {
		return nil, fmt.Errorf("ICMP_INTEGRATION_INVALID_CONFIG: executor, bounded sink, source identity and positive limits are required")
	}
	e := &Executor{base: base, sink: sink, cfg: cfg}
	e.component = newComponent(e)
	return e, nil
}
func (e *Executor) Component() *Component {
	if e == nil {
		return nil
	}
	return e.component
}

// Evidence contains observations, not a device-health verdict. Loss is omitted
// for collection failures and unreachable samples whose transmission is unknown.
// RTT is omitted when no Echo Reply was received; zero is never invented.
type Evidence struct {
	SchemaVersion    int                   `json:"schemaVersion"`
	Target           string                `json:"target"`
	IPVersion        int                   `json:"ipVersion"`
	Source           string                `json:"source"`
	StartedAt        time.Time             `json:"startedAt"`
	CompletedAt      time.Time             `json:"completedAt"`
	RequestedSamples int                   `json:"requestedSamples,omitempty"`
	Received         int                   `json:"received"`
	PacketLossPct    *float64              `json:"packetLossPct,omitempty"`
	MinRTTMicros     *int64                `json:"minRttMicros,omitempty"`
	AvgRTTMicros     *int64                `json:"avgRttMicros,omitempty"`
	MaxRTTMicros     *int64                `json:"maxRttMicros,omitempty"`
	Samples          []protocolicmp.Sample `json:"samples,omitempty"`
	ErrorCode        string                `json:"errorCode,omitempty"`
}

func (e *Executor) Execute(ctx context.Context, job scheduler.Job) (scheduler.ExecutionResult, error) {
	if e == nil || e.base == nil || e.sink == nil || e.component == nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(fmt.Errorf("ICMP_INTEGRATION_INVALID_CONFIG: executor is not initialized"))
	}
	if job.Kind != protocolicmp.JobKind || job.Class != scheduler.ClassNetworkPoll {
		return scheduler.ExecutionResult{}, scheduler.Permanent(fmt.Errorf("ICMP_INTEGRATION_INVALID_JOB: ICMP NETWORK_POLL job required"))
	}
	if ctx == nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(fmt.Errorf("ICMP_INTEGRATION_INVALID_CONTEXT"))
	}
	if err := ctx.Err(); err != nil {
		return scheduler.ExecutionResult{}, err
	}
	e.component.recordExecution()
	started := time.Now().UTC()
	result, sourceErr := e.base.Execute(ctx, job)
	status, quality := "COLLECTED", "GOOD"
	var evidence Evidence
	if sourceErr != nil {
		e.component.recordFailure(stableErrorCode(sourceErr), false)
		if ctx.Err() != nil {
			return scheduler.ExecutionResult{}, sourceErr
		}
		// Only genuine protocol source failures with an attributable target become
		// failed observations. Invalid jobs/requests retain scheduler failure only.
		code := protocolicmp.CodeOf(sourceErr)
		if code == "" || code == protocolicmp.CodeInvalidRequest || code == protocolicmp.CodeResourceLimit {
			return scheduler.ExecutionResult{}, sourceErr
		}
		var request struct {
			Target    string `json:"target"`
			IPVersion int    `json:"ipVersion"`
		}
		if len(job.Payload) > e.cfg.MaxResultBytes || json.Unmarshal(job.Payload, &request) != nil || !validTarget(request.Target, request.IPVersion) {
			return scheduler.ExecutionResult{}, sourceErr
		}
		status, quality = "COLLECTION_FAILED", "UNKNOWN"
		evidence = Evidence{SchemaVersion: 1, Target: request.Target, IPVersion: request.IPVersion, Source: "SENSOR_PING_SOCKET", StartedAt: started, CompletedAt: time.Now().UTC(), ErrorCode: string(code)}
	} else {
		decoded, err := decodeResult(result.Payload, e.cfg)
		if err != nil {
			e.component.recordFailure(stableErrorCode(err), false)
			return scheduler.ExecutionResult{}, scheduler.Permanent(err)
		}
		evidence = Evidence{SchemaVersion: 1, Target: decoded.Target, IPVersion: decoded.IPVersion, Source: "SENSOR_PING_SOCKET", StartedAt: decoded.StartedAt, CompletedAt: decoded.CompletedAt, RequestedSamples: decoded.Sent, Received: decoded.Received, Samples: decoded.Samples, PacketLossPct: &decoded.PacketLossPct}
		if decoded.Received > 0 {
			evidence.MinRTTMicros = &decoded.MinRTTMicros
			evidence.AvgRTTMicros = &decoded.AvgRTTMicros
			evidence.MaxRTTMicros = &decoded.MaxRTTMicros
		}
		if decoded.Received < decoded.Sent {
			status = "COLLECTED_WITH_LOSS"
			quality = "PARTIAL"
		}
		for _, sample := range decoded.Samples {
			if sample.ErrorCode == protocolicmp.CodeUnreachable {
				evidence.PacketLossPct = nil
				status = "PARTIAL"
				quality = "UNKNOWN"
			}
		}
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(fmt.Errorf("ICMP_INTEGRATION_INVALID_RESULT: encoding failed"))
	}
	env := observation.Envelope{SchemaVersion: observation.SchemaVersion, BatchID: batchID(job), RecordType: RecordType, CollectedAt: evidence.CompletedAt.UTC(), SensorID: e.cfg.SensorID, TenantID: string(job.TenantID), EntityID: string(job.EntityID), Job: observation.JobCorrelation{JobID: string(job.JobID), AssignmentKey: string(job.AssignmentKey), AssignmentVersion: job.AssignmentVersion, Attempt: job.Attempt}, Protocol: "ICMP", ProtocolVersion: strconv.Itoa(evidence.IPVersion), Operation: "ECHO", Status: status, Quality: quality, ProtocolSchemaVersion: 1, ProtocolPayload: raw}
	limit := e.cfg.MaxEnvelopeBytes
	if e.sink.MaxPayloadBytes() < limit {
		limit = e.sink.MaxPayloadBytes()
	}
	if _, err = observation.EncodeBounded(ctx, env, limit); err != nil {
		if ctx.Err() != nil {
			return scheduler.ExecutionResult{}, ctx.Err()
		}
		e.component.recordFailure(stableErrorCode(err), false)
		return scheduler.ExecutionResult{}, scheduler.Permanent(err)
	}
	receipt, err := e.sink.Write(ctx, env)
	if err != nil {
		if ctx.Err() != nil {
			return scheduler.ExecutionResult{}, ctx.Err()
		}
		e.component.recordFailure("ICMP_INTEGRATION_STORAGE_UNAVAILABLE", true)
		return scheduler.ExecutionResult{}, scheduler.Permanent(fmt.Errorf("ICMP_INTEGRATION_STORAGE_UNAVAILABLE: %w", err))
	}
	if sourceErr != nil {
		return scheduler.ExecutionResult{}, sourceErr
	}
	e.component.recordSuccess()
	payload, _ := json.Marshal(receipt) // already durable; never retry a committed write for receipt encoding
	return scheduler.ExecutionResult{Payload: payload}, nil
}

func validTarget(target string, version int) bool {
	ip := net.ParseIP(target)
	return ip != nil && ((version == 4 && ip.To4() != nil) || (version == 6 && ip.To4() == nil))
}
func decodeResult(payload []byte, cfg Config) (protocolicmp.Result, error) {
	invalid := func() (protocolicmp.Result, error) {
		return protocolicmp.Result{}, fmt.Errorf("ICMP_INTEGRATION_INVALID_RESULT: invalid bounded ICMP result")
	}
	if len(payload) == 0 || len(payload) > cfg.MaxResultBytes {
		return invalid()
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var r protocolicmp.Result
	if err := dec.Decode(&r); err != nil {
		return invalid()
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return invalid()
	}
	if r.SchemaVersion != protocolicmp.ResultSchemaVersion || !validTarget(r.Target, r.IPVersion) || r.StartedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) || r.Sent <= 0 || r.Sent > cfg.MaxSamples || len(r.Samples) != r.Sent || r.Received < 0 || r.Received > r.Sent {
		return invalid()
	}
	received := 0
	for i, s := range r.Samples {
		if s.Sequence != i+1 || s.RTTMicros < 0 {
			return invalid()
		}
		if s.Success {
			if s.ErrorCode != "" {
				return invalid()
			}
			received++
		} else if s.ErrorCode != protocolicmp.CodeTimeout && s.ErrorCode != protocolicmp.CodeUnreachable {
			return invalid()
		}
	}
	if received != r.Received || math.IsNaN(r.PacketLossPct) || math.Abs(r.PacketLossPct-float64(r.Sent-r.Received)*100/float64(r.Sent)) > 0.000001 {
		return invalid()
	}
	if r.MinRTTMicros < 0 || r.AvgRTTMicros < r.MinRTTMicros || r.MaxRTTMicros < r.AvgRTTMicros || (r.Received == 0 && (r.MinRTTMicros != 0 || r.AvgRTTMicros != 0 || r.MaxRTTMicros != 0)) {
		return invalid()
	}
	return r, nil
}
func batchID(job scheduler.Job) string {
	h := sha256.New()
	for _, part := range []string{"icmp-observation-v1", string(job.TenantID), string(job.EntityID), string(job.AssignmentKey), string(job.JobID), strconv.FormatUint(job.AssignmentVersion, 10), strconv.FormatUint(uint64(job.Attempt), 10)} {
		h.Write([]byte{0})
		h.Write([]byte(part))
	}
	return "icmp-" + hex.EncodeToString(h.Sum(nil)[:16])
}
func stableErrorCode(err error) string {
	if code := protocolicmp.CodeOf(err); code != "" {
		return string(code)
	}
	if errors.Is(err, context.Canceled) {
		return "ICMP_CANCELLED"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "ICMP_DEADLINE_EXCEEDED"
	}
	return "ICMP_INTEGRATION_FAILED"
}
