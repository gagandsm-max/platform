package recurring

import "sync/atomic"

// Snapshot is a low-cardinality recurring scheduler observability view.
type Snapshot struct {
	DesiredGeneration      int64
	AppliedGeneration      int64
	ActiveDefinitions      int64
	DueWork                uint64
	Submitted              uint64
	SkippedCoalesced       uint64
	OverdueFreshnessDebt   uint64
	RejectedStaleDesired   uint64
	RejectedConflict       uint64
	QueueAdmissionRejected uint64
	RestoreFailures        uint64
}

// Metrics holds atomic counters for recurring scheduling.
type Metrics struct {
	desiredGeneration      atomic.Int64
	appliedGeneration      atomic.Int64
	activeDefinitions      atomic.Int64
	dueWork                atomic.Uint64
	submitted              atomic.Uint64
	skippedCoalesced       atomic.Uint64
	overdueFreshnessDebt   atomic.Uint64
	rejectedStaleDesired   atomic.Uint64
	rejectedConflict       atomic.Uint64
	queueAdmissionRejected atomic.Uint64
	restoreFailures        atomic.Uint64
}

// Snapshot returns a point-in-time copy of counters.
func (m *Metrics) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{}
	}
	return Snapshot{
		DesiredGeneration:      m.desiredGeneration.Load(),
		AppliedGeneration:      m.appliedGeneration.Load(),
		ActiveDefinitions:      m.activeDefinitions.Load(),
		DueWork:                m.dueWork.Load(),
		Submitted:              m.submitted.Load(),
		SkippedCoalesced:       m.skippedCoalesced.Load(),
		OverdueFreshnessDebt:   m.overdueFreshnessDebt.Load(),
		RejectedStaleDesired:   m.rejectedStaleDesired.Load(),
		RejectedConflict:       m.rejectedConflict.Load(),
		QueueAdmissionRejected: m.queueAdmissionRejected.Load(),
		RestoreFailures:        m.restoreFailures.Load(),
	}
}

func (m *Metrics) SetDesiredGeneration(v int64)       { m.desiredGeneration.Store(v) }
func (m *Metrics) SetAppliedGeneration(v int64)       { m.appliedGeneration.Store(v) }
func (m *Metrics) SetActiveDefinitions(v int64)       { m.activeDefinitions.Store(v) }
func (m *Metrics) AddDueWork(n uint64)                { m.dueWork.Add(n) }
func (m *Metrics) AddSubmitted(n uint64)              { m.submitted.Add(n) }
func (m *Metrics) AddSkippedCoalesced(n uint64)       { m.skippedCoalesced.Add(n) }
func (m *Metrics) AddOverdueFreshnessDebt(n uint64)   { m.overdueFreshnessDebt.Add(n) }
func (m *Metrics) AddRejectedStaleDesired(n uint64)   { m.rejectedStaleDesired.Add(n) }
func (m *Metrics) AddRejectedConflict(n uint64)       { m.rejectedConflict.Add(n) }
func (m *Metrics) AddQueueAdmissionRejected(n uint64) { m.queueAdmissionRejected.Add(n) }
func (m *Metrics) AddRestoreFailures(n uint64)        { m.restoreFailures.Add(n) }
