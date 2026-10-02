package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"platform/sensor/internal/control"
	"platform/sensor/internal/packsync"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
)

const (
	componentName         = "platform-sensor"
	defaultVersion        = "0.1.0-dev"
	hardStoragePercent    = 80.0
	reasonShuttingDown    = "SENSOR_SHUTTING_DOWN"
	reasonSpoolCheck      = "SENSOR_SPOOL_UNWRITABLE"
	reasonSpoolPressure   = "SENSOR_SPOOL_HARD_PRESSURE"
	reasonAgentSpoolCheck = "SENSOR_AGENT_SPOOL_UNWRITABLE"
	reasonAgentSpoolHard  = "SENSOR_AGENT_SPOOL_HARD_PRESSURE"
	reasonPackUnavailable = "SENSOR_PACK_SYNC_UNAVAILABLE"
	reasonPackTrust       = "SENSOR_PACK_TRUST_NOT_ESTABLISHED"
	reasonPackAuthority   = "SENSOR_PACK_CORE_AUTHORITY_NOT_ESTABLISHED"
	reasonPackDegraded    = "SENSOR_PACK_DEGRADED"
	reasonSchedulerAbsent = "SENSOR_SCHEDULER_UNAVAILABLE"
	reasonSchedulerState  = "SENSOR_SCHEDULER_NOT_RUNNING"
	reasonSchedulerQueue  = "SENSOR_SCHEDULER_QUEUE_SATURATED"
	reasonControlAbsent   = "SENSOR_CONTROL_UNAVAILABLE"
	reasonControlUntrust  = "SENSOR_CONTROL_STATE_UNTRUSTED"
	reasonControlDegraded = "SENSOR_CONTROL_DEGRADED"
)

type storageStatusProvider interface {
	CheckStorageUsage() (float64, error)
}

type packSyncStatusProvider interface {
	Snapshot() packsync.Status
}

type schedulerStatusProvider interface {
	Snapshot() scheduler.Snapshot
}

type controlStatusProvider interface {
	Snapshot() control.Status
}

type Option func(*HealthServer)

// WithIdentity binds health/supportability responses to the same immutable
// process identity and version as the structured logger.
func WithIdentity(version, serviceInstanceID string) Option {
	return func(hs *HealthServer) {
		if strings.TrimSpace(version) != "" {
			hs.version = version
		}
		if strings.TrimSpace(serviceInstanceID) != "" {
			hs.serviceInstanceID = serviceInstanceID
		}
	}
}

type HealthServer struct {
	server     *http.Server
	spool      storageStatusProvider
	agentSpool storageStatusProvider
	packSync   packSyncStatusProvider
	scheduler  schedulerStatusProvider
	control    controlStatusProvider
	mu         sync.RWMutex
	logger     *slog.Logger

	version           string
	serviceInstanceID string
	startedAt         time.Time
	shuttingDown      atomic.Bool
}

func NewHealthServer(addr string, diskSpool *spool.DiskSpool, logger *slog.Logger, opts ...Option) *HealthServer {
	if strings.TrimSpace(addr) == "" {
		addr = ":0"
	}
	if logger == nil {
		logger = slog.Default()
	}
	mux := http.NewServeMux()
	hs := &HealthServer{
		server: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		spool:             diskSpool,
		logger:            logger,
		version:           defaultVersion,
		serviceInstanceID: fmt.Sprintf("sensor-health-%d", time.Now().UTC().UnixNano()),
		startedAt:         time.Now().UTC(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(hs)
		}
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

// MarkShuttingDown changes only process/readiness semantics. It does not infer
// monitored-target failure and does not mutate scheduler/control authority.
func (hs *HealthServer) MarkShuttingDown() {
	if hs != nil {
		hs.shuttingDown.Store(true)
	}
}

func (hs *HealthServer) handleLive(w http.ResponseWriter, _ *http.Request) {
	status := "UP"
	if hs.shuttingDown.Load() {
		status = "SHUTTING_DOWN"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":              status,
		"component":           componentName,
		"ts":                  nowTimestamp(),
		"service_instance_id": hs.serviceInstanceID,
	})
}

type evaluation struct {
	status     string
	ready      bool
	httpStatus int
	reason     string

	storagePercent      float64
	storageKnown        bool
	agentStoragePercent float64
	agentStorageKnown   bool
	pack                 packsync.Status
	packPresent          bool
	scheduler            scheduler.Snapshot
	schedulerPresent     bool
	control              control.Status
	controlPresent       bool
}

func (e *evaluation) degrade(reason string) {
	if e.status == "UP" {
		e.status = "DEGRADED"
		e.reason = reason
	}
}

func (e *evaluation) notReady(reason string) {
	if e.status != "NOT_READY" {
		e.status = "NOT_READY"
		e.ready = false
		e.httpStatus = http.StatusServiceUnavailable
		e.reason = reason
	}
}

func (e *evaluation) degradedNotReady(reason string) {
	if e.status == "NOT_READY" {
		return
	}
	e.status = "DEGRADED"
	e.ready = false
	e.httpStatus = http.StatusServiceUnavailable
	e.reason = reason
}

func (hs *HealthServer) evaluate() evaluation {
	ev := evaluation{status: "UP", ready: true, httpStatus: http.StatusOK}
	if hs.shuttingDown.Load() {
		ev.notReady(reasonShuttingDown)
	}

	if hs.spool == nil {
		ev.notReady(reasonSpoolCheck)
	} else {
		pct, err := hs.spool.CheckStorageUsage()
		if err != nil {
			hs.logger.Warn("Sensor spool storage check failed",
				"error_code", "SENSOR_SPOOL_STORAGE_CHECK_FAILED",
				"safe_context", map[string]any{
					"reason_class": "storage_check_failed",
					"spool_type":   "telemetry",
				})
			ev.notReady(reasonSpoolCheck)
		} else {
			ev.storageKnown = true
			ev.storagePercent = pct
			if pct >= hardStoragePercent {
				ev.notReady(reasonSpoolPressure)
			}
		}
	}

	hs.mu.RLock()
	agentSpool := hs.agentSpool
	packSync := hs.packSync
	schedulerProvider := hs.scheduler
	controlProvider := hs.control
	hs.mu.RUnlock()

	if agentSpool != nil {
		pct, err := agentSpool.CheckStorageUsage()
		if err != nil {
			hs.logger.Warn("Agent ingress spool storage check failed",
				"error_code", "SENSOR_AGENT_SPOOL_STORAGE_CHECK_FAILED",
				"safe_context", map[string]any{
					"reason_class": "storage_check_failed",
					"spool_type":   "agent_ingress",
				})
			ev.notReady(reasonAgentSpoolCheck)
		} else {
			ev.agentStorageKnown = true
			ev.agentStoragePercent = pct
			if pct >= hardStoragePercent {
				ev.notReady(reasonAgentSpoolHard)
			}
		}
	}

	if packSync == nil {
		ev.notReady(reasonPackUnavailable)
	} else {
		ev.packPresent = true
		ev.pack = packSync.Snapshot()
		switch {
		case !ev.pack.TrustedApplied:
			ev.notReady(reasonPackTrust)
		case !ev.pack.CoreAuthorityEstablished:
			ev.notReady(reasonPackAuthority)
		case ev.pack.Degraded:
			ev.degrade(reasonPackDegraded)
		}
	}

	if schedulerProvider == nil {
		ev.notReady(reasonSchedulerAbsent)
	} else {
		ev.schedulerPresent = true
		ev.scheduler = schedulerProvider.Snapshot()
		if ev.scheduler.State != scheduler.StateRunning {
			ev.notReady(reasonSchedulerState)
		} else if schedulerQueueSaturated(ev.scheduler) {
			ev.degradedNotReady(reasonSchedulerQueue)
		}
	}

	if controlProvider == nil {
		ev.notReady(reasonControlAbsent)
	} else {
		ev.controlPresent = true
		ev.control = controlProvider.Snapshot()
		switch ev.control.State {
		case control.StateUntrusted:
			ev.notReady(reasonControlUntrust)
		case control.StateSyncPending, control.StateDegraded:
			ev.degrade(reasonControlDegraded)
		case control.StateInSync:
			// healthy
		default:
			ev.degrade(reasonControlDegraded)
		}
	}

	return ev
}

func (hs *HealthServer) handleReady(w http.ResponseWriter, _ *http.Request) {
	ev := hs.evaluate()
	body := map[string]any{
		"status":              ev.status,
		"ready":               ev.ready,
		"http_semantic":       ev.httpStatus,
		"component":           componentName,
		"ts":                  nowTimestamp(),
		"service_instance_id": hs.serviceInstanceID,
	}
	if ev.reason != "" {
		body["reason_code"] = ev.reason
	}
	writeJSON(w, ev.httpStatus, body)
}

func (hs *HealthServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	ev := hs.evaluate()
	state := ev.status
	if hs.shuttingDown.Load() {
		state = "SHUTTING_DOWN"
	}
	body := map[string]any{
		"document_type":       "SUPPORT_EVIDENCE",
		"component":           componentName,
		"version":             hs.version,
		"service_instance_id": hs.serviceInstanceID,
		"ts":                  nowTimestamp(),
		"uptime_seconds":      uptimeSeconds(hs.startedAt),
		"component_state":     state,
		"dependency_status":   dependencyStatus(ev),
	}

	if ev.storageKnown {
		body["storage_pressure_percent"] = percentInt(ev.storagePercent)
		body["spool_pressure_percent"] = percentInt(ev.storagePercent)
	}
	if ev.agentStorageKnown {
		agentPct := percentInt(ev.agentStoragePercent)
		if current, ok := body["spool_pressure_percent"].(int); !ok || agentPct > current {
			body["spool_pressure_percent"] = agentPct
		}
	}
	if ev.schedulerPresent {
		body["queue_depth"] = maxZeroInt(ev.scheduler.QueueDepth)
		body["queue_capacity"] = maxZeroInt(ev.scheduler.QueueCapacity)
		body["scheduler_status"] = schedulerSupportStatus(ev.scheduler.State)
	}
	if ev.controlPresent {
		body["control_reconciliation_status"] = controlSupportStatus(ev.control.State)
	}

	counters := supportCounters(ev)
	if len(counters) != 0 {
		body["counters"] = counters
	}
	timestamps := supportTimestamps(ev)
	if len(timestamps) != 0 {
		body["state_timestamps"] = timestamps
	}
	if ev.reason != "" {
		body["reason_codes"] = []string{ev.reason}
	}

	writeJSON(w, ev.httpStatus, body)
}

func dependencyStatus(ev evaluation) map[string]string {
	out := map[string]string{}
	switch {
	case !ev.storageKnown:
		out["local_spool"] = "DOWN"
	case ev.storagePercent >= hardStoragePercent:
		out["local_spool"] = "DEGRADED"
	default:
		out["local_spool"] = "UP"
	}
	if ev.agentStorageKnown {
		if ev.agentStoragePercent >= hardStoragePercent {
			out["agent_spool"] = "DEGRADED"
		} else {
			out["agent_spool"] = "UP"
		}
	}
	switch {
	case !ev.packPresent:
		out["capability_pack"] = "DOWN"
	case !ev.pack.TrustedApplied || !ev.pack.CoreAuthorityEstablished:
		out["capability_pack"] = "DOWN"
	case ev.pack.Degraded:
		out["capability_pack"] = "DEGRADED"
	default:
		out["capability_pack"] = "UP"
	}
	switch {
	case !ev.schedulerPresent:
		out["scheduler"] = "DOWN"
	case ev.scheduler.State != scheduler.StateRunning:
		out["scheduler"] = "DOWN"
	case schedulerQueueSaturated(ev.scheduler):
		out["scheduler"] = "DEGRADED"
	default:
		out["scheduler"] = "UP"
	}
	switch {
	case !ev.controlPresent:
		out["control"] = "DOWN"
	case ev.control.State == control.StateInSync:
		out["control"] = "UP"
	case ev.control.State == control.StateSyncPending || ev.control.State == control.StateDegraded:
		out["control"] = "DEGRADED"
	case ev.control.State == control.StateUntrusted:
		out["control"] = "DOWN"
	default:
		out["control"] = "UNKNOWN"
	}
	return out
}

func supportCounters(ev evaluation) map[string]uint64 {
	out := map[string]uint64{}
	if ev.schedulerPresent {
		out["scheduler_submitted"] = safeCounter(ev.scheduler.Submitted)
		out["scheduler_completed"] = safeCounter(ev.scheduler.Completed)
		out["scheduler_failed"] = safeCounter(ev.scheduler.Failed)
		out["scheduler_retries"] = safeCounter(ev.scheduler.Retries)
		out["scheduler_rejected_queue_full"] = safeCounter(ev.scheduler.RejectedQueueFull)
	}
	if ev.controlPresent {
		out["control_reconcile_attempts"] = safeCounter(ev.control.ReconcileAttempts)
		out["control_applied"] = safeCounter(ev.control.AppliedCount)
		out["control_rejected"] = safeCounter(ev.control.RejectedCount)
		out["control_core_transport_failures"] = safeCounter(ev.control.CoreTransportFailures)
		out["control_durable_store_failures"] = safeCounter(ev.control.DurableStoreFailures)
		out["control_ack_failures"] = safeCounter(ev.control.AckFailures)
	}
	return out
}

func safeCounter(v uint64) uint64 {
	const max = uint64(9007199254740991)
	if v > max {
		return max
	}
	return v
}

func supportTimestamps(ev evaluation) map[string]string {
	out := map[string]string{}
	if ev.packPresent && !ev.pack.LastSuccessfulSync.IsZero() {
		out["pack_last_successful_sync"] = ev.pack.LastSuccessfulSync.UTC().Format(time.RFC3339Nano)
	}
	if ev.controlPresent && !ev.control.LastSuccessfulReconcile.IsZero() {
		out["control_last_successful_reconcile"] = ev.control.LastSuccessfulReconcile.UTC().Format(time.RFC3339Nano)
	}
	return out
}

func schedulerQueueSaturated(snap scheduler.Snapshot) bool {
	for _, pool := range snap.Pools {
		if pool.QueueCapacity > 0 && pool.QueueDepth >= pool.QueueCapacity {
			return true
		}
	}
	return snap.QueueCapacity > 0 && snap.QueueDepth >= snap.QueueCapacity
}

func schedulerSupportStatus(state scheduler.State) string {
	switch state {
	case scheduler.StateRunning:
		return "RUNNING"
	case scheduler.StateShuttingDown:
		return "DEGRADED"
	case scheduler.StateStopped:
		return "STOPPED"
	default:
		return "UNKNOWN"
	}
}

func controlSupportStatus(state string) string {
	switch state {
	case control.StateInSync:
		return "IN_SYNC"
	case control.StateSyncPending:
		return "SYNC_PENDING"
	case control.StateDegraded:
		return "DEGRADED"
	case control.StateUntrusted:
		return "REJECTED"
	default:
		return "UNKNOWN"
	}
}

func percentInt(v float64) int {
	if math.IsNaN(v) || v <= 0 {
		return 0
	}
	if v >= 100 {
		return 100
	}
	return int(math.Round(v))
}

func maxZeroInt(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func uptimeSeconds(start time.Time) int64 {
	if start.IsZero() {
		return 0
	}
	seconds := int64(time.Since(start).Seconds())
	if seconds < 0 {
		return 0
	}
	return seconds
}

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (hs *HealthServer) Start() error {
	hs.logger.Info("Starting sensor health server",
		"safe_context", map[string]any{"listen_configured": hs.server.Addr != ""})
	if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (hs *HealthServer) Shutdown(ctx context.Context) error {
	hs.MarkShuttingDown()
	return hs.server.Shutdown(ctx)
}
