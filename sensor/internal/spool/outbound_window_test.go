package spool

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
// next ReadOrReserveOutboundBatch must re-latch cleanupUnresolved (cleanup-only,
// not confirmed corruption) without inventing ACK progress or touching unacked
// records / active windows.
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
	if st.ProtectionState != ProtectionDegraded || st.CorruptionDetected || reopened.degraded {
		t.Fatalf("continuing sidecar unlink must degrade via cleanup-only latch: %+v degraded=%v", st, reopened.degraded)
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
	if _, err := reopened.CheckStorageUsage(); err != nil {
		t.Fatalf("health must recover after cleanup-only sidecar repair: %v", err)
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
// cleanupUnresolved (cleanup-only) and ReconcileAckedCleanup repairs the stale sidecar.
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
	if s.degraded {
		t.Fatal("stale-sidecar unlink must not set independent degraded")
	}
	st, err := s.Stats()
	if err != nil || st.ProtectionState != ProtectionDegraded || st.CorruptionDetected {
		t.Fatalf("sidecar cleanup fault must degrade without CorruptionDetected: %+v err=%v", st, err)
	}
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("failed sidecar clear must leave sidecar intact: %v", err)
	}

	// Sidecar directory fsync failure after successful unlink is also cleanup-only.
	s.removeOutboundWindow = nil
	s.syncDirFn = func(string) error {
		return newError(CodeIO, "failed to fsync spool directory", errors.New("injected sidecar dir fsync failure"))
	}
	// Restore sidecar so clear path runs unlink then failing fsync.
	writeOutboundSidecar(t, s, batch.FirstSequence, batch.LastSequence)
	if err := s.ReconcileAckedCleanup(); err == nil {
		t.Fatal("expected stale-sidecar directory fsync failure during reconcile")
	}
	if !s.cleanupUnresolved || s.degraded {
		t.Fatalf("sidecar fsync must stay cleanup-only: unresolved=%v degraded=%v", s.cleanupUnresolved, s.degraded)
	}

	s.syncDirFn = nil
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
	if _, err := s.CheckStorageUsage(); err != nil {
		t.Fatalf("health must recover after cleanup-only sidecar repair: %v", err)
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

// TestUnsafeOutboundRangeCapacityBaselineDocumentsMakeslicePanic isolates the
// Issue #351 defect class: deriving []Record capacity from an untrusted
// last-first span equal to MaxSafeSequence panics with makeslice before any
// record I/O. Production readExactRangeLocked must never take this path.
func TestUnsafeOutboundRangeCapacityBaselineDocumentsMakeslicePanic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected makeslice panic from untrusted MaxSafeSequence capacity")
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, "makeslice") && !strings.Contains(msg, "cap out of range") {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	count := int(MaxSafeSequence)
	if count <= 0 || uint64(count) != MaxSafeSequence {
		// 32-bit int cannot hold MaxSafeSequence; the historical overflow guard
		// would reject before make. Force the 64-bit failure mode explicitly.
		panic("makeslice: cap out of range")
	}
	_ = make([]Record, 0, count)
	t.Fatal("make with MaxSafeSequence capacity must not return")
}

func TestReadExactRangeRejectsUntrustedSpanBeforeAllocation(t *testing.T) {
	for _, kind := range []SpoolKind{KindTelemetry, KindAgentIngress} {
		t.Run(string(kind), func(t *testing.T) {
			s := openTestKind(t, 1024, kind)
			add(t, s, "only")
			st := snapshotSpoolDurable(t, s)
			if st.nextSequence != 2 || st.ackedThrough != 0 {
				t.Fatalf("tiny state precondition: %#v", st)
			}
			writeOutboundSidecar(t, s, 1, MaxSafeSequence)
			before := snapshotSpoolDurable(t, s)
			batch, err := s.ReadOrReserveOutboundBatch(10, 1024)
			if CodeOf(err) != CodeOutboundWindow {
				t.Fatalf("want SPOOL_OUTBOUND_WINDOW, got batch=%#v err=%v", batch, err)
			}
			if len(batch.Records) != 0 || batch.FirstSequence != 0 || batch.LastSequence != 0 || batch.PayloadBytes != 0 {
				t.Fatalf("reject must return empty batch, got %#v", batch)
			}
			assertSpoolDurableUnchanged(t, s, before)
		})
	}
}

func TestReadExactRangeRejectsImpossibleAndGappedActiveRanges(t *testing.T) {
	for _, kind := range []SpoolKind{KindTelemetry, KindAgentIngress} {
		t.Run(string(kind), func(t *testing.T) {
			s := openTestKind(t, 1024, kind)
			for i := 0; i < 3; i++ {
				add(t, s, fmt.Sprintf("r%d", i))
			}
			if _, err := s.ReadOrReserveOutboundBatch(2, 1024); err != nil {
				t.Fatal(err)
			}
			cases := []struct {
				name       string
				first, last uint64
			}{
				{name: "inverted", first: 2, last: 1},
				{name: "ahead_of_issued", first: 1, last: 99},
				{name: "gapped_active", first: 2, last: 2},
				{name: "zero_first", first: 0, last: 1},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					writeOutboundSidecar(t, s, tc.first, tc.last)
					before := snapshotSpoolDurable(t, s)
					batch, err := s.ReadOrReserveOutboundBatch(10, 1024)
					if CodeOf(err) != CodeOutboundWindow {
						t.Fatalf("want SPOOL_OUTBOUND_WINDOW, got %#v err=%v", batch, err)
					}
					if len(batch.Records) != 0 {
						t.Fatalf("empty batch required, got %#v", batch)
					}
					assertSpoolDurableUnchanged(t, s, before)
				})
			}
		})
	}
}

func TestReadExactRangeRejectsAlreadyAckedActiveOverlap(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "a")
	add(t, s, "b")
	if _, err := s.ReadOrReserveOutboundBatch(2, 1024); err != nil {
		t.Fatal(err)
	}
	// Advance watermark into the frozen range without clearing the sidecar
	// (crash/partial path). Reconstruction must fail closed; stale full-ACK
	// cleanup remains the LastSequence <= AckedThrough reconcile path.
	if err := s.AcknowledgeThrough(1); err != nil {
		t.Fatal(err)
	}
	before := snapshotSpoolDurable(t, s)
	batch, err := s.ReadOrReserveOutboundBatch(10, 1024)
	if CodeOf(err) != CodeOutboundWindow {
		t.Fatalf("partially ACKed active window must fail closed: %#v err=%v", batch, err)
	}
	if len(batch.Records) != 0 {
		t.Fatalf("empty batch required, got %#v", batch)
	}
	assertSpoolDurableUnchanged(t, s, before)
}

func TestReadExactRangeNormalExactReplayAndMissingRecords(t *testing.T) {
	s := openTest(t, 1024)
	for i := 0; i < 3; i++ {
		add(t, s, fmt.Sprintf("body-%d", i))
	}
	first, err := s.ReadOrReserveOutboundBatch(3, 1024)
	if err != nil || first.FirstSequence != 1 || first.LastSequence != 3 || len(first.Records) != 3 {
		t.Fatalf("reserve: %#v err=%v", first, err)
	}
	sum := checksumRecords(first.Records)
	replay, err := s.ReadOrReserveOutboundBatch(1, 1)
	if err != nil || replay.FirstSequence != 1 || replay.LastSequence != 3 || len(replay.Records) != 3 {
		t.Fatalf("exact replay under tiny caps: %#v err=%v", replay, err)
	}
	if checksumRecords(replay.Records) != sum {
		t.Fatal("replay checksum drift")
	}

	if err := os.Remove(filepath.Join(s.dir, recordFileName(2))); err != nil {
		t.Fatal(err)
	}
	before := snapshotSpoolDurable(t, s)
	batch, err := s.ReadOrReserveOutboundBatch(10, 1024)
	if CodeOf(err) != CodeOutboundWindow {
		t.Fatalf("missing middle must fail closed: %#v err=%v", batch, err)
	}
	assertSpoolDurableUnchanged(t, s, before)

	sEnd := openTest(t, 1024)
	for i := 0; i < 3; i++ {
		add(t, sEnd, fmt.Sprintf("e%d", i))
	}
	if _, err := sEnd.ReadOrReserveOutboundBatch(3, 1024); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(sEnd.dir, recordFileName(3))); err != nil {
		t.Fatal(err)
	}
	beforeEnd := snapshotSpoolDurable(t, sEnd)
	batch, err = sEnd.ReadOrReserveOutboundBatch(10, 1024)
	if CodeOf(err) != CodeOutboundWindow {
		t.Fatalf("missing end must fail closed: %#v err=%v", batch, err)
	}
	assertSpoolDurableUnchanged(t, sEnd, beforeEnd)
}

func TestReadExactRangeStalePostAckCleanupPreserved(t *testing.T) {
	for _, kind := range []SpoolKind{KindTelemetry, KindAgentIngress} {
		t.Run(string(kind), func(t *testing.T) {
			s := openTestKind(t, 1024, kind)
			add(t, s, "a")
			add(t, s, "b")
			if _, err := s.ReadOrReserveOutboundBatch(1, 1024); err != nil {
				t.Fatal(err)
			}
			if err := s.AcknowledgeThrough(1); err != nil {
				t.Fatal(err)
			}
			next, err := s.ReadOrReserveOutboundBatch(10, 1024)
			if err != nil {
				t.Fatal(err)
			}
			if next.FirstSequence != 2 || next.LastSequence != 2 || len(next.Records) != 1 {
				t.Fatalf("stale fully-ACKed sidecar must reconcile; got %#v", next)
			}
		})
	}
}

func TestFrozenReplayAcrossSmallerConfigBothKinds(t *testing.T) {
	for _, kind := range []SpoolKind{KindTelemetry, KindAgentIngress} {
		t.Run(string(kind), func(t *testing.T) {
			s := openTestKind(t, 1024, kind)
			for i := 0; i < 5; i++ {
				add(t, s, fmt.Sprintf("p%d", i))
			}
			original, err := s.ReadOrReserveOutboundBatch(5, 1024)
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.ReadOrReserveOutboundBatch(1, 1)
			if err != nil || got.FirstSequence != 1 || got.LastSequence != 5 || len(got.Records) != 5 {
				t.Fatalf("frozen replay under (1,1) caps: %#v err=%v", got, err)
			}
			if checksumRecords(got.Records) != checksumRecords(original.Records) {
				t.Fatal("frozen identity drift under smaller caps")
			}
		})
	}
}

func openTestKind(t *testing.T, max int, kind SpoolKind) *DiskSpool {
	t.Helper()
	s, err := Open(Config{Dir: t.TempDir(), MaxRecordBytes: max, Kind: kind})
	if err != nil {
		t.Fatal(err)
	}
	s.space = func(extra int64) (spaceStatus, error) {
		return spaceStatus{totalBytes: 1000000, usedBytes: 1000, usedPct: .1}, nil
	}
	return s
}

func writeOutboundSidecar(t *testing.T, s *DiskSpool, first, last uint64) {
	t.Helper()
	raw, err := json.Marshal(outboundWindow{
		SchemaVersion: outboundWindowSchema,
		StreamID:      s.StreamID(),
		FirstSequence: first,
		LastSequence:  last,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.dir, outboundWindowFileName), raw, 0640); err != nil {
		t.Fatal(err)
	}
}

type durableSnap struct {
	ackedThrough  uint64
	nextSequence  uint64
	sidecar       string
	sidecarExists bool
	recordNames   []string
	unresolved    bool
	degraded      bool
}

func snapshotSpoolDurable(t *testing.T, s *DiskSpool) durableSnap {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := durableSnap{
		ackedThrough: s.state.AckedThrough,
		nextSequence: s.state.NextSequence,
		unresolved:   s.cleanupUnresolved,
		degraded:     s.degraded,
	}
	path := filepath.Join(s.dir, outboundWindowFileName)
	raw, err := os.ReadFile(path)
	if err == nil {
		snap.sidecarExists = true
		snap.sidecar = string(raw)
	} else if !os.IsNotExist(err) {
		t.Fatalf("sidecar read: %v", err)
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "record-") {
			snap.recordNames = append(snap.recordNames, e.Name())
		}
	}
	return snap
}

func assertSpoolDurableUnchanged(t *testing.T, s *DiskSpool, before durableSnap) {
	t.Helper()
	after := snapshotSpoolDurable(t, s)
	if after.ackedThrough != before.ackedThrough || after.nextSequence != before.nextSequence {
		t.Fatalf("watermark mutated: before=%#v after=%#v", before, after)
	}
	if after.sidecarExists != before.sidecarExists || after.sidecar != before.sidecar {
		t.Fatalf("sidecar mutated: before=%#v after=%#v", before, after)
	}
	if after.unresolved != before.unresolved || after.degraded != before.degraded {
		t.Fatalf("latches mutated: before=%#v after=%#v", before, after)
	}
	if len(after.recordNames) != len(before.recordNames) {
		t.Fatalf("record files mutated: before=%v after=%v", before.recordNames, after.recordNames)
	}
	for i := range before.recordNames {
		if after.recordNames[i] != before.recordNames[i] {
			t.Fatalf("record files mutated: before=%v after=%v", before.recordNames, after.recordNames)
		}
	}
}
