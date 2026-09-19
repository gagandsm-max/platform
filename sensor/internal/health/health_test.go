package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/packsync"
	"platform/sensor/internal/spool"
)

func newTestHealth(t *testing.T, syncer *packsync.Syncer) *HealthServer {
	t.Helper()
	disk, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	hs := NewHealthServer(0, disk, slog.Default())
	if syncer != nil {
		hs.SetPackSync(syncer)
	}
	return hs
}

func TestLivezOKWhenPackTrustMissing(t *testing.T) {
	syncer := failClosedPackSyncer(t)
	hs := newTestHealth(t, syncer)

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
	syncer := failClosedPackSyncer(t)
	hs := newTestHealth(t, syncer)

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
	dir := t.TempDir()
	syncer, err := packsync.New(packsync.Config{Dir: dir, PollInterval: time.Hour}, stubPackClient{})
	if err != nil {
		t.Fatal(err)
	}
	// Establish Core authority via successful gen0 desired-state sync.
	syncer.TestingSetSuccess(0, 0)
	if !syncer.Snapshot().TrustedApplied || !syncer.Snapshot().CoreAuthorityEstablished {
		t.Fatal("test setup requires trusted local state and Core authority")
	}
	syncer.TestingSetError(packsync.ErrCodeSyncTransient, false)

	hs := newTestHealth(t, syncer)
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
	dir := t.TempDir()
	syncer, err := packsync.New(packsync.Config{Dir: dir, PollInterval: time.Hour}, stubPackClient{})
	if err != nil {
		t.Fatal(err)
	}
	if syncer.Snapshot().CoreAuthorityEstablished || syncer.Snapshot().Ready() {
		t.Fatal("fresh install must not be ready before Core authority")
	}

	hs := newTestHealth(t, syncer)
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
	dir := t.TempDir()
	syncer, err := packsync.New(packsync.Config{Dir: dir, PollInterval: time.Hour}, stubPackClient{})
	if err != nil {
		t.Fatal(err)
	}
	syncer.TestingSetSuccess(0, 0)
	syncer.TestingSetError(packsync.ErrCodeAuthFailed, true)

	hs := newTestHealth(t, syncer)
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
	syncer := failClosedPackSyncer(t)
	hs := newTestHealth(t, syncer)

	rr := httptest.NewRecorder()
	hs.handleReady(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected fail-closed before restore: %d", rr.Code)
	}

	syncer.TestingSetSuccess(1, 1)

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
	syncer := failClosedPackSyncer(t)
	hs := newTestHealth(t, syncer)
	rr := httptest.NewRecorder()
	hs.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("/health must fail closed on missing pack trust: %d", rr.Code)
	}
}

func failClosedPackSyncer(t *testing.T) *packsync.Syncer {
	t.Helper()
	dir := t.TempDir()
	artDir := filepath.Join(dir, "artifacts")
	stateDir := filepath.Join(dir, "state")
	_ = os.MkdirAll(artDir, 0o700)
	_ = os.MkdirAll(stateDir, 0o700)
	if err := os.WriteFile(filepath.Join(stateDir, "applied.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := packsync.New(packsync.Config{Dir: dir, PollInterval: time.Hour}, stubPackClient{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().TrustedApplied {
		t.Fatal("expected untrusted syncer")
	}
	return s
}

type stubPackClient struct{}

func (stubPackClient) FetchCapabilityPackDesiredState(context.Context) (coreclient.CapabilityPackDesiredState, error) {
	return coreclient.CapabilityPackDesiredState{}, nil
}

func (stubPackClient) FetchCapabilityPackArtifact(context.Context, string, string, int) (coreclient.CapabilityPackArtifact, error) {
	return coreclient.CapabilityPackArtifact{}, nil
}

func (stubPackClient) AckCapabilityPackAppliedState(context.Context, int64, string) (coreclient.AppliedStateAck, error) {
	return coreclient.AppliedStateAck{}, nil
}
