package identity

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHTTPEnrollmentTransportUsesCallerSuppliedPathSendsCSRAndOmitsPrivateKey(t *testing.T) {
	now := time.Now().UTC()
	material := makeEnrollmentMaterial(t, "sensor-http", now.Add(-time.Minute), now.Add(time.Hour))
	var received map[string]json.RawMessage
	var rawBody []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/contract-supplied/enroll" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		rawBody, _ = io.ReadAll(r.Body)
		if err := json.Unmarshal(rawBody, &received); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(enrollmentWireResponse{
			SensorID:       material.Response.SensorID,
			CertificatePEM: string(material.Response.CertificatePEM),
			CABundlePEM:    string(material.Response.CABundlePEM),
		})
	}))
	defer server.Close()

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	caFile := filepath.Join(t.TempDir(), "server-ca.crt")
	if err := os.WriteFile(caFile, caPEM, 0600); err != nil {
		t.Fatal(err)
	}
	transport, err := NewHTTPEnrollmentTransport(server.URL+"/contract-supplied/enroll", caFile, time.Second, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.Enroll(context.Background(), []byte("test-only-token"), EnrollmentRequest{
		Role:   "NETWORK",
		SiteID: "site-a",
		CSRPem: string(material.CSRPem),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.SensorID != "sensor-http" {
		t.Fatalf("response=%+v", response)
	}
	if string(received["token"]) != `"test-only-token"` || string(received["siteId"]) != `"site-a"` {
		t.Fatalf("request=%v", received)
	}
	if _, ok := received["csrPem"]; !ok {
		t.Fatalf("csrPem missing from request: %s", rawBody)
	}
	if _, ok := received["privateKeyPem"]; ok {
		t.Fatalf("privateKeyPem present in request: %s", rawBody)
	}
	lower := strings.ToLower(string(rawBody))
	if strings.Contains(lower, "privatekey") || strings.Contains(string(rawBody), "BEGIN PRIVATE KEY") {
		t.Fatalf("private key material in request body: %s", rawBody)
	}
}

func TestHTTPEnrollmentTransportRejectsLegacyPrivateKeyResponse(t *testing.T) {
	now := time.Now().UTC()
	material := makeEnrollmentMaterial(t, "sensor-http", now.Add(-time.Minute), now.Add(time.Hour))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"sensorId":       material.Response.SensorID,
			"certificatePem": string(material.Response.CertificatePEM),
			"privateKeyPem":  string(material.KeyPEM),
			"caBundlePem":    string(material.Response.CABundlePEM),
		})
	}))
	defer server.Close()
	caFile := filepath.Join(t.TempDir(), "server-ca.crt")
	_ = os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	transport, err := NewHTTPEnrollmentTransport(server.URL+"/enroll", caFile, time.Second, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	_, err = transport.Enroll(context.Background(), []byte("token"), EnrollmentRequest{
		Role:   "NETWORK",
		CSRPem: string(material.CSRPem),
	})
	if err == nil || !strings.Contains(err.Error(), "RESPONSE_INVALID") {
		t.Fatalf("legacy privateKeyPem response accepted: %v", err)
	}
}

func TestHTTPEnrollmentTransportRejectsHTTPAndUntrustedTLS(t *testing.T) {
	if _, err := NewHTTPEnrollmentTransport("http://core.test/enroll", "/missing", time.Second, 1024); err == nil {
		t.Fatal("HTTP enrollment endpoint accepted")
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	otherCert, _ := x509.ParseCertificate(server.Certificate().Raw)
	_ = otherCert
	badCA := filepath.Join(t.TempDir(), "bad-ca.crt")
	_ = os.WriteFile(badCA, []byte("not a certificate"), 0600)
	if _, err := NewHTTPEnrollmentTransport(server.URL+"/enroll", badCA, time.Second, 1024); err == nil {
		t.Fatal("invalid CA accepted")
	}
}

func TestHTTPEnrollmentTransportErrorNeverContainsToken(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	defer server.Close()
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	_ = os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	transport, err := NewHTTPEnrollmentTransport(server.URL+"/enroll", caFile, time.Second, 1024)
	if err != nil {
		t.Fatal(err)
	}
	_, err = transport.Enroll(context.Background(), []byte("never-log-this-token"), EnrollmentRequest{
		Role:   "NETWORK",
		CSRPem: "-----BEGIN CERTIFICATE REQUEST-----\nMIIB\n-----END CERTIFICATE REQUEST-----\n",
	})
	if err == nil || strings.Contains(err.Error(), "never-log-this-token") {
		t.Fatalf("error=%v", err)
	}
}

func TestHTTPEnrollmentTransportResponseBound(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 128)))
	}))
	defer server.Close()
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	_ = os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	transport, err := NewHTTPEnrollmentTransport(server.URL+"/enroll", caFile, time.Second, 32)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Enroll(context.Background(), []byte("token"), EnrollmentRequest{
		Role:   "NETWORK",
		CSRPem: "-----BEGIN CERTIFICATE REQUEST-----\nMIIB\n-----END CERTIFICATE REQUEST-----\n",
	}); err == nil || !strings.Contains(err.Error(), "TOO_LARGE") {
		t.Fatalf("error=%v", err)
	}
}

func TestHTTPEnrollmentTransportRequiresCSRPem(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	_ = os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	transport, err := NewHTTPEnrollmentTransport(server.URL+"/enroll", caFile, time.Second, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Enroll(context.Background(), []byte("token"), EnrollmentRequest{Role: "NETWORK"}); err == nil {
		t.Fatal("missing csrPem accepted")
	}
}

func TestHTTPEnrollmentTransportRejectsTrailingJSON(t *testing.T) {
	now := time.Now().UTC()
	material := makeEnrollmentMaterial(t, "sensor-http", now.Add(-time.Minute), now.Add(time.Hour))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first, _ := json.Marshal(enrollmentWireResponse{
			SensorID:       material.Response.SensorID,
			CertificatePEM: string(material.Response.CertificatePEM),
			CABundlePEM:    string(material.Response.CABundlePEM),
		})
		_, _ = w.Write(first)
		_, _ = w.Write([]byte(`{"extra":true}`))
	}))
	defer server.Close()
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	_ = os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	transport, err := NewHTTPEnrollmentTransport(server.URL+"/enroll", caFile, time.Second, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	_, err = transport.Enroll(context.Background(), []byte("token"), EnrollmentRequest{
		Role:   "NETWORK",
		CSRPem: string(material.CSRPem),
	})
	if err == nil || !strings.Contains(err.Error(), "RESPONSE_INVALID") {
		t.Fatalf("trailing JSON accepted: %v", err)
	}
}
