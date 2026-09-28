package health

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"platform/sensor/internal/packsync"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
)

type fakePackStatusProvider struct {
	status packsync.Status
}

func (f *fakePackStatusProvider) Snapshot() packsync.Status {
	return f.status
}

type fakeSchedulerStatusProvider struct {
	snap scheduler.Snapshot
}

func (f *fakeSchedulerStatusProvider) Snapshot() scheduler.Snapshot {
	return f.snap
}

func runningScheduler() *fakeSchedulerStatusProvider {
	return &fakeSchedulerStatusProvider{
		snap: scheduler.Snapshot{
			State:             scheduler.StateRunning,
			ConfiguredWorkers: 2,
			QueueCapacity:     8,
		},
	}
}

func newTestHealth(t *testing.T, syncer packSyncStatusProvider) *HealthServer {
	t.Helper()
	disk, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	hs := NewHealthServer(":0", disk, slog.Default())
	if syncer != nil {
		hs.SetPackSync(syncer)
	}
	hs.SetScheduler(runningScheduler())
	return hs
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

func TestLivezOKWhenPackTrustMissing(t *testing.T) {
	hs := newTestHealth(t, &fakePackStatusProvider{
		status: packsync.Status{
			TrustedApplied:           false,
			CoreAuthorityEstablished: false,
		},
	})

	rr := httptest.NewRecorder()
	hs.handleLive(rr, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("livez status=%d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "UP" {
		t.Fatalf("livez body=%v", body)
	}
}

func TestReadyzFailClosedWhenPackTrustMissing(t *testing.T) {
	hs := newTestHealth(t, &fakePackStatusProvider{
		status: packsync.Status{
			TrustedApplied:           false,
			CoreAuthorityEstablished: false,
		},
	})

	rr := httptest.NewRecorder()
	hs.handleReady(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "PACK_TRUST_NOT_ESTABLISHED" {
		t.Fatalf("expected PACK_TRUST_NOT_ESTABLISHED, got %v", body["status"])
	}
	pack, _ := body["capability_pack_sync"].(map[string]any)
	if pack == nil {
		t.Fatal("missing capability_pack_sync")
	}
	if pack["trusted_applied"] != false {
		t.Fatalf("trusted_applied=%v", pack["trusted_applied"])
	}
	raw := rr.Body.String()
	for _, forbidden := range []string{"password", "token", "secret", "private_key"} {
		if strings.Contains(strings.ToLower(raw), forbidden) {
			t.Fatalf("health payload must not expose %q", forbidden)
		}
	}
	if _, ok := pack["checksum"]; ok {
		t.Fatal("must not expose checksum in health payload")
	}
}

func TestReadyzRemainsReadyOnTransientWithTrustedApplied(t *testing.T) {
	hs := newTestHealth(t, &fakePackStatusProvider{
		status: packsync.Status{
			AppliedGeneration:        0,
			DesiredGeneration:        0,
			TrustedApplied:           true,
			CoreAuthorityEstablished: true,
			LastErrorCode:            packsync.ErrCodeSyncTransient,
			Degraded:                 false,
		},
	})

	rr := httptest.NewRecorder()
	hs.handleReady(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("transient with trusted applied must stay ready: %d %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	pack := body["capability_pack_sync"].(map[string]any)
	if pack["trusted_applied"] != true {
		t.Fatalf("expected trusted_applied true: %v", pack)
	}
	if pack["core_authority_established"] != true {
		t.Fatalf("expected core_authority_established true: %v", pack)
	}
	if pack["last_error_code"] != packsync.ErrCodeSyncTransient {
		t.Fatalf("expected transient error code retained: %v", pack)
	}
}

func TestReadyzNotReadyFreshInstallBeforeCoreAuthority(t *testing.T) {
	hs := newTestHealth(t, &fakePackStatusProvider{
		status: packsync.Status{
			AppliedGeneration:        0,
			DesiredGeneration:        0,
			TrustedApplied:           true,
			CoreAuthorityEstablished: false,
		},
	})

	rrLive := httptest.NewRecorder()
	hs.handleLive(rrLive, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if rrLive.Code != http.StatusOK {
		t.Fatalf("livez must stay UP: %d", rrLive.Code)
	}

	rr := httptest.NewRecorder()
	hs.handleReady(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz must be non-2xx before Core authority: %d %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "PACK_CORE_AUTHORITY_NOT_ESTABLISHED" {
		t.Fatalf("expected PACK_CORE_AUTHORITY_NOT_ESTABLISHED, got %v", body["status"])
	}
	pack := body["capability_pack_sync"].(map[string]any)
	if pack["trusted_applied"] != true || pack["core_authority_established"] != false {
		t.Fatalf("expected local trust without Core authority: %v", pack)
	}
}

func TestReadyzAuthDegradedWithTrustedStaysReady(t *testing.T) {
	hs := newTestHealth(t, &fakePackStatusProvider{
		status: packsync.Status{
			AppliedGeneration:        0,
			DesiredGeneration:        0,
			TrustedApplied:           true,
			CoreAuthorityEstablished: true,
			LastErrorCode:            packsync.ErrCodeAuthFailed,
			Degraded:                 true,
		},
	})

	rr := httptest.NewRecorder()
	hs.handleReady(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("auth degraded with trusted applied must stay ready: %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "DEGRADED" {
		t.Fatalf("expected DEGRADED status text, got %v", body["status"])
	}
}

func TestReadyzRecoversAfterTrustedStateRestored(t *testing.T) {
	provider := &fakePackStatusProvider{
		status: packsync.Status{
			TrustedApplied:           false,
			CoreAuthorityEstablished: false,
		},
	}
	hs := newTestHealth(t, provider)

	rr := httptest.NewRecorder()
	hs.handleReady(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected fail-closed before restore: %d", rr.Code)
	}

	provider.status = packsync.Status{
		AppliedGeneration:        1,
		DesiredGeneration:        1,
		PackCount:                1,
		TrustedApplied:           true,
		CoreAuthorityEstablished: true,
	}

	rr2 := httptest.NewRecorder()
	hs.handleReady(rr2, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected ready after trust restored: %d %s", rr2.Code, rr2.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr2.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	pack := body["capability_pack_sync"].(map[string]any)
	if pack["trusted_applied"] != true {
		t.Fatalf("trusted_applied not restored: %v", pack)
	}
}

func TestHealthAlsoFailClosedWhenPackTrustMissing(t *testing.T) {
	hs := newTestHealth(t, &fakePackStatusProvider{
		status: packsync.Status{
			TrustedApplied:           false,
			CoreAuthorityEstablished: false,
		},
	})
	rr := httptest.NewRecorder()
	hs.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("/health must fail closed on missing pack trust: %d", rr.Code)
	}
}

func TestHealthServerRealSyncerCompatibility(t *testing.T) {
	var syncer *packsync.Syncer
	hs := newTestHealth(t, syncer)
	if hs.packSync != nil {
		t.Fatal("expected nil packSync when passing nil syncer")
	}
}

func TestHealthReadyWithRunningScheduler(t *testing.T) {
	hs := newTestHealth(t, &fakePackStatusProvider{
		status: packsync.Status{
			TrustedApplied:           true,
			CoreAuthorityEstablished: true,
		},
	})

	for _, path := range []string{"/health", "/readyz"} {
		rr := httptest.NewRecorder()
		hs.handleHealth(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		sched, ok := body["scheduler"].(map[string]any)
		if !ok || sched == nil {
			t.Fatalf("%s missing scheduler payload: %v", path, body)
		}
		for _, key := range []string{
			"state", "configured_workers", "active_workers", "queue_depth", "queue_capacity",
			"tracked_assignments", "tracked_jobs", "highest_observed_fencing_version",
			"highest_accepted_fencing_version", "submitted", "accepted", "completed", "failed",
			"retries", "rejected_queue_full", "rejected_stale_lease", "rejected_tracking_capacity",
			"expired", "cancelled", "deadline_exceeded",
		} {
			if _, present := sched[key]; !present {
				t.Fatalf("%s missing scheduler field %q: %v", path, key, sched)
			}
		}
		raw := rr.Body.String()
		for _, forbidden := range []string{"password", "token", "secret", "private_key", "tenant_id", "job_id", "assignment_key", "payload"} {
			if strings.Contains(strings.ToLower(raw), forbidden) {
				t.Fatalf("health payload must not expose %q", forbidden)
			}
		}
	}
}

func TestHealthFailClosedWhenSchedulerProviderAbsent(t *testing.T) {
	disk, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	hs := NewHealthServer(":0", disk, slog.Default())
	hs.SetPackSync(&fakePackStatusProvider{
		status: packsync.Status{TrustedApplied: true, CoreAuthorityEstablished: true},
	})

	for _, path := range []string{"/health", "/readyz"} {
		rr := httptest.NewRecorder()
		hs.handleHealth(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s expected 503, got %d", path, rr.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["status"] != "SCHEDULER_UNAVAILABLE" {
			t.Fatalf("%s status=%v", path, body["status"])
		}
	}

	rrLive := httptest.NewRecorder()
	hs.handleLive(rrLive, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if rrLive.Code != http.StatusOK {
		t.Fatalf("livez must stay 200: %d", rrLive.Code)
	}
}

func TestHealthFailClosedWhenSchedulerNotRunning(t *testing.T) {
	for _, state := range []scheduler.State{scheduler.StateShuttingDown, scheduler.StateStopped} {
		hs := newTestHealth(t, &fakePackStatusProvider{
			status: packsync.Status{TrustedApplied: true, CoreAuthorityEstablished: true},
		})
		hs.SetScheduler(&fakeSchedulerStatusProvider{snap: scheduler.Snapshot{State: state}})

		for _, path := range []string{"/health", "/readyz"} {
			rr := httptest.NewRecorder()
			hs.handleHealth(rr, httptest.NewRequest(http.MethodGet, path, nil))
			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("%s state=%s expected 503, got %d", path, state, rr.Code)
			}
			var body map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["status"] != "SCHEDULER_NOT_RUNNING" {
				t.Fatalf("%s state=%s status=%v", path, state, body["status"])
			}
		}

		rrLive := httptest.NewRecorder()
		hs.handleLive(rrLive, httptest.NewRequest(http.MethodGet, "/livez", nil))
		if rrLive.Code != http.StatusOK {
			t.Fatalf("livez must stay 200 for state %s: %d", state, rrLive.Code)
		}
	}
}

func TestSchedulerStatusDoesNotOverwriteHigherPriorityFailures(t *testing.T) {
	disk, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	hs := NewHealthServer(":0", disk, slog.Default())
	hs.SetPackSync(&fakePackStatusProvider{
		status: packsync.Status{TrustedApplied: false, CoreAuthorityEstablished: false},
	})
	// Scheduler absent must not replace pack trust failure status.
	rr := httptest.NewRecorder()
	hs.handleReady(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "PACK_TRUST_NOT_ESTABLISHED" {
		t.Fatalf("expected pack precedence, got %v", body["status"])
	}
}
