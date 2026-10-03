package scheduler

import (
	"context"
	"encoding/json"
	"errors"
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
	Collectors        []CollectorStatus `json:"collectors"`
}

type collectorRuntime struct {
	entry       collector.Entry
	phaseOffset time.Duration
	interval    time.Duration
	timeout     time.Duration

	mu         sync.Mutex
	inFlight   bool
	lastRun    time.Time
	lastError  string
	runCount   uint64
	errorCount uint64
}

type Scheduler struct {
	ident          *identity.AgentIdentity
	registry       *collector.Registry
	enqueuer       Enqueuer
	maxConcurrency int
	logger         *slog.Logger

	runtimes []*collectorRuntime
	sem      chan struct{}

	mu        sync.Mutex
	running   bool
	lastRun   time.Time
	runErrors int

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	inFlightCount atomic.Int32
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
		runtimes = append(runtimes, &collectorRuntime{
			entry:       entry,
			phaseOffset: phaseOffset(entry.Name, interval),
			interval:    interval,
			timeout:     timeout,
		})
	}

	return &Scheduler{
		ident:          ident,
		registry:       registry,
		enqueuer:       enqueuer,
		maxConcurrency: maxConcurrency,
		logger:         logger,
		runtimes:       runtimes,
		sem:            make(chan struct{}, maxConcurrency),
		stopCh:         make(chan struct{}),
	}, nil
}

// phaseOffset returns a deterministic offset in [0, interval) for stable names.
func phaseOffset(name string, interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return time.Duration(h.Sum64() % uint64(interval))
}

func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	for _, rt := range s.runtimes {
		if !rt.entry.Enabled {
			continue
		}
		rt := rt
		s.wg.Add(1)
		go s.runCollectorLoop(ctx, rt)
	}
}

func (s *Scheduler) runCollectorLoop(ctx context.Context, rt *collectorRuntime) {
	defer s.wg.Done()

	timer := time.NewTimer(rt.phaseOffset)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			s.executeCollector(ctx, rt)
			timer.Reset(rt.interval)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) executeCollector(ctx context.Context, rt *collectorRuntime) {
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-s.stopCh:
		return
	case <-ctx.Done():
		return
	}

	s.inFlightCount.Add(1)
	rt.mu.Lock()
	rt.inFlight = true
	rt.mu.Unlock()
	defer func() {
		rt.mu.Lock()
		rt.inFlight = false
		rt.mu.Unlock()
		s.inFlightCount.Add(-1)
	}()

	startTime := time.Now().UTC()
	runCtx, cancel := context.WithTimeout(ctx, rt.timeout)
	defer cancel()

	samples, err := rt.entry.Collector.Collect(runCtx)

	var observations []telemetry.Observation
	var collectorErrors []telemetry.CollectorError

	if err != nil {
		summary := safeErrorSummary(err)
		collectorErrors = append(collectorErrors, telemetry.CollectorError{
			Collector: rt.entry.Name,
			Code:      "COLLECTION_FAILED",
			Summary:   summary,
		})
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

		s.logger.Warn("Collector execution failed",
			"event_code", "AGENT_COLLECTOR_FAILED",
			"result", "FAILED",
			"safe_context", map[string]any{
				"collector":    rt.entry.Name,
				"reason_class": "collection_failed",
			},
		)
	} else {
		for _, sample := range samples {
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
	}

	durationMs := time.Since(startTime).Milliseconds()
	if len(observations) == 0 && len(collectorErrors) == 0 {
		return
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
		CollectedAt:   startTime,
		DurationMs:    durationMs,
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
		rt.mu.Lock()
		rt.errorCount++
		rt.lastError = "enqueue_failed"
		rt.mu.Unlock()
		s.logger.Error("Failed to enqueue collection batch",
			"event_code", "AGENT_COLLECTION_ENQUEUE_FAILED",
			"result", "FAILED",
			"safe_context", map[string]any{
				"collector":    rt.entry.Name,
				"reason_class": "enqueue_failed",
			},
		)
		return
	}

	s.logger.Debug("Enqueued collection batch",
		"event_code", "AGENT_COLLECTION_ENQUEUED",
		"result", "ACCEPTED",
		"safe_context", map[string]any{
			"collector":         rt.entry.Name,
			"observation_count": len(observations),
			"error_count":       len(collectorErrors),
			"duration_ms":       durationMs,
		},
	)
}

// RunOnce executes every enabled collector once with timeout isolation and
// bounded concurrency. Intended for tests and explicit one-shot collection.
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
}

func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
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

// PhaseOffsetForTest exposes deterministic phase calculation for tests.
func PhaseOffsetForTest(name string, interval time.Duration) time.Duration {
	return phaseOffset(name, interval)
}
