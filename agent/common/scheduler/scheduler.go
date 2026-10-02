package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"platform/agent/common/collector"
	"platform/agent/common/identity"
	"platform/agent/common/queue"
	"platform/agent/common/telemetry"
)

type Enqueuer interface {
	Enqueue(payload []byte) (*queue.Record, error)
}

type Scheduler struct {
	ident      *identity.AgentIdentity
	collectors []collector.HostCollector
	enqueuer   Enqueuer
	interval   time.Duration
	timeout    time.Duration
	logger     *slog.Logger
	mu         sync.Mutex
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
	lastRun    time.Time
	runErrors  int
}

func NewScheduler(
	ident *identity.AgentIdentity,
	collectors []collector.HostCollector,
	enqueuer Enqueuer,
	interval time.Duration,
	timeout time.Duration,
	logger *slog.Logger,
) (*Scheduler, error) {
	if ident == nil || ident.AgentID == "" || ident.TenantID == "" || ident.HostID == "" {
		return nil, errors.New("SCHEDULER_IDENTITY_INVALID: complete agent identity required")
	}
	if enqueuer == nil {
		return nil, errors.New("SCHEDULER_ENQUEUER_REQUIRED: enqueuer required")
	}
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Scheduler{
		ident:      ident,
		collectors: collectors,
		enqueuer:   enqueuer,
		interval:   interval,
		timeout:    timeout,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}, nil
}

func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		// Run immediate initial collection
		s.RunOnce(ctx)

		for {
			select {
			case <-ticker.C:
				s.RunOnce(ctx)
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *Scheduler) RunOnce(ctx context.Context) {
	// Mutex prevents overlapping runaway cycles
	s.mu.Lock()
	defer s.mu.Unlock()

	startTime := time.Now().UTC()
	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var observations []telemetry.Observation
	var collectorErrors []telemetry.CollectorError

	for _, c := range s.collectors {
		samples, err := c.Collect(runCtx)
		if err != nil {
			s.runErrors++
			collectorErrors = append(collectorErrors, telemetry.CollectorError{
				Collector: c.Name(),
				Code:      "COLLECTION_FAILED",
				Summary:   err.Error(),
			})
			s.logger.Warn("Collector execution failed",
				"event_code", "AGENT_COLLECTOR_FAILED",
				"result", "FAILED",
				"safe_context", map[string]any{
					"collector":    c.Name(),
					"reason_class": "collection_failed",
				},
			)
			continue
		}

		for _, sample := range samples {
			obs, normErr := telemetry.Normalize(s.ident, c.Name(), startTime, sample)
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
	}

	durationMs := time.Since(startTime).Milliseconds()
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

	if len(observations) > 0 || len(collectorErrors) > 0 {
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
			s.runErrors++
			s.logger.Error("Failed to enqueue collection batch",
				"event_code", "AGENT_COLLECTION_ENQUEUE_FAILED",
				"result", "FAILED",
				"safe_context", map[string]any{"reason_class": "enqueue_failed"},
			)
			return
		}
		s.logger.Debug("Enqueued collection batch",
			"event_code", "AGENT_COLLECTION_ENQUEUED",
			"result", "ACCEPTED",
			"safe_context", map[string]any{
				"observation_count": len(observations),
				"error_count":       len(collectorErrors),
				"duration_ms":       durationMs,
			},
		)
	}

	s.lastRun = startTime
}

func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *Scheduler) Stats() (time.Time, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRun, s.runErrors
}
