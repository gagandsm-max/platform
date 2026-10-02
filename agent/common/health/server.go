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

	"platform/agent/common/delivery"
	"platform/agent/common/queue"
)

const (
	componentName  = "platform-agent"
	defaultVersion = "0.1.0-dev"
)

type StatsProvider interface {
	QueueStats() queue.QueueStats
	DeliveryStats() delivery.DeliveryStats
	CollectorStats() (time.Time, int)
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
	status, ready, httpStatus, reason := s.evaluateReadiness()
	body := map[string]any{
		"status":              status,
		"ready":               ready,
		"http_semantic":       httpStatus,
		"component":           componentName,
		"ts":                  nowTimestamp(),
		"service_instance_id": s.serviceInstanceID,
	}
	if reason != "" {
		body["reason_code"] = reason
	}
	writeJSON(w, httpStatus, body)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	status, ready, httpStatus, reasons := s.evaluateDetailed()
	componentState := status
	if s.shuttingDown.Load() {
		componentState = "SHUTTING_DOWN"
	}

	var qStats queue.QueueStats
	var dStats delivery.DeliveryStats
	var lastRun time.Time
	var collectorErrors int
	if s.provider != nil {
		qStats = s.provider.QueueStats()
		dStats = s.provider.DeliveryStats()
		lastRun, collectorErrors = s.provider.CollectorStats()
	}

	body := map[string]any{
		"document_type":       "SUPPORT_EVIDENCE",
		"component":           componentName,
		"version":             s.version,
		"service_instance_id": s.serviceInstanceID,
		"ts":                  nowTimestamp(),
		"uptime_seconds":      int64(time.Since(s.startTime).Seconds()),
		"component_state":     componentState,
		"queue_depth":         maxZeroInt(qStats.ActiveCount),
		"delivery_status":     deliverySupportStatus(dStats, qStats),
		"dependency_status": map[string]string{
			"sensor_transport": sensorTransportStatus(dStats),
		},
		"counters": supportCounters(qStats, dStats, collectorErrors),
	}
	if qStats.ActiveCount >= 0 {
		// Capacity is not always known as a hard max; omit when zero/unknown.
		if capHint := queueCapacityHint(qStats); capHint > 0 {
			body["queue_capacity"] = capHint
		}
	}
	if !lastRun.IsZero() {
		body["state_timestamps"] = map[string]string{
			"collector_last_run": lastRun.UTC().Format(time.RFC3339Nano),
		}
	}
	if len(reasons) > 0 {
		body["reason_codes"] = reasons
	}
	_ = ready // readiness is expressed via /ready; detailed evidence stays SUPPORT_EVIDENCE-shaped.
	writeJSON(w, httpStatus, body)
}

func (s *Server) evaluateReadiness() (status string, ready bool, httpStatus int, reason string) {
	status = "UP"
	ready = true
	httpStatus = http.StatusOK

	if s.shuttingDown.Load() {
		return "NOT_READY", false, http.StatusServiceUnavailable, "AGENT_SHUTTING_DOWN"
	}

	var deliv delivery.DeliveryStats
	if s.provider != nil {
		deliv = s.provider.DeliveryStats()
	}
	if !deliv.ReceiverAvailable || deliv.LastFailureClass != "" {
		return "NOT_READY", false, http.StatusServiceUnavailable, unavailableReason(deliv)
	}
	return status, ready, httpStatus, ""
}

func (s *Server) evaluateDetailed() (status string, ready bool, httpStatus int, reasons []string) {
	status = "UP"
	ready = true
	httpStatus = http.StatusOK

	if s.shuttingDown.Load() {
		status = "SHUTTING_DOWN"
		ready = false
		httpStatus = http.StatusServiceUnavailable
		reasons = append(reasons, "AGENT_SHUTTING_DOWN")
	}

	var qStats queue.QueueStats
	var dStats delivery.DeliveryStats
	var collectorErrors int
	if s.provider != nil {
		qStats = s.provider.QueueStats()
		dStats = s.provider.DeliveryStats()
		_, collectorErrors = s.provider.CollectorStats()
	}

	if !dStats.ReceiverAvailable || dStats.LastFailureClass != "" {
		if status == "UP" {
			status = "DEGRADED"
		}
		httpStatus = http.StatusServiceUnavailable
		ready = false
		reasons = append(reasons, unavailableReason(dStats))
	}
	if qStats.AtCapacity {
		status = "DEGRADED"
		httpStatus = http.StatusServiceUnavailable
		ready = false
		reasons = append(reasons, "QUEUE_FULL")
	}
	if qStats.QuarantineCount > 0 {
		status = "DEGRADED"
		httpStatus = http.StatusServiceUnavailable
		ready = false
		reasons = append(reasons, "QUEUE_QUARANTINE_PRESENT")
	}
	if qStats.DeliveryBlocked {
		status = "DEGRADED"
		httpStatus = http.StatusServiceUnavailable
		ready = false
		reasons = append(reasons, "QUEUE_DELIVERY_BLOCKED")
	}
	if qStats.UnknownSequenceBlock {
		status = "DEGRADED"
		httpStatus = http.StatusServiceUnavailable
		ready = false
		reasons = append(reasons, "QUEUE_UNKNOWN_SEQUENCE_BLOCK")
	}
	if qStats.LocationUncertain {
		status = "DEGRADED"
		httpStatus = http.StatusServiceUnavailable
		ready = false
		reasons = append(reasons, "QUEUE_QUARANTINE_UNCERTAIN")
	}
	if qStats.MetaUncertain {
		status = "DEGRADED"
		httpStatus = http.StatusServiceUnavailable
		ready = false
		reasons = append(reasons, "QUEUE_META_UNCERTAIN")
	}
	if qStats.AckRecoveryConflictBlock {
		status = "DEGRADED"
		httpStatus = http.StatusServiceUnavailable
		ready = false
		reasons = append(reasons, "QUEUE_ACK_RECOVERY_CONFLICT")
	}
	if qStats.QuarantineAtCapacity {
		status = "DEGRADED"
		httpStatus = http.StatusServiceUnavailable
		ready = false
		reasons = append(reasons, "QUEUE_QUARANTINE_FULL")
	}
	if collectorErrors > 0 {
		status = "DEGRADED"
		httpStatus = http.StatusServiceUnavailable
		ready = false
		reasons = append(reasons, "COLLECTOR_ERRORS")
	}
	return status, ready, httpStatus, reasons
}

func supportCounters(q queue.QueueStats, d delivery.DeliveryStats, collectorErrors int) map[string]int64 {
	out := map[string]int64{
		"queue_active_count":     int64(maxZeroInt(q.ActiveCount)),
		"queue_active_bytes":     maxZeroInt64(q.ActiveBytes),
		"quarantine_count":       int64(maxZeroInt(q.QuarantineCount)),
		"quarantine_bytes":       maxZeroInt64(q.QuarantineBytes),
		"delivery_attempts":      maxZeroInt64(d.Attempts),
		"delivery_successes":     maxZeroInt64(d.Successes),
		"delivery_retry_attempt": int64(maxZeroInt(d.RetryAttempt)),
		"collector_errors":       int64(maxZeroInt(collectorErrors)),
	}
	return out
}

func deliverySupportStatus(d delivery.DeliveryStats, q queue.QueueStats) string {
	if q.DeliveryBlocked || q.AckRecoveryConflictBlock || q.UnknownSequenceBlock {
		return "BLOCKED"
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
	if !d.ReceiverAvailable || d.LastFailureClass != "" {
		return "DOWN"
	}
	return "UP"
}

func queueCapacityHint(q queue.QueueStats) int {
	// Durable queue does not always expose a numeric capacity; when at capacity,
	// report current depth as a hard bound signal for SUPPORT_EVIDENCE consumers.
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
