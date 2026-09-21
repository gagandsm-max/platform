package recurring

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"platform/sensor/internal/coreclient"
)

// CoreClient is the narrow Core surface for recurring schedule sync.
type CoreClient interface {
	FetchRecurringScheduleDesiredState(ctx context.Context) (coreclient.RecurringScheduleDesiredState, error)
	AckRecurringScheduleAppliedState(ctx context.Context, generation int64, checksum string) (coreclient.AppliedStateAck, error)
}

// SyncerConfig configures the recurring syncer.
type SyncerConfig struct {
	Dir            string
	ExpectedTenant string
	PollInterval   time.Duration
	Logger         *slog.Logger
	Metrics        *Metrics
}

// Syncer pulls Core desired state, applies under fencing, persists and ACKs.
// On Core fetch failure it keeps running from durable applied state.
type Syncer struct {
	cfg     SyncerConfig
	client  CoreClient
	store   *Store
	engine  *DueEngine
	metrics *Metrics

	mu      sync.RWMutex
	applied AppliedState

	stop chan struct{}
	done chan struct{}
}

// New restores durable applied state (fail-closed on corrupt) and returns a Syncer.
func New(cfg SyncerConfig, client CoreClient, engine *DueEngine) (*Syncer, error) {
	if client == nil {
		return nil, fmt.Errorf("%s: core client is required", ErrCodeInvalidConfig)
	}
	if engine == nil {
		engine = NewDueEngine()
	}
	store, err := NewStore(cfg.Dir)
	if err != nil {
		return nil, err
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Metrics == nil {
		cfg.Metrics = &Metrics{}
	}
	s := &Syncer{
		cfg:     cfg,
		client:  client,
		store:   store,
		engine:  engine,
		metrics: cfg.Metrics,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	applied, present, err := store.LoadApplied()
	if err != nil {
		s.metrics.AddRestoreFailures(1)
		return nil, err
	}
	if !present {
		applied = AppliedState{Generation: 0, Definitions: []RecurringDefinition{}}
	}
	s.applied = applied
	s.metrics.SetAppliedGeneration(applied.Generation)
	s.metrics.SetActiveDefinitions(int64(len(applied.Definitions)))
	// Restart must not stampede: suppress the in-progress cadence slot so due
	// work resumes on the next phase-aligned cycle (phase spreading still holds).
	if present && len(applied.Definitions) > 0 {
		engine.SuppressCurrentCycle(time.Now().UTC(), applied.Definitions)
	}
	return s, nil
}

// Applied returns a copy of the in-memory applied state.
func (s *Syncer) Applied() AppliedState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.applied
	out.Definitions = append([]RecurringDefinition(nil), s.applied.Definitions...)
	return out
}

// ApplyInMemory applies a desired state without Core (unit tests / local inject).
func (s *Syncer) ApplyInMemory(desired DesiredState) (ApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics.SetDesiredGeneration(desired.Generation)
	next, result, err := ApplyDesired(s.applied, desired, s.cfg.ExpectedTenant)
	if err != nil {
		switch result.ReasonCode {
		case ErrCodeStateRegression:
			s.metrics.AddRejectedStaleDesired(1)
		case ErrCodeStateConflict:
			s.metrics.AddRejectedConflict(1)
		}
		return result, err
	}
	if result.Noop {
		return result, nil
	}
	if err := s.store.SaveApplied(next); err != nil {
		return ApplyResult{Rejected: true, ReasonCode: ErrCodeDurableWrite}, err
	}
	s.applied = next
	s.engine.Reset()
	s.metrics.SetAppliedGeneration(next.Generation)
	s.metrics.SetActiveDefinitions(int64(len(next.Definitions)))
	return result, nil
}

// SyncOnce fetches Core desired state, applies, persists and ACKs.
// Fetch failures leave durable applied running (temporary Core outage).
func (s *Syncer) SyncOnce(ctx context.Context) error {
	desiredWire, err := s.client.FetchRecurringScheduleDesiredState(ctx)
	if err != nil {
		if s.cfg.Logger != nil {
			s.cfg.Logger.Warn("recurring desired-state fetch failed; continuing from durable applied", "error", err)
		}
		return err
	}
	desired := DesiredState{
		Generation:  desiredWire.Generation,
		Checksum:    desiredWire.Checksum,
		Definitions: mapWireDefinitions(desiredWire.Definitions),
	}
	s.metrics.SetDesiredGeneration(desired.Generation)

	s.mu.Lock()
	next, result, applyErr := ApplyDesired(s.applied, desired, s.cfg.ExpectedTenant)
	if applyErr != nil {
		switch result.ReasonCode {
		case ErrCodeStateRegression:
			s.metrics.AddRejectedStaleDesired(1)
		case ErrCodeStateConflict:
			s.metrics.AddRejectedConflict(1)
		}
		s.mu.Unlock()
		return applyErr
	}
	if !result.Noop {
		if err := s.store.SaveApplied(next); err != nil {
			s.mu.Unlock()
			return err
		}
		s.applied = next
		s.engine.Reset()
		s.metrics.SetAppliedGeneration(next.Generation)
		s.metrics.SetActiveDefinitions(int64(len(next.Definitions)))
	}
	applied := s.applied
	s.mu.Unlock()

	if applied.Generation == 0 {
		return nil
	}
	_, ackErr := s.client.AckRecurringScheduleAppliedState(ctx, applied.Generation, applied.Checksum)
	return ackErr
}

// Start begins background polling.
func (s *Syncer) Start(ctx context.Context) {
	go s.loop(ctx)
}

// Shutdown stops the background loop.
func (s *Syncer) Shutdown(ctx context.Context) error {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Syncer) loop(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	_ = s.SyncOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case <-ticker.C:
			_ = s.SyncOnce(ctx)
		}
	}
}

func mapWireDefinitions(in []coreclient.RecurringScheduleDefinition) []RecurringDefinition {
	out := make([]RecurringDefinition, 0, len(in))
	for _, d := range in {
		def := RecurringDefinition{
			AssignmentID:         d.AssignmentID,
			TenantID:             d.TenantID,
			EntityID:             d.EntityID,
			MonitorDefinitionID:  d.MonitorDefinitionID,
			SchedulingClass:      SchedulingClass(d.SchedulingClass),
			ExecutionClass:       d.ExecutionClass,
			CadenceMs:            d.CadenceMs,
			FreshnessMs:          d.FreshnessMs,
			PhaseVersion:         d.PhaseVersion,
			Generation:           d.Generation,
			ExecutionTimeoutMs:   d.ExecutionTimeoutMs,
			CredentialProfileRef: d.CredentialProfileRef,
			Protocol:             d.Protocol,
		}
		if d.SNMP != nil {
			def.SNMP = &SNMPParams{
				Target:    d.SNMP.Target,
				Port:      d.SNMP.Port,
				Version:   d.SNMP.Version,
				Operation: d.SNMP.Operation,
				OIDs:      append([]string(nil), d.SNMP.OIDs...),
				TimeoutMs: d.SNMP.TimeoutMs,
				Limits: SNMPLimits{
					MaxOIDs:            d.SNMP.Limits.MaxOIDs,
					MaxWalkEntries:     d.SNMP.Limits.MaxWalkEntries,
					MaxBulkRepetitions: d.SNMP.Limits.MaxBulkRepetitions,
					MaxResults:         d.SNMP.Limits.MaxResults,
					MaxResultBytes:     d.SNMP.Limits.MaxResultBytes,
				},
			}
		}
		out = append(out, def)
	}
	return out
}
