package health

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"platform/sensor/internal/agentdrain"
	"platform/sensor/internal/control"
	"platform/sensor/internal/packsync"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
	"platform/sensor/internal/supportability"
)

type fakePackStatusProvider struct {
	status packsync.Status
}

func (f *fakePackStatusProvider) Snapshot() packsync.Status { return f.status }

type fakeSchedulerStatusProvider struct {
	snap scheduler.Snapshot
}

func (f *fakeSchedulerStatusProvider) Snapshot() scheduler.Snapshot { return f.snap }

type fakeControlStatusProvider struct {
	status control.Status
}

func (f *fakeControlStatusProvider) Snapshot() control.Status { return f.status }

type fakeRuntimeStatusProvider struct {
	health RuntimeHealth
}

func (f *fakeRuntimeStatusProvider) Snapshot() RuntimeHealth { return f.health }

type fakeStorageStatusProvider struct {
	pct float64
	err error
}

func (f *fakeStorageStatusProvider) CheckStorageUsage() (float64, error) {
	return f.pct, f.err
}

func trustedPack() *fakePackStatusProvider {
	return &fakePackStatusProvider{status: packsync.Status{
		TrustedApplied:           true,
		CoreAuthorityEstablished: true,
	}}
}

func inSyncControl() *fakeControlStatusProvider {
	return &fakeControlStatusProvider{status: control.Status{State: control.StateInSync}}
}

func runningScheduler() *fakeSchedulerStatusProvider {
	return &fakeSchedulerStatusProvider{snap: scheduler.Snapshot{
		State:             scheduler.StateRunning,
		ConfiguredWorkers: 2,
		QueueCapacity:     8,
	}}
}

func newTestHealth(t *testing.T, pack packSyncStatusProvider) *HealthServer {
	t.Helper()
	disk, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	hs := NewHealthServer(":0", disk, slog.Default(),
		WithIdentity("0.1.0-test", "sensor-instance-test"))
	if pack != nil {
		hs.SetPackSync(pack)
	}
	hs.SetScheduler(runningScheduler())
	hs.SetControl(inSyncControl())
	return hs
}

func request(t *testing.T, hs *HealthServer, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rr := httptest.NewRecorder()
	hs.server.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s decode: %v body=%s", path, err, rr.Body.String())
	}
	return rr, body
}

func requireKeys(t *testing.T, body map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := body[key]; !ok {
			t.Fatalf("missing %q in %v", key, body)
		}
	}
}

func requireOnlyKeys(t *testing.T, body map[string]any, allowed ...string) {
	t.Helper()
	set := map[string]bool{}
	for _, key := range allowed {
		set[key] = true
	}
	for key := range body {
		if !set[key] {
			t.Fatalf("unexpected key %q in %v", key, body)
		}
	}
}

func TestHealthAddressControlsBind(t *testing.T) {
	disk, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	hs := NewHealthServer("127.0.0.1:18088", disk, slog.Default())
	if got := hs.ListenAddr(); got != "127.0.0.1:18088" {
		t.Fatalf("ListenAddr=%q", got)
	}
}

func TestLivezContractAndShutdownState(t *testing.T) {
	hs := newTestHealth(t, trustedPack())

	rr, body := request(t, hs, "/livez")
	if rr.Code != http.StatusOK || body["status"] != "UP" {
		t.Fatalf("livez=%d body=%v", rr.Code, body)
	}
	requireOnlyKeys(t, body, "status", "component", "ts", "service_instance_id")
	if body["component"] != componentName || body["service_instance_id"] != "sensor-instance-test" {
		t.Fatalf("identity=%v", body)
	}
	if ts, _ := body["ts"].(string); !strings.HasSuffix(ts, "Z") {
		t.Fatalf("non-UTC ts=%q", ts)
	}

	hs.MarkShuttingDown()
	rr, body = request(t, hs, "/livez")
	if rr.Code != http.StatusOK || body["status"] != "SHUTTING_DOWN" {
		t.Fatalf("shutdown livez=%d body=%v", rr.Code, body)
	}
}

func TestReadyzIsCompactCanonicalEnvelope(t *testing.T) {
	hs := newTestHealth(t, trustedPack())
	rr, body := request(t, hs, "/readyz")
	if rr.Code != http.StatusOK {
		t.Fatalf("readyz=%d body=%v", rr.Code, body)
	}
	if body["status"] != "UP" || body["ready"] != true || body["http_semantic"] != float64(200) {
		t.Fatalf("ready matrix=%v", body)
	}
	requireOnlyKeys(t, body, "status", "ready", "http_semantic", "component", "ts", "service_instance_id")
}

func TestReadyzPackTrustAndCoreAuthorityFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		status packsync.Status
		reason string
	}{
		{"trust", packsync.Status{TrustedApplied: false}, reasonPackTrust},
		{"authority", packsync.Status{TrustedApplied: true, CoreAuthorityEstablished: false}, reasonPackAuthority},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hs := newTestHealth(t, &fakePackStatusProvider{status: tc.status})
			rr, body := request(t, hs, "/readyz")
			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("code=%d body=%v", rr.Code, body)
			}
			if body["status"] != "NOT_READY" || body["ready"] != false ||
				body["http_semantic"] != float64(503) || body["reason_code"] != tc.reason {
				t.Fatalf("body=%v", body)
			}
			requireOnlyKeys(t, body, "status", "ready", "http_semantic", "component", "ts", "service_instance_id", "reason_code")
		})
	}
}

func TestReadyzMissingPackProviderFailsClosed(t *testing.T) {
	hs := newTestHealth(t, nil)
	rr, body := request(t, hs, "/readyz")
	if rr.Code != http.StatusServiceUnavailable || body["reason_code"] != reasonPackUnavailable {
		t.Fatalf("code=%d body=%v", rr.Code, body)
	}
}

func TestReadyzTrustedPackDegradedRemainsReady(t *testing.T) {
	hs := newTestHealth(t, &fakePackStatusProvider{status: packsync.Status{
		TrustedApplied:           true,
		CoreAuthorityEstablished: true,
		Degraded:                 true,
	}})
	rr, body := request(t, hs, "/readyz")
	if rr.Code != http.StatusOK || body["status"] != "DEGRADED" ||
		body["ready"] != true || body["reason_code"] != reasonPackDegraded {
		t.Fatalf("code=%d body=%v", rr.Code, body)
	}
}

func TestReadyzPackFailurePrecedesSchedulerAndControlFailures(t *testing.T) {
	hs := newTestHealth(t, &fakePackStatusProvider{status: packsync.Status{
		TrustedApplied: false,
	}})
	hs.SetScheduler(nil)
	hs.SetControl(nil)
	rr, body := request(t, hs, "/readyz")
	if rr.Code != http.StatusServiceUnavailable || body["reason_code"] != reasonPackTrust {
		t.Fatalf("precedence code=%d body=%v", rr.Code, body)
	}
}

func TestReadyzSchedulerFailureDoesNotKillLiveness(t *testing.T) {
	hs := newTestHealth(t, trustedPack())
	hs.SetScheduler(&fakeSchedulerStatusProvider{snap: scheduler.Snapshot{State: scheduler.StateStopped}})
	rr, body := request(t, hs, "/readyz")
	if rr.Code != http.StatusServiceUnavailable || body["status"] != "NOT_READY" ||
		body["reason_code"] != reasonSchedulerState {
		t.Fatalf("readyz=%d body=%v", rr.Code, body)
	}
	rr, body = request(t, hs, "/livez")
	if rr.Code != http.StatusOK || body["status"] != "UP" {
		t.Fatalf("livez=%d body=%v", rr.Code, body)
	}
}

func TestReadyzQueueSaturationIsDegradedNotReady(t *testing.T) {
	hs := newTestHealth(t, trustedPack())
	hs.SetScheduler(&fakeSchedulerStatusProvider{snap: scheduler.Snapshot{
		State:         scheduler.StateRunning,
		QueueDepth:    8,
		QueueCapacity: 8,
	}})

	rr, body := request(t, hs, "/readyz")
	if rr.Code != http.StatusServiceUnavailable ||
		body["status"] != "DEGRADED" ||
		body["ready"] != false ||
		body["http_semantic"] != float64(503) ||
		body["reason_code"] != reasonSchedulerQueue {
		t.Fatalf("queue saturation readiness=%d body=%v", rr.Code, body)
	}

	rr, body = request(t, hs, "/livez")
	if rr.Code != http.StatusOK || body["status"] != "UP" {
		t.Fatalf("queue saturation must not kill liveness: %d body=%v", rr.Code, body)
	}

	rr, body = request(t, hs, "/health")
	deps, ok := body["dependency_status"].(map[string]any)
	if rr.Code != http.StatusServiceUnavailable || body["component_state"] != "DEGRADED" ||
		!ok || deps["scheduler"] != "DEGRADED" {
		t.Fatalf("queue saturation support evidence=%d body=%v", rr.Code, body)
	}
}

func TestReadyzPerPoolSaturationIsDegradedNotReady(t *testing.T) {
	hs := newTestHealth(t, trustedPack())
	hs.SetScheduler(&fakeSchedulerStatusProvider{snap: scheduler.Snapshot{
		State:         scheduler.StateRunning,
		QueueDepth:    8,
		QueueCapacity: 12,
		Pools: map[scheduler.ExecutionClass]scheduler.PoolSnapshot{
			scheduler.ClassNetworkPoll: {QueueDepth: 8, QueueCapacity: 8},
			scheduler.ClassStreaming:   {QueueDepth: 0, QueueCapacity: 4},
		},
	}})

	rr, body := request(t, hs, "/readyz")
	if rr.Code != http.StatusServiceUnavailable ||
		body["status"] != "DEGRADED" ||
		body["ready"] != false ||
		body["http_semantic"] != float64(503) ||
		body["reason_code"] != reasonSchedulerQueue {
		t.Fatalf("per-pool saturation readiness=%d body=%v", rr.Code, body)
	}

	rr, body = request(t, hs, "/health")
	deps, ok := body["dependency_status"].(map[string]any)
	if rr.Code != http.StatusServiceUnavailable || body["component_state"] != "DEGRADED" ||
		!ok || deps["scheduler"] != "DEGRADED" {
		t.Fatalf("per-pool saturation support evidence=%d body=%v", rr.Code, body)
	}
}

func TestReadyzControlStateMatrix(t *testing.T) {
	cases := []struct {
		name       string
		control    control.Status
		code       int
		status     string
		ready      bool
		reasonCode string
	}{
		{"in_sync", control.Status{State: control.StateInSync}, 200, "UP", true, ""},
		{"sync_pending", control.Status{State: control.StateSyncPending}, 200, "DEGRADED", true, reasonControlDegraded},
		{"degraded_core", control.Status{State: control.StateDegraded, LastCode: control.CodeCoreUnavailable}, 200, "DEGRADED", true, reasonControlDegraded},
		{"untrusted", control.Status{State: control.StateUntrusted, LastCode: control.CodeStateCorrupt}, 503, "NOT_READY", false, reasonControlUntrust},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hs := newTestHealth(t, trustedPack())
			hs.SetControl(&fakeControlStatusProvider{status: tc.control})
			rr, body := request(t, hs, "/readyz")
			if rr.Code != tc.code || body["status"] != tc.status || body["ready"] != tc.ready {
				t.Fatalf("code=%d body=%v", rr.Code, body)
			}
			if tc.reasonCode == "" {
				if _, ok := body["reason_code"]; ok {
					t.Fatalf("unexpected reason=%v", body["reason_code"])
				}
			} else if body["reason_code"] != tc.reasonCode {
				t.Fatalf("reason=%v want=%s", body["reason_code"], tc.reasonCode)
			}
		})
	}
}

func TestReadyzRuntimeHealthMatrix(t *testing.T) {
	cases := []struct {
		name       string
		runtime    *fakeRuntimeStatusProvider
		code       int
		status     string
		ready      bool
		reasonCode string
		dep        string
	}{
		{"absent_preserves_legacy", nil, 200, "UP", true, "", ""},
		{"ready", &fakeRuntimeStatusProvider{health: RuntimeHealth{State: "READY"}}, 200, "UP", true, "", "UP"},
		{"degraded", &fakeRuntimeStatusProvider{health: RuntimeHealth{State: "DEGRADED"}}, 503, "DEGRADED", false, reasonRuntimeDegraded, "DEGRADED"},
		{"failed", &fakeRuntimeStatusProvider{health: RuntimeHealth{State: "FAILED"}}, 503, "NOT_READY", false, reasonRuntimeNotReady, "DOWN"},
		{"stopped", &fakeRuntimeStatusProvider{health: RuntimeHealth{State: "STOPPED"}}, 503, "NOT_READY", false, reasonRuntimeNotReady, "DOWN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hs := newTestHealth(t, trustedPack())
			if tc.runtime != nil {
				hs.SetRuntime(tc.runtime)
			}
			rr, body := request(t, hs, "/readyz")
			if rr.Code != tc.code || body["status"] != tc.status || body["ready"] != tc.ready {
				t.Fatalf("readyz code=%d body=%v", rr.Code, body)
			}
			if tc.reasonCode == "" {
				if _, ok := body["reason_code"]; ok {
					t.Fatalf("unexpected reason=%v", body["reason_code"])
				}
			} else if body["reason_code"] != tc.reasonCode {
				t.Fatalf("reason=%v want=%s", body["reason_code"], tc.reasonCode)
			}
			rr, body = request(t, hs, "/health")
			deps, _ := body["dependency_status"].(map[string]any)
			if tc.dep == "" {
				if _, ok := deps["runtime"]; ok {
					t.Fatalf("legacy readiness must omit runtime dependency: %v", deps)
				}
			} else if deps["runtime"] != tc.dep {
				t.Fatalf("runtime dependency=%v want=%s body=%v", deps["runtime"], tc.dep, body)
			}
			rr, body = request(t, hs, "/livez")
			if rr.Code != http.StatusOK || body["status"] != "UP" {
				t.Fatalf("runtime health must not kill liveness: %d body=%v", rr.Code, body)
			}
		})
	}
}

func TestReadyzStorageProtectionAndAgentSpoolProtection(t *testing.T) {
	cases := []struct {
		name   string
		wire   func(*HealthServer)
		reason string
	}{
		{"sensor_spool_pressure", func(hs *HealthServer) {
			hs.spool = &fakeStorageStatusProvider{pct: 80}
		}, reasonSpoolPressure},
		{"sensor_spool_unwritable", func(hs *HealthServer) {
			hs.spool = &fakeStorageStatusProvider{err: errors.New("disk unavailable")}
		}, reasonSpoolCheck},
		{"agent_spool_pressure", func(hs *HealthServer) {
			hs.agentSpool = &fakeStorageStatusProvider{pct: 80}
		}, reasonAgentSpoolHard},
		{"agent_spool_unwritable", func(hs *HealthServer) {
			hs.agentSpool = &fakeStorageStatusProvider{err: errors.New("agent disk unavailable")}
		}, reasonAgentSpoolCheck},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hs := newTestHealth(t, trustedPack())
			tc.wire(hs)
			rr, body := request(t, hs, "/readyz")
			if rr.Code != http.StatusServiceUnavailable || body["status"] != "NOT_READY" ||
				body["reason_code"] != tc.reason {
				t.Fatalf("code=%d body=%v", rr.Code, body)
			}
			rr, body = request(t, hs, "/livez")
			if rr.Code != http.StatusOK || body["status"] != "UP" {
				t.Fatalf("livez=%d body=%v", rr.Code, body)
			}
		})
	}
}

func TestReadyzStorageBelowProtectionWatermarkRemainsReady(t *testing.T) {
	for _, pct := range []float64{79.5, 79.9} {
		t.Run(fmt.Sprintf("%.1f", pct), func(t *testing.T) {
			hs := newTestHealth(t, trustedPack())
			hs.spool = &fakeStorageStatusProvider{pct: pct}

			rr, body := request(t, hs, "/readyz")
			if rr.Code != http.StatusOK || body["status"] != "UP" || body["ready"] != true {
				t.Fatalf("below-watermark readiness=%d body=%v", rr.Code, body)
			}
			rr, body = request(t, hs, "/health")
			if rr.Code != http.StatusOK || body["storage_pressure_percent"] == float64(80) {
				t.Fatalf("below-watermark support evidence=%d body=%v", rr.Code, body)
			}
			if body["storage_pressure_percent"] != float64(79) {
				t.Fatalf("storage_pressure_percent=%v want=79", body["storage_pressure_percent"])
			}
		})
	}
}

func TestSupportEvidenceMarksHardStorageFailuresDown(t *testing.T) {
	cases := []struct {
		name string
		wire func(*HealthServer)
		key  string
	}{
		{"telemetry_pressure", func(hs *HealthServer) { hs.spool = &fakeStorageStatusProvider{pct: 80} }, "local_spool"},
		{"agent_pressure", func(hs *HealthServer) { hs.agentSpool = &fakeStorageStatusProvider{pct: 80} }, "agent_spool"},
		{"agent_check_failure", func(hs *HealthServer) {
			hs.agentSpool = &fakeStorageStatusProvider{err: errors.New("agent disk unavailable")}
		}, "agent_spool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hs := newTestHealth(t, trustedPack())
			tc.wire(hs)
			rr, body := request(t, hs, "/health")
			deps, ok := body["dependency_status"].(map[string]any)
			if rr.Code != http.StatusServiceUnavailable || !ok || deps[tc.key] != "DOWN" {
				t.Fatalf("support evidence=%d body=%v", rr.Code, body)
			}
		})
	}
}

func TestHealthAndHealthzAreBoundedSupportEvidence(t *testing.T) {
	hs := newTestHealth(t, trustedPack())
	for _, path := range []string{"/health", "/healthz"} {
		rr, body := request(t, hs, path)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s=%d body=%v", path, rr.Code, body)
		}
		requireKeys(t, body,
			"document_type", "component", "version", "service_instance_id", "ts",
			"uptime_seconds", "component_state", "dependency_status", "storage_pressure_percent",
			"spool_pressure_percent", "queue_depth", "queue_capacity", "scheduler_status",
			"control_reconciliation_status", "counters",
		)
		if body["document_type"] != "SUPPORT_EVIDENCE" || body["component"] != componentName ||
			body["component_state"] != "UP" {
			t.Fatalf("%s body=%v", path, body)
		}
		raw := strings.ToLower(rr.Body.String())
		for _, forbidden := range []string{
			"desired_generation", "applied_generation", "assignment_key", "job_id", "tenant_id",
			"payload", "password", "token", "secret", "private_key", "authorization",
		} {
			if strings.Contains(raw, forbidden) {
				t.Fatalf("%s leaked %q: %s", path, forbidden, rr.Body.String())
			}
		}
	}
}

func TestDetailedHealthFailClosedUsesSupportEvidenceNotReadinessBody(t *testing.T) {
	hs := newTestHealth(t, &fakePackStatusProvider{status: packsync.Status{TrustedApplied: false}})
	rr, body := request(t, hs, "/health")
	if rr.Code != http.StatusServiceUnavailable || body["document_type"] != "SUPPORT_EVIDENCE" ||
		body["component_state"] != "NOT_READY" {
		t.Fatalf("code=%d body=%v", rr.Code, body)
	}
	if _, exists := body["ready"]; exists {
		t.Fatalf("detailed support evidence must not duplicate readiness envelope: %v", body)
	}
	reasons, ok := body["reason_codes"].([]any)
	if !ok || len(reasons) != 1 || reasons[0] != reasonPackTrust {
		t.Fatalf("reason_codes=%v", body["reason_codes"])
	}
}

func TestRawDependencyErrorDoesNotLeakThroughHealthLogging(t *testing.T) {
	var logs bytes.Buffer
	level := new(slog.LevelVar)
	level.Set(slog.LevelDebug)
	logger := supportability.NewJSONLogger(&logs, level, "0.1.0-test", "sensor-instance-test").
		With("sensor_id", "sensor-net-01")

	hs := newTestHealth(t, trustedPack())
	hs.logger = logger
	hs.spool = &fakeStorageStatusProvider{err: errors.New("Bearer fictional-credential")}
	_, _ = request(t, hs, "/readyz")

	raw := strings.ToLower(logs.String())
	if strings.Contains(raw, "fictional-credential") || strings.Contains(raw, "\"error\"") {
		t.Fatalf("raw dependency error leaked: %s", logs.String())
	}
	if !strings.Contains(raw, "sensor_spool_storage_check_failed") {
		t.Fatalf("stable event code missing: %s", logs.String())
	}
}

func TestSupportEvidenceCarriesBoundedCountersAndTimestamps(t *testing.T) {
	packTime := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	controlTime := time.Now().UTC().Add(-30 * time.Second).Truncate(time.Second)
	hs := newTestHealth(t, &fakePackStatusProvider{status: packsync.Status{
		TrustedApplied:           true,
		CoreAuthorityEstablished: true,
		LastSuccessfulSync:       packTime,
	}})
	hs.SetScheduler(&fakeSchedulerStatusProvider{snap: scheduler.Snapshot{
		State:             scheduler.StateRunning,
		QueueDepth:        3,
		QueueCapacity:     8,
		Submitted:         10,
		Completed:         7,
		Failed:            1,
		Retries:           2,
		RejectedQueueFull: 1,
	}})
	hs.SetControl(&fakeControlStatusProvider{status: control.Status{
		State:                   control.StateInSync,
		LastSuccessfulReconcile: controlTime,
		ReconcileAttempts:       9,
		AppliedCount:            7,
		RejectedCount:           1,
		CoreTransportFailures:   1,
	}})

	_, body := request(t, hs, "/health")
	counters, ok := body["counters"].(map[string]any)
	if !ok || counters["scheduler_submitted"] != float64(10) ||
		counters["control_reconcile_attempts"] != float64(9) {
		t.Fatalf("counters=%v", body["counters"])
	}
	stamps, ok := body["state_timestamps"].(map[string]any)
	if !ok || stamps["pack_last_successful_sync"] == "" ||
		stamps["control_last_successful_reconcile"] == "" {
		t.Fatalf("state_timestamps=%v", body["state_timestamps"])
	}
}

type fakeAgentDeliveryStatusProvider struct {
	status agentdrain.DeliveryStatus
}

func (f *fakeAgentDeliveryStatusProvider) Snapshot() agentdrain.DeliveryStatus { return f.status }

func TestAgentDeliveryBlockedDegradesWhileAgentSpoolRemainsUp(t *testing.T) {
	hs := newTestHealth(t, trustedPack())
	hs.agentSpool = &fakeStorageStatusProvider{pct: 10}
	hs.SetAgentDelivery(&fakeAgentDeliveryStatusProvider{status: agentdrain.DeliveryStatus{
		State:          agentdrain.DeliveryStateDegraded,
		Blocked:        true,
		ReasonCode:     "ERR_BATCH_TOO_LARGE",
		PendingRecords: 1,
		PendingBytes:   128,
		FailuresPerm:   3,
		BlockedSince:   time.Now().UTC().Add(-time.Minute),
	}})

	rr, body := request(t, hs, "/readyz")
	if rr.Code != http.StatusOK || body["status"] != "DEGRADED" || body["ready"] != true ||
		body["reason_code"] != reasonAgentDeliveryBlocked {
		t.Fatalf("readyz must stay ready while delivery degrades: %d %v", rr.Code, body)
	}

	rr, body = request(t, hs, "/health")
	deps, ok := body["dependency_status"].(map[string]any)
	if !ok {
		t.Fatalf("deps missing: %v", body)
	}
	if rr.Code != http.StatusOK || body["component_state"] != "DEGRADED" ||
		deps["agent_spool"] != "UP" || deps["agent_delivery"] != "DEGRADED" {
		t.Fatalf("delivery vs spool split failed: %d body=%v", rr.Code, body)
	}
	if body["agent_delivery_status"] != "DEGRADED" || body["agent_delivery_reason_code"] != "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("delivery support fields=%v", body)
	}
	counters, _ := body["counters"].(map[string]any)
	if counters["agent_delivery_pending_records"] != float64(1) ||
		counters["agent_delivery_failures_permanent"] != float64(3) {
		t.Fatalf("delivery counters=%v", counters)
	}
	stamps, _ := body["state_timestamps"].(map[string]any)
	if stamps["agent_delivery_blocked_since"] == nil || stamps["agent_delivery_blocked_since"] == "" {
		t.Fatalf("blocked_since missing: %v", stamps)
	}

	raw := strings.ToLower(rr.Body.String())
	for _, forbidden := range []string{"device_down", "password", "token", "secret", "bearer", "payload"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("leaked %q: %s", forbidden, rr.Body.String())
		}
	}
}

func TestAgentDeliveryClearsOnlyWhenProviderReportsUp(t *testing.T) {
	hs := newTestHealth(t, trustedPack())
	provider := &fakeAgentDeliveryStatusProvider{status: agentdrain.DeliveryStatus{
		State: agentdrain.DeliveryStateDegraded, Blocked: true, ReasonCode: "ERR_BATCH_TOO_LARGE",
	}}
	hs.SetAgentDelivery(provider)
	rr, body := request(t, hs, "/health")
	deps := body["dependency_status"].(map[string]any)
	if body["component_state"] != "DEGRADED" || deps["agent_delivery"] != "DEGRADED" {
		t.Fatalf("blocked=%v", body)
	}

	provider.status = agentdrain.DeliveryStatus{
		State: agentdrain.DeliveryStateUp, DurableAcks: 1, LastSuccessTime: time.Now().UTC(),
	}
	rr, body = request(t, hs, "/health")
	deps = body["dependency_status"].(map[string]any)
	if rr.Code != http.StatusOK || body["component_state"] != "UP" || deps["agent_delivery"] != "UP" {
		t.Fatalf("recovery=%d body=%v", rr.Code, body)
	}
	if _, ok := body["agent_delivery_reason_code"]; ok {
		t.Fatalf("reason must clear: %v", body["agent_delivery_reason_code"])
	}
}

func TestDisabledAgentReceiverOmitsDeliveryDependency(t *testing.T) {
	hs := newTestHealth(t, trustedPack())
	hs.SetAgentDelivery(nil)
	rr, body := request(t, hs, "/health")
	deps, ok := body["dependency_status"].(map[string]any)
	if rr.Code != http.StatusOK || !ok {
		t.Fatalf("health=%d body=%v", rr.Code, body)
	}
	if _, exists := deps["agent_delivery"]; exists {
		t.Fatalf("disabled receiver invented agent_delivery: %v", deps)
	}
	if _, exists := deps["agent_spool"]; exists {
		t.Fatalf("disabled receiver invented agent_spool: %v", deps)
	}
	if _, exists := body["agent_delivery_status"]; exists {
		t.Fatalf("disabled receiver invented agent_delivery_status: %v", body)
	}
	if body["component_state"] != "UP" {
		t.Fatalf("false degraded: %v", body["component_state"])
	}
}

func TestNilTypedDrainProviderDoesNotDegrade(t *testing.T) {
	hs := newTestHealth(t, trustedPack())
	var drain *agentdrain.Drain
	hs.SetAgentDelivery(drain)
	rr, body := request(t, hs, "/health")
	deps := body["dependency_status"].(map[string]any)
	if rr.Code != http.StatusOK || body["component_state"] != "UP" {
		t.Fatalf("nil typed drain degraded health: %d %v", rr.Code, body)
	}
	if _, exists := deps["agent_delivery"]; exists {
		t.Fatalf("nil typed drain invented dependency: %v", deps)
	}
}

func TestAgentDeliveryDoesNotReportDeviceDown(t *testing.T) {
	hs := newTestHealth(t, trustedPack())
	hs.agentSpool = &fakeStorageStatusProvider{pct: 5}
	hs.SetAgentDelivery(&fakeAgentDeliveryStatusProvider{status: agentdrain.DeliveryStatus{
		State: agentdrain.DeliveryStateDegraded, Blocked: true, ReasonCode: "ERR_BATCH_TOO_LARGE",
	}})
	for _, path := range []string{"/health", "/readyz", "/livez"} {
		rr, body := request(t, hs, path)
		raw := strings.ToUpper(rr.Body.String())
		if strings.Contains(raw, "DEVICE_DOWN") {
			t.Fatalf("%s reported DEVICE_DOWN: %s", path, rr.Body.String())
		}
		if path == "/livez" && (rr.Code != http.StatusOK || body["status"] != "UP") {
			t.Fatalf("liveness changed by delivery block: %d %v", rr.Code, body)
		}
	}
}

func TestAgentDeliveryConcurrentHealthReads(t *testing.T) {
	hs := newTestHealth(t, trustedPack())
	hs.SetAgentDelivery(&fakeAgentDeliveryStatusProvider{status: agentdrain.DeliveryStatus{
		State: agentdrain.DeliveryStateDegraded, Blocked: true, ReasonCode: "ERR_REQUEST_TOO_LARGE",
		PendingRecords: 2,
	}})
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 40; j++ {
				rr, body := request(t, hs, "/health")
				deps, _ := body["dependency_status"].(map[string]any)
				if rr.Code != http.StatusOK || body["component_state"] != "DEGRADED" ||
					deps["agent_delivery"] != "DEGRADED" {
					t.Errorf("concurrent health=%d body=%v", rr.Code, body)
					return
				}
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

// Simultaneous Agent-delivery block + ICMP/runtime degradation must preserve both
// dependency surfaces, withhold readiness for runtime only, and recover independently.
func TestAgentDeliveryAndRuntimeDegradeIndependently(t *testing.T) {
	hs := newTestHealth(t, trustedPack())
	hs.agentSpool = &fakeStorageStatusProvider{pct: 10}
	delivery := &fakeAgentDeliveryStatusProvider{status: agentdrain.DeliveryStatus{
		State: agentdrain.DeliveryStateDegraded, Blocked: true, ReasonCode: "ERR_BATCH_TOO_LARGE",
	}}
	runtime := &fakeRuntimeStatusProvider{health: RuntimeHealth{State: "DEGRADED"}}
	hs.SetAgentDelivery(delivery)
	hs.SetRuntime(runtime)

	rr, body := request(t, hs, "/readyz")
	if rr.Code != http.StatusServiceUnavailable || body["status"] != "DEGRADED" ||
		body["ready"] != false || body["reason_code"] != reasonRuntimeDegraded {
		t.Fatalf("simultaneous must withhold readiness for runtime: %d %v", rr.Code, body)
	}
	rr, body = request(t, hs, "/health")
	deps, _ := body["dependency_status"].(map[string]any)
	if deps["agent_delivery"] != "DEGRADED" || deps["runtime"] != "DEGRADED" ||
		deps["agent_spool"] != "UP" {
		t.Fatalf("both deps must stay visible: %v", deps)
	}
	raw := strings.ToUpper(rr.Body.String())
	if strings.Contains(raw, "DEVICE_DOWN") {
		t.Fatalf("false DEVICE_DOWN: %s", rr.Body.String())
	}
	rr, body = request(t, hs, "/livez")
	if rr.Code != http.StatusOK || body["status"] != "UP" {
		t.Fatalf("liveness must stay up: %d %v", rr.Code, body)
	}

	// Recover runtime first; Agent delivery remains blocked → ready but DEGRADED.
	runtime.health = RuntimeHealth{State: "READY"}
	rr, body = request(t, hs, "/readyz")
	if rr.Code != http.StatusOK || body["status"] != "DEGRADED" || body["ready"] != true ||
		body["reason_code"] != reasonAgentDeliveryBlocked {
		t.Fatalf("runtime recovery with delivery blocked: %d %v", rr.Code, body)
	}
	rr, body = request(t, hs, "/health")
	deps, _ = body["dependency_status"].(map[string]any)
	if deps["runtime"] != "UP" || deps["agent_delivery"] != "DEGRADED" {
		t.Fatalf("independent runtime recovery: %v", deps)
	}

	// Re-block runtime while clearing delivery → not ready from runtime alone.
	runtime.health = RuntimeHealth{State: "DEGRADED"}
	delivery.status = agentdrain.DeliveryStatus{
		State: agentdrain.DeliveryStateUp, DurableAcks: 1, LastSuccessTime: time.Now().UTC(),
	}
	rr, body = request(t, hs, "/readyz")
	if rr.Code != http.StatusServiceUnavailable || body["status"] != "DEGRADED" ||
		body["ready"] != false || body["reason_code"] != reasonRuntimeDegraded {
		t.Fatalf("delivery recovery with runtime degraded: %d %v", rr.Code, body)
	}
	rr, body = request(t, hs, "/health")
	deps, _ = body["dependency_status"].(map[string]any)
	if deps["agent_delivery"] != "UP" || deps["runtime"] != "DEGRADED" {
		t.Fatalf("independent delivery recovery: %v", deps)
	}

	// Full recovery.
	runtime.health = RuntimeHealth{State: "READY"}
	rr, body = request(t, hs, "/readyz")
	if rr.Code != http.StatusOK || body["status"] != "UP" || body["ready"] != true {
		t.Fatalf("full recovery readyz: %d %v", rr.Code, body)
	}
	if _, ok := body["reason_code"]; ok {
		t.Fatalf("reason must clear after full recovery: %v", body["reason_code"])
	}
}
