package delivery

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/queue"
)

// In-package wire definitions matching the Sensor receiver contract
type harnessDeliveryRecord struct {
	Sequence  uint64          `json:"sequence"`
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"createdAt"`
	Checksum  string          `json:"checksum"`
	Payload   json.RawMessage `json:"payload"`
}

type harnessDeliveryEnvelope struct {
	SchemaVersion int                     `json:"schemaVersion"`
	AgentID       string                  `json:"agentId"`
	TenantID      string                  `json:"tenantId"`
	SiteID        string                  `json:"siteId,omitempty"`
	CorrelationID string                  `json:"correlationId"`
	BatchID       string                  `json:"batchId"`
	AgentStreamID string                  `json:"agentStreamId,omitempty"`
	FirstSequence uint64                  `json:"firstSequence"`
	LastSequence  uint64                  `json:"lastSequence"`
	RecordCount   int                     `json:"recordCount"`
	Records       []harnessDeliveryRecord `json:"records"`
}

type harnessDeliveryAck struct {
	Status                  string `json:"status"`
	AcceptedThroughSequence uint64 `json:"accepted_through_sequence"`
	AgentID                 string `json:"agent_id"`
	BatchID                 string `json:"batch_id"`
	CorrelationID           string `json:"correlation_id"`
}

type harnessErrorResponse struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type harnessTestServer struct {
	mu           sync.Mutex
	assigned     map[string]bool
	accepted     []harnessDeliveryEnvelope
	replays      map[string]harnessDeliveryAck
	sinkFailure  bool
	dropResponse bool
	corruptAck   bool
}

func newHarnessTestServer() *harnessTestServer {
	return &harnessTestServer{
		assigned: make(map[string]bool),
		replays:  make(map[string]harnessDeliveryAck),
	}
}

func (s *harnessTestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(harnessErrorResponse{
			Status:  "ERROR",
			Code:    "ERR_AUTH_CERTIFICATE_REQUIRED",
			Message: "mTLS client certificate required",
		})
		return
	}

	certCN := r.TLS.PeerCertificates[0].Subject.CommonName

	var env harnessDeliveryEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(harnessErrorResponse{
			Status:  "ERROR",
			Code:    "ERR_MALFORMED_JSON",
			Message: "Malformed JSON payload",
		})
		return
	}

	if !strings.EqualFold(certCN, env.AgentID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(harnessErrorResponse{
			Status:  "ERROR",
			Code:    "ERR_AUTH_IDENTITY_MISMATCH",
			Message: "Client certificate CN does not match claimed agentId",
		})
		return
	}

	assignKey := fmt.Sprintf("%s:%s", env.TenantID, env.AgentID)
	if !s.assigned[assignKey] {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(harnessErrorResponse{
			Status:  "ERROR",
			Code:    "ERR_AGENT_NOT_ASSIGNED",
			Message: "Agent is not assigned to this sensor",
		})
		return
	}

	batchKey := fmt.Sprintf("%s:%s", env.AgentID, env.BatchID)
	if cachedAck, exists := s.replays[batchKey]; exists {
		// Match production Sensor: exact replay re-enters durable path and ACKs
		// with THIS attempt's correlation_id (not the prior attempt's value).
		cachedAck.CorrelationID = env.CorrelationID
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(cachedAck)
		return
	}

	if s.sinkFailure {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(harnessErrorResponse{
			Status:  "ERROR",
			Code:    "ERR_ACCEPTANCE_SINK_FAILED",
			Message: "Downstream sensor sink failed",
		})
		return
	}

	if s.dropResponse {
		s.accepted = append(s.accepted, env)
		s.replays[batchKey] = harnessDeliveryAck{
			Status:                  "ACCEPTED",
			AcceptedThroughSequence: env.LastSequence,
			AgentID:                 env.AgentID,
			BatchID:                 env.BatchID,
			CorrelationID:           env.CorrelationID,
		}
		s.dropResponse = false
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		}
		return
	}

	if s.corruptAck {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(harnessDeliveryAck{
			Status:                  "ACCEPTED",
			AcceptedThroughSequence: env.LastSequence + 100,
			AgentID:                 env.AgentID,
			BatchID:                 env.BatchID,
			CorrelationID:           env.CorrelationID,
		})
		return
	}

	s.accepted = append(s.accepted, env)
	ack := harnessDeliveryAck{
		Status:                  "ACCEPTED",
		AcceptedThroughSequence: env.LastSequence,
		AgentID:                 env.AgentID,
		BatchID:                 env.BatchID,
		CorrelationID:           env.CorrelationID,
	}
	s.replays[batchKey] = ack

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ack)
}

func setupHarnessPKI(t *testing.T, clientCN string) (*tls.Config, *tls.Config) {
	t.Helper()
	caPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Harness-Root-CA"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caBytes, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPriv.PublicKey, caPriv)

	serverPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	serverBytes, _ := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverPriv.PublicKey, caPriv)

	clientPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: clientCN},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientBytes, _ := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientPriv.PublicKey, caPriv)

	caPool := x509.NewCertPool()
	caCert, _ := x509.ParseCertificate(caBytes)
	caPool.AddCert(caCert)

	clientTLS := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{clientBytes},
			PrivateKey:  clientPriv,
		}},
	}
	serverTLS := &tls.Config{
		ClientCAs:  caPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{serverBytes},
			PrivateKey:  serverPriv,
		}},
	}

	return clientTLS, serverTLS
}

func TestHarnessPipeline(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "harness-test-*")
	if err != nil {
		t.Fatalf("temp dir creation failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	clientTLS, serverTLS := setupHarnessPKI(t, "agent-01")
	hServer := newHarnessTestServer()
	hServer.assigned["tenant-01:agent-01"] = true

	ts := httptest.NewUnstartedServer(hServer)
	ts.TLS = serverTLS
	ts.StartTLS()
	defer ts.Close()

	q, err := queue.NewDurableQueue(tempDir, 100, 10*1024*1024, 64*1024)
	if err != nil {
		t.Fatalf("failed to create durable queue: %v", err)
	}

	tr, err := client.NewHTTPSTransportForTest(client.HTTPSConfig{
		SensorURL: ts.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, clientTLS)
	if err != nil {
		t.Fatalf("failed to create transport: %v", err)
	}

	worker, err := NewWorker(q, tr, "agent-01", 10, 1024*1024, 10*time.Millisecond, 50*time.Millisecond, 2*time.Second, nil)
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	ctx := context.Background()

	_, _ = q.Enqueue([]byte(`{"metric":"cpu","val":25.0}`))
	_, _ = q.Enqueue([]byte(`{"metric":"mem","val":50.0}`))
	if q.Len() != 2 {
		t.Fatalf("expected queue len 2, got %d", q.Len())
	}

	delivered, err := worker.DeliverOnce(ctx)
	if err != nil || !delivered {
		t.Fatalf("expected delivery success, delivered=%v err=%v", delivered, err)
	}
	if q.Len() != 0 {
		t.Errorf("expected queue drained to 0 after valid ACK, got %d", q.Len())
	}

	hServer.sinkFailure = true
	_, _ = q.Enqueue([]byte(`{"metric":"disk","val":80.0}`))
	delivered, err = worker.DeliverOnce(ctx)
	if delivered || err == nil {
		t.Fatalf("expected delivery failure on sink error, delivered=%v err=%v", delivered, err)
	}
	if q.Len() != 1 {
		t.Errorf("queue records must remain when delivery fails, got len %d", q.Len())
	}
	hServer.sinkFailure = false

	hServer.dropResponse = true
	delivered, err = worker.DeliverOnce(ctx)
	if delivered || err == nil {
		t.Fatalf("expected error on dropped connection, got delivered=%v err=%v", delivered, err)
	}
	acceptedCountBefore := len(hServer.accepted)

	delivered, err = worker.DeliverOnce(ctx)
	if err != nil || !delivered {
		t.Fatalf("expected retry delivery to succeed via replay cache, delivered=%v err=%v", delivered, err)
	}
	if q.Len() != 0 {
		t.Errorf("expected queue drained after replay ACK, got %d", q.Len())
	}
	if len(hServer.accepted) != acceptedCountBefore {
		t.Errorf("replay must not duplicate records in server sink; before=%d, after=%d", acceptedCountBefore, len(hServer.accepted))
	}

	hServer.corruptAck = true
	_, _ = q.Enqueue([]byte(`{"metric":"load","val":1.5}`))
	delivered, err = worker.DeliverOnce(ctx)
	if delivered || err == nil {
		t.Fatalf("expected delivery to fail on corrupted ACK, got delivered=%v err=%v", delivered, err)
	}
	if q.Len() != 1 {
		t.Errorf("queue must retain records when ACK validation fails, got len %d", q.Len())
	}
	hServer.corruptAck = false

	delivered, err = worker.DeliverOnce(ctx)
	if err != nil || !delivered {
		t.Fatalf("failed to drain valid batch: %v", err)
	}

	hServer.sinkFailure = true
	_, _ = q.Enqueue([]byte(`{"metric":"net_in","val":100}`))
	_, _ = q.Enqueue([]byte(`{"metric":"net_out","val":200}`))
	delivered, err = worker.DeliverOnce(ctx)
	if delivered || err == nil {
		t.Fatalf("expected failure during receiver outage")
	}
	hServer.sinkFailure = false
	delivered, err = worker.DeliverOnce(ctx)
	if err != nil || !delivered {
		t.Fatalf("expected recovery to succeed, got %v", err)
	}

	wrongClientTLS, _ := setupHarnessPKI(t, "agent-imposter")
	wrongTr, _ := client.NewHTTPSTransportForTest(client.HTTPSConfig{
		SensorURL: ts.URL,
		AgentID:   "agent-01",
		TenantID:  "tenant-01",
	}, wrongClientTLS)
	wrongWorker, _ := NewWorker(q, wrongTr, "agent-01", 10, 1024*1024, 10*time.Millisecond, 50*time.Millisecond, 2*time.Second, nil)

	_, _ = q.Enqueue([]byte(`{"metric":"rogue_check","val":1}`))
	delivered, err = wrongWorker.DeliverOnce(ctx)
	if delivered || err == nil {
		t.Fatalf("expected failure on cert CN mismatch")
	}
	if q.Len() != 1 {
		t.Errorf("queue must retain records when cert mismatch occurs, got %d", q.Len())
	}

	delete(hServer.assigned, "tenant-01:agent-01")
	delivered, err = worker.DeliverOnce(ctx)
	if delivered || err == nil {
		t.Fatalf("expected failure when agent is unassigned")
	}
	hServer.assigned["tenant-01:agent-01"] = true
	delivered, err = worker.DeliverOnce(ctx)
	if err != nil || !delivered {
		t.Fatalf("expected delivery after assignment restored: %v", err)
	}
	if q.Len() != 0 {
		t.Errorf("expected queue empty, got %d", q.Len())
	}
}
