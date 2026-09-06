package common

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/config"
	"platform/agent/common/identity"
	"platform/agent/common/queue"
)

type AgentService struct {
	cfg      *config.Config
	identity *identity.AgentIdentity
	queue    *queue.MemoryQueue
	client   *client.CoreClient
	logger   *slog.Logger
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewAgentService(cfg *config.Config, ident *identity.AgentIdentity, logger *slog.Logger) *AgentService {
	q := queue.NewMemoryQueue(cfg.MaxQueueItems)
	cl := client.NewCoreClient(cfg.CoreURL, ident, logger)
	return &AgentService{
		cfg:      cfg,
		identity: ident,
		queue:    q,
		client:   cl,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

func (s *AgentService) Start(ctx context.Context) {
	s.logger.Info("Starting Platform Agent service workers",
		"agent_id", s.identity.AgentID,
		"os", s.identity.OS,
		"arch", s.identity.Arch,
	)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.cfg.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.sendHeartbeat(ctx)
			case <-s.stopCh:
				s.logger.Info("Agent worker loop shutting down")
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *AgentService) sendHeartbeat(ctx context.Context) {
	if err := s.client.Heartbeat(ctx); err != nil {
		s.logger.Debug("Agent heartbeat attempt", "status", "core_unreachable_or_waiting", "error", err)
	} else {
		s.logger.Info("Agent heartbeat sent successfully")
	}
}

func (s *AgentService) Stop() {
	s.logger.Info("Stopping Platform Agent service")
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("Platform Agent service stopped")
}
