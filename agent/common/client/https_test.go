package client

import (
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
	"strings"
	"testing"
	"time"

	"platform/agent/common/queue"
)

func generateTestCertificates(t *testing.T, commonName string) (caPEM, certPEM, keyPEM []byte, tlsConfig *tls.Config, serverTLS *tls.Config) {
	t.Helper()

	// 1. Generate CA
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

	// 2. Generate Server Certificate
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

	// 3. Generate Client Certificate
	clientPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate client key: %v", err)
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
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
	}

	serverTLSConfig := &tls.Config{
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{serverCert},
	}

	return caPEM, certPEM, keyPEM, clientTLSConfig, serverTLSConfig
}

func TestHTTPSConfigValidation(t *testing.T) {
	// Insecure URL scheme
	_, err := NewHTTPSTransport(HTTPSConfig{
		SensorURL: "http://127.0.0.1:8080",
		AgentID:   "agent-1",
		TenantID:  "tenant-1",
	})
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_SCHEME_INSECURE") {
		t.Errorf("expected DELIVERY_SCHEME_INSECURE for http URL, got %v", err)
	}

	// URL credentials rejected
	_, err = NewHTTPSTransport(HTTPSConfig{
		SensorURL: "https://user:pass@127.0.0.1:8443",
		AgentID:   "agent-1",
		TenantID:  "tenant-1",
	})
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_URL_INSECURE") {
		t.Errorf("expected DELIVERY_URL_INSECURE for userinfo in URL, got %v", err)
	}

	// Empty Agent ID
	_, err = NewHTTPSTransport(HTTPSConfig{
		SensorURL: "https://127.0.0.1:8443",
		AgentID:   "",
		TenantID:  "tenant-1",
	})
	if err == nil || !strings.Contains(err.Error(), "DELIVERY_CONFIG_INVALID") {
		t.Errorf("expected DELIVERY_CONFIG_INVALID for empty AgentID, got %v", err)
	}
}

func TestHTTPSDeliverSuccess(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/telemetry" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Correlation-ID") == "" {
			t.Errorf("expected X-Correlation-ID header")
		}

		var env DeliveryEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Errorf("failed to decode envelope: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if env.AgentID != "agent-01" || env.TenantID != "tenant-01" || len(env.Records) != 2 {
			t.Errorf("unexpected envelope content: %+v", env)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeliveryAck{
			Status:                  "ACCEPTED",
			AcceptedThroughSequence: 10,
			AgentID:                 "agent-01",
			CorrelationID:           env.CorrelationID,
		})
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransport(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
		TLSConfig: clientTLS,
	})
	if err != nil {
		t.Fatalf("failed to create transport: %v", err)
	}

	batch := Batch{
		CorrelationID: "test-corr-1",
		Records: []queue.Record{
			{Sequence: 9, ID: "r-9", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{"x":1}`)},
			{Sequence: 10, ID: "r-10", CreatedAt: time.Now().UTC(), Checksum: "def", Payload: []byte(`{"x":2}`)},
		},
	}

	ack, err := tr.Deliver(context.Background(), batch)
	if err != nil {
		t.Fatalf("deliver failed: %v", err)
	}

	if ack.AcceptedThrough != 10 {
		t.Errorf("expected accepted through 10, got %d", ack.AcceptedThrough)
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

	tr, err := NewHTTPSTransport(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
		TLSConfig: clientTLS,
	})
	if err != nil {
		t.Fatalf("failed to create transport: %v", err)
	}

	batch := Batch{
		CorrelationID: "test-corr-1",
		Records: []queue.Record{
			{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{"x":1}`)},
		},
	}

	_, err = tr.Deliver(context.Background(), batch)
	if err == nil {
		t.Fatalf("expected error on 403 Forbidden, got nil")
	}

	failClass := Classify(err)
	if failClass != FailureAuthentication {
		t.Errorf("expected FailureAuthentication, got %q (err: %v)", failClass, err)
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

	tr, err := NewHTTPSTransport(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
		TLSConfig: clientTLS,
	})
	if err != nil {
		t.Fatalf("failed to create transport: %v", err)
	}

	batch := Batch{
		CorrelationID: "test-corr-1",
		Records: []queue.Record{
			{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{"x":1}`)},
		},
	}

	_, err = tr.Deliver(context.Background(), batch)
	if err == nil {
		t.Fatalf("expected error on 503 Service Unavailable, got nil")
	}

	failClass := Classify(err)
	if failClass != FailureTemporary {
		t.Errorf("expected FailureTemporary on 503, got %q", failClass)
	}
}

func TestHTTPSDeliverMalformedAck(t *testing.T) {
	_, _, _, clientTLS, serverTLS := generateTestCertificates(t, "agent-01")

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"CORRUPT", "invalidJson`)) // Malformed JSON
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	tr, err := NewHTTPSTransport(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
		TLSConfig: clientTLS,
	})
	if err != nil {
		t.Fatalf("failed to create transport: %v", err)
	}

	batch := Batch{
		CorrelationID: "test-corr-1",
		Records: []queue.Record{
			{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{"x":1}`)},
		},
	}

	_, err = tr.Deliver(context.Background(), batch)
	if err == nil {
		t.Fatalf("expected error on malformed ACK, got nil")
	}

	failClass := Classify(err)
	if failClass != FailurePermanent {
		t.Errorf("expected FailurePermanent on malformed ACK, got %q", failClass)
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

	tr, err := NewHTTPSTransport(HTTPSConfig{
		SensorURL: server.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
		TLSConfig: clientTLS,
	})
	if err != nil {
		t.Fatalf("failed to create transport: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	batch := Batch{
		CorrelationID: "test-corr-1",
		Records: []queue.Record{
			{Sequence: 1, ID: "r-1", CreatedAt: time.Now().UTC(), Checksum: "abc", Payload: []byte(`{"x":1}`)},
		},
	}

	_, err = tr.Deliver(ctx, batch)
	if err == nil {
		t.Fatalf("expected error on context timeout, got nil")
	}

	failClass := Classify(err)
	if failClass != FailureTemporary {
		t.Errorf("expected FailureTemporary on timeout, got %q", failClass)
	}
}
