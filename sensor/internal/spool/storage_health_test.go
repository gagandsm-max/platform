package spool

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckStorageUsageHealthyLowUtilisation(t *testing.T) {
	s := openTest(t, 1024)
	defer s.Close()
	add(t, s, "healthy")

	pct, err := s.CheckStorageUsage()
	if err != nil {
		t.Fatalf("healthy low utilisation must succeed: %v", err)
	}
	if pct != .1 {
		t.Fatalf("usedPct=%v want=0.1", pct)
	}
}

func TestCheckStorageUsageCapacityBoundaryUnchanged(t *testing.T) {
	s := openTest(t, 1024)
	defer s.Close()
	s.space = func(int64) (spaceStatus, error) {
		return spaceStatus{totalBytes: 100, usedBytes: 80, usedPct: 80}, nil
	}

	pct, err := s.CheckStorageUsage()
	if err != nil {
		t.Fatalf("80%% capacity must remain a successful provider reading: %v", err)
	}
	if pct != 80 {
		t.Fatalf("usedPct=%v want=80", pct)
	}
	if _, err := s.Append(AppendRequest{Class: ClassMetric, Type: "m", Payload: []byte("x"), CreatedAt: time.Now()}); CodeOf(err) != CodeProtection {
		t.Fatalf("append must still enforce fixed 80%% protection: %v", err)
	}
}

func TestCheckStorageUsageClosedSpoolNotHealthy(t *testing.T) {
	s := openTest(t, 1024)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	pct, err := s.CheckStorageUsage()
	if CodeOf(err) != CodeClosed {
		t.Fatalf("closed spool must fail closed: pct=%v err=%v", pct, err)
	}
	if pct != .1 {
		t.Fatalf("closed spool should still report last capacity when available: pct=%v", pct)
	}
}

func TestCheckStorageUsagePostAckUnlinkFailure(t *testing.T) {
	s := openTest(t, 1024)
	defer s.Close()
	acked := add(t, s, "acked-body")
	pending := add(t, s, "pending-body")
	orphanPath := filepath.Join(s.dir, recordFileName(acked.Sequence))
	pendingPath := filepath.Join(s.dir, recordFileName(pending.Sequence))
	pendingRaw, err := os.ReadFile(pendingPath)
	if err != nil {
		t.Fatal(err)
	}
	s.removeFile = func(string) error { return errors.New("injected record remove failure") }

	if err := s.AcknowledgeThrough(acked.Sequence); err == nil {
		t.Fatal("expected post-ACK unlink failure")
	}
	ackedThrough, err := s.AckedThrough()
	if err != nil || ackedThrough != acked.Sequence {
		t.Fatalf("durable watermark must remain correct: acked=%d err=%v", ackedThrough, err)
	}
	if _, err := os.Stat(orphanPath); err != nil {
		t.Fatalf("orphan evidence must be preserved: %v", err)
	}
	gotPending, err := os.ReadFile(pendingPath)
	if err != nil || string(gotPending) != string(pendingRaw) {
		t.Fatalf("remaining pending data must stay intact")
	}

	pct, err := s.CheckStorageUsage()
	if CodeOf(err) != CodeIO {
		t.Fatalf("post-ACK cleanup fault must fail storage health: pct=%v err=%v", pct, err)
	}
	if pct != .1 {
		t.Fatalf("capacity must remain truthful under integrity failure: pct=%v", pct)
	}
	if strings.Contains(strings.ToLower(err.Error()), "corrupt") {
		t.Fatalf("unlink cleanup failure must not be labelled confirmed corruption: %v", err)
	}
}

func TestCheckStorageUsageAfterPublicCorruptionQuarantine(t *testing.T) {
	s := openTest(t, 1024)
	defer s.Close()
	r := add(t, s, "payload")
	path := filepath.Join(s.dir, recordFileName(r.Sequence))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	rec.Payload = []byte("evil")
	bad, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bad, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadBatch(10, 100); CodeOf(err) != CodeCorruption {
		t.Fatalf("public read/quarantine path must detect corruption: %v", err)
	}
	entries, err := os.ReadDir(s.quarantineDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("corrupt record must be quarantined: n=%d err=%v", len(entries), err)
	}

	pct, err := s.CheckStorageUsage()
	if err == nil || CodeOf(err) != CodeIO {
		t.Fatalf("storage health must be nonhealthy after confirmed corruption latch: pct=%v err=%v", pct, err)
	}
	if pct != .1 {
		t.Fatalf("capacity must remain truthful: pct=%v", pct)
	}
}

func TestCheckStorageUsageHonoursCleanupUnresolvedIndependently(t *testing.T) {
	s := openTest(t, 1024)
	defer s.Close()
	s.cleanupUnresolved = true
	s.degraded = false

	pct, err := s.CheckStorageUsage()
	if CodeOf(err) != CodeIO {
		t.Fatalf("cleanupUnresolved alone must fail storage health: pct=%v err=%v", pct, err)
	}
	if pct != .1 {
		t.Fatalf("capacity must remain truthful: pct=%v", pct)
	}

	// Proven cleanup clears cleanupUnresolved but preserves the separate degraded latch.
	acked := add(t, s, "acked")
	pending := add(t, s, "pending")
	s.removeFile = func(string) error { return errors.New("injected record remove failure") }
	if err := s.AcknowledgeThrough(acked.Sequence); err == nil {
		t.Fatal("expected remove failure")
	}
	s.removeFile = nil
	if err := s.ReconcileAckedCleanup(); err != nil {
		t.Fatal(err)
	}
	unresolved, _, err := s.UnresolvedAckedCleanup()
	if err != nil || unresolved {
		t.Fatalf("reconcile must clear cleanupUnresolved: unresolved=%v err=%v", unresolved, err)
	}
	if !s.degraded {
		t.Fatal("reconcile must preserve the separate degraded latch")
	}
	if _, err := s.CheckStorageUsage(); CodeOf(err) != CodeIO {
		t.Fatalf("degraded latch alone must keep storage health nonhealthy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, recordFileName(pending.Sequence))); err != nil {
		t.Fatalf("pending record must survive reconcile: %v", err)
	}
}

func TestCheckStorageUsageRepeatedChecksAreNonMutating(t *testing.T) {
	s := openTest(t, 1024)
	defer s.Close()
	acked := add(t, s, "acked")
	pending := add(t, s, "pending")
	s.removeFile = func(string) error { return errors.New("injected record remove failure") }
	if err := s.AcknowledgeThrough(acked.Sequence); err == nil {
		t.Fatal("expected remove failure")
	}
	batch, err := s.ReadOrReserveOutboundBatch(1, 1024)
	if err != nil || len(batch.Records) != 1 || batch.FirstSequence != pending.Sequence {
		t.Fatalf("frozen outbound window setup failed: %+v err=%v", batch, err)
	}

	beforeAcked, err := s.AckedThrough()
	if err != nil {
		t.Fatal(err)
	}
	beforeUnresolved, _, err := s.UnresolvedAckedCleanup()
	if err != nil {
		t.Fatal(err)
	}
	beforePending, err := s.ListPendingFiles(10)
	if err != nil {
		t.Fatal(err)
	}
	beforeDegraded := s.degraded
	beforeCleanup := s.cleanupUnresolved
	beforeQuarantined := s.quarantined
	windowBefore, err := os.ReadFile(filepath.Join(s.dir, outboundWindowFileName))
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		if _, err := s.CheckStorageUsage(); CodeOf(err) != CodeIO {
			t.Fatalf("repeated health check %d: %v", i, err)
		}
	}

	afterAcked, err := s.AckedThrough()
	if err != nil || afterAcked != beforeAcked {
		t.Fatalf("ACK watermark mutated: before=%d after=%d err=%v", beforeAcked, afterAcked, err)
	}
	afterUnresolved, _, err := s.UnresolvedAckedCleanup()
	if err != nil || afterUnresolved != beforeUnresolved {
		t.Fatalf("cleanup latch mutated: before=%v after=%v err=%v", beforeUnresolved, afterUnresolved, err)
	}
	afterPending, err := s.ListPendingFiles(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterPending) != len(beforePending) {
		t.Fatalf("pending files mutated: before=%v after=%v", beforePending, afterPending)
	}
	for i := range beforePending {
		if afterPending[i] != beforePending[i] {
			t.Fatalf("pending file order/path mutated: before=%v after=%v", beforePending, afterPending)
		}
	}
	windowAfter, err := os.ReadFile(filepath.Join(s.dir, outboundWindowFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(windowAfter) != string(windowBefore) {
		t.Fatal("frozen outbound window bytes mutated by health checks")
	}
	if s.degraded != beforeDegraded || s.cleanupUnresolved != beforeCleanup || s.quarantined != beforeQuarantined {
		t.Fatalf("fault latches mutated: degraded=%v/%v cleanup=%v/%v quarantined=%d/%d",
			beforeDegraded, s.degraded, beforeCleanup, s.cleanupUnresolved, beforeQuarantined, s.quarantined)
	}
	replay, err := s.ReadOrReserveOutboundBatch(1, 1024)
	if err != nil || replay.FirstSequence != batch.FirstSequence || replay.LastSequence != batch.LastSequence {
		t.Fatalf("immutable outbound replay broken: %+v err=%v", replay, err)
	}
}

func TestCheckStorageUsageTransientCapacityFailureDoesNotLatch(t *testing.T) {
	s := openTest(t, 1024)
	defer s.Close()
	fail := true
	s.space = func(int64) (spaceStatus, error) {
		if fail {
			return spaceStatus{}, errors.New("transient statfs")
		}
		return spaceStatus{totalBytes: 1000000, usedBytes: 1000, usedPct: .1}, nil
	}

	if _, err := s.CheckStorageUsage(); err == nil {
		t.Fatal("transient capacity failure must surface")
	}
	if s.degraded || s.cleanupUnresolved {
		t.Fatalf("transient capacity failure must not create persistent fault latches: degraded=%v cleanup=%v", s.degraded, s.cleanupUnresolved)
	}

	fail = false
	pct, err := s.CheckStorageUsage()
	if err != nil || pct != .1 {
		t.Fatalf("subsequent successful capacity query must recover: pct=%v err=%v", pct, err)
	}
	if s.degraded || s.cleanupUnresolved {
		t.Fatalf("recovery must leave latches clear: degraded=%v cleanup=%v", s.degraded, s.cleanupUnresolved)
	}
}

func TestCheckStorageUsageDoesNotScanUnseenMalformedFile(t *testing.T) {
	s := openTest(t, 1024)
	defer s.Close()
	add(t, s, "good")
	malformed := filepath.Join(s.dir, recordFileName(99))
	if err := os.WriteFile(malformed, []byte(`{"not":"a-valid-spool-record"`), 0640); err != nil {
		t.Fatal(err)
	}

	pct, err := s.CheckStorageUsage()
	if err != nil {
		t.Fatalf("health must not fail solely because an unseen malformed file exists: pct=%v err=%v", pct, err)
	}
	if pct != .1 {
		t.Fatalf("usedPct=%v want=0.1", pct)
	}
	if _, err := os.Stat(malformed); err != nil {
		t.Fatalf("malformed file must remain unscanned/unquarantined: %v", err)
	}
	entries, err := os.ReadDir(s.quarantineDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("health must not quarantine unseen malformed files: n=%d err=%v", len(entries), err)
	}
	if s.degraded || s.cleanupUnresolved {
		t.Fatalf("health must not invent fault latches: degraded=%v cleanup=%v", s.degraded, s.cleanupUnresolved)
	}
}
