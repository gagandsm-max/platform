package control

import (
	"sync"
	"time"
)

// Metrics holds bounded in-memory control reconciliation counters for Slice 4 HealthServer wiring.
type Metrics struct {
	mu sync.RWMutex

	ReconcileAttempts      uint64
	NoDesiredCount         uint64
	AppliedCount           uint64
	RejectedCount          uint64
	LostAckRepairCount     uint64
	CoreRegressionCount    uint64
	GenerationConflictCount uint64
	ChecksumMismatchCount  uint64
	CoreTransportFailures  uint64
	DurableStoreFailures   uint64
	AckFailures            uint64
	LastSuccessUnixNano    int64
	LastAppliedGeneration  uint64
}

// Snapshot is a consistent copy of Metrics.
type MetricsSnapshot struct {
	ReconcileAttempts       uint64
	NoDesiredCount          uint64
	AppliedCount            uint64
	RejectedCount           uint64
	LostAckRepairCount      uint64
	CoreRegressionCount     uint64
	GenerationConflictCount uint64
	ChecksumMismatchCount   uint64
	CoreTransportFailures   uint64
	DurableStoreFailures    uint64
	AckFailures             uint64
	LastSuccessUnixNano     int64
	LastAppliedGeneration   uint64
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return MetricsSnapshot{
		ReconcileAttempts:       m.ReconcileAttempts,
		NoDesiredCount:          m.NoDesiredCount,
		AppliedCount:            m.AppliedCount,
		RejectedCount:           m.RejectedCount,
		LostAckRepairCount:      m.LostAckRepairCount,
		CoreRegressionCount:     m.CoreRegressionCount,
		GenerationConflictCount: m.GenerationConflictCount,
		ChecksumMismatchCount:   m.ChecksumMismatchCount,
		CoreTransportFailures:   m.CoreTransportFailures,
		DurableStoreFailures:    m.DurableStoreFailures,
		AckFailures:             m.AckFailures,
		LastSuccessUnixNano:     m.LastSuccessUnixNano,
		LastAppliedGeneration:   m.LastAppliedGeneration,
	}
}

func (m *Metrics) markAttempt() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.ReconcileAttempts++
	m.mu.Unlock()
}

func (m *Metrics) markNoDesired() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.NoDesiredCount++
	m.LastSuccessUnixNano = time.Now().UTC().UnixNano()
	m.mu.Unlock()
}

func (m *Metrics) markApplied(generation uint64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.AppliedCount++
	m.LastAppliedGeneration = generation
	m.LastSuccessUnixNano = time.Now().UTC().UnixNano()
	m.mu.Unlock()
}

func (m *Metrics) markRejected() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.RejectedCount++
	m.mu.Unlock()
}

func (m *Metrics) markLostAck() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.LostAckRepairCount++
	m.LastSuccessUnixNano = time.Now().UTC().UnixNano()
	m.mu.Unlock()
}

func (m *Metrics) markRegression() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.CoreRegressionCount++
	m.mu.Unlock()
}

func (m *Metrics) markConflict() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.GenerationConflictCount++
	m.mu.Unlock()
}

func (m *Metrics) markChecksumMismatch() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.ChecksumMismatchCount++
	m.mu.Unlock()
}

func (m *Metrics) markTransportFailure() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.CoreTransportFailures++
	m.mu.Unlock()
}

func (m *Metrics) markDurableFailure() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.DurableStoreFailures++
	m.mu.Unlock()
}

func (m *Metrics) markAckFailure() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.AckFailures++
	m.mu.Unlock()
}
