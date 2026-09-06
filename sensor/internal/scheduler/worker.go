package scheduler

import (
	"context"
	"errors"
)

func (s *Scheduler) runWorker(pool *workerPool) {
	defer s.workers.Done()
	for item := range pool.queue {
		execCtx, finish, err := s.beginExecution(item.job)
		if err != nil {
			continue
		}
		s.executeWithRetry(execCtx, item.job)
		finish()
	}
}

func (s *Scheduler) beginExecution(job Job) (context.Context, func(), error) {
	now := s.hooks.now()
	s.mu.Lock()
	if s.state != StateRunning {
		s.metrics.cancelled.Add(1)
		s.mu.Unlock()
		return nil, nil, newError(CodeShuttingDown, "scheduler is shutting down", nil)
	}
	if !job.LeaseExpiresAt.After(now) {
		s.metrics.expired.Add(1)
		s.metrics.failed.Add(1)
		s.mu.Unlock()
		return nil, nil, newError(CodeLeaseExpired, "job lease expired before execution started", nil)
	}

	assignment, exists := s.assignments[job.AssignmentKey]
	if !exists || job.AssignmentVersion != assignment.highestSeen {
		s.metrics.rejectedStaleLease.Add(1)
		s.mu.Unlock()
		return nil, nil, newError(CodeStaleFence, "job was superseded before execution started", nil)
	}
	record, exists := assignment.jobs[job.JobID]
	if !exists || record.version != job.AssignmentVersion {
		s.metrics.rejectedStaleLease.Add(1)
		s.mu.Unlock()
		return nil, nil, newError(CodeStaleFence, "job is no longer an accepted current lease", nil)
	}

	baseCtx, baseCancel := context.WithCancelCause(s.rootCtx)
	timeout := job.Timeout
	if timeout == 0 {
		timeout = s.cfg.DefaultExecutionTimeout
	}
	deadline := now.Add(timeout)
	deadlineCause := error(causeDeadlineExceeded)
	if job.LeaseExpiresAt.Before(deadline) {
		deadline = job.LeaseExpiresAt
		deadlineCause = causeLeaseExpired
	}
	execCtx, deadlineCancel := context.WithDeadlineCause(baseCtx, deadline, deadlineCause)

	activeJobs := s.active[job.AssignmentKey]
	if activeJobs == nil {
		activeJobs = make(map[JobID]activeExecution)
		s.active[job.AssignmentKey] = activeJobs
	}
	activeJobs[job.JobID] = activeExecution{
		version: job.AssignmentVersion,
		cancel:  baseCancel,
	}
	s.activeByClass[job.Class]++
	s.mu.Unlock()

	finish := func() {
		deadlineCancel()
		baseCancel(nil)
		s.mu.Lock()
		if activeJobs := s.active[job.AssignmentKey]; activeJobs != nil {
			if _, exists := activeJobs[job.JobID]; exists {
				delete(activeJobs, job.JobID)
				if s.activeByClass[job.Class] > 0 {
					s.activeByClass[job.Class]--
				}
			}
			if len(activeJobs) == 0 {
				delete(s.active, job.AssignmentKey)
			}
		}
		s.mu.Unlock()
	}
	return execCtx, finish, nil
}

func (s *Scheduler) executeWithRetry(ctx context.Context, job Job) {
	for attempt := 1; attempt <= s.cfg.Retry.MaxAttempts; attempt++ {
		if code := contextOutcomeCode(ctx); code != "" {
			s.recordExecutionOutcome(code)
			return
		}

		execJob := job.clone()
		execJob.Attempt = uint32(attempt)
		_, err := s.executor.Execute(ctx, execJob)
		if err == nil {
			if code := contextOutcomeCode(ctx); code != "" {
				s.recordExecutionOutcome(code)
				return
			}
			s.metrics.completed.Add(1)
			return
		}

		code := executionErrorCode(ctx, err)
		if code == CodeTransient && attempt < s.cfg.Retry.MaxAttempts {
			delay := backoffDuration(s.cfg.Retry, attempt, s.hooks.jitter)
			if waitErr := s.hooks.wait(ctx, delay); waitErr != nil {
				waitCode := executionErrorCode(ctx, waitErr)
				s.recordExecutionOutcome(waitCode)
				return
			}
			s.metrics.retries.Add(1)
			continue
		}

		s.recordExecutionOutcome(code)
		if s.logger != nil && code != CodeCancelled && code != CodeShuttingDown && code != CodeStaleFence {
			s.logger.Debug("sensor scheduler job finished with error", "error_code", code, "execution_class", job.Class, "job_kind", job.Kind, "attempt", attempt)
		}
		return
	}
}

func contextOutcomeCode(ctx context.Context) ErrorCode {
	cause := context.Cause(ctx)
	if cause == nil {
		return ""
	}
	var schedulerErr *Error
	if errors.As(cause, &schedulerErr) {
		return schedulerErr.Code
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return CodeDeadlineExceeded
	}
	return CodeCancelled
}

func executionErrorCode(ctx context.Context, err error) ErrorCode {
	if code := contextOutcomeCode(ctx); code != "" {
		return code
	}
	var schedulerErr *Error
	if errors.As(err, &schedulerErr) {
		switch schedulerErr.Code {
		case CodeTransient, CodePermanent, CodeCancelled, CodeDeadlineExceeded, CodeLeaseExpired, CodeStaleFence, CodeShuttingDown:
			return schedulerErr.Code
		default:
			return CodePermanent
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return CodeDeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		return CodeCancelled
	}
	return CodePermanent
}

func (s *Scheduler) recordExecutionOutcome(code ErrorCode) {
	switch code {
	case CodeStaleFence:
		s.metrics.rejectedStaleLease.Add(1)
		s.metrics.cancelled.Add(1)
	case CodeLeaseExpired:
		s.metrics.expired.Add(1)
		s.metrics.failed.Add(1)
	case CodeDeadlineExceeded:
		s.metrics.deadlineExceeded.Add(1)
		s.metrics.failed.Add(1)
	case CodeCancelled, CodeShuttingDown:
		s.metrics.cancelled.Add(1)
	default:
		s.metrics.failed.Add(1)
	}
}
