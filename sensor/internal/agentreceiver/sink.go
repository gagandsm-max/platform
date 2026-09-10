package agentreceiver

import (
	"context"
	"errors"
)

// AcceptanceResult carries the downstream durable watermark confirmed by the sensor.
type AcceptanceResult struct {
	AcceptedThrough uint64
}

// AcceptanceSink defines the boundary between the HTTP Agent Receiver and the downstream
// Sensor durable observation spool / normalizer.
//
// A successful AcceptedThrough result means every record through that watermark has been
// durably accepted into Sensor-owned replayable storage (or an equivalent durable boundary
// owned by the Sensor). Callers must not treat in-memory test sinks as production durability.
type AcceptanceSink interface {
	AcceptBatch(ctx context.Context, env DeliveryEnvelope) (AcceptanceResult, error)
}

// FailClosedAcceptanceSink is the mandatory production default when the Sensor-side
// normalizer / durable spool adapter is not yet integrated. It refuses to issue ACKs.
type FailClosedAcceptanceSink struct{}

func NewFailClosedAcceptanceSink() *FailClosedAcceptanceSink {
	return &FailClosedAcceptanceSink{}
}

func (s *FailClosedAcceptanceSink) AcceptBatch(ctx context.Context, env DeliveryEnvelope) (AcceptanceResult, error) {
	return AcceptanceResult{}, errors.New("ERR_ACCEPTANCE_SINK_UNAVAILABLE: downstream sensor spool adapter not wired in this foundation checkpoint")
}
