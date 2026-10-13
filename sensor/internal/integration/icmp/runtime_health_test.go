package icmpintegration

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
