//go:build pipeline

package agentreceiver_test

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
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/sensor/internal/agentdrain"
	"platform/sensor/internal/agentreceiver"
	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/spool"
)

// BUILD-001 certification slice:
// production SpoolAcceptanceSink → production DiskSpool → production agentdrain
// → production coreclient.Client HTTPS → isolated mTLS httptest Mock Core.
//
// Not production E2E. No Docker. No shared Compose. No PG/Kafka.
// Does not certify Agent↔Sensor transport completion (BUILD-002).

const (
	sliceSensorID = "sensor-infra-01"
	sliceTenantID = "11111111-1111-4111-8111-111111111111"
	sliceAgentID  = "agent-01"
	sliceBatchID  = "batch-agent-slice-001"
	sliceCorrID   = "corr-agent-slice-001"
)

type slicePKI struct {
	caPEM, serverCertPEM, serverKeyPEM, clientCertPEM, clientKeyPEM []byte
	caPool                                                          *x509.CertPool
}

func makeSlicePKI(t *testing.T) slicePKI {
	t.Helper()
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "build-001-slice-ca"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	makeLeaf := func(serial int64, cn string, server bool) ([]byte, []byte) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    now.Add(-time.Hour),
			NotAfter:     now.Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
		}
		if server {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
			template.DNSNames = []string{"localhost"}
			template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		} else {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		}
		der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
			pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	}

	serverCert, serverKey := makeLeaf(2, "localhost", true)
	clientCert, clientKey := makeLeaf(3, "sensor-infra-01", false)
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to load CA pool")
	}
	return slicePKI{
		caPEM: caPEM, serverCertPEM: serverCert, serverKeyPEM: serverKey,
		clientCertPEM: clientCert, clientKeyPEM: clientKey, caPool: pool,
	}
}

func writeSliceClientPKI(t *testing.T, pki slicePKI) (ca, cert, key string) {
	t.Helper()
	dir := t.TempDir()
	ca = filepath.Join(dir, "ca.crt")
	cert = filepath.Join(dir, "client.crt")
	key = filepath.Join(dir, "client.key")
	mustWrite(t, ca, pki.caPEM)
	mustWrite(t, cert, pki.clientCertPEM)
	mustWrite(t, key, pki.clientKeyPEM)
	return
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func startSliceMTLSServer(t *testing.T, pki slicePKI, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	cert, err := tls.X509KeyPair(pki.serverCertPEM, pki.serverKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pki.caPool,
		MinVersion:   tls.VersionTLS12,
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func newSliceCoreClient(t *testing.T, baseURL string, pki slicePKI) *coreclient.Client {
	t.Helper()
	ca, cert, key := writeSliceClientPKI(t, pki)
	client, err := coreclient.New(coreclient.Config{
		BaseURL:          baseURL,
		CertificateFile:  cert,
		PrivateKeyFile:   key,
		CAFile:           ca,
		RequestTimeout:   2 * time.Second,
		MaxResponseBytes: 64 * 1024,
	})
	if err != nil {
		t.Fatalf("coreclient.New: %v", err)
	}
	return client
}

func openProductionAgentSpool(t *testing.T, dir string) *spool.DiskSpool {
	t.Helper()
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 512 * 1024})
	if err != nil {
		t.Fatalf("spool.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func sampleEnvelope(t *testing.T) agentreceiver.DeliveryEnvelope {
	t.Helper()
	created := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	payload1 := []byte(`{"schemaVersion":1,"sourceType":"AGENT","hostId":"host-1","metric":"cpu"}`)
	payload2 := []byte(`{"schemaVersion":1,"sourceType":"AGENT","hostId":"host-1","metric":"mem"}`)
	rec1 := agentreceiver.DeliveryRecord{
		Sequence: 1, ID: "rec-1", CreatedAt: created, Payload: payload1,
	}
	rec1.Checksum = agentreceiver.ComputeRecordChecksum(rec1.Sequence, rec1.ID, rec1.CreatedAt, rec1.Payload)
	rec2 := agentreceiver.DeliveryRecord{
		Sequence: 2, ID: "rec-2", CreatedAt: created.Add(time.Second), Payload: payload2,
	}
	rec2.Checksum = agentreceiver.ComputeRecordChecksum(rec2.Sequence, rec2.ID, rec2.CreatedAt, rec2.Payload)
	return agentreceiver.DeliveryEnvelope{
		SchemaVersion:          1,
		AgentID:                sliceAgentID,
		TenantID:               sliceTenantID,
		CorrelationID:          sliceCorrID,
		BatchID:                sliceBatchID,
		FirstSequence:          1,
		LastSequence:           2,
		RecordCount:            2,
		Records:                []agentreceiver.DeliveryRecord{rec1, rec2},
		ReceivingSensorID:      sliceSensorID,
		AssignmentGeneration:   1,
	}
}

func newDrain(t *testing.T, spoolDir string, sp *spool.DiskSpool, client *coreclient.Client) *agentdrain.Drain {
	t.Helper()
	d, err := agentdrain.New(agentdrain.Config{
		SensorID:             sliceSensorID,
		AgentSpoolDir:        spoolDir,
		MaxBatchRecords:      32,
		MaxBatchPayloadBytes: 1024 * 1024,
		MaxRequestBytes:      2 * 1024 * 1024,
		RequestTimeout:       2 * time.Second,
		InitialBackoff:       time.Millisecond,
		MaxBackoff:           5 * time.Millisecond,
		JitterFraction:       0,
		IdleInterval:         time.Hour,
		ShutdownGrace:        time.Second,
	}, sp, client)
	if err != nil {
		t.Fatalf("agentdrain.New: %v", err)
	}
	return d
}

func encodeValidACK(batch coreclient.AgentTelemetryBatch, mutate func(map[string]any)) []byte {
	ack := map[string]any{
		"status":                         "ACCEPTED",
		"sensorId":                       batch.SensorID,
		"batchId":                        batch.BatchID,
		"acceptedThroughIngressSequence": batch.LastIngressSequence,
		"correlationId":                  batch.CorrelationID,
	}
	if mutate != nil {
		mutate(ack)
	}
	raw, _ := json.Marshal(ack)
	return raw
}

func assertPendingCount(t *testing.T, sp *spool.DiskSpool, want int) {
	t.Helper()
	st, err := sp.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.RecordCount != want {
		t.Fatalf("pending RecordCount=%d want %d", st.RecordCount, want)
	}
}

func assertClientCertRequired(t *testing.T, r *http.Request) {
	t.Helper()
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		t.Fatal("Mock Core must require a verified Sensor client certificate")
	}
}

func TestProductionDrainSliceHappyPath(t *testing.T) {
	spoolDir := t.TempDir()
	sp := openProductionAgentSpool(t, spoolDir)
	sink, err := agentreceiver.NewSpoolAcceptanceSink(sp, spoolDir)
	if err != nil {
		t.Fatalf("NewSpoolAcceptanceSink: %v", err)
	}

	env := sampleEnvelope(t)
	res, err := sink.AcceptBatch(context.Background(), env)
	if err != nil {
		t.Fatalf("AcceptBatch: %v", err)
	}
	if res.AcceptedThrough != env.LastSequence {
		t.Fatalf("AcceptedThrough=%d want %d", res.AcceptedThrough, env.LastSequence)
	}
	assertPendingCount(t, sp, 1)

	var gotBatch coreclient.AgentTelemetryBatch
	var gotOnce atomic.Bool
	pki := makeSlicePKI(t)
	server := startSliceMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertClientCertRequired(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/sensor/agent-telemetry" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, "read", 500)
			return
		}
		if err := json.Unmarshal(body, &gotBatch); err != nil {
			t.Errorf("decode batch: %v", err)
			http.Error(w, "json", 400)
			return
		}
		gotOnce.Store(true)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encodeValidACK(gotBatch, nil))
	}))

	client := newSliceCoreClient(t, server.URL, pki)
	d := newDrain(t, spoolDir, sp, client)

	ok, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !ok {
		t.Fatal("RunOnce returned ok=false")
	}
	if !gotOnce.Load() {
		t.Fatal("Mock Core did not receive AgentTelemetryBatch")
	}

	if gotBatch.SchemaVersion != 1 {
		t.Fatalf("schemaVersion=%d", gotBatch.SchemaVersion)
	}
	if gotBatch.SensorID != sliceSensorID {
		t.Fatalf("sensorId=%q", gotBatch.SensorID)
	}
	if gotBatch.FirstIngressSequence != 1 || gotBatch.LastIngressSequence != 1 {
		t.Fatalf("ingress range [%d..%d]", gotBatch.FirstIngressSequence, gotBatch.LastIngressSequence)
	}
	if gotBatch.RecordCount != 1 || len(gotBatch.Records) != 1 {
		t.Fatalf("recordCount=%d len=%d", gotBatch.RecordCount, len(gotBatch.Records))
	}
	if gotBatch.Checksum == "" || gotBatch.CorrelationID == "" || gotBatch.BatchID == "" {
		t.Fatalf("missing checksum/correlation/batchId: %+v", gotBatch)
	}

	rec := gotBatch.Records[0]
	if rec.TenantID != sliceTenantID || rec.AgentID != sliceAgentID || rec.AcceptingSensorID != sliceSensorID {
		t.Fatalf("identity fields: %+v", rec)
	}
	if rec.AssignmentGeneration != 1 || rec.AgentFirstSequence != 1 || rec.AgentLastSequence != 2 {
		t.Fatalf("agent range/generation: %+v", rec)
	}
	if rec.AgentBatchID != sliceBatchID || rec.CorrelationID != sliceCorrID || rec.AcceptedAt == "" {
		t.Fatalf("batch/correlation/acceptedAt: %+v", rec)
	}
	var nested []agentreceiver.DeliveryRecord
	if err := json.Unmarshal(rec.Records, &nested); err != nil {
		t.Fatalf("nested records: %v", err)
	}
	if len(nested) != 2 {
		t.Fatalf("nested len=%d", len(nested))
	}
	for i, want := range env.Records {
		if nested[i].Sequence != want.Sequence || nested[i].ID != want.ID || nested[i].Checksum != want.Checksum {
			t.Fatalf("nested[%d]=%+v want seq/id/checksum from %+v", i, nested[i], want)
		}
		if len(nested[i].Payload) == 0 {
			t.Fatalf("nested[%d] payload empty", i)
		}
	}

	assertPendingCount(t, sp, 0)
	if d.Metrics().DurableAcks.Load() != 1 {
		t.Fatalf("DurableAcks=%d", d.Metrics().DurableAcks.Load())
	}
	if d.Metrics().Successes.Load() != 1 {
		t.Fatalf("Successes=%d", d.Metrics().Successes.Load())
	}
}

func TestProductionDrainSliceInvalidACKRetainsSpool(t *testing.T) {
	spoolDir := t.TempDir()
	sp := openProductionAgentSpool(t, spoolDir)
	sink, err := agentreceiver.NewSpoolAcceptanceSink(sp, spoolDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sink.AcceptBatch(context.Background(), sampleEnvelope(t)); err != nil {
		t.Fatal(err)
	}
	assertPendingCount(t, sp, 1)

	pki := makeSlicePKI(t)
	server := startSliceMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertClientCertRequired(t, r)
		var batch coreclient.AgentTelemetryBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			http.Error(w, "json", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Structurally valid HTTP 200 with wrong batchId — production coreclient must reject.
		_, _ = w.Write(encodeValidACK(batch, func(m map[string]any) {
			m["batchId"] = "wrong-batch-id"
		}))
	}))

	client := newSliceCoreClient(t, server.URL, pki)
	d := newDrain(t, spoolDir, sp, client)
	ok, err := d.RunOnce(context.Background())
	if err == nil || ok {
		t.Fatalf("expected invalid-ACK failure, ok=%v err=%v", ok, err)
	}
	assertPendingCount(t, sp, 1)
	if d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("DurableAcks must remain 0, got %d", d.Metrics().DurableAcks.Load())
	}
}

func TestProductionDrainSliceTransientCoreFailureRetainsSpool(t *testing.T) {
	spoolDir := t.TempDir()
	sp := openProductionAgentSpool(t, spoolDir)
	sink, err := agentreceiver.NewSpoolAcceptanceSink(sp, spoolDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sink.AcceptBatch(context.Background(), sampleEnvelope(t)); err != nil {
		t.Fatal(err)
	}

	pki := makeSlicePKI(t)
	server := startSliceMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertClientCertRequired(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"ERROR","code":"ERR_CORE_UNAVAILABLE","message":"temporary"}`))
	}))

	client := newSliceCoreClient(t, server.URL, pki)
	d := newDrain(t, spoolDir, sp, client)
	ok, err := d.RunOnce(context.Background())
	if err == nil || ok {
		t.Fatalf("expected transient failure, ok=%v err=%v", ok, err)
	}
	var te *agentdrain.Error
	if !errors.As(err, &te) || !te.Transient() {
		t.Fatalf("expected transient agentdrain.Error, got %v", err)
	}
	assertPendingCount(t, sp, 1)
	if d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("DurableAcks=%d", d.Metrics().DurableAcks.Load())
	}
	if d.Metrics().FailuresTransient.Load() < 1 {
		t.Fatalf("FailuresTransient=%d", d.Metrics().FailuresTransient.Load())
	}
}

func TestProductionDrainSliceAmbiguousResponseRetainsThenRetryDrains(t *testing.T) {
	spoolDir := t.TempDir()
	sp := openProductionAgentSpool(t, spoolDir)
	sink, err := agentreceiver.NewSpoolAcceptanceSink(sp, spoolDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sink.AcceptBatch(context.Background(), sampleEnvelope(t)); err != nil {
		t.Fatal(err)
	}

	var requests atomic.Int64
	var mu sync.Mutex
	var lastBatch coreclient.AgentTelemetryBatch
	pki := makeSlicePKI(t)
	server := startSliceMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertClientCertRequired(t, r)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		n := requests.Add(1)
		var batch coreclient.AgentTelemetryBatch
		if err := json.Unmarshal(body, &batch); err != nil {
			t.Errorf("decode: %v", err)
			return
		}
		mu.Lock()
		lastBatch = batch
		mu.Unlock()

		if n == 1 {
			// Receive complete request then drop connection before a valid ACK is delivered.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("ResponseWriter is not a Hijacker")
				http.Error(w, "no hijack", 500)
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encodeValidACK(batch, nil))
	}))

	client := newSliceCoreClient(t, server.URL, pki)
	d := newDrain(t, spoolDir, sp, client)

	ok, err := d.RunOnce(context.Background())
	if err == nil || ok {
		t.Fatalf("first RunOnce must fail after ambiguous response, ok=%v err=%v", ok, err)
	}
	if requests.Load() != 1 {
		t.Fatalf("Core must have received first request, got %d", requests.Load())
	}
	assertPendingCount(t, sp, 1)
	if d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("DurableAcks after ambiguous=%d", d.Metrics().DurableAcks.Load())
	}

	ok, err = d.RunOnce(context.Background())
	if err != nil || !ok {
		t.Fatalf("second RunOnce must succeed, ok=%v err=%v", ok, err)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected 2 Core receives, got %d", requests.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if lastBatch.SensorID != sliceSensorID || lastBatch.RecordCount != 1 {
		t.Fatalf("retry batch unexpected: %+v", lastBatch)
	}
	assertPendingCount(t, sp, 0)
	if d.Metrics().DurableAcks.Load() != 1 {
		t.Fatalf("DurableAcks after retry=%d", d.Metrics().DurableAcks.Load())
	}
}

func TestProductionDrainSliceReopenPreservesAcceptedBatch(t *testing.T) {
	spoolDir := t.TempDir()
	sp1, err := spool.Open(spool.Config{Dir: spoolDir, MaxRecordBytes: 512 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := agentreceiver.NewSpoolAcceptanceSink(sp1, spoolDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sink.AcceptBatch(context.Background(), sampleEnvelope(t)); err != nil {
		t.Fatal(err)
	}
	assertPendingCount(t, sp1, 1)
	if err := sp1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	sp2 := openProductionAgentSpool(t, spoolDir)
	assertPendingCount(t, sp2, 1)

	pki := makeSlicePKI(t)
	server := startSliceMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertClientCertRequired(t, r)
		var batch coreclient.AgentTelemetryBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			http.Error(w, "json", 400)
			return
		}
		if batch.Records[0].AgentBatchID != sliceBatchID || batch.Records[0].AgentLastSequence != 2 {
			t.Errorf("reopened spool lost Agent identity: %+v", batch.Records[0])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encodeValidACK(batch, nil))
	}))

	client := newSliceCoreClient(t, server.URL, pki)
	d := newDrain(t, spoolDir, sp2, client)
	ok, err := d.RunOnce(context.Background())
	if err != nil || !ok {
		t.Fatalf("drain after reopen: ok=%v err=%v", ok, err)
	}
	assertPendingCount(t, sp2, 0)
	if d.Metrics().DurableAcks.Load() != 1 {
		t.Fatalf("DurableAcks=%d", d.Metrics().DurableAcks.Load())
	}
}
