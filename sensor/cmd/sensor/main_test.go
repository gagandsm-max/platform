package main

import (
	"log/slog"
	"testing"
	"time"

	"platform/sensor/internal/config"
	sensorruntime "platform/sensor/internal/runtime"
)

func testRuntime() config.RuntimeConfig {
	return config.RuntimeConfig{
		SensorID:     "sensor-001",
		Role:         sensorruntime.RoleNetwork,
		CoreEndpoint: "https://core.example.test",
		SpoolPath:    "/var/lib/platform/sensor/spool",
		HealthAddress: ":8088",
		TLS: config.TLSFiles{
			Certificate: "/var/lib/platform/sensor/identity/sensor.crt",
			PrivateKey:  "/var/lib/platform/sensor/identity/sensor.key",
			CA:          "/var/lib/platform/sensor/identity/ca.crt",
		},
		ConfigurationVersion:    1,
		LogLevel:                "DEBUG",
		StorageWatermarkPercent: config.StorageProtectionWatermarkPercent,
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
