package agentreceiver_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/delivery"
	"platform/agent/common/queue"
	"platform/sensor/internal/agentreceiver"
	"platform/sensor/internal/runtime"
)

// Integration test (not production E2E): DurableQueue → Worker → NewHTTPSTransportForTest
// → real TLS → httptest mounting agentreceiver.Receiver → StaticTestIdentityVerifier →
// InMemoryAssignmentAuthorizer → DurableTestAcceptanceSink → batch-bound ACK → queue deletion.
func TestRealReceiverPipelineIntegration(t *testing.T) {
	clientTLS, serverTLS, clientCN := buildIntegrationPKI(t)

	queueDir := t.TempDir()
	sinkDir := t.TempDir()

	ident := agentreceiver.NewStaticTestIdentityVerifier()
	ident.AllowCommonName(clientCN, "agent-01")

	auth := agentreceiver.NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-01", "sensor-infra-01", "tenant-01", "site-primary")

	sink, err := agentreceiver.NewDurableTestAcceptanceSink(sinkDir)
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	rcv, err := agentreceiver.NewReceiver(
		agentreceiver.Config{
			SensorID:   "sensor-infra-01",
			SensorRole: runtime.RoleInfrastructure,
		},
		ident,
		auth,
		sink,
		logger,
	)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle(agentreceiver.EndpointPath, rcv)

	dropOnce := &dropOnceHandler{inner: mux}
	ts := httptest.NewUnstartedServer(dropOnce)
	ts.TLS = serverTLS
	ts.StartTLS()
	defer ts.Close()

	q, err := queue.NewDurableQueue(queueDir, 100, 10*1024*1024, 64*1024)
	if err != nil {
		t.Fatal(err)
	}

	tr, err := client.NewHTTPSTransportForTest(client.HTTPSConfig{
		SensorURL: ts.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
		SiteID:    "site-primary",
	}, clientTLS)
	if err != nil {
		t.Fatal(err)
	}

	worker, err := delivery.NewWorker(q, tr, 10, 1024*1024, 10*time.Millisecond, 50*time.Millisecond, 3*time.Second, logger)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Happy path: durable accept → batch-bound ACK → queue deletion
	if _, err := q.Enqueue([]byte(`{"metric":"cpu","val":1}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte(`{"metric":"mem","val":2}`)); err != nil {
		t.Fatal(err)
	}
	delivered, err := worker.DeliverOnce(ctx)
	if err != nil || !delivered {
		t.Fatalf("expected success, delivered=%v err=%v", delivered, err)
	}
	if q.Len() != 0 {
		t.Fatalf("expected queue drained, got %d", q.Len())
	}
	if sink.BatchCount() != 1 {
		t.Fatalf("expected 1 durable sink batch, got %d", sink.BatchCount())
	}

	// Network drop after sink accept but before Agent reads ACK → retain → retry → batch-bound ACK
	if _, err := q.Enqueue([]byte(`{"metric":"disk","val":3}`)); err != nil {
		t.Fatal(err)
	}
	dropOnce.arm()
	delivered, err = worker.DeliverOnce(ctx)
	if delivered || err == nil {
		t.Fatalf("expected drop to fail delivery, delivered=%v err=%v", delivered, err)
	}
	if q.Len() != 1 {
		t.Fatalf("unsafe deletion must not occur after lost ACK, got len=%d", q.Len())
	}
	acceptedBeforeRetry := sink.BatchCount()

	delivered, err = worker.DeliverOnce(ctx)
	if err != nil || !delivered {
		t.Fatalf("retry must succeed with batch-bound ACK, delivered=%v err=%v", delivered, err)
	}
	if q.Len() != 0 {
		t.Fatalf("queue must drain after final ACK, got %d", q.Len())
	}
	// Exact replay should not re-accept into sink
	if sink.BatchCount() != acceptedBeforeRetry {
		t.Fatalf("replay must not duplicate sink accept; before=%d after=%d", acceptedBeforeRetry, sink.BatchCount())
	}
}

type dropOnceHandler struct {
	inner   http.Handler
	armed   bool
	dropped bool
}

func (h *dropOnceHandler) arm() {
	h.armed = true
	h.dropped = false
}

func (h *dropOnceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.armed && !h.dropped {
		h.dropped = true
		h.armed = false
		// Let the real receiver process (and accept) then abort the response.
		rw := &captureResponseWriter{header: make(http.Header)}
		h.inner.ServeHTTP(rw, r)
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack unsupported", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		_ = conn.Close()
		return
	}
	h.inner.ServeHTTP(w, r)
}

type captureResponseWriter struct {
	header http.Header
	code   int
	body   []byte
}

func (c *captureResponseWriter) Header() http.Header         { return c.header }
func (c *captureResponseWriter) WriteHeader(statusCode int)  { c.code = statusCode }
func (c *captureResponseWriter) Write(b []byte) (int, error) { c.body = append(c.body, b...); return len(b), nil }

func buildIntegrationPKI(t *testing.T) (clientTLS, serverTLS *tls.Config, clientCN string) {
	t.Helper()
	clientCN = "agent-01"

	caPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Integration-Test-CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatal(err)
	}

	serverPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverBytes, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatal(err)
	}

	clientPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: clientCN},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientBytes, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatal(err)
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caBytes}))

	clientTLS = &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{clientBytes},
			PrivateKey:  clientPriv,
		}},
	}
	serverTLS = &tls.Config{
		ClientCAs:  caPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{serverBytes},
			PrivateKey:  serverPriv,
		}},
	}

	return clientTLS, serverTLS, clientCN
}
