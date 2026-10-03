package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/delivery"
	"platform/agent/common/queue"
)

const (
	componentName  = "platform-agent"
	defaultVersion = "0.1.0-dev"
)

// CollectorSnapshot is safe per-collector evidence for readiness.
type CollectorSnapshot struct {
	Name        string
	Enabled     bool
	Outstanding bool
	LastError   string
}

// RuntimeSnapshot is the narrow operational evidence boundary used by the
// single readiness evaluator for /ready, /healthz and /health.
type RuntimeSnapshot struct {
	Lifecycle          string
	Queue              queue.QueueStats
	Delivery           delivery.DeliveryStats
	SchedulerRunning   bool
	EnabledCollectors  int
	Collectors         []CollectorSnapshot
	CollectorLastRun   time.Time
	CollectorRunErrors int // cumulative historical counter; not a hard readiness input
}

// StatsProvider supplies one atomic-enough runtime snapshot for health decisions.
// Implementations must not hold AgentService lifecycle locks across subsystem locks.
type StatsProvider interface {
	RuntimeSnapshot() RuntimeSnapshot
}

// readinessDecision is the single evaluator result for readiness and support evidence.
type readinessDecision struct {
	status     string
	ready      bool
	httpStatus int
	reasons    []string
}

type Option func(*Server)

// WithIdentity binds health/supportability responses to the same immutable
// process identity and version as the structured logger.
func WithIdentity(version, serviceInstanceID string) Option {
	return func(s *Server) {
		if strings.TrimSpace(version) != "" {
			s.version = version
		}
		if strings.TrimSpace(serviceInstanceID) != "" {
			s.serviceInstanceID = serviceInstanceID
		}
	}
}

type Server struct {
	addr      string
	provider  StatsProvider
	logger    *slog.Logger
	startTime time.Time
	httpSrv   *http.Server
	mu        sync.Mutex
	isLive    bool

	version           string
	serviceInstanceID string
	shuttingDown      atomic.Bool
}

func NewServer(addr string, provider StatsProvider, logger *slog.Logger, opts ...Option) (*Server, error) {
	if addr == "" {
		addr = "127.0.0.1:8089"
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("HEALTH_ADDR_INVALID: %w", err)
	}
	if host != "127.0.0.1" && host != "::1" && host != "localhost" {
		return nil, errors.New("HEALTH_ADDR_NOT_LOOPBACK: health server must bind to loopback address")
	}
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		addr:              addr,
		provider:          provider,
		logger:            logger,
		startTime:         time.Now().UTC(),
		isLive:            true,
		version:           defaultVersion,
		serviceInstanceID: fmt.Sprintf("agent-health-%d", time.Now().UTC().UnixNano()),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/live", s.handleLive)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/healthz", s.handleReady) // BUILD-005: preserve Agent compatibility alias to readiness
	mux.HandleFunc("/health", s.handleHealth)

	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	return s, nil
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		s.logger.Error("Health server listen failed",
			"event_code", "AGENT_HEALTH_LISTEN_FAILED",
			"safe_context", map[string]any{"reason_class": "listen_failed"})
		return fmt.Errorf("HEALTH_LISTEN_FAILED: %w", err)
	}

	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("Health server encountered error",
				"event_code", "AGENT_HEALTH_SERVER_FAILED",
				"safe_context", map[string]any{"reason_class": "runtime_failed"})
		}
	}()

	s.logger.Info("Health server listening",
		"safe_context", map[string]any{"listen_configured": true})
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.shuttingDown.Store(true)
	s.mu.Lock()
	s.isLive = false
	s.mu.Unlock()
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) handleLive(w http.ResponseWriter, _ *http.Request) {
	status := "UP"
	s.mu.Lock()
	live := s.isLive
	s.mu.Unlock()
	if !live || s.shuttingDown.Load() {
		status = "SHUTTING_DOWN"
	}
	// Liveness remains process-only and always HTTP 200 while the process can answer.
	writeJSON(w, http.StatusOK, map[string]any{
		"status":              status,
		"component":           componentName,
		"ts":                  nowTimestamp(),
		"service_instance_id": s.serviceInstanceID,
	})
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	d := s.evaluateReadiness()
	body := map[string]any{
		"status":              d.status,
		"ready":               d.ready,
		"http_semantic":       d.httpStatus,
		"component":           componentName,
		"ts":                  nowTimestamp(),
		"service_instance_id": s.serviceInstanceID,
	}
	if len(d.reasons) > 0 {
		body["reason_code"] = d.reasons[0]
	}
	if len(d.reasons) > 1 {
		body["reason_codes"] = append([]string(nil), d.reasons...)
	}
	writeJSON(w, d.httpStatus, body)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	d := s.evaluateReadiness()
	snap := s.snapshot()

	componentState := d.status
	if s.shuttingDown.Load() {
		componentState = "SHUTTING_DOWN"
	}

	body := map[string]any{
		"document_type":       "SUPPORT_EVIDENCE",
		"component":           componentName,
		"version":             s.version,
		"service_instance_id": s.serviceInstanceID,
		"ts":                  nowTimestamp(),
		"uptime_seconds":      int64(time.Since(s.startTime).Seconds()),
		"component_state":     componentState,
		"queue_depth":         maxZeroInt(snap.Queue.ActiveCount),
		"delivery_status":     deliverySupportStatus(snap.Delivery, snap.Queue),
		"dependency_status": map[string]string{
			"sensor_transport": sensorTransportStatus(snap.Delivery),
		},
		"counters": supportCounters(snap),
	}
	if snap.Queue.ActiveCount >= 0 {
		if capHint := queueCapacityHint(snap.Queue); capHint > 0 {
			body["queue_capacity"] = capHint
		}
	}
	if !snap.CollectorLastRun.IsZero() {
		body["state_timestamps"] = map[string]string{
			"collector_last_run": snap.CollectorLastRun.UTC().Format(time.RFC3339Nano),
		}
	}
	if len(d.reasons) > 0 {
		body["reason_codes"] = append([]string(nil), d.reasons...)
	}
	writeJSON(w, d.httpStatus, body)
}

func (s *Server) snapshot() RuntimeSnapshot {
	if s.provider == nil {
		return RuntimeSnapshot{}
	}
	return s.provider.RuntimeSnapshot()
}

// evaluateReadiness is the single local-safety readiness decision for /ready,
// /healthz and /health. External Sensor/upstream degradation yields DEGRADED
// with ready=true while local collection + durable buffering remain safe.
func (s *Server) evaluateReadiness() readinessDecision {
	var hard, degraded []string

	if s.shuttingDown.Load() {
		hard = appendUnique(hard, "AGENT_SHUTTING_DOWN")
	}

	snap := s.snapshot()
	hard = appendUnique(hard, lifecycleHardReasons(snap.Lifecycle)...)

	if snap.Lifecycle == "RUNNING" {
		if !snap.SchedulerRunning {
			hard = appendUnique(hard, "AGENT_SCHEDULER_NOT_RUNNING")
		}
		if snap.EnabledCollectors <= 0 {
			hard = appendUnique(hard, "AGENT_NO_ENABLED_COLLECTORS")
		} else if allEnabledOutstanding(snap) {
			hard = appendUnique(hard, "AGENT_ALL_COLLECTORS_BLOCKED")
		}
	}

	hard = appendUnique(hard, queueHardReasons(snap.Queue)...)
	hard = appendUnique(hard, deliveryHardReasons(snap.Delivery)...)

	if len(hard) == 0 {
		degraded = appendUnique(degraded, deliveryDegradedReasons(snap.Delivery)...)
		if snap.Queue.QuarantineCount > 0 {
			degraded = appendUnique(degraded, "QUEUE_QUARANTINE_PRESENT")
		}
		degraded = appendUnique(degraded, collectorDegradedReasons(snap)...)
	} else {
		// Preserve useful secondary quarantine evidence alongside hard blocks.
		if snap.Queue.QuarantineCount > 0 {
			hard = appendUnique(hard, "QUEUE_QUARANTINE_PRESENT")
		}
	}

	switch {
	case len(hard) > 0:
		return readinessDecision{
			status:     "NOT_READY",
			ready:      false,
			httpStatus: http.StatusServiceUnavailable,
			reasons:    hard,
		}
	case len(degraded) > 0:
		return readinessDecision{
			status:     "DEGRADED",
			ready:      true,
			httpStatus: http.StatusOK,
			reasons:    degraded,
		}
	default:
		return readinessDecision{
			status:     "UP",
			ready:      true,
			httpStatus: http.StatusOK,
		}
	}
}

func lifecycleHardReasons(state string) []string {
	switch state {
	case "RUNNING":
		return nil
	case "NEW":
		return []string{"AGENT_NOT_STARTED"}
	case "STARTING":
		return []string{"AGENT_STARTING"}
	case "STOPPING":
		return []string{"AGENT_SHUTTING_DOWN"}
	case "STOPPED":
		return []string{"AGENT_STOPPED"}
	case "FAILED":
		return []string{"AGENT_FAILED"}
	default:
		// Fail closed on missing/invalid lifecycle.
		return []string{"AGENT_LIFECYCLE_INVALID"}
	}
}

func queueHardReasons(q queue.QueueStats) []string {
	var reasons []string
	if q.AtCapacity {
		reasons = appendUnique(reasons, "QUEUE_FULL")
	}
	if q.QuarantineAtCapacity {
		reasons = appendUnique(reasons, "QUEUE_QUARANTINE_FULL")
	}
	if q.AckRecoveryConflictBlock {
		reasons = appendUnique(reasons, "QUEUE_ACK_RECOVERY_CONFLICT")
	}
	if q.UnknownSequenceBlock {
		reasons = appendUnique(reasons, "QUEUE_UNKNOWN_SEQUENCE_BLOCK")
	}
	if q.LocationUncertain {
		reasons = appendUnique(reasons, "QUEUE_QUARANTINE_UNCERTAIN")
	}
	if q.MetaUncertain {
		reasons = appendUnique(reasons, "QUEUE_META_UNCERTAIN")
	}
	if q.SequenceGapBlock {
		reasons = appendUnique(reasons, "QUEUE_SEQUENCE_GAP")
	}
	if q.SequenceConflictBlock {
		reasons = appendUnique(reasons, "QUEUE_SEQUENCE_CONFLICT")
	}
	// Remaining delivery block after specific integrity flags.
	if q.DeliveryBlocked &&
		!q.AckRecoveryConflictBlock &&
		!q.UnknownSequenceBlock &&
		!q.LocationUncertain &&
		!q.MetaUncertain &&
		!q.SequenceGapBlock &&
		!q.SequenceConflictBlock {
		reasons = appendUnique(reasons, "QUEUE_DELIVERY_BLOCKED")
	}
	return reasons
}

func deliveryHardReasons(d delivery.DeliveryStats) []string {
	switch d.LastFailureClass {
	case client.FailureBlocked:
		if d.LastErrorCode != "" {
			return []string{d.LastErrorCode}
		}
		return []string{"DELIVERY_BLOCKED"}
	case client.FailureOversized:
		if d.LastErrorCode != "" {
			return []string{d.LastErrorCode}
		}
		return []string{"DELIVERY_OVERSIZED"}
	default:
		return nil
	}
}

func deliveryDegradedReasons(d delivery.DeliveryStats) []string {
	switch d.LastFailureClass {
	case client.FailureBlocked, client.FailureOversized:
		return nil
	case client.FailureTemporary, client.FailureAuthentication, client.FailurePermanent, client.FailureReceiverUnavailable:
		return []string{unavailableReason(d)}
	default:
		if !d.ReceiverAvailable || d.LastFailureClass != "" {
			return []string{unavailableReason(d)}
		}
		return nil
	}
}

func allEnabledOutstanding(snap RuntimeSnapshot) bool {
	enabled := 0
	outstanding := 0
	for _, c := range snap.Collectors {
		if !c.Enabled {
			continue
		}
		enabled++
		if c.Outstanding {
			outstanding++
		}
	}
	if enabled == 0 {
		return false
	}
	return outstanding == enabled
}

func collectorDegradedReasons(snap RuntimeSnapshot) []string {
	var reasons []string
	enabled := 0
	outstanding := 0
	hasCurrentError := false
	for _, c := range snap.Collectors {
		if !c.Enabled {
			continue
		}
		enabled++
		if c.Outstanding {
			outstanding++
		}
		if strings.TrimSpace(c.LastError) != "" {
			hasCurrentError = true
		}
	}
	if enabled == 0 {
		return nil
	}
	if outstanding > 0 && outstanding < enabled {
		reasons = appendUnique(reasons, "COLLECTOR_OUTSTANDING")
	}
	if hasCurrentError {
		reasons = appendUnique(reasons, "COLLECTOR_DEGRADED")
	}
	return reasons
}

func supportCounters(snap RuntimeSnapshot) map[string]int64 {
	return map[string]int64{
		"queue_active_count":     int64(maxZeroInt(snap.Queue.ActiveCount)),
		"queue_active_bytes":     maxZeroInt64(snap.Queue.ActiveBytes),
		"quarantine_count":       int64(maxZeroInt(snap.Queue.QuarantineCount)),
		"quarantine_bytes":       maxZeroInt64(snap.Queue.QuarantineBytes),
		"delivery_attempts":      maxZeroInt64(snap.Delivery.Attempts),
		"delivery_successes":     maxZeroInt64(snap.Delivery.Successes),
		"delivery_retry_attempt": int64(maxZeroInt(snap.Delivery.RetryAttempt)),
		"collector_errors":       int64(maxZeroInt(snap.CollectorRunErrors)),
	}
}

func deliverySupportStatus(d delivery.DeliveryStats, q queue.QueueStats) string {
	if q.DeliveryBlocked || q.AckRecoveryConflictBlock || q.UnknownSequenceBlock ||
		q.SequenceGapBlock || q.SequenceConflictBlock {
		return "BLOCKED"
	}
	if d.LastFailureClass == client.FailureBlocked || d.LastFailureClass == client.FailureOversized {
		return "FAILED"
	}
	if !d.ReceiverAvailable || d.LastFailureClass != "" {
		return "FAILED"
	}
	if q.AtCapacity || q.ActiveCount > 0 {
		return "BACKLOG"
	}
	return "OK"
}

func sensorTransportStatus(d delivery.DeliveryStats) string {
	// Transport depends on Sensor receiver availability only.
	// Local queue failures keep ReceiverAvailable=true and must not be
	// misreported as sensor_transport=DOWN (see delivery.recordQueueFailure).
	if !d.ReceiverAvailable {
		return "DOWN"
	}
	return "UP"
}

func queueCapacityHint(q queue.QueueStats) int {
	if q.AtCapacity && q.ActiveCount > 0 {
		return q.ActiveCount
	}
	return 0
}

func unavailableReason(d delivery.DeliveryStats) string {
	if d.LastErrorCode != "" {
		return d.LastErrorCode
	}
	return "SENSOR_TRANSPORT_UNAVAILABLE"
}

func appendUnique(reasons []string, codes ...string) []string {
	for _, code := range codes {
		if code == "" {
			continue
		}
		found := false
		for _, existing := range reasons {
			if existing == code {
				found = true
				break
			}
		}
		if !found {
			reasons = append(reasons, code)
		}
	}
	return reasons
}

func maxZeroInt(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func maxZeroInt64(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
