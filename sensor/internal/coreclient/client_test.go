package coreclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testPKI struct {
	caPEM, serverCertPEM, serverKeyPEM, clientCertPEM, clientKeyPEM []byte
	caPool *x509.CertPool
}

func makeTestPKI(t *testing.T) testPKI {
	t.Helper()
	now := time.Now()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil { t.Fatal(err) }
	caCert, _ := x509.ParseCertificate(caDER)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	makeLeaf := func(serial int64, cn string, server bool) ([]byte, []byte) {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: cn}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
		if server {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}; template.DNSNames = []string{"localhost"}; template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		} else { template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth} }
		der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey); if err != nil { t.Fatal(err) }
		keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	}
	serverCert, serverKey := makeLeaf(2, "localhost", true); clientCert, clientKey := makeLeaf(3, "sensor-test", false)
	pool := x509.NewCertPool(); pool.AppendCertsFromPEM(caPEM)
	return testPKI{caPEM: caPEM, serverCertPEM: serverCert, serverKeyPEM: serverKey, clientCertPEM: clientCert, clientKeyPEM: clientKey, caPool: pool}
}

func writePKI(t *testing.T, pki testPKI) (ca, cert, key string) {
	t.Helper(); dir := t.TempDir(); ca = filepath.Join(dir, "ca.crt"); cert = filepath.Join(dir, "client.crt"); key = filepath.Join(dir, "client.key")
	if err := os.WriteFile(ca, pki.caPEM, 0600); err != nil { t.Fatal(err) }; if err := os.WriteFile(cert, pki.clientCertPEM, 0600); err != nil { t.Fatal(err) }; if err := os.WriteFile(key, pki.clientKeyPEM, 0600); err != nil { t.Fatal(err) }; return
}

func startMTLSServer(t *testing.T, pki testPKI, handler http.Handler) *httptest.Server {
	t.Helper(); server := httptest.NewUnstartedServer(handler); cert, err := tls.X509KeyPair(pki.serverCertPEM, pki.serverKeyPEM); if err != nil { t.Fatal(err) }
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pki.caPool, MinVersion: tls.VersionTLS12}
	server.StartTLS(); t.Cleanup(server.Close); return server
}

func newTestClient(t *testing.T, serverURL string, pki testPKI, max int) *Client {
	t.Helper(); ca, cert, key := writePKI(t, pki); client, err := New(Config{BaseURL: serverURL, CertificateFile: cert, PrivateKeyFile: key, CAFile: ca, RequestTimeout: time.Second, MaxResponseBytes: max}); if err != nil { t.Fatal(err) }; return client
}

func TestNewFailsClosedForHTTPMissingCAAndBadCA(t *testing.T) {
	pki := makeTestPKI(t); ca, cert, key := writePKI(t, pki)
	cases := []Config{{BaseURL: "http://core.test", CertificateFile: cert, PrivateKeyFile: key, CAFile: ca, RequestTimeout: time.Second, MaxResponseBytes: 1024}, {BaseURL: "https://core.test", CertificateFile: cert, PrivateKeyFile: key, RequestTimeout: time.Second, MaxResponseBytes: 1024}}
	for _, cfg := range cases { if _, err := New(cfg); err == nil { t.Fatalf("config accepted: %+v", cfg) } }
	bad := filepath.Join(t.TempDir(), "bad-ca"); _ = os.WriteFile(bad, []byte("not a certificate"), 0600)
	if _, err := New(Config{BaseURL: "https://core.test", CertificateFile: cert, PrivateKeyFile: key, CAFile: bad, RequestTimeout: time.Second, MaxResponseBytes: 1024}); !IsCode(err, CodeTLSConfiguration) { t.Fatalf("error=%v", err) }
}

func TestStrictMTLSHeartbeatAndStatus(t *testing.T) {
	pki := makeTestPKI(t); mux := http.NewServeMux()
	mux.HandleFunc(statusPath, func(w http.ResponseWriter, r *http.Request) { if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 { t.Error("missing client certificate") }; _ = json.NewEncoder(w).Encode(StatusResponse{Status: "UP", Authenticated: true, SensorIdentity: "sensor-test"}) })
	mux.HandleFunc(heartbeatPath, func(w http.ResponseWriter, r *http.Request) { if r.Header.Get("X-Correlation-ID") != "corr-1" { t.Errorf("correlation=%q", r.Header.Get("X-Correlation-ID")) }; _ = json.NewEncoder(w).Encode(map[string]any{"status": "ACCEPTED", "authenticated": true, "sensor_identity": "sensor-test", "futureCompatibleField": true}) })
	server := startMTLSServer(t, pki, mux); client := newTestClient(t, server.URL, pki, 4096)
	status, err := client.Status(context.Background(), ""); if err != nil || status.Status != "UP" { t.Fatalf("status=%+v err=%v", status, err) }
	hb, err := client.SendHeartbeat(context.Background(), HeartbeatRequest{SensorID: "sensor-test", Role: "NETWORK", State: "READY", CorrelationID: "corr-1"}); if err != nil || hb.Status != "ACCEPTED" { t.Fatalf("heartbeat=%+v err=%v", hb, err) }
}

func TestUploadRequiresDurableAckAndUsesIdempotencyHeader(t *testing.T) {
	pki := makeTestPKI(t); server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { if r.URL.Path != telemetryPath { http.NotFound(w, r); return }; if r.Header.Get("Idempotency-Key") != "batch-1" { t.Errorf("idempotency=%q", r.Header.Get("Idempotency-Key")) }; _ = json.NewEncoder(w).Encode(map[string]any{"status": "ACCEPTED", "sensor_identity": "sensor-test"}) }))
	client := newTestClient(t, server.URL, pki, 4096); _, err := client.Upload(context.Background(), validBatch()); if !IsCode(err, CodeAckContractMissing) || IsTransient(err) { t.Fatalf("error=%v", err) }
}

func TestUploadAcceptsBoundedDurableAck(t *testing.T) {
	pki := makeTestPKI(t); accepted := uint64(2); server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(UploadAck{Status: "ACCEPTED", AcceptedThroughSequence: &accepted}) })); client := newTestClient(t, server.URL, pki, 4096)
	ack, err := client.Upload(context.Background(), validBatch()); if err != nil || ack.AcceptedThroughSequence == nil || *ack.AcceptedThroughSequence != 2 { t.Fatalf("ack=%+v err=%v", ack, err) }
}

func TestUploadRejectsAckOutsideSubmittedRange(t *testing.T) {
	pki := makeTestPKI(t); accepted := uint64(99); server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(UploadAck{Status: "ACCEPTED", AcceptedThroughSequence: &accepted}) })); client := newTestClient(t, server.URL, pki, 4096)
	if _, err := client.Upload(context.Background(), validBatch()); !IsCode(err, CodeProtocol) { t.Fatalf("error=%v", err) }
}

func TestResponseBodyBound(t *testing.T) {
	pki := makeTestPKI(t); server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, strings.Repeat("x", 128)) })); client := newTestClient(t, server.URL, pki, 32)
	if _, err := client.Status(context.Background(), ""); !IsCode(err, CodeResponseTooLarge) { t.Fatalf("error=%v", err) }
}

func TestHTTPStatusClassification(t *testing.T) {
	pki := makeTestPKI(t)
	for _, tc := range []struct{ status int; code ErrorCode; transient bool }{{401, CodeAuthentication, false}, {403, CodeAuthorization, false}, {429, CodeRateLimited, true}, {503, CodeServerUnavailable, true}} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) { server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status) })); client := newTestClient(t, server.URL, pki, 256); _, err := client.Status(context.Background(), ""); if !IsCode(err, tc.code) || IsTransient(err) != tc.transient { t.Fatalf("error=%v transient=%v", err, IsTransient(err)) } })
	}
}

func TestContextCancellation(t *testing.T) {
	pki := makeTestPKI(t); server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })); client := newTestClient(t, server.URL, pki, 256); ctx, cancel := context.WithCancel(context.Background()); cancel()
	if _, err := client.Status(ctx, ""); !IsCode(err, CodeCancelled) { t.Fatalf("error=%v", err) }
}

func TestLegacyConstructorFailsClosed(t *testing.T) {
	client := NewCoreClient("https://core.test", "", "", "", nil); if ok, err := client.CheckConnectivity(context.Background()); ok || !IsCode(err, CodeInvalidConfig) { t.Fatalf("ok=%v err=%v", ok, err) }
}

func validBatch() Batch { return Batch{SchemaVersion: 1, SensorID: "sensor-test", BootEpoch: "boot-1", BatchID: "batch-1", FirstSequence: 1, LastSequence: 2, Checksum: "abc", Records: []json.RawMessage{json.RawMessage(`{"x":1}`), json.RawMessage(`{"x":2}`)}} }
