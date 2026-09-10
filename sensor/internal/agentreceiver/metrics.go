package agentreceiver

import "sync/atomic"

// MetricsSnapshot provides a copy of current atomic counter values.
type MetricsSnapshot struct {
	AcceptedBatches       int64  `json:"accepted_batches"`
	AcceptedRecords       int64  `json:"accepted_records"`
	AuthFailures          int64  `json:"auth_failures"`
	AssignmentDenials     int64  `json:"assignment_denials"`
	MalformedBatches      int64  `json:"malformed_batches"`
	ChecksumFailures      int64  `json:"checksum_failures"`
	ReplayExactHits       int64  `json:"replay_exact_hits"`
	ReplayConflicts       int64  `json:"replay_conflicts"`
	SinkFailures          int64  `json:"sink_failures"`
	LastAckedSequence     uint64 `json:"last_acked_sequence"`
	RoleIneligibleErrors  int64  `json:"role_ineligible_errors"`
}

// ReceiverMetrics tracks low-cardinality operational telemetry for the Agent receiver.
// It avoids high-cardinality labels (such as Agent ID, Tenant ID, Record ID, Batch ID).
type ReceiverMetrics struct {
	AcceptedBatches      atomic.Int64
	AcceptedRecords      atomic.Int64
	AuthFailures         atomic.Int64
	AssignmentDenials    atomic.Int64
	MalformedBatches     atomic.Int64
	ChecksumFailures     atomic.Int64
	ReplayExactHits      atomic.Int64
	ReplayConflicts      atomic.Int64
	SinkFailures         atomic.Int64
	LastAckedSequence    atomic.Uint64
	RoleIneligibleErrors atomic.Int64
}

func NewReceiverMetrics() *ReceiverMetrics {
	return &ReceiverMetrics{}
}

func (m *ReceiverMetrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		AcceptedBatches:      m.AcceptedBatches.Load(),
		AcceptedRecords:      m.AcceptedRecords.Load(),
		AuthFailures:         m.AuthFailures.Load(),
		AssignmentDenials:    m.AssignmentDenials.Load(),
		MalformedBatches:     m.MalformedBatches.Load(),
		ChecksumFailures:     m.ChecksumFailures.Load(),
		ReplayExactHits:      m.ReplayExactHits.Load(),
		ReplayConflicts:      m.ReplayConflicts.Load(),
		SinkFailures:         m.SinkFailures.Load(),
		LastAckedSequence:    m.LastAckedSequence.Load(),
		RoleIneligibleErrors: m.RoleIneligibleErrors.Load(),
	}
}

func (m *ReceiverMetrics) Reset() {
	m.AcceptedBatches.Store(0)
	m.AcceptedRecords.Store(0)
	m.AuthFailures.Store(0)
	m.AssignmentDenials.Store(0)
	m.MalformedBatches.Store(0)
	m.ChecksumFailures.Store(0)
	m.ReplayExactHits.Store(0)
	m.ReplayConflicts.Store(0)
	m.SinkFailures.Store(0)
	m.LastAckedSequence.Store(0)
	m.RoleIneligibleErrors.Store(0)
}
