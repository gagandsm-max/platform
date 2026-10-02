package common

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/collector"
	"platform/agent/common/config"
	"platform/agent/common/delivery"
	"platform/agent/common/health"
	"platform/agent/common/identity"
	"platform/agent/common/queue"
	"platform/agent/common/scheduler"
)

type AgentService struct {
	cfg       *config.Config
	identity  *identity.AgentIdentity
	queue     *queue.DurableQueue
	transport client.Transport
	scheduler *scheduler.Scheduler
	worker    *delivery.Worker
	healthSrv *health.Server
	logger    *slog.Logger

	version           string
	serviceInstanceID string
}

// ServiceOption configures Agent service construction for BUILD-005 supportability identity.
type ServiceOption func(*AgentService)

// WithSupportabilityIdentity binds the health SUPPORT_EVIDENCE envelope to the
// same process-local identity used by the canonical structured logger.
func WithSupportabilityIdentity(version, serviceInstanceID string) ServiceOption {
	return func(s *AgentService) {
		if strings.TrimSpace(version) != "" {
			s.version = version
		}
		if strings.TrimSpace(serviceInstanceID) != "" {
			s.serviceInstanceID = serviceInstanceID
		}
	}
}

func NewAgentService(
	cfg *config.Config,
	ident *identity.AgentIdentity,
	tr client.Transport,
	logger *slog.Logger,
	opts ...ServiceOption,
) (*AgentService, error) {
	if cfg == nil {
		return nil, errors.New("AGENT_CONFIG_REQUIRED: config must be provided")
	}
	if ident == nil {
		return nil, errors.New("AGENT_IDENTITY_REQUIRED: identity must be provided")
	}
	if logger == nil {
		logger = slog.Default()
	}

	// If no transport injected, wire production HTTPS+mTLS to the assigned Sensor.
	// Tests inject UnavailableTransport or a fake Transport explicitly.
	if tr == nil {
		httpsTr, err := client.NewHTTPSTransport(client.HTTPSConfig{
			SensorURL:      cfg.SensorURL,
			AgentID:        ident.AgentID,
			TenantID:       ident.TenantID,
			SiteID:         cfg.SiteID,
			CACertFile:     cfg.CAFile,
			ClientCertFile: cfg.CertFile,
			ClientKeyFile:  cfg.KeyFile,
			Timeout:        cfg.DeliveryTimeout,
			Logger:         logger,
		})
		if err != nil {
			return nil, fmt.Errorf("AGENT_TRANSPORT_INIT_FAILED: %w", err)
		}
		tr = httpsTr
	}

	// 1. Initialize durable queue
	q, err := queue.NewDurableQueue(
		cfg.QueueDir,
		cfg.MaxBufferRecords,
		cfg.MaxBufferBytes,
		cfg.MaxRecordBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("AGENT_QUEUE_INIT_FAILED: %w", err)
	}

	// 2. Initialize host collectors
	collectors := collector.NewDefaultCollectors(cfg.MaxFilesystems)

	// 3. Initialize scheduler
	sched, err := scheduler.NewScheduler(
		ident,
		collectors,
		q,
		cfg.CollectionInterval,
		cfg.CollectionTimeout,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("AGENT_SCHEDULER_INIT_FAILED: %w", err)
	}

	// 4. Initialize delivery worker
	worker, err := delivery.NewWorker(
		q,
		tr,
		ident.AgentID,
		cfg.MaxBatchRecords,
		cfg.MaxBatchBytes,
		cfg.RetryBase,
		cfg.RetryMax,
		cfg.DeliveryTimeout,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("AGENT_DELIVERY_INIT_FAILED: %w", err)
	}

	svc := &AgentService{
		cfg:               cfg,
		identity:          ident,
		queue:             q,
		transport:         tr,
		scheduler:         sched,
		worker:            worker,
		logger:            logger,
		version:           "0.1.0-dev",
		serviceInstanceID: "",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}

	// 5. Initialize health server
	healthOpts := []health.Option{}
	if svc.version != "" || svc.serviceInstanceID != "" {
		healthOpts = append(healthOpts, health.WithIdentity(svc.version, svc.serviceInstanceID))
	}
	healthSrv, err := health.NewServer(cfg.HealthAddress, svc, logger, healthOpts...)
	if err != nil {
		return nil, fmt.Errorf("AGENT_HEALTH_INIT_FAILED: %w", err)
	}
	svc.healthSrv = healthSrv

	return svc, nil
}

func (s *AgentService) Start(ctx context.Context) error {
	// agent_id/tenant_id are bound once via logger.With in linux main; do not
	// re-emit them here (duplicate JSON keys invalidate envelope validators).
	s.logger.Info("Starting Platform Agent service workers",
		"safe_context", map[string]any{
			"host_id":            s.identity.HostID,
			"os":                 s.identity.OS,
			"arch":               s.identity.Arch,
			"receiver_available": s.transport.Available(),
		},
	)

	// Start health server
	if err := s.healthSrv.Start(); err != nil {
		s.logger.Error("Failed to start health server",
			"event_code", "AGENT_HEALTH_START_FAILED",
			"safe_context", map[string]any{"reason_class": "startup_failed"})
		return fmt.Errorf("AGENT_HEALTH_START_FAILED: %w", err)
	}

	// Start delivery loop
	s.worker.Start(ctx)

	// Start collection scheduler
	s.scheduler.Start(ctx)

	return nil
}

func (s *AgentService) Stop() error {
	s.logger.Info("Stopping Platform Agent service",
		"safe_context", map[string]any{"reason_class": "shutdown_requested"})

	// 1. Stop scheduler
	s.scheduler.Stop()

	// 2. Stop delivery worker
	s.worker.Stop()

	// 3. Stop health server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.healthSrv.Stop(shutdownCtx); err != nil {
		s.logger.Warn("Health server shutdown error",
			"event_code", "AGENT_HEALTH_SHUTDOWN_FAILED",
			"safe_context", map[string]any{"reason_class": "shutdown_failed"})
	}

	s.logger.Info("Platform Agent service stopped cleanly")
	return nil
}

func (s *AgentService) QueueStats() queue.QueueStats {
	return s.queue.Stats()
}

func (s *AgentService) DeliveryStats() delivery.DeliveryStats {
	return s.worker.Stats()
}

func (s *AgentService) CollectorStats() (time.Time, int) {
	return s.scheduler.Stats()
}
