package health

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"platform/sensor/internal/agentdrain"
	"platform/sensor/internal/spool"
)

func degradeRealDiskSpool(t *testing.T, disk *spool.DiskSpool) {
	t.Helper()
	_, err := disk.Append(spool.AppendRequest{
		Class:     spool.ClassMetric,
		Type:      "metric",
		Payload:   []byte("integrity-probe"),
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	files, err := disk.ListPendingFiles(1)
	if err != nil || len(files) != 1 {
		t.Fatalf("pending files=%v err=%v", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var rec spool.Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	rec.Payload = []byte("evil")
	bad, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[0], bad, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := disk.ReadBatch(10, 1024*1024); spool.CodeOf(err) != spool.CodeCorruption {
		t.Fatalf("public read/quarantine path must latch degradation: %v", err)
	}
	if pct, err := disk.CheckStorageUsage(); err == nil {
		t.Fatalf("degraded real provider must fail CheckStorageUsage: pct=%v", pct)
	}
}

func assertNoSensitiveLeak(t *testing.T, path string, raw string) {
	t.Helper()
	lower := strings.ToLower(raw)
	for _, forbidden := range []string{
		"device_down", "password", "token", "secret", "bearer", "payload",
		"private_key", "authorization", "evil", "integrity-probe",
		"injected", "statfs",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("%s leaked %q: %s", path, forbidden, raw)
		}
	}
}

func TestReadyzFailsForRealDegradedTelemetrySpool(t *testing.T) {
	disk, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	degradeRealDiskSpool(t, disk)

	hs := NewHealthServer(":0", disk, slog.Default(),
		WithIdentity("0.1.0-test", "sensor-instance-test"))
	hs.SetPackSync(trustedPack())
	hs.SetScheduler(runningScheduler())
	hs.SetControl(inSyncControl())

	rr, body := request(t, hs, "/readyz")
	if rr.Code != http.StatusServiceUnavailable || body["status"] != "NOT_READY" ||
		body["reason_code"] != reasonSpoolCheck {
		t.Fatalf("degraded telemetry spool must fail readiness: %d %v", rr.Code, body)
	}
	assertNoSensitiveLeak(t, "/readyz", rr.Body.String())

	rr, body = request(t, hs, "/health")
	deps, ok := body["dependency_status"].(map[string]any)
	if !ok || rr.Code != http.StatusServiceUnavailable || deps["local_spool"] != "DOWN" {
		t.Fatalf("support evidence must stop advertising local_spool UP: %d %v", rr.Code, body)
	}
	if body["component_state"] != "NOT_READY" {
		t.Fatalf("component_state=%v", body["component_state"])
	}
	assertNoSensitiveLeak(t, "/health", rr.Body.String())

	rr, body = request(t, hs, "/livez")
	if rr.Code != http.StatusOK || body["status"] != "UP" {
		t.Fatalf("liveness must remain independent: %d %v", rr.Code, body)
	}
}

func TestReadyzFailsForRealDegradedAgentSpoolWhileDeliveryRemainsUp(t *testing.T) {
	telemetry, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer telemetry.Close()
	agentDisk, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer agentDisk.Close()
	degradeRealDiskSpool(t, agentDisk)

	hs := NewHealthServer(":0", telemetry, slog.Default(),
		WithIdentity("0.1.0-test", "sensor-instance-test"))
	hs.SetPackSync(trustedPack())
	hs.SetScheduler(runningScheduler())
	hs.SetControl(inSyncControl())
	hs.SetAgentSpool(agentDisk)
	hs.SetAgentDelivery(&fakeAgentDeliveryStatusProvider{status: agentdrain.DeliveryStatus{
		State:           agentdrain.DeliveryStateUp,
		DurableAcks:     2,
		LastSuccessTime: time.Now().UTC(),
	}})

	rr, body := request(t, hs, "/readyz")
	if rr.Code != http.StatusServiceUnavailable || body["status"] != "NOT_READY" ||
		body["reason_code"] != reasonAgentSpoolCheck {
		t.Fatalf("degraded agent spool must fail readiness: %d %v", rr.Code, body)
	}
	assertNoSensitiveLeak(t, "/readyz", rr.Body.String())

	rr, body = request(t, hs, "/health")
	deps, ok := body["dependency_status"].(map[string]any)
	if !ok {
		t.Fatalf("deps missing: %v", body)
	}
	if rr.Code != http.StatusServiceUnavailable || deps["agent_spool"] != "DOWN" {
		t.Fatalf("agent_spool must not stay UP: %d %v", rr.Code, body)
	}
	if deps["local_spool"] != "UP" {
		t.Fatalf("healthy telemetry spool must remain UP: %v", deps)
	}
	if deps["agent_delivery"] != "UP" {
		t.Fatalf("agent delivery must remain independently UP: %v", deps)
	}
	if body["agent_delivery_status"] != "UP" {
		t.Fatalf("agent_delivery_status=%v", body["agent_delivery_status"])
	}
	assertNoSensitiveLeak(t, "/health", rr.Body.String())
}

func TestDisabledAgentReceiverStillOmitsAgentSpoolWithRealTelemetry(t *testing.T) {
	disk, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	hs := NewHealthServer(":0", disk, slog.Default(),
		WithIdentity("0.1.0-test", "sensor-instance-test"))
	hs.SetPackSync(trustedPack())
	hs.SetScheduler(runningScheduler())
	hs.SetControl(inSyncControl())
	hs.SetAgentDelivery(nil)

	rr, body := request(t, hs, "/health")
	deps, ok := body["dependency_status"].(map[string]any)
	if rr.Code != http.StatusOK || !ok || body["component_state"] != "UP" {
		t.Fatalf("healthy fixture failed: %d %v", rr.Code, body)
	}
	if _, exists := deps["agent_spool"]; exists {
		t.Fatalf("disabled receiver invented agent_spool: %v", deps)
	}
	if _, exists := deps["agent_delivery"]; exists {
		t.Fatalf("disabled receiver invented agent_delivery: %v", deps)
	}
	if deps["local_spool"] != "UP" {
		t.Fatalf("healthy local_spool=%v", deps["local_spool"])
	}
	assertNoSensitiveLeak(t, "/health", rr.Body.String())
}

func TestHealthyRealSpoolAndPressureBehaviorRemainCorrect(t *testing.T) {
	disk, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	hs := NewHealthServer(":0", disk, slog.Default(),
		WithIdentity("0.1.0-test", "sensor-instance-test"))
	hs.SetPackSync(trustedPack())
	hs.SetScheduler(runningScheduler())
	hs.SetControl(inSyncControl())

	rr, body := request(t, hs, "/readyz")
	if rr.Code != http.StatusOK || body["status"] != "UP" || body["ready"] != true {
		t.Fatalf("healthy real spool must stay ready: %d %v", rr.Code, body)
	}
	rr, body = request(t, hs, "/health")
	deps := body["dependency_status"].(map[string]any)
	if rr.Code != http.StatusOK || deps["local_spool"] != "UP" || body["component_state"] != "UP" {
		t.Fatalf("healthy support evidence failed: %d %v", rr.Code, body)
	}

	hs.spool = &fakeStorageStatusProvider{pct: 80}
	rr, body = request(t, hs, "/readyz")
	if rr.Code != http.StatusServiceUnavailable || body["reason_code"] != reasonSpoolPressure {
		t.Fatalf("80%% pressure behavior changed: %d %v", rr.Code, body)
	}
	rr, body = request(t, hs, "/health")
	deps = body["dependency_status"].(map[string]any)
	if deps["local_spool"] != "DOWN" {
		t.Fatalf("pressure must mark local_spool DOWN: %v", deps)
	}

	hs.spool = &fakeStorageStatusProvider{err: errors.New("disk unavailable")}
	rr, body = request(t, hs, "/readyz")
	if rr.Code != http.StatusServiceUnavailable || body["reason_code"] != reasonSpoolCheck {
		t.Fatalf("provider error path changed: %d %v", rr.Code, body)
	}
	assertNoSensitiveLeak(t, "/readyz", rr.Body.String())
}
