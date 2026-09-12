package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"platform/agent/common/queue"
)

func generateTestCertificates(t *testing.T, commonName string) (caPEM, certPEM, keyPEM []byte, tlsConfig *tls.Config, serverTLS *tls.Config) {
	now := time.Now()
	return generateTestCertificatesWindow(t, commonName, now.Add(-1*time.Hour), now.Add(24*time.Hour))
}

func generateTestCertificatesWindow(t *testing.T, commonName string, clientNotBefore, clientNotAfter time.Time) (caPEM, certPEM, keyPEM []byte, tlsConfig *tls.Config, serverTLS *tls.Config) {
	t.Helper()

	caPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Platform-Dev-Test-CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatalf("failed to create CA cert: %v", err)
	}
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caBytes})

	serverPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate server key: %v", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "127.0.0.1",
		},
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	serverBytes, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatalf("failed to create server cert: %v", err)
	}
	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverBytes})
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverPriv)})

	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("failed to parse server keypair: %v", err)
	}

	clientPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate client key: %v", err)
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:   clientNotBefore,
		NotAfter:    clientNotAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientBytes, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatalf("failed to create client cert: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientBytes})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientPriv)})

	clientCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed to parse client keypair: %v", err)
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)

	clientTLSConfig := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{clientCert},
		MinVersion:   tls.VersionTLS12,
	}

	serverTLSConfig := &tls.Config{
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{serverCert},
		MinVersion:   tls.VersionTLS12,
	}

	return caPEM, certPEM, keyPEM, clientTLSConfig, serverTLSConfig
}

func writePEMFiles(t *testing.T, caPEM, certPEM, keyPEM []byte) (caPath, certPath, keyPath string) {
	t.Helper()
	dir := t.TempDir()
	caPath = filepath.Join(dir, "ca.pem")
	certPath = filepath.Join(dir, "client.pem")
	keyPath = filepath.Join(dir, "client.key")
	if err := os.WriteFile(caPath, caPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	return caPath, certPath, keyPath
}

func TestHTTPSConfigValidation(t *testing.T) {
	_, err := NewHTTPSTransport(HTTPSConfig{
		SensorURL: "http://127.0.0.1:8080",
		AgentID:   "agent-1",
		TenantID:  "tenant-1",
	})
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_SCHEME_INSECURE") {
		t.Errorf("expected DELIVERY_SCHEME_INSECURE for http URL, got %v", err)
	}

	_, err = NewHTTPSTransport(HTTPSConfig{
		SensorURL: "https://user:pass@127.0.0.1:8443",
		AgentID:   "agent-1",
		TenantID:  "tenant-1",
	})
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_URL_INSECURE") {
		t.Errorf("expected DELIVERY_URL_INSECURE for userinfo in URL, got %v", err)
	}

	_, err = NewHTTPSTransport(HTTPSConfig{
		SensorURL: "https://127.0.0.1:8443",
		AgentID:   "",
		TenantID:  "tenant-1",
	})
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_CONFIG_INVALID") {
		t.Errorf("expected DELIVERY_CONFIG_INVALID for empty AgentID, got %v", err)
	}
}

func TestEnforceTLSConstraintsRejectsInsecureSkip(t *testing.T) {
	_, _, _, clientTLS, _ := generateTestCertificates(t, "agent-01")
	clientTLS.InsecureSkipVerify = true
	_, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: "https://127.0.0.1:8443",
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_TLS_INSECURE") {
		t.Fatalf("expected insecure skip rejection, got %v", err)
	}
}

func TestEnforceTLSConstraintsRejectsTLSBelow12(t *testing.T) {
	_, _, _, clientTLS, _ := generateTestCertificates(t, "agent-01")
	clientTLS.MinVersion = tls.VersionTLS10
	_, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: "https://127.0.0.1:8443",
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_TLS_INSECURE") {
		t.Fatalf("expected TLS <1.2 rejection, got %v", err)
	}
}

func TestEnforceTLSConstraintsRejectsMissingClientCert(t *testing.T) {
	_, _, _, clientTLS, _ := generateTestCertificates(t, "agent-01")
	clientTLS.Certificates = nil
	_, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: "https://127.0.0.1:8443",
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_TLS_MISSING") {
		t.Fatalf("expected missing client cert rejection, got %v", err)
	}
}

func TestEnforceTLSConstraintsRejectsMissingRootCAs(t *testing.T) {
	_, _, _, clientTLS, _ := generateTestCertificates(t, "agent-01")
	clientTLS.RootCAs = nil
	_, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: "https://127.0.0.1:8443",
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_TLS_MISSING") {
		t.Fatalf("expected missing RootCAs rejection, got %v", err)
	}
}

func TestNewHTTPSTransportFromFiles(t *testing.T) {
	caPEM, certPEM, keyPEM, _, _ := generateTestCertificates(t, "agent-01")
	caPath, certPath, keyPath := writePEMFiles(t, caPEM, certPEM, keyPEM)

	tr, err := NewHTTPSTransport(HTTPSConfig{
		SensorURL:      "https://127.0.0.1:8443",
		AgentID:        "agent-01",
		TenantID:       "tenant-01",
		CACertFile:     caPath,
		ClientCertFile: certPath,
		ClientKeyFile:  keyPath,
	})
	if err != nil {
		t.Fatalf("valid CA+client cert should succeed: %v", err)
	}
	if tr == nil {
		t.Fatal("expected transport")
	}

	_, err = NewHTTPSTransport(HTTPSConfig{
		SensorURL:      "https://127.0.0.1:8443",
		AgentID:        "agent-01",
		TenantID:       "tenant-01",
		CACertFile:     caPath,
		ClientCertFile: certPath,
		ClientKeyFile:  keyPath + ".missing",
	})
	if err == nil {
		t.Fatal("expected failure for missing key file")
	}

	badCA := filepath.Join(t.TempDir(), "bad-ca.pem")
	if err := os.WriteFile(badCA, []byte("not-a-cert"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = NewHTTPSTransport(HTTPSConfig{
		SensorURL:      "https://127.0.0.1:8443",
		AgentID:        "agent-01",
		TenantID:       "tenant-01",
		CACertFile:     badCA,
		ClientCertFile: certPath,
		ClientKeyFile:  keyPath,
	})
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_CA_PARSE_FAILED") {
		t.Fatalf("expected bad CA rejection, got %v", err)
	}
}

func ackHandler(t *testing.T, agentID string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/telemetry" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var env DeliveryEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Errorf("failed to decode envelope: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeliveryAck{
			Status:                  "ACCEPTED",
			AcceptedThroughSequence: env.LastSequence,
			AgentID:                 agentID,
			BatchID:                 env.BatchID,
			CorrelationID:           env.CorrelationID,
		})
	}
}

func testBatch(agentID string, recs ...queue.Record) Batch {
	return Batch{
		CorrelationID: "test-corr-1",
		AgentID:       agentID,
		BatchID:       BuildBatchID(agentID, recs),
		Records:       recs,
	}
}

func TestHTTPSDeliverSuccess(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")

	server := httptest.NewUnstartedServer(ackHandler(t, "agent-01"))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatalf("failed to create transport: %v", err)
	}

	recs := []queue.Record{
		{Sequence: 9, ID: "r-9", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{"x":1}`)},
		{Sequence: 10, ID: "r-10", CreatedAt: time.Now().UTC(), Checksum: "def", Payload: []byte(`{"x":2}`)},
	}
	batch := Batch{
		CorrelationID: "test-corr-1",
		AgentID:       "agent-01",
		BatchID:       BuildBatchID("agent-01", recs),
		Records:       recs,
	}

	ack, err := tr.Deliver(context.Background(), batch)
	if err != nil {
		t.Fatalf("deliver failed: %v", err)
	}
	if ack.AcceptedThrough != 10 {
		t.Errorf("expected accepted through 10, got %d", ack.AcceptedThrough)
	}
	if ack.BatchID == "" || ack.AgentID != "agent-01" {
		t.Errorf("expected batch-bound ACK identity, got %+v", ack)
	}
}

func TestHTTPSDeliverUsesConfiguredTenant(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")
	var gotTenant string
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env DeliveryEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Errorf("decode: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotTenant = env.TenantID
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeliveryAck{
			Status:                  "ACCEPTED",
			AcceptedThroughSequence: env.LastSequence,
			AgentID:                 env.AgentID,
			BatchID:                 env.BatchID,
			CorrelationID:           env.CorrelationID,
		})
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-config-authority",
	}, clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{"tenantId":"forged-tenant"}`)})); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if gotTenant != "tenant-config-authority" {
		t.Fatalf("envelope tenant must come from transport config, got %q", gotTenant)
	}
}

func TestHTTPSDeliverRejectsBatchMismatch(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeliveryAck{
			Status:                  "ACCEPTED",
			AcceptedThroughSequence: 1,
			AgentID:                 "agent-01",
			BatchID:                 "wrong-batch",
			CorrelationID:           "c1",
		})
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{}`)}))
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_ACK_BATCH_MISMATCH") {
		t.Fatalf("expected batch mismatch, got %v", err)
	}
}

func TestHTTPSDeliverRejectsTrailingJSON(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env DeliveryEnvelope
		_ = json.NewDecoder(r.Body).Decode(&env)
		w.Header().Set("Content-Type", "application/json")
		ack, _ := json.Marshal(DeliveryAck{
			Status:                  "ACCEPTED",
			AcceptedThroughSequence: env.LastSequence,
			AgentID:                 "agent-01",
			BatchID:                 env.BatchID,
			CorrelationID:           env.CorrelationID,
		})
		_, _ = w.Write(append(ack, []byte(`{"extra":true}`)...))
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{}`)}))
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_ACK_TRAILING_DATA") {
		t.Fatalf("expected trailing data rejection, got %v", err)
	}
}

func TestHTTPSDeliverAuthenticationFailure(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Status:  "ERROR",
			Code:    "ERR_AGENT_NOT_ASSIGNED",
			Message: "Agent is not assigned to this sensor",
		})
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatalf("failed to create transport: %v", err)
	}

	_, err = tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{"x":1}`)}))
	if err == nil {
		t.Fatalf("expected error on 403 Forbidden, got nil")
	}
	if Classify(err) != FailureAuthentication {
		t.Errorf("expected FailureAuthentication, got %q (err: %v)", Classify(err), err)
	}
}

func TestHTTPSDeliverServerError(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Status:  "ERROR",
			Code:    "ERR_ACCEPTANCE_SINK_FAILED",
			Message: "Spool queue is full",
		})
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatalf("failed to create transport: %v", err)
	}

	_, err = tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{"x":1}`)}))
	if err == nil {
		t.Fatalf("expected error on 503 Service Unavailable, got nil")
	}
	if Classify(err) != FailureTemporary {
		t.Errorf("expected FailureTemporary on 503, got %q", Classify(err))
	}
}

func TestHTTPSDeliverMalformedAck(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"CORRUPT", "invalidJson`))
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatalf("failed to create transport: %v", err)
	}

	_, err = tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{"x":1}`)}))
	if err == nil {
		t.Fatalf("expected error on malformed ACK, got nil")
	}
	if Classify(err) != FailurePermanent {
		t.Errorf("expected FailurePermanent on malformed ACK, got %q", Classify(err))
	}
}

func TestHTTPSDeliverContextTimeout(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatalf("failed to create transport: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = tr.Deliver(ctx, testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{"x":1}`)}))
	if err == nil {
		t.Fatalf("expected error on context timeout, got nil")
	}
	if Classify(err) != FailureTemporary {
		t.Errorf("expected FailureTemporary on timeout, got %q", Classify(err))
	}
}

func TestHTTPSDeliverRejectsHTTPRedirect(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")
	followed := false
	cleartext := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed = true
		w.WriteHeader(http.StatusOK)
	}))
	defer cleartext.Close()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, cleartext.URL+"/api/v1/agent/telemetry", http.StatusTemporaryRedirect)
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{}`)}))
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_REDIRECT_FORBIDDEN") {
		t.Fatalf("expected redirect forbidden, got %v", err)
	}
	if Classify(err) != FailurePermanent {
		t.Fatalf("expected FailurePermanent on redirect, got %q", Classify(err))
	}
	if followed {
		t.Fatal("agent must not follow a 307 redirect to cleartext HTTP")
	}
}

func TestHTTPSDeliverRejectsForeignHostRedirect(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example.invalid/api/v1/agent/telemetry", http.StatusPermanentRedirect)
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{}`)}))
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_REDIRECT_FORBIDDEN") {
		t.Fatalf("expected foreign-host redirect forbidden, got %v", err)
	}
}

func TestHTTPSDeliverIgnoresEnvironmentProxy(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")
	proxyHits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("https_proxy", proxy.URL)
	t.Setenv("ALL_PROXY", proxy.URL)
	t.Setenv("all_proxy", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	server := httptest.NewUnstartedServer(ackHandler(t, "agent-01"))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{}`)})); err != nil {
		t.Fatalf("direct Sensor delivery must ignore HTTPS_PROXY: %v", err)
	}
	if proxyHits != 0 {
		t.Fatalf("environment proxy was used (%d hits); Agent must disable ProxyFromEnvironment", proxyHits)
	}
}

func TestHTTPSDeliverRejectsExpiredClientCert(t *testing.T) {
	now := time.Now()
	_, _, _, clientTLS, serverTLS := generateTestCertificatesWindow(t, "agent-01", now.Add(-48*time.Hour), now.Add(-24*time.Hour))

	server := httptest.NewUnstartedServer(ackHandler(t, "agent-01"))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{}`)}))
	if err == nil {
		t.Fatal("expected TLS failure for expired client certificate")
	}
	if Classify(err) != FailureTemporary {
		t.Fatalf("handshake failure should classify temporary, got %q (%v)", Classify(err), err)
	}
}

func TestHTTPSDeliverRejectsNotYetValidClientCert(t *testing.T) {
	now := time.Now()
	_, _, _, clientTLS, serverTLS := generateTestCertificatesWindow(t, "agent-01", now.Add(24*time.Hour), now.Add(48*time.Hour))

	server := httptest.NewUnstartedServer(ackHandler(t, "agent-01"))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{}`)}))
	if err == nil {
		t.Fatal("expected TLS failure for not-yet-valid client certificate")
	}
}

func TestHTTPSDeliverRejectsWrongCA(t *testing.T) {
	_, _, _, clientTLS, _ := generateTestCertificates(t, "agent-01")
	_, _, _, _, otherServerTLS := generateTestCertificates(t, "agent-other")

	server := httptest.NewUnstartedServer(ackHandler(t, "agent-01"))
	server.TLS = otherServerTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{}`)}))
	if err == nil {
		t.Fatal("expected TLS failure when Sensor cert is signed by an untrusted CA")
	}
}

func TestHTTPSDeliverRejectsOversizedAck(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte("a"), 2048))
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL:        server.URL,
		AgentID:          "agent-01",
		TenantID:         "tenant-01",
		MaxResponseBytes: 64,
	}, clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{}`)}))
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_RESPONSE_TRUNCATED") {
		t.Fatalf("expected truncated ACK rejection, got %v", err)
	}
}

func TestHTTPSDeliverRejectsUnknownAckFields(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ACCEPTED","accepted_through_sequence":1,"agent_id":"agent-01","batch_id":"x","correlation_id":"c","evil":true}`))
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransportForTest(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.Deliver(context.Background(), testBatch("agent-01", queue.Record{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{}`)}))
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_ACK_MALFORMED") {
		t.Fatalf("expected unknown ACK field rejection, got %v", err)
	}
}
