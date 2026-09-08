package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
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
	if m.probeFn != nil { return m.probeFn(ctx, target, timeout) }
	return true, 1200, "ICMP_ECHO_REPLY", nil
}

type mockSNMPProber struct {
	probeFn func(ctx context.Context, target string, port int, timeout time.Duration, credentialProfileRef string) (map[string]any, string, error)
}

func (m *mockSNMPProber) ProbeIdentity(ctx context.Context, target string, port int, timeout time.Duration, credentialProfileRef string) (map[string]any, string, error) {
	if m.probeFn != nil { return m.probeFn(ctx, target, port, timeout, credentialProfileRef) }
	return map[string]any{
		"sys_descr": "Test network device", "sys_object_id": "1.3.6.1.4.1.9.1.1745", "sys_name": "switch-test-01",
	}, "SNMP_GET_RESPONSE", nil
}

type mockCoreDiscoveryClient struct {
	fetchTaskFn     func(ctx context.Context) (coreclient.DiscoveryTaskResponse, error)
	submitResultsFn func(ctx context.Context, results coreclient.DiscoveryResultSubmission, correlationID string) (coreclient.DiscoveryResultAck, error)
}

func (m *mockCoreDiscoveryClient) FetchDiscoveryTask(ctx context.Context) (coreclient.DiscoveryTaskResponse, error) {
	if m.fetchTaskFn != nil { return m.fetchTaskFn(ctx) }
	return coreclient.DiscoveryTaskResponse{HasTask: false}, nil
}

func (m *mockCoreDiscoveryClient) SubmitDiscoveryResults(ctx context.Context, results coreclient.DiscoveryResultSubmission, correlationID string) (coreclient.DiscoveryResultAck, error) {
	if m.submitResultsFn != nil { return m.submitResultsFn(ctx, results, correlationID) }
	return coreclient.DiscoveryResultAck{
		Status: "ACCEPTED", RequestID: results.RequestID, SensorIdentity: "sensor-test-01",
		ReceivedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func TestExecutor_Execute_InvalidJobClass(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 5, nil)
	_, err := exec.Execute(context.Background(), scheduler.Job{Class: scheduler.ClassNetworkPoll, Payload: []byte(`{}`)})
	if err == nil || !scheduler.IsCode(err, scheduler.CodePermanent) { t.Fatalf("expected permanent invalid-class error, got %v", err) }
}

func TestExecutor_Execute_MalformedPayload(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 5, nil)
	_, err := exec.Execute(context.Background(), scheduler.Job{Class: scheduler.ClassDiscovery, Payload: []byte(`{invalid json`)})
	if err == nil || !scheduler.IsCode(err, scheduler.CodePermanent) { t.Fatalf("expected permanent malformed-payload error, got %v", err) }
}

func TestExecutor_Run_ICMPSuccessRequiresProberSuccess(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 2, nil)
	exec.SetProbers(&mockICMPProber{probeFn: func(context.Context, string, time.Duration) (bool, int64, string, error) {
		return true, 1500, "ICMP_ECHO_REPLY", nil
	}}, nil)
	res := exec.Run(context.Background(), taskFor("req-1", []string{"ICMP"}, "192.168.1.1"))
	if len(res.Evidence) != 1 || res.Evidence[0].Status != "SUCCESS" || res.Evidence[0].ReachabilityKind != "OBSERVED" {
		t.Fatalf("unexpected ICMP success evidence: %+v", res.Evidence)
	}
}

func TestExecutor_Run_NoSocketFallbackCanTurnIcmpFailureIntoSuccess(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 2, nil)
	exec.SetProbers(&mockICMPProber{probeFn: func(context.Context, string, time.Duration) (bool, int64, string, error) {
		return false, 0, "ICMP_TIMEOUT", errors.New("no echo reply")
	}}, nil)
	res := exec.Run(context.Background(), taskFor("req-no-fallback", []string{"ICMP"}, "192.168.1.2"))
	if len(res.Evidence) != 1 { t.Fatalf("expected one evidence item, got %d", len(res.Evidence)) }
	ev := res.Evidence[0]
	if ev.Status != "INCONCLUSIVE" || ev.ReachabilityKind != "UNKNOWN" || ev.TechnicalCode != "ICMP_TIMEOUT" {
		t.Fatalf("ICMP failure was promoted or misclassified: %+v", ev)
	}
}

func TestExecutor_Run_SNMPTimeoutRemainsInconclusiveUnknown(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 2, nil)
	exec.SetProbers(nil, &mockSNMPProber{probeFn: func(context.Context, string, int, time.Duration, string) (map[string]any, string, error) {
		return nil, "SNMP_TIMEOUT", errors.New("timeout")
	}})
	task := taskFor("req-snmp-timeout", []string{"SNMP"}, "192.168.1.3")
	task.CredentialProfileRef = "cred-profile-1"
	res := exec.Run(context.Background(), task)
	if len(res.Evidence) != 1 { t.Fatalf("expected one evidence item, got %d", len(res.Evidence)) }
	ev := res.Evidence[0]
	if ev.Status != "INCONCLUSIVE" || ev.ReachabilityKind != "UNKNOWN" || ev.TechnicalCode != "SNMP_TIMEOUT" {
		t.Fatalf("SNMP timeout was causally over-classified: %+v", ev)
	}
	for _, forbidden := range []string{"DEVICE_DOWN", "FIREWALL_BLOCKED", "WRONG_CREDENTIAL"} {
		if strings.Contains(strings.ToUpper(ev.TechnicalCode), forbidden) { t.Fatalf("prohibited causal code %s", ev.TechnicalCode) }
	}
}

func TestExecutor_DefaultSNMPFailsClosedWithoutCredentialResolver(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 2, nil)
	task := taskFor("req-snmp-blocked", []string{"SNMP"}, "192.168.1.4")
	task.CredentialProfileRef = "cred-profile-1"
	res := exec.Run(context.Background(), task)
	if len(res.Evidence) != 1 { t.Fatalf("expected one evidence item, got %d", len(res.Evidence)) }
	ev := res.Evidence[0]
	if ev.Status != "INCONCLUSIVE" || ev.ReachabilityKind != "UNKNOWN" || ev.TechnicalCode != "SNMP_CREDENTIAL_RESOLVER_UNAVAILABLE" {
		t.Fatalf("default SNMP path did not fail closed: %+v", ev)
	}
}

func TestExecutor_SNMPReceivesOnlyOpaqueCredentialProfileReference(t *testing.T) {
	var receivedRef string
	exec := NewExecutor("sensor-test-01", 2, nil)
	exec.SetProbers(nil, &mockSNMPProber{probeFn: func(_ context.Context, _ string, _ int, _ time.Duration, credentialProfileRef string) (map[string]any, string, error) {
		receivedRef = credentialProfileRef
		return map[string]any{"sys_name": "device-1"}, "SNMP_GET_RESPONSE", nil
	}})
	task := taskFor("req-snmp-ref", []string{"SNMP"}, "192.168.1.5")
	task.CredentialProfileRef = "cred-profile-opaque-42"
	res := exec.Run(context.Background(), task)
	if receivedRef != task.CredentialProfileRef { t.Fatalf("credential profile ref not preserved: %q", receivedRef) }
	encoded, _ := json.Marshal(res.Evidence)
	if strings.Contains(string(encoded), task.CredentialProfileRef) { t.Fatalf("credential profile reference leaked into evidence: %s", encoded) }
}

func TestExecutor_Run_BoundedConcurrencyAndDeterministicOrdering(t *testing.T) {
	var maxActive int32
	var currentActive int32
	exec := NewExecutor("sensor-test-01", 3, nil)
	exec.SetProbers(&mockICMPProber{probeFn: func(_ context.Context, target string, _ time.Duration) (bool, int64, string, error) {
		active := atomic.AddInt32(&currentActive, 1)
		for {
			oldMax := atomic.LoadInt32(&maxActive)
			if active <= oldMax || atomic.CompareAndSwapInt32(&maxActive, oldMax, active) { break }
		}
		time.Sleep(15 * time.Millisecond)
		atomic.AddInt32(&currentActive, -1)
		return true, 500, "ICMP_ECHO_REPLY", nil
	}}, nil)
	targets := make([]Target, 10)
	for i := 0; i < 10; i++ { targets[i] = Target{Address: fmt.Sprintf("192.168.1.%d", i+1), Type: "IPV4"} }
	res := exec.Run(context.Background(), Task{TaskID: "task-concurrency", RequestID: "req-concurrency", TenantID: "tenant-alpha", Targets: targets, RequestedMethods: []string{"ICMP"}, Timeout: time.Second})
	if len(res.Evidence) != 10 { t.Fatalf("expected 10 evidence items, got %d", len(res.Evidence)) }
	if maxActive > 3 { t.Fatalf("expected max 3 concurrent probes, observed %d", maxActive) }
	for i, ev := range res.Evidence {
		if ev.TargetAddress != targets[i].Address { t.Fatalf("evidence order changed at %d: %s", i, ev.TargetAddress) }
	}
}

func TestExecutor_Run_Cancellation(t *testing.T) {
	exec := NewExecutor("sensor-test-01", 1, nil)
	exec.SetProbers(&mockICMPProber{probeFn: func(ctx context.Context, _ string, _ time.Duration) (bool, int64, string, error) {
		select {
		case <-ctx.Done(): return false, 0, "CANCELLED", ctx.Err()
		case <-time.After(100 * time.Millisecond): return true, 100, "ICMP_ECHO_REPLY", nil
		}
	}}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(15*time.Millisecond, cancel)
	res := exec.Run(ctx, Task{TaskID: "task-cancel", RequestID: "req-cancel", TenantID: "tenant-alpha", Targets: []Target{{Address: "192.168.1.1"}, {Address: "192.168.1.2"}}, RequestedMethods: []string{"ICMP"}, Timeout: time.Second})
	if res.Status != "FAILED" { t.Fatalf("expected FAILED on cancellation, got %s", res.Status) }
}

func TestResolvedDestinationSafetyPredicate(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "0.0.0.0", "::", "::1", "::ffff:127.0.0.1", "169.254.169.254", "fe80::1", "224.0.0.1", "ff02::1"} {
		ip := net.ParseIP(raw)
		if ip == nil { t.Fatalf("test IP did not parse: %s", raw) }
		if !prohibitedIP(ip) { t.Fatalf("prohibited resolved destination accepted: %s", raw) }
	}
	for _, raw := range []string{"10.0.0.1", "192.168.1.1", "fd00::1"} {
		if prohibitedIP(net.ParseIP(raw)) { t.Fatalf("private monitoring target was prohibited: %s", raw) }
	}
}

func TestService_PollOncePreservesFenceAndResultWireHasNoSensorOrTenantAuthority(t *testing.T) {
	var received coreclient.DiscoveryResultSubmission
	mockClient := &mockCoreDiscoveryClient{
		fetchTaskFn: func(context.Context) (coreclient.DiscoveryTaskResponse, error) {
			return coreclient.DiscoveryTaskResponse{
				Status: "ASSIGNED", HasTask: true, RequestID: "018f0e0a-0000-7000-8000-000000000123",
				TenantID: "tenant-1", CorrelationID: "corr-123", AssignmentVersion: 7,
				Targets: []string{"192.168.1.50"}, RequestedMethods: []string{"ICMP"}, TimeoutMS: 1000,
			}, nil
		},
		submitResultsFn: func(_ context.Context, results coreclient.DiscoveryResultSubmission, _ string) (coreclient.DiscoveryResultAck, error) {
			received = results
			return coreclient.DiscoveryResultAck{Status: "ACCEPTED", RequestID: results.RequestID, SensorIdentity: "sensor-test-01"}, nil
		},
	}
	exec := NewExecutor("sensor-test-01", 2, nil)
	exec.SetProbers(&mockICMPProber{}, nil)
	svc := NewService("sensor-test-01", mockClient, exec, 100*time.Millisecond, nil)
	if err := svc.PollOnce(context.Background()); err != nil { t.Fatalf("PollOnce failed: %v", err) }
	if received.AssignmentVersion != 7 { t.Fatalf("assignment fence lost: %d", received.AssignmentVersion) }
	if len(received.Evidence) != 1 || received.Evidence[0].Status != "SUCCESS" { t.Fatalf("unexpected result: %+v", received) }
	wire, err := json.Marshal(received); if err != nil { t.Fatal(err) }
	if strings.Contains(string(wire), "sensor_id") || strings.Contains(string(wire), "tenant_id") {
		t.Fatalf("result submission contains client authority: %s", wire)
	}
}

func TestService_PollOnceRejectsMissingAssignmentFence(t *testing.T) {
	mockClient := &mockCoreDiscoveryClient{fetchTaskFn: func(context.Context) (coreclient.DiscoveryTaskResponse, error) {
		return coreclient.DiscoveryTaskResponse{HasTask: true, RequestID: "req-no-fence", TenantID: "tenant-1", Targets: []string{"192.168.1.1"}, RequestedMethods: []string{"ICMP"}}, nil
	}}
	svc := NewService("sensor-test-01", mockClient, NewExecutor("sensor-test-01", 1, nil), time.Second, nil)
	if err := svc.PollOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "DISCOVERY_TASK_CONTRACT_INVALID") {
		t.Fatalf("expected missing-fence rejection, got %v", err)
	}
}

func taskFor(requestID string, methods []string, address string) Task {
	return Task{
		TaskID: requestID, RequestID: requestID, TenantID: "tenant-alpha", CorrelationID: "corr-1",
		AssignmentVersion: 1, Targets: []Target{{Address: address, Type: targetType(address)}},
		RequestedMethods: methods, SNMPPort: 161, Timeout: 500 * time.Millisecond,
	}
}
