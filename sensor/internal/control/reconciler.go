package control

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"platform/sensor/internal/config"
	"platform/sensor/internal/coreclient"
)

// ControlClient is the Core transport surface required by the reconciler.
type ControlClient interface {
	FetchControlDesiredState(ctx context.Context) (*coreclient.ControlDesiredState, error)
	AckControlAppliedState(ctx context.Context, req coreclient.ControlAppliedAckRequest) (coreclient.ControlAppliedAckResponse, error)
}

// Result describes one RunOnce outcome without secret-bearing payloads.
type Result struct {
	Action     string
	Generation uint64
	Checksum   string
	Outcome    string
	Code       string
}

// SnapshotStore loads and commits durable APPLIED control snapshots.
type SnapshotStore interface {
	Load() (*AppliedSnapshot, error)
	Commit(snap AppliedSnapshot) error
}

// Reconciler runs bounded synchronous desired-state reconciliation.
type Reconciler struct {
	client   ControlClient
	store    SnapshotStore
	baseline config.RuntimeConfig
	applier  Applier
	logger   *slog.Logger
	metrics  *Metrics

	mu sync.Mutex
}

// NewReconciler constructs a Slice 3 reconciler. Production main wiring is deferred to Slice 4.
func NewReconciler(
	client ControlClient,
	store SnapshotStore,
	baseline config.RuntimeConfig,
	applier Applier,
	logger *slog.Logger,
	metrics *Metrics,
) (*Reconciler, error) {
	if client == nil {
		return nil, newCodedError(CodeInvalidDesired, "control client is required", nil)
	}
	if store == nil {
		return nil, newCodedError(CodeInvalidDesired, "control store is required", nil)
	}
	if applier == nil {
		return nil, newCodedError(CodeInvalidDesired, "control applier is required", nil)
	}
	if logger == nil {
		logger = slog.Default()
	}
	if metrics == nil {
		metrics = &Metrics{}
	}
	if err := baseline.Validate(); err != nil {
		return nil, newCodedError(CodeConfigInvalid, "baseline RuntimeConfig is invalid", err)
	}
	return &Reconciler{
		client:   client,
		store:    store,
		baseline: baseline,
		applier:  applier,
		logger:   logger,
		metrics:  metrics,
	}, nil
}

// MetricsSnapshot returns current reconciler metrics.
func (r *Reconciler) MetricsSnapshot() MetricsSnapshot {
	if r == nil || r.metrics == nil {
		return MetricsSnapshot{}
	}
	return r.metrics.Snapshot()
}

// RunOnce fetches Core desired state and reconciles once. Concurrent calls are serialized.
func (r *Reconciler) RunOnce(ctx context.Context) (Result, error) {
	if r == nil {
		return Result{}, newCodedError(CodeInvalidDesired, "reconciler is nil", nil)
	}
	if ctx == nil {
		return Result{}, newCodedError(CodeInvalidDesired, "context must not be nil", nil)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics.markAttempt()

	desired, err := r.client.FetchControlDesiredState(ctx)
	if err != nil {
		code := mapTransportCode(err)
		if code == CodeInvalidDesired {
			r.logEvent("control_reconcile_invalid_desired", 0, "", code)
			return Result{Action: "INVALID_DESIRED", Code: code}, newCodedError(code, "Core control desired-state is invalid", err)
		}
		r.metrics.markTransportFailure()
		r.logEvent("control_reconcile_core_unavailable", 0, "", code)
		return Result{Action: "CORE_UNAVAILABLE", Code: code}, newCodedError(code, "Core control desired-state unavailable", err)
	}
	if desired == nil {
		r.metrics.markNoDesired()
		r.logEvent("control_reconcile_no_desired", 0, "", "")
		return Result{Action: "NO_DESIRED"}, nil
	}

	local, err := r.store.Load()
	if err != nil {
		r.metrics.markDurableFailure()
		return Result{Action: "STATE_CORRUPT", Code: CodeStateCorrupt}, err
	}

	if desired.SchemaVersion != SchemaVersionV1 {
		return r.rejectUnsupportedOrInvalid(ctx, desired, CodeUnsupportedSchema, "unsupported schemaVersion")
	}

	computed, err := ComputeDesiredChecksum(toControlConfig(desired.Configuration))
	if err != nil {
		return Result{Action: "INVALID_DESIRED", Code: CodeInvalidDesired}, newCodedError(CodeInvalidDesired, "failed to compute desired checksum", err)
	}
	if computed != desired.DesiredChecksum {
		r.metrics.markChecksumMismatch()
		r.metrics.markRejected()
		ackErr := r.sendRejected(ctx, desired.DesiredGeneration, desired.DesiredChecksum, CodeChecksumMismatch, "semantic checksum mismatch")
		r.logEvent("control_reconcile_checksum_mismatch", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), CodeChecksumMismatch)
		if ackErr != nil {
			r.metrics.markAckFailure()
			return Result{
				Action: "CHECKSUM_MISMATCH", Generation: desired.DesiredGeneration,
				Checksum: desired.DesiredChecksum, Outcome: OutcomeRejected, Code: CodeChecksumMismatch,
			}, newCodedError(CodeAckFailed, "REJECTED ACK failed after checksum mismatch", ackErr)
		}
		return Result{
			Action: "CHECKSUM_MISMATCH", Generation: desired.DesiredGeneration,
			Checksum: desired.DesiredChecksum, Outcome: OutcomeRejected, Code: CodeChecksumMismatch,
		}, nil
	}

	localGen := uint64(0)
	localChecksum := ""
	if local != nil {
		localGen = local.Generation
		localChecksum = local.Checksum
	}

	if local != nil && desired.DesiredGeneration < localGen {
		r.metrics.markRegression()
		r.logEvent("control_reconcile_core_regression", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), CodeCoreRegression)
		return Result{
			Action: "CORE_REGRESSION", Generation: desired.DesiredGeneration,
			Checksum: desired.DesiredChecksum, Code: CodeCoreRegression,
		}, newCodedError(CodeCoreRegression, "Core desired generation regresses local applied generation", nil)
	}

	if local != nil && desired.DesiredGeneration == localGen {
		if desired.DesiredChecksum == localChecksum {
			candidate, overlayErr := OverlayDesired(r.baseline, local.Configuration)
			if overlayErr != nil {
				r.metrics.markRejected()
				rejectCode := CodeConfigInvalid
				ackErr := r.sendRejected(ctx, desired.DesiredGeneration, desired.DesiredChecksum, rejectCode, "lost-ACK candidate configuration invalid")
				r.logEvent("control_reconcile_lost_ack_apply_failed", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), rejectCode)
				if ackErr != nil {
					r.metrics.markAckFailure()
					return Result{
						Action: "APPLY_FAILED", Generation: desired.DesiredGeneration,
						Checksum: desired.DesiredChecksum, Outcome: OutcomeRejected, Code: rejectCode,
					}, newCodedError(CodeAckFailed, "REJECTED ACK failed after lost-ACK overlay failure", ackErr)
				}
				return Result{
					Action: "APPLY_FAILED", Generation: desired.DesiredGeneration,
					Checksum: desired.DesiredChecksum, Outcome: OutcomeRejected, Code: rejectCode,
				}, nil
			}
			if err := r.applier.Apply(ctx, desired.DesiredGeneration, desired.DesiredChecksum, candidate); err != nil {
				r.metrics.markRejected()
				rejectCode := RejectCodeForApply(err)
				ackErr := r.sendRejected(ctx, desired.DesiredGeneration, desired.DesiredChecksum, rejectCode, "lost-ACK runtime re-proof failed")
				r.logEvent("control_reconcile_lost_ack_apply_failed", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), rejectCode)
				if ackErr != nil {
					r.metrics.markAckFailure()
					return Result{
						Action: "APPLY_FAILED", Generation: desired.DesiredGeneration,
						Checksum: desired.DesiredChecksum, Outcome: OutcomeRejected, Code: rejectCode,
					}, newCodedError(CodeAckFailed, "REJECTED ACK failed after lost-ACK apply failure", ackErr)
				}
				return Result{
					Action: "APPLY_FAILED", Generation: desired.DesiredGeneration,
					Checksum: desired.DesiredChecksum, Outcome: OutcomeRejected, Code: rejectCode,
				}, nil
			}
			if err := r.store.Commit(*local); err != nil {
				r.metrics.markDurableFailure()
				r.logEvent("control_reconcile_durability_failed", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), CodeStateDurabilityFailed)
				if CodeOf(err) == "" {
					err = newCodedError(CodeStateDurabilityFailed, "durable store commit failed", err)
				}
				return Result{
					Action:     "DURABILITY_FAILED",
					Generation: desired.DesiredGeneration,
					Checksum:   desired.DesiredChecksum,
					Code:       CodeStateDurabilityFailed,
				}, err
			}
			ackErr := r.sendApplied(ctx, desired.DesiredGeneration, desired.DesiredChecksum)
			if ackErr != nil {
				r.metrics.markAckFailure()
				r.logEvent("control_reconcile_lost_ack_failed", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), CodeAckFailed)
				return Result{
					Action: "LOST_ACK_REPAIR", Generation: desired.DesiredGeneration,
					Checksum: desired.DesiredChecksum, Outcome: OutcomeApplied, Code: CodeAckFailed,
				}, newCodedError(CodeAckFailed, "lost-ACK repair failed", ackErr)
			}
			r.metrics.markLostAck()
			r.logEvent("control_reconcile_lost_ack_repaired", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), "")
			return Result{
				Action: "LOST_ACK_REPAIR", Generation: desired.DesiredGeneration,
				Checksum: desired.DesiredChecksum, Outcome: OutcomeApplied,
			}, nil
		}
		r.metrics.markConflict()
		r.logEvent("control_reconcile_generation_conflict", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), CodeGenerationConflict)
		return Result{
			Action: "GENERATION_CONFLICT", Generation: desired.DesiredGeneration,
			Checksum: desired.DesiredChecksum, Code: CodeGenerationConflict,
		}, newCodedError(CodeGenerationConflict, "same generation with different checksum", nil)
	}

	// First desired or newer generation: apply path.
	candidate, err := OverlayDesired(r.baseline, toControlConfig(desired.Configuration))
	if err != nil {
		r.metrics.markRejected()
		ackErr := r.sendRejected(ctx, desired.DesiredGeneration, desired.DesiredChecksum, CodeConfigInvalid, "candidate configuration invalid")
		r.logEvent("control_reconcile_config_invalid", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), CodeConfigInvalid)
		if ackErr != nil {
			r.metrics.markAckFailure()
			return Result{
				Action: "CONFIG_INVALID", Generation: desired.DesiredGeneration,
				Checksum: desired.DesiredChecksum, Outcome: OutcomeRejected, Code: CodeConfigInvalid,
			}, newCodedError(CodeAckFailed, "REJECTED ACK failed after config invalid", ackErr)
		}
		return Result{
			Action: "CONFIG_INVALID", Generation: desired.DesiredGeneration,
			Checksum: desired.DesiredChecksum, Outcome: OutcomeRejected, Code: CodeConfigInvalid,
		}, nil
	}

	if err := r.applier.Apply(ctx, desired.DesiredGeneration, desired.DesiredChecksum, candidate); err != nil {
		r.metrics.markRejected()
		rejectCode := RejectCodeForApply(err)
		ackErr := r.sendRejected(ctx, desired.DesiredGeneration, desired.DesiredChecksum, rejectCode, "runtime apply failed")
		r.logEvent("control_reconcile_apply_failed", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), rejectCode)
		if ackErr != nil {
			r.metrics.markAckFailure()
			return Result{
				Action: "APPLY_FAILED", Generation: desired.DesiredGeneration,
				Checksum: desired.DesiredChecksum, Outcome: OutcomeRejected, Code: rejectCode,
			}, newCodedError(CodeAckFailed, "REJECTED ACK failed after apply failure", ackErr)
		}
		return Result{
			Action: "APPLY_FAILED", Generation: desired.DesiredGeneration,
			Checksum: desired.DesiredChecksum, Outcome: OutcomeRejected, Code: rejectCode,
		}, nil
	}

	snap := AppliedSnapshot{
		SchemaVersion: storeSchemaVersion,
		Generation:    desired.DesiredGeneration,
		Checksum:      desired.DesiredChecksum,
		Configuration: toControlConfig(desired.Configuration),
		AppliedAt:     time.Now().UTC(),
	}
	if err := r.store.Commit(snap); err != nil {
		r.metrics.markDurableFailure()
		r.logEvent("control_reconcile_durability_failed", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), CodeStateDurabilityFailed)
		return Result{
			Action: "DURABILITY_FAILED", Generation: desired.DesiredGeneration,
			Checksum: desired.DesiredChecksum, Code: CodeStateDurabilityFailed,
		}, err
	}

	if err := r.sendApplied(ctx, desired.DesiredGeneration, desired.DesiredChecksum); err != nil {
		r.metrics.markAckFailure()
		r.logEvent("control_reconcile_ack_failed", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), CodeAckFailed)
		return Result{
			Action: "ACK_FAILED", Generation: desired.DesiredGeneration,
			Checksum: desired.DesiredChecksum, Outcome: OutcomeApplied, Code: CodeAckFailed,
		}, newCodedError(CodeAckFailed, "APPLIED ACK failed after durable commit", err)
	}

	r.metrics.markApplied(desired.DesiredGeneration)
	r.logEvent("control_reconcile_applied", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), "")
	return Result{
		Action: "APPLIED", Generation: desired.DesiredGeneration,
		Checksum: desired.DesiredChecksum, Outcome: OutcomeApplied,
	}, nil
}

func (r *Reconciler) rejectUnsupportedOrInvalid(ctx context.Context, desired *coreclient.ControlDesiredState, code, message string) (Result, error) {
	r.metrics.markRejected()
	ackErr := r.sendRejected(ctx, desired.DesiredGeneration, desired.DesiredChecksum, code, message)
	r.logEvent("control_reconcile_unsupported_schema", desired.DesiredGeneration, checksumPrefix(desired.DesiredChecksum), code)
	if ackErr != nil {
		r.metrics.markAckFailure()
		return Result{
			Action: "UNSUPPORTED_SCHEMA", Generation: desired.DesiredGeneration,
			Checksum: desired.DesiredChecksum, Outcome: OutcomeRejected, Code: code,
		}, newCodedError(CodeAckFailed, "REJECTED ACK failed after unsupported schema", ackErr)
	}
	return Result{
		Action: "UNSUPPORTED_SCHEMA", Generation: desired.DesiredGeneration,
		Checksum: desired.DesiredChecksum, Outcome: OutcomeRejected, Code: code,
	}, nil
}

func (r *Reconciler) sendApplied(ctx context.Context, generation uint64, checksum string) error {
	_, err := r.client.AckControlAppliedState(ctx, coreclient.ControlAppliedAckRequest{
		SchemaVersion: SchemaVersionV1,
		Generation:    generation,
		Checksum:      checksum,
		Outcome:       OutcomeApplied,
	})
	return err
}

func (r *Reconciler) sendRejected(ctx context.Context, generation uint64, checksum, code, message string) error {
	if len(message) > MaxRejectionMessageLen {
		message = message[:MaxRejectionMessageLen]
	}
	_, err := r.client.AckControlAppliedState(ctx, coreclient.ControlAppliedAckRequest{
		SchemaVersion:    SchemaVersionV1,
		Generation:       generation,
		Checksum:         checksum,
		Outcome:          OutcomeRejected,
		RejectionCode:    code,
		RejectionMessage: message,
	})
	return err
}

func (r *Reconciler) logEvent(event string, generation uint64, checksumPrefixValue, code string) {
	attrs := []any{
		"event", event,
		"generation", generation,
	}
	if checksumPrefixValue != "" {
		attrs = append(attrs, "checksum_prefix", checksumPrefixValue)
	}
	if code != "" {
		attrs = append(attrs, "code", code)
	}
	r.logger.Info("sensor_control_reconcile", attrs...)
}

func toControlConfig(in coreclient.ControlDesiredConfiguration) DesiredConfiguration {
	pools := make(map[string]DesiredPool, len(in.Scheduler.Pools))
	for name, pool := range in.Scheduler.Pools {
		pools[name] = DesiredPool{Workers: pool.Workers, QueueDepth: pool.QueueDepth}
	}
	return DesiredConfiguration{
		LogLevel: in.LogLevel,
		Scheduler: DesiredScheduler{
			MaxWorkers:                in.Scheduler.MaxWorkers,
			MaxQueueDepth:             in.Scheduler.MaxQueueDepth,
			MaxTrackedAssignments:     in.Scheduler.MaxTrackedAssignments,
			MaxTrackedJobs:            in.Scheduler.MaxTrackedJobs,
			MaxPayloadBytes:           in.Scheduler.MaxPayloadBytes,
			DefaultExecutionTimeoutMs: in.Scheduler.DefaultExecutionTimeoutMs,
			ShutdownGracePeriodMs:     in.Scheduler.ShutdownGracePeriodMs,
			RetryMaxAttempts:          in.Scheduler.RetryMaxAttempts,
			RetryInitialBackoffMs:     in.Scheduler.RetryInitialBackoffMs,
			RetryMaxBackoffMs:         in.Scheduler.RetryMaxBackoffMs,
			RetryJitterBasisPoints:    in.Scheduler.RetryJitterBasisPoints,
			Pools:                     pools,
		},
		Transport: DesiredTransport{
			RequestTimeoutMs:   in.Transport.RequestTimeoutMs,
			MaxRequestBytes:    in.Transport.MaxRequestBytes,
			MaxResponseBytes:   in.Transport.MaxResponseBytes,
			UploadBatchRecords: in.Transport.UploadBatchRecords,
			UploadBatchBytes:   in.Transport.UploadBatchBytes,
		},
	}
}

func checksumPrefix(checksum string) string {
	if len(checksum) < 12 {
		return checksum
	}
	return checksum[:12]
}

func mapTransportCode(err error) string {
	if err == nil {
		return CodeCoreUnavailable
	}
	if CodeOf(err) == CodeInvalidDesired {
		return CodeInvalidDesired
	}
	if coreclient.IsCode(err, coreclient.CodeControlProtocol) ||
		coreclient.IsCode(err, coreclient.CodeProtocol) ||
		coreclient.IsCode(err, coreclient.CodeResponseTooLarge) {
		return CodeInvalidDesired
	}
	return CodeCoreUnavailable
}
