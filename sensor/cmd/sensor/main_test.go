package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"platform/sensor/internal/config"
	sensorruntime "platform/sensor/internal/runtime"
	"platform/sensor/internal/scheduler"
)

func testRuntime() config.RuntimeConfig {
	return config.RuntimeConfig{
		SensorID:      "sensor-001",
		Role:          sensorruntime.RoleNetwork,
		CoreEndpoint:  "https://core.example.test",
		SpoolPath:     "/var/lib/platform/sensor/spool",
		HealthAddress: ":8088",
		TLS: config.TLSFiles{
			Certificate: "/var/lib/platform/sensor/identity/sensor.crt",
			PrivateKey:  "/var/lib/platform/sensor/identity/sensor.key",
			CA:          "/var/lib/platform/sensor/identity/ca.crt",
		},
		ConfigurationVersion:    1,
		LogLevel:                "DEBUG",
		StorageWatermarkPercent: config.StorageProtectionWatermarkPercent,
		Scheduler: config.SchedulerConfig{
			MaxWorkers:              8,
			MaxQueueDepth:           16,
			MaxTrackedAssignments:   64,
			MaxTrackedJobs:          24,
			MaxPayloadBytes:         1048576,
			DefaultExecutionTimeout: 10 * time.Second,
			ShutdownGracePeriod:     5 * time.Second,
			RetryMaxAttempts:        3,
			RetryInitialBackoff:     100 * time.Millisecond,
			RetryMaxBackoff:         2 * time.Second,
			RetryJitterFraction:     0.2,
			Pools: map[string]config.PoolConfig{
				"NETWORK_POLL": {Workers: 4, QueueDepth: 8},
				"STREAMING":    {Workers: 2, QueueDepth: 4},
			},
		},
		Transport: config.TransportConfig{
			RequestTimeout:     10 * time.Second,
			MaxRequestBytes:    2097152,
			MaxResponseBytes:   2097152,
			UploadBatchRecords: 100,
			UploadBatchBytes:   1048576,
		},
	}
}

func TestCoreClientUsesRuntimeTransport(t *testing.T) {
	rt := testRuntime()
	cfg := coreClientConfigFromRuntime(rt, slog.Default())
	if cfg.RequestTimeout != rt.Transport.RequestTimeout {
		t.Fatalf("timeout=%v", cfg.RequestTimeout)
	}
	if cfg.MaxResponseBytes != rt.Transport.MaxResponseBytes {
		t.Fatalf("MaxResponseBytes=%d", cfg.MaxResponseBytes)
	}
	if cfg.BaseURL != rt.CoreEndpoint {
		t.Fatalf("BaseURL=%q", cfg.BaseURL)
	}
	if cfg.CertificateFile != rt.TLS.Certificate || cfg.PrivateKeyFile != rt.TLS.PrivateKey || cfg.CAFile != rt.TLS.CA {
		t.Fatalf("TLS wiring mismatch: %+v", cfg)
	}
}

func TestUploaderUsesRuntimeTransportBounds(t *testing.T) {
	rt := testRuntime()
	cfg := uploaderConfigFromRuntime(rt, "stream-test")
	if cfg.MaxRequestBytes != rt.Transport.MaxRequestBytes {
		t.Fatalf("MaxRequestBytes=%d", cfg.MaxRequestBytes)
	}
	if cfg.MaxBatchRecords != rt.Transport.UploadBatchRecords {
		t.Fatalf("MaxBatchRecords=%d", cfg.MaxBatchRecords)
	}
	if cfg.MaxBatchPayloadBytes != rt.Transport.UploadBatchBytes {
		t.Fatalf("MaxBatchPayloadBytes=%d", cfg.MaxBatchPayloadBytes)
	}
	if cfg.SensorID != rt.SensorID {
		t.Fatalf("SensorID=%q", cfg.SensorID)
	}
}

func TestApplyLogLevelControlsStartupLogger(t *testing.T) {
	lv := new(slog.LevelVar)
	if err := config.ApplyLogLevel(lv, "WARN"); err != nil {
		t.Fatal(err)
	}
	if lv.Level() != slog.LevelWarn {
		t.Fatalf("level=%v", lv.Level())
	}
}

func TestNewProductionSchedulerInitialSnapshot(t *testing.T) {
	rt := testRuntime()
	sched, err := newProductionScheduler(rt, slog.Default())
	if err != nil {
		t.Fatalf("newProductionScheduler: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = sched.Shutdown(ctx)
	})

	snap := sched.Snapshot()
	if snap.State != scheduler.StateRunning {
		t.Fatalf("state=%s", snap.State)
	}
	wantWorkers := 0
	wantQueue := 0
	for _, pool := range rt.Scheduler.Pools {
		wantWorkers += pool.Workers
		wantQueue += pool.QueueDepth
	}
	if snap.ConfiguredWorkers != wantWorkers {
		t.Fatalf("ConfiguredWorkers=%d want=%d", snap.ConfiguredWorkers, wantWorkers)
	}
	if snap.QueueCapacity != wantQueue {
		t.Fatalf("QueueCapacity=%d want=%d", snap.QueueCapacity, wantQueue)
	}
	if snap.ActiveWorkers != 0 {
		t.Fatalf("ActiveWorkers=%d", snap.ActiveWorkers)
	}
	if snap.QueueDepth != 0 {
		t.Fatalf("QueueDepth=%d", snap.QueueDepth)
	}
	if snap.TrackedJobs != 0 {
		t.Fatalf("TrackedJobs=%d", snap.TrackedJobs)
	}
	if snap.TrackedAssignments != 0 {
		t.Fatalf("TrackedAssignments=%d", snap.TrackedAssignments)
	}
}

func TestEmptyRegistryFailsClosedWithoutRetry(t *testing.T) {
	rt := testRuntime()
	sched, err := newProductionScheduler(rt, slog.Default())
	if err != nil {
		t.Fatalf("newProductionScheduler: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = sched.Shutdown(ctx)
	})

	job := scheduler.Job{
		JobID:             "job-slice6-failclosed",
		AssignmentKey:     "assignment-slice6",
		TenantID:          "tenant-slice6",
		EntityID:          "entity-slice6",
		Kind:              scheduler.JobKind("UNKNOWN_KIND_NO_ROUTE"),
		Class:             scheduler.ClassNetworkPoll,
		AssignmentVersion: 1,
		LeaseExpiresAt:    time.Now().Add(30 * time.Second),
		Timeout:           200 * time.Millisecond,
		Payload:           []byte(`{"bounded":true}`),
	}
	sub, err := sched.Submit(job)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if sub.Status != scheduler.SubmissionAccepted {
		t.Fatalf("submission status=%s", sub.Status)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := sched.Snapshot()
		if snap.Failed == 1 && snap.Completed == 0 && snap.Retries == 0 && snap.TrackedJobs == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	snap := sched.Snapshot()
	t.Fatalf("fail-closed counters not reached: failed=%d completed=%d retries=%d tracked=%d",
		snap.Failed, snap.Completed, snap.Retries, snap.TrackedJobs)
}

func TestProductionSchedulerIdleShutdownWithinConfiguredGrace(t *testing.T) {
	rt := testRuntime()
	rt.Scheduler.ShutdownGracePeriod = 500 * time.Millisecond

	sched, err := newProductionScheduler(rt, slog.Default())
	if err != nil {
		t.Fatalf("newProductionScheduler: %v", err)
	}

	start := time.Now()
	if err := sched.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	elapsed := time.Since(start)

	if snap := sched.Snapshot(); snap.State != scheduler.StateStopped {
		t.Fatalf("state=%s want=%s", snap.State, scheduler.StateStopped)
	}
	if elapsed > rt.Scheduler.ShutdownGracePeriod+250*time.Millisecond {
		t.Fatalf("shutdown elapsed %v exceeded grace+allowance %v",
			elapsed, rt.Scheduler.ShutdownGracePeriod+250*time.Millisecond)
	}
}
