package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
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
}
