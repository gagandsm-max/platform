package agentreceiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

// InMemoryAcceptanceSink is TEST-ONLY and is NOT durable correctness authority.
// It retains batches in process memory for unit tests and harnesses only.
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

// DurableTestAcceptanceSink is TEST-ONLY. It writes accepted batches under a directory
// with atomic temp+rename semantics sufficient for integration tests. It is not the
// production Sensor spool adapter.
type DurableTestAcceptanceSink struct {
	mu  sync.Mutex
	dir string
	n   int
}

func NewDurableTestAcceptanceSink(dir string) (*DurableTestAcceptanceSink, error) {
	if dir == "" {
		return nil, errors.New("durable test sink directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &DurableTestAcceptanceSink{dir: dir}, nil
}

func (s *DurableTestAcceptanceSink) AcceptBatch(ctx context.Context, env DeliveryEnvelope) (AcceptanceResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.n++
	name := fmt.Sprintf("batch_%06d_%s.json", s.n, env.BatchID)
	target := filepath.Join(s.dir, name)
	tmp, err := os.CreateTemp(s.dir, "batch.*.tmp")
	if err != nil {
		return AcceptanceResult{}, err
	}
	tmpName := tmp.Name()
	enc := json.NewEncoder(tmp)
	if err := enc.Encode(env); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return AcceptanceResult{}, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return AcceptanceResult{}, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return AcceptanceResult{}, err
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return AcceptanceResult{}, err
	}
	return AcceptanceResult{AcceptedThrough: env.LastSequence}, nil
}

func (s *DurableTestAcceptanceSink) BatchCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}
