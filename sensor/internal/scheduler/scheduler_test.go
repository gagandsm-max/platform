package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type executorFunc func(context.Context, Job) (ExecutionResult, error)

func (f executorFunc) Execute(ctx context.Context, job Job) (ExecutionResult, error) {
	return f(ctx, job)
}

func testConfig() Config {
	return Config{
		MaxWorkers:              4,
		MaxQueueDepth:           32,
		MaxTrackedAssignments:   128,
		MaxTrackedJobs:          128,
		MaxPayloadBytes:         1024,
		DefaultExecutionTimeout: 200 * time.Millisecond,
		ShutdownGracePeriod:     500 * time.Millisecond,
		Retry: RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
			JitterFraction: 0,
		},
		Pools: map[ExecutionClass]PoolConfig{
			ClassNetworkPoll: {Workers: 1, QueueDepth: 8},
		},
	}
}

func baseJob(id string, assignment string, version uint64, class ExecutionClass) Job {
	return Job{
		JobID:             JobID(id),
		AssignmentKey:     AssignmentKey(assignment),
		TenantID:          TenantID("tenant-test"),
		EntityID:          EntityID("entity-test"),
		Kind:              JobKind("TEST"),
		Class:             class,
		AssignmentVersion: version,
		LeaseExpiresAt:    time.Now().Add(2 * time.Second),
		Timeout:           200 * time.Millisecond,
		Payload:           []byte(`{"test":true}`),
	}
}

func newTestScheduler(t *testing.T, cfg Config, executor Executor, opts ...Option) *Scheduler {
	t.Helper()
	s, err := New(cfg, executor, opts...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	})
	return s
}

func waitFor(t *testing.T, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if !condition() {
		t.Fatalf("timed out waiting for %s", description)
	}
}

func immediateRetryHooks() runtimeHooks {
	hooks := defaultRuntimeHooks()
	hooks.wait = func(ctx context.Context, _ time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	hooks.jitter = func(delay time.Duration, _ float64) time.Duration { return delay }
	return hooks
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "zero max workers", mutate: func(c *Config) { c.MaxWorkers = 0 }},
		{name: "zero max queue", mutate: func(c *Config) { c.MaxQueueDepth = 0 }},
		{name: "zero tracked assignments", mutate: func(c *Config) { c.MaxTrackedAssignments = 0 }},
		{name: "zero tracked jobs", mutate: func(c *Config) { c.MaxTrackedJobs = 0 }},
		{name: "zero payload bound", mutate: func(c *Config) { c.MaxPayloadBytes = 0 }},
		{name: "zero execution timeout", mutate: func(c *Config) { c.DefaultExecutionTimeout = 0 }},
		{name: "zero shutdown grace", mutate: func(c *Config) { c.ShutdownGracePeriod = 0 }},
		{name: "no pools", mutate: func(c *Config) { c.Pools = nil }},
		{name: "worker overflow", mutate: func(c *Config) { c.MaxWorkers = 1; c.Pools = map[ExecutionClass]PoolConfig{ClassNetworkPoll: {Workers: 2, QueueDepth: 1}} }},
		{name: "queue overflow", mutate: func(c *Config) { c.MaxQueueDepth = 1; c.Pools = map[ExecutionClass]PoolConfig{ClassNetworkPoll: {Workers: 1, QueueDepth: 2}} }},
		{name: "tracking cannot cover active and queued", mutate: func(c *Config) { c.MaxTrackedJobs = 1 }},
		{name: "zero attempts", mutate: func(c *Config) { c.Retry.MaxAttempts = 0 }},
		{name: "zero initial backoff", mutate: func(c *Config) { c.Retry.InitialBackoff = 0 }},
		{name: "max backoff below initial", mutate: func(c *Config) { c.Retry.MaxBackoff = time.Microsecond }},
		{name: "jitter above one", mutate: func(c *Config) { c.Retry.JitterFraction = 1.1 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig()
			tt.mutate(&cfg)
			if err := cfg.Validate(); !IsCode(err, CodeInvalidConfig) {
				t.Fatalf("Validate() error = %v, want %s", err, CodeInvalidConfig)
			}
		})
	}

	cfg := testConfig()
	if _, err := New(cfg, nil); !IsCode(err, CodeInvalidConfig) {
		t.Fatalf("New(nil executor) error = %v, want %s", err, CodeInvalidConfig)
	}
}

func TestSuccessfulJobExecution(t *testing.T) {
	executed := make(chan Job, 1)
	s := newTestScheduler(t, testConfig(), executorFunc(func(_ context.Context, job Job) (ExecutionResult, error) {
		executed <- job
		return ExecutionResult{}, nil
	}))

	submission, err := s.Submit(baseJob("job-1", "assignment-1", 1, ClassNetworkPoll))
	if err != nil || submission.Status != SubmissionAccepted {
		t.Fatalf("Submit() = %+v, %v", submission, err)
	}
	select {
	case job := <-executed:
		if job.Attempt != 1 {
			t.Fatalf("executor attempt = %d, want 1", job.Attempt)
		}
	case <-time.After(time.Second):
		t.Fatal("job was not executed")
	}
	waitFor(t, func() bool { return s.Snapshot().Completed == 1 }, "completed metric")
}

func TestMaximumWorkerConcurrencyRespected(t *testing.T) {
	cfg := testConfig()
	cfg.MaxWorkers = 2
	cfg.MaxQueueDepth = 8
	cfg.MaxTrackedJobs = 16
	cfg.Pools = map[ExecutionClass]PoolConfig{ClassNetworkPoll: {Workers: 2, QueueDepth: 8}}

	var active atomic.Int32
	var maximum atomic.Int32
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	exec := executorFunc(func(_ context.Context, _ Job) (ExecutionResult, error) {
		current := active.Add(1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		active.Add(-1)
		return ExecutionResult{}, nil
	})
	s := newTestScheduler(t, cfg, exec)

	for i := 0; i < 4; i++ {
		if _, err := s.Submit(baseJob(fmt.Sprintf("job-%d", i), fmt.Sprintf("assignment-%d", i), 1, ClassNetworkPoll)); err != nil {
			t.Fatalf("Submit(%d) error = %v", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	if got := maximum.Load(); got > 2 {
		t.Fatalf("maximum concurrency = %d, want <= 2", got)
	}
	close(release)
	waitFor(t, func() bool { return s.Snapshot().Completed == 4 }, "all jobs completed")
	if got := maximum.Load(); got != 2 {
		t.Fatalf("maximum concurrency = %d, want 2", got)
	}
}

func TestQueueCapacityBounded(t *testing.T) {
	cfg := testConfig()
	cfg.MaxWorkers = 1
	cfg.MaxQueueDepth = 1
	cfg.MaxTrackedJobs = 2
	cfg.Pools = map[ExecutionClass]PoolConfig{ClassNetworkPoll: {Workers: 1, QueueDepth: 1}}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	s := newTestScheduler(t, cfg, executorFunc(func(_ context.Context, _ Job) (ExecutionResult, error) {
		started <- struct{}{}
		<-release
		return ExecutionResult{}, nil
	}))

	if _, err := s.Submit(baseJob("active", "active-assignment", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := s.Submit(baseJob("queued", "queued-assignment", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	snapshot := s.Snapshot()
	if snapshot.QueueDepth != 1 || snapshot.QueueCapacity != 1 {
		t.Fatalf("queue depth/capacity = %d/%d, want 1/1", snapshot.QueueDepth, snapshot.QueueCapacity)
	}
	close(release)
}

func TestQueueFullTypedRejection(t *testing.T) {
	cfg := testConfig()
	cfg.MaxWorkers = 1
	cfg.MaxQueueDepth = 1
	cfg.MaxTrackedJobs = 8
	cfg.MaxTrackedAssignments = 8
	cfg.Pools = map[ExecutionClass]PoolConfig{ClassNetworkPoll: {Workers: 1, QueueDepth: 1}}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	s := newTestScheduler(t, cfg, executorFunc(func(_ context.Context, _ Job) (ExecutionResult, error) {
		started <- struct{}{}
		<-release
		return ExecutionResult{}, nil
	}))

	_, _ = s.Submit(baseJob("active", "assignment-a", 1, ClassNetworkPoll))
	<-started
	_, _ = s.Submit(baseJob("queued", "assignment-b", 1, ClassNetworkPoll))
	if _, err := s.Submit(baseJob("rejected", "assignment-c", 1, ClassNetworkPoll)); !IsCode(err, CodeQueueFull) {
		t.Fatalf("Submit() error = %v, want %s", err, CodeQueueFull)
	}
	if got := s.Snapshot().RejectedQueueFull; got != 1 {
		t.Fatalf("RejectedQueueFull = %d, want 1", got)
	}
	close(release)
}

func TestStaleFenceRejected(t *testing.T) {
	s := newTestScheduler(t, testConfig(), executorFunc(func(context.Context, Job) (ExecutionResult, error) {
		return ExecutionResult{}, nil
	}))
	if _, err := s.Submit(baseJob("new", "assignment", 2, ClassNetworkPoll)); err != nil {
		t.Fatalf("new fence submit error = %v", err)
	}
	if _, err := s.Submit(baseJob("stale", "assignment", 1, ClassNetworkPoll)); !IsCode(err, CodeStaleFence) {
		t.Fatalf("stale submit error = %v, want %s", err, CodeStaleFence)
	}
}

func TestNewerFenceAcceptedAndCancelsOlder(t *testing.T) {
	oldStarted := make(chan struct{})
	oldCancelled := make(chan ErrorCode, 1)
	newExecuted := make(chan struct{}, 1)
	exec := executorFunc(func(ctx context.Context, job Job) (ExecutionResult, error) {
		if job.AssignmentVersion == 1 {
			close(oldStarted)
			<-ctx.Done()
			oldCancelled <- contextOutcomeCode(ctx)
			return ExecutionResult{}, ctx.Err()
		}
		newExecuted <- struct{}{}
		return ExecutionResult{}, nil
	})
	s := newTestScheduler(t, testConfig(), exec)

	if _, err := s.Submit(baseJob("old", "assignment", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	<-oldStarted
	submission, err := s.Submit(baseJob("new", "assignment", 2, ClassNetworkPoll))
	if err != nil || submission.Status != SubmissionAccepted {
		t.Fatalf("new fence Submit() = %+v, %v", submission, err)
	}
	select {
	case code := <-oldCancelled:
		if code != CodeStaleFence {
			t.Fatalf("old cancellation code = %s, want %s", code, CodeStaleFence)
		}
	case <-time.After(time.Second):
		t.Fatal("old execution was not cancelled")
	}
	select {
	case <-newExecuted:
	case <-time.After(time.Second):
		t.Fatal("new execution did not run")
	}
	snapshot := s.Snapshot()
	if snapshot.HighestObservedFencingVersion != 2 || snapshot.HighestAcceptedFencingVersion != 2 {
		t.Fatalf("fencing snapshot = observed %d accepted %d, want 2/2", snapshot.HighestObservedFencingVersion, snapshot.HighestAcceptedFencingVersion)
	}
}

func TestEqualVersionDuplicateIdempotent(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	s := newTestScheduler(t, testConfig(), executorFunc(func(_ context.Context, _ Job) (ExecutionResult, error) {
		calls.Add(1)
		started <- struct{}{}
		<-release
		return ExecutionResult{}, nil
	}))
	job := baseJob("same-job", "assignment", 7, ClassNetworkPoll)
	if submission, err := s.Submit(job); err != nil || submission.Status != SubmissionAccepted {
		t.Fatalf("first Submit() = %+v, %v", submission, err)
	}
	if submission, err := s.Submit(job); err != nil || submission.Status != SubmissionDuplicate {
		t.Fatalf("duplicate Submit() = %+v, %v", submission, err)
	}
	<-started
	close(release)
	waitFor(t, func() bool { return s.Snapshot().Completed == 1 }, "single completion")
	if calls.Load() != 1 {
		t.Fatalf("executor calls = %d, want 1", calls.Load())
	}
}

func TestExpiredLeaseRejected(t *testing.T) {
	s := newTestScheduler(t, testConfig(), executorFunc(func(context.Context, Job) (ExecutionResult, error) {
		return ExecutionResult{}, nil
	}))
	job := baseJob("expired", "assignment", 1, ClassNetworkPoll)
	job.LeaseExpiresAt = time.Now().Add(-time.Second)
	if _, err := s.Submit(job); !IsCode(err, CodeLeaseExpired) {
		t.Fatalf("Submit() error = %v, want %s", err, CodeLeaseExpired)
	}
	if snapshot := s.Snapshot(); snapshot.Expired != 1 || snapshot.Accepted != 0 {
		t.Fatalf("snapshot expired/accepted = %d/%d, want 1/0", snapshot.Expired, snapshot.Accepted)
	}
}

func TestContextCancellationReachesExecutor(t *testing.T) {
	started := make(chan struct{})
	causeSeen := make(chan ErrorCode, 1)
	s := newTestScheduler(t, testConfig(), executorFunc(func(ctx context.Context, _ Job) (ExecutionResult, error) {
		close(started)
		<-ctx.Done()
		causeSeen <- contextOutcomeCode(ctx)
		return ExecutionResult{}, ctx.Err()
	}))
	if _, err := s.Submit(baseJob("job", "assignment", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	select {
	case code := <-causeSeen:
		if code != CodeShuttingDown {
			t.Fatalf("cancellation code = %s, want %s", code, CodeShuttingDown)
		}
	case <-time.After(time.Second):
		t.Fatal("executor did not observe cancellation")
	}
}

func TestExecutionTimeoutEnforced(t *testing.T) {
	causeSeen := make(chan ErrorCode, 1)
	s := newTestScheduler(t, testConfig(), executorFunc(func(ctx context.Context, _ Job) (ExecutionResult, error) {
		<-ctx.Done()
		causeSeen <- contextOutcomeCode(ctx)
		return ExecutionResult{}, ctx.Err()
	}))
	job := baseJob("timeout", "assignment", 1, ClassNetworkPoll)
	job.Timeout = 20 * time.Millisecond
	if _, err := s.Submit(job); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-causeSeen:
		if code != CodeDeadlineExceeded {
			t.Fatalf("deadline code = %s, want %s", code, CodeDeadlineExceeded)
		}
	case <-time.After(time.Second):
		t.Fatal("execution timeout did not cancel executor")
	}
	waitFor(t, func() bool { return s.Snapshot().DeadlineExceeded == 1 }, "deadline metric")
}

func TestTransientErrorRetries(t *testing.T) {
	var calls atomic.Int32
	s := newTestScheduler(t, testConfig(), executorFunc(func(context.Context, Job) (ExecutionResult, error) {
		if calls.Add(1) < 3 {
			return ExecutionResult{}, Transient(errors.New("temporary"))
		}
		return ExecutionResult{}, nil
	}), withRuntimeHooks(immediateRetryHooks()))
	if _, err := s.Submit(baseJob("retry", "assignment", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return s.Snapshot().Completed == 1 }, "retried completion")
	if calls.Load() != 3 || s.Snapshot().Retries != 2 {
		t.Fatalf("calls/retries = %d/%d, want 3/2", calls.Load(), s.Snapshot().Retries)
	}
}

func TestPermanentErrorDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	s := newTestScheduler(t, testConfig(), executorFunc(func(context.Context, Job) (ExecutionResult, error) {
		calls.Add(1)
		return ExecutionResult{}, Permanent(errors.New("invalid request"))
	}), withRuntimeHooks(immediateRetryHooks()))
	if _, err := s.Submit(baseJob("permanent", "assignment", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return s.Snapshot().Failed == 1 }, "permanent failure")
	if calls.Load() != 1 || s.Snapshot().Retries != 0 {
		t.Fatalf("calls/retries = %d/%d, want 1/0", calls.Load(), s.Snapshot().Retries)
	}
}

func TestMaximumRetryAttemptsRespected(t *testing.T) {
	cfg := testConfig()
	cfg.Retry.MaxAttempts = 3
	var calls atomic.Int32
	s := newTestScheduler(t, cfg, executorFunc(func(context.Context, Job) (ExecutionResult, error) {
		calls.Add(1)
		return ExecutionResult{}, Transient(errors.New("still temporary"))
	}), withRuntimeHooks(immediateRetryHooks()))
	if _, err := s.Submit(baseJob("max-retry", "assignment", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return s.Snapshot().Failed == 1 }, "retry exhaustion")
	if calls.Load() != 3 || s.Snapshot().Retries != 2 {
		t.Fatalf("calls/retries = %d/%d, want 3/2", calls.Load(), s.Snapshot().Retries)
	}
}

func TestRetryCancellationStopsPromptly(t *testing.T) {
	waiting := make(chan struct{})
	var once sync.Once
	var calls atomic.Int32
	hooks := defaultRuntimeHooks()
	hooks.jitter = func(delay time.Duration, _ float64) time.Duration { return delay }
	hooks.wait = func(ctx context.Context, _ time.Duration) error {
		once.Do(func() { close(waiting) })
		<-ctx.Done()
		return ctx.Err()
	}
	s := newTestScheduler(t, testConfig(), executorFunc(func(context.Context, Job) (ExecutionResult, error) {
		calls.Add(1)
		return ExecutionResult{}, Transient(errors.New("temporary"))
	}), withRuntimeHooks(hooks))
	if _, err := s.Submit(baseJob("cancel-retry", "assignment", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	<-waiting
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if calls.Load() != 1 || s.Snapshot().Retries != 0 {
		t.Fatalf("calls/retries = %d/%d, want 1/0", calls.Load(), s.Snapshot().Retries)
	}
}

func TestGracefulShutdown(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})
	s := newTestScheduler(t, testConfig(), executorFunc(func(ctx context.Context, _ Job) (ExecutionResult, error) {
		close(started)
		<-ctx.Done()
		close(finished)
		return ExecutionResult{}, ctx.Err()
	}))
	if _, err := s.Submit(baseJob("shutdown", "assignment", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("executor did not finish")
	}
	if got := s.Snapshot().State; got != StateStopped {
		t.Fatalf("state = %s, want %s", got, StateStopped)
	}
}

func TestSubmissionRejectedDuringAndAfterShutdown(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	s := newTestScheduler(t, testConfig(), executorFunc(func(context.Context, Job) (ExecutionResult, error) {
		close(started)
		<-release
		return ExecutionResult{}, nil
	}))
	if _, err := s.Submit(baseJob("running", "assignment-running", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	<-started
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- s.Shutdown(context.Background()) }()
	waitFor(t, func() bool { return s.Snapshot().State == StateShuttingDown }, "shutting down state")
	if _, err := s.Submit(baseJob("during", "assignment-during", 1, ClassNetworkPoll)); !IsCode(err, CodeShuttingDown) {
		t.Fatalf("during-shutdown Submit() error = %v, want %s", err, CodeShuttingDown)
	}
	close(release)
	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if _, err := s.Submit(baseJob("after", "assignment-after", 1, ClassNetworkPoll)); !IsCode(err, CodeShuttingDown) {
		t.Fatalf("after-shutdown Submit() error = %v, want %s", err, CodeShuttingDown)
	}
}

func TestRepeatedShutdownSafe(t *testing.T) {
	s := newTestScheduler(t, testConfig(), executorFunc(func(context.Context, Job) (ExecutionResult, error) {
		return ExecutionResult{}, nil
	}))
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown() error = %v", err)
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
	if s.Snapshot().State != StateStopped {
		t.Fatalf("state = %s, want %s", s.Snapshot().State, StateStopped)
	}
}

func TestMetricsCountersReflectOutcomes(t *testing.T) {
	s := newTestScheduler(t, testConfig(), executorFunc(func(_ context.Context, job Job) (ExecutionResult, error) {
		if job.Kind == "FAIL" {
			return ExecutionResult{}, Permanent(errors.New("failed"))
		}
		return ExecutionResult{}, nil
	}), withRuntimeHooks(immediateRetryHooks()))
	okJob := baseJob("ok", "assignment-ok", 1, ClassNetworkPoll)
	failJob := baseJob("fail", "assignment-fail", 1, ClassNetworkPoll)
	failJob.Kind = "FAIL"
	expiredJob := baseJob("expired", "assignment-expired", 1, ClassNetworkPoll)
	expiredJob.LeaseExpiresAt = time.Now().Add(-time.Second)
	_, _ = s.Submit(okJob)
	_, _ = s.Submit(failJob)
	_, expiredErr := s.Submit(expiredJob)
	if !IsCode(expiredErr, CodeLeaseExpired) {
		t.Fatalf("expired error = %v", expiredErr)
	}
	waitFor(t, func() bool {
		snapshot := s.Snapshot()
		return snapshot.Completed == 1 && snapshot.Failed == 1
	}, "success and failure metrics")
	snapshot := s.Snapshot()
	if snapshot.Submitted != 3 || snapshot.Accepted != 2 || snapshot.Completed != 1 || snapshot.Failed != 1 || snapshot.Expired != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestConcurrentSubmissionsPreserveFencing(t *testing.T) {
	cfg := testConfig()
	cfg.MaxWorkers = 1
	cfg.MaxQueueDepth = 64
	cfg.MaxTrackedJobs = 128
	cfg.Pools = map[ExecutionClass]PoolConfig{ClassNetworkPoll: {Workers: 1, QueueDepth: 64}}
	blockerStarted := make(chan struct{})
	releaseBlocker := make(chan struct{})
	version20 := make(chan struct{})
	var version20Once sync.Once
	var mu sync.Mutex
	executedVersions := make([]uint64, 0)
	exec := executorFunc(func(_ context.Context, job Job) (ExecutionResult, error) {
		if job.AssignmentKey == "blocker" {
			close(blockerStarted)
			<-releaseBlocker
			return ExecutionResult{}, nil
		}
		mu.Lock()
		executedVersions = append(executedVersions, job.AssignmentVersion)
		mu.Unlock()
		if job.AssignmentVersion == 20 {
			version20Once.Do(func() { close(version20) })
		}
		return ExecutionResult{}, nil
	})
	s := newTestScheduler(t, cfg, exec)
	if _, err := s.Submit(baseJob("blocker", "blocker", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	<-blockerStarted

	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for version := 1; version <= 20; version++ {
		version := version
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Submit(baseJob(fmt.Sprintf("job-%d", version), "target", uint64(version), ClassNetworkPoll))
			if err != nil && !IsCode(err, CodeStaleFence) {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("unexpected concurrent submit error = %v", err)
	}
	if got := s.Snapshot().HighestObservedFencingVersion; got != 20 {
		t.Fatalf("highest observed fence = %d, want 20", got)
	}
	close(releaseBlocker)
	select {
	case <-version20:
	case <-time.After(time.Second):
		t.Fatal("highest fencing version did not execute")
	}
	waitFor(t, func() bool { return s.Snapshot().QueueDepth == 0 && s.Snapshot().ActiveWorkers == 0 }, "target queue drain")
	mu.Lock()
	defer mu.Unlock()
	for _, version := range executedVersions {
		if version != 20 {
			t.Fatalf("stale version %d executed after higher fencing version was known", version)
		}
	}
}

func TestExecutionClassesUseIndependentPools(t *testing.T) {
	cfg := testConfig()
	cfg.MaxWorkers = 2
	cfg.MaxQueueDepth = 4
	cfg.MaxTrackedJobs = 8
	cfg.Pools = map[ExecutionClass]PoolConfig{
		ClassNetworkPoll: {Workers: 1, QueueDepth: 2},
		ClassNCM:         {Workers: 1, QueueDepth: 2},
	}
	networkStarted := make(chan struct{})
	releaseNetwork := make(chan struct{})
	ncmExecuted := make(chan struct{}, 1)
	s := newTestScheduler(t, cfg, executorFunc(func(_ context.Context, job Job) (ExecutionResult, error) {
		if job.Class == ClassNetworkPoll {
			close(networkStarted)
			<-releaseNetwork
			return ExecutionResult{}, nil
		}
		ncmExecuted <- struct{}{}
		return ExecutionResult{}, nil
	}))
	if _, err := s.Submit(baseJob("network", "assignment-network", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	<-networkStarted
	if _, err := s.Submit(baseJob("ncm", "assignment-ncm", 1, ClassNCM)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ncmExecuted:
	case <-time.After(time.Second):
		t.Fatal("NCM pool was starved by blocked network pool")
	}
	close(releaseNetwork)
}

func TestCompletedJobsReleaseTrackingCapacity(t *testing.T) {
	cfg := testConfig()
	cfg.MaxTrackedJobs = 4
	cfg.MaxWorkers = 1
	cfg.MaxQueueDepth = 2
	cfg.Pools = map[ExecutionClass]PoolConfig{ClassNetworkPoll: {Workers: 1, QueueDepth: 2}}
	s := newTestScheduler(t, cfg, executorFunc(func(_ context.Context, _ Job) (ExecutionResult, error) {
		return ExecutionResult{}, nil
	}))

	for i := 0; i < 12; i++ {
		job := baseJob(fmt.Sprintf("job-%d", i), "assignment-stable", 1, ClassNetworkPoll)
		if _, err := s.Submit(job); err != nil {
			t.Fatalf("Submit(%d) error = %v", i, err)
		}
		waitFor(t, func() bool { return s.Snapshot().Completed == uint64(i+1) }, "completion")
		if snap := s.Snapshot(); snap.TrackedJobs != 0 {
			t.Fatalf("after job %d TrackedJobs=%d, want 0", i, snap.TrackedJobs)
		}
	}
}

func TestLeaseExpiryReleasesTrackedJobsWithoutSnapshot(t *testing.T) {
	now := time.Now()
	cfg := testConfig()
	cfg.MaxTrackedJobs = 4
	cfg.MaxWorkers = 1
	cfg.MaxQueueDepth = 2
	cfg.Pools = map[ExecutionClass]PoolConfig{ClassNetworkPoll: {Workers: 1, QueueDepth: 2}}
	blocked := make(chan struct{})
	var nowMu sync.Mutex
	current := now
	s := newTestScheduler(t, cfg, executorFunc(func(ctx context.Context, _ Job) (ExecutionResult, error) {
		select {
		case <-blocked:
			return ExecutionResult{}, nil
		case <-ctx.Done():
			return ExecutionResult{}, ctx.Err()
		}
	}), withRuntimeHooks(runtimeHooks{
		now: func() time.Time {
			nowMu.Lock()
			defer nowMu.Unlock()
			return current
		},
		wait: func(ctx context.Context, d time.Duration) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
				return nil
			}
		},
		jitter: func(d time.Duration, _ float64) time.Duration { return d },
	}))

	job1 := baseJob("a", "asg", 1, ClassNetworkPoll)
	job1.LeaseExpiresAt = now.Add(50 * time.Millisecond)
	job2 := baseJob("b", "asg", 1, ClassNetworkPoll)
	job2.LeaseExpiresAt = now.Add(50 * time.Millisecond)
	if _, err := s.Submit(job1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(job2); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return s.Snapshot().ActiveWorkers == 1 }, "worker busy")
	nowMu.Lock()
	current = now.Add(100 * time.Millisecond)
	nowMu.Unlock()
	job3 := baseJob("c", "asg", 1, ClassNetworkPoll)
	job3.LeaseExpiresAt = current.Add(time.Second)
	if _, err := s.Submit(job3); err != nil {
		t.Fatalf("Submit after expiry prune error = %v", err)
	}
	close(blocked)
}

func TestAssignmentChurnRequiresRetireThenForget(t *testing.T) {
	cfg := testConfig()
	cfg.MaxTrackedAssignments = 2
	s := newTestScheduler(t, cfg, executorFunc(func(_ context.Context, _ Job) (ExecutionResult, error) {
		return ExecutionResult{}, nil
	}))
	if _, err := s.Submit(baseJob("j1", "a1", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(baseJob("j2", "a2", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(baseJob("j3", "a3", 1, ClassNetworkPoll)); !IsCode(err, CodeTrackingCapacity) {
		t.Fatalf("expected tracking capacity, got %v", err)
	}
	waitFor(t, func() bool { return s.Snapshot().Completed >= 2 }, "initial completions")
	if err := s.RetireAssignment(AssignmentKey("a1"), 1); err != nil {
		t.Fatal(err)
	}
	// Still at capacity due to tombstone + a2.
	if _, err := s.Submit(baseJob("j3", "a3", 1, ClassNetworkPoll)); !IsCode(err, CodeTrackingCapacity) {
		t.Fatalf("expected capacity with tombstone present, got %v", err)
	}
	if err := s.ForgetAssignmentFence(AssignmentKey("a1"), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(baseJob("j3", "a3", 1, ClassNetworkPoll)); err != nil {
		t.Fatalf("after forget, Submit = %v", err)
	}
	waitFor(t, func() bool { return s.Snapshot().Completed >= 3 }, "a3 completed")
	if err := s.RetireAssignment(AssignmentKey("a2"), 1); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetAssignmentFence(AssignmentKey("a2"), 1); err != nil {
		t.Fatal(err)
	}
	if err := s.RetireAssignment(AssignmentKey("a3"), 1); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetAssignmentFence(AssignmentKey("a3"), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(baseJob("old", "a1", 1, ClassNetworkPoll)); err != nil {
		t.Fatalf("reactivation after forget should allow new work: %v", err)
	}
}

func TestRetireKeepsFenceTombstone(t *testing.T) {
	s := newTestScheduler(t, testConfig(), executorFunc(func(_ context.Context, _ Job) (ExecutionResult, error) {
		return ExecutionResult{}, nil
	}))
	if _, err := s.Submit(baseJob("j1", "asg", 5, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return s.Snapshot().Completed == 1 }, "done")
	if err := s.RetireAssignment(AssignmentKey("asg"), 5); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(baseJob("stale", "asg", 4, ClassNetworkPoll)); !IsCode(err, CodeStaleFence) {
		t.Fatalf("expected stale fence against tombstone, got %v", err)
	}
	if _, err := s.Submit(baseJob("fresh", "asg", 6, ClassNetworkPoll)); err != nil {
		t.Fatalf("newer fence reactivation failed: %v", err)
	}
}

func TestDuplicateIdentityConflict(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	s := newTestScheduler(t, testConfig(), executorFunc(func(ctx context.Context, _ Job) (ExecutionResult, error) {
		close(started)
		select {
		case <-release:
			return ExecutionResult{}, nil
		case <-ctx.Done():
			return ExecutionResult{}, ctx.Err()
		}
	}))
	job := baseJob("same", "asg", 1, ClassNetworkPoll)
	if _, err := s.Submit(job); err != nil {
		t.Fatal(err)
	}
	<-started
	conflict := job
	conflict.Payload = []byte(`{"test":"different"}`)
	if _, err := s.Submit(conflict); !IsCode(err, CodeJobConflict) {
		t.Fatalf("expected job conflict, got %v", err)
	}
	dup := job.clone()
	if submission, err := s.Submit(dup); err != nil || submission.Status != SubmissionDuplicate {
		t.Fatalf("identical duplicate = %+v err=%v", submission, err)
	}
	close(release)
}

func TestNonCooperativeExecutorShutdown(t *testing.T) {
	cfg := testConfig()
	cfg.ShutdownGracePeriod = 50 * time.Millisecond
	release := make(chan struct{})
	s, err := New(cfg, executorFunc(func(_ context.Context, _ Job) (ExecutionResult, error) {
		<-release // ignores ctx
		return ExecutionResult{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(baseJob("block", "asg", 1, ClassNetworkPoll)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return s.Snapshot().ActiveWorkers == 1 }, "worker active")
	err = s.Shutdown(context.Background())
	if err == nil {
		t.Fatal("expected shutdown grace failure")
	}
	if !IsCode(err, CodeDeadlineExceeded) {
		t.Fatalf("shutdown error = %v, want deadline", err)
	}
	if snap := s.Snapshot(); snap.State == StateStopped {
		t.Fatalf("state incorrectly STOPPED while executor still running: %s", snap.State)
	}
	close(release)
	waitFor(t, func() bool { return s.Snapshot().State == StateStopped }, "eventual stop")
}
