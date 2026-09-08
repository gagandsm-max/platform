package discovery

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"platform/sensor/internal/coreclient"
)

type CoreDiscoveryClient interface {
	FetchDiscoveryTask(ctx context.Context, sensorID string) (*coreclient.DiscoveryTaskResponse, error)
	SubmitDiscoveryResults(ctx context.Context, results coreclient.DiscoveryResultSubmission, correlationID string) (coreclient.DiscoveryResultAck, error)
}

type Service struct {
	sensorID     string
	client       CoreDiscoveryClient
	executor     *Executor
	pollInterval time.Duration
	logger       *slog.Logger
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

func NewService(
	sensorID string,
	client CoreDiscoveryClient,
	executor *Executor,
	pollInterval time.Duration,
	logger *slog.Logger,
) *Service {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		sensorID:     sensorID,
		client:       client,
		executor:     executor,
		pollInterval: pollInterval,
		logger:       logger,
		stopCh:       make(chan struct{}),
	}
}

func (s *Service) Start(ctx context.Context) {
	s.logger.Info("Starting sensor discovery worker loop", "sensor_id", s.sensorID, "interval", s.pollInterval)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.PollOnce(ctx); err != nil {
					s.logger.Debug("Discovery poll completed with note", "error", err)
				}
			case <-s.stopCh:
				s.logger.Info("Sensor discovery worker loop exiting")
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *Service) PollOnce(ctx context.Context) error {
	if s.client == nil {
		return nil
	}

	taskResp, err := s.client.FetchDiscoveryTask(ctx, s.sensorID)
	if err != nil {
		return err
	}
	if !taskResp.HasTask {
		return nil
	}

	s.logger.Info("Received assigned discovery task from Core",
		"request_id", taskResp.RequestID,
		"tenant_id", taskResp.TenantID,
		"target_count", len(taskResp.Targets),
	)

	targets := make([]Target, len(taskResp.Targets))
	for i, t := range taskResp.Targets {
		targets[i] = Target{Address: t, Type: "IPV4"}
	}

	task := Task{
		TaskID:           taskResp.RequestID,
		RequestID:        taskResp.RequestID,
		TenantID:         taskResp.TenantID,
		CorrelationID:    taskResp.CorrelationID,
		Targets:          targets,
		RequestedMethods: taskResp.RequestedMethods,
		SNMPPort:         taskResp.SNMPPort,
		Timeout:          time.Duration(taskResp.TimeoutMS) * time.Millisecond,
		MaxRetries:       taskResp.MaxRetries,
	}

	result := s.executor.Run(ctx, task)

	evidencePayloads := make([]coreclient.DiscoveryEvidencePayload, len(result.Evidence))
	for i, ev := range result.Evidence {
		evidencePayloads[i] = coreclient.DiscoveryEvidencePayload{
			Target:           ev.TargetAddress,
			Protocol:         ev.Protocol,
			Method:           ev.Method,
			Status:           ev.Status,
			ReachabilityKind: ev.ReachabilityKind,
			TechnicalCode:    ev.TechnicalCode,
			Attributes:       ev.Attributes,
			RawEvidence:      ev.RawEvidence,
			DurationMS:       ev.DurationMs,
		}
	}

	submission := coreclient.DiscoveryResultSubmission{
		SensorID:      s.sensorID,
		RequestID:     task.RequestID,
		TenantID:      task.TenantID,
		Status:        result.Status,
		FailureReason: result.FailureReason,
		Evidence:      evidencePayloads,
	}

	ack, err := s.client.SubmitDiscoveryResults(ctx, submission, task.CorrelationID)
	if err != nil {
		s.logger.Error("Failed to submit discovery results to Core", "request_id", task.RequestID, "error", err)
		return err
	}

	s.logger.Info("Successfully delivered discovery results to Core",
		"request_id", ack.RequestID,
		"status", ack.Status,
		"sensor_identity", ack.SensorIdentity,
	)

	return nil
}

func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}
