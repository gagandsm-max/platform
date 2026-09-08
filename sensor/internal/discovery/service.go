package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/scheduler"
)

type CoreDiscoveryClient interface {
	FetchDiscoveryTask(ctx context.Context) (coreclient.DiscoveryTaskResponse, error)
	SubmitDiscoveryResults(ctx context.Context, results coreclient.DiscoveryResultSubmission, correlationID string) (coreclient.DiscoveryResultAck, error)
}

type Service struct {
	sensorID     string
	client       CoreDiscoveryClient
	executor     *Executor
	pollInterval time.Duration
	logger       *slog.Logger
	stopCh       chan struct{}
	startOnce    sync.Once
	stopOnce     sync.Once
	wg           sync.WaitGroup
}

func NewService(sensorID string, client CoreDiscoveryClient, executor *Executor, pollInterval time.Duration, logger *slog.Logger) *Service {
	if pollInterval <= 0 { pollInterval = 5 * time.Second }
	if logger == nil { logger = slog.Default() }
	return &Service{
		sensorID: sensorID, client: client, executor: executor, pollInterval: pollInterval,
		logger: logger, stopCh: make(chan struct{}),
	}
}

func (s *Service) Start(ctx context.Context) {
	s.startOnce.Do(func() {
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
						s.logger.Debug("Discovery poll completed with classified failure", "error_code", safeErrorCode(err))
					}
				case <-s.stopCh:
					s.logger.Info("Sensor discovery worker loop exiting")
					return
				case <-ctx.Done():
					return
				}
			}
		}()
	})
}

// PollOnce is serialized by the service loop. Core atomically claims one assigned task and
// supplies the authoritative assignment fence; no local autonomous target discovery occurs.
func (s *Service) PollOnce(ctx context.Context) error {
	if s.client == nil || s.executor == nil { return fmt.Errorf("DISCOVERY_RUNTIME_NOT_INITIALIZED") }
	taskResp, err := s.client.FetchDiscoveryTask(ctx)
	if err != nil { return err }
	if !taskResp.HasTask { return nil }
	if taskResp.RequestID == "" || taskResp.TenantID == "" || taskResp.AssignmentVersion == 0 {
		return fmt.Errorf("DISCOVERY_TASK_CONTRACT_INVALID")
	}

	s.logger.Info("Received fenced discovery task from Core",
		"request_id", taskResp.RequestID, "tenant_id", taskResp.TenantID,
		"assignment_version", taskResp.AssignmentVersion, "target_count", len(taskResp.Targets))

	targets := make([]Target, len(taskResp.Targets))
	for i, address := range taskResp.Targets { targets[i] = Target{Address: address, Type: targetType(address)} }
	task := Task{
		TaskID: taskResp.RequestID, RequestID: taskResp.RequestID, TenantID: taskResp.TenantID,
		CorrelationID: taskResp.CorrelationID, AssignmentVersion: taskResp.AssignmentVersion,
		Targets: targets, RequestedMethods: taskResp.RequestedMethods,
		CredentialProfileRef: taskResp.CredentialProfileRef, SNMPPort: taskResp.SNMPPort,
		Timeout: time.Duration(taskResp.TimeoutMS) * time.Millisecond, MaxRetries: taskResp.MaxRetries,
	}
	payload, err := json.Marshal(task)
	if err != nil { return fmt.Errorf("DISCOVERY_TASK_ENCODE_FAILED") }
	execution, err := s.executor.Execute(ctx, scheduler.Job{
		JobID: scheduler.JobID(task.RequestID), AssignmentKey: scheduler.AssignmentKey("discovery:" + task.RequestID),
		TenantID: scheduler.TenantID(task.TenantID), Kind: JobKindDiscovery, Class: scheduler.ClassDiscovery,
		AssignmentVersion: task.AssignmentVersion, Timeout: task.Timeout, Payload: payload,
	})
	if err != nil { return err }
	var result Result
	if err := json.Unmarshal(execution.Payload, &result); err != nil { return fmt.Errorf("DISCOVERY_RESULT_DECODE_FAILED") }

	evidencePayloads := make([]coreclient.DiscoveryEvidencePayload, len(result.Evidence))
	for i, ev := range result.Evidence {
		evidencePayloads[i] = coreclient.DiscoveryEvidencePayload{
			Target: ev.TargetAddress, Protocol: ev.Protocol, Method: ev.Method, Status: ev.Status,
			ReachabilityKind: ev.ReachabilityKind, TechnicalCode: ev.TechnicalCode,
			Attributes: ev.Attributes, RawEvidence: ev.RawEvidence, DurationMS: ev.DurationMs,
		}
	}
	submission := coreclient.DiscoveryResultSubmission{
		RequestID: task.RequestID, AssignmentVersion: task.AssignmentVersion,
		Status: result.Status, FailureReason: result.FailureReason, Evidence: evidencePayloads,
	}
	ack, err := s.client.SubmitDiscoveryResults(ctx, submission, task.CorrelationID)
	if err != nil { return err }
	if ack.RequestID != task.RequestID { return fmt.Errorf("DISCOVERY_ACK_REQUEST_MISMATCH") }
	if ack.Status != "ACCEPTED" && ack.Status != "ALREADY_ACCEPTED" { return fmt.Errorf("DISCOVERY_ACK_INVALID") }
	s.logger.Info("Discovery result acknowledged by Core", "request_id", ack.RequestID, "status", ack.Status)
	return nil
}

func targetType(address string) string {
	if ip := net.ParseIP(address); ip != nil {
		if ip.To4() != nil { return "IPV4" }
		return "IPV6"
	}
	return "HOSTNAME"
}

func safeErrorCode(err error) string {
	if err == nil { return "" }
	if coreclient.IsCode(err, coreclient.CodeAuthentication) { return string(coreclient.CodeAuthentication) }
	if coreclient.IsCode(err, coreclient.CodeAuthorization) { return string(coreclient.CodeAuthorization) }
	if coreclient.IsCode(err, coreclient.CodeTimeout) { return string(coreclient.CodeTimeout) }
	if coreclient.IsTransient(err) { return "DISCOVERY_CORE_TEMPORARY" }
	return "DISCOVERY_OPERATION_FAILED"
}

func (s *Service) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}
