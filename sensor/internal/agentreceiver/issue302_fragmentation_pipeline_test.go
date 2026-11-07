//go:build pipeline

package agentreceiver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/collector"
	"platform/agent/common/delivery"
	"platform/agent/common/identity"
	"platform/agent/common/queue"
	"platform/agent/common/scheduler"
	"platform/agent/common/telemetry"
	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/runtime"
	"platform/sensor/internal/spool"
)

type issue302Collector struct{}

func (*issue302Collector) Name() string { return "fixture" }
func (*issue302Collector) Collect(context.Context) ([]telemetry.Sample, error) {
	out := make([]telemetry.Sample, 192)
	for i := range out {
		out[i] = telemetry.Sample{MetricKey: "fixture.metric", Value: float64(i), Unit: "count", Labels: map[string]string{"fixture": strings.Repeat("x", 128)}}
	}
	return out, nil
}

// Only Core authority lookup is a fixture. Certificate fingerprint derivation,
// wire transport, receiver validation and spool/replay custody are production code.
type issue302Authority struct{ fingerprint string }

func (a issue302Authority) ResolveAgentAuthority(_ context.Context, fp string) (coreclient.AgentAuthorityView, error) {
	if fp != a.fingerprint {
		return coreclient.AgentAuthorityView{}, fmt.Errorf("unassigned fingerprint")
	}
	return coreclient.AgentAuthorityView{Status: "ACCEPTED", AgentID: "agent-302", TenantID: "tenant-302", SiteID: "site-302", SensorID: "sensor-302", AssignmentGeneration: 1}, nil
}

func TestIssue302FragmentedMTLSSpoolRestartAndLostACK(t *testing.T) {
	ca, key := mustCreateCA(t)
	serverDER, serverKey := mustCreateServerKeyPair(t, ca, key)
	clientDER, clientKey := mustCreateClientKeyPair(t, ca, key, "untrusted-display-name")
	pkiDir := t.TempDir()
	caFile := writePEM(t, pkiDir, "ca.crt", "CERTIFICATE", ca.Raw)
	clientFile := writePEM(t, pkiDir, "client.crt", "CERTIFICATE", clientDER)
	keyFile := writePEM(t, pkiDir, "client.key", "PRIVATE KEY", mustMarshalEC(t, clientKey))
	leaf, err := x509.ParseCertificate(clientDER)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewCoreBackedAuthorizer(issue302Authority{LeafFingerprintSHA256(leaf)}, "sensor-302")
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	spoolDir := filepath.Join(t.TempDir(), "sensor-spool")
	var sp *spool.DiskSpool
	var server *httptest.Server
	startReceiver := func(drop bool) {
		var err error
		sp, err = spool.Open(spool.Config{Dir: spoolDir, MaxRecordBytes: DefaultMaxBodyBytes})
		if err != nil {
			t.Fatal(err)
		}
		sink, err := NewSpoolAcceptanceSink(sp, spoolDir)
		if err != nil {
			t.Fatal(err)
		}
		rcv, err := NewReceiver(Config{SensorID: "sensor-302", SensorRole: runtime.RoleInfrastructure}, NewFingerprintIdentityVerifier(), auth, sink, logger)
		if err != nil {
			t.Fatal(err)
		}
		var handler http.Handler = rcv
		if drop {
			handler = &dropOnceHandler{inner: rcv, armed: true}
		}
		server = httptest.NewUnstartedServer(handler)
		server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{serverDER}, PrivateKey: serverKey}}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS12}
		server.StartTLS()
	}
	startReceiver(true)
	defer func() { server.Close(); _ = sp.Close() }()
	queueDir := t.TempDir()
	q, err := queue.NewDurableQueue(queueDir, 100, 4<<20, 65536)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := collector.NewRegistry([]collector.Registration{{Collector: &issue302Collector{}, Enabled: true, Interval: time.Hour, Timeout: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	ident := &identity.AgentIdentity{AgentID: "agent-302", TenantID: "tenant-302", HostID: "host-302", Hostname: "node", OS: "linux", Arch: "amd64"}
	sched, err := scheduler.NewScheduler(ident, reg, q, 1, logger)
	if err != nil {
		t.Fatal(err)
	}
	sched.RunOnce(context.Background())
	if err = sched.StopContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	original, err := q.ReadBatch(100, 384*1024)
	if err != nil || len(original) < 2 {
		t.Fatalf("fragment count=%d err=%v", len(original), err)
	}
	expected := map[string]telemetry.Observation{}
	totalBytes := 0
	for _, r := range original {
		if len(r.Payload) > 65536 {
			t.Fatal("oversized admitted fragment")
		}
		totalBytes += len(r.Payload)
		var b telemetry.CollectionBatch
		if err = json.Unmarshal(r.Payload, &b); err != nil {
			t.Fatal(err)
		}
		for _, o := range b.Observations {
			if _, dup := expected[o.ObservationID]; dup {
				t.Fatal("duplicate source identity")
			}
			expected[o.ObservationID] = o
		}
	}
	if len(expected) != 192 || totalBytes <= 65536 {
		t.Fatalf("fixture count=%d bytes=%d", len(expected), totalBytes)
	}
	stream := q.AgentStreamID()
	newWorker := func(tenant string) *delivery.Worker {
		tr, err := client.NewHTTPSTransport(client.HTTPSConfig{SensorURL: server.URL, AgentID: ident.AgentID, TenantID: tenant, SiteID: "site-302", CACertFile: caFile, ClientCertFile: clientFile, ClientKeyFile: keyFile, Timeout: time.Second, Logger: logger})
		if err != nil {
			t.Fatal(err)
		}
		w, err := delivery.NewWorker(q, tr, ident.AgentID, 1, 384*1024, time.Millisecond, 10*time.Millisecond, time.Second, logger)
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	worker := newWorker(ident.TenantID)
	if delivered, err := worker.DeliverOnce(context.Background()); err == nil || delivered || q.Len() != len(original) {
		t.Fatalf("lost ACK deleted custody: delivered=%v err=%v remaining=%d", delivered, err, q.Len())
	}
	stats, err := sp.Stats()
	if err != nil || stats.RecordCount != 1 {
		t.Fatalf("durable Sensor accept missing: %+v %v", stats, err)
	}
	server.Close()
	if err = sp.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen both durable owners after the lost ACK. This is service restart,
	// not physical power-loss qualification.
	q, err = queue.NewDurableQueue(queueDir, 100, 4<<20, 65536)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := q.ReadBatch(100, 384*1024)
	if err != nil || !reflect.DeepEqual(replay, original) || q.AgentStreamID() != stream {
		t.Fatal("Agent restart changed replay")
	}
	startReceiver(false)
	wrongTenant := newWorker("tenant-other")
	if delivered, err := wrongTenant.DeliverOnce(context.Background()); err == nil || delivered || q.Len() != len(original) {
		t.Fatal("cross-tenant source accepted")
	}
	stats, err = sp.Stats()
	if err != nil || stats.RecordCount != 1 {
		t.Fatal("denied source changed Sensor custody")
	}
	worker = newWorker(ident.TenantID)
	for i := 0; i < len(original); i++ {
		if delivered, err := worker.DeliverOnce(context.Background()); err != nil || !delivered {
			t.Fatalf("reconnect/replay delivery %d: %v", i, err)
		}
	}
	if q.Len() != 0 {
		t.Fatal("valid ACK did not advance queue")
	}
	batch, err := sp.ReadBatch(100, 4<<20)
	if err != nil || len(batch.Records) != len(original) {
		t.Fatalf("Sensor replay duplicate/missing records=%d err=%v", len(batch.Records), err)
	}
	seen := map[string]bool{}
	for _, r := range batch.Records {
		var stored durableBatchPayload
		if err = json.Unmarshal(r.Payload, &stored); err != nil {
			t.Fatal(err)
		}
		if stored.TenantID != ident.TenantID || stored.AgentID != ident.AgentID || stored.AgentStreamID != stream {
			t.Fatal("stored authority changed")
		}
		for _, r := range stored.Envelope.Records {
			var b telemetry.CollectionBatch
			if err = json.Unmarshal(r.Payload, &b); err != nil {
				t.Fatal(err)
			}
			for _, o := range b.Observations {
				want, ok := expected[o.ObservationID]
				if !ok || seen[o.ObservationID] || !reflect.DeepEqual(want, o) {
					t.Fatal("stored observation missing/duplicated/altered")
				}
				seen[o.ObservationID] = true
			}
		}
	}
	if len(seen) != len(expected) {
		t.Fatalf("stored=%d expected=%d", len(seen), len(expected))
	}
	t.Logf("encoded_collection_bytes_sum=%d fragments=%d source_observations=%d stored_observations=%d; actual_mTLS_receiver_and_Sensor_spool=true; Core_authority=fixture", totalBytes, len(original), len(expected), len(seen))
}
