package spool

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestOutboundWindowFrozenAcrossGrowthAndRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Config{Dir: dir, MaxRecordBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	s.space = func(int64) (spaceStatus, error) { return spaceStatus{1000000, 1000, .1}, nil }

	for i := 1; i <= 5; i++ {
		add(t, s, fmt.Sprintf("p%d", i))
	}
	first, err := s.ReadOrReserveOutboundBatch(5, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if first.FirstSequence != 1 || first.LastSequence != 5 || len(first.Records) != 5 {
		t.Fatalf("first window=%#v", first)
	}
	sum1 := checksumRecords(first.Records)

	for i := 6; i <= 10; i++ {
		add(t, s, fmt.Sprintf("p%d", i))
	}
	second, err := s.ReadOrReserveOutboundBatch(10, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if second.FirstSequence != 1 || second.LastSequence != 5 || len(second.Records) != 5 {
		t.Fatalf("growth must not expand frozen window: %#v", second)
	}
	if checksumRecords(second.Records) != sum1 {
		t.Fatal("reconstructed frozen window checksum must match first reservation")
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(Config{Dir: dir, MaxRecordBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	s2.space = s.space
	third, err := s2.ReadOrReserveOutboundBatch(10, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if third.FirstSequence != 1 || third.LastSequence != 5 {
		t.Fatalf("restart must preserve frozen window: %#v", third)
	}
	if checksumRecords(third.Records) != sum1 {
		t.Fatal("restart reconstruction checksum drift")
	}

	if err := s2.CompleteOutboundBatch(5); err != nil {
		t.Fatal(err)
	}
	next, err := s2.ReadOrReserveOutboundBatch(10, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if next.FirstSequence != 6 || next.LastSequence != 10 {
		t.Fatalf("after ACK next window must start at 6: %#v", next)
	}
}

func TestOutboundWindowCorruptSidecarFailsClosed(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "a")
	if _, err := s.ReadOrReserveOutboundBatch(1, 1024); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.dir, outboundWindowFileName)
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1`), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadOrReserveOutboundBatch(1, 1024); CodeOf(err) != CodeOutboundWindow {
		t.Fatalf("corrupt sidecar must fail closed, got %v", err)
	}
}

func TestOutboundWindowUnknownFieldFailsClosed(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "a")
	if _, err := s.ReadOrReserveOutboundBatch(1, 1024); err != nil {
		t.Fatal(err)
	}
	raw := fmt.Sprintf(
		`{"schemaVersion":1,"streamId":%q,"firstSequence":1,"lastSequence":1,"extra":true}`,
		s.StreamID(),
	)
	if err := os.WriteFile(filepath.Join(s.dir, outboundWindowFileName), []byte(raw), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadOrReserveOutboundBatch(1, 1024); CodeOf(err) != CodeOutboundWindow {
		t.Fatalf("unknown field must fail closed, got %v", err)
	}
}

func TestOutboundWindowTrailingJSONFailsClosed(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "a")
	if _, err := s.ReadOrReserveOutboundBatch(1, 1024); err != nil {
		t.Fatal(err)
	}
	base, err := json.Marshal(outboundWindow{
		SchemaVersion: outboundWindowSchema,
		StreamID:      s.StreamID(),
		FirstSequence: 1,
		LastSequence:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.dir, outboundWindowFileName)
	if err := os.WriteFile(path, append(base, []byte(`{"x":1}`)...), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadOrReserveOutboundBatch(1, 1024); CodeOf(err) != CodeOutboundWindow {
		t.Fatalf("trailing JSON value must fail closed, got %v", err)
	}
	// Trailing whitespace alone remains valid.
	if err := os.WriteFile(path, append(base, []byte("  \n\t")...), 0640); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadOrReserveOutboundBatch(1, 1024)
	if err != nil || got.FirstSequence != 1 || got.LastSequence != 1 {
		t.Fatalf("trailing whitespace must remain valid: %#v err=%v", got, err)
	}
}

func TestOutboundWindowStreamMismatchFailsClosed(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "a")
	if _, err := s.ReadOrReserveOutboundBatch(1, 1024); err != nil {
		t.Fatal(err)
	}
	win := outboundWindow{
		SchemaVersion: outboundWindowSchema,
		StreamID:      "stream-ffffffffffffffffffffffffffffffff",
		FirstSequence: 1,
		LastSequence:  1,
	}
	raw, _ := json.Marshal(win)
	if err := os.WriteFile(filepath.Join(s.dir, outboundWindowFileName), raw, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadOrReserveOutboundBatch(1, 1024); CodeOf(err) != CodeOutboundWindow {
		t.Fatalf("stream mismatch must fail closed, got %v", err)
	}
}

func TestOutboundWindowStaleAfterAckSelfReconciles(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "a")
	add(t, s, "b")
	if _, err := s.ReadOrReserveOutboundBatch(1, 1024); err != nil {
		t.Fatal(err)
	}
	// Simulate crash after durable ACK of seq 1 but before sidecar clear.
	if err := s.AcknowledgeThrough(1); err != nil {
		t.Fatal(err)
	}
	next, err := s.ReadOrReserveOutboundBatch(10, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if next.FirstSequence != 2 || next.LastSequence != 2 {
		t.Fatalf("stale fully-ACKed sidecar must reconcile; got %#v", next)
	}
}

// TestReadOrReserveOutboundBatchRestartSidecarClearFailureLatchesUnresolved
// reproduces restart after a post-commit sidecar unlink failure: Open recover
// clears the in-memory unresolved flag, but a continuing clear failure on the
// next ReadOrReserveOutboundBatch must re-latch cleanupUnresolved + degraded
// without inventing ACK progress or touching unacked records / active windows.
func TestReadOrReserveOutboundBatchRestartSidecarClearFailureLatchesUnresolved(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Config{Dir: dir, MaxRecordBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	s.space = func(int64) (spaceStatus, error) { return spaceStatus{1000000, 1000, .1}, nil }
	add(t, s, "acked-body")
	add(t, s, "pending-body")
	batch, err := s.ReadOrReserveOutboundBatch(1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if batch.FirstSequence != 1 || batch.LastSequence != 1 {
		t.Fatalf("frozen window=%#v", batch)
	}
	sidecar := filepath.Join(dir, outboundWindowFileName)
	s.removeOutboundWindow = func(string) error {
		return errors.New("injected outbound window sidecar clear failure")
	}
	if err := s.CompleteOutboundBatch(batch.LastSequence); err == nil {
		t.Fatal("expected CompleteOutboundBatch sidecar clear failure")
	}
	unresolved, seq, err := s.UnresolvedAckedCleanup()
	if err != nil || !unresolved || seq != batch.LastSequence {
		t.Fatalf("pre-restart latch missing: unresolved=%v seq=%d err=%v", unresolved, seq, err)
	}
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("stale sidecar must survive crash window: %v", err)
	}
	pendingPath := filepath.Join(dir, recordFileName(2))
	if _, err := os.Stat(pendingPath); err != nil {
		t.Fatalf("unacked record must survive: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(Config{Dir: dir, MaxRecordBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	reopened.space = func(int64) (spaceStatus, error) { return spaceStatus{1000000, 1000, .1}, nil }
	// recover() clears the in-memory flag; continuing sidecar unlink failure
	// after restart must surface storage degradation again.
	unresolved, _, err = reopened.UnresolvedAckedCleanup()
	if err != nil || unresolved {
		t.Fatalf("Open recover clears in-memory unresolved; got unresolved=%v err=%v", unresolved, err)
	}
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("stale fully-acked sidecar must remain after restart: %v", err)
	}
	reopened.removeOutboundWindow = func(string) error {
		return errors.New("injected outbound window sidecar clear failure after restart")
	}
	if _, err := reopened.ReadOrReserveOutboundBatch(10, 1024); err == nil {
		t.Fatal("restart sidecar clear failure must surface")
	}
	unresolved, seq, err = reopened.UnresolvedAckedCleanup()
	if err != nil || !unresolved || seq < 1 {
		t.Fatalf("restart clear failure must latch unresolved: unresolved=%v seq=%d err=%v", unresolved, seq, err)
	}
	st, err := reopened.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.ProtectionState != ProtectionDegraded {
		t.Fatalf("continuing unlink failure must degrade storage: %+v", st)
	}
	if st.AckedThrough < 1 || st.RecordCount != 1 {
		t.Fatalf("watermark and unacked identity must hold: %+v", st)
	}
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("failed restart clear must leave sidecar intact: %v", err)
	}
	if _, err := os.Stat(pendingPath); err != nil {
		t.Fatalf("unacked record must remain after failed restart clear: %v", err)
	}

	reopened.removeOutboundWindow = nil
	if err := reopened.ReconcileAckedCleanup(); err != nil {
		t.Fatalf("reconcile must clear stale sidecar after verified unlink+sync: %v", err)
	}
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Fatalf("stale sidecar must be removed after reconcile, err=%v", err)
	}
	unresolved, _, err = reopened.UnresolvedAckedCleanup()
	if err != nil || unresolved {
		t.Fatalf("verified cleanup must clear unresolved: unresolved=%v err=%v", unresolved, err)
	}
	next, err := reopened.ReadOrReserveOutboundBatch(10, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if next.FirstSequence != 2 || next.LastSequence != 2 || len(next.Records) != 1 {
		t.Fatalf("pending window identity after cleanup: %#v", next)
	}
}

// TestCompleteOutboundBatchSidecarClearFailureLatchesUnresolved proves that
// after durable ACK, outbound-window sidecar unlink failure latches
// cleanupUnresolved and ReconcileAckedCleanup repairs the stale sidecar.
func TestCompleteOutboundBatchSidecarClearFailureLatchesUnresolved(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "body")
	batch, err := s.ReadOrReserveOutboundBatch(1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	sidecar := filepath.Join(s.dir, outboundWindowFileName)
	s.removeOutboundWindow = func(string) error {
		return errors.New("injected outbound window sidecar clear failure")
	}
	if err := s.CompleteOutboundBatch(batch.LastSequence); err == nil {
		t.Fatal("expected CompleteOutboundBatch sidecar clear failure")
	}
	acked, err := s.AckedThrough()
	if err != nil || acked != batch.LastSequence {
		t.Fatalf("watermark must be durable after sidecar clear failure: acked=%d err=%v", acked, err)
	}
	unresolved, seq, err := s.UnresolvedAckedCleanup()
	if err != nil || !unresolved || seq != batch.LastSequence {
		t.Fatalf("sidecar clear failure must latch unresolved: unresolved=%v seq=%d err=%v", unresolved, seq, err)
	}
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("failed sidecar clear must leave sidecar intact: %v", err)
	}
	s.removeOutboundWindow = nil
	if err := s.ReconcileAckedCleanup(); err != nil {
		t.Fatalf("reconcile must clear stale sidecar: %v", err)
	}
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Fatalf("stale sidecar must be removed, err=%v", err)
	}
	unresolved, _, err = s.UnresolvedAckedCleanup()
	if err != nil || unresolved {
		t.Fatalf("reconcile must clear unresolved: unresolved=%v err=%v", unresolved, err)
	}
}

func TestReconcileAckedCleanupPreservesActiveOutboundWindow(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "a")
	add(t, s, "b")
	batch, err := s.ReadOrReserveOutboundBatch(1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	s.cleanupUnresolved = true
	if err := s.ReconcileAckedCleanup(); err != nil {
		t.Fatal(err)
	}
	sidecar := filepath.Join(s.dir, outboundWindowFileName)
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("active frozen window must survive reconcile: %v", err)
	}
	next, err := s.ReadOrReserveOutboundBatch(1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if next.FirstSequence != batch.FirstSequence || next.LastSequence != batch.LastSequence {
		t.Fatalf("frozen window identity changed: got %#v want %#v", next, batch)
	}
}

func TestOutboundWindowRejectsLargerReplacement(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "a")
	add(t, s, "b")
	if _, err := s.ReadOrReserveOutboundBatch(1, 1024); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.persistOutboundWindowLocked(outboundWindow{
		SchemaVersion: outboundWindowSchema,
		StreamID:      s.streamID,
		FirstSequence: 1,
		LastSequence:  2,
	})
	if CodeOf(err) != CodeOutboundWindow {
		t.Fatalf("must refuse enlarging frozen window, got %v", err)
	}
}

func TestOutboundWindowAbandonAllowsNewSelection(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "a")
	add(t, s, "b")
	first, err := s.ReadOrReserveOutboundBatch(2, 1024)
	if err != nil || first.LastSequence != 2 {
		t.Fatalf("%#v %v", first, err)
	}
	if err := s.AbandonOutboundBatch(); err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeThrough(1); err != nil {
		t.Fatal(err)
	}
	next, err := s.ReadOrReserveOutboundBatch(2, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if next.FirstSequence != 2 || next.LastSequence != 2 {
		t.Fatalf("after poison-style abandon+ACK head, new window starts at 2: %#v", next)
	}
}

func checksumRecords(recs []Record) string {
	h := ""
	for _, r := range recs {
		h += r.Checksum + ":"
	}
	return h
}

func TestRebatchOutboundWindowDisabledPreservesIdentity(t *testing.T) {
	s := openTest(t, 1024)
	for i := 0; i < 5; i++ {
		add(t, s, fmt.Sprintf("p%d", i))
	}
	original, err := s.ReadOrReserveOutboundBatch(5, 1024)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.dir, outboundWindowFileName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, last := range []uint64{4, 5} {
		if err := s.RebatchOutboundBatch(1, last); CodeOf(err) != CodeOutboundWindow {
			t.Fatalf("rebatching allowed: %v", err)
		}
		after, err := os.ReadFile(path)
		if err != nil || string(before) != string(after) {
			t.Fatal("sidecar changed")
		}
		got, err := s.ReadOrReserveOutboundBatch(1, 1)
		if err != nil || got.FirstSequence != 1 || got.LastSequence != 5 || checksumRecords(got.Records) != checksumRecords(original.Records) {
			t.Fatal("source identity drift")
		}
	}
	if err := s.CompleteOutboundBatch(2); CodeOf(err) != CodeOutboundWindow {
		t.Fatal("split ACK accepted")
	}
	if err := s.CompleteOutboundBatch(5); err != nil {
		t.Fatal(err)
	}
	// The singleton path must also refuse mutation.
	add(t, s, "singleton")
	one, err := s.ReadOrReserveOutboundBatch(1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebatchOutboundBatch(one.FirstSequence, one.LastSequence); CodeOf(err) != CodeOutboundWindow {
		t.Fatal("singleton mutation allowed")
	}
	blocked, err := s.OutboundBatchSizeRejected()
	if err != nil || blocked {
		t.Fatal("new permanent flag")
	}
	if err := s.CompleteOutboundBatch(one.LastSequence); err != nil {
		t.Fatal(err)
	}
}
