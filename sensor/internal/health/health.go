package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"platform/sensor/internal/control"
	"platform/sensor/internal/packsync"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
)

type packSyncStatusProvider interface {
	Snapshot() packsync.Status
}

type schedulerStatusProvider interface {
	Snapshot() scheduler.Snapshot
}

type controlStatusProvider interface {
	Snapshot() control.Status
}

type HealthServer struct {
	server     *http.Server
	spool      *spool.DiskSpool
	agentSpool *spool.DiskSpool
	packSync   packSyncStatusProvider
	scheduler  schedulerStatusProvider
	control    controlStatusProvider
	mu         sync.RWMutex
	logger     *slog.Logger
}

func NewHealthServer(addr string, diskSpool *spool.DiskSpool, logger *slog.Logger) *HealthServer {
	if strings.TrimSpace(addr) == "" {
		addr = ":0"
	}
	mux := http.NewServeMux()
	hs := &HealthServer{
		server: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		spool:  diskSpool,
		logger: logger,
	}

	mux.HandleFunc("/health", hs.handleHealth)
	mux.HandleFunc("/healthz", hs.handleHealth)
	mux.HandleFunc("/readyz", hs.handleReady)
	mux.HandleFunc("/livez", hs.handleLive)

	return hs
}

// ListenAddr returns the configured HTTP bind address (host:port or :port).
func (hs *HealthServer) ListenAddr() string {
	if hs == nil || hs.server == nil {
		return ""
	}
	return hs.server.Addr
}

// SetAgentSpool optionally wires the dedicated Agent ingress spool into readiness checks.
func (hs *HealthServer) SetAgentSpool(agentSpool *spool.DiskSpool) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.agentSpool = agentSpool
}

// SetPackSync optionally wires Capability Pack sync status (no secrets).
func (hs *HealthServer) SetPackSync(syncer packSyncStatusProvider) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if syncer == nil || syncer == (*packsync.Syncer)(nil) {
		hs.packSync = nil
		return
	}
	hs.packSync = syncer
}

// SetScheduler attaches the production scheduler status provider used by
// /health and /readyz. Nil clears the provider.
func (hs *HealthServer) SetScheduler(provider schedulerStatusProvider) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.scheduler = provider
}

// SetControl attaches the production control reconciler status provider.
func (hs *HealthServer) SetControl(provider controlStatusProvider) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.control = provider
}

func (hs *HealthServer) handleLive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "UP",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (hs *HealthServer) handleReady(w http.ResponseWriter, r *http.Request) {
	hs.handleHealth(w, r)
}

func (hs *HealthServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	storagePercent, err := hs.spool.CheckStorageUsage()
	status := "UP"
	httpStatus := http.StatusOK

	if err != nil {
		hs.logger.Warn("Failed checking disk storage in health probe", "error", err)
		status = "DEGRADED"
		httpStatus = http.StatusServiceUnavailable
	} else if storagePercent >= 95.0 {
		status = "OUT_OF_DISK"
		httpStatus = http.StatusServiceUnavailable
	}

	hs.mu.RLock()
	agentSpool := hs.agentSpool
	packSync := hs.packSync
	schedulerProvider := hs.scheduler
	controlProvider := hs.control
	hs.mu.RUnlock()
	agentStorage := 0.0
	if agentSpool != nil {
		agentPct, agentErr := agentSpool.CheckStorageUsage()
		agentStorage = agentPct
		if agentErr != nil {
			hs.logger.Warn("Failed checking agent spool storage in health probe", "error", agentErr)
			status = "AGENT_SPOOL_UNWRITABLE"
			httpStatus = http.StatusServiceUnavailable
		} else if agentPct >= 95.0 {
			status = "AGENT_SPOOL_HARD_PRESSURE"
			httpStatus = http.StatusServiceUnavailable
		}
	}

	packStatus := map[string]any{}
	if packSync != nil {
		snap := packSync.Snapshot()
		packStatus = map[string]any{
			"desired_generation":         snap.DesiredGeneration,
			"applied_generation":         snap.AppliedGeneration,
			"pack_count":                 snap.PackCount,
			"last_successful_sync":       formatTime(snap.LastSuccessfulSync),
			"last_error_code":            snap.LastErrorCode,
			"degraded":                   snap.Degraded,
			"trusted_applied":            snap.TrustedApplied,
			"core_authority_established": snap.CoreAuthorityEstablished,
		}
		if !snap.TrustedApplied {
			httpStatus = http.StatusServiceUnavailable
			if status == "UP" || status == "DEGRADED" {
				status = "PACK_TRUST_NOT_ESTABLISHED"
			}
		} else if !snap.CoreAuthorityEstablished {
			httpStatus = http.StatusServiceUnavailable
			if status == "UP" || status == "DEGRADED" {
				status = "PACK_CORE_AUTHORITY_NOT_ESTABLISHED"
			}
		} else if snap.Degraded && status == "UP" {
			status = "DEGRADED"
		}
	}

	schedulerStatus := map[string]any{}
	if schedulerProvider == nil {
		httpStatus = http.StatusServiceUnavailable
		if status == "UP" || status == "DEGRADED" {
			status = "SCHEDULER_UNAVAILABLE"
		}
	} else {
		snap := schedulerProvider.Snapshot()
		schedulerStatus = schedulerPayload(snap)
		if snap.State != scheduler.StateRunning {
			httpStatus = http.StatusServiceUnavailable
			if status == "UP" || status == "DEGRADED" {
				status = "SCHEDULER_NOT_RUNNING"
			}
		}
	}

	controlStatus := map[string]any{}
	if controlProvider != nil {
		snap := controlProvider.Snapshot()
		controlStatus = controlPayload(snap)
		switch snap.State {
		case control.StateUntrusted:
			httpStatus = http.StatusServiceUnavailable
			if status == "UP" || status == "DEGRADED" {
				status = "CONTROL_STATE_UNTRUSTED"
			}
		case control.StateSyncPending, control.StateDegraded:
			if status == "UP" {
				status = "DEGRADED"
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":                status,
		"storage_percent":       fmt.Sprintf("%.2f%%", storagePercent),
		"agent_storage_percent": fmt.Sprintf("%.2f%%", agentStorage),
		"capability_pack_sync":  packStatus,
		"scheduler":             schedulerStatus,
		"control":               controlStatus,
		"timestamp":             time.Now().UTC().Format(time.RFC3339),
	})
}

func schedulerPayload(snap scheduler.Snapshot) map[string]any {
	return map[string]any{
		"state":                            snap.State,
		"configured_workers":               snap.ConfiguredWorkers,
		"active_workers":                   snap.ActiveWorkers,
		"queue_depth":                      snap.QueueDepth,
		"queue_capacity":                   snap.QueueCapacity,
		"tracked_assignments":              snap.TrackedAssignments,
		"tracked_jobs":                     snap.TrackedJobs,
		"highest_observed_fencing_version": snap.HighestObservedFencingVersion,
		"highest_accepted_fencing_version": snap.HighestAcceptedFencingVersion,
		"submitted":                        snap.Submitted,
		"accepted":                         snap.Accepted,
		"completed":                        snap.Completed,
		"failed":                           snap.Failed,
		"retries":                          snap.Retries,
		"rejected_queue_full":              snap.RejectedQueueFull,
		"rejected_stale_lease":             snap.RejectedStaleLease,
		"rejected_tracking_capacity":       snap.RejectedTrackingCapacity,
		"expired":                          snap.Expired,
		"cancelled":                        snap.Cancelled,
		"deadline_exceeded":                snap.DeadlineExceeded,
	}
}

func controlPayload(snap control.Status) map[string]any {
	return map[string]any{
		"state":                     snap.State,
		"last_action":               snap.LastAction,
		"last_code":                 snap.LastCode,
		"last_generation":           snap.LastGeneration,
		"last_successful_reconcile": formatTime(snap.LastSuccessfulReconcile),
		"reconcile_attempts":        snap.ReconcileAttempts,
		"no_desired_count":          snap.NoDesiredCount,
		"applied_count":             snap.AppliedCount,
		"rejected_count":            snap.RejectedCount,
		"lost_ack_repair_count":     snap.LostAckRepairCount,
		"core_regression_count":     snap.CoreRegressionCount,
		"generation_conflict_count": snap.GenerationConflictCount,
		"checksum_mismatch_count":   snap.ChecksumMismatchCount,
		"core_transport_failures":   snap.CoreTransportFailures,
		"durable_store_failures":    snap.DurableStoreFailures,
		"ack_failures":              snap.AckFailures,
		"last_applied_generation":   snap.LastAppliedGeneration,
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (hs *HealthServer) Start() error {
	hs.logger.Info("Starting sensor health server", "addr", hs.server.Addr)
	if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (hs *HealthServer) Shutdown(ctx context.Context) error {
	return hs.server.Shutdown(ctx)
}
