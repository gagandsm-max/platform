package health

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestReadyzStorageProtectionAndAgentSpoolProtection(t *testing.T) {
	cases := []struct {
		name   string
		wire   func(*HealthServer)
		reason string
	}{
		{"sensor_spool_pressure", func(hs *HealthServer) {
			hs.spool = &fakeStorageStatusProvider{pct: 96}
		}, reasonSpoolPressure},
		{"sensor_spool_unwritable", func(hs *HealthServer) {
			hs.spool = &fakeStorageStatusProvider{err: errors.New("disk unavailable")}
		}, reasonSpoolCheck},
		{"agent_spool_pressure", func(hs *HealthServer) {
			hs.agentSpool = &fakeStorageStatusProvider{pct: 96}
		}, reasonAgentSpoolHard},
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
