package spool

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"platform/sensor/internal/control"
)

func openTest(t *testing.T, max int) *DiskSpool {
	t.Helper()
	s, err := Open(Config{Dir: t.TempDir(), MaxRecordBytes: max})
	if err != nil {
		t.Fatal(err)
	}
	s.space = func(extra int64) (spaceStatus, error) {
		return spaceStatus{totalBytes: 1000000, usedBytes: 1000, usedPct: .1}, nil
	}
	return s
}
func add(t *testing.T, s *DiskSpool, p string) Record {
	t.Helper()
	r, e := s.Append(AppendRequest{Class: ClassMetric, Type: "metric", Payload: []byte(p), CreatedAt: time.Now().Add(-time.Second)})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestAppendSequencesRestartAndStateRecovery(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(Config{Dir: dir, MaxRecordBytes: 1024})
	if e != nil {
		t.Fatal(e)
	}
	s.space = func(int64) (spaceStatus, error) { return spaceStatus{1000000, 1000, .1}, nil }
	streamID := s.StreamID()
	if streamID == "" {
		t.Fatal("durable spool stream ID must be initialized")
	}
	for i := 1; i <= 20; i++ {
		r, e := s.Append(AppendRequest{Class: ClassMetric, Type: "m", Payload: []byte("x")})
		if e != nil || r.Sequence != uint64(i) {
			t.Fatalf("append %d: %#v %v", i, r, e)
		}
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s2, e := Open(Config{Dir: dir, MaxRecordBytes: 1024})
	if e != nil {
		t.Fatal(e)
	}
	s2.space = s.space
	if s2.StreamID() != streamID {
		t.Fatalf("stream ID changed across process-style reopen: before=%s after=%s", streamID, s2.StreamID())
	}
	r, e := s2.Append(AppendRequest{Class: ClassEvent, Type: "e", Payload: []byte("y")})
	if e != nil || r.Sequence != 21 {
		t.Fatalf("restart seq=%d err=%v", r.Sequence, e)
	}
}
func TestSpoolRecreationCreatesNewStreamID(t *testing.T) {
	first := openTest(t, 1024)
	firstID := first.StreamID()
	if firstID == "" {
		t.Fatal("first stream ID missing")
	}
	second := openTest(t, 1024)
	secondID := second.StreamID()
	if secondID == "" {
		t.Fatal("second stream ID missing")
	}
	if firstID == secondID {
		t.Fatalf("independent spool generations must not share stream ID: %s", firstID)
	}
}

func TestOrderedBatchBounds(t *testing.T) {
	s := openTest(t, 1024)
	for _, p := range []string{"aa", "bbb", "cccc"} {
		add(t, s, p)
	}
	b, e := s.ReadBatch(2, 10)
	if e != nil {
		t.Fatal(e)
	}
	if len(b.Records) != 2 || b.FirstSequence != 1 || b.LastSequence != 2 {
		t.Fatalf("bad batch %#v", b)
	}
	b, e = s.ReadBatch(10, 4)
	if e != nil {
		t.Fatal(e)
	}
	if len(b.Records) != 1 || b.PayloadBytes != 2 {
		t.Fatalf("byte bound %#v", b)
	}
}
func TestAckDuplicateInvalidForwardAndDeletion(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "a")
	add(t, s, "b")
	add(t, s, "c")
	if e := s.AcknowledgeThrough(2); e != nil {
		t.Fatal(e)
	}
	if e := s.AcknowledgeThrough(2); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(s.dir, recordFileName(1))); !os.IsNotExist(e) {
		t.Fatalf("acked record retained: %v", e)
	}
	if e := s.AcknowledgeThrough(4); CodeOf(e) != CodeSequence {
		t.Fatalf("forward ack: %v", e)
	}
	b, e := s.ReadBatch(10, 100)
	if e != nil || len(b.Records) != 1 || b.FirstSequence != 3 {
		t.Fatalf("remaining %#v %v", b, e)
	}
}
func TestPartialAndCorruptQuarantine(t *testing.T) {
	s := openTest(t, 1024)
	r := add(t, s, "payload")
	p := filepath.Join(s.dir, recordFileName(r.Sequence))
	data, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	var rec Record
	if e = json.Unmarshal(data, &rec); e != nil {
		t.Fatal(e)
	}
	rec.Payload = []byte("evil")
	bad, _ := json.Marshal(rec)
	if e = os.WriteFile(p, bad, 0640); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ReadBatch(10, 100); CodeOf(e) != CodeCorruption {
		t.Fatalf("want corruption got %v", e)
	}
	entries, e := os.ReadDir(s.quarantineDir)
	if e != nil || len(entries) != 1 {
		t.Fatalf("quarantine=%d err=%v", len(entries), e)
	}
	st, _ := s.Stats()
	if !st.CorruptionDetected {
		t.Fatal("degradation not exposed")
	}
}
func TestQuarantineLogIncludesSpoolKind(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	dir := t.TempDir()
	s, err := Open(Config{Dir: dir, MaxRecordBytes: 1024, Kind: KindAgentIngress, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	s.space = func(int64) (spaceStatus, error) {
		return spaceStatus{totalBytes: 1000000, usedBytes: 1000, usedPct: .1}, nil
	}
	r := add(t, s, "payload")
	p := filepath.Join(s.dir, recordFileName(r.Sequence))
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var rec Record
	if err = json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	rec.Payload = []byte("evil")
	bad, _ := json.Marshal(rec)
	if err = os.WriteFile(p, bad, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadBatch(10, 100); CodeOf(err) != CodeCorruption {
		t.Fatalf("want corruption got %v", err)
	}
	if !strings.Contains(logs.String(), "\"spool_type\":\"agent_ingress\"") {
		t.Fatalf("quarantine log must identify Agent ingress spool: %s", logs.String())
	}
}

func TestRecoveryQuarantinesPartial(t *testing.T) {
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(dir, "record-00000000000000000001.json.tmp"), []byte("partial"), 0640); e != nil {
		t.Fatal(e)
	}
	s, e := Open(Config{Dir: dir, MaxRecordBytes: 1024})
	if e != nil {
		t.Fatal(e)
	}
	entries, _ := os.ReadDir(s.quarantineDir)
	if len(entries) != 1 {
		t.Fatalf("want quarantine got %d", len(entries))
	}
}
func TestProtectionAndRecoveryAfterFreeSpace(t *testing.T) {
	s := openTest(t, 1024)
	protected := true
	s.space = func(int64) (spaceStatus, error) {
		if protected {
			return spaceStatus{100, 79, 79}, nil
		}
		return spaceStatus{1000, 1, .1}, nil
	}
	if _, e := s.Append(AppendRequest{Class: ClassMetric, Type: "m", Payload: []byte("123")}); CodeOf(e) != CodeProtection {
		t.Fatalf("want protection got %v", e)
	}
	protected = false
	if _, e := s.Append(AppendRequest{Class: ClassMetric, Type: "m", Payload: []byte("123")}); e != nil {
		t.Fatal(e)
	}
}
func TestConcurrentAppendStrictSequences(t *testing.T) {
	s := openTest(t, 1024)
	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, e := s.Append(AppendRequest{Class: ClassOperational, Type: "op", Payload: []byte("x")})
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	b, e := s.ReadBatch(n, 1024)
	if e != nil {
		t.Fatal(e)
	}
	if len(b.Records) != n {
		t.Fatalf("got %d", len(b.Records))
	}
	for i, r := range b.Records {
		if r.Sequence != uint64(i+1) {
			t.Fatalf("sequence %d at %d", r.Sequence, i)
		}
	}
}
func TestCloseIdempotentAndRejectAppend(t *testing.T) {
	s := openTest(t, 1024)
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Append(AppendRequest{Class: ClassMetric, Type: "m", Payload: []byte("x")}); CodeOf(e) != CodeClosed {
		t.Fatalf("want closed got %v", e)
	}
}
func TestSymlinkRootAndStateRejected(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if e := os.Mkdir(real, 0750); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(root, "link")
	if e := os.Symlink(real, link); e != nil {
		t.Skip(e)
	}
	if _, e := Open(Config{Dir: link, MaxRecordBytes: 100}); CodeOf(e) != CodeUnsafePath {
		t.Fatalf("root symlink: %v", e)
	}
	dir := filepath.Join(root, "spool")
	s, e := Open(Config{Dir: dir, MaxRecordBytes: 100})
	if e != nil {
		t.Fatal(e)
	}
	_ = s.Close()
	_ = os.Remove(filepath.Join(dir, stateFileName))
	if e = os.Symlink(filepath.Join(root, "outside"), filepath.Join(dir, stateFileName)); e != nil {
		t.Skip(e)
	}
	if _, e = Open(Config{Dir: dir, MaxRecordBytes: 100}); CodeOf(e) != CodeUnsafePath {
		t.Fatalf("state symlink: %v", e)
	}
}
func TestAckCannotCrossMissingSequence(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "a")
	add(t, s, "b")
	if e := os.Remove(filepath.Join(s.dir, recordFileName(1))); e != nil {
		t.Fatal(e)
	}
	if e := s.AcknowledgeThrough(2); CodeOf(e) != CodeSequence {
		t.Fatalf("want gap error got %v", e)
	}
}
func TestLegacyUnsafeDeleteDisabled(t *testing.T) {
	s := openTest(t, 1024)
	if e := s.Remove("/tmp/anything"); CodeOf(e) != CodeUnsafeLegacyAPI {
		t.Fatalf("legacy remove=%v", e)
	}
}
func TestStatsOldestAndBytes(t *testing.T) {
	s := openTest(t, 1024)
	add(t, s, "abc")
	st, e := s.Stats()
	if e != nil {
		t.Fatal(e)
	}
	if st.RecordCount != 1 || st.SpoolBytes <= 0 || st.OldestRecordAge <= 0 {
		t.Fatalf("bad stats %#v", st)
	}
}
func TestSpaceInspectionErrorDegrades(t *testing.T) {
	s := openTest(t, 1024)
	s.space = func(int64) (spaceStatus, error) { return spaceStatus{}, errors.New("statfs") }
	if _, e := s.Append(AppendRequest{Class: ClassMetric, Type: "m", Payload: []byte("x")}); CodeOf(e) != CodeIO {
		t.Fatalf("want io got %v", e)
	}
}

func TestControlDirectoryCoexistsWithSpoolRecovery(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Config{Dir: dir, MaxRecordBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	s.space = func(extra int64) (spaceStatus, error) {
		return spaceStatus{totalBytes: 1000000, usedBytes: 1000, usedPct: .1}, nil
	}
	add(t, s, "keep-me")
	controlDir := filepath.Join(dir, "control")
	if err := os.MkdirAll(controlDir, 0o750); err != nil {
		t.Fatal(err)
	}
	snapPath := filepath.Join(controlDir, "applied-state.json")
	store, err := control.NewStore(snapPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg := control.GoldenConfiguration()
	checksum, err := control.ComputeDesiredChecksum(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(control.AppliedSnapshot{
		SchemaVersion: 1,
		Generation:    9,
		Checksum:      checksum,
		Configuration: cfg,
		AppliedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(Config{Dir: dir, MaxRecordBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	s2.space = func(extra int64) (spaceStatus, error) {
		return spaceStatus{totalBytes: 1000000, usedBytes: 1000, usedPct: .1}, nil
	}
	defer s2.Close()

	if _, err := os.Stat(controlDir); err != nil {
		t.Fatalf("control dir removed: %v", err)
	}
	loaded, err := store.Load()
	if err != nil || loaded == nil || loaded.Generation != 9 {
		t.Fatalf("snapshot lost: %#v err=%v", loaded, err)
	}
	st, err := s2.Stats()
	if err != nil || st.RecordCount != 1 {
		t.Fatalf("spool records broken: %#v err=%v", st, err)
	}
	add(t, s2, "after-recovery")
	entries, err := os.ReadDir(filepath.Join(dir, quarantineDirName))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "applied-state") || strings.Contains(e.Name(), "control") {
			t.Fatalf("control artifact quarantined: %s", e.Name())
		}
	}
}
