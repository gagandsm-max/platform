package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/sensor/internal/config"
	"platform/sensor/internal/control"
	"platform/sensor/internal/coreclient"
	sensorruntime "platform/sensor/internal/runtime"
)

func applierBaseline() config.RuntimeConfig {
	return config.RuntimeConfig{
		SensorID: "sensor-001", Role: sensorruntime.RoleNetwork, SiteID: "site-a", PoolID: "pool-a",
		CoreEndpoint: "https://core.example.test", SpoolPath: "/var/lib/platform/sensor/spool",
		HealthAddress: ":8088",
		TLS: config.TLSFiles{
			Certificate: "/var/lib/platform/sensor/identity/sensor.crt",
			PrivateKey:  "/var/lib/platform/sensor/identity/sensor.key",
			CA:          "/var/lib/platform/sensor/identity/ca.crt",
		},
		ConfigurationVersion: 1, LogLevel: "INFO",
		EnabledCapabilities:     []sensorruntime.Capability{sensorruntime.CapabilitySNMP},
		StorageWatermarkPercent: config.StorageProtectionWatermarkPercent,
		Scheduler: config.SchedulerConfig{
			MaxWorkers: 4, MaxQueueDepth: 16, MaxTrackedAssignments: 32, MaxTrackedJobs: 64,
			MaxPayloadBytes: 1_048_576, DefaultExecutionTimeout: 5 * time.Second,
			ShutdownGracePeriod: 10 * time.Second, RetryMaxAttempts: 3,
			RetryInitialBackoff: 100 * time.Millisecond, RetryMaxBackoff: time.Second,
			RetryJitterFraction: 0.05,
			Pools: map[string]config.PoolConfig{
				"NETWORK_POLL": {Workers: 2, QueueDepth: 8},
			},
		},
		Transport: config.TransportConfig{
			RequestTimeout: 15 * time.Second, MaxRequestBytes: 1_048_576, MaxResponseBytes: 1_048_576,
			UploadBatchRecords: 100, UploadBatchBytes: 524_288,
		},
	}
}

func TestRuntimeApplierExactMatchNoMutation(t *testing.T) {
	lv := new(slog.LevelVar)
	_ = config.ApplyLogLevel(lv, "INFO")
	state := newRuntimeConfigState(applierBaseline(), lv)
	before := state.Snapshot()
	if err := state.Apply(context.Background(), 1, "c", before); err != nil {
		t.Fatal(err)
	}
	if state.Snapshot().LogLevel != before.LogLevel || lv.Level() != slog.LevelInfo {
		t.Fatal("unexpected mutation")
	}
}

func TestRuntimeApplierLogLevelOnly(t *testing.T) {
	lv := new(slog.LevelVar)
	_ = config.ApplyLogLevel(lv, "INFO")
	state := newRuntimeConfigState(applierBaseline(), lv)
	candidate := state.Snapshot()
	candidate.LogLevel = "DEBUG"
	if err := state.Apply(context.Background(), 1, "c", candidate); err != nil {
		t.Fatal(err)
	}
	if state.Snapshot().LogLevel != "DEBUG" || lv.Level() != slog.LevelDebug {
		t.Fatalf("log level not applied")
	}
}

func TestRuntimeApplierSchedulerRestartRequired(t *testing.T) {
	lv := new(slog.LevelVar)
	_ = config.ApplyLogLevel(lv, "INFO")
	state := newRuntimeConfigState(applierBaseline(), lv)
	candidate := state.Snapshot()
	candidate.Scheduler.MaxWorkers = 8
	err := state.Apply(context.Background(), 1, "c", candidate)
	if control.CodeOf(err) != control.CodeRestartRequired {
		t.Fatalf("err=%v", err)
	}
	if state.Snapshot().Scheduler.MaxWorkers != 4 || lv.Level() != slog.LevelInfo {
		t.Fatal("mutation on restart-required")
	}
}

func TestRuntimeApplierTransportRestartRequired(t *testing.T) {
	lv := new(slog.LevelVar)
	_ = config.ApplyLogLevel(lv, "INFO")
	state := newRuntimeConfigState(applierBaseline(), lv)
	candidate := state.Snapshot()
	candidate.Transport.RequestTimeout = 30 * time.Second
	if control.CodeOf(state.Apply(context.Background(), 1, "c", candidate)) != control.CodeRestartRequired {
		t.Fatal("expected restart required")
	}
}

func TestRuntimeApplierLogLevelPlusSchedulerAtomic(t *testing.T) {
	lv := new(slog.LevelVar)
	_ = config.ApplyLogLevel(lv, "INFO")
	state := newRuntimeConfigState(applierBaseline(), lv)
	candidate := state.Snapshot()
	candidate.LogLevel = "DEBUG"
	candidate.Scheduler.MaxWorkers = 8
	if control.CodeOf(state.Apply(context.Background(), 1, "c", candidate)) != control.CodeRestartRequired {
		t.Fatal("expected restart required")
	}
	if lv.Level() != slog.LevelInfo || state.Snapshot().LogLevel != "INFO" {
		t.Fatal("partial logLevel mutation")
	}
}

func TestRuntimeApplierLogLevelPlusTransportAtomic(t *testing.T) {
	lv := new(slog.LevelVar)
	_ = config.ApplyLogLevel(lv, "INFO")
	state := newRuntimeConfigState(applierBaseline(), lv)
	candidate := state.Snapshot()
	candidate.LogLevel = "WARN"
	candidate.Transport.MaxRequestBytes = 2_000_000
	if control.CodeOf(state.Apply(context.Background(), 1, "c", candidate)) != control.CodeRestartRequired {
		t.Fatal("expected restart required")
	}
	if lv.Level() != slog.LevelInfo || state.Snapshot().LogLevel != "INFO" {
		t.Fatal("partial logLevel mutation")
	}
}

func TestRuntimeApplierFailedReloadableLeavesCurrent(t *testing.T) {
	state := newRuntimeConfigState(applierBaseline(), nil)
	candidate := state.Snapshot()
	candidate.LogLevel = "DEBUG"
	if control.CodeOf(state.Apply(context.Background(), 1, "c", candidate)) != control.CodeApplyFailed {
		t.Fatal("expected apply failed")
	}
	if state.Snapshot().LogLevel != "INFO" {
		t.Fatal("current mutated after failed apply")
	}
}

func TestRuntimeApplierConcurrentSnapshotApply(t *testing.T) {
	lv := new(slog.LevelVar)
	_ = config.ApplyLogLevel(lv, "INFO")
	state := newRuntimeConfigState(applierBaseline(), lv)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = state.Snapshot()
		}()
		go func() {
			defer wg.Done()
			c := state.Snapshot()
			c.LogLevel = "DEBUG"
			_ = state.Apply(context.Background(), 1, "c", c)
		}()
	}
	wg.Wait()
}

type loopFakeClient struct {
	mu       sync.Mutex
	fetchErr error
	calls    int
}

func (f *loopFakeClient) FetchControlDesiredState(ctx context.Context) (*coreclient.ControlDesiredState, error) {
	f.mu.Lock()
	f.calls++
	err := f.fetchErr
	f.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if err != nil {
		return nil, err
	}
	return nil, nil // NO_DESIRED
}

func (f *loopFakeClient) AckControlAppliedState(context.Context, coreclient.ControlAppliedAckRequest) (coreclient.ControlAppliedAckResponse, error) {
	return coreclient.ControlAppliedAckResponse{}, nil
}

func (f *loopFakeClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestControlReconcileLoopSerialCancellation(t *testing.T) {
	client := &loopFakeClient{}
	store, err := control.NewStore(filepath.Join(t.TempDir(), "applied.json"))
	if err != nil {
		t.Fatal(err)
	}
	var concurrent, maxConcurrent int32
	applier := control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		cur := atomic.AddInt32(&concurrent, 1)
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if cur <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, cur) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt32(&concurrent, -1)
		return nil
	})
	metrics := &control.Metrics{}
	status := control.NewStatusTracker(metrics)
	rec, err := control.NewReconciler(client, store, applierBaseline(), applier, slog.Default(), metrics)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go runControlReconcileLoop(ctx, rec, status, 20*time.Millisecond, slog.Default(), done)

	deadline := time.Now().Add(time.Second)
	for client.callCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if client.callCount() < 1 {
		t.Fatal("expected at least one reconcile")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not exit")
	}
	if atomic.LoadInt32(&maxConcurrent) > 1 {
		t.Fatalf("overlapping RunOnce: max=%d", maxConcurrent)
	}
	final := client.callCount()
	time.Sleep(50 * time.Millisecond)
	if client.callCount() != final {
		t.Fatalf("calls after cancel: before=%d after=%d", final, client.callCount())
	}
}

func TestControlReconcileLoopCoreFailureWaitsInterval(t *testing.T) {
	client := &loopFakeClient{fetchErr: context.DeadlineExceeded}
	store, err := control.NewStore(filepath.Join(t.TempDir(), "applied.json"))
	if err != nil {
		t.Fatal(err)
	}
	metrics := &control.Metrics{}
	status := control.NewStatusTracker(metrics)
	rec, err := control.NewReconciler(client, store, applierBaseline(), control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return nil
	}), slog.Default(), metrics)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go runControlReconcileLoop(ctx, rec, status, 80*time.Millisecond, slog.Default(), done)
	deadline := time.Now().Add(time.Second)
	for client.callCount() < 1 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	first := client.callCount()
	if first < 1 {
		t.Fatal("expected immediate first reconcile")
	}
	time.Sleep(40 * time.Millisecond)
	if client.callCount() != first {
		t.Fatalf("busy-looped before interval: first=%d now=%d", first, client.callCount())
	}
	deadline = time.Now().Add(250 * time.Millisecond)
	for client.callCount() <= first && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if client.callCount() <= first {
		t.Fatal("expected next interval reconcile")
	}
	cancel()
	<-done
}
