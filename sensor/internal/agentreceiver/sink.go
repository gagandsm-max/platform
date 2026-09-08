package agentreceiver

import (
	"context"
	"errors"
	"sync"
)

// AcceptanceResult carries the downstream durable watermark confirmed by the sensor.
type AcceptanceResult struct {
	AcceptedThrough uint64
}

// AcceptanceSink defines the boundary between the HTTP Agent Receiver and the downstream
// Sensor durable observation spool / normalizer. The receiver delegates validated batches to this sink.
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

// InMemoryAcceptanceSink provides a deterministic in-memory sink for unit tests and harnesses.
type InMemoryAcceptanceSink struct {
	mu            sync.Mutex
	batches       []DeliveryEnvelope
	records       []DeliveryRecord
	injectedError error
}

func NewInMemoryAcceptanceSink() *InMemoryAcceptanceSink {
	return &InMemoryAcceptanceSink{}
}

func (s *InMemoryAcceptanceSink) SetInjectedError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.injectedError = err
}

func (s *InMemoryAcceptanceSink) AcceptBatch(ctx context.Context, env DeliveryEnvelope) (AcceptanceResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.injectedError != nil {
		return AcceptanceResult{}, s.injectedError
	}

	s.batches = append(s.batches, env)
	s.records = append(s.records, env.Records...)

	return AcceptanceResult{
		AcceptedThrough: env.LastSequence,
	}, nil
}

func (s *InMemoryAcceptanceSink) Batches() []DeliveryEnvelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]DeliveryEnvelope, len(s.batches))
	copy(out, s.batches)
	return out
}

func (s *InMemoryAcceptanceSink) Records() []DeliveryRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]DeliveryRecord, len(s.records))
	copy(out, s.records)
	return out
}

func (s *InMemoryAcceptanceSink) BatchCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.batches)
}

func (s *InMemoryAcceptanceSink) RecordCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

func (s *InMemoryAcceptanceSink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches = nil
	s.records = nil
	s.injectedError = nil
}
