package config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	sensorruntime "platform/sensor/internal/runtime"
)

func validMap() map[string]string {
	return map[string]string{
		"PLATFORM_SENSOR_ID": "sensor-001",
		"PLATFORM_SENSOR_ROLE": "NETWORK",
		"PLATFORM_SENSOR_CORE_ENDPOINT": "https://core.example.test",
		"PLATFORM_SENSOR_SPOOL_PATH": "/var/lib/platform/sensor/spool",
		"PLATFORM_SENSOR_HEALTH_ADDRESS": "127.0.0.1:8088",
		"PLATFORM_SENSOR_TLS_CERT_FILE": "/var/lib/platform/sensor/identity/sensor.crt",
		"PLATFORM_SENSOR_TLS_KEY_FILE": "/var/lib/platform/sensor/identity/sensor.key",
		"PLATFORM_SENSOR_CA_FILE": "/var/lib/platform/sensor/identity/ca.crt",
		"PLATFORM_SENSOR_CONFIG_VERSION": "7",
		"PLATFORM_SENSOR_LOG_LEVEL": "INFO",
		"PLATFORM_SENSOR_ENABLED_CAPABILITIES": "SNMP,ICMP",
		"PLATFORM_SENSOR_STORAGE_WATERMARK_PERCENT": "80",
		"PLATFORM_SENSOR_SCHEDULER_MAX_WORKERS": "8",
		"PLATFORM_SENSOR_SCHEDULER_MAX_QUEUE_DEPTH": "16",
		"PLATFORM_SENSOR_SCHEDULER_MAX_TRACKED_ASSIGNMENTS": "64",
		"PLATFORM_SENSOR_SCHEDULER_MAX_TRACKED_JOBS": "24",
		"PLATFORM_SENSOR_SCHEDULER_MAX_PAYLOAD_BYTES": "1048576",
		"PLATFORM_SENSOR_SCHEDULER_DEFAULT_EXECUTION_TIMEOUT": "10s",
		"PLATFORM_SENSOR_SCHEDULER_SHUTDOWN_GRACE": "5s",
		"PLATFORM_SENSOR_SCHEDULER_RETRY_MAX_ATTEMPTS": "3",
		"PLATFORM_SENSOR_SCHEDULER_RETRY_INITIAL_BACKOFF": "100ms",
		"PLATFORM_SENSOR_SCHEDULER_RETRY_MAX_BACKOFF": "2s",
		"PLATFORM_SENSOR_SCHEDULER_RETRY_JITTER": "0.2",
		"PLATFORM_SENSOR_SCHEDULER_POOLS_JSON": `{"NETWORK_POLL":{"workers":4,"queueDepth":8},"STREAMING":{"workers":2,"queueDepth":4}}`,
		"PLATFORM_SENSOR_TRANSPORT_REQUEST_TIMEOUT": "10s",
		"PLATFORM_SENSOR_TRANSPORT_MAX_REQUEST_BYTES": "2097152",
		"PLATFORM_SENSOR_TRANSPORT_MAX_RESPONSE_BYTES": "2097152",
		"PLATFORM_SENSOR_UPLOAD_BATCH_RECORDS": "100",
		"PLATFORM_SENSOR_UPLOAD_BATCH_BYTES": "1048576",
	}
}

func TestLoadMapValidOperationalConfig(t *testing.T) {
	cfg, err := LoadMap(validMap())
	if err != nil { t.Fatalf("LoadMap() error = %v", err) }
	if cfg.Role != sensorruntime.RoleNetwork || cfg.ConfigurationVersion != 7 { t.Fatalf("unexpected config: %+v", cfg) }
	if cfg.StorageWatermarkPercent != 80 { t.Fatalf("watermark=%d", cfg.StorageWatermarkPercent) }
}

func TestLoadMapValidEnrollmentConfig(t *testing.T) {
	values := validMap()
	delete(values, "PLATFORM_SENSOR_ID"); delete(values, "PLATFORM_SENSOR_TLS_CERT_FILE"); delete(values, "PLATFORM_SENSOR_TLS_KEY_FILE")
	values["PLATFORM_SENSOR_BOOTSTRAP_TOKEN"] = "test-only-bootstrap"
	cfg, err := LoadMap(values)
	if err != nil { t.Fatalf("LoadMap() error = %v", err) }
	if !cfg.Snapshot().EnrollmentMode || cfg.SensorID != "" { t.Fatalf("expected enrollment mode: %+v", cfg.Snapshot()) }
}

func TestUnknownSensorFieldRejected(t *testing.T) {
	values := validMap(); values["PLATFORM_SENSOR_MADE_UP_SETTING"] = "1"
	if _, err := LoadMap(values); err == nil || !strings.Contains(err.Error(), "SENSOR_CONFIG_UNKNOWN_FIELD") { t.Fatalf("error=%v", err) }
}

func TestFixedWatermarkCannotBeChanged(t *testing.T) {
	for _, value := range []string{"79", "81", "0", "95"} {
		values := validMap(); values["PLATFORM_SENSOR_STORAGE_WATERMARK_PERCENT"] = value
		if _, err := LoadMap(values); err == nil || !strings.Contains(err.Error(), "STORAGE_PROTECTION_FIXED") { t.Fatalf("watermark %s error=%v", value, err) }
	}
}

func TestCoreEndpointRequiresHTTPSOrigin(t *testing.T) {
	for _, endpoint := range []string{"http://core.example.test", "https://user:pass@core.example.test", "https://core.example.test/path?x=1", "core.example.test"} {
		values := validMap(); values["PLATFORM_SENSOR_CORE_ENDPOINT"] = endpoint
		if _, err := LoadMap(values); err == nil { t.Fatalf("endpoint %q accepted", endpoint) }
	}
}

func TestIdentityModeMustBeUnambiguous(t *testing.T) {
	values := validMap(); values["PLATFORM_SENSOR_BOOTSTRAP_TOKEN"] = "test-only-bootstrap"
	if _, err := LoadMap(values); err == nil || !strings.Contains(err.Error(), "IDENTITY_CONFLICT") { t.Fatalf("error=%v", err) }
	values = validMap(); delete(values, "PLATFORM_SENSOR_ID"); delete(values, "PLATFORM_SENSOR_TLS_CERT_FILE"); delete(values, "PLATFORM_SENSOR_TLS_KEY_FILE")
	if _, err := LoadMap(values); err == nil || !strings.Contains(err.Error(), "IDENTITY_REQUIRED") { t.Fatalf("error=%v", err) }
}

func TestTLSKeypairMustBeComplete(t *testing.T) {
	values := validMap(); delete(values, "PLATFORM_SENSOR_TLS_KEY_FILE")
	if _, err := LoadMap(values); err == nil || !strings.Contains(err.Error(), "TLS_KEYPAIR_INCOMPLETE") { t.Fatalf("error=%v", err) }
}

func TestRoleCapabilityValidation(t *testing.T) {
	values := validMap(); values["PLATFORM_SENSOR_ROLE"] = "SYNTHETIC"; values["PLATFORM_SENSOR_ENABLED_CAPABILITIES"] = "SNMP"
	if _, err := LoadMap(values); err == nil || !strings.Contains(err.Error(), "INVALID_ROLE_CAPABILITIES") { t.Fatalf("error=%v", err) }
}

func TestSchedulerPoolBoundsValidated(t *testing.T) {
	values := validMap(); values["PLATFORM_SENSOR_SCHEDULER_POOLS_JSON"] = `{"NETWORK_POLL":{"workers":9,"queueDepth":8}}`
	if _, err := LoadMap(values); err == nil || !strings.Contains(err.Error(), "POOL_BOUNDS_EXCEEDED") { t.Fatalf("error=%v", err) }
}

func TestSchedulerPoolJSONRejectsUnknownFieldsAndTrailingDocument(t *testing.T) {
	for _, raw := range []string{`{"NETWORK_POLL":{"workers":1,"queueDepth":1,"mystery":1}}`, `{"NETWORK_POLL":{"workers":1,"queueDepth":1}} {}`} {
		values := validMap(); values["PLATFORM_SENSOR_SCHEDULER_POOLS_JSON"] = raw
		if _, err := LoadMap(values); err == nil { t.Fatalf("scheduler pool JSON accepted: %s", raw) }
	}
}

func TestNoImplicitSchedulerDefaults(t *testing.T) {
	values := validMap(); delete(values, "PLATFORM_SENSOR_SCHEDULER_MAX_WORKERS")
	if _, err := LoadMap(values); err == nil { t.Fatal("missing scheduler worker bound was defaulted") }
}

func TestTransportBatchCannotExceedRequestBound(t *testing.T) {
	values := validMap(); values["PLATFORM_SENSOR_UPLOAD_BATCH_BYTES"] = "3000000"
	if _, err := LoadMap(values); err == nil || !strings.Contains(err.Error(), "UPLOAD_BOUND_INVALID") { t.Fatalf("error=%v", err) }
}

func TestSecretAndConfigRenderingAreRedacted(t *testing.T) {
	cfg, err := LoadMap(validMap()); if err != nil { t.Fatal(err) }
	cfg.BootstrapToken = Secret("super-secret-token")
	renderings := []string{fmt.Sprintf("%v", cfg.BootstrapToken), fmt.Sprintf("%+v", cfg.BootstrapToken), fmt.Sprintf("%#v", cfg.BootstrapToken), cfg.String(), fmt.Sprintf("%+v", cfg.Snapshot())}
	for _, rendered := range renderings { if strings.Contains(rendered, "super-secret-token") { t.Fatalf("secret leaked in %q", rendered) } }
}

func TestSafeSnapshotCopiesMutableConfiguration(t *testing.T) {
	cfg, err := LoadMap(validMap()); if err != nil { t.Fatal(err) }
	snap := cfg.Snapshot(); snap.EnabledCapabilities[0] = "MUTATED"; snap.Scheduler.Pools["NETWORK_POLL"] = PoolConfig{Workers: 999, QueueDepth: 999}
	if cfg.EnabledCapabilities[0] == "MUTATED" || cfg.Scheduler.Pools["NETWORK_POLL"].Workers == 999 { t.Fatal("safe snapshot aliases mutable config") }
}

func TestDiffClassifiesReloadableAndRestartRequiredChanges(t *testing.T) {
	oldCfg, _ := LoadMap(validMap()); newCfg := oldCfg; newCfg.ConfigurationVersion++; newCfg.LogLevel = "DEBUG"; newCfg.CoreEndpoint = "https://core2.example.test"
	changes := Diff(oldCfg, newCfg); got := map[string]ChangeClass{}; for _, change := range changes { got[change.Field] = change.Class }
	if got["configuration_version"] != ChangeReloadable || got["log_level"] != ChangeReloadable || got["core_endpoint"] != ChangeRestartRequired { t.Fatalf("changes=%v", changes) }
}

func TestDeterministicCapabilityParsing(t *testing.T) {
	values := validMap(); values["PLATFORM_SENSOR_ENABLED_CAPABILITIES"] = " snmp , ICMP "
	cfg, err := LoadMap(values); if err != nil { t.Fatal(err) }
	want := []sensorruntime.Capability{sensorruntime.CapabilitySNMP, sensorruntime.CapabilityICMP}
	if !reflect.DeepEqual(cfg.EnabledCapabilities, want) { t.Fatalf("capabilities=%v want=%v", cfg.EnabledCapabilities, want) }
}
