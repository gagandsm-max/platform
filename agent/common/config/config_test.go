package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func generateTestCertAndKey(t *testing.T, dir string) (string, string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "agent.platform.local",
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(time.Hour),
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}

	certPath := filepath.Join(dir, "agent.crt")
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_ = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	_ = certFile.Close()

	keyPath := filepath.Join(dir, "agent.key")
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	_ = pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	_ = keyFile.Close()

	return certPath, keyPath
}

func TestConfigValidation(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := generateTestCertAndKey(t, dir)

	baseEnv := []string{
		"PLATFORM_AGENT_TENANT_ID=tenant-123",
		"PLATFORM_AGENT_ID=agent-123",
		"PLATFORM_AGENT_SENSOR_URL=https://sensor.platform.local:8443",
		"PLATFORM_AGENT_CORE_URL=https://core.platform.local:8443",
		"PLATFORM_AGENT_CA_FILE=" + certPath,
		"PLATFORM_AGENT_CERT_FILE=" + certPath,
		"PLATFORM_AGENT_KEY_FILE=" + keyPath,
	}

	// 1. Valid base config
	cfg, err := LoadConfigFromEnv(baseEnv)
	if err != nil {
		t.Fatalf("unexpected error loading valid config: %v", err)
	}
	if cfg.TenantID != "tenant-123" {
		t.Fatalf("tenant mismatch: %s", cfg.TenantID)
	}
	if cfg.AgentID != "agent-123" {
		t.Fatalf("agent id mismatch: %s", cfg.AgentID)
	}
	if cfg.SensorURL != "https://sensor.platform.local:8443" {
		t.Fatalf("sensor url mismatch: %s", cfg.SensorURL)
	}
	if cfg.CoreURL != "https://core.platform.local:8443" {
		t.Fatalf("core url mismatch: %s", cfg.CoreURL)
	}
	if cfg.MaxBatchBytes != 384*1024 {
		t.Fatalf("default MaxBatchBytes must stay under Core 512KiB record limit, got %d", cfg.MaxBatchBytes)
	}
	if cfg.EBPFEnabled || cfg.EBPFPreflightTimeout != 2*time.Second || cfg.EBPFRecheckMin != 5*time.Minute || cfg.EBPFRecheckMax != time.Hour {
		t.Fatalf("unexpected safe eBPF defaults: enabled=%v timeout=%v min=%v max=%v", cfg.EBPFEnabled, cfg.EBPFPreflightTimeout, cfg.EBPFRecheckMin, cfg.EBPFRecheckMax)
	}

	// 2. Cleartext HTTP rejected for Sensor telemetry URL
	httpEnv := append([]string{}, baseEnv...)
	httpEnv[2] = "PLATFORM_AGENT_SENSOR_URL=http://sensor.platform.local:8443"
	_, err = LoadConfigFromEnv(httpEnv)
	if err == nil || !strings.Contains(err.Error(), "SCHEME_INSECURE") {
		t.Fatalf("expected SCHEME_INSECURE error, got %v", err)
	}

	// 3. Userinfo/credentials in Sensor URL rejected
	credEnv := append([]string{}, baseEnv...)
	credEnv[2] = "PLATFORM_AGENT_SENSOR_URL=https://user:pass@sensor.platform.local:8443"
	_, err = LoadConfigFromEnv(credEnv)
	if err == nil || !strings.Contains(err.Error(), "CREDENTIALS_PROHIBITED") {
		t.Fatalf("expected CREDENTIALS_PROHIBITED error, got %v", err)
	}
	if strings.Contains(err.Error(), "pass") || strings.Contains(err.Error(), "user:pass") {
		t.Fatalf("config error must not echo URL credentials: %v", err)
	}

	httpCore := append([]string{}, baseEnv...)
	httpCore[3] = "PLATFORM_AGENT_CORE_URL=http://core.platform.local:8443"
	_, err = LoadConfigFromEnv(httpCore)
	if err == nil || !strings.Contains(err.Error(), "SCHEME_INSECURE") {
		t.Fatalf("expected SCHEME_INSECURE for Core URL, got %v", err)
	}

	// 4. Sensor URL path rejected (origin only)
	pathEnv := append([]string{}, baseEnv...)
	pathEnv[2] = "PLATFORM_AGENT_SENSOR_URL=https://sensor.platform.local:8443/api/v1/agent/telemetry"
	_, err = LoadConfigFromEnv(pathEnv)
	if err == nil || !strings.Contains(err.Error(), "PATH_PROHIBITED") {
		t.Fatalf("expected PATH_PROHIBITED error, got %v", err)
	}

	// 5. Unknown PLATFORM_AGENT_* rejected fail-closed
	unknownEnv := append(baseEnv, "PLATFORM_AGENT_ROGUE_SETTING=dangerous")
	_, err = LoadConfigFromEnv(unknownEnv)
	if err == nil || !strings.Contains(err.Error(), "CONFIG_UNKNOWN_VARIABLE") {
		t.Fatalf("expected CONFIG_UNKNOWN_VARIABLE error, got %v", err)
	}

	// 6. Insecure key permissions rejected
	_ = os.Chmod(keyPath, 0666)
	_, err = LoadConfigFromEnv(baseEnv)
	if err == nil || !strings.Contains(err.Error(), "private key permissions are insecure") {
		t.Fatalf("expected insecure key permissions error, got %v", err)
	}
	_ = os.Chmod(keyPath, 0600)

	// 7. Non-loopback health address rejected
	badHealth := append(baseEnv, "PLATFORM_AGENT_HEALTH_ADDRESS=0.0.0.0:8089")
	_, err = LoadConfigFromEnv(badHealth)
	if err == nil || !strings.Contains(err.Error(), "HEALTH_ADDRESS_INSECURE") {
		t.Fatalf("expected HEALTH_ADDRESS_INSECURE error, got %v", err)
	}

	// 8. Missing Sensor URL fails closed
	noSensor := []string{
		"PLATFORM_AGENT_TENANT_ID=tenant-123",
		"PLATFORM_AGENT_ID=agent-123",
		"PLATFORM_AGENT_CA_FILE=" + certPath,
		"PLATFORM_AGENT_CERT_FILE=" + certPath,
		"PLATFORM_AGENT_KEY_FILE=" + keyPath,
	}
	_, err = LoadConfigFromEnv(noSensor)
	if err == nil || !strings.Contains(err.Error(), "CONFIG_SENSOR_URL_REQUIRED") {
		t.Fatalf("expected CONFIG_SENSOR_URL_REQUIRED, got %v", err)
	}

	// 9. Missing Agent ID fails closed
	noID := append([]string{}, baseEnv...)
	noID[1] = "PLATFORM_AGENT_ID="
	_, err = LoadConfigFromEnv(noID)
	if err == nil || !strings.Contains(err.Error(), "CONFIG_AGENT_ID_REQUIRED") {
		t.Fatalf("expected CONFIG_AGENT_ID_REQUIRED, got %v", err)
	}

	// 10. Optional Core URL may be omitted; telemetry still uses Sensor URL
	noCore := []string{
		"PLATFORM_AGENT_TENANT_ID=tenant-123",
		"PLATFORM_AGENT_ID=agent-123",
		"PLATFORM_AGENT_SENSOR_URL=https://sensor.platform.local:8443",
		"PLATFORM_AGENT_CA_FILE=" + certPath,
		"PLATFORM_AGENT_CERT_FILE=" + certPath,
		"PLATFORM_AGENT_KEY_FILE=" + keyPath,
	}
	cfg, err = LoadConfigFromEnv(noCore)
	if err != nil {
		t.Fatalf("core URL should be optional: %v", err)
	}
	if cfg.SensorURL != "https://sensor.platform.local:8443" {
		t.Fatalf("sensor url mismatch without core: %s", cfg.SensorURL)
	}
	if cfg.CoreURL != "" {
		t.Fatalf("expected empty core url, got %s", cfg.CoreURL)
	}

	// 11. Batch-size contract: valid maximum is accepted; first invalid value fails closed.
	maxEnv := append([]string{}, baseEnv...)
	maxEnv = append(maxEnv, fmt.Sprintf("PLATFORM_AGENT_MAX_BATCH_BYTES=%d", MaxConfiguredBatchBytes))
	cfg, err = LoadConfigFromEnv(maxEnv)
	if err != nil {
		t.Fatalf("valid maximum MaxBatchBytes must load: %v", err)
	}
	if cfg.MaxBatchBytes != MaxConfiguredBatchBytes {
		t.Fatalf("expected MaxBatchBytes=%d, got %d", MaxConfiguredBatchBytes, cfg.MaxBatchBytes)
	}

	overEnv := append([]string{}, baseEnv...)
	overEnv = append(overEnv, fmt.Sprintf("PLATFORM_AGENT_MAX_BATCH_BYTES=%d", MaxConfiguredBatchBytes+1))
	_, err = LoadConfigFromEnv(overEnv)
	if err == nil || !strings.Contains(err.Error(), "CONFIG_MAX_BATCH_BYTES_OUT_OF_BOUNDS") {
		t.Fatalf("first invalid MaxBatchBytes must fail closed, got %v", err)
	}

	legacyTenMB := append([]string{}, baseEnv...)
	legacyTenMB = append(legacyTenMB, "PLATFORM_AGENT_MAX_BATCH_BYTES=10485760")
	_, err = LoadConfigFromEnv(legacyTenMB)
	if err == nil || !strings.Contains(err.Error(), "CONFIG_MAX_BATCH_BYTES_OUT_OF_BOUNDS") {
		t.Fatalf("10MiB MaxBatchBytes must no longer be configurable, got %v", err)
	}

	justBelow := append([]string{}, baseEnv...)
	justBelow = append(justBelow, fmt.Sprintf("PLATFORM_AGENT_MAX_BATCH_BYTES=%d", MaxConfiguredBatchBytes-1))
	cfg, err = LoadConfigFromEnv(justBelow)
	if err != nil {
		t.Fatalf("just-below-max MaxBatchBytes must load: %v", err)
	}
	if cfg.MaxBatchBytes != MaxConfiguredBatchBytes-1 {
		t.Fatalf("expected MaxBatchBytes=%d, got %d", MaxConfiguredBatchBytes-1, cfg.MaxBatchBytes)
	}

	overRecord := append([]string{}, baseEnv...)
	overRecord = append(overRecord, fmt.Sprintf("PLATFORM_AGENT_MAX_RECORD_BYTES=%d", MaxConfiguredRecordBytes+1))
	_, err = LoadConfigFromEnv(overRecord)
	if err == nil || !strings.Contains(err.Error(), "CONFIG_MAX_RECORD_BYTES_OUT_OF_BOUNDS") {
		t.Fatalf("MaxRecordBytes above wrap-safe cap must fail closed, got %v", err)
	}

	// 12. Preflight-only eBPF config is bounded and strict.
	ebpfEnv := append([]string{}, baseEnv...)
	ebpfEnv = append(ebpfEnv,
		"PLATFORM_AGENT_EBPF_ENABLED=true",
		"PLATFORM_AGENT_EBPF_PREFLIGHT_TIMEOUT=750ms",
		"PLATFORM_AGENT_EBPF_RECHECK_MIN=2m",
		"PLATFORM_AGENT_EBPF_RECHECK_MAX=30m",
	)
	cfg, err = LoadConfigFromEnv(ebpfEnv)
	if err != nil {
		t.Fatalf("valid eBPF preflight config: %v", err)
	}
	if !cfg.EBPFEnabled || cfg.EBPFPreflightTimeout != 750*time.Millisecond || cfg.EBPFRecheckMin != 2*time.Minute || cfg.EBPFRecheckMax != 30*time.Minute {
		t.Fatalf("eBPF preflight config mismatch: %+v", cfg)
	}
	for _, setting := range []string{
		"PLATFORM_AGENT_EBPF_ENABLED=1",
		"PLATFORM_AGENT_EBPF_PREFLIGHT_TIMEOUT=99ms",
		"PLATFORM_AGENT_EBPF_RECHECK_MIN=30s",
		"PLATFORM_AGENT_EBPF_RECHECK_MAX=25h",
	} {
		bad := append([]string{}, baseEnv...)
		bad = append(bad, setting)
		if _, err := LoadConfigFromEnv(bad); err == nil {
			t.Fatalf("expected bounded config rejection for %s", setting)
		}
	}
	badBounds := append([]string{}, baseEnv...)
	badBounds = append(badBounds, "PLATFORM_AGENT_EBPF_RECHECK_MIN=2h", "PLATFORM_AGENT_EBPF_RECHECK_MAX=1h")
	if _, err := LoadConfigFromEnv(badBounds); err == nil || !strings.Contains(err.Error(), "CONFIG_EBPF_RECHECK_BOUNDS_INVALID") {
		t.Fatalf("expected eBPF recheck bounds rejection, got %v", err)
	}
}
