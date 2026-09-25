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
	"sync"
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

func TestDurableReplayRejectsOverlappingSourceSequenceRanges(t *testing.T) {
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

	first := sampleEnvelope(
		"agent-a",
		"tenant-1",
		sampleRecord(100, "r-100", []byte(`{"v":100}`)),
		sampleRecord(101, "r-101", []byte(`{"v":101}`)),
	)
	first.BatchID = "batch-100-101"
	first.ReceivingSensorID = "sensor-infra-01"
	first.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatalf("initial range accept failed: %v", err)
	}

	overlap := sampleEnvelope(
		"agent-a",
		"tenant-1",
		sampleRecord(101, "r-101", []byte(`{"v":101}`)),
		sampleRecord(102, "r-102", []byte(`{"v":102}`)),
	)
	overlap.BatchID = "batch-101-102"
	overlap.ReceivingSensorID = "sensor-infra-01"
	overlap.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), overlap); !IsDurableReplayConflict(err) {
		t.Fatalf("overlapping source sequence range must conflict, got %v", err)
	}

	contained := sampleEnvelope("agent-a", "tenant-1", sampleRecord(100, "r-100", []byte(`{"v":100}`)))
	contained.BatchID = "batch-100"
	contained.ReceivingSensorID = "sensor-infra-01"
	contained.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), contained); !IsDurableReplayConflict(err) {
		t.Fatalf("contained source sequence range must conflict, got %v", err)
	}

	disjoint := sampleEnvelope("agent-a", "tenant-1", sampleRecord(102, "r-102", []byte(`{"v":102}`)))
	disjoint.BatchID = "batch-102"
	disjoint.ReceivingSensorID = "sensor-infra-01"
	disjoint.AssignmentGeneration = 1
	if res, err := sink.AcceptBatch(context.Background(), disjoint); err != nil || res.AcceptedThrough != 102 {
		t.Fatalf("disjoint next source range should be accepted: res=%+v err=%v", res, err)
	}

	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordCount != 2 {
		t.Fatalf("overlap attempts must not append, recordCount=%d", stats.RecordCount)
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

func drainPending(t *testing.T, s *spool.DiskSpool) {
	t.Helper()
	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordCount == 0 {
		return
	}
	through := stats.NextSequence - 1
	if err := s.AcknowledgeThrough(through); err != nil {
		t.Fatalf("drain AcknowledgeThrough(%d): %v", through, err)
	}
	stats, err = s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordCount != 0 {
		t.Fatalf("expected empty pending spool after drain, got %d", stats.RecordCount)
	}
}

func TestLostAckAfterDrainIdempotentReplay(t *testing.T) {
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

	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(900, "r-900", []byte(`{"lost-ack":true}`)))
	env.BatchID = "batch-lost-ack-900"
	env.ReceivingSensorID = "sensor-infra-01"
	env.AssignmentGeneration = 1
	first, err := sink.AcceptBatch(context.Background(), env)
	if err != nil || first.AcceptedThrough != 900 {
		t.Fatalf("initial accept failed: res=%+v err=%v", first, err)
	}

	drainPending(t, s)
	if _, err := os.Stat(sink.indexPath(batchIndexKey(env.TenantID, env.AgentID, env.FirstSequence, env.LastSequence))); err != nil {
		t.Fatalf("acceptance receipt must survive drain: %v", err)
	}

	replay, err := sink.AcceptBatch(context.Background(), env)
	if err != nil || replay.AcceptedThrough != first.AcceptedThrough {
		t.Fatalf("identical post-drain replay must return original watermark: res=%+v err=%v", replay, err)
	}
	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordCount != 0 {
		t.Fatalf("post-drain identical replay appended a second logical batch: recordCount=%d", stats.RecordCount)
	}
}

func TestLostAckAfterDrainSurvivesSensorRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent-spool")
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := NewSpoolAcceptanceSink(s, dir)
	if err != nil {
		t.Fatal(err)
	}
	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(901, "r-901", []byte(`{"restart":true}`)))
	env.BatchID = "batch-lost-ack-restart-901"
	env.ReceivingSensorID = "sensor-infra-01"
	env.AssignmentGeneration = 1
	first, err := sink.AcceptBatch(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	drainPending(t, s)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	sink2, err := NewSpoolAcceptanceSink(s2, dir)
	if err != nil {
		t.Fatalf("restart recovery failed: %v", err)
	}
	replay, err := sink2.AcceptBatch(context.Background(), env)
	if err != nil || replay.AcceptedThrough != first.AcceptedThrough {
		t.Fatalf("post-restart drained replay failed: res=%+v err=%v", replay, err)
	}
	stats, err := s2.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordCount != 0 {
		t.Fatalf("restart replay appended duplicate: recordCount=%d", stats.RecordCount)
	}
}

func TestLostAckAfterDrainMutatedReplayConflicts(t *testing.T) {
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
	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(902, "r-902", []byte(`{"original":true}`)))
	env.BatchID = "batch-lost-ack-mutate-902"
	env.ReceivingSensorID = "sensor-infra-01"
	env.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	drainPending(t, s)

	mutated := env
	mutated.Records = []DeliveryRecord{sampleRecord(902, "r-902", []byte(`{"original":false}`))}
	mutated.RecordCount = 1
	if _, err := sink.AcceptBatch(context.Background(), mutated); !IsDurableReplayConflict(err) {
		t.Fatalf("mutated post-drain replay must conflict, got %v", err)
	}
	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordCount != 0 {
		t.Fatalf("mutated replay must not append: recordCount=%d", stats.RecordCount)
	}
}

func TestCompactedOldReplay(t *testing.T) {
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

	// 1. Accept sequential range 1..10
	env1 := sampleEnvelope("agent-a", "tenant-1",
		sampleRecord(1, "r-1", []byte(`{"m":1}`)),
		sampleRecord(10, "r-10", []byte(`{"m":10}`)),
	)
	env1.FirstSequence = 1
	env1.LastSequence = 10
	env1.BatchID = "batch-1-10"
	env1.ReceivingSensorID = "sensor-infra-01"
	env1.AssignmentGeneration = 1
	if res, err := sink.AcceptBatch(context.Background(), env1); err != nil || res.AcceptedThrough != 10 {
		t.Fatalf("accept 1..10 failed: res=%+v err=%v", res, err)
	}

	// 2. Drain spool
	drainPending(t, s)

	// 3. Accept next contiguous range 11..20 to advance and persist contiguous watermark to 20
	env2 := sampleEnvelope("agent-a", "tenant-1",
		sampleRecord(11, "r-11", []byte(`{"m":11}`)),
		sampleRecord(20, "r-20", []byte(`{"m":20}`)),
	)
	env2.FirstSequence = 11
	env2.LastSequence = 20
	env2.BatchID = "batch-11-20"
	env2.ReceivingSensorID = "sensor-infra-01"
	env2.AssignmentGeneration = 1
	if res, err := sink.AcceptBatch(context.Background(), env2); err != nil || res.AcceptedThrough != 20 {
		t.Fatalf("accept 11..20 failed: res=%+v err=%v", res, err)
	}

	// Check contiguous watermark
	if wm := sink.ContiguousWatermark("tenant-1", "agent-a"); wm != 20 {
		t.Fatalf("expected watermark 20, got %d", wm)
	}

	// Verify exact receipt for 1..10 was compacted and deleted from batch-index
	key1 := batchIndexKey("tenant-1", "agent-a", 1, 10)
	indexPath1 := filepath.Join(dir, batchIndexDirName, key1+".json")
	if _, err := os.Stat(indexPath1); !os.IsNotExist(err) {
		t.Fatalf("receipt 1..10 must be compacted from disk, stat err: %v", err)
	}

	// 4. Replay old compacted range 1..10
	statsBefore, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	res, err := sink.AcceptBatch(context.Background(), env1)
	if err != nil {
		t.Fatalf("compacted old replay must succeed, got err: %v", err)
	}
	if res.AcceptedThrough != 10 {
		t.Fatalf("compacted old replay acceptedThrough must be 10, got %d", res.AcceptedThrough)
	}

	// 5. Verify zero new spool appends
	statsAfter, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if statsAfter.RecordCount != statsBefore.RecordCount {
		t.Fatalf("compacted replay must not append to spool: before=%d after=%d", statsBefore.RecordCount, statsAfter.RecordCount)
	}
}

func TestCompactedOldReplayAfterSensorRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent-spool")
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := NewSpoolAcceptanceSink(s, dir)
	if err != nil {
		t.Fatal(err)
	}

	env1 := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`)))
	env1.BatchID = "batch-1"
	env1.ReceivingSensorID = "sensor-infra-01"
	env1.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env1); err != nil {
		t.Fatal(err)
	}
	drainPending(t, s)

	// Close sink and spool
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen spool and reconstruct sink
	s2, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	sink2, err := NewSpoolAcceptanceSink(s2, dir)
	if err != nil {
		t.Fatal(err)
	}

	if wm := sink2.ContiguousWatermark("tenant-1", "agent-a"); wm != 1 {
		t.Fatalf("expected reloaded watermark 1, got %d", wm)
	}

	statsBefore, err := s2.Stats()
	if err != nil {
		t.Fatal(err)
	}

	res, err := sink2.AcceptBatch(context.Background(), env1)
	if err != nil {
		t.Fatalf("compacted replay after restart failed: %v", err)
	}
	if res.AcceptedThrough != 1 {
		t.Fatalf("expected acceptedThrough 1, got %d", res.AcceptedThrough)
	}

	statsAfter, err := s2.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if statsAfter.RecordCount != statsBefore.RecordCount {
		t.Fatalf("replay after restart must not append to spool: before=%d after=%d", statsBefore.RecordCount, statsAfter.RecordCount)
	}
}

func TestPartialOverlapWithContiguousPrefix(t *testing.T) {
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

	// Accept sequential range 1..100
	recs := make([]DeliveryRecord, 100)
	for i := 1; i <= 100; i++ {
		recs[i-1] = sampleRecord(uint64(i), fmt.Sprintf("r-%d", i), []byte(fmt.Sprintf(`{"i":%d}`, i)))
	}
	env := sampleEnvelope("agent-a", "tenant-1", recs...)
	env.FirstSequence = 1
	env.LastSequence = 100
	env.BatchID = "batch-1-100"
	env.ReceivingSensorID = "sensor-infra-01"
	env.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	if wm := sink.ContiguousWatermark("tenant-1", "agent-a"); wm != 100 {
		t.Fatalf("expected watermark 100, got %d", wm)
	}

	// Incoming 95..105: overlaps 100 (FirstSequence 95 <= 100 && LastSequence 105 > 100)
	overlapRecs := make([]DeliveryRecord, 11)
	for i := 95; i <= 105; i++ {
		overlapRecs[i-95] = sampleRecord(uint64(i), fmt.Sprintf("r-%d", i), []byte(fmt.Sprintf(`{"i":%d}`, i)))
	}
	overlapEnv := sampleEnvelope("agent-a", "tenant-1", overlapRecs...)
	overlapEnv.FirstSequence = 95
	overlapEnv.LastSequence = 105
	overlapEnv.BatchID = "batch-95-105"
	overlapEnv.ReceivingSensorID = "sensor-infra-01"
	overlapEnv.AssignmentGeneration = 1

	statsBefore, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}

	_, err = sink.AcceptBatch(context.Background(), overlapEnv)
	if !IsDurableReplayConflict(err) {
		t.Fatalf("partial overlap with contiguous prefix must return DurableReplayConflictError, got: %v", err)
	}

	statsAfter, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if statsAfter.RecordCount != statsBefore.RecordCount {
		t.Fatalf("rejected partial overlap must not append to spool: before=%d after=%d", statsBefore.RecordCount, statsAfter.RecordCount)
	}
}

func TestHighSequenceDoesNotHOLBlock(t *testing.T) {
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

	// 1. Accept high sequence 900001
	highEnv := sampleEnvelope("agent-a", "tenant-1", sampleRecord(900001, "r-900001", []byte(`{"v":900001}`)))
	highEnv.BatchID = "batch-900001"
	highEnv.ReceivingSensorID = "sensor-infra-01"
	highEnv.AssignmentGeneration = 1
	if res, err := sink.AcceptBatch(context.Background(), highEnv); err != nil || res.AcceptedThrough != 900001 {
		t.Fatalf("high sequence accept failed: res=%+v err=%v", res, err)
	}

	// Contiguous watermark must remain 0
	if wm := sink.ContiguousWatermark("tenant-1", "agent-a"); wm != 0 {
		t.Fatalf("watermark must be 0 across gap, got %d", wm)
	}
	if count := sink.SparseRangeCount("tenant-1", "agent-a"); count != 1 {
		t.Fatalf("expected 1 sparse range, got %d", count)
	}

	// 2. Accept sequence 1
	env1 := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r-1", []byte(`{"v":1}`)))
	env1.BatchID = "batch-1"
	env1.ReceivingSensorID = "sensor-infra-01"
	env1.AssignmentGeneration = 1
	if res, err := sink.AcceptBatch(context.Background(), env1); err != nil || res.AcceptedThrough != 1 {
		t.Fatalf("sequence 1 must still be accepted: res=%+v err=%v", res, err)
	}
	if wm := sink.ContiguousWatermark("tenant-1", "agent-a"); wm != 1 {
		t.Fatalf("watermark must advance to 1, got %d", wm)
	}

	// 3. Accept sequence 2
	env2 := sampleEnvelope("agent-a", "tenant-1", sampleRecord(2, "r-2", []byte(`{"v":2}`)))
	env2.BatchID = "batch-2"
	env2.ReceivingSensorID = "sensor-infra-01"
	env2.AssignmentGeneration = 1
	if res, err := sink.AcceptBatch(context.Background(), env2); err != nil || res.AcceptedThrough != 2 {
		t.Fatalf("sequence 2 must be accepted: res=%+v err=%v", res, err)
	}
	if wm := sink.ContiguousWatermark("tenant-1", "agent-a"); wm != 2 {
		t.Fatalf("watermark must advance to 2, got %d", wm)
	}

	// 900001 must still remain sparse
	if count := sink.SparseRangeCount("tenant-1", "agent-a"); count != 1 {
		t.Fatalf("900001 must remain sparse, count=%d", count)
	}
}

func TestGapClosureAdvancesWatermarkAndCompacts(t *testing.T) {
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

	// 1. Accept contiguous 1..10
	env1 := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r-1", []byte(`{"v":1}`)), sampleRecord(10, "r-10", []byte(`{"v":10}`)))
	env1.FirstSequence = 1
	env1.LastSequence = 10
	env1.BatchID = "batch-1-10"
	env1.ReceivingSensorID = "sensor-infra-01"
	env1.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env1); err != nil {
		t.Fatal(err)
	}
	if wm := sink.ContiguousWatermark("tenant-1", "agent-a"); wm != 10 {
		t.Fatalf("expected watermark 10, got %d", wm)
	}

	// 2. Accept sparse 21..30
	env3 := sampleEnvelope("agent-a", "tenant-1", sampleRecord(21, "r-21", []byte(`{"v":21}`)), sampleRecord(30, "r-30", []byte(`{"v":30}`)))
	env3.FirstSequence = 21
	env3.LastSequence = 30
	env3.BatchID = "batch-21-30"
	env3.ReceivingSensorID = "sensor-infra-01"
	env3.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env3); err != nil {
		t.Fatal(err)
	}
	if wm := sink.ContiguousWatermark("tenant-1", "agent-a"); wm != 10 {
		t.Fatalf("watermark must still be 10, got %d", wm)
	}
	if count := sink.SparseRangeCount("tenant-1", "agent-a"); count != 1 {
		t.Fatalf("expected 1 sparse range, got %d", count)
	}

	drainPending(t, s)

	// 3. Accept gap-closing range 11..20
	env2 := sampleEnvelope("agent-a", "tenant-1", sampleRecord(11, "r-11", []byte(`{"v":11}`)), sampleRecord(20, "r-20", []byte(`{"v":20}`)))
	env2.FirstSequence = 11
	env2.LastSequence = 20
	env2.BatchID = "batch-11-20"
	env2.ReceivingSensorID = "sensor-infra-01"
	env2.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env2); err != nil {
		t.Fatal(err)
	}

	// Contiguous watermark should now advance to 30!
	if wm := sink.ContiguousWatermark("tenant-1", "agent-a"); wm != 30 {
		t.Fatalf("gap closure must advance watermark to 30, got %d", wm)
	}

	// Drain remaining and verify compaction
	drainPending(t, s)
	if err := sink.rebuildIndexFromSpool(); err != nil {
		t.Fatal(err)
	}
	if count := sink.SparseRangeCount("tenant-1", "agent-a"); count != 0 {
		t.Fatalf("all ranges covered by watermark 30 should be compacted, got %d remaining", count)
	}
}

func TestRestartWatermarkPersistence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent-spool")
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := NewSpoolAcceptanceSink(s, dir)
	if err != nil {
		t.Fatal(err)
	}

	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r-1", []byte(`{"v":1}`)), sampleRecord(50, "r-50", []byte(`{"v":50}`)))
	env.FirstSequence = 1
	env.LastSequence = 50
	env.BatchID = "batch-1-50"
	env.ReceivingSensorID = "sensor-infra-01"
	env.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	drainPending(t, s)

	// Close sink and spool
	_ = s.Close()

	// Reopen
	s2, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	sink2, err := NewSpoolAcceptanceSink(s2, dir)
	if err != nil {
		t.Fatal(err)
	}

	if wm := sink2.ContiguousWatermark("tenant-1", "agent-a"); wm != 50 {
		t.Fatalf("expected watermark 50 after restart, got %d", wm)
	}

	// Replaying compacted range 1..50 must return ACCEPTED with zero new appends
	res, err := sink2.AcceptBatch(context.Background(), env)
	if err != nil || res.AcceptedThrough != 50 {
		t.Fatalf("compacted replay after restart failed: res=%+v err=%v", res, err)
	}
	stats, err := s2.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordCount != 0 {
		t.Fatalf("replay must not append: recordCount=%d", stats.RecordCount)
	}
}

func TestCorruptWatermarkStateFailsClosed(t *testing.T) {
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

	// Accept a valid batch so a watermark file is created
	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r-1", []byte(`{"v":1}`)))
	env.BatchID = "batch-1"
	env.ReceivingSensorID = "sensor-infra-01"
	env.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	stateDir := filepath.Join(dir, streamStateDirName)
	streamKey := sourceRangeStreamKey("tenant-1", "agent-a")
	stateFile := filepath.Join(stateDir, streamKey+".json")

	// Test case 1: Truncated JSON
	if err := os.WriteFile(stateFile, []byte(`{"tenantId":"tenant-1",`), 0640); err != nil {
		t.Fatal(err)
	}
	if err := sink.rebuildIndexFromSpool(); err == nil {
		t.Fatal("truncated watermark JSON must fail closed")
	}

	// Test case 2: Invalid identity (empty agentId)
	if err := os.WriteFile(stateFile, []byte(`{"tenantId":"tenant-1","agentId":"","contiguousAcceptedThrough":1}`), 0640); err != nil {
		t.Fatal(err)
	}
	if err := sink.rebuildIndexFromSpool(); err == nil {
		t.Fatal("watermark file missing agentId must fail closed")
	}

	// Test case 3: Key does not match identity
	wrongKeyFile := filepath.Join(stateDir, "0000000000000000000000000000000000000000000000000000000000000000.json")
	if err := os.WriteFile(wrongKeyFile, []byte(`{"tenantId":"tenant-1","agentId":"agent-a","contiguousAcceptedThrough":1}`), 0640); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(wrongKeyFile)
	if err := sink.rebuildIndexFromSpool(); err == nil {
		t.Fatal("mismatched watermark key must fail closed")
	}
}

func TestConcurrentIdenticalDuplicates(t *testing.T) {
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

	env := sampleEnvelope("agent-a", "tenant-1",
		sampleRecord(1, "r-1", []byte(`{"v":1}`)),
		sampleRecord(5, "r-5", []byte(`{"v":5}`)),
	)
	env.FirstSequence = 1
	env.LastSequence = 5
	env.BatchID = "batch-concurrent-1-5"
	env.ReceivingSensorID = "sensor-infra-01"
	env.AssignmentGeneration = 1

	const goroutines = 10
	errs := make(chan error, goroutines)
	results := make(chan AcceptanceResult, goroutines)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := sink.AcceptBatch(context.Background(), env)
			if err != nil {
				errs <- err
			} else {
				results <- res
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(results)

	for err := range errs {
		t.Fatalf("unexpected error from concurrent submission: %v", err)
	}
	count := 0
	for res := range results {
		count++
		if res.AcceptedThrough != 5 {
			t.Fatalf("expected acceptedThrough 5, got %d", res.AcceptedThrough)
		}
	}
	if count != goroutines {
		t.Fatalf("expected %d results, got %d", goroutines, count)
	}

	// Exactly ONE spool record should have been appended!
	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordCount != 1 {
		t.Fatalf("expected exactly 1 spool record, got %d", stats.RecordCount)
	}
}

func TestConcurrentConflictingDuplicates(t *testing.T) {
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

	envA := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r-1", []byte(`{"version":"A"}`)))
	envA.BatchID = "batch-1-A"
	envA.ReceivingSensorID = "sensor-infra-01"
	envA.AssignmentGeneration = 1

	envB := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r-1", []byte(`{"version":"B"}`)))
	envB.BatchID = "batch-1-B"
	envB.ReceivingSensorID = "sensor-infra-01"
	envB.AssignmentGeneration = 1

	const attempts = 8
	var wg sync.WaitGroup
	var successCount, conflictCount int
	var countMu sync.Mutex

	for i := 0; i < attempts; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := sink.AcceptBatch(context.Background(), envA)
			countMu.Lock()
			defer countMu.Unlock()
			if err == nil {
				successCount++
			} else if IsDurableReplayConflict(err) {
				conflictCount++
			}
		}()
		go func() {
			defer wg.Done()
			_, err := sink.AcceptBatch(context.Background(), envB)
			countMu.Lock()
			defer countMu.Unlock()
			if err == nil {
				successCount++
			} else if IsDurableReplayConflict(err) {
				conflictCount++
			}
		}()
	}
	wg.Wait()

	// Exactly one immutable version won and was appended; conflicting version got conflict
	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordCount != 1 {
		t.Fatalf("expected exactly 1 winning spool record, got %d", stats.RecordCount)
	}
	if successCount < 1 || conflictCount < 1 {
		t.Fatalf("expected both successes for winning version and conflicts for losing version: success=%d conflict=%d", successCount, conflictCount)
	}
}

func TestSparseStateCapacityGuardsWithoutEviction(t *testing.T) {
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

	// Set sparse limit to 2 for this test
	sink.SetMaxSparseRangesForTest(2)

	// 1. Accept sparse range 1: 500..505
	env1 := sampleEnvelope("agent-a", "tenant-1", sampleRecord(500, "r-500", []byte(`{"v":1}`)), sampleRecord(505, "r-505", []byte(`{"v":1}`)))
	env1.FirstSequence = 500
	env1.LastSequence = 505
	env1.BatchID = "batch-500"
	env1.ReceivingSensorID = "sensor-infra-01"
	env1.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env1); err != nil {
		t.Fatalf("sparse range 1 accept failed: %v", err)
	}

	// 2. Accept sparse range 2: 600..605
	env2 := sampleEnvelope("agent-a", "tenant-1", sampleRecord(600, "r-600", []byte(`{"v":2}`)), sampleRecord(605, "r-605", []byte(`{"v":2}`)))
	env2.FirstSequence = 600
	env2.LastSequence = 605
	env2.BatchID = "batch-600"
	env2.ReceivingSensorID = "sensor-infra-01"
	env2.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), env2); err != nil {
		t.Fatalf("sparse range 2 accept failed: %v", err)
	}

	if count := sink.SparseRangeCount("tenant-1", "agent-a"); count != 2 {
		t.Fatalf("expected 2 sparse ranges, got %d", count)
	}

	// 3. Attempting 3rd disjoint sparse range (700..705) MUST fail closed
	env3 := sampleEnvelope("agent-a", "tenant-1", sampleRecord(700, "r-700", []byte(`{"v":3}`)), sampleRecord(705, "r-705", []byte(`{"v":3}`)))
	env3.FirstSequence = 700
	env3.LastSequence = 705
	env3.BatchID = "batch-700"
	env3.ReceivingSensorID = "sensor-infra-01"
	env3.AssignmentGeneration = 1

	statsBefore, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sink.AcceptBatch(context.Background(), env3); err == nil {
		t.Fatal("excess disjoint sparse range must be rejected when capacity is reached")
	}

	// Must NOT append to spool
	statsAfter, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if statsAfter.RecordCount != statsBefore.RecordCount {
		t.Fatalf("rejected range must not append: before=%d after=%d", statsBefore.RecordCount, statsAfter.RecordCount)
	}

	// Existing sparse ranges must NOT be evicted!
	if count := sink.SparseRangeCount("tenant-1", "agent-a"); count != 2 {
		t.Fatalf("existing sparse ranges must not be evicted, count=%d", count)
	}

	// Replay of existing sparse range 1 must still be recognized!
	if res, err := sink.AcceptBatch(context.Background(), env1); err != nil || res.AcceptedThrough != 505 {
		t.Fatalf("existing sparse range 1 must remain replayable: res=%+v err=%v", res, err)
	}

	// 4. Legitimate contiguous gap-closing traffic (1..10) MUST STILL BE ACCEPTED
	contiguousEnv := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r-1", []byte(`{"v":1}`)), sampleRecord(10, "r-10", []byte(`{"v":10}`)))
	contiguousEnv.FirstSequence = 1
	contiguousEnv.LastSequence = 10
	contiguousEnv.BatchID = "batch-1-10"
	contiguousEnv.ReceivingSensorID = "sensor-infra-01"
	contiguousEnv.AssignmentGeneration = 1

	if res, err := sink.AcceptBatch(context.Background(), contiguousEnv); err != nil || res.AcceptedThrough != 10 {
		t.Fatalf("contiguous gap-closing traffic must be accepted even when sparse limit reached: res=%+v err=%v", res, err)
	}
	if wm := sink.ContiguousWatermark("tenant-1", "agent-a"); wm != 10 {
		t.Fatalf("expected watermark 10, got %d", wm)
	}
}

func TestTenantAndAgentStreamWatermarkIsolation(t *testing.T) {
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

	// Accept 1..100 for Tenant A, Agent X
	envAX := sampleEnvelope("agent-x", "tenant-a", sampleRecord(1, "r-1", []byte(`{"t":"a","a":"x"}`)), sampleRecord(100, "r-100", []byte(`{"t":"a","a":"x"}`)))
	envAX.FirstSequence = 1
	envAX.LastSequence = 100
	envAX.BatchID = "batch-a-x"
	envAX.ReceivingSensorID = "sensor-infra-01"
	envAX.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), envAX); err != nil {
		t.Fatal(err)
	}

	// Watermark for Tenant A / Agent X is 100
	if wm := sink.ContiguousWatermark("tenant-a", "agent-x"); wm != 100 {
		t.Fatalf("expected watermark 100, got %d", wm)
	}

	// Other streams must remain at watermark 0
	if wm := sink.ContiguousWatermark("tenant-b", "agent-x"); wm != 0 {
		t.Fatalf("tenant-b/agent-x watermark must be 0, got %d", wm)
	}
	if wm := sink.ContiguousWatermark("tenant-a", "agent-y"); wm != 0 {
		t.Fatalf("tenant-a/agent-y watermark must be 0, got %d", wm)
	}
	if wm := sink.ContiguousWatermark("tenant-b", "agent-y"); wm != 0 {
		t.Fatalf("tenant-b/agent-y watermark must be 0, got %d", wm)
	}

	// Submitting sequence 1..10 for Tenant B / Agent X must be accepted and advance ITS OWN watermark to 10
	envBX := sampleEnvelope("agent-x", "tenant-b", sampleRecord(1, "r-1", []byte(`{"t":"b","a":"x"}`)), sampleRecord(10, "r-10", []byte(`{"t":"b","a":"x"}`)))
	envBX.FirstSequence = 1
	envBX.LastSequence = 10
	envBX.BatchID = "batch-b-x"
	envBX.ReceivingSensorID = "sensor-infra-01"
	envBX.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), envBX); err != nil {
		t.Fatal(err)
	}
	if wm := sink.ContiguousWatermark("tenant-b", "agent-x"); wm != 10 {
		t.Fatalf("expected tenant-b/agent-x watermark 10, got %d", wm)
	}
	// Tenant A / Agent X still 100
	if wm := sink.ContiguousWatermark("tenant-a", "agent-x"); wm != 100 {
		t.Fatalf("tenant-a/agent-x watermark must still be 100, got %d", wm)
	}
}

func TestLegacyV1PathUnchangedUnderSeparateModernOwner(t *testing.T) {
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

	// Distinct agents: legacy agent-legacy remains on v1 while agent-modern adopts v2.
	legacy := sampleEnvelope("agent-legacy", "tenant-1", sampleRecord(1, "r1", []byte(`{"v":1}`)))
	legacy.ReceivingSensorID = "sensor-infra-01"
	legacy.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	modern := sampleV2Envelope("agent-modern", "tenant-1", testStreamA(), sampleRecord(1, "rm1", []byte(`{"v":2}`)))
	modern.ReceivingSensorID = "sensor-infra-01"
	modern.AssignmentGeneration = 1
	if _, err := sink.AcceptBatch(context.Background(), modern); err != nil {
		t.Fatal(err)
	}
	if sink.ContiguousWatermark("tenant-1", "agent-legacy") != 1 {
		t.Fatal("legacy agent watermark must remain independent")
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-legacy") != "" {
		t.Fatal("legacy agent must not have modern owner")
	}
}
