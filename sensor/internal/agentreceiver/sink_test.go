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

// InMemoryAcceptanceSink is TEST-ONLY and is NOT durable correctness authority.
// It retains logically accepted batches in process memory for unit tests and
// harnesses only. Exact source-range/content replays are idempotent so tests can
// observe logical acceptance while the Receiver still re-enters the sink to
// re-verify its acceptance authority.
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

	checksum := ComputeBatchChecksum(env.Records)
	for _, existing := range s.batches {
		if existing.TenantID == env.TenantID &&
			existing.AgentID == env.AgentID &&
			existing.FirstSequence == env.FirstSequence &&
			existing.LastSequence == env.LastSequence &&
			ComputeBatchChecksum(existing.Records) == checksum {
			return AcceptanceResult{AcceptedThrough: existing.LastSequence}, nil
		}
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
// production Sensor spool adapter. Exact source-range/content replay is logically
// idempotent so Receiver replay still re-enters the sink without creating a second
// durable test artifact.
type DurableTestAcceptanceSink struct {
	mu       sync.Mutex
	dir      string
	n        int
	accepted map[string]string
}

func NewDurableTestAcceptanceSink(dir string) (*DurableTestAcceptanceSink, error) {
	if dir == "" {
		return nil, errors.New("durable test sink directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &DurableTestAcceptanceSink{dir: dir, accepted: make(map[string]string)}, nil
}

func (s *DurableTestAcceptanceSink) AcceptBatch(ctx context.Context, env DeliveryEnvelope) (AcceptanceResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s\x00%s\x00%d\x00%d", env.TenantID, env.AgentID, env.FirstSequence, env.LastSequence)
	checksum := ComputeBatchChecksum(env.Records)
	if existing, ok := s.accepted[key]; ok {
		if existing != checksum {
			return AcceptanceResult{}, errors.New("durable test sink replay conflict")
		}
		return AcceptanceResult{AcceptedThrough: env.LastSequence}, nil
	}

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
	dirFile, err := os.Open(s.dir)
	if err != nil {
		return AcceptanceResult{}, err
	}
	if err := dirFile.Sync(); err != nil {
		_ = dirFile.Close()
		return AcceptanceResult{}, err
	}
	if err := dirFile.Close(); err != nil {
		return AcceptanceResult{}, err
	}
	s.accepted[key] = checksum
	return AcceptanceResult{AcceptedThrough: env.LastSequence}, nil
}

func (s *DurableTestAcceptanceSink) BatchCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}
