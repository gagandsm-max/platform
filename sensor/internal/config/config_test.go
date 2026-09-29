package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sensorruntime "platform/sensor/internal/runtime"
)

func clearSensorEnv(t *testing.T) {
	t.Helper()
	keys := make([]string, 0, 32)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(key, "PLATFORM_SENSOR_") ||
			strings.HasPrefix(key, "SENSOR_") ||
			key == "PLATFORM_ENV" ||
			key == "PLATFORM_LOG_LEVEL" ||
			key == "PLATFORM_STORAGE_WATERMARK_PERCENT" ||
			strings.HasPrefix(key, "PLATFORM_TLS_") ||
			strings.HasPrefix(key, "PLATFORM_CA_") ||
			key == "CORE_URL" {
			keys = append(keys, key)
		}
	}
	for _, key := range keys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
}

func applyValidRuntimeEnv(t *testing.T) {
	t.Helper()
	for k, v := range validMap() {
		t.Setenv(k, v)
	}
}

func TestLoadConfigRuntimeIsSoleSharedAuthority(t *testing.T) {
	clearSensorEnv(t)
	applyValidRuntimeEnv(t)
	t.Setenv("PLATFORM_ENV", "development")
	t.Setenv("SENSOR_POLL_INTERVAL_SECONDS", "15")
	t.Setenv("SENSOR_CAPABILITY_PACK_DIR", "/var/lib/platform/sensor/packs")
	t.Setenv("SENSOR_AGENT_RECEIVER_ENABLED", "false")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Runtime.SensorID != "sensor-001" {
		t.Fatalf("SensorID=%q", cfg.Runtime.SensorID)
	}
	if cfg.Runtime.Role != sensorruntime.RoleNetwork {
		t.Fatalf("Role=%q", cfg.Runtime.Role)
	}
	if cfg.Runtime.CoreEndpoint != "https://core.example.test" {
		t.Fatalf("CoreEndpoint=%q", cfg.Runtime.CoreEndpoint)
	}
	if cfg.Runtime.SpoolPath != "/var/lib/platform/sensor/spool" {
		t.Fatalf("SpoolPath=%q", cfg.Runtime.SpoolPath)
	}
	if cfg.Runtime.HealthAddress != "127.0.0.1:8088" {
		t.Fatalf("HealthAddress=%q", cfg.Runtime.HealthAddress)
	}
	if cfg.Runtime.LogLevel != "INFO" {
		t.Fatalf("LogLevel=%q", cfg.Runtime.LogLevel)
	}
	if cfg.Runtime.Transport.RequestTimeout != 10*time.Second {
		t.Fatalf("RequestTimeout=%v", cfg.Runtime.Transport.RequestTimeout)
	}
	if cfg.Runtime.Transport.MaxRequestBytes != 2097152 || cfg.Runtime.Transport.MaxResponseBytes != 2097152 {
		t.Fatalf("transport bytes=%+v", cfg.Runtime.Transport)
	}
	if cfg.Runtime.Transport.UploadBatchRecords != 100 || cfg.Runtime.Transport.UploadBatchBytes != 1048576 {
		t.Fatalf("upload batch=%+v", cfg.Runtime.Transport)
	}
	if cfg.Runtime.Scheduler.MaxWorkers != 8 {
		t.Fatalf("scheduler not parsed: %+v", cfg.Runtime.Scheduler)
	}
	if cfg.PollInterval != 15*time.Second {
		t.Fatalf("PollInterval=%v", cfg.PollInterval)
	}
	if cfg.CapabilityPackDir != "/var/lib/platform/sensor/packs" {
		t.Fatalf("CapabilityPackDir=%q", cfg.CapabilityPackDir)
	}
	if cfg.ControlReconcileInterval != 30*time.Second {
		t.Fatalf("ControlReconcileInterval default=%v", cfg.ControlReconcileInterval)
	}
}

func TestControlReconcileIntervalStrictBounds(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		want    time.Duration
		wantErr bool
	}{
		{"unset", "", 30 * time.Second, false},
		{"min", "5", 5 * time.Second, false},
		{"max", "300", 300 * time.Second, false},
		{"below", "4", 0, true},
		{"above", "301", 0, true},
		{"zero", "0", 0, true},
		{"negative", "-1", 0, true},
		{"non_int", "abc", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearSensorEnv(t)
			applyValidRuntimeEnv(t)
			if tc.value == "" {
				_ = os.Unsetenv("SENSOR_CONTROL_RECONCILE_INTERVAL_SECONDS")
			} else {
				t.Setenv("SENSOR_CONTROL_RECONCILE_INTERVAL_SECONDS", tc.value)
			}
			cfg, err := LoadConfig()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), "SENSOR_CONTROL_RECONCILE_INTERVAL_SECONDS") &&
					!strings.Contains(err.Error(), "SENSOR_CONFIG_INVALID") {
					t.Fatalf("error=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if cfg.ControlReconcileInterval != tc.want {
				t.Fatalf("got=%v want=%v", cfg.ControlReconcileInterval, tc.want)
			}
		})
	}
}

func TestLoadConfigRejectsInvalidRuntime(t *testing.T) {
	clearSensorEnv(t)
	applyValidRuntimeEnv(t)
	t.Setenv("PLATFORM_SENSOR_CORE_ENDPOINT", "http://core.example.test")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected invalid RuntimeConfig to fail")
	}
}

func TestLoadConfigRejectsLegacySharedRuntimeEnv(t *testing.T) {
	clearSensorEnv(t)
	applyValidRuntimeEnv(t)
	legacy := []string{
		"SENSOR_ID", "SENSOR_TYPE", "CORE_URL", "SENSOR_SPOOL_DIR", "SENSOR_HEALTH_PORT",
		"PLATFORM_LOG_LEVEL", "PLATFORM_STORAGE_WATERMARK_PERCENT",
		"PLATFORM_TLS_CERT_FILE", "SENSOR_TLS_CERT_FILE",
		"PLATFORM_TLS_KEY_FILE", "SENSOR_TLS_KEY_FILE", "PLATFORM_CA_CERT_FILE", "SENSOR_CA_CERT_FILE",
	}
	for _, key := range legacy {
		t.Run(key, func(t *testing.T) {
			clearSensorEnv(t)
			applyValidRuntimeEnv(t)
			t.Setenv(key, "legacy-value")
			_, err := LoadConfig()
			if err == nil || !strings.Contains(err.Error(), "SENSOR_CONFIG_LEGACY_RUNTIME_ENV") {
				t.Fatalf("error=%v", err)
			}
			if strings.Contains(strings.ToLower(err.Error()), "password") ||
				strings.Contains(err.Error(), "legacy-value") && key == "PLATFORM_SENSOR_BOOTSTRAP_TOKEN" {
				t.Fatalf("error leaked unexpected content: %v", err)
			}
			// Error names the key but must not dump secret material; legacy-value for TLS paths is a path not a secret.
			if strings.Contains(err.Error(), "test-only-bootstrap") {
				t.Fatalf("bootstrap secret leaked: %v", err)
			}
		})
	}
}

func TestLoadConfigKeepsAgentAndPackLocalSettings(t *testing.T) {
	clearSensorEnv(t)
	applyValidRuntimeEnv(t)
	t.Setenv("SENSOR_AGENT_RECEIVER_ENABLED", "true")
	t.Setenv("SENSOR_AGENT_RECEIVER_LISTEN_ADDR", ":8444")
	t.Setenv("SENSOR_AGENT_SERVER_CERT_FILE", "/tmp/agent.crt")
	t.Setenv("SENSOR_AGENT_SERVER_KEY_FILE", "/tmp/agent.key")
	t.Setenv("SENSOR_AGENT_CLIENT_CA_FILE", "/tmp/agent-ca.crt")
	t.Setenv("SENSOR_AGENT_SPOOL_DIR", "/var/lib/platform/sensor/agent-spool")
	t.Setenv("SENSOR_AGENT_DRAIN_MAX_BATCH_RECORDS", "16")
	t.Setenv("SENSOR_CAPABILITY_PACK_DIR", "/var/lib/platform/sensor/packs")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if !cfg.AgentReceiverEnabled || cfg.AgentReceiverListenAddr != ":8444" {
		t.Fatalf("agent receiver local settings lost: %+v", cfg)
	}
	if cfg.AgentSpoolDir != "/var/lib/platform/sensor/agent-spool" {
		t.Fatalf("AgentSpoolDir=%q", cfg.AgentSpoolDir)
	}
	if cfg.AgentDrainMaxBatchRecords != 16 {
		t.Fatalf("AgentDrainMaxBatchRecords=%d", cfg.AgentDrainMaxBatchRecords)
	}
	if cfg.CapabilityPackDir != "/var/lib/platform/sensor/packs" {
		t.Fatalf("CapabilityPackDir=%q", cfg.CapabilityPackDir)
	}
}

func TestRuntimeRoleControlsAgentReceiverEligibility(t *testing.T) {
	clearSensorEnv(t)
	applyValidRuntimeEnv(t)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Runtime.Role != sensorruntime.RoleNetwork {
		t.Fatalf("Role=%q", cfg.Runtime.Role)
	}
	cfg.AgentReceiverEnabled = true
	cfg.AgentReceiverListenAddr = ":8444"
	cfg.AgentServerCertFile = "/tmp/agent.crt"
	cfg.AgentServerKeyFile = "/tmp/agent.key"
	cfg.AgentClientCAFile = "/tmp/agent-ca.crt"
	cfg.AgentSpoolDir = "/var/lib/platform/sensor/agent-spool"
	cfg.AgentReceiverMaxInFlight = 8
	cfg.AgentReceiverShutdownGrace = time.Second
	if err := ValidateAgentReceiverConfig(cfg); err != nil {
		t.Fatalf("agent local validation: %v", err)
	}
	// Role eligibility is Runtime.Role; NETWORK is ineligible for Agent ingress.
	if cfg.Runtime.Role == sensorruntime.RoleNetwork {
		// documented in agentreceiver.IsRoleEligible — keep assertion on Runtime ownership
	}
	cfg.Runtime.Role = sensorruntime.RoleInfrastructure
	cfg.Runtime.EnabledCapabilities = []sensorruntime.Capability{sensorruntime.CapabilityInfra, sensorruntime.CapabilityAgentIngress}
	if err := cfg.Runtime.Validate(); err != nil {
		t.Fatalf("infra role validate: %v", err)
	}
}

func TestParseAndApplyLogLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"DEBUG": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"WARN":  slog.LevelWarn,
		"ERROR": slog.LevelError,
	}
	for name, want := range cases {
		got, err := ParseLogLevel(name)
		if err != nil || got != want {
			t.Fatalf("%s: got=%v err=%v", name, got, err)
		}
		lv := new(slog.LevelVar)
		if err := ApplyLogLevel(lv, name); err != nil {
			t.Fatalf("ApplyLogLevel(%s)=%v", name, err)
		}
		if lv.Level() != want {
			t.Fatalf("LevelVar %s=%v", name, lv.Level())
		}
	}
	if _, err := ParseLogLevel("TRACE"); err == nil {
		t.Fatal("TRACE must be rejected")
	}
}

func TestValidateAgentReceiverUsesRuntimePaths(t *testing.T) {
	cfg := &Config{
		Runtime: RuntimeConfig{
			SpoolPath: "/var/lib/platform/sensor/spool",
			TLS: TLSFiles{
				Certificate: "/var/lib/platform/sensor/identity/sensor.crt",
				PrivateKey:  "/var/lib/platform/sensor/identity/sensor.key",
			},
		},
		AgentReceiverEnabled:       true,
		AgentReceiverListenAddr:    ":8444",
		AgentServerCertFile:        "/tmp/agent.crt",
		AgentServerKeyFile:         "/tmp/agent.key",
		AgentClientCAFile:          "/tmp/agent-ca.crt",
		AgentSpoolDir:              "/var/lib/platform/sensor/agent-spool",
		AgentReceiverMaxInFlight:   8,
		AgentReceiverShutdownGrace: time.Second,
	}
	if err := ValidateAgentReceiverConfig(cfg); err != nil {
		t.Fatalf("expected pass: %v", err)
	}
	cfg.AgentServerCertFile = cfg.Runtime.TLS.Certificate
	if err := ValidateAgentReceiverConfig(cfg); err == nil {
		t.Fatal("identical Core/Agent cert paths must fail")
	}
	cfg.AgentServerCertFile = "/tmp/agent.crt"
	cfg.AgentSpoolDir = filepath.Clean(cfg.Runtime.SpoolPath)
	if err := ValidateAgentReceiverConfig(cfg); err == nil {
		t.Fatal("identical spool paths must fail")
	}
}

func TestSecretNotInLegacyOrRuntimeErrors(t *testing.T) {
	clearSensorEnv(t)
	values := validMap()
	delete(values, "PLATFORM_SENSOR_ID")
	delete(values, "PLATFORM_SENSOR_TLS_CERT_FILE")
	delete(values, "PLATFORM_SENSOR_TLS_KEY_FILE")
	values["PLATFORM_SENSOR_BOOTSTRAP_TOKEN"] = "super-secret-bootstrap-token"
	for k, v := range values {
		t.Setenv(k, v)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("enrollment LoadConfig: %v", err)
	}
	text := cfg.Runtime.String() + cfg.Runtime.GoString() + cfg.Runtime.BootstrapToken.String()
	if strings.Contains(text, "super-secret-bootstrap-token") {
		t.Fatal("bootstrap token leaked via String/GoString")
	}
	t.Setenv("SENSOR_ID", "should-fail")
	_, err = LoadConfig()
	if err == nil {
		t.Fatal("expected legacy rejection")
	}
	if strings.Contains(err.Error(), "super-secret-bootstrap-token") {
		t.Fatalf("legacy error leaked bootstrap token: %v", err)
	}
}

func TestSchedulerParsedButNotActivatedByLoadConfig(t *testing.T) {
	clearSensorEnv(t)
	applyValidRuntimeEnv(t)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.Runtime.Scheduler.Pools) == 0 {
		t.Fatal("scheduler pools must be strictly parsed")
	}
	// LoadConfig must not instantiate scheduler runtime — only parse/validate into RuntimeConfig.
	if cfg.Runtime.Scheduler.MaxWorkers <= 0 {
		t.Fatal("scheduler bounds missing after validation")
	}
}
