package agentreceiver

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"platform/sensor/internal/runtime"
	"platform/sensor/internal/spool"
)

func TestReceiverReplayCacheCannotAcknowledgeMissingDurableRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent-spool")
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	sink, err := NewSpoolAcceptanceSink(s, dir)
	if err != nil {
		t.Fatal(err)
	}

	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-a", "agent-a")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.AllowGeneration("agent-a", "sensor-infra-01", "tenant-1", "site-1", 1)

	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)
	srv, client := mockTLSServer(rcv, "agent-a")
	defer srv.Close()

	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r-1", []byte(`{"v":1}`)))
	env.SiteID = "site-1"
	env.BatchID = "batch-durable-replay"
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	first := postJSON(t, client, srv.URL+EndpointPath, body)
	defer first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("initial durable acceptance failed: HTTP %d", first.StatusCode)
	}
	if rcv.ReplayTracker().Size() != 1 {
		t.Fatalf("expected process-local replay entry after acceptance, size=%d", rcv.ReplayTracker().Size())
	}

	key := batchIndexKey("tenant-1", "agent-a", 1, 1)
	entry, ok, err := sink.readIndexLocked(key)
	if err != nil || !ok {
		t.Fatalf("expected durable index entry: ok=%v err=%v", ok, err)
	}
	entry.SpoolSequence = 999999
	if err := sink.writeIndexLocked(key, entry); err != nil {
		t.Fatal(err)
	}

	// The exact same BatchID/range/checksum is still present in ReplayTracker.
	// It must not produce a cached 200; the Receiver must re-enter the durable
	// sink, discover that durable spool truth cannot be verified, and fail closed.
	retry := postJSON(t, client, srv.URL+EndpointPath, body)
	defer retry.Body.Close()
	if retry.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("process-local replay cache bypassed durable verification: HTTP %d", retry.StatusCode)
	}
	var er ErrorResponse
	if err := json.NewDecoder(retry.Body).Decode(&er); err != nil {
		t.Fatal(err)
	}
	if er.Code != ErrCodeSinkFailed {
		t.Fatalf("unexpected replay durability error code %q", er.Code)
	}
}

func TestReceiverLostAckAfterDrainReturnsEquivalentAcceptance(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent-spool")
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sink, err := NewSpoolAcceptanceSink(s, dir)
	if err != nil {
		t.Fatal(err)
	}

	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-a", "agent-a")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.AllowGeneration("agent-a", "sensor-infra-01", "tenant-1", "site-1", 1)
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)
	srv, client := mockTLSServer(rcv, "agent-a")
	defer srv.Close()

	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(44, "r-44", []byte(`{"lost":true}`)))
	env.SiteID = "site-1"
	env.BatchID = "batch-http-lost-ack"
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	first := postJSON(t, client, srv.URL+EndpointPath, body)
	defer first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("initial accept: HTTP %d", first.StatusCode)
	}
	var ack1 DeliveryAck
	if err := json.NewDecoder(first.Body).Decode(&ack1); err != nil {
		t.Fatal(err)
	}

	drainPending(t, s)

	retry := postJSON(t, client, srv.URL+EndpointPath, body)
	defer retry.Body.Close()
	if retry.StatusCode != http.StatusOK {
		t.Fatalf("post-drain identical replay: HTTP %d", retry.StatusCode)
	}
	var ack2 DeliveryAck
	if err := json.NewDecoder(retry.Body).Decode(&ack2); err != nil {
		t.Fatal(err)
	}
	if ack2.Status != "ACCEPTED" || ack2.AcceptedThroughSequence != ack1.AcceptedThroughSequence {
		t.Fatalf("expected equivalent acceptance, first=%+v retry=%+v", ack1, ack2)
	}
	stats, _ := s.Stats()
	if stats.RecordCount != 0 {
		t.Fatalf("HTTP replay after drain appended: recordCount=%d", stats.RecordCount)
	}

	mutated := env
	mutated.Records = []DeliveryRecord{sampleRecord(44, "r-44", []byte(`{"lost":false}`))}
	mutated.RecordCount = 1
	mutBody, err := json.Marshal(mutated)
	if err != nil {
		t.Fatal(err)
	}
	conflict := postJSON(t, client, srv.URL+EndpointPath, mutBody)
	defer conflict.Body.Close()
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("mutated post-drain replay expected 409, got %d", conflict.StatusCode)
	}
}

func TestReceiverExactReplayDoesNotBypassDurableOwnerAuthority(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent-spool")
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sink, err := NewSpoolAcceptanceSink(s, dir)
	if err != nil {
		t.Fatal(err)
	}
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-a", "agent-a")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.AllowGeneration("agent-a", "sensor-infra-01", "tenant-1", "site-1", 1)
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)
	srv, client := mockTLSServer(rcv, "agent-a")
	defer srv.Close()

	stream := testStreamA()
	env1 := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	env1.SiteID = "site-1"
	env1.BatchID = "batch-a-1"
	body1, err := json.Marshal(env1)
	if err != nil {
		t.Fatal(err)
	}
	first := postJSON(t, client, srv.URL+EndpointPath, body1)
	defer first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("HTTP %d", first.StatusCode)
	}

	env2 := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(2, "r2", []byte(`{"v":2}`)))
	env2.SiteID = "site-1"
	env2.BatchID = "batch-a-2"
	body2, err := json.Marshal(env2)
	if err != nil {
		t.Fatal(err)
	}
	second := postJSON(t, client, srv.URL+EndpointPath, body2)
	defer second.Body.Close()
	if second.StatusCode != http.StatusOK {
		t.Fatalf("HTTP %d", second.StatusCode)
	}
	if rcv.ReplayTracker().Size() < 1 {
		t.Fatal("expected replay cache entry")
	}

	// Fence stream A by establishing B as owner via sink directly.
	b := testStreamB()
	replace := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "rb", []byte(`{"v":9}`)))
	replace.ReceivingSensorID = "sensor-infra-01"
	replace.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), replace); err != nil {
		t.Fatal(err)
	}

	// Exact process-local replay of A's seq>1 batch must re-enter durable sink and get 409.
	retry := postJSON(t, client, srv.URL+EndpointPath, body2)
	defer retry.Body.Close()
	if retry.StatusCode != http.StatusConflict {
		t.Fatalf("exact replay must not bypass owner authority, HTTP %d", retry.StatusCode)
	}
	var er ErrorResponse
	if err := json.NewDecoder(retry.Body).Decode(&er); err != nil {
		t.Fatal(err)
	}
	if er.Code != ErrCodeStreamStaleOrFenced {
		t.Fatalf("code %q", er.Code)
	}
}
