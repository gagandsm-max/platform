package spool

import (
	"encoding/json"
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
