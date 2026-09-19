package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"platform/sensor/internal/packsync"
	"platform/sensor/internal/spool"
)

type HealthServer struct {
	server     *http.Server
	spool      *spool.DiskSpool
	agentSpool *spool.DiskSpool
	packSync   *packsync.Syncer
	mu         sync.RWMutex
	logger     *slog.Logger
}

func NewHealthServer(port int, diskSpool *spool.DiskSpool, logger *slog.Logger) *HealthServer {
	mux := http.NewServeMux()
	hs := &HealthServer{
		server: &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
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

// SetAgentSpool optionally wires the dedicated Agent ingress spool into readiness checks.
func (hs *HealthServer) SetAgentSpool(agentSpool *spool.DiskSpool) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.agentSpool = agentSpool
}

// SetPackSync optionally wires Capability Pack sync status (no secrets).
func (hs *HealthServer) SetPackSync(syncer *packsync.Syncer) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.packSync = syncer
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
			// Fail closed: trusted pack state could not be established (e.g. corrupt recovery).
			httpStatus = http.StatusServiceUnavailable
			if status == "UP" || status == "DEGRADED" {
				status = "PACK_TRUST_NOT_ESTABLISHED"
			}
		} else if !snap.CoreAuthorityEstablished {
			// Local empty/structurally clean state is not Core-confirmed authority.
			httpStatus = http.StatusServiceUnavailable
			if status == "UP" || status == "DEGRADED" {
				status = "PACK_CORE_AUTHORITY_NOT_ESTABLISHED"
			}
		} else if snap.Degraded && status == "UP" {
			// Auth/degraded with retained trusted applied + Core authority stays ready (HTTP 200).
			status = "DEGRADED"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":                status,
		"storage_percent":       fmt.Sprintf("%.2f%%", storagePercent),
		"agent_storage_percent": fmt.Sprintf("%.2f%%", agentStorage),
		"capability_pack_sync":  packStatus,
		"timestamp":             time.Now().UTC().Format(time.RFC3339),
	})
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
