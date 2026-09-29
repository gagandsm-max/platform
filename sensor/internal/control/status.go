package control

import (
	"sync"
	"time"
)

// Health/control readiness states for Sensor self-monitoring.
const (
	StateSyncPending  = "SYNC_PENDING"
	StateInSync       = "IN_SYNC"
	StateDegraded     = "DEGRADED"
	StateUntrusted    = "STATE_UNTRUSTED"
)

// Status is a bounded control-plane self-monitoring snapshot (no secrets/config bodies).
type Status struct {
	State                   string
	LastAction              string
	LastCode                string
	LastGeneration          uint64
	LastSuccessfulReconcile time.Time
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
	LastAppliedGeneration   uint64
}

// StatusTracker records the latest reconcile outcome for health/readiness.
type StatusTracker struct {
	mu sync.RWMutex

	state          string
	lastAction     string
	lastCode       string
	lastGeneration uint64
	lastSuccess    time.Time
	untrusted      bool
	metrics        *Metrics
}

// NewStatusTracker constructs a SYNC_PENDING tracker bound to reconciler metrics.
func NewStatusTracker(metrics *Metrics) *StatusTracker {
	if metrics == nil {
		metrics = &Metrics{}
	}
	return &StatusTracker{
		state:   StateSyncPending,
		metrics: metrics,
	}
}

// MarkUntrusted records durable control state that cannot be trusted (e.g. corrupt snapshot).
func (t *StatusTracker) MarkUntrusted(code string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.untrusted = true
	t.state = StateUntrusted
	if code != "" {
		t.lastCode = code
	} else {
		t.lastCode = CodeStateCorrupt
	}
	t.lastAction = "STATE_CORRUPT"
}

// Observe records one RunOnce result into bounded health state.
func (t *StatusTracker) Observe(res Result, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.lastAction = res.Action
	t.lastCode = res.Code
	if res.Code == "" && err != nil {
		t.lastCode = CodeOf(err)
	}
	if res.Generation != 0 {
		t.lastGeneration = res.Generation
	}

	switch res.Action {
	case "STATE_CORRUPT", "DURABILITY_FAILED":
		t.untrusted = true
		t.state = StateUntrusted
		if t.lastCode == "" {
			if res.Action == "DURABILITY_FAILED" {
				t.lastCode = CodeStateDurabilityFailed
			} else {
				t.lastCode = CodeStateCorrupt
			}
		}
		return
	}

	// Successful durable re-proof may clear untrusted. NO_DESIRED cannot —
	// Core 204 returns before store.Load() and does not prove durable trust.
	if err == nil && (res.Action == "APPLIED" || res.Action == "LOST_ACK_REPAIR") {
		t.untrusted = false
		t.state = StateInSync
		t.lastSuccess = time.Now().UTC()
		t.lastCode = ""
		return
	}
	if err == nil && res.Action == "NO_DESIRED" {
		if t.untrusted {
			t.state = StateUntrusted
			return
		}
		t.state = StateInSync
		t.lastSuccess = time.Now().UTC()
		t.lastCode = ""
		return
	}

	if t.untrusted {
		t.state = StateUntrusted
		return
	}

	if t.lastAction == "" && t.state == "" {
		t.state = StateSyncPending
		return
	}

	// Any completed non-success outcome is DEGRADED while local state remains trustworthy.
	if res.Action != "" {
		t.state = StateDegraded
		return
	}
	if t.state == "" {
		t.state = StateSyncPending
	}
}

// Snapshot returns a consistent health payload view.
func (t *StatusTracker) Snapshot() Status {
	if t == nil {
		return Status{State: StateSyncPending}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	ms := t.metrics.Snapshot()
	state := t.state
	if state == "" {
		state = StateSyncPending
	}
	if t.untrusted {
		state = StateUntrusted
	}
	return Status{
		State:                   state,
		LastAction:              t.lastAction,
		LastCode:                t.lastCode,
		LastGeneration:          t.lastGeneration,
		LastSuccessfulReconcile: t.lastSuccess,
		ReconcileAttempts:       ms.ReconcileAttempts,
		NoDesiredCount:          ms.NoDesiredCount,
		AppliedCount:            ms.AppliedCount,
		RejectedCount:           ms.RejectedCount,
		LostAckRepairCount:      ms.LostAckRepairCount,
		CoreRegressionCount:     ms.CoreRegressionCount,
		GenerationConflictCount: ms.GenerationConflictCount,
		ChecksumMismatchCount:   ms.ChecksumMismatchCount,
		CoreTransportFailures:   ms.CoreTransportFailures,
		DurableStoreFailures:    ms.DurableStoreFailures,
		AckFailures:             ms.AckFailures,
		LastAppliedGeneration:   ms.LastAppliedGeneration,
	}
}
