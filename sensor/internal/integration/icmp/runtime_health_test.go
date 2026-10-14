package icmpintegration

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/sensor/internal/control"
	"platform/sensor/internal/health"
	"platform/sensor/internal/observation"
	"platform/sensor/internal/packsync"
	protocolicmp "platform/sensor/internal/protocol/icmp"
	"platform/sensor/internal/runtime"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
)

type runtimeTestScheduler struct {
	state scheduler.State
}

func (s *runtimeTestScheduler) Submit(scheduler.Job) (scheduler.Submission, error) {
	return scheduler.Submission{}, nil
}
func (s *runtimeTestScheduler) Shutdown(context.Context) error {
	s.state = scheduler.StateStopped
	return nil
}
func (s *runtimeTestScheduler) Snapshot() scheduler.Snapshot {
	if s.state == "" {
		s.state = scheduler.StateRunning
	}
	return scheduler.Snapshot{State: s.state, QueueCapacity: 8}
}

type packOK struct{}

func (packOK) Snapshot() packsync.Status {
	return packsync.Status{TrustedApplied: true, CoreAuthorityEstablished: true}
}

type controlOK struct{}

func (controlOK) Snapshot() control.Status { return control.Status{State: control.StateInSync} }

type refreshingRuntimeHealth struct {
	rt *runtime.Runtime
}

func (a refreshingRuntimeHealth) Snapshot() health.RuntimeHealth {
	_ = a.rt.RefreshHealth(context.Background())
	return health.RuntimeHealth{State: string(a.rt.Snapshot().State)}
}

func TestPostStartICMPDegradationSurfacedOnReadyz(t *testing.T) {
	disk, err := spool.Open(spool.Config{Dir: t.TempDir(), MaxRecordBytes: 8192, Logger: slog.Default()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disk.Close() })
	sink, err := observation.NewDiskSink(disk, 8192)
	if err != nil {
		t.Fatal(err)
	}

	registry := runtime.NewExecutorRegistry()
	sched := &runtimeTestScheduler{state: scheduler.StateRunning}
	rt, err := runtime.New(runtime.Config{
		Role:                runtime.RoleNetwork,
		EnabledCapabilities: []runtime.Capability{runtime.CapabilityICMP},
	}, sched, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	exec, err := NewExecutor(fixtureExecutor{
		err: scheduler.Permanent(&protocolicmp.Error{Code: protocolicmp.CodePermission}),
	}, sink, Config{SensorID: "sensor", MaxResultBytes: 4096, MaxEnvelopeBytes: 8192, MaxSamples: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := Register(rt, registry, exec); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := rt.RefreshHealth(context.Background()); err != nil {
		t.Fatalf("initial ready: %v", err)
	}

	healthDisk, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	hs := health.NewHealthServer(":0", healthDisk, slog.Default())
	hs.SetPackSync(packOK{})
	hs.SetScheduler(sched)
	hs.SetControl(controlOK{})
	hs.SetRuntime(refreshingRuntimeHealth{rt: rt})

	assertReadyz := func(wantCode int, wantStatus string, wantReady bool, wantReason string) {
		t.Helper()
		rr := httptest.NewRecorder()
		hs.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		var body map[string]any
		if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if rr.Code != wantCode || body["status"] != wantStatus || body["ready"] != wantReady {
			t.Fatalf("readyz code=%d body=%v", rr.Code, body)
		}
		if wantReason == "" {
			if _, ok := body["reason_code"]; ok {
				t.Fatalf("unexpected reason=%v", body["reason_code"])
			}
		} else if body["reason_code"] != wantReason {
			t.Fatalf("reason=%v want=%s", body["reason_code"], wantReason)
		}
	}

	assertReadyz(http.StatusOK, "UP", true, "")

	job := scheduler.Job{
		Kind: protocolicmp.JobKind, Class: scheduler.ClassNetworkPoll,
		JobID: "job", AssignmentKey: "assignment", AssignmentVersion: 1,
		TenantID: "tenant", EntityID: "entity",
		Payload: []byte(`{"schemaVersion":1,"target":"127.0.0.1","ipVersion":4}`),
		Timeout: time.Second,
	}
	if _, err := exec.Execute(context.Background(), job); err == nil {
		t.Fatal("permission failure expected")
	}
	snap := exec.Component().Snapshot()
	if !snap.Degraded || snap.LastErrorCode != string(protocolicmp.CodePermission) || snap.SpoolWriteFailures != 0 {
		t.Fatalf("component snap=%+v", snap)
	}
	assertReadyz(http.StatusServiceUnavailable, "DEGRADED", false, "SENSOR_RUNTIME_DEGRADED")

	// Recovery: swap base to success and clear degradation.
	exec.base = fixtureExecutor{result: fixtureResult()}
	if _, err := exec.Execute(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if snap := exec.Component().Snapshot(); snap.Degraded {
		t.Fatalf("expected recovery snap=%+v", snap)
	}
	assertReadyz(http.StatusOK, "UP", true, "")
}

func TestStartupPermissionDeniedSurfacedOnReadyzWithoutExecute(t *testing.T) {
	disk, err := spool.Open(spool.Config{Dir: t.TempDir(), MaxRecordBytes: 8192, Logger: slog.Default()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disk.Close() })
	sink, err := observation.NewDiskSink(disk, 8192)
	if err != nil {
		t.Fatal(err)
	}

	registry := runtime.NewExecutorRegistry()
	sched := &runtimeTestScheduler{state: scheduler.StateRunning}
	rt, err := runtime.New(runtime.Config{
		Role:                runtime.RoleNetwork,
		EnabledCapabilities: []runtime.Capability{runtime.CapabilityICMP},
	}, sched, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	var checks atomic.Uint64
	exec, err := NewExecutor(fixtureExecutor{result: fixtureResult()}, sink, Config{
		SensorID: "sensor", MaxResultBytes: 4096, MaxEnvelopeBytes: 8192, MaxSamples: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	exec.Component().SetLocalCapabilityCheck(func(context.Context) error {
		checks.Add(1)
		return &protocolicmp.Error{Code: protocolicmp.CodePermission}
	})
	if err := Register(rt, registry, exec); err != nil {
		t.Fatal(err)
	}
	startErr := rt.Start(context.Background())
	if rt.Snapshot().State != runtime.StateDegraded {
		t.Fatalf("state=%s startErr=%v", rt.Snapshot().State, startErr)
	}
	if startErr == nil {
		t.Fatal("expected Start to surface SENSOR_RUNTIME_DEGRADED")
	}
	if checks.Load() != 1 {
		t.Fatalf("capability checks=%d want 1", checks.Load())
	}
	snap := exec.Component().Snapshot()
	if !snap.Degraded || snap.LastErrorCode != string(protocolicmp.CodePermission) || snap.Executions != 0 {
		t.Fatalf("startup snap=%+v", snap)
	}
	if err := rt.RefreshHealth(context.Background()); err == nil {
		t.Fatal("expected degraded refresh error")
	}

	healthDisk, err := spool.NewDiskSpool(t.TempDir(), 80, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	hs := health.NewHealthServer(":0", healthDisk, slog.Default())
	hs.SetPackSync(packOK{})
	hs.SetScheduler(sched)
	hs.SetControl(controlOK{})
	hs.SetRuntime(refreshingRuntimeHealth{rt: rt})

	rr := httptest.NewRecorder()
	hs.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusServiceUnavailable || body["status"] != "DEGRADED" || body["ready"] != false || body["reason_code"] != "SENSOR_RUNTIME_DEGRADED" {
		t.Fatalf("readyz code=%d body=%v", rr.Code, body)
	}
	// Second Start must not re-run the privilege check.
	if err := rt.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if checks.Load() != 1 {
		t.Fatalf("capability check must be one-time checks=%d", checks.Load())
	}

	// Recovery after capability restored: successful Execute clears degradation.
	if _, err := exec.Execute(context.Background(), scheduler.Job{
		Kind: protocolicmp.JobKind, Class: scheduler.ClassNetworkPoll,
		JobID: "job", AssignmentKey: "assignment", AssignmentVersion: 1,
		TenantID: "tenant", EntityID: "entity",
		Payload: []byte(`{"schemaVersion":1,"target":"127.0.0.1","ipVersion":4}`),
		Timeout: time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	if snap := exec.Component().Snapshot(); snap.Degraded {
		t.Fatalf("expected recovery snap=%+v", snap)
	}
	if err := rt.RefreshHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	hs.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	body = map[string]any{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusOK || body["status"] != "UP" || body["ready"] != true {
		t.Fatalf("recovered readyz code=%d body=%v", rr.Code, body)
	}
}

func TestStartupProtocolUnsupportedDegradesWithoutExecute(t *testing.T) {
	disk, err := spool.Open(spool.Config{Dir: t.TempDir(), MaxRecordBytes: 8192, Logger: slog.Default()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disk.Close() })
	sink, err := observation.NewDiskSink(disk, 8192)
	if err != nil {
		t.Fatal(err)
	}
	registry := runtime.NewExecutorRegistry()
	sched := &runtimeTestScheduler{state: scheduler.StateRunning}
	rt, err := runtime.New(runtime.Config{
		Role:                runtime.RoleNetwork,
		EnabledCapabilities: []runtime.Capability{runtime.CapabilityICMP},
	}, sched, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	exec, err := NewExecutor(fixtureExecutor{result: fixtureResult()}, sink, Config{
		SensorID: "sensor", MaxResultBytes: 4096, MaxEnvelopeBytes: 8192, MaxSamples: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	exec.Component().SetLocalCapabilityCheck(func(context.Context) error {
		return &protocolicmp.Error{Code: protocolicmp.CodeProtocol}
	})
	if err := Register(rt, registry, exec); err != nil {
		t.Fatal(err)
	}
	startErr := rt.Start(context.Background())
	if rt.Snapshot().State != runtime.StateDegraded {
		t.Fatalf("state=%s startErr=%v", rt.Snapshot().State, startErr)
	}
	snap := exec.Component().Snapshot()
	if !snap.Degraded || snap.LastErrorCode != string(protocolicmp.CodeProtocol) || snap.Executions != 0 {
		t.Fatalf("snap=%+v", snap)
	}
}

func TestStartupCapabilityCheckRaceWithReady(t *testing.T) {
	exec, _ := integrationFixture(t, fixtureExecutor{result: fixtureResult()})
	exec.component.markRegistered()
	var started sync.WaitGroup
	started.Add(1)
	exec.Component().SetLocalCapabilityCheck(func(context.Context) error {
		started.Done()
		time.Sleep(20 * time.Millisecond)
		return &protocolicmp.Error{Code: protocolicmp.CodePermission}
	})
	var readyErrs atomic.Uint64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started.Wait()
			if err := exec.Component().Ready(context.Background()); err != nil {
				readyErrs.Add(1)
			}
		}()
	}
	if err := exec.Component().Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if !exec.Component().Snapshot().Degraded {
		t.Fatal("expected degraded after startup capability denial")
	}
	if err := exec.Component().Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if exec.Component().Snapshot().Executions != 0 {
		t.Fatal("startup check must not count executions")
	}
}
