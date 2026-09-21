package recurring

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"platform/sensor/internal/protocol/snmp"
	"platform/sensor/internal/scheduler"
)

// JobSubmitter is the narrow scheduler surface used by the emitter.
type JobSubmitter interface {
	Submit(job scheduler.Job) (scheduler.Submission, error)
}

// CredentialResolver materializes governed credential profile secrets at submit time.
type CredentialResolver interface {
	Resolve(tenantID, profileRef string) (snmp.Credentials, error)
}

// AppliedStateProvider supplies the currently applied recurring state.
type AppliedStateProvider interface {
	Applied() AppliedState
}

// Emitter evaluates due work and submits ordinary scheduler jobs.
type Emitter struct {
	provider  AppliedStateProvider
	engine    *DueEngine
	submitter JobSubmitter
	resolver  CredentialResolver
	metrics   *Metrics
	leaseTTL  time.Duration
	mu        sync.Mutex
}

// EmitterConfig configures the recurring emitter.
type EmitterConfig struct {
	LeaseTTL time.Duration
	Metrics  *Metrics
}

// NewEmitter constructs an emitter. leaseTTL defaults to 30s when unset.
func NewEmitter(provider AppliedStateProvider, engine *DueEngine, submitter JobSubmitter, resolver CredentialResolver, cfg EmitterConfig) (*Emitter, error) {
	if provider == nil || engine == nil || submitter == nil || resolver == nil {
		return nil, fmt.Errorf("%s: provider, engine, submitter and resolver are required", ErrCodeInvalidConfig)
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 30 * time.Second
	}
	if cfg.Metrics == nil {
		cfg.Metrics = &Metrics{}
	}
	return &Emitter{
		provider:  provider,
		engine:    engine,
		submitter: submitter,
		resolver:  resolver,
		metrics:   cfg.Metrics,
		leaseTTL:  cfg.LeaseTTL,
	}, nil
}

// Tick evaluates applied definitions and submits due SNMP work.
func (e *Emitter) Tick(ctx context.Context, now time.Time) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	applied := e.provider.Applied()
	e.metrics.SetAppliedGeneration(applied.Generation)
	e.metrics.SetActiveDefinitions(int64(len(applied.Definitions)))

	due := e.engine.Evaluate(now, applied.Definitions)
	e.metrics.AddDueWork(uint64(len(due)))

	for _, work := range due {
		if work.MissedSkipped > 0 {
			e.metrics.AddSkippedCoalesced(uint64(work.MissedSkipped))
		}
		if work.FreshnessDebt {
			e.metrics.AddOverdueFreshnessDebt(1)
		}
		job, err := e.buildJob(work, applied.Generation, now)
		if err != nil {
			continue
		}
		_, err = e.submitter.Submit(job)
		if err != nil {
			if scheduler.IsCode(err, scheduler.CodeQueueFull) || scheduler.IsCode(err, scheduler.CodeTrackingCapacity) {
				e.metrics.AddQueueAdmissionRejected(1)
				continue
			}
			continue
		}
		timeout := time.Duration(work.Definition.ExecutionTimeoutMs) * time.Millisecond
		inFlightUntil := now.Add(timeout)
		if inFlightUntil.Before(now.Add(e.leaseTTL)) {
			inFlightUntil = now.Add(e.leaseTTL)
		}
		e.engine.MarkSubmitted(work.Definition.AssignmentID, work.CurrentDue, now, inFlightUntil)
		e.metrics.AddSubmitted(1)
	}
	return nil
}

func (e *Emitter) buildJob(work DueWork, generation int64, now time.Time) (scheduler.Job, error) {
	def := work.Definition
	if def.Protocol != ProtocolSNMP || def.SNMP == nil {
		return scheduler.Job{}, fmt.Errorf("%s: only SNMP definitions are supported in Slice A", ErrCodeInvalidDesired)
	}
	creds, err := e.resolver.Resolve(def.TenantID, def.CredentialProfileRef)
	if err != nil {
		return scheduler.Job{}, err
	}
	req, err := buildSNMPRequest(def, creds)
	if err != nil {
		return scheduler.Job{}, err
	}
	payload, err := snmp.EncodeSchedulerPayload(req)
	if err != nil {
		return scheduler.Job{}, err
	}

	class := scheduler.ExecutionClass(def.ExecutionClass)
	if class == "" {
		class = scheduler.ClassNetworkPoll
	}
	timeout := time.Duration(def.ExecutionTimeoutMs) * time.Millisecond
	jobID := def.AssignmentID + "@" + strconv.FormatInt(work.CurrentDue.UnixNano(), 10)
	return scheduler.Job{
		JobID:             scheduler.JobID(jobID),
		AssignmentKey:     scheduler.AssignmentKey(def.AssignmentID),
		TenantID:          scheduler.TenantID(def.TenantID),
		EntityID:          scheduler.EntityID(def.EntityID),
		Kind:              snmp.SchedulerJobKind,
		Class:             class,
		AssignmentVersion: uint64(generation),
		LeaseExpiresAt:    now.Add(e.leaseTTL),
		Timeout:           timeout,
		Payload:           payload,
	}, nil
}

func buildSNMPRequest(def RecurringDefinition, creds snmp.Credentials) (snmp.Request, error) {
	p := def.SNMP
	op := snmp.Operation(strings.ToUpper(p.Operation))
	req := snmp.Request{
		Target:           p.Target,
		Port:             p.Port,
		Version:          snmp.Version(p.Version),
		Credentials:      creds,
		OIDs:             append([]string(nil), p.OIDs...),
		Operation:        op,
		Timeout:          time.Duration(p.TimeoutMs) * time.Millisecond,
		ExecutionTimeout: time.Duration(def.ExecutionTimeoutMs) * time.Millisecond,
		Retry: snmp.RetryPolicy{
			MaxAttempts:    1,
			InitialBackoff: 0,
			MaxBackoff:     0,
			JitterFraction: 0,
		},
		Limits: snmp.Limits{
			MaxOIDs:            p.Limits.MaxOIDs,
			MaxWalkEntries:     p.Limits.MaxWalkEntries,
			MaxBulkRepetitions: p.Limits.MaxBulkRepetitions,
			MaxResults:         p.Limits.MaxResults,
			MaxResultBytes:     p.Limits.MaxResultBytes,
		},
		CorrelationID: def.AssignmentID,
	}
	return req, nil
}
