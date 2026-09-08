package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/scheduler"
)

type mockICMPProber struct {
	probeFn func(ctx context.Context, target string, timeout time.Duration) (bool, int64, string, error)
}

func (m *mockICMPProber) Probe(ctx context.Context, target string, timeout time.Duration) (bool, int64, string, error) {
	if m.probeFn != nil {
		return m.probeFn(ctx, target, timeout)
	}
	return true, 1200, "ICMP_ECHO_REPLY", nil
}

type mockSNMPProber struct {
	probeFn func(ctx context.Context, target string, port int, timeout time.Duration) (map[string]any, string, error)
}

func (m *mockSNMPProber) ProbeIdentity(ctx context.Context, target string, port int, timeout time.Duration) (map[string]any, string, error) {
	if m.probeFn != nil {
		return m.probeFn(ctx, target, port, timeout)
	}
	return map[string]any{
		"sys_descr":     "Cisco IOS Software, Catalyst 3850",
		"sys_object_id": "1.3.6.1.4.1.9.1.1745",
		"sys_name":      "switch-core-01",
	}, "SNMP_V2C_SUCCESS", nil
}

type mockCoreDiscoveryClient struct {
	fetchTaskFn     func(ctx context.Context, sensorID string) (*coreclient.DiscoveryTaskResponse, error)
	submitResultsFn func(ctx context.Context, results coreclient.DiscoveryResultSubmission, correlationID string) (coreclient.DiscoveryResultAck, error)
}

func (m *mockCoreDiscoveryClient) FetchDiscoveryTask(ctx context.Context, sensorID string) (*coreclient.DiscoveryTaskResponse, error) {
	if m.fetchTaskFn != nil {
		return m.fetchTaskFn(ctx, sensorID)
	}
	return &coreclient.DiscoveryTaskResponse{HasTask: false}, nil
}

func (m *mockCoreDiscoveryClient) SubmitDiscoveryResults(ctx context.Context, results coreclient.DiscoveryResultSubmission, correlationID string) (coreclient.DiscoveryResultAck, error) {
	if m.submitResultsFn != nil {
		return m.submitResultsFn(ctx, results, correlationID)
	}
	return coreclient.DiscoveryResultAck{
		Status:         "COMPLETED",
		RequestID:      results.RequestID,
		SensorIdentity: results.SensorID,
		ReceivedAt:     time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func TestExecutor_Execute_InvalidJobClass(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 5, nil)
	job := scheduler.Job{
		Class:   scheduler.ClassNetworkPoll,
		Payload: []byte(`{}`),
	}
	_, err := exec.Execute(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for invalid job class, got nil")
	}
	if !scheduler.IsCode(err, scheduler.CodePermanent) {
		t.Fatalf("expected permanent scheduler error code, got %v", err)
	}
}

func TestExecutor_Execute_MalformedPayload(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 5, nil)
	job := scheduler.Job{
		Class:   scheduler.ClassDiscovery,
		Payload: []byte(`{invalid json`),
	}
	_, err := exec.Execute(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for malformed payload, got nil")
	}
	if !scheduler.IsCode(err, scheduler.CodePermanent) {
		t.Fatalf("expected permanent scheduler error code, got %v", err)
	}
}

func TestExecutor_Run_ICMP_Success(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 2, nil)
	exec.SetProbers(&mockICMPProber{
		probeFn: func(ctx context.Context, target string, timeout time.Duration) (bool, int64, string, error) {
			return true, 1500, "ICMP_ECHO_REPLY", nil
		},
	}, nil)

	task := Task{
		TaskID:           "task-1",
		RequestID:        "req-1",
		TenantID:         "tenant-alpha",
		CorrelationID:    "corr-1",
		Targets:          []Target{{Address: "192.168.1.1", Type: "IPV4"}},
		RequestedMethods: []string{"ICMP"},
		Timeout:          500 * time.Millisecond,
	}

	res := exec.Run(context.Background(), task)
	if res.Status != "COMPLETED" {
		t.Fatalf("expected status COMPLETED, got %s", res.Status)
	}
	if len(res.Evidence) != 1 {
		t.Fatalf("expected 1 evidence item, got %d", len(res.Evidence))
	}
	ev := res.Evidence[0]
	if ev.Protocol != "ICMP" {
		t.Errorf("expected protocol ICMP, got %s", ev.Protocol)
	}
	if ev.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", ev.Status)
	}
	if ev.ReachabilityKind != "OBSERVED" {
		t.Errorf("expected reachability OBSERVED, got %s", ev.ReachabilityKind)
	}
	if ev.TechnicalCode != "ICMP_ECHO_REPLY" {
		t.Errorf("expected technical code ICMP_ECHO_REPLY, got %s", ev.TechnicalCode)
	}
}

func TestExecutor_Run_ICMP_Timeout_NonCausal(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 2, nil)
	exec.SetProbers(&mockICMPProber{
		probeFn: func(ctx context.Context, target string, timeout time.Duration) (bool, int64, string, error) {
			return false, 0, "ICMP_TIMEOUT", errors.New("request timed out")
		},
	}, nil)

	task := Task{
		TaskID:           "task-2",
		RequestID:        "req-2",
		TenantID:         "tenant-alpha",
		Targets:          []Target{{Address: "192.168.1.2", Type: "IPV4"}},
		RequestedMethods: []string{"ICMP"},
	}

	res := exec.Run(context.Background(), task)
	if len(res.Evidence) != 1 {
		t.Fatalf("expected 1 evidence item, got %d", len(res.Evidence))
	}
	ev := res.Evidence[0]
	// Non-causal rule: ICMP failure MUST be INCONCLUSIVE with reachability UNKNOWN
	// It MUST NOT declare device down or permanent failure!
	if ev.Status != "INCONCLUSIVE" {
		t.Errorf("expected status INCONCLUSIVE for ICMP timeout, got %s", ev.Status)
	}
	if ev.ReachabilityKind != "UNKNOWN" {
		t.Errorf("expected reachability UNKNOWN for ICMP timeout, got %s", ev.ReachabilityKind)
	}
	if ev.TechnicalCode != "ICMP_TIMEOUT" {
		t.Errorf("expected techCode ICMP_TIMEOUT, got %s", ev.TechnicalCode)
	}
}

func TestExecutor_Run_SNMP_Success(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 2, nil)
	exec.SetProbers(nil, &mockSNMPProber{
		probeFn: func(ctx context.Context, target string, port int, timeout time.Duration) (map[string]any, string, error) {
			return map[string]any{
				"sys_descr":     "Linux debian-node 6.1.0",
				"sys_object_id": "1.3.6.1.4.1.8072.3.2.10",
				"sys_name":      "srv-prod-01",
			}, "SNMP_GET_RESPONSE", nil
		},
	})

	task := Task{
		TaskID:           "task-3",
		RequestID:        "req-3",
		TenantID:         "tenant-alpha",
		Targets:          []Target{{Address: "192.168.1.3", Type: "IPV4"}},
		RequestedMethods: []string{"SNMP"},
	}

	res := exec.Run(context.Background(), task)
	if len(res.Evidence) != 1 {
		t.Fatalf("expected 1 evidence item, got %d", len(res.Evidence))
	}
	ev := res.Evidence[0]
	if ev.Protocol != "SNMP" {
		t.Errorf("expected protocol SNMP, got %s", ev.Protocol)
	}
	if ev.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", ev.Status)
	}
	if ev.ReachabilityKind != "OBSERVED" {
		t.Errorf("expected reachability OBSERVED, got %s", ev.ReachabilityKind)
	}
	if ev.Attributes["sys_name"] != "srv-prod-01" {
		t.Errorf("expected sys_name srv-prod-01, got %v", ev.Attributes["sys_name"])
	}
}

func TestExecutor_Run_SNMP_Timeout_NonCausal(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 2, nil)
	exec.SetProbers(nil, &mockSNMPProber{
		probeFn: func(ctx context.Context, target string, port int, timeout time.Duration) (map[string]any, string, error) {
			return nil, "SNMP_TIMEOUT", errors.New("timeout reading from socket")
		},
	})

	task := Task{
		TaskID:           "task-4",
		RequestID:        "req-4",
		TenantID:         "tenant-alpha",
		Targets:          []Target{{Address: "192.168.1.4", Type: "IPV4"}},
		RequestedMethods: []string{"SNMP"},
	}

	res := exec.Run(context.Background(), task)
	if len(res.Evidence) != 1 {
		t.Fatalf("expected 1 evidence item, got %d", len(res.Evidence))
	}
	ev := res.Evidence[0]
	// Non-causal rule: SNMP timeout MUST be INCONCLUSIVE with reachability UNKNOWN
	// It MUST NOT assume wrong credentials or device down!
	if ev.Status != "INCONCLUSIVE" {
		t.Errorf("expected status INCONCLUSIVE for SNMP timeout, got %s", ev.Status)
	}
	if ev.ReachabilityKind != "UNKNOWN" {
		t.Errorf("expected reachability UNKNOWN for SNMP timeout, got %s", ev.ReachabilityKind)
	}
	if ev.TechnicalCode != "SNMP_TIMEOUT" {
		t.Errorf("expected techCode SNMP_TIMEOUT, got %s", ev.TechnicalCode)
	}
}

func TestExecutor_Run_BoundedConcurrency(t *testing.T) {
	var maxActive int32
	var currentActive int32

	exec := NewExecutor("sensor-test-01", 3, nil)
	exec.SetProbers(&mockICMPProber{
		probeFn: func(ctx context.Context, target string, timeout time.Duration) (bool, int64, string, error) {
			active := atomic.AddInt32(&currentActive, 1)
			for {
				oldMax := atomic.LoadInt32(&maxActive)
				if active <= oldMax || atomic.CompareAndSwapInt32(&maxActive, oldMax, active) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			atomic.AddInt32(&currentActive, -1)
			return true, 500, "ICMP_ECHO_REPLY", nil
		},
	}, nil)

	targets := make([]Target, 10)
	for i := 0; i < 10; i++ {
		targets[i] = Target{Address: fmt.Sprintf("192.168.1.%d", i+1), Type: "IPV4"}
	}

	task := Task{
		TaskID:           "task-concurrency",
		RequestID:        "req-concurrency",
		TenantID:         "tenant-alpha",
		Targets:          targets,
		RequestedMethods: []string{"ICMP"},
	}

	res := exec.Run(context.Background(), task)
	if len(res.Evidence) != 10 {
		t.Fatalf("expected 10 evidence items, got %d", len(res.Evidence))
	}
	if maxActive > 3 {
		t.Fatalf("concurrency violated: expected max 3 concurrent probes, observed %d", maxActive)
	}
}

func TestExecutor_Run_Cancellation(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 1, nil)
	exec.SetProbers(&mockICMPProber{
		probeFn: func(ctx context.Context, target string, timeout time.Duration) (bool, int64, string, error) {
			select {
			case <-ctx.Done():
				return false, 0, "CANCELLED", ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return true, 100, "ICMP_ECHO_REPLY", nil
			}
		},
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately after launch
	time.AfterFunc(15*time.Millisecond, cancel)

	targets := []Target{
		{Address: "192.168.1.1", Type: "IPV4"},
		{Address: "192.168.1.2", Type: "IPV4"},
		{Address: "192.168.1.3", Type: "IPV4"},
	}

	task := Task{
		TaskID:           "task-cancel",
		RequestID:        "req-cancel",
		TenantID:         "tenant-alpha",
		Targets:          targets,
		RequestedMethods: []string{"ICMP"},
	}

	res := exec.Run(ctx, task)
	if res.Status != "FAILED" {
		t.Fatalf("expected task status FAILED on cancellation, got %s", res.Status)
	}
}

func TestExecutor_ZeroCredentialsInEvidence(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 2, nil)
	exec.SetProbers(
		&mockICMPProber{},
		&mockSNMPProber{
			probeFn: func(ctx context.Context, target string, port int, timeout time.Duration) (map[string]any, string, error) {
				return map[string]any{
					"sys_descr": "Mock Switch",
					"sys_name":  "sw1",
				}, "SNMP_GET_RESPONSE", nil
			},
		},
	)

	task := Task{
		TaskID:           "task-clean",
		RequestID:        "req-clean",
		TenantID:         "tenant-alpha",
		Targets:          []Target{{Address: "192.168.1.1", Type: "IPV4"}},
		RequestedMethods: []string{"ICMP", "SNMP"},
	}

	res := exec.Run(context.Background(), task)
	for _, ev := range res.Evidence {
		rawBytes, _ := json.Marshal(ev)
		rawStr := string(rawBytes)
		prohibited := []string{"password", "secret", "community", "private", "credential"}
		for _, word := range prohibited {
			if containsInsensitive(rawStr, word) {
				t.Fatalf("prohibited secret keyword %q found in evidence payload: %s", word, rawStr)
			}
		}
	}
}

func containsInsensitive(s, substr string) bool {
	return len(s) >= len(substr) && (len(substr) == 0 || (searchInsensitive(s, substr)))
}

func searchInsensitive(s, substr string) bool {
	sLow := []byte(s)
	subLow := []byte(substr)
	for i := range sLow {
		if sLow[i] >= 'A' && sLow[i] <= 'Z' {
			sLow[i] += 32
		}
	}
	for i := range subLow {
		if subLow[i] >= 'A' && subLow[i] <= 'Z' {
			subLow[i] += 32
		}
	}
	for i := 0; i+len(subLow) <= len(sLow); i++ {
		match := true
		for j := 0; j < len(subLow); j++ {
			if sLow[i+j] != subLow[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestService_PollOnce(t *testing.T) {
	var receivedResults coreclient.DiscoveryResultSubmission

	mockClient := &mockCoreDiscoveryClient{
		fetchTaskFn: func(ctx context.Context, sensorID string) (*coreclient.DiscoveryTaskResponse, error) {
			return &coreclient.DiscoveryTaskResponse{
				Status:           "OK",
				HasTask:          true,
				RequestID:        "req-test-123",
				TenantID:         "tenant-1",
				CorrelationID:    "corr-123",
				Targets:          []string{"192.168.1.50"},
				RequestedMethods: []string{"ICMP"},
				TimeoutMS:        1000,
			}, nil
		},
		submitResultsFn: func(ctx context.Context, results coreclient.DiscoveryResultSubmission, correlationID string) (coreclient.DiscoveryResultAck, error) {
			receivedResults = results
			return coreclient.DiscoveryResultAck{
				Status:         "COMPLETED",
				RequestID:      results.RequestID,
				SensorIdentity: "sensor-test-01",
				ReceivedAt:     time.Now().UTC().Format(time.RFC3339),
			}, nil
		},
	}

	exec := NewExecutor("sensor-test-01", 2, nil)
	exec.SetProbers(&mockICMPProber{
		probeFn: func(ctx context.Context, target string, timeout time.Duration) (bool, int64, string, error) {
			return true, 800, "ICMP_ECHO_REPLY", nil
		},
	}, nil)

	svc := NewService("sensor-test-01", mockClient, exec, 100*time.Millisecond, nil)

	err := svc.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	if receivedResults.RequestID != "req-test-123" {
		t.Fatalf("expected request ID req-test-123, got %s", receivedResults.RequestID)
	}
	if len(receivedResults.Evidence) != 1 {
		t.Fatalf("expected 1 evidence item, got %d", len(receivedResults.Evidence))
	}
	if receivedResults.Evidence[0].Status != "SUCCESS" {
		t.Fatalf("expected evidence status SUCCESS, got %s", receivedResults.Evidence[0].Status)
	}
}
