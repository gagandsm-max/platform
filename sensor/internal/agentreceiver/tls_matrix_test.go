package agentreceiver

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"platform/sensor/internal/runtime"
)

func TestRealTLSClientCertMatrix(t *testing.T) {
	caCert, caKey := mustCreateCA(t)
	serverTLS := mustServerTLS(t, caCert, caKey)
	trustedClientTLS, trustedCN := mustClientTLS(t, caCert, caKey, "agent-trusted")
	untrustedClientTLS := mustUntrustedClientTLS(t, caCert, "agent-untrusted")

	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName(trustedCN, "agent-trusted")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-trusted", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	rcv, err := NewReceiver(Config{
		SensorID:   "sensor-infra-01",
		SensorRole: runtime.RoleInfrastructure,
	}, ident, auth, sink, logger)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewUnstartedServer(rcv)
	ts.TLS = serverTLS
	ts.StartTLS()
	defer ts.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-trusted", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))

	t.Run("trusted_valid_mapped_proceeds", func(t *testing.T) {
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: trustedClientTLS}}
		resp, err := client.Post(ts.URL+EndpointPath, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if sink.BatchCount() < 1 {
			t.Fatal("expected assignment path to reach sink")
		}
	})

	t.Run("untrusted_client_tls_reject", func(t *testing.T) {
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: untrustedClientTLS}}
		_, err := client.Post(ts.URL+EndpointPath, "application/json", bytes.NewReader(body))
		if err == nil {
			t.Fatal("expected TLS handshake failure for untrusted client cert")
		}
	})

	t.Run("no_client_cert_reject", func(t *testing.T) {
		noClient := trustedClientTLS.Clone()
		noClient.Certificates = nil
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: noClient}}
		_, err := client.Post(ts.URL+EndpointPath, "application/json", bytes.NewReader(body))
		if err == nil {
			t.Fatal("expected TLS failure when client certificate is absent")
		}
	})

	t.Run("trusted_unmapped_identity_reject", func(t *testing.T) {
		otherTLS, _ := mustClientTLS(t, caCert, caKey, "agent-unmapped")
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: otherTLS}}
		resp, err := client.Post(ts.URL+EndpointPath, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 for unmapped identity, got %d", resp.StatusCode)
		}
	})

	t.Run("trusted_mapped_reaches_assignment", func(t *testing.T) {
		auth.Revoke("agent-trusted", "sensor-infra-01")
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: trustedClientTLS}}
		resp, err := client.Post(ts.URL+EndpointPath, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected assignment denial after identity success, got %d", resp.StatusCode)
		}
		var errResp ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Code != ErrCodeAgentNotAssigned {
			t.Fatalf("expected %s, got %s", ErrCodeAgentNotAssigned, errResp.Code)
		}
	})
}

func mustCreateCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Receiver-Test-CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func mustServerTLS(t *testing.T, caCert *x509.Certificate, caKey *rsa.PrivateKey) *tls.Config {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}
}

func mustClientTLS(t *testing.T, caCert *x509.Certificate, caKey *rsa.PrivateKey, cn string) (*tls.Config, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	return &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		MinVersion:   tls.VersionTLS12,
	}, cn
}

// mustUntrustedClientTLS builds a client that trusts serverCA for the server cert
// but presents a client certificate signed by a different CA.
func mustUntrustedClientTLS(t *testing.T, serverCA *x509.Certificate, cn string) *tls.Config {
	t.Helper()
	otherCA, otherKey := mustCreateCA(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(99),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, otherCA, &key.PublicKey, otherKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(serverCA)
	return &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		MinVersion:   tls.VersionTLS12,
	}
}
