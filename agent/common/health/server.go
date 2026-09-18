package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"platform/agent/common/delivery"
	"platform/agent/common/queue"
)

type StatsProvider interface {
	QueueStats() queue.QueueStats
	DeliveryStats() delivery.DeliveryStats
	CollectorStats() (time.Time, int)
}

type Server struct {
	addr      string
	provider  StatsProvider
	logger    *slog.Logger
	startTime time.Time
	httpSrv   *http.Server
	mu        sync.Mutex
	isLive    bool
}

func NewServer(addr string, provider StatsProvider, logger *slog.Logger) (*Server, error) {
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
		addr:      addr,
		provider:  provider,
		logger:    logger,
		startTime: time.Now().UTC(),
		isLive:    true,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/live", s.handleLive)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/healthz", s.handleReady)
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
		return fmt.Errorf("HEALTH_LISTEN_FAILED: %w", err)
	}

	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("Health server encountered error", "error", err)
		}
	}()

	s.logger.Info("Health server listening", "address", s.addr)
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.isLive = false
	s.mu.Unlock()
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	live := s.isLive
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if live {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "UP",
			"runtime": "platform-agent",
			"uptime":  time.Since(s.startTime).String(),
		})
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "DOWN",
		})
	}
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var deliv delivery.DeliveryStats
	if s.provider != nil {
		deliv = s.provider.DeliveryStats()
	}

	// HTTPS transport construction is not Sensor reachability. /ready degrades
	// when the transport is unwired or the last delivery attempt failed.
	if !deliv.ReceiverAvailable || deliv.LastFailureClass != "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":             "DEGRADED",
			"ready":              false,
			"receiver_available": deliv.ReceiverAvailable,
			"reason":             unavailableReason(deliv),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":             "UP",
		"ready":              true,
		"receiver_available": true,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var qStats queue.QueueStats
	var dStats delivery.DeliveryStats
	var lastRun time.Time
	var collectorErrors int

	if s.provider != nil {
		qStats = s.provider.QueueStats()
		dStats = s.provider.DeliveryStats()
		lastRun, collectorErrors = s.provider.CollectorStats()
	}

	status := "UP"
	reasons := []string{}
	if !dStats.ReceiverAvailable || dStats.LastFailureClass != "" {
		status = "DEGRADED"
		reasons = append(reasons, unavailableReason(dStats))
	}
	if qStats.AtCapacity {
		status = "DEGRADED"
		reasons = append(reasons, "QUEUE_FULL")
	}
	if qStats.QuarantineCount > 0 {
		status = "DEGRADED"
		reasons = append(reasons, "QUEUE_QUARANTINE_PRESENT")
	}
	if qStats.DeliveryBlocked {
		status = "DEGRADED"
		reasons = append(reasons, "QUEUE_DELIVERY_BLOCKED")
	}
	if qStats.UnknownSequenceBlock {
		status = "DEGRADED"
		reasons = append(reasons, "QUEUE_UNKNOWN_SEQUENCE_BLOCK")
	}
	if qStats.LocationUncertain {
		status = "DEGRADED"
		reasons = append(reasons, "QUEUE_QUARANTINE_UNCERTAIN")
	}
	if qStats.MetaUncertain {
		status = "DEGRADED"
		reasons = append(reasons, "QUEUE_META_UNCERTAIN")
	}
	if qStats.AckRecoveryConflictBlock {
		status = "DEGRADED"
		reasons = append(reasons, "QUEUE_ACK_RECOVERY_CONFLICT")
	}
	if qStats.QuarantineAtCapacity {
		status = "DEGRADED"
		reasons = append(reasons, "QUEUE_QUARANTINE_FULL")
	}
	if collectorErrors > 0 {
		status = "DEGRADED"
		reasons = append(reasons, "COLLECTOR_ERRORS")
	}

	resp := map[string]any{
		"status":     status,
		"runtime":    "platform-agent",
		"uptime_sec": time.Since(s.startTime).Seconds(),
		"reasons":    reasons,
		"collector": map[string]any{
			"last_run":         lastRun,
			"collector_errors": collectorErrors,
		},
		"queue": map[string]any{
			"active_count":           qStats.ActiveCount,
			"active_bytes":           qStats.ActiveBytes,
			"quarantine_count":       qStats.QuarantineCount,
			"quarantine_bytes":       qStats.QuarantineBytes,
			"oldest_age_sec":         qStats.OldestAge.Seconds(),
			"last_sequence":          qStats.LastSequence,
			"at_capacity":            qStats.AtCapacity,
			"quarantine_at_capacity": qStats.QuarantineAtCapacity,
			"blocked_from_sequence":  qStats.BlockedFromSequence,
			"delivery_blocked":       qStats.DeliveryBlocked,
			"unknown_sequence_block": qStats.UnknownSequenceBlock,
			"location_uncertain":         qStats.LocationUncertain,
			"meta_uncertain":             qStats.MetaUncertain,
			"ack_recovery_conflict":      qStats.AckRecoveryConflictBlock,
		},
		"delivery": map[string]any{
			"attempts":           dStats.Attempts,
			"successes":          dStats.Successes,
			"last_attempt":       dStats.LastAttemptTime,
			"last_success":       dStats.LastSuccessTime,
			"last_failure_class": dStats.LastFailureClass,
			"last_error_code":    dStats.LastErrorCode,
			"receiver_available": dStats.ReceiverAvailable,
			"retry_attempt":      dStats.RetryAttempt,
		},
	}

	if status == "DEGRADED" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func unavailableReason(d delivery.DeliveryStats) string {
	if d.LastErrorCode != "" {
		return d.LastErrorCode
	}
	return "SENSOR_TRANSPORT_UNAVAILABLE"
}
