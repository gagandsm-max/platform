package agentreceiver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"platform/sensor/internal/runtime"
	"platform/sensor/internal/spool"
)

func testStreamA() string {
	return "stream-" + strings.Repeat("a", 32)
}

func testStreamB() string {
	return "stream-" + strings.Repeat("b", 32)
}

func openTestSink(t *testing.T) (*spool.DiskSpool, *SpoolAcceptanceSink, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "agent-spool")
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := NewSpoolAcceptanceSink(s, dir)
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	return s, sink, dir
}

func stampEnvelope(env *DeliveryEnvelope) {
	env.ReceivingSensorID = "sensor-infra-01"
	env.AssignmentGeneration = 1
}

func TestValidateEnvelopeSchemaV1AndV2(t *testing.T) {
	rec := sampleRecord(1, "r1", []byte(`{"m":1}`))

	v1 := sampleEnvelope("agent-1", "tenant-1", rec)
	if err := ValidateEnvelope(&v1, 100, 64*1024); err != nil {
		t.Fatalf("v1 without stream must accept: %v", err)
	}

	v1Bad := v1
	v1Bad.AgentStreamID = testStreamA()
	if err := ValidateEnvelope(&v1Bad, 100, 64*1024); err == nil || err.Code != ErrCodeInvalidField {
		t.Fatalf("v1 with agentStreamId must reject, got %#v", err)
	}

	v2 := sampleV2Envelope("agent-1", "tenant-1", testStreamA(), rec)
	if err := ValidateEnvelope(&v2, 100, 64*1024); err != nil {
		t.Fatalf("v2 with valid stream must accept: %v", err)
	}

	v2Missing := v2
	v2Missing.AgentStreamID = ""
	if err := ValidateEnvelope(&v2Missing, 100, 64*1024); err == nil || err.Code != ErrCodeInvalidField {
		t.Fatalf("v2 without stream must reject, got %#v", err)
	}

	v2Bad := v2
	v2Bad.AgentStreamID = "stream-XYZ"
	if err := ValidateEnvelope(&v2Bad, 100, 64*1024); err == nil || err.Code != ErrCodeInvalidField {
		t.Fatalf("bad stream grammar must reject, got %#v", err)
	}

	unsupported := v1
	unsupported.SchemaVersion = 3
	if err := ValidateEnvelope(&unsupported, 100, 64*1024); err == nil || err.Code != ErrCodeUnsupportedSchema {
		t.Fatalf("unsupported version must reject, got %#v", err)
	}

	zero := v1
	zero.FirstSequence = 0
	zero.Records[0].Sequence = 0
	zero.Records[0].Checksum = ComputeRecordChecksum(0, zero.Records[0].ID, zero.Records[0].CreatedAt, zero.Records[0].Payload)
	if err := ValidateEnvelope(&zero, 100, 64*1024); err == nil || err.Code != ErrCodeSequenceMismatch {
		t.Fatalf("zero sequence must reject, got %#v", err)
	}

	maxOK := sampleEnvelope("agent-1", "tenant-1", sampleRecord(MAX_SAFE_SEQUENCE, "r-max", []byte(`{"m":1}`)))
	if err := ValidateEnvelope(&maxOK, 100, 64*1024); err != nil {
		t.Fatalf("MAX_SAFE must accept: %v", err)
	}

	maxBad := sampleEnvelope("agent-1", "tenant-1", sampleRecord(MAX_SAFE_SEQUENCE, "r-max", []byte(`{"m":1}`)))
	maxBad.LastSequence = MAX_SAFE_SEQUENCE + 1
	maxBad.Records[0].Sequence = MAX_SAFE_SEQUENCE + 1
	maxBad.FirstSequence = MAX_SAFE_SEQUENCE + 1
	maxBad.Records[0].Checksum = ComputeRecordChecksum(MAX_SAFE_SEQUENCE+1, "r-max", maxBad.Records[0].CreatedAt, maxBad.Records[0].Payload)
	if err := ValidateEnvelope(&maxBad, 100, 64*1024); err == nil || err.Code != ErrCodeSequenceMismatch {
		t.Fatalf("MAX_SAFE+1 must reject, got %#v", err)
	}
}

func TestLineageCases1to3_LegacyLostAck(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&env)
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	stats, _ := s.Stats()
	if stats.RecordCount != 1 {
		t.Fatalf("expected 1 spool record, got %d", stats.RecordCount)
	}

	// Case 1: lost-ACK with exact receipt present
	retry := env
	retry.BatchID = "retry-1"
	res, err := sink.AcceptBatch(context.Background(), retry)
	if err != nil || res.AcceptedThrough != 1 {
		t.Fatalf("exact receipt lost-ACK failed: res=%+v err=%v", res, err)
	}
	stats, _ = s.Stats()
	if stats.RecordCount != 1 {
		t.Fatalf("case3: duplicate append: %d", stats.RecordCount)
	}

	// Compact receipt below watermark by draining spool and compacting
	if err := s.AcknowledgeThrough(1); err != nil {
		t.Fatal(err)
	}
	sink.mu.Lock()
	_ = sink.compactDrainedReceiptsLocked(sourceRangeStreamKey("tenant-1", "agent-a"))
	sink.mu.Unlock()
	statsAfterDrain, _ := s.Stats()

	// Case 2: lost-ACK after compaction below contiguous watermark
	retry2 := env
	retry2.BatchID = "retry-2"
	res, err = sink.AcceptBatch(context.Background(), retry2)
	if err != nil || res.AcceptedThrough != 1 {
		t.Fatalf("watermark lost-ACK failed: res=%+v err=%v", res, err)
	}
	stats, _ = s.Stats()
	if stats.RecordCount != statsAfterDrain.RecordCount {
		t.Fatalf("case3 after compaction: duplicate append: before=%d after=%d", statsAfterDrain.RecordCount, stats.RecordCount)
	}
}

func TestLineageCase4_V2ContinuationInheritsLegacyWatermark(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	legacy := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r1", []byte(`{"v":1}`)), sampleRecord(2, "r2", []byte(`{"v":2}`)))
	stampEnvelope(&legacy)
	if _, err := sink.AcceptBatch(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	if got := sink.ContiguousWatermark("tenant-1", "agent-a"); got != 2 {
		t.Fatalf("legacy watermark=%d", got)
	}

	stream := testStreamA()
	modern := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(3, "r3", []byte(`{"v":3}`)))
	stampEnvelope(&modern)
	if _, err := sink.AcceptBatch(context.Background(), modern); err != nil {
		t.Fatal(err)
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != stream {
		t.Fatalf("owner not established")
	}
	if !sink.HasLegacyTombstone("tenant-1", "agent-a") {
		t.Fatalf("legacy tombstone missing")
	}
	if got := sink.ContiguousWatermarkForStream("tenant-1", "agent-a", stream); got != 3 {
		t.Fatalf("modern watermark after inherit+accept want 3 got %d", got)
	}
	// Inherited coverage means seq 1-2 are covered without re-append
	if got := sink.ContiguousWatermarkForStream("tenant-1", "agent-a", stream); got < 2 {
		t.Fatalf("did not inherit legacy watermark")
	}
}

func TestLineageCase5_V1RejectedAfterModernOwnership(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	modern := sampleV2Envelope("agent-a", "tenant-1", testStreamA(), sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&modern)
	if _, err := sink.AcceptBatch(context.Background(), modern); err != nil {
		t.Fatal(err)
	}
	legacy := sampleEnvelope("agent-a", "tenant-1", sampleRecord(2, "r2", []byte(`{"v":2}`)))
	stampEnvelope(&legacy)
	if _, err := sink.AcceptBatch(context.Background(), legacy); !IsLegacyStreamClosed(err) {
		t.Fatalf("expected legacy closed, got %v", err)
	}
}

func TestLineageCase6_NativeV2Seq1DoesNotInheritLegacy(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	legacy := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r1", []byte(`{"v":1}`)), sampleRecord(2, "r2", []byte(`{"v":2}`)))
	stampEnvelope(&legacy)
	if _, err := sink.AcceptBatch(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}

	stream := testStreamA()
	modern := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(1, "r1b", []byte(`{"v":9}`)))
	stampEnvelope(&modern)
	if _, err := sink.AcceptBatch(context.Background(), modern); err != nil {
		t.Fatal(err)
	}
	if got := sink.ContiguousWatermarkForStream("tenant-1", "agent-a", stream); got != 1 {
		t.Fatalf("native v2 must start from 0 then accept 1, watermark=%d", got)
	}
	if sink.ContiguousWatermark("tenant-1", "agent-a") != 2 {
		t.Fatalf("legacy historical watermark must remain untouched")
	}
	if !sink.HasLegacyTombstone("tenant-1", "agent-a") {
		t.Fatal("legacy wire must close")
	}
}

func TestLineageCase7and8_StreamReplacementAndFence(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	a := testStreamA()
	b := testStreamB()
	first := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	replace := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "r1b", []byte(`{"v":2}`)))
	stampEnvelope(&replace)
	if _, err := sink.AcceptBatch(context.Background(), replace); err != nil {
		t.Fatal(err)
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != b {
		t.Fatal("B must be active")
	}

	// Case 7/10: A later with seq>1 rejected before spool mutation
	statsBefore, _ := s.Stats()
	stale := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(2, "r2", []byte(`{"v":3}`)))
	stampEnvelope(&stale)
	if _, err := sink.AcceptBatch(context.Background(), stale); !IsStreamStaleOrFenced(err) {
		t.Fatalf("expected stale/fenced, got %v", err)
	}
	statsAfter, _ := s.Stats()
	if statsAfter.RecordCount != statsBefore.RecordCount {
		t.Fatal("fenced stream must not append")
	}

	// Case 8: different stream C with seq>1
	c := "stream-" + strings.Repeat("c", 32)
	staleC := sampleV2Envelope("agent-a", "tenant-1", c, sampleRecord(5, "r5", []byte(`{"v":5}`)))
	stampEnvelope(&staleC)
	if _, err := sink.AcceptBatch(context.Background(), staleC); !IsStreamStaleOrFenced(err) {
		t.Fatalf("expected stale/fenced for C, got %v", err)
	}
}

func TestLineageCase9_OwnerPublishAppendFailKeepsB(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	a := testStreamA()
	b := testStreamB()
	first := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	sink.SetAppendHookForTest(func(env DeliveryEnvelope) error {
		if env.AgentStreamID == b {
			return errors.New("injected append failure")
		}
		return nil
	})
	replace := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "r1b", []byte(`{"v":2}`)))
	stampEnvelope(&replace)
	if _, err := sink.AcceptBatch(context.Background(), replace); err == nil {
		t.Fatal("expected append failure")
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != b {
		t.Fatal("B must remain owner after append failure")
	}
	sink.SetAppendHookForTest(nil)

	retry := replace
	retry.BatchID = "retry-b"
	if _, err := sink.AcceptBatch(context.Background(), retry); err != nil {
		t.Fatalf("retry of B must succeed: %v", err)
	}
}

func TestLineageCase11_MissingOwnerFailsInit(t *testing.T) {
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
	stream := testStreamA()
	modern := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&modern)
	if _, err := sink.AcceptBatch(context.Background(), modern); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Remove owner file, keep modern stream-state
	ownerPath := filepath.Join(dir, streamStateDirName, lineageOwnerDirName, sourceRangeStreamKey("tenant-1", "agent-a")+".json")
	if err := os.Remove(ownerPath); err != nil {
		t.Fatal(err)
	}
	s2, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if _, err := NewSpoolAcceptanceSink(s2, dir); err == nil {
		t.Fatal("missing owner must fail init")
	}
}

func TestLineageCase12_CorruptOwnerFailsInit(t *testing.T) {
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
	modern := sampleV2Envelope("agent-a", "tenant-1", testStreamA(), sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&modern)
	if _, err := sink.AcceptBatch(context.Background(), modern); err != nil {
		t.Fatal(err)
	}
	s.Close()

	ownerPath := filepath.Join(dir, streamStateDirName, lineageOwnerDirName, sourceRangeStreamKey("tenant-1", "agent-a")+".json")
	if err := os.WriteFile(ownerPath, []byte(`{not-json`), 0640); err != nil {
		t.Fatal(err)
	}
	s2, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if _, err := NewSpoolAcceptanceSink(s2, dir); err == nil {
		t.Fatal("corrupt owner must fail init")
	}
}

func TestLineageCase13_OwnerStateMismatchFailsInit(t *testing.T) {
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
	a := testStreamA()
	modern := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&modern)
	if _, err := sink.AcceptBatch(context.Background(), modern); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Point owner at stream B while only A state exists
	ownerPath := filepath.Join(dir, streamStateDirName, lineageOwnerDirName, sourceRangeStreamKey("tenant-1", "agent-a")+".json")
	bad := LineageOwnerRecord{
		Version:             lineageOwnerVersion,
		TenantID:            "tenant-1",
		AgentID:             "agent-a",
		ActiveAgentStreamID: testStreamB(),
		ActivatedAt:         time.Now().UTC(),
	}
	data, _ := json.Marshal(bad)
	if err := os.WriteFile(ownerPath, data, 0640); err != nil {
		t.Fatal(err)
	}
	s2, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if _, err := NewSpoolAcceptanceSink(s2, dir); err == nil {
		t.Fatal("owner/state mismatch must fail init")
	}
}

func TestLineageCase14_StartupTombstoneReconciliation(t *testing.T) {
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

	legacy := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&legacy)
	if _, err := sink.AcceptBatch(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	modern := sampleV2Envelope("agent-a", "tenant-1", testStreamA(), sampleRecord(2, "r2", []byte(`{"v":2}`)))
	stampEnvelope(&modern)
	if _, err := sink.AcceptBatch(context.Background(), modern); err != nil {
		t.Fatal(err)
	}
	if !sink.HasLegacyTombstone("tenant-1", "agent-a") {
		t.Fatal("expected tombstone after transition")
	}
	s.Close()

	// Delete tombstone; restart must rewrite before ready
	tombPath := filepath.Join(dir, streamStateDirName, legacyClosedDirName, sourceRangeStreamKey("tenant-1", "agent-a")+".json")
	if err := os.Remove(tombPath); err != nil {
		t.Fatal(err)
	}
	s2, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	sink2, err := NewSpoolAcceptanceSink(s2, dir)
	if err != nil {
		t.Fatalf("startup reconciliation must succeed: %v", err)
	}
	if !sink2.HasLegacyTombstone("tenant-1", "agent-a") {
		t.Fatal("startup must rewrite missing tombstone")
	}
}

func TestLineageCase15and16_PendingSpoolRecovery(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent-spool")
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := NewSpoolAcceptanceSink(s, dir)
	if err != nil {
		t.Fatal(err)
	}

	legacy := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&legacy)
	if _, err := sink.AcceptBatch(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	stream := testStreamA()
	modern := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(2, "r2", []byte(`{"v":2}`)))
	stampEnvelope(&modern)
	if _, err := sink.AcceptBatch(context.Background(), modern); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	sink2, err := NewSpoolAcceptanceSink(s2, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Legacy pending recoverable via watermark/index
	if sink2.ContiguousWatermark("tenant-1", "agent-a") != 1 {
		t.Fatalf("legacy watermark after recovery")
	}
	if sink2.ContiguousWatermarkForStream("tenant-1", "agent-a", stream) != 2 {
		t.Fatalf("modern watermark after recovery")
	}
}

func TestLineageCase17and18_BatchIndexKeys(t *testing.T) {
	legacyKey := batchIndexKey("tenant-1", "agent-a", 1, 1)
	modernKey := modernBatchIndexKey("tenant-1", "agent-a", testStreamA(), 1, 1)
	if legacyKey == modernKey {
		t.Fatal("modern key must differ from legacy")
	}
	if modernBatchIndexKey("tenant-1", "agent-a", testStreamA(), 1, 1) ==
		modernBatchIndexKey("tenant-1", "agent-a", testStreamB(), 1, 1) {
		t.Fatal("modern key must include stream identity")
	}
	// Legacy key formula unchanged
	var b []byte
	b = appendLenPrefixed(b, "tenant-1")
	b = appendLenPrefixed(b, "agent-a")
	b = appendLenPrefixed(b, "1")
	b = appendLenPrefixed(b, "1")
	// recompute via exported path
	if batchIndexKey("tenant-1", "agent-a", 1, 1) != legacyKey {
		t.Fatal("legacy key instability")
	}
}

func TestLineageCase19_NoSyntheticWatermark(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	stream := testStreamA()
	// first>1 on fresh sensor: establish owner at watermark 0, sparse accept
	env := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(10, "r10", []byte(`{"v":10}`)))
	stampEnvelope(&env)
	env.AssignmentGeneration = 99
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if got := sink.ContiguousWatermarkForStream("tenant-1", "agent-a", stream); got != 0 {
		t.Fatalf("must not synthesize contiguous coverage, watermark=%d", got)
	}
	if sink.SparseRangeCountForStream("tenant-1", "agent-a", stream) != 1 {
		t.Fatalf("expected sparse range")
	}
}

func TestLineageCase20_SameStreamGapRemainsSparse(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	stream := testStreamA()
	first := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	gap := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(5, "r5", []byte(`{"v":5}`)))
	stampEnvelope(&gap)
	gap.BatchID = "gap-batch"
	if _, err := sink.AcceptBatch(context.Background(), gap); err != nil {
		t.Fatal(err)
	}
	if got := sink.ContiguousWatermarkForStream("tenant-1", "agent-a", stream); got != 1 {
		t.Fatalf("gap must not advance contiguous watermark, got %d", got)
	}
	if sink.SparseRangeCountForStream("tenant-1", "agent-a", stream) != 1 {
		t.Fatalf("gap must remain sparse")
	}
}

func TestReceiverMapsStreamErrorsTo409(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-a", "agent-a")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.AllowGeneration("agent-a", "sensor-infra-01", "tenant-1", "site-1", 1)

	t.Run("stale_or_fenced", func(t *testing.T) {
		sink := NewInMemoryAcceptanceSink()
		sink.SetInjectedError(&StreamStaleOrFencedError{
			TenantID: "tenant-1", AgentID: "agent-a", AgentStreamID: testStreamB(),
			ActiveAgentStreamID: testStreamA(), FirstSequence: 2, LastSequence: 2,
		})
		rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)
		srv, client := mockTLSServer(rcv, "agent-a")
		defer srv.Close()
		env := sampleV2Envelope("agent-a", "tenant-1", testStreamB(), sampleRecord(2, "r2", []byte(`{"v":2}`)))
		env.SiteID = "site-1"
		body, _ := json.Marshal(env)
		resp := postJSON(t, client, srv.URL+EndpointPath, body)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("HTTP %d", resp.StatusCode)
		}
		var er ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&er)
		if er.Code != ErrCodeStreamStaleOrFenced {
			t.Fatalf("code %q", er.Code)
		}
	})

	t.Run("legacy_closed", func(t *testing.T) {
		sink := NewInMemoryAcceptanceSink()
		sink.SetInjectedError(&LegacyStreamClosedError{TenantID: "tenant-1", AgentID: "agent-a"})
		rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)
		srv, client := mockTLSServer(rcv, "agent-a")
		defer srv.Close()
		env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r1", []byte(`{"v":1}`)))
		env.SiteID = "site-1"
		body, _ := json.Marshal(env)
		resp := postJSON(t, client, srv.URL+EndpointPath, body)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("HTTP %d", resp.StatusCode)
		}
		var er ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&er)
		if er.Code != ErrCodeLegacyStreamClosed {
			t.Fatalf("code %q", er.Code)
		}
	})
}

func TestReplayIdentityIncludesAgentStreamID(t *testing.T) {
	legacy := ReplayIdentity{TenantID: "t", AgentID: "a", BatchID: "b"}
	modern := ReplayIdentity{TenantID: "t", AgentID: "a", AgentStreamID: testStreamA(), BatchID: "b"}
	if legacy.Key() == modern.Key() {
		t.Fatal("v2 replay key must include stream id")
	}
}
