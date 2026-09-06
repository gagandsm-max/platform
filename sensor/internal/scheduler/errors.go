package scheduler

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidConfig             ErrorCode = "SCHEDULER_INVALID_CONFIG"
	CodeInvalidJob                ErrorCode = "SCHEDULER_INVALID_JOB"
	CodeQueueFull                 ErrorCode = "SCHEDULER_QUEUE_FULL"
	CodeStaleFence                ErrorCode = "SCHEDULER_STALE_FENCE"
	CodeLeaseExpired              ErrorCode = "SCHEDULER_LEASE_EXPIRED"
	CodeShuttingDown              ErrorCode = "SCHEDULER_SHUTTING_DOWN"
	CodeCancelled                 ErrorCode = "SCHEDULER_CANCELLED"
	CodeDeadlineExceeded          ErrorCode = "SCHEDULER_DEADLINE_EXCEEDED"
	CodeTransient                 ErrorCode = "SCHEDULER_TRANSIENT"
	CodePermanent                 ErrorCode = "SCHEDULER_PERMANENT"
	CodeExecutionClassUnavailable ErrorCode = "SCHEDULER_EXECUTION_CLASS_UNAVAILABLE"
	CodeTrackingCapacity          ErrorCode = "SCHEDULER_TRACKING_CAPACITY"
	CodeJobConflict               ErrorCode = "SCHEDULER_JOB_CONFLICT"
)

// Error is the stable machine-readable scheduler error contract.
type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func newError(code ErrorCode, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func IsCode(err error, code ErrorCode) bool {
	var schedulerErr *Error
	return errors.As(err, &schedulerErr) && schedulerErr.Code == code
}

// Transient marks an executor failure as retryable.
func Transient(err error) error {
	if err == nil {
		return nil
	}
	return newError(CodeTransient, "transient executor failure", err)
}

// Permanent marks an executor failure as non-retryable.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return newError(CodePermanent, "permanent executor failure", err)
}

var (
	causeStaleFence       = newError(CodeStaleFence, "job superseded by newer fencing version", nil)
	causeShuttingDown     = newError(CodeShuttingDown, "scheduler is shutting down", nil)
	causeLeaseExpired     = newError(CodeLeaseExpired, "job lease expired during execution", nil)
	causeDeadlineExceeded = newError(CodeDeadlineExceeded, "job execution deadline exceeded", nil)
)
