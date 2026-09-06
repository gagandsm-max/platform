package scheduler

import (
	"context"
	"time"
)

type JobID string
type TenantID string
type EntityID string
type AssignmentKey string
type JobKind string
type ExecutionClass string

const (
	ClassNetworkPoll ExecutionClass = "NETWORK_POLL"
	ClassStreaming   ExecutionClass = "STREAMING"
	ClassInfra       ExecutionClass = "INFRA"
	ClassDiscovery   ExecutionClass = "DISCOVERY"
	ClassNCM         ExecutionClass = "NCM"
	ClassSynthetic   ExecutionClass = "SYNTHETIC"
)

// Job is the protocol-neutral unit of scheduled work received from Core.
// Payload is opaque to the scheduler and is never logged or exposed in metrics.
type Job struct {
	JobID             JobID
	AssignmentKey     AssignmentKey
	TenantID          TenantID
	EntityID          EntityID
	Kind              JobKind
	Class             ExecutionClass
	AssignmentVersion uint64
	LeaseExpiresAt    time.Time
	Timeout           time.Duration
	Attempt           uint32
	Payload           []byte
}

// ExecutionResult is opaque scheduler output. The scheduler does not interpret
// or log Payload; protocol executors remain responsible for domain semantics.
type ExecutionResult struct {
	Payload []byte
}

// Executor is intentionally protocol-neutral. Implementations must honor ctx
// cancellation and deadlines promptly enough for scheduler shutdown/fencing.
type Executor interface {
	Execute(ctx context.Context, job Job) (ExecutionResult, error)
}

type SubmissionStatus string

const (
	SubmissionAccepted  SubmissionStatus = "ACCEPTED"
	SubmissionDuplicate SubmissionStatus = "DUPLICATE"
)

type Submission struct {
	Status SubmissionStatus
}

func (j Job) clone() Job {
	cloned := j
	if j.Payload != nil {
		cloned.Payload = append([]byte(nil), j.Payload...)
	}
	return cloned
}
