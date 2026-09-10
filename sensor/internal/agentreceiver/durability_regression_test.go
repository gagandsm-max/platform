package agentreceiver

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"platform/sensor/internal/runtime"
	"platform/sensor/internal/spool"
)

type fingerprintOnlyTestVerifier struct{}

func (fingerprintOnlyTestVerifier) VerifyPeerIdentity(_ []*x509.Certificate) (AuthenticatedAgentIdentity, error) {
	return AuthenticatedAgentIdentity{FingerprintSHA256: strings.Repeat("a", 64)}, nil
}

type fixedAssignmentAuthorizer struct{ ctx AssignmentContext }

func (a fixedAssignmentAuthorizer) Authorize(context.Context, AuthenticatedAgentIdentity, string) (AssignmentContext, error) {
	return a.ctx, nil
}

func TestReceiverRejectsCaseMismatchedAgentClaimAfterAuthority(t *testing.T) {
	auth := fixedAssignmentAuthorizer{ctx: AssignmentContext{
		TenantID: "tenant-1", SiteID: "site-1", SensorID: "sensor-infra-01",
		AgentID: "agent-a", Generation: 1, Assigned: true,
	}}
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, fingerprintOnlyTestVerifier{}, auth, NewInMemoryAcceptanceSink())
	srv, client := mockTLSServer(rcv, "ignored-cn")
	defer srv.Close()

	env := sampleEnvelope("AGENT-A", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`)))
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("case-mismatched Agent claim must be rejected, got HTTP %d", resp.StatusCode)
	}
	var er ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		t.Fatal(err)
	}
	if er.Code != ErrCodeAuthIdentityMismatch {
		t.Fatalf("unexpected error code %q", er.Code)
	}
}

func TestDurableReplayIdentityIgnoresBatchIDButProtectsSequenceContents(t *testing.T) {
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

	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(100, "r-100", []byte(`{"v":1}`)))
	env.BatchID = "batch-a"
	env.ReceivingSensorID = "sensor-infra-01"
	env.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	same := env
	same.BatchID = "batch-b"
	if res, err := sink.AcceptBatch(context.Background(), same); err != nil || res.AcceptedThrough != 100 {
		t.Fatalf("same immutable Agent sequence with a new BatchID must deduplicate: res=%+v err=%v", res, err)
	}
	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordCount != 1 {
		t.Fatalf("same sequence/content with different BatchID appended again: recordCount=%d", stats.RecordCount)
	}

	conflict := env
	conflict.BatchID = "batch-c"
	conflict.Records = []DeliveryRecord{sampleRecord(100, "r-100-conflict", []byte(`{"v":2}`))}
	conflict.RecordCount = 1
	if _, err := sink.AcceptBatch(context.Background(), conflict); !IsDurableReplayConflict(err) {
		t.Fatalf("changed immutable content for an accepted Agent sequence must conflict, got %v", err)
	}
	stats, _ = s.Stats()
	if stats.RecordCount != 1 {
		t.Fatalf("conflicting replay must not append: recordCount=%d", stats.RecordCount)
	}
}

func TestIndexPublicationFailureBlocksFurtherAppendUntilReconciled(t *testing.T) {
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

	publishErr := errors.New("injected index publication failure")
	sink.indexPublisher = func(string, batchIndexEntry) error { return publishErr }

	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r-1", []byte(`{"v":1}`)))
	env.BatchID = "batch-index-fail"
	env.ReceivingSensorID = "sensor-infra-01"
	env.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env); !errors.Is(err, publishErr) {
		t.Fatalf("expected injected index publication failure, got %v", err)
	}
	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordCount != 1 {
		t.Fatalf("first attempt should leave exactly one durable orphan, got %d", stats.RecordCount)
	}

	// The second attempt must reconcile first. Because publication remains
	// unavailable, reconciliation fails and no second durable payload is appended.
	if _, err := sink.AcceptBatch(context.Background(), env); !errors.Is(err, publishErr) {
		t.Fatalf("expected reconciliation to remain fail-closed, got %v", err)
	}
	stats, _ = s.Stats()
	if stats.RecordCount != 1 {
		t.Fatalf("retry after index failure must not append another orphan, got %d", stats.RecordCount)
	}

	// Restore index publication. The next attempt must rebuild from spool truth,
	// deduplicate the already-durable payload, and return the durable watermark.
	sink.indexPublisher = nil
	res, err := sink.AcceptBatch(context.Background(), env)
	if err != nil || res.AcceptedThrough != 1 {
		t.Fatalf("reconciliation after index recovery failed: res=%+v err=%v", res, err)
	}
	stats, _ = s.Stats()
	if stats.RecordCount != 1 {
		t.Fatalf("successful reconciliation must not append another durable payload, got %d", stats.RecordCount)
	}
}

func TestVerifiedIndexCannotAcknowledgeMissingDurableRecord(t *testing.T) {
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
	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(7, "r-7", []byte(`{"v":7}`)))
	env.BatchID = "batch-stale-index"
	env.ReceivingSensorID = "sensor-infra-01"
	env.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	key := batchIndexKey(env.TenantID, env.AgentID, env.FirstSequence, env.LastSequence)
	entry, ok, err := sink.readIndexLocked(key)
	if err != nil || !ok {
		t.Fatalf("expected index entry: ok=%v err=%v", ok, err)
	}
	entry.SpoolSequence = 999999
	if err := sink.writeIndexLocked(key, entry); err != nil {
		t.Fatal(err)
	}
	if _, err := sink.AcceptBatch(context.Background(), env); err == nil {
		t.Fatal("index without its referenced durable record must never authorize an ACK")
	}
}

func TestRebuildIndexesEntirePendingSpoolBeyond1024(t *testing.T) {
	if testing.Short() {
		t.Skip("durability recovery boundary test")
	}
	dir := filepath.Join(t.TempDir(), "agent-spool")
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var last DeliveryEnvelope
	for i := 1; i <= 1025; i++ {
		seq := uint64(i)
		rec := sampleRecord(seq, fmt.Sprintf("r-%d", i), []byte(fmt.Sprintf(`{"v":%d}`, i)))
		env := sampleEnvelope("agent-a", "tenant-1", rec)
		env.BatchID = fmt.Sprintf("batch-%04d", i)
		env.ReceivingSensorID = "sensor-infra-01"
		env.AssignmentGeneration = 1
		payload := durableBatchPayload{
			TenantID: env.TenantID, AgentID: env.AgentID, SensorID: env.ReceivingSensorID,
			Generation: env.AssignmentGeneration, FirstSequence: env.FirstSequence,
			LastSequence: env.LastSequence, RecordIDs: []string{rec.ID},
			BatchChecksum: ComputeBatchChecksum(env.Records), BatchID: env.BatchID,
			CorrelationID: env.CorrelationID, Timestamp: time.Now().UTC(), Envelope: env,
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Append(spool.AppendRequest{Class: spool.ClassOperational, Type: agentBatchRecordType, Payload: encoded, CreatedAt: payload.Timestamp}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		last = env
	}

	sink, err := NewSpoolAcceptanceSink(s, dir)
	if err != nil {
		t.Fatalf("full recovery failed: %v", err)
	}
	lastKey := batchIndexKey(last.TenantID, last.AgentID, last.FirstSequence, last.LastSequence)
	if _, err := os.Stat(sink.indexPath(lastKey)); err != nil {
		t.Fatalf("record beyond old 1024 recovery boundary was not indexed: %v", err)
	}
	before, _ := s.Stats()
	if res, err := sink.AcceptBatch(context.Background(), last); err != nil || res.AcceptedThrough != last.LastSequence {
		t.Fatalf("retry after full rebuild failed: res=%+v err=%v", res, err)
	}
	after, _ := s.Stats()
	if after.RecordCount != before.RecordCount {
		t.Fatalf("retry after >1024 recovery appended duplicate: before=%d after=%d", before.RecordCount, after.RecordCount)
	}
}
