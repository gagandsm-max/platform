package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"platform/agent/common/collector"
	"platform/agent/common/identity"
	"platform/agent/common/queue"
	"platform/agent/common/telemetry"
)

const (
	defaultMaxConcurrency = 2
	defaultInterval       = 15 * time.Second
	defaultTimeout        = 5 * time.Second

	// phaseAlgorithmVersion is an explicit input to the deterministic phase key so
	// future algorithm changes do not silently reshuffle an estate.
	phaseAlgorithmVersion = "v1"

	// maxPendingItemsPerCollector bounds merged observations+errors retained for
	// one collector between aggregate flushes. Overflow is explicit, never silent.
	maxPendingItemsPerCollector = 256
)

type Enqueuer interface {
	Enqueue(payload []byte) (*queue.Record, error)
}

// CollectorStatus is per-collector scheduler evidence.
type CollectorStatus struct {
	Name        string        `json:"name"`
	Enabled     bool          `json:"enabled"`
	Interval    time.Duration `json:"interval"`
	Timeout     time.Duration `json:"timeout"`
	PhaseOffset time.Duration `json:"phase_offset"`
	InFlight    bool          `json:"in_flight"`
	Outstanding bool          `json:"outstanding,omitempty"`
	LastRun     time.Time     `json:"last_run,omitempty"`
	LastError   string        `json:"last_error,omitempty"`
	RunCount    uint64        `json:"run_count"`
	ErrorCount  uint64        `json:"error_count"`
}

// Status is scheduler operability evidence for health/support surfaces.
type Status struct {
	Running           bool              `json:"running"`
	MaxConcurrency    int               `json:"max_concurrency"`
	InFlight          int               `json:"in_flight"`
	EnabledCollectors int               `json:"enabled_collectors"`
	TotalCollectors   int               `json:"total_collectors"`
	LastRun           time.Time         `json:"last_run,omitempty"`
	RunErrors         int               `json:"run_errors"`
	PendingOverflows  int               `json:"pending_overflows,omitempty"`
	Collectors        []CollectorStatus `json:"collectors"`
}

type collectResult struct {
	samples []telemetry.Sample
	err     error
}

// pendingContribution is one collector's deposit into the open aggregate cycle.
type pendingContribution struct {
	name         string
	collectedAt  time.Time
	durationMs   int64
	observations []telemetry.Observation
	errors       []telemetry.CollectorError
}

type collectorRuntime struct {
	entry       collector.Entry
	phaseOffset time.Duration
	interval    time.Duration
	timeout     time.Duration

	mu          sync.Mutex
	inFlight    bool
	outstanding bool // non-cooperative Collect still running after timeout
	lastRun     time.Time
	lastError   string
	runCount    uint64
	errorCount  uint64
}

type Scheduler struct {
	ident          *identity.AgentIdentity
	registry       *collector.Registry
	enqueuer       Enqueuer
	maxConcurrency int
	flushInterval  time.Duration
	logger         *slog.Logger
	nowFunc        func() time.Time // optional clock injection for deterministic tests

	runtimes []*collectorRuntime
	sem      chan struct{}

	pendingMu  sync.Mutex
	pending    map[string]pendingContribution
	cycleStart time.Time

	mu               sync.Mutex
	running          bool
	lastRun          time.Time
	runErrors        int
	pendingOverflows int

	stopCh   chan struct{}
	stopOnce sync.Once
	stopDone chan struct{} // closed exactly once after loops + accepted executions complete
	wg       sync.WaitGroup

	// execMu + stopping gate registration of ALL collector executions (loops and RunOnce).
	// execWG tracks every accepted execution through Collect completion, including
	// non-cooperative outstanding waiters. Stop sets stopping under execMu before Wait
	// so no positive Add can race a zero-counter Wait.
	execMu   sync.Mutex
	stopping bool
	execWG   sync.WaitGroup

	inFlightCount atomic.Int32

	// classifyBarrier, when set by tests, runs after runCtx termination is observed
	// and before failure/shutdown classification. Production remains nil.
	classifyBarrier func()
}

func NewScheduler(
	ident *identity.AgentIdentity,
	registry *collector.Registry,
	enqueuer Enqueuer,
	maxConcurrency int,
	logger *slog.Logger,
) (*Scheduler, error) {
	if ident == nil || ident.AgentID == "" || ident.TenantID == "" || ident.HostID == "" {
		return nil, errors.New("SCHEDULER_IDENTITY_INVALID: complete agent identity required")
	}
	if registry == nil || registry.Len() == 0 {
		return nil, errors.New("SCHEDULER_REGISTRY_REQUIRED: non-empty collector registry required")
	}
	if enqueuer == nil {
		return nil, errors.New("SCHEDULER_ENQUEUER_REQUIRED: enqueuer required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if maxConcurrency <= 0 {
		maxConcurrency = defaultMaxConcurrency
	}
	enabled := registry.EnabledEntries()
	if len(enabled) == 0 {
		return nil, errors.New("SCHEDULER_NO_ENABLED_COLLECTORS: at least one enabled collector is required")
	}
	if maxConcurrency > len(enabled) {
		maxConcurrency = len(enabled)
	}

	flushInterval := time.Duration(0)
	runtimes := make([]*collectorRuntime, 0, registry.Len())
	for _, entry := range registry.Entries() {
		interval := entry.Interval
		if interval <= 0 {
			interval = defaultInterval
		}
		timeout := entry.Timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}
		if entry.Enabled {
			if flushInterval <= 0 || interval < flushInterval {
				flushInterval = interval
			}
		}
		runtimes = append(runtimes, &collectorRuntime{
			entry: entry,
			phaseOffset: phaseOffset(
				ident.TenantID,
				ident.AgentID,
				entry.Name,
				phaseAlgorithmVersion,
				interval,
			),
			interval: interval,
			timeout:  timeout,
		})
	}
	if flushInterval <= 0 {
		flushInterval = defaultInterval
	}

	return &Scheduler{
		ident:          ident,
		registry:       registry,
		enqueuer:       enqueuer,
		maxConcurrency: maxConcurrency,
		flushInterval:  flushInterval,
		logger:         logger,
		runtimes:       runtimes,
		sem:            make(chan struct{}, maxConcurrency),
		pending:        make(map[string]pendingContribution),
		stopCh:         make(chan struct{}),
		stopDone:       make(chan struct{}),
	}, nil
}

func (s *Scheduler) now() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc().UTC()
	}
	return time.Now().UTC()
}

// phaseOffset returns a deterministic offset in [0, interval) from fleet-stable inputs.
// Sensor identity is intentionally excluded so Sensor reassignment does not reshuffle phase.
func phaseOffset(tenantID, agentID, collectorName, algorithmVersion string, interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(tenantID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(agentID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(collectorName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(algorithmVersion))
	return time.Duration(h.Sum64() % uint64(interval))
}

// nextCadenceDue returns the earliest Unix-epoch-anchored due time at or after now:
// due(k) = epoch + phase + k*interval.
func nextCadenceDue(now time.Time, interval, phase time.Duration) time.Time {
	if interval <= 0 {
		return now.UTC()
	}
	nowNS := now.UTC().UnixNano()
	intervalNS := interval.Nanoseconds()
	phaseNS := phase.Nanoseconds()
	if nowNS <= phaseNS {
		return time.Unix(0, phaseNS).UTC()
	}
	elapsed := nowNS - phaseNS
	k := elapsed / intervalNS
	dueNS := phaseNS + k*intervalNS
	if dueNS < nowNS {
		dueNS += intervalNS
	}
	return time.Unix(0, dueNS).UTC()
}

// nextCadenceDueAfter returns the next due strictly after `after`, coalescing any
// missed periods so at most one future boundary is scheduled.
func nextCadenceDueAfter(after time.Time, interval, phase time.Duration) time.Time {
	if interval <= 0 {
		return after.UTC().Add(time.Nanosecond)
	}
	due := nextCadenceDue(after, interval, phase)
	if !due.After(after) {
		due = due.Add(interval)
	}
	return due
}

// coalesceNextDue advances from a fired due boundary to the next future nominal
// boundary after wall-clock now, skipping missed periods without catch-up work.
func coalesceNextDue(firedDue, now time.Time, interval, phase time.Duration) time.Time {
	next := nextCadenceDueAfter(firedDue, interval, phase)
	now = now.UTC()
	for !next.After(now) {
		next = next.Add(interval)
	}
	return next
}

func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.runFlushLoop(ctx)

	for _, rt := range s.runtimes {
		if !rt.entry.Enabled {
			continue
		}
		rt := rt
		s.wg.Add(1)
		go s.runCollectorLoop(ctx, rt)
	}
}

func (s *Scheduler) runFlushLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.flushPending()
		case <-s.stopCh:
			s.flushPending()
			return
		case <-ctx.Done():
			s.flushPending()
			return
		}
	}
}

func (s *Scheduler) runCollectorLoop(ctx context.Context, rt *collectorRuntime) {
	defer s.wg.Done()

	now := s.now()
	due := nextCadenceDue(now, rt.interval, rt.phaseOffset)
	timer := time.NewTimer(due.Sub(now))
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			rt.mu.Lock()
			busy := rt.inFlight || rt.outstanding
			rt.mu.Unlock()
			if !busy {
				s.executeCollector(ctx, rt)
			}
			// Nominal cadence: advance from the fired due boundary using wall clock
			// only to coalesce/skip missed periods. Execution/admission duration must
			// not permanently shift future dues.
			now = s.now()
			due = coalesceNextDue(due, now, rt.interval, rt.phaseOffset)
			delay := due.Sub(now)
			if delay < 0 {
				delay = 0
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(delay)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// admitExecution stop-gates and single-flight-admits one collector execution.
// On true, the caller owns: sem slot, inFlightCount+1, rt.inFlight=true, execWG+1
// and must release via normal completion or spawnOutstandingWaiter.
//
// Per-collector busy state is revalidated only after semaphore acquisition so a
// waiter can observe outstanding after the holder releases admission.
func (s *Scheduler) admitExecution(rt *collectorRuntime, ctx context.Context) bool {
	s.execMu.Lock()
	stopping := s.stopping
	s.execMu.Unlock()
	if stopping {
		return false
	}

	select {
	case s.sem <- struct{}{}:
	case <-s.stopCh:
		return false
	case <-ctx.Done():
		return false
	}

	// Re-validate under execMu after admission so Stop cannot race a new Add
	// and a second RunOnce cannot proceed while the first is in-flight/outstanding.
	s.execMu.Lock()
	if s.stopping {
		s.execMu.Unlock()
		<-s.sem
		return false
	}
	rt.mu.Lock()
	if rt.outstanding || rt.inFlight {
		rt.mu.Unlock()
		s.execMu.Unlock()
		<-s.sem
		return false
	}
	rt.inFlight = true
	rt.mu.Unlock()
	s.execWG.Add(1)
	s.execMu.Unlock()

	s.inFlightCount.Add(1)
	return true
}

func (s *Scheduler) executeCollector(ctx context.Context, rt *collectorRuntime) {
	if !s.admitExecution(rt, ctx) {
		return
	}

	var releaseOnce sync.Once
	releaseAdmission := func() {
		releaseOnce.Do(func() {
			<-s.sem
			s.inFlightCount.Add(-1)
		})
	}

	outstandingOwned := false
	defer func() {
		if outstandingOwned {
			return
		}
		rt.mu.Lock()
		rt.inFlight = false
		rt.mu.Unlock()
		releaseAdmission()
		s.execWG.Done()
	}()

	startTime := s.now()
	// WithTimeout preserves immutable Cause: DeadlineExceeded if the per-collector
	// deadline wins first; Canceled if parent/service cancellation wins first.
	runCtx, cancel := context.WithTimeout(ctx, rt.timeout)

	resultCh := make(chan collectResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- collectResult{err: fmt.Errorf("COLLECTION_PANIC: %v", r)}
			}
		}()
		samples, err := rt.entry.Collector.Collect(runCtx)
		resultCh <- collectResult{samples: samples, err: err}
	}()

	var res collectResult
	timedOut := false
	select {
	case res = <-resultCh:
		cancel()
	case <-runCtx.Done():
		select {
		case res = <-resultCh:
			cancel()
		default:
			timedOut = true
			cancel()
		}
	}

	// Capture cause before any test barrier / further scheduling work. Cause is
	// immutable once runCtx is done, so later Stop() cannot rewrite classification.
	runCause := context.Cause(runCtx)
	if s.classifyBarrier != nil {
		s.classifyBarrier()
	}

	if timedOut {
		if errors.Is(runCause, context.Canceled) && !errors.Is(runCause, context.DeadlineExceeded) {
			// Service/runtime cancellation won first — suppress synthetic failure.
			releaseAdmission()
			outstandingOwned = true
			s.spawnOutstandingWaiter(rt, resultCh)
			return
		}

		summary := "collection_timeout"
		if err := runCtx.Err(); err != nil {
			summary = safeErrorSummary(err)
		}
		s.finishCollectorFailure(rt, startTime, summary)
		s.deposit(pendingContribution{
			name:        rt.entry.Name,
			collectedAt: startTime,
			durationMs:  s.now().Sub(startTime).Milliseconds(),
			errors: []telemetry.CollectorError{{
				Collector: rt.entry.Name,
				Code:      "COLLECTION_FAILED",
				Summary:   summary,
			}},
		})

		releaseAdmission()
		outstandingOwned = true
		// Bound: at most one waiter per collector while outstanding.
		s.spawnOutstandingWaiter(rt, resultCh)
		return
	}

	durationMs := s.now().Sub(startTime).Milliseconds()
	if res.err != nil {
		if isContextTermination(res.err) &&
			errors.Is(runCause, context.Canceled) &&
			!errors.Is(runCause, context.DeadlineExceeded) {
			// Parent/service cancellation won first.
			return
		}
		summary := safeErrorSummary(res.err)
		s.finishCollectorFailure(rt, startTime, summary)
		s.deposit(pendingContribution{
			name:        rt.entry.Name,
			collectedAt: startTime,
			durationMs:  durationMs,
			errors: []telemetry.CollectorError{{
				Collector: rt.entry.Name,
				Code:      "COLLECTION_FAILED",
				Summary:   summary,
			}},
		})
		s.logger.Warn("Collector execution failed",
			"event_code", "AGENT_COLLECTOR_FAILED",
			"result", "FAILED",
			"safe_context", map[string]any{
				"collector":    rt.entry.Name,
				"reason_class": "collection_failed",
			},
		)
		return
	}

	var observations []telemetry.Observation
	for _, sample := range res.samples {
		obs, normErr := telemetry.Normalize(s.ident, rt.entry.Name, startTime, sample)
		if normErr != nil {
			s.logger.Debug("Telemetry normalization rejected sample",
				"event_code", "AGENT_TELEMETRY_NORMALIZE_REJECTED",
				"result", "REJECTED",
				"safe_context", map[string]any{
					"metric_key":   sample.MetricKey,
					"reason_class": "normalize_rejected",
				},
			)
			continue
		}
		observations = append(observations, obs)
	}

	rt.mu.Lock()
	rt.runCount++
	rt.lastRun = startTime
	rt.lastError = ""
	rt.mu.Unlock()

	s.mu.Lock()
	s.lastRun = startTime
	s.mu.Unlock()

	if len(observations) == 0 {
		return
	}
	s.deposit(pendingContribution{
		name:         rt.entry.Name,
		collectedAt:  startTime,
		durationMs:   durationMs,
		observations: observations,
	})
}

func (s *Scheduler) finishCollectorFailure(rt *collectorRuntime, startTime time.Time, summary string) {
	rt.mu.Lock()
	rt.runCount++
	rt.errorCount++
	rt.lastRun = startTime
	rt.lastError = summary
	rt.mu.Unlock()

	s.mu.Lock()
	s.runErrors++
	s.lastRun = startTime
	s.mu.Unlock()
}

func pendingItemCount(c pendingContribution) int {
	return len(c.observations) + len(c.errors)
}

func hasPendingOverflow(c pendingContribution) bool {
	for _, e := range c.errors {
		if e.Code == "PENDING_OVERFLOW" {
			return true
		}
	}
	return false
}

func (s *Scheduler) deposit(c pendingContribution) {
	if c.name == "" {
		return
	}
	if len(c.observations) == 0 && len(c.errors) == 0 {
		return
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if s.cycleStart.IsZero() || c.collectedAt.Before(s.cycleStart) {
		s.cycleStart = c.collectedAt
	}

	existing, ok := s.pending[c.name]
	if !ok {
		if pendingItemCount(c) > maxPendingItemsPerCollector {
			s.recordPendingOverflowLocked(c.name, &c)
			s.pending[c.name] = c
			return
		}
		s.pending[c.name] = c
		return
	}

	mergedCount := pendingItemCount(existing) + pendingItemCount(c)
	if mergedCount > maxPendingItemsPerCollector {
		s.recordPendingOverflowLocked(c.name, &existing)
		s.pending[c.name] = existing
		return
	}

	// Merge: preserve every accepted contribution until flush consumes it.
	if !c.collectedAt.IsZero() && (existing.collectedAt.IsZero() || c.collectedAt.Before(existing.collectedAt)) {
		existing.collectedAt = c.collectedAt
	}
	if c.durationMs > existing.durationMs {
		existing.durationMs = c.durationMs
	}
	existing.observations = append(existing.observations, c.observations...)
	existing.errors = append(existing.errors, c.errors...)
	s.pending[c.name] = existing
}

func (s *Scheduler) recordPendingOverflowLocked(collectorName string, target *pendingContribution) {
	s.mu.Lock()
	s.pendingOverflows++
	s.runErrors++
	s.mu.Unlock()
	if !hasPendingOverflow(*target) {
		target.errors = append(target.errors, telemetry.CollectorError{
			Collector: collectorName,
			Code:      "PENDING_OVERFLOW",
			Summary:   "pending aggregation bound exceeded; contribution dropped",
		})
	}
	// Hard bound must hold for observation-heavy and error-heavy pending alike.
	// Drop oldest observations first, then oldest non-overflow errors; never drop
	// the PENDING_OVERFLOW marker itself.
	for pendingItemCount(*target) > maxPendingItemsPerCollector {
		if len(target.observations) > 0 {
			target.observations = target.observations[1:]
			continue
		}
		if !trimOldestNonOverflowError(target) {
			break
		}
	}
	s.logger.Warn("Pending aggregation overflow",
		"event_code", "AGENT_COLLECTION_PENDING_OVERFLOW",
		"result", "REJECTED",
		"safe_context", map[string]any{
			"collector":    collectorName,
			"reason_class": "pending_overflow",
			"bound":        maxPendingItemsPerCollector,
		},
	)
}

func trimOldestNonOverflowError(target *pendingContribution) bool {
	for i, e := range target.errors {
		if e.Code == "PENDING_OVERFLOW" {
			continue
		}
		target.errors = append(target.errors[:i], target.errors[i+1:]...)
		return true
	}
	return false
}

// flushPending aggregates per-collector deposits into one CollectionBatch so
// durable-queue record cardinality tracks the collection cycle, not collector count.
func (s *Scheduler) flushPending() {
	s.pendingMu.Lock()
	if len(s.pending) == 0 {
		s.pendingMu.Unlock()
		return
	}
	contribs := make([]pendingContribution, 0, len(s.pending))
	for _, rt := range s.runtimes {
		if c, ok := s.pending[rt.entry.Name]; ok {
			contribs = append(contribs, c)
		}
	}
	cycleStart := s.cycleStart
	s.pending = make(map[string]pendingContribution)
	s.cycleStart = time.Time{}
	s.pendingMu.Unlock()

	var observations []telemetry.Observation
	var collectorErrors []telemetry.CollectorError
	var maxDur int64
	for _, c := range contribs {
		observations = append(observations, c.observations...)
		collectorErrors = append(collectorErrors, c.errors...)
		if c.durationMs > maxDur {
			maxDur = c.durationMs
		}
	}
	if len(observations) == 0 && len(collectorErrors) == 0 {
		return
	}
	if cycleStart.IsZero() {
		cycleStart = s.now()
	}

	batch := telemetry.CollectionBatch{
		SchemaVersion: telemetry.SchemaVersion,
		SourceType:    "AGENT",
		TenantID:      s.ident.TenantID,
		AgentID:       s.ident.AgentID,
		HostID:        s.ident.HostID,
		Hostname:      s.ident.Hostname,
		OS:            s.ident.OS,
		Arch:          s.ident.Arch,
		CollectedAt:   cycleStart,
		DurationMs:    maxDur,
		Observations:  observations,
		Errors:        collectorErrors,
	}

	payload, err := json.Marshal(batch)
	if err != nil {
		s.logger.Error("Failed to marshal collection batch",
			"event_code", "AGENT_COLLECTION_MARSHAL_FAILED",
			"result", "FAILED",
			"safe_context", map[string]any{"reason_class": "marshal_failed"},
		)
		return
	}

	if _, err := s.enqueuer.Enqueue(payload); err != nil {
		s.mu.Lock()
		s.runErrors++
		s.mu.Unlock()
		for _, c := range contribs {
			if rt := s.runtimeByName(c.name); rt != nil {
				rt.mu.Lock()
				rt.errorCount++
				rt.lastError = "enqueue_failed"
				rt.mu.Unlock()
			}
		}
		s.logger.Error("Failed to enqueue collection batch",
			"event_code", "AGENT_COLLECTION_ENQUEUE_FAILED",
			"result", "FAILED",
			"safe_context", map[string]any{
				"reason_class":      "enqueue_failed",
				"observation_count": len(observations),
				"error_count":       len(collectorErrors),
			},
		)
		return
	}

	s.logger.Debug("Enqueued collection batch",
		"event_code", "AGENT_COLLECTION_ENQUEUED",
		"result", "ACCEPTED",
		"safe_context", map[string]any{
			"observation_count": len(observations),
			"error_count":       len(collectorErrors),
			"duration_ms":       maxDur,
			"collector_count":   len(contribs),
		},
	)
}

func (s *Scheduler) runtimeByName(name string) *collectorRuntime {
	for _, rt := range s.runtimes {
		if rt.entry.Name == name {
			return rt
		}
	}
	return nil
}

// RunOnce executes every enabled collector once with timeout isolation and
// bounded concurrency, then flushes one aggregate durable batch. Executions are
// stop-gated through the same admitExecution lifecycle as scheduler loops; once
// Stop begins, RunOnce does not start new collector work.
func (s *Scheduler) RunOnce(ctx context.Context) {
	var wg sync.WaitGroup
	for _, rt := range s.runtimes {
		if !rt.entry.Enabled {
			continue
		}
		rt := rt
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.executeCollector(ctx, rt)
		}()
	}
	wg.Wait()
	s.flushPending()
}

func isContextTermination(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// spawnOutstandingWaiter keeps the accepted execution on execWG until Collect
// finishes. At most one outstanding waiter per collector is permitted.
func (s *Scheduler) spawnOutstandingWaiter(rt *collectorRuntime, resultCh <-chan collectResult) {
	rt.mu.Lock()
	if rt.outstanding {
		// Unreachable under correct single-flight admission; finish our execWG credit.
		rt.inFlight = false
		rt.mu.Unlock()
		go func() {
			<-resultCh
			s.execWG.Done()
		}()
		return
	}
	rt.outstanding = true
	rt.mu.Unlock()

	go func() {
		defer s.execWG.Done()
		<-resultCh
		rt.mu.Lock()
		rt.outstanding = false
		rt.inFlight = false
		rt.mu.Unlock()
	}()
}

func (s *Scheduler) beginStop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		// Atomically close admission before waiting so no new execWG.Add can race.
		s.execMu.Lock()
		s.stopping = true
		s.execMu.Unlock()
		go func() {
			s.wg.Wait()
			s.execWG.Wait()
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			close(s.stopDone)
		}()
	})
}

// StopContext requests scheduler shutdown and waits until loops and accepted
// collector executions (including outstanding waiters) exit, or ctx ends.
// Exactly one waiter coordinator is started; repeated calls share stopDone.
func (s *Scheduler) StopContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.beginStop()
	select {
	case <-s.stopDone:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("SCHEDULER_STOP_TIMEOUT: %w", ctx.Err())
	}
}

// SetClassifyBarrierForTest installs a hook invoked after runCtx termination is
// observed and before classification. Production callers must leave this unset.
func (s *Scheduler) SetClassifyBarrierForTest(fn func()) {
	s.classifyBarrier = fn
}

// Stop is a compatibility wrapper that waits without an external deadline.
func (s *Scheduler) Stop() {
	_ = s.StopContext(context.Background())
}

// Stats preserves the aggregate (lastRun, runErrors) contract used by health.
func (s *Scheduler) Stats() (time.Time, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRun, s.runErrors
}

// Status returns detailed scheduler/collector evidence without readiness semantics.
func (s *Scheduler) Status() Status {
	s.mu.Lock()
	running := s.running
	lastRun := s.lastRun
	runErrors := s.runErrors
	pendingOverflows := s.pendingOverflows
	s.mu.Unlock()

	collectors := make([]CollectorStatus, 0, len(s.runtimes))
	enabled := 0
	for _, rt := range s.runtimes {
		rt.mu.Lock()
		st := CollectorStatus{
			Name:        rt.entry.Name,
			Enabled:     rt.entry.Enabled,
			Interval:    rt.interval,
			Timeout:     rt.timeout,
			PhaseOffset: rt.phaseOffset,
			InFlight:    rt.inFlight,
			Outstanding: rt.outstanding,
			LastRun:     rt.lastRun,
			LastError:   rt.lastError,
			RunCount:    rt.runCount,
			ErrorCount:  rt.errorCount,
		}
		rt.mu.Unlock()
		if st.Enabled {
			enabled++
		}
		collectors = append(collectors, st)
	}

	return Status{
		Running:           running,
		MaxConcurrency:    s.maxConcurrency,
		InFlight:          int(s.inFlightCount.Load()),
		EnabledCollectors: enabled,
		TotalCollectors:   len(s.runtimes),
		LastRun:           lastRun,
		RunErrors:         runErrors,
		PendingOverflows:  pendingOverflows,
		Collectors:        collectors,
	}
}

func safeErrorSummary(err error) string {
	if err == nil {
		return ""
	}
	// Preserve existing safe summary behavior: error string only, no payloads.
	summary := err.Error()
	if len(summary) > 256 {
		return summary[:256]
	}
	return summary
}

// PhaseAlgorithmVersionForTest exposes the explicit phase algorithm version.
func PhaseAlgorithmVersionForTest() string { return phaseAlgorithmVersion }

// PhaseOffsetForTest exposes deterministic fleet-safe phase calculation for tests.
func PhaseOffsetForTest(tenantID, agentID, collectorName, algorithmVersion string, interval time.Duration) time.Duration {
	return phaseOffset(tenantID, agentID, collectorName, algorithmVersion, interval)
}

// NextCadenceDueForTest exposes epoch-anchored due calculation for tests.
func NextCadenceDueForTest(now time.Time, interval, phase time.Duration) time.Time {
	return nextCadenceDue(now, interval, phase)
}

// CoalesceNextDueForTest exposes missed-period coalesce/skip for tests.
func CoalesceNextDueForTest(firedDue, now time.Time, interval, phase time.Duration) time.Time {
	return coalesceNextDue(firedDue, now, interval, phase)
}
