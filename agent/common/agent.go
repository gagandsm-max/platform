package common

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
}

func NewAgentService(
	cfg *config.Config,
	ident *identity.AgentIdentity,
	tr client.Transport,
	logger *slog.Logger,
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
		cfg:       cfg,
		identity:  ident,
		queue:     q,
		transport: tr,
		scheduler: sched,
		worker:    worker,
		logger:    logger,
	}

	// 5. Initialize health server
	healthSrv, err := health.NewServer(cfg.HealthAddress, svc, logger)
	if err != nil {
		return nil, fmt.Errorf("AGENT_HEALTH_INIT_FAILED: %w", err)
	}
	svc.healthSrv = healthSrv

	return svc, nil
}

func (s *AgentService) Start(ctx context.Context) error {
	s.logger.Info("Starting Platform Agent service workers",
		"agent_id", s.identity.AgentID,
		"tenant_id", s.identity.TenantID,
		"host_id", s.identity.HostID,
		"os", s.identity.OS,
		"arch", s.identity.Arch,
		"receiver_available", s.transport.Available(),
	)

	// Start health server
	if err := s.healthSrv.Start(); err != nil {
		return fmt.Errorf("AGENT_HEALTH_START_FAILED: %w", err)
	}

	// Start delivery loop
	s.worker.Start(ctx)

	// Start collection scheduler
	s.scheduler.Start(ctx)

	return nil
}

func (s *AgentService) Stop() error {
	s.logger.Info("Stopping Platform Agent service")

	// 1. Stop scheduler
	s.scheduler.Stop()

	// 2. Stop delivery worker
	s.worker.Stop()

	// 3. Stop health server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.healthSrv.Stop(shutdownCtx); err != nil {
		s.logger.Warn("Health server shutdown error", "error", err)
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
