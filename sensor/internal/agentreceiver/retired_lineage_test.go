package agentreceiver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"platform/sensor/internal/spool"
)

func testStreamC() string {
	return "stream-" + strings.Repeat("c", 32)
}

func reopenTestSink(t *testing.T, dir string) (*spool.DiskSpool, *SpoolAcceptanceSink) {
	t.Helper()
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := NewSpoolAcceptanceSink(s, dir)
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	return s, sink
}

func containRetired(t *testing.T, sink *SpoolAcceptanceSink, tenantID, agentID string, want ...string) {
	t.Helper()
	got := sink.RetiredStreamIDsForTest(tenantID, agentID)
	set := make(map[string]bool, len(got))
	for _, id := range got {
		set[id] = true
	}
	for _, id := range want {
		if !set[id] {
			t.Fatalf("retired set missing %s; got %#v", id, got)
		}
	}
}

func assertFencedNoMutation(t *testing.T, s *spool.DiskSpool, sink *SpoolAcceptanceSink, env DeliveryEnvelope, ownerWant string, aWMWant, bWMWant uint64, aStream, bStream string) {
	t.Helper()
	statsBefore, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	tombBefore := sink.HasLegacyTombstone(env.TenantID, env.AgentID)
	if _, err := sink.AcceptBatch(context.Background(), env); !IsStreamStaleOrFenced(err) {
		t.Fatalf("expected STALE_OR_FENCED, got %v", err)
	}
	statsAfter, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if statsAfter.RecordCount != statsBefore.RecordCount {
		t.Fatal("fenced request must not append spool")
	}
	if sink.ActiveOwnerStreamID(env.TenantID, env.AgentID) != ownerWant {
		t.Fatalf("owner must remain %s, got %s", ownerWant, sink.ActiveOwnerStreamID(env.TenantID, env.AgentID))
	}
	if aStream != "" && sink.ContiguousWatermarkForStream(env.TenantID, env.AgentID, aStream) != aWMWant {
		t.Fatalf("A watermark changed: got %d want %d", sink.ContiguousWatermarkForStream(env.TenantID, env.AgentID, aStream), aWMWant)
	}
	if bStream != "" && sink.ContiguousWatermarkForStream(env.TenantID, env.AgentID, bStream) != bWMWant {
		t.Fatalf("B watermark changed: got %d want %d", sink.ContiguousWatermarkForStream(env.TenantID, env.AgentID, bStream), bWMWant)
	}
	if sink.HasLegacyTombstone(env.TenantID, env.AgentID) != tombBefore {
		t.Fatal("tombstone must not mutate on fence")
	}
}

// Mandatory Test 1 — A→B then stale A seq1
func TestRetiredLineage_AThenBThenStaleASeq1(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	a, b := testStreamA(), testStreamB()
	first := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	aWM := sink.ContiguousWatermarkForStream("tenant-1", "agent-a", a)

	replace := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "r1b", []byte(`{"v":2}`)))
	stampEnvelope(&replace)
	if _, err := sink.AcceptBatch(context.Background(), replace); err != nil {
		t.Fatal(err)
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != b {
		t.Fatal("B must be active")
	}
	containRetired(t, sink, "tenant-1", "agent-a", a)
	bWM := sink.ContiguousWatermarkForStream("tenant-1", "agent-a", b)

	stale := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&stale)
	assertFencedNoMutation(t, s, sink, stale, b, aWM, bWM, a, b)
}

// Mandatory Test 2 — A→B→C then stale A and B
func TestRetiredLineage_ABCThenStaleAAndB(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	a, b, c := testStreamA(), testStreamB(), testStreamC()
	for i, stream := range []string{a, b, c} {
		env := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(1, "r1", []byte(`{"n":1}`)))
		env.BatchID = "batch-abc-" + string(rune('a'+i))
		stampEnvelope(&env)
		if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
			t.Fatal(err)
		}
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != c {
		t.Fatal("C must be active")
	}
	containRetired(t, sink, "tenant-1", "agent-a", a, b)

	statsBefore, _ := s.Stats()
	for _, stream := range []string{a, b} {
		stale := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(1, "rx", []byte(`{"v":9}`)))
		stampEnvelope(&stale)
		if _, err := sink.AcceptBatch(context.Background(), stale); !IsStreamStaleOrFenced(err) {
			t.Fatalf("expected fence for %s: %v", stream, err)
		}
	}
	statsAfter, _ := s.Stats()
	if statsAfter.RecordCount != statsBefore.RecordCount {
		t.Fatal("stale A/B must not mutate spool")
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != c {
		t.Fatal("owner must remain C")
	}
}

// Mandatory Test 3 — exact historical batch-1 replay after retirement
func TestRetiredLineage_ExactHistoricalBatch1ReplayFenced(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	a, b := testStreamA(), testStreamB()
	historical := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "hist-1", []byte(`{"exact":true}`)))
	stampEnvelope(&historical)
	historical.BatchID = "batch-historical-exact-1"
	if _, err := sink.AcceptBatch(context.Background(), historical); err != nil {
		t.Fatal(err)
	}

	replace := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "r1b", []byte(`{"v":2}`)))
	stampEnvelope(&replace)
	if _, err := sink.AcceptBatch(context.Background(), replace); err != nil {
		t.Fatal(err)
	}

	replay := historical // exact stream/batch/seq/payload
	statsBefore, _ := s.Stats()
	if _, err := sink.AcceptBatch(context.Background(), replay); !IsStreamStaleOrFenced(err) {
		t.Fatalf("exact historical batch1 must be fenced, got %v", err)
	}
	statsAfter, _ := s.Stats()
	if statsAfter.RecordCount != statsBefore.RecordCount {
		t.Fatal("replay must not append")
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != b {
		t.Fatal("owner must remain B")
	}
}

// Mandatory Test 4 — unknown fresh B seq1 may replace A
func TestRetiredLineage_UnknownFreshBSeq1ReplacesA(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	a, b := testStreamA(), testStreamB()
	first := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	fresh := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "fresh", []byte(`{"v":2}`)))
	stampEnvelope(&fresh)
	if _, err := sink.AcceptBatch(context.Background(), fresh); err != nil {
		t.Fatal(err)
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != b {
		t.Fatal("B must become owner")
	}
	containRetired(t, sink, "tenant-1", "agent-a", a)
}

// Mandatory Test 5 — retired stream non-1 sequences remain fenced
func TestRetiredLineage_RetiredNonOneSequencesFenced(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	a, b := testStreamA(), testStreamB()
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
	for _, seq := range []uint64{2, 100} {
		stale := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(seq, "rx", []byte(`{"v":9}`)))
		stampEnvelope(&stale)
		if _, err := sink.AcceptBatch(context.Background(), stale); !IsStreamStaleOrFenced(err) {
			t.Fatalf("seq %d must be fenced: %v", seq, err)
		}
	}
}

// Mandatory Test 6 — owner publication failure then retry
func TestRetiredLineage_OwnerPublishFailureThenRetry(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	a, b := testStreamA(), testStreamB()
	first := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	sink.SetOwnerPublisherForTest(func(dir, path, tmpPattern string, v any) error {
		return errors.New("injected owner publish failure")
	})
	fail := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "r1b", []byte(`{"v":2}`)))
	stampEnvelope(&fail)
	if _, err := sink.AcceptBatch(context.Background(), fail); err == nil {
		t.Fatal("expected owner publish failure")
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != a {
		t.Fatal("A must remain active after failed owner publish")
	}
	if !sink.ReconcileRequiredForTest() {
		t.Fatal("reconcileRequired must be set after owner publish failure")
	}

	sink.SetOwnerPublisherForTest(nil)
	retry := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "r1b", []byte(`{"v":2}`)))
	stampEnvelope(&retry)
	if _, err := sink.AcceptBatch(context.Background(), retry); err != nil {
		t.Fatal(err)
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != b {
		t.Fatal("retry must transition to B")
	}
	containRetired(t, sink, "tenant-1", "agent-a", a)
}

// Mandatory Test 7 — orphan B then C retires A and B
func TestRetiredLineage_OrphanBThenCRetiresBoth(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	a, b, c := testStreamA(), testStreamB(), testStreamC()
	first := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	sink.SetOwnerPublisherForTest(func(dir, path, tmpPattern string, v any) error {
		return errors.New("injected owner publish failure for B")
	})
	orphanB := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "rb", []byte(`{"v":2}`)))
	stampEnvelope(&orphanB)
	if _, err := sink.AcceptBatch(context.Background(), orphanB); err == nil {
		t.Fatal("expected owner-B failure")
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != a {
		t.Fatal("A remains authoritative")
	}
	sink.SetOwnerPublisherForTest(nil)

	ownC := sampleV2Envelope("agent-a", "tenant-1", c, sampleRecord(1, "rc", []byte(`{"v":3}`)))
	stampEnvelope(&ownC)
	if _, err := sink.AcceptBatch(context.Background(), ownC); err != nil {
		t.Fatal(err)
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != c {
		t.Fatal("C must be owner")
	}
	containRetired(t, sink, "tenant-1", "agent-a", a, b)

	staleB := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "rb", []byte(`{"v":2}`)))
	stampEnvelope(&staleB)
	if _, err := sink.AcceptBatch(context.Background(), staleB); !IsStreamStaleOrFenced(err) {
		t.Fatalf("orphan B seq1 must be fenced after C: %v", err)
	}
}

// Mandatory Test 8 — restart after failed transition
func TestRetiredLineage_RestartAfterFailedTransition(t *testing.T) {
	s, sink, dir := openTestSink(t)

	a, b := testStreamA(), testStreamB()
	first := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	sink.SetOwnerPublisherForTest(func(dir, path, tmpPattern string, v any) error {
		return errors.New("injected owner publish failure")
	})
	fail := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "r1b", []byte(`{"v":2}`)))
	stampEnvelope(&fail)
	if _, err := sink.AcceptBatch(context.Background(), fail); err == nil {
		t.Fatal("expected failure")
	}
	s.Close()

	s2, sink2 := reopenTestSink(t, dir)
	defer s2.Close()
	if sink2.ActiveOwnerStreamID("tenant-1", "agent-a") != a {
		t.Fatal("disk owner A must remain authoritative after restart")
	}
	retry := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "r1b", []byte(`{"v":2}`)))
	stampEnvelope(&retry)
	if _, err := sink2.AcceptBatch(context.Background(), retry); err != nil {
		t.Fatal(err)
	}
	if sink2.ActiveOwnerStreamID("tenant-1", "agent-a") != b {
		t.Fatal("B retry must succeed after restart")
	}
	containRetired(t, sink2, "tenant-1", "agent-a", a)
}

// Mandatory Test 9 — restart after successful A→B
func TestRetiredLineage_RestartAfterSuccessfulTransition(t *testing.T) {
	s, sink, dir := openTestSink(t)

	a, b := testStreamA(), testStreamB()
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
	s.Close()

	s2, sink2 := reopenTestSink(t, dir)
	defer s2.Close()
	if sink2.ActiveOwnerStreamID("tenant-1", "agent-a") != b {
		t.Fatal("owner B must survive restart")
	}
	containRetired(t, sink2, "tenant-1", "agent-a", a)
	stale := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&stale)
	if _, err := sink2.AcceptBatch(context.Background(), stale); !IsStreamStaleOrFenced(err) {
		t.Fatalf("A seq1 must remain fenced after restart: %v", err)
	}
}

// Mandatory Test 10 — multiple restart history A→B→C
func TestRetiredLineage_MultipleRestartHistory(t *testing.T) {
	s, sink, dir := openTestSink(t)

	a, b, c := testStreamA(), testStreamB(), testStreamC()
	for _, stream := range []string{a, b, c} {
		env := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(1, "r1", []byte(`{"v":1}`)))
		stampEnvelope(&env)
		if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	s2, sink2 := reopenTestSink(t, dir)
	defer s2.Close()
	if sink2.ActiveOwnerStreamID("tenant-1", "agent-a") != c {
		t.Fatal("C must remain active")
	}
	containRetired(t, sink2, "tenant-1", "agent-a", a, b)
	for _, stream := range []string{a, b} {
		stale := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(1, "rx", []byte(`{"v":9}`)))
		stampEnvelope(&stale)
		if _, err := sink2.AcceptBatch(context.Background(), stale); !IsStreamStaleOrFenced(err) {
			t.Fatalf("%s seq1 must be fenced after restart", stream)
		}
	}
}

// Mandatory Test 11 — retired list / owner corruption fail-closed
func TestRetiredLineage_OwnerCorruptionFailClosed(t *testing.T) {
	cases := []struct {
		name  string
		mutate func(owner *LineageOwnerRecord)
	}{
		{"duplicate retired", func(o *LineageOwnerRecord) {
			o.RetiredAgentStreamIDs = []string{testStreamB(), testStreamB()}
		}},
		{"active in retired", func(o *LineageOwnerRecord) {
			o.RetiredAgentStreamIDs = []string{o.ActiveAgentStreamID}
		}},
		{"invalid grammar", func(o *LineageOwnerRecord) {
			o.RetiredAgentStreamIDs = []string{"stream-NOTHEX"}
		}},
		{"over capacity", func(o *LineageOwnerRecord) {
			ids := make([]string, maxRetiredAgentStreamIDs+1)
			for i := range ids {
				ids[i] = "stream-" + strings.Repeat(string(rune('0'+(i%10))), 32)
				// ensure unique via embedding index hex-ish
				ids[i] = uniqueTestStreamID(i)
			}
			o.RetiredAgentStreamIDs = ids
		}},
		{"unknown field", nil}, // handled separately
		{"bad owner hash", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, sink, dir := openTestSink(t)
			a := testStreamA()
			env := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
			stampEnvelope(&env)
			if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
				t.Fatal(err)
			}
			s.Close()

			ownerPath := filepath.Join(dir, streamStateDirName, lineageOwnerDirName, sourceRangeStreamKey("tenant-1", "agent-a")+".json")
			data, err := os.ReadFile(ownerPath)
			if err != nil {
				t.Fatal(err)
			}

			switch tc.name {
			case "unknown field":
				var raw map[string]any
				if err := json.Unmarshal(data, &raw); err != nil {
					t.Fatal(err)
				}
				raw["unexpectedField"] = true
				bad, _ := json.Marshal(raw)
				if err := os.WriteFile(ownerPath, bad, 0640); err != nil {
					t.Fatal(err)
				}
			case "bad owner hash":
				wrong := filepath.Join(filepath.Dir(ownerPath), sourceRangeStreamKey("tenant-other", "agent-a")+".json")
				if err := os.Rename(ownerPath, wrong); err != nil {
					t.Fatal(err)
				}
			default:
				var owner LineageOwnerRecord
				if err := json.Unmarshal(data, &owner); err != nil {
					t.Fatal(err)
				}
				tc.mutate(&owner)
				bad, err := json.Marshal(owner)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(ownerPath, bad, 0640); err != nil {
					t.Fatal(err)
				}
			}

			sp, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
			if err != nil {
				t.Fatal(err)
			}
			defer sp.Close()
			if _, err := NewSpoolAcceptanceSink(sp, dir); err == nil {
				t.Fatal("corrupt owner must fail closed on init")
			}
		})
	}
}

func uniqueTestStreamID(i int) string {
	// 32 hex chars derived from index
	const hex = "0123456789abcdef"
	var b strings.Builder
	b.WriteString("stream-")
	for n := 0; n < 32; n++ {
		b.WriteByte(hex[(i+n*17)%16])
	}
	// Force uniqueness across first 1025 by mixing i into last 8 nibbles
	s := b.String()
	suffix := make([]byte, 8)
	v := i
	for n := 7; n >= 0; n-- {
		suffix[n] = hex[v%16]
		v /= 16
	}
	return s[:len(s)-8] + string(suffix)
}

// Mandatory Test 12 — tenant isolation of retired fencing
func TestRetiredLineage_TenantIsolation(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	stream := testStreamA()
	alt := testStreamB()

	ta := sampleV2Envelope("agent-x", "tenant-a", stream, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&ta)
	if _, err := sink.AcceptBatch(context.Background(), ta); err != nil {
		t.Fatal(err)
	}
	replaceA := sampleV2Envelope("agent-x", "tenant-a", alt, sampleRecord(1, "r2", []byte(`{"v":2}`)))
	stampEnvelope(&replaceA)
	if _, err := sink.AcceptBatch(context.Background(), replaceA); err != nil {
		t.Fatal(err)
	}
	containRetired(t, sink, "tenant-a", "agent-x", stream)

	// Same stream text under tenant-b must still be writable (not fenced by tenant-a retirement).
	tb := sampleV2Envelope("agent-x", "tenant-b", stream, sampleRecord(1, "r1b", []byte(`{"v":1}`)))
	stampEnvelope(&tb)
	if _, err := sink.AcceptBatch(context.Background(), tb); err != nil {
		t.Fatalf("tenant-b must not inherit tenant-a retired fence: %v", err)
	}
	if sink.ActiveOwnerStreamID("tenant-b", "agent-x") != stream {
		t.Fatal("tenant-b owner must be stream")
	}
}

// Mandatory Test 13 — rename/dirsync uncertainty forces reconcile
func TestRetiredLineage_RenameDirsyncUncertaintyReconcile(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()

	a, b := testStreamA(), testStreamB()
	first := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	// Simulate: durable owner file is published (rename succeeded) but caller sees error (dirsync fail).
	sink.SetOwnerPublisherForTest(func(dir, path, tmpPattern string, v any) error {
		if err := atomicWriteJSONFile(dir, path, tmpPattern, v); err != nil {
			return err
		}
		return errors.New("injected dirsync failure after rename")
	})
	uncertain := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "r1b", []byte(`{"v":2}`)))
	stampEnvelope(&uncertain)
	if _, err := sink.AcceptBatch(context.Background(), uncertain); err == nil {
		t.Fatal("expected publication uncertainty error")
	}
	if !sink.ReconcileRequiredForTest() {
		t.Fatal("reconcileRequired must be true after uncertain owner publish")
	}
	// Stale in-memory owner must not remain authoritative without reconcile.
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") == b {
		t.Fatal("memory must not claim B until publish returns success")
	}

	sink.SetOwnerPublisherForTest(nil)
	// Next ingress must reconcile disk (owner B already published) before accepting.
	staleA := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&staleA)
	if _, err := sink.AcceptBatch(context.Background(), staleA); !IsStreamStaleOrFenced(err) {
		t.Fatalf("next ingress must reconcile to B then fence A: %v", err)
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != b {
		t.Fatal("reconcile must load durable owner B")
	}
	containRetired(t, sink, "tenant-1", "agent-a", a)

	// Active B remains writable after reconcile.
	nextB := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "r1b", []byte(`{"v":2}`)))
	stampEnvelope(&nextB)
	if _, err := sink.AcceptBatch(context.Background(), nextB); err != nil {
		t.Fatalf("B seq1 retry after reconcile must succeed: %v", err)
	}
}

func TestBuildRetiredAgentStreamIDs_CapacityFailClosed(t *testing.T) {
	previous := &LineageOwnerRecord{
		ActiveAgentStreamID:   testStreamA(),
		RetiredAgentStreamIDs: make([]string, maxRetiredAgentStreamIDs),
	}
	for i := 0; i < maxRetiredAgentStreamIDs; i++ {
		previous.RetiredAgentStreamIDs[i] = uniqueTestStreamID(i)
	}
	_, err := buildRetiredAgentStreamIDs(previous, "tenant-1", "agent-a", testStreamB(), nil)
	if err == nil {
		t.Fatal("must fail closed when retiring would exceed bound")
	}
}

func TestValidateRetiredAgentStreamIDs_ActiveExcluded(t *testing.T) {
	err := validateRetiredAgentStreamIDs(testStreamA(), []string{testStreamA()})
	if err == nil {
		t.Fatal("active in retired must fail")
	}
}

func TestRetiredLineage_OwnerRecordRoundTripFields(t *testing.T) {
	s, sink, dir := openTestSink(t)
	a, b := testStreamA(), testStreamB()
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
	s.Close()

	data, err := os.ReadFile(filepath.Join(dir, streamStateDirName, lineageOwnerDirName, sourceRangeStreamKey("tenant-1", "agent-a")+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["retiredAgentStreamIds"]; !ok {
		t.Fatal("owner JSON must include retiredAgentStreamIds")
	}
	if raw["activeAgentStreamId"] != b {
		t.Fatal("activeAgentStreamId mismatch")
	}
	if raw["previousAgentStreamId"] != a {
		t.Fatal("previousAgentStreamId provenance expected")
	}
}
