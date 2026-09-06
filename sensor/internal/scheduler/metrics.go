package scheduler

import "sync/atomic"

type State string

const (
	StateRunning      State = "RUNNING"
	StateShuttingDown State = "SHUTTING_DOWN"
	StateStopped      State = "STOPPED"
)

type PoolSnapshot struct {
	Workers       int
	ActiveWorkers int
	QueueDepth    int
	QueueCapacity int
}

type Snapshot struct {
	State                         State
	QueueDepth                    int
	QueueCapacity                 int
	ConfiguredWorkers             int
	ActiveWorkers                 int
	TrackedAssignments            int
	TrackedJobs                   int
	HighestObservedFencingVersion uint64
	HighestAcceptedFencingVersion uint64
	Pools                         map[ExecutionClass]PoolSnapshot
	Submitted                     uint64
	Accepted                      uint64
	Duplicates                    uint64
	Completed                     uint64
	Failed                        uint64
	Retries                       uint64
	RejectedQueueFull             uint64
	RejectedStaleLease            uint64
	RejectedTrackingCapacity      uint64
	Expired                       uint64
	Cancelled                     uint64
	DeadlineExceeded              uint64
}

type metricCounters struct {
	submitted                atomic.Uint64
	accepted                 atomic.Uint64
	duplicates               atomic.Uint64
	completed                atomic.Uint64
	failed                   atomic.Uint64
	retries                  atomic.Uint64
	rejectedQueueFull        atomic.Uint64
	rejectedStaleLease       atomic.Uint64
	rejectedTrackingCapacity atomic.Uint64
	expired                  atomic.Uint64
	cancelled                atomic.Uint64
	deadlineExceeded         atomic.Uint64
}
