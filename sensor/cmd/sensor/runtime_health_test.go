package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"

	"platform/sensor/internal/observation"
	sensorruntime "platform/sensor/internal/runtime"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
)

type stubRuntimeComponent struct {
	name       string
	capability sensorruntime.Capability
	degraded   atomic.Bool
}

func (s *stubRuntimeComponent) Name() string                   { return s.name }
func (s *stubRuntimeComponent) Capability() sensorruntime.Capability { return s.capability }
func (s *stubRuntimeComponent) Start(context.Context) error    { return nil }
func (s *stubRuntimeComponent) Ready(context.Context) error {
	if s.degraded.Load() {
		return fmt.Errorf("%s_COMPONENT_DEGRADED: collector unusable", s.name)
	}
	return nil
}
func (s *stubRuntimeComponent) Drain(context.Context) error    { return nil }
func (s *stubRuntimeComponent) Shutdown(context.Context) error { return nil }

type stubScheduler struct{}

func (stubScheduler) Submit(scheduler.Job) (scheduler.Submission, error) {
	return scheduler.Submission{}, nil
}
func (stubScheduler) Shutdown(context.Context) error { return nil }
func (stubScheduler) Snapshot() scheduler.Snapshot {
	return scheduler.Snapshot{State: scheduler.StateRunning}
}

func TestRuntimeHealthAdapterReflectsComponentDegradationAndRecovery(t *testing.T) {
	registry := sensorruntime.NewExecutorRegistry()
	rt, err := sensorruntime.New(sensorruntime.Config{
		Role:                sensorruntime.RoleNetwork,
		EnabledCapabilities: []sensorruntime.Capability{sensorruntime.CapabilityICMP},
	}, stubScheduler{}, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	comp := &stubRuntimeComponent{name: "icmp", capability: sensorruntime.CapabilityICMP}
	if err := rt.RegisterComponent(comp); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	adapter := runtimeHealthAdapter{rt: rt}
	if got := adapter.Snapshot(); got.State != string(sensorruntime.StateReady) {
		t.Fatalf("initial=%+v", got)
	}

	comp.degraded.Store(true)
	if got := adapter.Snapshot(); got.State != string(sensorruntime.StateDegraded) {
		t.Fatalf("degraded=%+v", got)
	}

	comp.degraded.Store(false)
	if got := adapter.Snapshot(); got.State != string(sensorruntime.StateReady) {
		t.Fatalf("recovered=%+v", got)
	}
}

func TestInfrastructureBootstrapOmitsRuntimeHealthProvider(t *testing.T) {
	cfg := testRuntime()
	cfg.Role = sensorruntime.RoleInfrastructure
	cfg.EnabledCapabilities = []sensorruntime.Capability{sensorruntime.CapabilityInfra, sensorruntime.CapabilityAgentIngress}
	disk, err := spool.Open(spool.Config{Dir: t.TempDir(), MaxRecordBytes: 1 << 20, Logger: slog.Default()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disk.Close() })
	sink, err := observation.NewDiskSink(disk, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	b, err := startProductionBootstrap(context.Background(), cfg, sink, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.shutdown(context.Background()) })
	if b.Runtime != nil {
		t.Fatal("INFRASTRUCTURE must not activate Universal Sensor runtime")
	}
	if b.Scheduler == nil || b.Scheduler.Snapshot().State != scheduler.StateRunning {
		t.Fatal("INFRASTRUCTURE must retain production scheduler")
	}
}
