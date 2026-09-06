package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sync"
	"time"
)

type Logger interface {
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
}

type Option func(*options)

type options struct {
	logger Logger
	hooks  runtimeHooks
}

func WithLogger(logger Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

func withRuntimeHooks(hooks runtimeHooks) Option {
	return func(o *options) {
		if hooks.now != nil {
			o.hooks.now = hooks.now
		}
		if hooks.wait != nil {
			o.hooks.wait = hooks.wait
		}
		if hooks.jitter != nil {
			o.hooks.jitter = hooks.jitter
		}
	}
}

type queuedJob struct {
	job Job
}

type workerPool struct {
	class ExecutionClass
	cfg   PoolConfig
	queue chan queuedJob
}

type jobRecord struct {
	version        uint64
	leaseExpiresAt time.Time
	fingerprint    [32]byte
}

type assignmentState struct {
	highestSeen uint64
	jobs        map[JobID]jobRecord
}

type activeExecution struct {
	version uint64
	cancel  context.CancelCauseFunc
}

type Scheduler struct {
	cfg      Config
	executor Executor
	logger   Logger
	hooks    runtimeHooks

	mu                   sync.Mutex
	state                State
	pools                map[ExecutionClass]*workerPool
	assignments          map[AssignmentKey]*assignmentState
	fenceTombstones      map[AssignmentKey]uint64
	trackedJobs          int
	active               map[AssignmentKey]map[JobID]activeExecution
	activeByClass        map[ExecutionClass]int
	highestObservedFence uint64
	highestAcceptedFence uint64
	rootCtx              context.Context
	rootCancel           context.CancelCauseFunc
	workers              sync.WaitGroup
	shutdownOnce         sync.Once
	stopped              chan struct{}
	metrics              metricCounters
}

func New(cfg Config, executor Executor, opts ...Option) (*Scheduler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if executor == nil {
		return nil, newError(CodeInvalidConfig, "executor must not be nil", nil)
	}

	resolved := options{hooks: defaultRuntimeHooks()}
	for _, opt := range opts {
		if opt != nil {
			opt(&resolved)
		}
	}

	rootCtx, rootCancel := context.WithCancelCause(context.Background())
	s := &Scheduler{
		cfg:           cloneConfig(cfg),
		executor:      executor,
		logger:        resolved.logger,
		hooks:         resolved.hooks,
		state:         StateRunning,
		pools:         make(map[ExecutionClass]*workerPool, len(cfg.Pools)),
		assignments:     make(map[AssignmentKey]*assignmentState),
		fenceTombstones: make(map[AssignmentKey]uint64),
		active:          make(map[AssignmentKey]map[JobID]activeExecution),
		activeByClass:   make(map[ExecutionClass]int, len(cfg.Pools)),
		rootCtx:         rootCtx,
		rootCancel:      rootCancel,
		stopped:         make(chan struct{}),
	}

	for class, poolCfg := range s.cfg.Pools {
		s.pools[class] = &workerPool{
			class: class,
			cfg:   poolCfg,
			queue: make(chan queuedJob, poolCfg.QueueDepth),
		}
	}
	for _, pool := range s.pools {
		for i := 0; i < pool.cfg.Workers; i++ {
			s.workers.Add(1)
			go s.runWorker(pool)
		}
	}
	return s, nil
}

func (s *Scheduler) Submit(job Job) (Submission, error) {
	s.metrics.submitted.Add(1)
	if err := s.validateJob(job); err != nil {
		return Submission{}, err
	}

	pool, ok := s.pools[job.Class]
	if !ok {
		return Submission{}, newError(CodeExecutionClassUnavailable, "no worker pool configured for execution class", nil)
	}

	now := s.hooks.now()
	fingerprint := jobContentFingerprint(job)
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		return Submission{}, newError(CodeShuttingDown, "scheduler is not accepting new work", nil)
	}

	if tombstone, ok := s.fenceTombstones[job.AssignmentKey]; ok && job.AssignmentVersion < tombstone {
		s.metrics.rejectedStaleLease.Add(1)
		return Submission{}, newError(CodeStaleFence, "job fencing version is older than retired assignment tombstone", nil)
	}

	assignment, exists := s.assignments[job.AssignmentKey]
	if !exists {
		if tombstone, ok := s.fenceTombstones[job.AssignmentKey]; ok && job.AssignmentVersion > tombstone {
			// Newer authoritative fence reactivates the key; drop tombstone first so the
			// assignment slot can be reused without falsely exhausting capacity.
			delete(s.fenceTombstones, job.AssignmentKey)
		}
		if s.trackedAssignmentSlots() >= s.cfg.MaxTrackedAssignments {
			s.metrics.rejectedTrackingCapacity.Add(1)
			return Submission{}, newError(CodeTrackingCapacity, "tracked assignment capacity reached", nil)
		}
		assignment = &assignmentState{
			highestSeen: job.AssignmentVersion,
			jobs:        make(map[JobID]jobRecord),
		}
		s.assignments[job.AssignmentKey] = assignment
		if job.AssignmentVersion > s.highestObservedFence {
			s.highestObservedFence = job.AssignmentVersion
		}
	} else {
		s.pruneExpiredJobsLocked(assignment, now)
		if job.AssignmentVersion < assignment.highestSeen {
			s.metrics.rejectedStaleLease.Add(1)
			return Submission{}, newError(CodeStaleFence, "job fencing version is older than the highest observed version", nil)
		}
		if job.AssignmentVersion > assignment.highestSeen {
			s.observeNewFenceLocked(job.AssignmentKey, assignment, job.AssignmentVersion)
		}
	}

	// A newer fencing version is authoritative stale-work information even when
	// its own lease has expired. Observe/supersede first, then decide whether the
	// new job itself may execute.
	if !job.LeaseExpiresAt.After(now) {
		s.metrics.expired.Add(1)
		return Submission{}, newError(CodeLeaseExpired, "job lease has already expired", nil)
	}

	if record, duplicate := assignment.jobs[job.JobID]; duplicate && record.version == job.AssignmentVersion {
		if record.fingerprint != fingerprint {
			return Submission{}, newError(CodeJobConflict, "duplicate job identity with conflicting immutable content", nil)
		}
		s.metrics.duplicates.Add(1)
		return Submission{Status: SubmissionDuplicate}, nil
	}
	if s.trackedJobs >= s.cfg.MaxTrackedJobs {
		s.metrics.rejectedTrackingCapacity.Add(1)
		return Submission{}, newError(CodeTrackingCapacity, "tracked job capacity reached", nil)
	}

	queued := queuedJob{job: job.clone()}
	select {
	case pool.queue <- queued:
		assignment.jobs[job.JobID] = jobRecord{
			version:        job.AssignmentVersion,
			leaseExpiresAt: job.LeaseExpiresAt,
			fingerprint:    fingerprint,
		}
		s.trackedJobs++
		if job.AssignmentVersion > s.highestAcceptedFence {
			s.highestAcceptedFence = job.AssignmentVersion
		}
		s.metrics.accepted.Add(1)
		return Submission{Status: SubmissionAccepted}, nil
	default:
		s.metrics.rejectedQueueFull.Add(1)
		return Submission{}, newError(CodeQueueFull, "execution-class queue is full", nil)
	}
}

func jobContentFingerprint(job Job) [32]byte {
	h := sha256.New()
	writeFingerprintString(h, string(job.JobID))
	writeFingerprintString(h, string(job.AssignmentKey))
	writeFingerprintString(h, string(job.TenantID))
	writeFingerprintString(h, string(job.EntityID))
	writeFingerprintString(h, string(job.Kind))
	writeFingerprintString(h, string(job.Class))
	var versionBuf [8]byte
	binary.BigEndian.PutUint64(versionBuf[:], job.AssignmentVersion)
	_, _ = h.Write(versionBuf[:])
	_, _ = h.Write(job.Payload)
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

func writeFingerprintString(h interface{ Write([]byte) (int, error) }, value string) {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(value)))
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write([]byte(value))
}

func (s *Scheduler) trackedAssignmentSlots() int {
	return len(s.assignments) + len(s.fenceTombstones)
}

// RetireAssignment is the authoritative Core-driven lifecycle API that releases
// assignment job-tracking capacity while retaining a fencing tombstone so older
// versions remain stale-rejected. It does not silently forget fencing history.
func (s *Scheduler) RetireAssignment(key AssignmentKey, version uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key == "" {
		return newError(CodeInvalidJob, "AssignmentKey must not be empty", nil)
	}

	highest := version
	if assignment, exists := s.assignments[key]; exists {
		if version < assignment.highestSeen {
			return newError(CodeStaleFence, "retire version is older than highest observed fencing version", nil)
		}
		highest = assignment.highestSeen
		if version > highest {
			highest = version
		}
		for jobID := range assignment.jobs {
			delete(assignment.jobs, jobID)
			if s.trackedJobs > 0 {
				s.trackedJobs--
			}
		}
		for _, active := range s.active[key] {
			active.cancel(causeShuttingDown)
		}
		delete(s.assignments, key)
		// Converting an active assignment into a tombstone reuses the same capacity slot.
	} else {
		if tombstone, ok := s.fenceTombstones[key]; ok && version < tombstone {
			return newError(CodeStaleFence, "retire version is older than existing fence tombstone", nil)
		}
		if _, ok := s.fenceTombstones[key]; !ok && s.trackedAssignmentSlots() >= s.cfg.MaxTrackedAssignments {
			return newError(CodeTrackingCapacity, "tracked assignment capacity reached", nil)
		}
	}
	if tombstone, ok := s.fenceTombstones[key]; ok && version < tombstone {
		return newError(CodeStaleFence, "retire version is older than existing fence tombstone", nil)
	}
	if highest > s.highestObservedFence {
		s.highestObservedFence = highest
	}
	s.fenceTombstones[key] = highest
	return nil
}

// ForgetAssignmentFence removes a fencing tombstone after Core authoritatively
// confirms the assignment key will never be reused below the provided version.
func (s *Scheduler) ForgetAssignmentFence(key AssignmentKey, version uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.assignments[key]; exists {
		return newError(CodeInvalidJob, "assignment still active; retire before forgetting fence", nil)
	}
	tombstone, ok := s.fenceTombstones[key]
	if !ok {
		return nil
	}
	if version < tombstone {
		return newError(CodeStaleFence, "forget version is older than fence tombstone", nil)
	}
	delete(s.fenceTombstones, key)
	return nil
}

func (s *Scheduler) releaseTrackedJobLocked(key AssignmentKey, id JobID) {
	assignment, ok := s.assignments[key]
	if !ok {
		return
	}
	if _, exists := assignment.jobs[id]; !exists {
		return
	}
	delete(assignment.jobs, id)
	if s.trackedJobs > 0 {
		s.trackedJobs--
	}
}

func (s *Scheduler) validateJob(job Job) error {
	if job.JobID == "" {
		return newError(CodeInvalidJob, "JobID must not be empty", nil)
	}
	if job.AssignmentKey == "" {
		return newError(CodeInvalidJob, "AssignmentKey must not be empty", nil)
	}
	if job.TenantID == "" {
		return newError(CodeInvalidJob, "TenantID must not be empty", nil)
	}
	if job.Kind == "" {
		return newError(CodeInvalidJob, "JobKind must not be empty", nil)
	}
	if job.Class == "" {
		return newError(CodeInvalidJob, "ExecutionClass must not be empty", nil)
	}
	if job.LeaseExpiresAt.IsZero() {
		return newError(CodeInvalidJob, "LeaseExpiresAt must be set", nil)
	}
	if job.Timeout < 0 {
		return newError(CodeInvalidJob, "Timeout must not be negative", nil)
	}
	if job.Attempt != 0 {
		return newError(CodeInvalidJob, "Attempt is scheduler-owned and must be zero on submission", nil)
	}
	if len(job.Payload) > s.cfg.MaxPayloadBytes {
		return newError(CodeInvalidJob, "Payload exceeds MaxPayloadBytes", nil)
	}
	return nil
}

func (s *Scheduler) observeNewFenceLocked(key AssignmentKey, assignment *assignmentState, version uint64) {
	assignment.highestSeen = version
	if version > s.highestObservedFence {
		s.highestObservedFence = version
	}

	for jobID, record := range assignment.jobs {
		if record.version < version {
			delete(assignment.jobs, jobID)
			if s.trackedJobs > 0 {
				s.trackedJobs--
			}
		}
	}
	for _, active := range s.active[key] {
		if active.version < version {
			active.cancel(causeStaleFence)
		}
	}
}

func (s *Scheduler) pruneExpiredJobsLocked(assignment *assignmentState, now time.Time) {
	for jobID, record := range assignment.jobs {
		if !record.leaseExpiresAt.After(now) {
			delete(assignment.jobs, jobID)
			if s.trackedJobs > 0 {
				s.trackedJobs--
			}
		}
	}
}

func (s *Scheduler) Snapshot() Snapshot {
	now := s.hooks.now()
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, assignment := range s.assignments {
		s.pruneExpiredJobsLocked(assignment, now)
	}

	snapshot := Snapshot{
		State:                         s.state,
		TrackedAssignments:            s.trackedAssignmentSlots(),
		TrackedJobs:                   s.trackedJobs,
		HighestObservedFencingVersion: s.highestObservedFence,
		HighestAcceptedFencingVersion: s.highestAcceptedFence,
		Pools:                         make(map[ExecutionClass]PoolSnapshot, len(s.pools)),
		Submitted:                     s.metrics.submitted.Load(),
		Accepted:                      s.metrics.accepted.Load(),
		Duplicates:                    s.metrics.duplicates.Load(),
		Completed:                     s.metrics.completed.Load(),
		Failed:                        s.metrics.failed.Load(),
		Retries:                       s.metrics.retries.Load(),
		RejectedQueueFull:             s.metrics.rejectedQueueFull.Load(),
		RejectedStaleLease:            s.metrics.rejectedStaleLease.Load(),
		RejectedTrackingCapacity:      s.metrics.rejectedTrackingCapacity.Load(),
		Expired:                       s.metrics.expired.Load(),
		Cancelled:                     s.metrics.cancelled.Load(),
		DeadlineExceeded:              s.metrics.deadlineExceeded.Load(),
	}

	for class, pool := range s.pools {
		active := s.activeByClass[class]
		poolSnapshot := PoolSnapshot{
			Workers:       pool.cfg.Workers,
			ActiveWorkers: active,
			QueueDepth:    len(pool.queue),
			QueueCapacity: cap(pool.queue),
		}
		snapshot.Pools[class] = poolSnapshot
		snapshot.ConfiguredWorkers += pool.cfg.Workers
		snapshot.ActiveWorkers += active
		snapshot.QueueDepth += poolSnapshot.QueueDepth
		snapshot.QueueCapacity += poolSnapshot.QueueCapacity
	}
	return snapshot
}

func (s *Scheduler) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.shutdownOnce.Do(func() {
		s.mu.Lock()
		if s.state == StateRunning {
			s.state = StateShuttingDown
			s.rootCancel(causeShuttingDown)
			for _, pool := range s.pools {
				close(pool.queue)
			}
		}
		s.mu.Unlock()

		go func() {
			s.workers.Wait()
			s.mu.Lock()
			s.state = StateStopped
			s.mu.Unlock()
			close(s.stopped)
		}()
	})

	graceCtx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownGracePeriod)
	defer cancel()
	select {
	case <-s.stopped:
		return nil
	case <-graceCtx.Done():
		code := CodeDeadlineExceeded
		if errors.Is(graceCtx.Err(), context.Canceled) {
			code = CodeCancelled
		}
		if s.logger != nil {
			s.logger.Warn("sensor scheduler shutdown grace expired", "error_code", code)
		}
		return newError(code, "scheduler shutdown did not complete within grace", graceCtx.Err())
	}
}
