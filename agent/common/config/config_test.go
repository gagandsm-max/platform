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
	if cfg.CoreURL != "https://core.platform.local:8443" {
		t.Fatalf("core url mismatch: %s", cfg.CoreURL)
	}

	// 2. Cleartext HTTP rejected
	httpEnv := append([]string{}, baseEnv...)
	httpEnv[1] = "PLATFORM_AGENT_CORE_URL=http://core.platform.local:8443"
	_, err = LoadConfigFromEnv(httpEnv)
	if err == nil || !strings.Contains(err.Error(), "SCHEME_INSECURE") {
		t.Fatalf("expected SCHEME_INSECURE error, got %v", err)
	}

	// 3. Userinfo/credentials in URL rejected
	credEnv := append([]string{}, baseEnv...)
	credEnv[1] = "PLATFORM_AGENT_CORE_URL=https://user:pass@core.platform.local:8443"
	_, err = LoadConfigFromEnv(credEnv)
	if err == nil || !strings.Contains(err.Error(), "CREDENTIALS_PROHIBITED") {
		t.Fatalf("expected CREDENTIALS_PROHIBITED error, got %v", err)
	}

	// 4. URL path rejected (origin only)
	pathEnv := append([]string{}, baseEnv...)
	pathEnv[1] = "PLATFORM_AGENT_CORE_URL=https://core.platform.local:8443/api/v1/telemetry"
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
}
