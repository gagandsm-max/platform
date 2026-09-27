package control_test

import (
	"path/filepath"
	"testing"
	"time"

	"platform/sensor/internal/config"
	sensorruntime "platform/sensor/internal/runtime"
)

func validBaseline(t *testing.T) config.RuntimeConfig {
	t.Helper()
	dir := t.TempDir()
	cfg := config.RuntimeConfig{
		SensorID:      "sensor-test-01",
		Role:          sensorruntime.RoleNetwork,
		SiteID:        "site-a",
		PoolID:        "pool-a",
		CoreEndpoint:  "https://core.example.invalid",
		SpoolPath:     filepath.Join(dir, "spool"),
		HealthAddress: ":18088",
		TLS: config.TLSFiles{
			Certificate: filepath.Join(dir, "cert.pem"),
			PrivateKey:  filepath.Join(dir, "key.pem"),
			CA:          filepath.Join(dir, "ca.pem"),
		},
		ConfigurationVersion:    1,
		LogLevel:                "INFO",
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
				"SYNTHETIC":    {Workers: 1, QueueDepth: 4},
			},
		},
		Transport: config.TransportConfig{
			RequestTimeout: 15 * time.Second, MaxRequestBytes: 1_048_576, MaxResponseBytes: 1_048_576,
			UploadBatchRecords: 100, UploadBatchBytes: 524_288,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("baseline invalid: %v", err)
	}
	return cfg
}
