package queue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func commitAcceptedThrough(t *testing.T, q *DurableQueue, through uint64) {
	t.Helper()
	batch, err := q.ReadBatch(1000, 10<<20)
	if err != nil {
		t.Fatalf("ReadBatch before CommitAccepted: %v", err)
	}
	var prefix []Record
	for _, r := range batch {
		prefix = append(prefix, r)
		if r.Sequence == through {
			break
		}
	}
	if len(prefix) == 0 || prefix[len(prefix)-1].Sequence != through {
		t.Fatalf("submitted batch missing acceptedThrough=%d: %+v", through, batch)
	}
	if err := q.CommitAccepted(prefix, through); err != nil {
		t.Fatalf("CommitAccepted(%d) failed: %v", through, err)
	}
}

func writeActiveRecord(t *testing.T, root string, seq uint64, id string, payload []byte) string {
	t.Helper()
	now := time.Now().UTC()
	checksum := computeChecksum(seq, id, now, payload)
	rec := Record{
		Sequence:  seq,
		ID:        id,
		CreatedAt: now,
		Checksum:  checksum,
		Payload:   payload,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	name := recordFileName(seq, id)
	path := filepath.Join(root, "active", name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestDurableQueueBasicOperations(t *testing.T) {
	tempDir := t.TempDir()

	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatalf("failed to initialize queue: %v", err)
	}

	// 1. Enqueue two records
	rec1, err := q.Enqueue([]byte("payload-1"))
	if err != nil {
		t.Fatalf("unexpected enqueue error 1: %v", err)
	}
	if rec1.Sequence != 1 {
		t.Fatalf("expected sequence 1, got %d", rec1.Sequence)
	}

	rec2, err := q.Enqueue([]byte("payload-2"))
	if err != nil {
		t.Fatalf("unexpected enqueue error 2: %v", err)
	}
	if rec2.Sequence != 2 {
		t.Fatalf("expected sequence 2, got %d", rec2.Sequence)
	}

	if q.Len() != 2 {
		t.Fatalf("expected 2 items, got %d", q.Len())
	}

	// 2. Read batch (does not delete)
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatalf("read batch failed: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 records in batch, got %d", len(batch))
	}
	if !bytes.Equal(batch[0].Payload, []byte("payload-1")) {
		t.Fatalf("payload mismatch: %s", batch[0].Payload)
	}
	if !bytes.Equal(batch[1].Payload, []byte("payload-2")) {
		t.Fatalf("payload mismatch: %s", batch[1].Payload)
	}

	// Len is STILL 2 (no deletion before ACK)
	if q.Len() != 2 {
		t.Fatalf("expected 2 items still retained in queue, got %d", q.Len())
	}

	// 3. Commit accepted through sequence 1
	if err := q.CommitAccepted(batch[:1], 1); err != nil {
		t.Fatalf("commit accepted failed: %v", err)
	}
	if q.Len() != 1 {
		t.Fatalf("expected 1 item after ACK deletion, got %d", q.Len())
	}

	// 4. Subsequent read sees only remaining sequence 2
	batch2, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch2) != 1 || batch2[0].Sequence != 2 {
		t.Fatalf("expected only sequence 2, got %+v", batch2)
	}
}

func TestDurableQueueCapacityLimits(t *testing.T) {
	tempDir := t.TempDir()

	// Max 2 records, 100 bytes payload limit
	q, err := NewDurableQueue(tempDir, 2, 1024, 50)
	if err != nil {
		t.Fatal(err)
	}

	// Record too large
	_, err = q.Enqueue(make([]byte, 60))
	if err != ErrRecordTooLarge {
		t.Fatalf("expected ErrRecordTooLarge, got %v", err)
	}

	// Fill to max records
	_, err = q.Enqueue([]byte("record-1"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.Enqueue([]byte("record-2"))
	if err != nil {
		t.Fatal(err)
	}

	// 3rd should hit ErrQueueFull
	_, err = q.Enqueue([]byte("record-3"))
	if err != ErrQueueFull {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
}

func TestDurableQueueCorruptionQuarantine(t *testing.T) {
	tempDir := t.TempDir()

	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}

	_, err = q.Enqueue([]byte("valid-data"))
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt the record file on disk
	// Find actual active file
	entries, _ := os.ReadDir(filepath.Join(tempDir, "active"))
	if len(entries) == 0 {
		t.Fatal("expected active file")
	}
	activeFile := filepath.Join(tempDir, "active", entries[0].Name())

	// Tamper with content to break checksum
	tampered := []byte(`{"sequence":1,"id":"rec-1","checksum":"wrong_hash","payload":"dGVzdA=="}`)
	if err := os.WriteFile(activeFile, tampered, 0600); err != nil {
		t.Fatal(err)
	}

	// ReadBatch should detect corruption and quarantine
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 0 {
		t.Fatalf("expected 0 valid records, got %d", len(batch))
	}

	stats := q.Stats()
	if stats.QuarantineCount != 1 {
		t.Fatalf("expected 1 quarantined record, got %d", stats.QuarantineCount)
	}
	if stats.ActiveCount != 0 {
		t.Fatalf("expected 0 active records, got %d", stats.ActiveCount)
	}
}

func TestDurableQueueRecoveryAfterRestart(t *testing.T) {
	tempDir := t.TempDir()

	// Instance 1 writes records
	q1, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}

	_, err = q1.Enqueue([]byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = q1.Enqueue([]byte("second"))
	if err != nil {
		t.Fatal(err)
	}

	// Instance 2 recovers state from disk
	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}

	if q2.Len() != 2 {
		t.Fatalf("expected 2 recovered items, got %d", q2.Len())
	}

	batch, err := q2.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 items, got %d", len(batch))
	}
	if batch[0].Sequence != 1 || batch[1].Sequence != 2 {
		t.Fatalf("sequence mismatch: %d, %d", batch[0].Sequence, batch[1].Sequence)
	}

	// Enqueue on recovered queue continues monotonic sequence
	rec3, err := q2.Enqueue([]byte("third"))
	if err != nil {
		t.Fatal(err)
	}
	if rec3.Sequence != 3 {
		t.Fatalf("expected sequence 3, got %d", rec3.Sequence)
	}
}

func TestDurableQueueSerializedByteBound(t *testing.T) {
	tempDir := t.TempDir()
	probe, err := NewDurableQueue(filepath.Join(tempDir, "probe"), 100, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("x"), 64)
	if _, err := probe.Enqueue(payload); err != nil {
		t.Fatal(err)
	}
	serialized := probe.Stats().ActiveBytes
	if serialized <= 0 {
		t.Fatal("expected positive serialized size")
	}

	q, err := NewDurableQueue(filepath.Join(tempDir, "bounded"), 100, serialized+10, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue(payload); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if _, err := q.Enqueue(payload); err != ErrQueueFull {
		t.Fatalf("expected ErrQueueFull on serialized size bound, got %v", err)
	}
	if q.Len() != 1 {
		t.Fatalf("queue-full must preserve prior records, got len=%d", q.Len())
	}
}

func TestDurableQueueTrailingJSONQuarantine(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := q.Enqueue([]byte("valid"))
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(tempDir, "active"))
	activeFile := filepath.Join(tempDir, "active", entries[0].Name())
	raw, err := os.ReadFile(activeFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(activeFile, append(raw, []byte(`{"extra":true}`)...), 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 0 {
		t.Fatalf("trailing JSON must quarantine, got %d", len(batch))
	}
	if q.Stats().QuarantineCount != 1 {
		t.Fatalf("expected quarantine, got %+v", q.Stats())
	}
	_ = rec
}

func TestDurableQueueMetadataTamperQuarantine(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(tempDir, "active"))
	activeFile := filepath.Join(tempDir, "active", entries[0].Name())
	raw, err := os.ReadFile(activeFile)
	if err != nil {
		t.Fatal(err)
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	rec.Sequence = 999
	tampered, _ := json.Marshal(rec)
	if err := os.WriteFile(activeFile, tampered, 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 0 {
		t.Fatal("metadata tamper must quarantine")
	}
}

func TestDurableQueueNoHeadDropOnFull(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 1, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	first, err := q.Enqueue([]byte("keep-me"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("reject-me")); err != ErrQueueFull {
		t.Fatalf("expected QUEUE_FULL, got %v", err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 1 || !bytes.Equal(batch[0].Payload, []byte("keep-me")) || batch[0].Sequence != first.Sequence {
		t.Fatalf("full queue must not head-drop: %+v", batch)
	}
}

func TestDurableQueueQuarantineFailureFailClosed(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("valid-data")); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(tempDir, "active"))
	activeFile := filepath.Join(tempDir, "active", entries[0].Name())
	if err := os.WriteFile(activeFile, []byte(`{"not":"valid"`), 0600); err != nil {
		t.Fatal(err)
	}
	// Replace quarantine dir with a regular file so rename fails.
	qPath := filepath.Join(tempDir, "quarantine")
	if err := os.RemoveAll(qPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(qPath, []byte("not-a-dir"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ReadBatch(10, 1024*1024); err == nil {
		t.Fatal("expected quarantine failure to surface")
	}
	if _, err := os.Stat(activeFile); err != nil {
		t.Fatalf("corrupt record must remain when quarantine fails: %v", err)
	}
}

func TestDurableQueueCommitAcceptedPartialOK(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("b")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CommitAccepted(batch[:1], 1); err != nil {
		t.Fatal(err)
	}
	batch, err = q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 1 || batch[0].Sequence != 2 {
		t.Fatalf("partial commit failed: %+v", batch)
	}
}

func TestDurableQueueSequenceLineageSurvivesDrain(t *testing.T) {
	tempDir := t.TempDir()

	q1, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	var last uint64
	var all []Record
	for i := 0; i < 3; i++ {
		rec, err := q1.Enqueue([]byte("payload"))
		if err != nil {
			t.Fatal(err)
		}
		last = rec.Sequence
		all = append(all, *rec)
	}
	if err := q1.CommitAccepted(all, last); err != nil {
		t.Fatal(err)
	}
	if q1.Len() != 0 {
		t.Fatalf("expected drained queue, got %d", q1.Len())
	}

	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	next, err := q2.Enqueue([]byte("after-reopen"))
	if err != nil {
		t.Fatal(err)
	}
	if next.Sequence <= last {
		t.Fatalf("next sequence %d must exceed prior high watermark %d", next.Sequence, last)
	}
}

func TestDurableQueueCorruptMetaFailsClosed(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("x")); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(tempDir, sequenceMetaFile)
	if err := os.WriteFile(metaPath, []byte("{not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024); err == nil {
		t.Fatal("corrupt sequence metadata must fail closed")
	}
}

func TestDurableQueueJSONCompleteness(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := q.Enqueue([]byte("valid-json"))
	if err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 1 || batch[0].Sequence != rec.Sequence {
		t.Fatalf("valid object must pass: err=%v batch=%+v", err, batch)
	}

	// object + second JSON object
	entries, _ := os.ReadDir(filepath.Join(tempDir, "active"))
	activeFile := filepath.Join(tempDir, "active", entries[0].Name())
	raw, err := os.ReadFile(activeFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(activeFile, append(raw, []byte(`{"extra":true}`)...), 0600); err != nil {
		t.Fatal(err)
	}
	batch, err = q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 0 {
		t.Fatalf("second JSON object must quarantine, got %d", len(batch))
	}

	// trailing non-JSON garbage on a fresh record
	q2Dir := t.TempDir()
	q2, err := NewDurableQueue(q2Dir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q2.Enqueue([]byte("again")); err != nil {
		t.Fatal(err)
	}
	entries2, _ := os.ReadDir(filepath.Join(q2Dir, "active"))
	active2 := filepath.Join(q2Dir, "active", entries2[0].Name())
	raw2, _ := os.ReadFile(active2)
	if err := os.WriteFile(active2, append(raw2, []byte("not-json-garbage")...), 0600); err != nil {
		t.Fatal(err)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 0 {
		t.Fatalf("trailing garbage must quarantine, got %d", len(batch))
	}
}

func TestDurableQueueCorruptMetaNeverResetsSequence(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	var last uint64
	for i := 0; i < 3; i++ {
		rec, err := q.Enqueue([]byte("payload"))
		if err != nil {
			t.Fatal(err)
		}
		last = rec.Sequence
	}
	metaPath := filepath.Join(tempDir, sequenceMetaFile)
	if err := os.WriteFile(metaPath, append([]byte(`{"highestSequence":3}`), []byte(`{"highestSequence":1}`)...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024); err == nil {
		t.Fatal("trailing second meta object must fail closed")
	}
	// After fail-closed reopen rejection, rewrite valid meta and ensure lineage continues above last.
	if err := os.WriteFile(metaPath, []byte(`{"highestSequence":3}`), 0600); err != nil {
		t.Fatal(err)
	}
	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	next, err := q2.Enqueue([]byte("after-corrupt-attempt"))
	if err != nil {
		t.Fatal(err)
	}
	if next.Sequence <= last {
		t.Fatalf("corrupt meta must never reset sequence; next=%d last=%d", next.Sequence, last)
	}
}

func activeRecordPath(t *testing.T, dir string, seq uint64) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "active"))
	if err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("rec_%019d_", seq)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			return filepath.Join(dir, "active", e.Name())
		}
	}
	t.Fatalf("no active record for sequence %d", seq)
	return ""
}

func corruptRecordFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(`{"sequence":1,"id":"rec-1","checksum":"wrong_hash","payload":"dGVzdA=="}`), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestDurableQueueMidQueueQuarantineBlocksLaterDelivery(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("three")); err != nil {
		t.Fatal(err)
	}
	corruptRecordFile(t, activeRecordPath(t, tempDir, 2))

	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatalf("prefix before poison must remain readable: %v", err)
	}
	if len(batch) != 1 || batch[0].Sequence != 1 || !bytes.Equal(batch[0].Payload, []byte("one")) {
		t.Fatalf("expected only sequence 1, got %+v", batch)
	}
	if q.Stats().QuarantineCount != 1 {
		t.Fatalf("expected seq 2 quarantined, stats=%+v", q.Stats())
	}

	laterPath := activeRecordPath(t, tempDir, 3)
	raw, err := os.ReadFile(laterPath)
	if err != nil {
		t.Fatalf("later record must remain intact: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"sequence":3`)) {
		t.Fatalf("later sequence missing: %s", raw)
	}

	commitAcceptedThrough(t, q, 1)
	batch, err = q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrDeliveryBlocked) {
		t.Fatalf("expected QUEUE_DELIVERY_BLOCKED after quarantined sequence, got batch=%+v err=%v", batch, err)
	}
	if len(batch) != 0 {
		t.Fatalf("blocked read must not return later records, got %+v", batch)
	}
	if _, err := os.Stat(laterPath); err != nil {
		t.Fatalf("sequence 3 must remain in active/: %v", err)
	}
	if q.Len() != 1 {
		t.Fatalf("later record must occupy active queue, len=%d", q.Len())
	}
}

func TestDurableQueueQuarantineBlockSurvivesRestart(t *testing.T) {
	tempDir := t.TempDir()
	q1, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q1.Enqueue([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q1.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	if _, err := q1.Enqueue([]byte("three")); err != nil {
		t.Fatal(err)
	}
	corruptRecordFile(t, activeRecordPath(t, tempDir, 2))
	if _, err := q1.ReadBatch(10, 1024*1024); err != nil {
		t.Fatal(err)
	}

	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := q2.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatalf("prefix must survive restart: %v", err)
	}
	if len(batch) != 1 || batch[0].Sequence != 1 {
		t.Fatalf("expected sequence 1 after restart, got %+v", batch)
	}
	commitAcceptedThrough(t, q2, 1)
	batch, err = q2.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrDeliveryBlocked) {
		t.Fatalf("blocked stream must survive restart, batch=%+v err=%v", batch, err)
	}
	if q2.Stats().BlockedFromSequence != 2 {
		t.Fatalf("expected blocked-from 2, got %+v", q2.Stats())
	}
	next, err := q2.Enqueue([]byte("four"))
	if err != nil {
		t.Fatal(err)
	}
	if next.Sequence != 4 {
		t.Fatalf("quarantine must not reset sequence lineage, got %d", next.Sequence)
	}
}

func TestDurableQueueQuarantineFullFailClosedNoOldestDelete(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	q.maxQuarantineRecords = 0

	if _, err := q.Enqueue([]byte("head-poison")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("later-valid")); err != nil {
		t.Fatal(err)
	}
	head := activeRecordPath(t, tempDir, 1)
	later := activeRecordPath(t, tempDir, 2)
	corruptRecordFile(t, head)

	_, err = q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrQuarantineFull) {
		t.Fatalf("expected QUEUE_QUARANTINE_FULL, got %v", err)
	}
	if q.Stats().QuarantineCount != 0 {
		t.Fatalf("quarantine-full must not transfer, stats=%+v", q.Stats())
	}
	if _, err := os.Stat(head); err != nil {
		t.Fatalf("poison head must remain in active/: %v", err)
	}
	if _, err := os.Stat(later); err != nil {
		t.Fatalf("later record must remain intact: %v", err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrQuarantineFull) {
		t.Fatalf("must not skip to later records when head cannot quarantine, batch=%+v err=%v", batch, err)
	}
}

func TestDurableQueueRecoverQuarantineFullFailClosed(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("first-poison")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("second-poison")); err != nil {
		t.Fatal(err)
	}
	first := activeRecordPath(t, tempDir, 1)
	second := activeRecordPath(t, tempDir, 2)
	corruptRecordFile(t, first)
	corruptRecordFile(t, second)
	q.maxQuarantineRecords = 1

	if err := q.recoverState(); err != nil && !errors.Is(err, ErrQuarantineFull) {
		t.Fatalf("recover must fail-closed retain, got %v", err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("second poison must remain in active/ when quarantine is full: %v", err)
	}
	qEntries, err := os.ReadDir(filepath.Join(tempDir, "quarantine"))
	if err != nil {
		t.Fatal(err)
	}
	jsonCount := 0
	for _, e := range qEntries {
		if strings.HasSuffix(e.Name(), ".json") {
			jsonCount++
		}
	}
	if jsonCount != 1 {
		t.Fatalf("must not delete oldest quarantine record, jsonCount=%d", jsonCount)
	}
}

func TestDurableQueueQuarantineDestExistsFailClosed(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	src := activeRecordPath(t, tempDir, 1)
	entries, _ := os.ReadDir(filepath.Join(tempDir, "active"))
	dest := filepath.Join(tempDir, "quarantine", entries[0].Name())
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("later")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if len(batch) != 0 {
		t.Fatalf("dest-exists/duplicate must not deliver any record, including the active copy: %+v err=%v", batch, err)
	}
	if !errors.Is(err, ErrQuarantineDestExists) && !errors.Is(err, ErrDeliveryBlocked) && !errors.Is(err, ErrQuarantineUncertain) {
		t.Fatalf("unexpected dest-exists handling: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("active record must remain when dest exists: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("existing quarantine dest must not be overwritten away: %v", err)
	}
	destRaw, _ := os.ReadFile(dest)
	if !bytes.Equal(destRaw, raw) {
		t.Fatal("must not overwrite existing quarantine destination")
	}
}

func TestDurableQueueEmptyPayloadQuarantine(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue(nil); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 0 {
		t.Fatalf("empty payload must not be delivered, got %+v", batch)
	}
	if q.Stats().QuarantineCount != 1 {
		t.Fatalf("empty payload must quarantine, stats=%+v", q.Stats())
	}
}

func TestDurableQueueReasonSidecarHasCodeOnly(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("secret-payload-must-not-be-copied-to-reason")); err != nil {
		t.Fatal(err)
	}
	corruptRecordFile(t, activeRecordPath(t, tempDir, 1))
	if _, err := q.ReadBatch(10, 1024*1024); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(tempDir, "quarantine"))
	if err != nil {
		t.Fatal(err)
	}
	foundReason := false
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".reason") {
			continue
		}
		foundReason = true
		if strings.HasSuffix(e.Name(), ".json") {
			t.Fatal("reason sidecar must not use .json suffix")
		}
		raw, err := os.ReadFile(filepath.Join(tempDir, "quarantine", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte("secret-payload-must-not-be-copied-to-reason")) {
			t.Fatalf("reason sidecar leaked payload: %s", raw)
		}
		var meta map[string]any
		if err := json.Unmarshal(raw, &meta); err != nil {
			t.Fatal(err)
		}
		if len(meta) != 1 {
			t.Fatalf("reason sidecar must contain only code, got %v", meta)
		}
		if meta["code"] != "QUEUE_RECORD_CORRUPT" {
			t.Fatalf("unexpected reason code %v", meta["code"])
		}
	}
	if !foundReason {
		t.Fatal("expected quarantine reason sidecar")
	}

	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if q2.Stats().QuarantineCount != 1 {
		t.Fatalf("reason sidecar must not be counted as a quarantined record, stats=%+v", q2.Stats())
	}
}

func recordPresent(t *testing.T, root, sub string, seq uint64) bool {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, sub))
	if err != nil {
		return false
	}
	prefix := fmt.Sprintf("rec_%019d_", seq)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".json") {
			return true
		}
	}
	return false
}

func TestDurableQueueDestSyncFailureRollsBackWithoutLoss(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("three")); err != nil {
		t.Fatal(err)
	}
	corruptRecordFile(t, activeRecordPath(t, tempDir, 2))
	q.syncHook = func(phase, dir string) error {
		if phase == syncPhaseQuarantineDest {
			return errors.New("injected dest sync fail")
		}
		return nil
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil && len(batch) != 0 {
		t.Fatalf("must not return a batch with dest-sync error: batch=%+v err=%v", batch, err)
	}
	if !recordPresent(t, tempDir, "active", 2) {
		t.Fatal("dest-sync failure must leave poison in active after durable rollback")
	}
	if recordPresent(t, tempDir, "quarantine", 2) {
		t.Fatal("successful rollback must not leave a quarantine copy of seq 2")
	}
	if err := func() error {
		batch, err := q.ReadBatch(10, 1024*1024)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		return q.CommitAccepted(batch[:1], 1)
	}(); err != nil && !recordPresent(t, tempDir, "active", 1) {
		t.Fatal(err)
	}
	q.syncHook = func(phase, dir string) error {
		if phase == syncPhaseQuarantineDest {
			return errors.New("injected dest sync fail")
		}
		return nil
	}
	batch, err = q.ReadBatch(10, 1024*1024)
	for _, rec := range batch {
		if rec.Sequence > 1 {
			t.Fatalf("later record delivered after dest-sync failure: %+v", rec)
		}
	}
	if !recordPresent(t, tempDir, "active", 3) {
		t.Fatal("later record must remain intact")
	}
}

func TestDurableQueueSourceSyncFailureKeepsQuarantineAuthoritative(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("three")); err != nil {
		t.Fatal(err)
	}
	corruptRecordFile(t, activeRecordPath(t, tempDir, 2))
	q.syncHook = func(phase, dir string) error {
		if phase == syncPhaseActiveSource {
			return errors.New("injected source sync fail")
		}
		return nil
	}
	_, err = q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrQuarantineSourceSyncFailed) && q.Stats().BlockedFromSequence != 2 {
		// prefix seq 1 may be returned first; force second read after committing prefix
		if batch, rerr := q.ReadBatch(10, 1024*1024); rerr == nil && len(batch) > 0 && batch[0].Sequence == 1 {
			_ = q.CommitAccepted(batch[:1], 1)
		}
		_, err = q.ReadBatch(10, 1024*1024)
	}
	if !recordPresent(t, tempDir, "quarantine", 2) {
		t.Fatal("source-sync failure must leave durable quarantine copy")
	}
	q.syncHook = nil
	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := q2.ReadBatch(10, 1024*1024)
	if errors.Is(err, ErrDeliveryBlocked) || errors.Is(err, ErrQuarantineSourceSyncFailed) || errors.Is(err, ErrQuarantineUncertain) {
		for _, rec := range batch {
			if rec.Sequence >= 2 {
				t.Fatalf("must not deliver seq >= 2 after source-sync failure: %+v", rec)
			}
		}
	} else if err == nil {
		for _, rec := range batch {
			if rec.Sequence >= 2 {
				t.Fatalf("must not deliver seq >= 2: %+v", rec)
			}
		}
		commitAcceptedThrough(t, q2, 1)
		batch, err = q2.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrDeliveryBlocked) {
			t.Fatalf("expected blocked after prefix drain, err=%v batch=%+v", err, batch)
		}
	} else {
		t.Fatalf("unexpected reopen error: %v", err)
	}
	if !recordPresent(t, tempDir, "quarantine", 2) && !recordPresent(t, tempDir, "active", 2) {
		t.Fatal("seq 2 must not be lost across reopen")
	}
	if !recordPresent(t, tempDir, "active", 3) {
		t.Fatal("later record must remain")
	}
}

func TestDurableQueueRollbackDestSyncFailureIsUncertain(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	corruptRecordFile(t, activeRecordPath(t, tempDir, 1))
	q.syncHook = func(phase, dir string) error {
		switch phase {
		case syncPhaseQuarantineDest:
			return errors.New("injected dest sync fail")
		case syncPhaseRollbackDest:
			return errors.New("injected rollback dest sync fail")
		}
		return nil
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrQuarantineUncertain) {
		t.Fatalf("expected QUEUE_QUARANTINE_UNCERTAIN, got batch=%+v err=%v", batch, err)
	}
	if len(batch) != 0 {
		t.Fatalf("uncertain location must not deliver, got %+v", batch)
	}
	if !recordPresent(t, tempDir, "active", 1) && !recordPresent(t, tempDir, "quarantine", 1) {
		t.Fatal("telemetry must exist in at least one location")
	}
	if recordPresent(t, tempDir, "active", 2) {
		batch2, err2 := q.ReadBatch(10, 1024*1024)
		if !errors.Is(err2, ErrQuarantineUncertain) || len(batch2) != 0 {
			t.Fatalf("must not continue delivery while uncertain: batch=%+v err=%v", batch2, err2)
		}
	}
	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
		t.Fatalf("uncertain state must survive reopen, batch=%+v err=%v", batch, err)
	}
	if !recordPresent(t, tempDir, "active", 1) && !recordPresent(t, tempDir, "quarantine", 1) {
		t.Fatal("telemetry must still exist after reopen")
	}
}

func TestDurableQueueDuplicateRecoveryNeverDeliversActiveCopy(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	src := activeRecordPath(t, tempDir, 1)
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(tempDir, "active"))
	var name1 string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "rec_0000000000000000001_") {
			name1 = e.Name()
			break
		}
	}
	if err := os.WriteFile(filepath.Join(tempDir, "quarantine", name1), raw, 0600); err != nil {
		t.Fatal(err)
	}
	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := q2.ReadBatch(10, 1024*1024)
	if err == nil {
		for _, rec := range batch {
			if rec.Sequence == 1 {
				t.Fatal("must never deliver active duplicate of quarantined record")
			}
		}
	}
	if !errors.Is(err, ErrDeliveryBlocked) && err != nil && !errors.Is(err, ErrQuarantineUncertain) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !recordPresent(t, tempDir, "active", 1) || !recordPresent(t, tempDir, "quarantine", 1) {
		t.Fatal("must not silently discard either duplicate copy")
	}

	conflictDir := t.TempDir()
	q3, err := NewDurableQueue(conflictDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q3.Enqueue([]byte("keep")); err != nil {
		t.Fatal(err)
	}
	src3 := activeRecordPath(t, conflictDir, 1)
	entries3, _ := os.ReadDir(filepath.Join(conflictDir, "active"))
	if err := os.WriteFile(filepath.Join(conflictDir, "quarantine", entries3[0].Name()), []byte(`{"conflict":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	_ = src3
	q4, err := NewDurableQueue(conflictDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = q4.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
		t.Fatalf("conflicting copies must fail closed, batch=%+v err=%v", batch, err)
	}
	if _, err := os.Stat(src3); err != nil {
		t.Fatalf("must not discard conflicting active copy: %v", err)
	}
}

func TestDurableQueueUnknownSequenceBlocksGlobally(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("three")); err != nil {
		t.Fatal(err)
	}
	malformed := filepath.Join(tempDir, "active", "rec_not-a-sequence_zzz.json")
	if err := os.WriteFile(malformed, []byte(`{"sequence":99,"id":"zzz","checksum":"x","payload":"c2VjcmV0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		batch, err := q.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrUnknownSequence) {
			t.Fatalf("call %d: expected unknown-sequence block, batch=%+v err=%v", i, batch, err)
		}
		if len(batch) != 0 {
			t.Fatalf("call %d: must not submit any records, got %+v", i, batch)
		}
	}
	if _, err := os.Stat(malformed); err != nil {
		t.Fatalf("malformed file must remain in active/: %v", err)
	}
	if !recordPresent(t, tempDir, "active", 3) {
		t.Fatal("later valid records must remain")
	}
	if q.Stats().UnknownSequenceBlock != true || q.Stats().DeliveryBlocked != true {
		t.Fatalf("stats must report unknown-sequence block: %+v", q.Stats())
	}

	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := q2.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrUnknownSequence) || len(batch) != 0 {
		t.Fatalf("blocked state must survive restart, batch=%+v err=%v", batch, err)
	}
	if _, err := os.Stat(malformed); err != nil {
		t.Fatalf("malformed file must remain after restart: %v", err)
	}
	if q2.Stats().ActiveCount != 4 {
		t.Fatalf("unknown-sequence file must count toward active capacity after restart, stats=%+v", q2.Stats())
	}
	if !q2.Stats().UnknownSequenceBlock || !q2.Stats().DeliveryBlocked {
		t.Fatalf("restart must restore unknown-sequence block: %+v", q2.Stats())
	}
}

func TestDurableQueueNoncanonicalWidthFailsClosed(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("canonical-one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("canonical-two")); err != nil {
		t.Fatal(err)
	}
	shortName := filepath.Join(tempDir, "active", "rec_2_reorder.json")
	if err := os.WriteFile(shortName, []byte(`{"sequence":2,"id":"reorder","checksum":"00","payload":"eA=="}`), 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrUnknownSequence) || len(batch) != 0 {
		t.Fatalf("noncanonical width must fail closed, batch=%+v err=%v", err, batch)
	}
	if _, err := os.Stat(shortName); err != nil {
		t.Fatalf("noncanonical file must remain: %v", err)
	}
	if !recordPresent(t, tempDir, "active", 1) || !recordPresent(t, tempDir, "active", 2) {
		t.Fatal("canonical records must remain")
	}
}

func TestDurableQueueIntentFailureBeforeRenameFailsClosed(t *testing.T) {
	assertNoRename := func(t *testing.T, tempDir string, q *DurableQueue) {
		t.Helper()
		if !recordPresent(t, tempDir, "active", 1) {
			t.Fatal("active record 1 must remain in active/ after intent failure")
		}
		if recordPresent(t, tempDir, "quarantine", 1) {
			t.Fatal("record 1 must NOT be in quarantine/ after intent failure")
		}
		if !q.Stats().LocationUncertain || !q.Stats().DeliveryBlocked {
			t.Fatalf("stats must reflect uncertain and blocked: %+v", q.Stats())
		}
		batch2, err2 := q.ReadBatch(10, 1024*1024)
		if !errors.Is(err2, ErrQuarantineUncertain) || len(batch2) != 0 {
			t.Fatalf("record 2 must not be delivered when queue is uncertain, got batch=%+v err=%v", batch2, err2)
		}
	}

	t.Run("intent creation failure before rename", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("record-1")); err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("record-2")); err != nil {
			t.Fatal(err)
		}
		corruptRecordFile(t, activeRecordPath(t, tempDir, 1))

		// Force OpenFile of quarantine.transition to fail before any rename.
		if err := os.Mkdir(filepath.Join(tempDir, transitionIntentFile), 0700); err != nil {
			t.Fatal(err)
		}

		batch, err := q.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
			t.Fatalf("expected ErrQuarantineUncertain and empty batch, got batch=%+v err=%v", batch, err)
		}
		assertNoRename(t, tempDir, q)
	})

	t.Run("intent file sync failure before rename", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("record-1")); err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("record-2")); err != nil {
			t.Fatal(err)
		}
		corruptRecordFile(t, activeRecordPath(t, tempDir, 1))

		q.syncHook = func(phase, target string) error {
			if phase == syncPhaseIntentFile {
				return errors.New("simulated intent sync failure")
			}
			return nil
		}

		batch, err := q.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
			t.Fatalf("expected ErrQuarantineUncertain and empty batch, got batch=%+v err=%v", batch, err)
		}
		assertNoRename(t, tempDir, q)
	})

	t.Run("intent parent-dir sync failure before rename", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("record-1")); err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("record-2")); err != nil {
			t.Fatal(err)
		}
		corruptRecordFile(t, activeRecordPath(t, tempDir, 1))

		q.syncHook = func(phase, target string) error {
			if phase == syncPhaseIntentDir {
				return errors.New("simulated intent dir sync failure")
			}
			return nil
		}

		batch, err := q.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
			t.Fatalf("expected ErrQuarantineUncertain and empty batch, got batch=%+v err=%v", batch, err)
		}
		assertNoRename(t, tempDir, q)
		if _, err := os.Lstat(filepath.Join(tempDir, transitionIntentFile)); err != nil {
			t.Fatalf("intent file should exist after write+file-sync even when parent dir sync fails: %v", err)
		}
	})
}

func TestDurableQueueSuccessfulIntentAndClear(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("record-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("record-2")); err != nil {
		t.Fatal(err)
	}

	var sawIntentFile, sawIntentDir, sawIntentClearDir bool
	var intentPresentDuringRename bool
	var intentMissingBeforeDirSync, intentMissingDuringRename bool
	q.syncHook = func(phase, target string) error {
		switch phase {
		case syncPhaseIntentFile:
			sawIntentFile = true
		case syncPhaseIntentDir:
			sawIntentDir = true
			if _, err := os.Lstat(filepath.Join(tempDir, transitionIntentFile)); err != nil {
				intentMissingBeforeDirSync = true
			}
		case syncPhaseQuarantineDest:
			if _, err := os.Lstat(filepath.Join(tempDir, transitionIntentFile)); err != nil {
				intentMissingDuringRename = true
			} else {
				intentPresentDuringRename = true
			}
		case syncPhaseIntentClearDir:
			sawIntentClearDir = true
		}
		return nil
	}

	corruptRecordFile(t, activeRecordPath(t, tempDir, 1))
	batch, err := q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrDeliveryBlocked) || len(batch) != 0 {
		t.Fatalf("successful quarantine must block later delivery: batch=%+v err=%v", batch, err)
	}
	if intentMissingBeforeDirSync || intentMissingDuringRename {
		t.Fatal("intent must remain durable from create through quarantine rename")
	}
	if !sawIntentFile || !sawIntentDir || !intentPresentDuringRename || !sawIntentClearDir {
		t.Fatalf("expected durable intent create/sync/clear phases; file=%v dir=%v duringRename=%v clearDir=%v",
			sawIntentFile, sawIntentDir, intentPresentDuringRename, sawIntentClearDir)
	}
	if _, err := os.Lstat(filepath.Join(tempDir, transitionIntentFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("intent must be cleared after durable quarantine, err=%v", err)
	}
	if !recordPresent(t, tempDir, "quarantine", 1) {
		t.Fatal("record 1 must be quarantined")
	}
	if recordPresent(t, tempDir, "active", 1) {
		t.Fatal("record 1 must leave active after successful quarantine")
	}
	if !q.Stats().DeliveryBlocked || q.Stats().BlockedFromSequence != 1 {
		t.Fatalf("successful quarantine must block from sequence 1: %+v", q.Stats())
	}
}

func TestDurableQueueExistingIntentRefusesOverwrite(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("rec-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("rec-2")); err != nil {
		t.Fatal(err)
	}

	// Plant an exclusive intent AFTER Open so recovery did not consume it into
	// blockedFromSequence; the next quarantine must refuse to overwrite it.
	rec1Path := activeRecordPath(t, tempDir, 1)
	rec1Name := filepath.Base(rec1Path)
	originalIntent := transitionIntent{Version: 1, Record: rec1Name, Sequence: 1, Code: "QUEUE_RECORD_CORRUPT"}
	originalData, err := json.Marshal(originalIntent)
	if err != nil {
		t.Fatal(err)
	}
	intentPath := filepath.Join(tempDir, transitionIntentFile)
	if err := os.WriteFile(intentPath, originalData, 0600); err != nil {
		t.Fatal(err)
	}

	corruptRecordFile(t, activeRecordPath(t, tempDir, 1))
	batch, err := q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
		t.Fatalf("existing intent must refuse a second quarantine transition: batch=%+v err=%v", batch, err)
	}

	got, err := os.ReadFile(intentPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, originalData) {
		t.Fatalf("existing intent must not be overwritten: got=%s want=%s", got, originalData)
	}
	if !recordPresent(t, tempDir, "active", 1) {
		t.Fatal("poison record must remain in active when intent lease is held")
	}
	if recordPresent(t, tempDir, "quarantine", 1) {
		t.Fatal("poison must not move while prior intent exists")
	}
	if _, err := os.Lstat(filepath.Join(tempDir, uncertainMarkerFile)); err != nil {
		t.Fatalf("uncertain marker must persist when intent overwrite is refused: %v", err)
	}
}

func TestDurableQueueIntentClearFailureLeavesFailClosed(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("record-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("record-2")); err != nil {
		t.Fatal(err)
	}

	corruptRecordFile(t, activeRecordPath(t, tempDir, 1))

	// Inject failure when clearing the transition intent
	q.removeHook = func(path string) error {
		if strings.HasSuffix(path, transitionIntentFile) {
			return errors.New("simulated remove intent failure")
		}
		return nil
	}

	batch, err := q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
		t.Fatalf("expected ErrQuarantineUncertain when clearing intent fails, batch=%+v err=%v", batch, err)
	}

	// Record is in quarantine/
	if !recordPresent(t, tempDir, "quarantine", 1) {
		t.Fatal("record 1 must be quarantined")
	}
	// Intent file must still exist
	intentPath := filepath.Join(tempDir, transitionIntentFile)
	if _, err := os.Stat(intentPath); err != nil {
		t.Fatalf("intent file must remain after failed clear: %v", err)
	}
	if !q.Stats().LocationUncertain || !q.Stats().DeliveryBlocked {
		t.Fatalf("stats must reflect uncertain and blocked: %+v", q.Stats())
	}

	// Restart: queue must recover intent and remain blocked
	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !q2.Stats().DeliveryBlocked {
		t.Fatalf("restart must remain delivery blocked: %+v", q2.Stats())
	}
	batch2, err2 := q2.ReadBatch(10, 1024*1024)
	if err2 == nil || len(batch2) != 0 {
		t.Fatalf("restarted queue must not deliver records: batch=%+v err=%v", batch2, err2)
	}
}

func TestDurableQueueRollbackDestSourceClearFailures(t *testing.T) {
	t.Run("rollback dest sync failure is uncertain", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("record-1")); err != nil {
			t.Fatal(err)
		}
		corruptRecordFile(t, activeRecordPath(t, tempDir, 1))

		q.syncHook = func(phase, target string) error {
			if phase == syncPhaseQuarantineDest {
				return errors.New("dest sync fail")
			}
			if phase == syncPhaseRollbackDest {
				return errors.New("rollback dest sync fail")
			}
			return nil
		}

		batch, err := q.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
			t.Fatalf("expected ErrQuarantineUncertain, got batch=%+v err=%v", batch, err)
		}
		if !q.Stats().LocationUncertain || !q.Stats().DeliveryBlocked {
			t.Fatalf("queue must be uncertain and delivery blocked: %+v", q.Stats())
		}
		if _, err := os.Lstat(filepath.Join(tempDir, transitionIntentFile)); err != nil {
			t.Fatalf("intent must remain until rollback durability is proven: %v", err)
		}
	})

	t.Run("rollback source sync failure sets location uncertain", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("record-1")); err != nil {
			t.Fatal(err)
		}
		corruptRecordFile(t, activeRecordPath(t, tempDir, 1))

		q.syncHook = func(phase, target string) error {
			if phase == syncPhaseQuarantineDest {
				return errors.New("dest sync fail")
			}
			if phase == syncPhaseRollbackSource {
				return errors.New("rollback source sync fail")
			}
			return nil
		}

		batch, err := q.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
			t.Fatalf("expected ErrQuarantineUncertain, got batch=%+v err=%v", batch, err)
		}
		if !q.Stats().LocationUncertain || !q.Stats().DeliveryBlocked {
			t.Fatalf("queue must be uncertain and delivery blocked: %+v", q.Stats())
		}
		if _, err := os.Lstat(filepath.Join(tempDir, transitionIntentFile)); err != nil {
			t.Fatalf("intent must remain when rollback source sync fails: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(tempDir, uncertainMarkerFile)); err != nil {
			t.Fatalf("uncertain marker must be durable after rollback source sync failure: %v", err)
		}
		if !recordPresent(t, tempDir, "active", 1) {
			t.Fatal("rollback dest durability must leave record in active/")
		}
	})

	t.Run("successful rollback clears intent after durability", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("record-1")); err != nil {
			t.Fatal(err)
		}
		corruptRecordFile(t, activeRecordPath(t, tempDir, 1))

		var sawRollbackDest, sawRollbackSource, sawClearDir bool
		q.syncHook = func(phase, target string) error {
			switch phase {
			case syncPhaseQuarantineDest:
				return errors.New("dest sync fail")
			case syncPhaseRollbackDest:
				sawRollbackDest = true
			case syncPhaseRollbackSource:
				sawRollbackSource = true
			case syncPhaseIntentClearDir:
				sawClearDir = true
			}
			return nil
		}

		batch, err := q.ReadBatch(10, 1024*1024)
		if err == nil || len(batch) != 0 {
			t.Fatalf("expected dest-sync failure after durable rollback, batch=%+v err=%v", batch, err)
		}
		if !sawRollbackDest || !sawRollbackSource || !sawClearDir {
			t.Fatalf("rollback durability phases missing: dest=%v source=%v clear=%v", sawRollbackDest, sawRollbackSource, sawClearDir)
		}
		if _, err := os.Lstat(filepath.Join(tempDir, transitionIntentFile)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("intent must clear after durable rollback, err=%v", err)
		}
		if !recordPresent(t, tempDir, "active", 1) || recordPresent(t, tempDir, "quarantine", 1) {
			t.Fatal("durable rollback must restore active-only copy")
		}
		if q.Stats().LocationUncertain {
			t.Fatalf("successful rollback must not leave location uncertain: %+v", q.Stats())
		}
	})

	t.Run("rollback clear intent failure sets location uncertain", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("record-1")); err != nil {
			t.Fatal(err)
		}
		corruptRecordFile(t, activeRecordPath(t, tempDir, 1))

		q.syncHook = func(phase, target string) error {
			if phase == syncPhaseQuarantineDest {
				return errors.New("dest sync fail")
			}
			return nil
		}
		q.removeHook = func(path string) error {
			if strings.HasSuffix(path, transitionIntentFile) {
				return errors.New("remove intent after rollback fail")
			}
			return nil
		}

		batch, err := q.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
			t.Fatalf("expected ErrQuarantineUncertain, got batch=%+v err=%v", batch, err)
		}
		if !q.Stats().LocationUncertain || !q.Stats().DeliveryBlocked {
			t.Fatalf("queue must be uncertain and delivery blocked: %+v", q.Stats())
		}
		if _, err := os.Lstat(filepath.Join(tempDir, transitionIntentFile)); err != nil {
			t.Fatalf("intent must remain after clear failure: %v", err)
		}
	})
}

func TestDurableQueueRecoverTransitionIntentMatrix(t *testing.T) {
	writeIntent := func(dir, record string, seq uint64, code string) {
		intent := transitionIntent{
			Version:  1,
			Record:   record,
			Sequence: seq,
			Code:     code,
		}
		data, _ := json.Marshal(intent)
		_ = os.WriteFile(filepath.Join(dir, transitionIntentFile), data, 0600)
	}

	t.Run("Case A: intent and quarantine only -> quarantine authoritative, blocked", func(t *testing.T) {
		dir := t.TempDir()
		q1, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q1.Enqueue([]byte("rec-1")); err != nil {
			t.Fatal(err)
		}
		if _, err := q1.Enqueue([]byte("rec-2")); err != nil {
			t.Fatal(err)
		}

		rec1Path := activeRecordPath(t, dir, 1)
		rec1Name := filepath.Base(rec1Path)
		// Move rec1 to quarantine manually
		data, _ := os.ReadFile(rec1Path)
		_ = os.WriteFile(filepath.Join(dir, "quarantine", rec1Name), data, 0600)
		_ = os.Remove(rec1Path)
		writeIntent(dir, rec1Name, 1, "QUEUE_RECORD_CORRUPT")

		q2, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !q2.Stats().DeliveryBlocked || q2.Stats().BlockedFromSequence != 1 {
			t.Fatalf("expected sequence 1 blocked, got stats: %+v", q2.Stats())
		}
		batch, err := q2.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrDeliveryBlocked) || len(batch) != 0 {
			t.Fatalf("expected ErrDeliveryBlocked and empty batch, got batch=%+v err=%v", batch, err)
		}
	})

	t.Run("Case B: intent and active only -> active retained, sequence blocked", func(t *testing.T) {
		dir := t.TempDir()
		q1, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q1.Enqueue([]byte("rec-1")); err != nil {
			t.Fatal(err)
		}
		if _, err := q1.Enqueue([]byte("rec-2")); err != nil {
			t.Fatal(err)
		}

		rec1Path := activeRecordPath(t, dir, 1)
		rec1Name := filepath.Base(rec1Path)
		writeIntent(dir, rec1Name, 1, "QUEUE_RECORD_CORRUPT")

		q2, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !q2.Stats().DeliveryBlocked || q2.Stats().BlockedFromSequence != 1 {
			t.Fatalf("expected sequence 1 blocked, got stats: %+v", q2.Stats())
		}
		batch, err := q2.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrDeliveryBlocked) || len(batch) != 0 {
			t.Fatalf("expected ErrDeliveryBlocked, got batch=%+v err=%v", batch, err)
		}
		if !recordPresent(t, dir, "active", 1) {
			t.Fatal("active record must be retained")
		}
	})

	t.Run("Case C: intent and both exist identical -> quarantine authoritative, active duplicate never delivered", func(t *testing.T) {
		dir := t.TempDir()
		q1, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q1.Enqueue([]byte("rec-1")); err != nil {
			t.Fatal(err)
		}
		rec1Path := activeRecordPath(t, dir, 1)
		rec1Name := filepath.Base(rec1Path)
		data, _ := os.ReadFile(rec1Path)
		_ = os.WriteFile(filepath.Join(dir, "quarantine", rec1Name), data, 0600)
		writeIntent(dir, rec1Name, 1, "QUEUE_RECORD_CORRUPT")

		q2, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !q2.Stats().DeliveryBlocked {
			t.Fatalf("expected delivery blocked, stats: %+v", q2.Stats())
		}
		batch, err := q2.ReadBatch(10, 1024*1024)
		if err == nil {
			for _, r := range batch {
				if r.Sequence == 1 {
					t.Fatal("active duplicate must never be delivered")
				}
			}
		}
		if !errors.Is(err, ErrDeliveryBlocked) && !errors.Is(err, ErrQuarantineUncertain) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Case D: intent and both exist conflicting -> location uncertain", func(t *testing.T) {
		dir := t.TempDir()
		q1, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q1.Enqueue([]byte("rec-1")); err != nil {
			t.Fatal(err)
		}
		rec1Path := activeRecordPath(t, dir, 1)
		rec1Name := filepath.Base(rec1Path)
		_ = os.WriteFile(filepath.Join(dir, "quarantine", rec1Name), []byte(`{"conflicting":true}`), 0600)
		writeIntent(dir, rec1Name, 1, "QUEUE_RECORD_CORRUPT")

		q2, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !q2.Stats().LocationUncertain || !q2.Stats().DeliveryBlocked {
			t.Fatalf("conflicting copies must enter location uncertain: %+v", q2.Stats())
		}
		batch, err := q2.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
			t.Fatalf("expected ErrQuarantineUncertain, got batch=%+v err=%v", batch, err)
		}
	})

	t.Run("Case E: intent and neither exists -> location uncertain", func(t *testing.T) {
		dir := t.TempDir()
		writeIntent(dir, "rec_0000000000000000001_missing.json", 1, "QUEUE_RECORD_CORRUPT")

		q, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !q.Stats().LocationUncertain || !q.Stats().DeliveryBlocked {
			t.Fatalf("missing both copies must enter location uncertain: %+v", q.Stats())
		}
		batch, err := q.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
			t.Fatalf("expected ErrQuarantineUncertain, got batch=%+v err=%v", batch, err)
		}
	})

	t.Run("corrupt intent JSON -> location uncertain", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, transitionIntentFile), []byte(`{not valid json`), 0600)
		q, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !q.Stats().LocationUncertain || !q.Stats().DeliveryBlocked {
			t.Fatalf("corrupt intent must enter location uncertain: %+v", q.Stats())
		}
	})

	t.Run("oversized intent JSON -> location uncertain", func(t *testing.T) {
		dir := t.TempDir()
		oversized := make([]byte, 5000)
		_ = os.WriteFile(filepath.Join(dir, transitionIntentFile), oversized, 0600)
		q, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !q.Stats().LocationUncertain || !q.Stats().DeliveryBlocked {
			t.Fatalf("oversized intent must enter location uncertain: %+v", q.Stats())
		}
	})

	t.Run("empty intent JSON -> location uncertain", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, transitionIntentFile), []byte{}, 0600)
		q, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !q.Stats().LocationUncertain || !q.Stats().DeliveryBlocked {
			t.Fatalf("empty intent must enter location uncertain: %+v", q.Stats())
		}
	})

	t.Run("symlink intent file -> location uncertain", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target.json")
		_ = os.WriteFile(target, []byte(`{"version":1,"record":"rec_0000000000000000001_a.json","sequence":1}`), 0600)
		_ = os.Symlink(target, filepath.Join(dir, transitionIntentFile))
		q, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !q.Stats().LocationUncertain || !q.Stats().DeliveryBlocked {
			t.Fatalf("symlink intent must enter location uncertain: %+v", q.Stats())
		}
	})

	t.Run("intent sequence mismatch -> location uncertain", func(t *testing.T) {
		dir := t.TempDir()
		writeIntent(dir, "rec_0000000000000000001_test.json", 99, "QUEUE_RECORD_CORRUPT")
		q, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !q.Stats().LocationUncertain || !q.Stats().DeliveryBlocked {
			t.Fatalf("sequence mismatch must enter location uncertain: %+v", q.Stats())
		}
	})

	t.Run("intent path traversal -> location uncertain", func(t *testing.T) {
		dir := t.TempDir()
		writeIntent(dir, "../outside.json", 1, "QUEUE_RECORD_CORRUPT")
		q, err := NewDurableQueue(dir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !q.Stats().LocationUncertain || !q.Stats().DeliveryBlocked {
			t.Fatalf("path traversal in intent must enter location uncertain: %+v", q.Stats())
		}
	})
}

func TestDurableQueueFilesystemAmbiguityFailClosed(t *testing.T) {
	t.Run("hasActiveAfterBlockedLocked directory scan failure fails closed", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("rec-1")); err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("rec-2")); err != nil {
			t.Fatal(err)
		}
		corruptRecordFile(t, activeRecordPath(t, tempDir, 1))

		// Trigger quarantine of record 1
		_, _ = q.ReadBatch(10, 1024*1024)

		// Inject readDir error
		q.readDirHook = func(dir string) error {
			return errors.New("simulated dir read error")
		}

		if !q.Stats().DeliveryBlocked {
			t.Fatalf("Stats.DeliveryBlocked must be true on dir scan error: %+v", q.Stats())
		}
		batch, err := q.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrQuarantineUncertain) || len(batch) != 0 {
			t.Fatalf("expected ErrQuarantineUncertain on dir scan error, got batch=%+v err=%v", batch, err)
		}
	})

	t.Run("reconcileDuplicatesLocked lstat error fails closed", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("rec-1")); err != nil {
			t.Fatal(err)
		}
		// Inject lstat error on quarantine check during duplicate reconciliation
		q.lstatHook = func(path string) error {
			if strings.Contains(path, "quarantine") {
				return os.ErrPermission
			}
			return nil
		}
		err = q.reconcileDuplicatesLocked()
		if err == nil {
			t.Fatal("reconcileDuplicatesLocked must fail closed on non-ENOENT stat error")
		}
	})

	t.Run("recoverTransitionIntent non-ENOENT lstat fails closed", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("rec-1")); err != nil {
			t.Fatal(err)
		}
		rec1Path := activeRecordPath(t, tempDir, 1)
		rec1Name := filepath.Base(rec1Path)
		intent := transitionIntent{Version: 1, Record: rec1Name, Sequence: 1, Code: "QUEUE_RECORD_CORRUPT"}
		data, _ := json.Marshal(intent)
		if err := os.WriteFile(filepath.Join(tempDir, transitionIntentFile), data, 0600); err != nil {
			t.Fatal(err)
		}

		q.lstatHook = func(path string) error {
			if path == filepath.Join(tempDir, "active", rec1Name) {
				return os.ErrPermission
			}
			return nil
		}
		seq, hasIntent, _ := q.recoverTransitionIntentLocked()
		if !hasIntent || seq != 1 || !q.locationUncertain {
			t.Fatalf("non-ENOENT active lstat must fail closed: seq=%d hasIntent=%v uncertain=%v", seq, hasIntent, q.locationUncertain)
		}
	})

	t.Run("sole active record at blocked sequence blocks delivery", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Enqueue([]byte("rec-1")); err != nil {
			t.Fatal(err)
		}
		rec1Path := activeRecordPath(t, tempDir, 1)
		rec1Name := filepath.Base(rec1Path)
		raw, _ := os.ReadFile(rec1Path)
		// Put copy in quarantine
		_ = os.WriteFile(filepath.Join(tempDir, "quarantine", rec1Name), raw, 0600)

		// Reopen queue: record 1 exists in active AND quarantine, nothing else in active
		q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !q2.Stats().DeliveryBlocked {
			t.Fatalf("sole active record at blocked sequence must report DeliveryBlocked: %+v", q2.Stats())
		}
		batch, err := q2.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrDeliveryBlocked) || len(batch) != 0 {
			t.Fatalf("expected ErrDeliveryBlocked and no records delivered, got batch=%+v err=%v", batch, err)
		}
	})
}

func TestDurableQueueSymlinkRejection(t *testing.T) {
	t.Run("active duplicate symlink is rejected without reading target", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		// Create a regular record in quarantine
		qRec := filepath.Join(tempDir, "quarantine", "rec_0000000000000000001_target.json")
		_ = os.WriteFile(qRec, []byte(`{"dummy":1}`), 0600)

		// Create a symlink in active/ pointing to some target file
		targetFile := filepath.Join(tempDir, "sensitive.txt")
		_ = os.WriteFile(targetFile, []byte("secret"), 0600)
		aSym := filepath.Join(tempDir, "active", "rec_0000000000000000001_target.json")
		_ = os.Symlink(targetFile, aSym)

		_, dupErr := q.duplicateStateLocked("rec_0000000000000000001_target.json")
		if dupErr == nil || !strings.Contains(dupErr.Error(), "SYMLINK_REJECTED") {
			t.Fatalf("expected symlink rejection error, got %v", dupErr)
		}
	})

	t.Run("quarantine duplicate symlink is rejected", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		targetFile := filepath.Join(tempDir, "sensitive.txt")
		_ = os.WriteFile(targetFile, []byte("secret"), 0600)
		qSym := filepath.Join(tempDir, "quarantine", "rec_0000000000000000001_target.json")
		_ = os.Symlink(targetFile, qSym)

		_, dupErr := q.duplicateStateLocked("rec_0000000000000000001_target.json")
		if dupErr == nil || !strings.Contains(dupErr.Error(), "SYMLINK_REJECTED") {
			t.Fatalf("expected symlink rejection error, got %v", dupErr)
		}
	})

	t.Run("quarantineRecordLocked rejects active symlink", func(t *testing.T) {
		tempDir := t.TempDir()
		q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatal(err)
		}
		targetFile := filepath.Join(tempDir, "target.json")
		_ = os.WriteFile(targetFile, []byte("test"), 0600)
		symName := "rec_0000000000000000001_sym.json"
		_ = os.Symlink(targetFile, filepath.Join(tempDir, "active", symName))

		err = q.quarantineRecordLocked(symName, "QUEUE_RECORD_CORRUPT")
		if err == nil || !strings.Contains(err.Error(), "SYMLINK_REJECTED") {
			t.Fatalf("expected active symlink rejection, got %v", err)
		}
	})
}

func TestDurableQueueJSONMetadataValidity(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("record-1")); err != nil {
		t.Fatal(err)
	}

	corruptRecordFile(t, activeRecordPath(t, tempDir, 1))

	// Trigger quarantine
	_, _ = q.ReadBatch(10, 1024*1024)

	// 1. Verify .reason sidecar is valid JSON
	entries, _ := os.ReadDir(filepath.Join(tempDir, "quarantine"))
	var reasonFound bool
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".reason") {
			reasonFound = true
			data, err := os.ReadFile(filepath.Join(tempDir, "quarantine", e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var sidecar struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(data, &sidecar); err != nil {
				t.Fatalf("reason sidecar must be valid JSON: %v, content=%s", err, string(data))
			}
			if sidecar.Code != "QUEUE_RECORD_CORRUPT" {
				t.Fatalf("unexpected sidecar code: %s", sidecar.Code)
			}
		}
	}
	if !reasonFound {
		t.Fatal("reason sidecar file not found")
	}

	// 2. Verify location.uncertain is valid JSON
	if err := q.enterUncertainLocked(errors.New("test uncertainty")); err == nil {
		t.Fatal("enterUncertainLocked should return error")
	}
	uncData, err := os.ReadFile(filepath.Join(tempDir, uncertainMarkerFile))
	if err != nil {
		t.Fatalf("location.uncertain marker must exist: %v", err)
	}
	var unc struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(uncData, &unc); err != nil {
		t.Fatalf("location.uncertain must be valid JSON: %v, content=%s", err, string(uncData))
	}
	if unc.Code != "QUEUE_QUARANTINE_UNCERTAIN" {
		t.Fatalf("unexpected uncertainty code: %s", unc.Code)
	}

	// 3. Verify quarantine.transition is valid JSON when written
	if err := q.persistTransitionIntentLocked("rec_0000000000000000005_test.json", 5, "QUEUE_RECORD_CORRUPT"); err != nil {
		t.Fatalf("persistTransitionIntentLocked failed: %v", err)
	}
	intentData, err := os.ReadFile(filepath.Join(tempDir, transitionIntentFile))
	if err != nil {
		t.Fatalf("intent file must exist: %v", err)
	}
	var intent struct {
		Version  int    `json:"version"`
		Record   string `json:"record"`
		Sequence uint64 `json:"sequence"`
		Code     string `json:"code"`
	}
	if err := json.Unmarshal(intentData, &intent); err != nil {
		t.Fatalf("quarantine.transition must be valid JSON: %v, content=%s", err, string(intentData))
	}
	if intent.Version != 1 || intent.Sequence != 5 || intent.Record != "rec_0000000000000000005_test.json" || intent.Code != "QUEUE_RECORD_CORRUPT" {
		t.Fatalf("unexpected intent content: %+v", intent)
	}
}

func TestDurableQueueCrossBatchGapBlockedAndSurvivesRestart(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{"one", "two", "three"} {
		if _, err := q.Enqueue([]byte(payload)); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 3 {
		t.Fatalf("expected initial batch of 3, got %+v err=%v", batch, err)
	}
	if err := q.CommitAccepted(batch[:1], 1); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(activeRecordPath(t, tempDir, 2)); err != nil {
		t.Fatal(err)
	}

	batch, err = q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrSequenceGap) || len(batch) != 0 {
		t.Fatalf("expected QUEUE_SEQUENCE_GAP after missing next sequence, batch=%+v err=%v", batch, err)
	}
	if !recordPresent(t, tempDir, "active", 3) {
		t.Fatal("later sequence must remain on disk")
	}

	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrSequenceGap) || len(batch) != 0 {
		t.Fatalf("gap must survive restart, batch=%+v err=%v", batch, err)
	}
	if !q2.Stats().SequenceGapBlock || !q2.Stats().DeliveryBlocked {
		t.Fatalf("stats must report gap block: %+v", q2.Stats())
	}
	if !recordPresent(t, tempDir, "active", 3) {
		t.Fatal("later sequence must never be submitted or deleted")
	}
}

func TestDurableQueueArbitraryInitialSequenceWithoutAcceptedBoundary(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "quarantine"), 0700); err != nil {
		t.Fatal(err)
	}
	writeActiveRecord(t, tempDir, 42, "rec-initial", []byte("first-allowed"))
	if err := os.WriteFile(filepath.Join(tempDir, sequenceMetaFile), []byte(`{"highestSequence":42}`), 0600); err != nil {
		t.Fatal(err)
	}

	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if q.Stats().AcceptedThrough != 0 {
		t.Fatalf("legacy meta must leave acceptedThrough unset, got %d", q.Stats().AcceptedThrough)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 1 || batch[0].Sequence != 42 {
		t.Fatalf("first sequence 42 must remain legal without accepted boundary, got %+v", batch)
	}
}

func TestDurableQueueDuplicateSequenceBlocksBeforeDelivery(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := q.Enqueue([]byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	writeActiveRecord(t, tempDir, rec.Sequence, "rec-conflict-B", []byte("bravo"))

	batch, err := q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrSequenceConflict) || len(batch) != 0 {
		t.Fatalf("expected QUEUE_SEQUENCE_CONFLICT before delivery, batch=%+v err=%v", batch, err)
	}
	entries, err := os.ReadDir(filepath.Join(tempDir, "active"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, e := range entries {
		if isRecordJSON(e.Name()) {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("both conflicting records must remain, count=%d", count)
	}

	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrSequenceConflict) || len(batch) != 0 {
		t.Fatalf("conflict must survive restart, batch=%+v err=%v", batch, err)
	}
	if !q2.Stats().SequenceConflictBlock {
		t.Fatalf("stats must report conflict block: %+v", q2.Stats())
	}
}

func TestDurableQueueAfterReadDuplicateInsertFailsClosed(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := q.Enqueue([]byte("only"))
	if err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 1 {
		t.Fatalf("expected one record, got %+v err=%v", batch, err)
	}
	writeActiveRecord(t, tempDir, rec.Sequence, "rec-after-read", []byte("intruder"))

	if err := q.CommitAccepted(batch, rec.Sequence); err == nil || !errors.Is(err, ErrSequenceConflict) {
		t.Fatalf("expected conflict on ACK commit after insertion, got %v", err)
	}
	if q.Stats().AcceptedThrough != 0 {
		t.Fatalf("acceptedThrough must not advance, got %d", q.Stats().AcceptedThrough)
	}
	entries, _ := os.ReadDir(filepath.Join(tempDir, "active"))
	n := 0
	for _, e := range entries {
		if isRecordJSON(e.Name()) {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("both records must remain after failed commit, count=%d", n)
	}
}

func TestDurableQueuePartialAckDeletesExactPrefixOnly(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"n", "n1", "n2"} {
		if _, err := q.Enqueue([]byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 3 {
		t.Fatalf("expected batch of 3, got %+v err=%v", batch, err)
	}
	if err := q.CommitAccepted(batch, 2); err != nil {
		t.Fatal(err)
	}
	if q.Stats().AcceptedThrough != 2 {
		t.Fatalf("acceptedThrough=%d", q.Stats().AcceptedThrough)
	}
	if recordPresent(t, tempDir, "active", 1) || recordPresent(t, tempDir, "active", 2) {
		t.Fatal("ACKed prefix must be deleted")
	}
	if !recordPresent(t, tempDir, "active", 3) {
		t.Fatal("N+2 must remain active")
	}
	batch, err = q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 1 || batch[0].Sequence != 3 {
		t.Fatalf("expected only sequence 3, got %+v err=%v", batch, err)
	}
}

func TestDurableQueueMetaPersistFailureDeletesNothing(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("b")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	q.syncHook = func(phase, target string) error {
		if phase == syncPhaseMetaFile {
			return errors.New("injected meta sync fail")
		}
		return nil
	}
	if err := q.CommitAccepted(batch[:1], 1); err == nil {
		t.Fatal("expected meta persistence failure")
	}
	if q.Stats().AcceptedThrough != 0 {
		t.Fatalf("accepted boundary must remain unchanged, got %d", q.Stats().AcceptedThrough)
	}
	if !recordPresent(t, tempDir, "active", 1) || !recordPresent(t, tempDir, "active", 2) {
		t.Fatal("no ACKed file may be deleted when meta persistence fails")
	}

	q.syncHook = nil
	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if q2.Stats().AcceptedThrough != 0 {
		t.Fatalf("restart must keep unset accepted boundary, got %d", q2.Stats().AcceptedThrough)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 2 {
		t.Fatalf("restart must remain safe with full queue, got %+v err=%v", batch, err)
	}
}

func TestDurableQueuePostMetaDeleteFailureReconcilesOnRestart(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a", "b", "c"} {
		if _, err := q.Enqueue([]byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 3 {
		t.Fatalf("expected 3 records, got %+v err=%v", batch, err)
	}
	q.removeHook = func(path string) error {
		if strings.Contains(path, string(filepath.Separator)+"active"+string(filepath.Separator)) && strings.HasSuffix(path, ".json") {
			return errors.New("injected active delete fail")
		}
		return nil
	}
	if err := q.CommitAccepted(batch[:2], 2); err == nil {
		t.Fatal("expected post-meta delete failure")
	}
	if q.Stats().AcceptedThrough != 2 {
		t.Fatalf("acceptedThrough must be durable after meta success, got %d", q.Stats().AcceptedThrough)
	}
	if !recordPresent(t, tempDir, "active", 1) || !recordPresent(t, tempDir, "active", 2) {
		t.Fatal("failed delete must leave ACKed leftovers for restart reconciliation")
	}
	if !recordPresent(t, tempDir, "active", 3) {
		t.Fatal("unacknowledged record must remain")
	}

	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if q2.Stats().AcceptedThrough != 2 {
		t.Fatalf("restart must load durable acceptedThrough=2, got %d", q2.Stats().AcceptedThrough)
	}
	if recordPresent(t, tempDir, "active", 1) || recordPresent(t, tempDir, "active", 2) {
		t.Fatal("restart must reconcile ACKed leftovers")
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 1 || batch[0].Sequence != 3 {
		t.Fatalf("no gap bypass; expected only seq 3, got %+v err=%v", batch, err)
	}
}

func TestDurableQueueLegacyMetaLoadsWithoutAcceptedBoundary(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "quarantine"), 0700); err != nil {
		t.Fatal(err)
	}
	writeActiveRecord(t, tempDir, 7, "legacy-rec", []byte("legacy"))
	if err := os.WriteFile(filepath.Join(tempDir, sequenceMetaFile), []byte(`{"highestSequence":7}`), 0600); err != nil {
		t.Fatal(err)
	}
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if q.Stats().AcceptedThrough != 0 || q.Stats().LastSequence != 7 {
		t.Fatalf("legacy meta load failed: %+v", q.Stats())
	}
}

func readSequenceMetaFile(t *testing.T, root string) sequenceMeta {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, sequenceMetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var meta sequenceMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	return meta
}

// A: after ACK meta durable + delete fail, same-seq replacement must not be deleted on restart.
func TestDurableQueueRestartSameSeqReplacementFailsClosed(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := q.Enqueue([]byte("original-A"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("keep-later")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) < 1 {
		t.Fatalf("expected batch, got %+v err=%v", batch, err)
	}
	authorizedID := batch[0].ID
	q.removeHook = func(path string) error {
		if strings.Contains(path, string(filepath.Separator)+"active"+string(filepath.Separator)) && strings.HasSuffix(path, ".json") {
			return errors.New("injected active delete fail")
		}
		return nil
	}
	if err := q.CommitAccepted(batch[:1], rec.Sequence); err == nil {
		t.Fatal("expected post-meta delete failure")
	}
	if q.Stats().AcceptedThrough != rec.Sequence {
		t.Fatalf("acceptedThrough must be durable, got %d", q.Stats().AcceptedThrough)
	}
	originalPath := activeRecordPath(t, tempDir, rec.Sequence)
	if err := os.Remove(originalPath); err != nil {
		t.Fatal(err)
	}
	writeActiveRecord(t, tempDir, rec.Sequence, "rec-replacement-B", []byte("replacement-B"))

	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !recordPresent(t, tempDir, "active", rec.Sequence) {
		t.Fatal("same-seq replacement must not be deleted on restart")
	}
	entries, err := os.ReadDir(filepath.Join(tempDir, "active"))
	if err != nil {
		t.Fatal(err)
	}
	foundReplacement := false
	for _, e := range entries {
		if e.Name() == recordFileName(rec.Sequence, "rec-replacement-B") {
			foundReplacement = true
		}
	}
	if !foundReplacement {
		t.Fatal("replacement record B must remain")
	}
	if q2.Stats().AcceptedThrough != rec.Sequence {
		t.Fatalf("acceptedThrough must remain %d, got %d", rec.Sequence, q2.Stats().AcceptedThrough)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrAckRecoveryConflict) || len(batch) != 0 {
		t.Fatalf("expected QUEUE_ACK_RECOVERY_CONFLICT with empty batch, got batch=%+v err=%v", batch, err)
	}
	stats := q2.Stats()
	if !stats.DeliveryBlocked || !stats.AckRecoveryConflictBlock {
		t.Fatalf("stats must be delivery-blocked with ack recovery conflict: %+v", stats)
	}
	meta := readSequenceMetaFile(t, tempDir)
	if len(meta.AcceptedRecords) != 1 || meta.AcceptedRecords[0].ID != authorizedID || meta.AcceptedRecords[0].Sequence != rec.Sequence {
		t.Fatalf("acceptedRecords auth must not be cleared, got %+v", meta.AcceptedRecords)
	}

	q3, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = q3.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrAckRecoveryConflict) || len(batch) != 0 {
		t.Fatalf("second restart must remain blocked, batch=%+v err=%v", batch, err)
	}
	if !recordPresent(t, tempDir, "active", rec.Sequence) {
		t.Fatal("replacement B must never be deleted")
	}
	meta = readSequenceMetaFile(t, tempDir)
	if len(meta.AcceptedRecords) != 1 || meta.AcceptedRecords[0].ID != authorizedID {
		t.Fatalf("auth metadata must survive second restart, got %+v", meta.AcceptedRecords)
	}
}

func TestDurableQueueUnauthorizedOldSequenceFailsClosed(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "quarantine"), 0700); err != nil {
		t.Fatal(err)
	}
	writeActiveRecord(t, tempDir, 1, "unexpected-old", []byte("unauthorized"))
	writeActiveRecord(t, tempDir, 2, "later", []byte("later"))
	metaPayload := `{"highestSequence":2,"acceptedThrough":1}`
	if err := os.WriteFile(filepath.Join(tempDir, sequenceMetaFile), []byte(metaPayload), 0600); err != nil {
		t.Fatal(err)
	}

	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !recordPresent(t, tempDir, "active", 1) {
		t.Fatal("unauthorized old-sequence record must be retained")
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrAckRecoveryConflict) || len(batch) != 0 {
		t.Fatalf("expected QUEUE_ACK_RECOVERY_CONFLICT, got batch=%+v err=%v", batch, err)
	}
	stats := q.Stats()
	if !stats.DeliveryBlocked || !stats.AckRecoveryConflictBlock || stats.AcceptedThrough != 1 {
		t.Fatalf("must fail closed with acceptedThrough unchanged: %+v", stats)
	}
	meta := readSequenceMetaFile(t, tempDir)
	if meta.AcceptedThrough != 1 || len(meta.AcceptedRecords) != 0 {
		t.Fatalf("meta must remain acceptedThrough=1 without fabricated auth: %+v", meta)
	}
	if !recordPresent(t, tempDir, "active", 1) || !recordPresent(t, tempDir, "active", 2) {
		t.Fatal("neither record may be deleted")
	}

	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrAckRecoveryConflict) || len(batch) != 0 {
		t.Fatalf("restart must remain blocked, batch=%+v err=%v", batch, err)
	}
}

// B: lower-sequence insert between Read and Commit is rejected before meta advance.
func TestDurableQueueLowerSeqInsertBeforeCommitFailsClosed(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "quarantine"), 0700); err != nil {
		t.Fatal(err)
	}
	writeActiveRecord(t, tempDir, 5, "rec-five", []byte("five"))
	if err := os.WriteFile(filepath.Join(tempDir, sequenceMetaFile), []byte(`{"highestSequence":5}`), 0600); err != nil {
		t.Fatal(err)
	}
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 1 || batch[0].Sequence != 5 {
		t.Fatalf("expected seq 5, got %+v err=%v", batch, err)
	}
	writeActiveRecord(t, tempDir, 3, "rec-lower-insert", []byte("lower"))

	if err := q.CommitAccepted(batch, 5); err == nil {
		t.Fatal("expected commit rejection for lower-seq insert not in ACK prefix")
	}
	if q.Stats().AcceptedThrough != 0 {
		t.Fatalf("acceptedThrough must not advance, got %d", q.Stats().AcceptedThrough)
	}
	if !recordPresent(t, tempDir, "active", 5) || !recordPresent(t, tempDir, "active", 3) {
		t.Fatal("commit must delete nothing on rejection")
	}

	// Also when acceptedThrough was already nonzero.
	tempDir2 := t.TempDir()
	q2, err := NewDurableQueue(tempDir2, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a", "b", "c"} {
		if _, err := q2.Enqueue([]byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 3 {
		t.Fatalf("expected 3, got %+v err=%v", batch, err)
	}
	if err := q2.CommitAccepted(batch[:1], 1); err != nil {
		t.Fatal(err)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) < 1 {
		t.Fatalf("expected next batch, got %+v err=%v", batch, err)
	}
	writeActiveRecord(t, tempDir2, 1, "rec-post-ack-intruder", []byte("sneak"))
	if err := q2.CommitAccepted(batch[:1], batch[0].Sequence); err == nil {
		t.Fatal("expected rejection of active seq <= proposed acceptedThrough not in prefix")
	}
	if q2.Stats().AcceptedThrough != 1 {
		t.Fatalf("acceptedThrough must stay at 1, got %d", q2.Stats().AcceptedThrough)
	}
	if recordPresent(t, tempDir2, "active", 2) == false {
		t.Fatal("unacked records must remain after rejected commit")
	}
}

// C: meta dirsync failure after rename is QUEUE_META_UNCERTAIN; no stale overwrite.
func TestDurableQueueMetaDirSyncAfterRenameFailsClosed(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("b")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	q.syncHook = func(phase, target string) error {
		if phase == syncPhaseMetaDir {
			return errors.New("injected meta dir sync fail")
		}
		return nil
	}
	err = q.CommitAccepted(batch[:1], 1)
	if err == nil || !errors.Is(err, ErrMetaUncertain) {
		t.Fatalf("expected QUEUE_META_UNCERTAIN, got %v", err)
	}
	if !q.Stats().MetaUncertain || !q.Stats().DeliveryBlocked {
		t.Fatalf("stats must report meta uncertain / delivery blocked: %+v", q.Stats())
	}
	if q.Stats().AcceptedThrough != 1 {
		t.Fatalf("in-memory acceptedThrough must adopt durable/intended value, got %d", q.Stats().AcceptedThrough)
	}
	if _, err := q.Enqueue([]byte("c")); !errors.Is(err, ErrMetaUncertain) {
		t.Fatalf("Enqueue must fail closed while meta uncertain, got %v", err)
	}
	if _, err := q.ReadBatch(10, 1024*1024); !errors.Is(err, ErrMetaUncertain) {
		t.Fatalf("ReadBatch must fail closed while meta uncertain, got %v", err)
	}
	if err := q.CommitAccepted(batch[:1], 1); !errors.Is(err, ErrMetaUncertain) {
		t.Fatalf("CommitAccepted must fail closed while meta uncertain, got %v", err)
	}
	// Disk meta must not be overwritten with a regressing acceptedThrough.
	meta := readSequenceMetaFile(t, tempDir)
	if meta.AcceptedThrough != 1 {
		t.Fatalf("disk acceptedThrough must remain 1, got %d", meta.AcceptedThrough)
	}
}

// D: missing entire unacked tail is QUEUE_SEQUENCE_GAP (empty active with hole).
func TestDurableQueueMissingUnackedTailIsSequenceGap(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a", "b", "c"} {
		if _, err := q.Enqueue([]byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 3 {
		t.Fatalf("expected 3, got %+v err=%v", batch, err)
	}
	if err := q.CommitAccepted(batch[:1], 1); err != nil {
		t.Fatal(err)
	}
	for _, seq := range []uint64{2, 3} {
		if err := os.Remove(activeRecordPath(t, tempDir, seq)); err != nil {
			t.Fatal(err)
		}
	}
	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrSequenceGap) || len(batch) != 0 {
		t.Fatalf("empty active with unacked highestSequence must be QUEUE_SEQUENCE_GAP, got %+v err=%v", batch, err)
	}
	if !q2.Stats().SequenceGapBlock || !q2.Stats().DeliveryBlocked {
		t.Fatalf("stats must report gap block: %+v", q2.Stats())
	}
	if q2.Stats().LastSequence != 3 || q2.Stats().AcceptedThrough != 1 {
		t.Fatalf("lineage must remain highest=3 accepted=1, got %+v", q2.Stats())
	}
}

// E: authorized leftover exact ID deletes on restart.
func TestDurableQueueAuthorizedLeftoverExactIDDeletesOnRestart(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a", "b", "c"} {
		if _, err := q.Enqueue([]byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 3 {
		t.Fatalf("expected 3, got %+v err=%v", batch, err)
	}
	authorizedID := batch[0].ID
	q.removeHook = func(path string) error {
		if strings.Contains(path, string(filepath.Separator)+"active"+string(filepath.Separator)) && strings.HasSuffix(path, ".json") {
			return errors.New("injected active delete fail")
		}
		return nil
	}
	if err := q.CommitAccepted(batch[:1], 1); err == nil {
		t.Fatal("expected delete failure after meta")
	}
	meta := readSequenceMetaFile(t, tempDir)
	if len(meta.AcceptedRecords) != 1 || meta.AcceptedRecords[0].ID != authorizedID || meta.AcceptedRecords[0].Sequence != 1 {
		t.Fatalf("meta must retain exact ACK auth leftover, got %+v", meta.AcceptedRecords)
	}

	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if recordPresent(t, tempDir, "active", 1) {
		t.Fatal("exact authorized leftover must be deleted on restart")
	}
	if !recordPresent(t, tempDir, "active", 2) || !recordPresent(t, tempDir, "active", 3) {
		t.Fatal("unacked records must remain")
	}
	meta = readSequenceMetaFile(t, tempDir)
	if len(meta.AcceptedRecords) != 0 {
		t.Fatalf("acceptedRecords must clear after successful restart reconcile, got %+v", meta.AcceptedRecords)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 2 || batch[0].Sequence != 2 {
		t.Fatalf("expected seq 2..3 after reconcile, got %+v err=%v", batch, err)
	}
}

// F: auth list cleared after successful commit.
func TestDurableQueueAcceptedRecordsClearedAfterSuccess(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("only")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 1 {
		t.Fatalf("expected 1, got %+v err=%v", batch, err)
	}
	if err := q.CommitAccepted(batch, 1); err != nil {
		t.Fatal(err)
	}
	meta := readSequenceMetaFile(t, tempDir)
	if meta.AcceptedThrough != 1 {
		t.Fatalf("acceptedThrough=%d", meta.AcceptedThrough)
	}
	if len(meta.AcceptedRecords) != 0 {
		t.Fatalf("acceptedRecords must be empty after successful delete, got %+v", meta.AcceptedRecords)
	}
	if q.Len() != 0 {
		t.Fatalf("queue must be empty, len=%d", q.Len())
	}
}

// G: legacy meta (covered also by TestDurableQueueLegacyMetaLoadsWithoutAcceptedBoundary).
func TestDurableQueueLegacyMetaAcceptedRecordsAbsent(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "quarantine"), 0700); err != nil {
		t.Fatal(err)
	}
	writeActiveRecord(t, tempDir, 9, "legacy-only", []byte("legacy-payload"))
	if err := os.WriteFile(filepath.Join(tempDir, sequenceMetaFile), []byte(`{"highestSequence":9}`), 0600); err != nil {
		t.Fatal(err)
	}
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if q.Stats().AcceptedThrough != 0 || q.Stats().LastSequence != 9 {
		t.Fatalf("legacy meta load failed: %+v", q.Stats())
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 1 || batch[0].Sequence != 9 {
		t.Fatalf("legacy queue must deliver seq 9, got %+v err=%v", batch, err)
	}
}

// H: acceptedThrough never regresses across meta uncertain / subsequent writes.
func TestDurableQueueAcceptedThroughNeverRegresses(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("b")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CommitAccepted(batch[:1], 1); err != nil {
		t.Fatal(err)
	}
	if err := q.persistSequenceMetaLocked(q.lastSeq, 0, nil); err == nil {
		t.Fatal("persist must reject acceptedThrough regression")
	}
	if q.Stats().AcceptedThrough != 1 {
		t.Fatalf("acceptedThrough must stay 1, got %d", q.Stats().AcceptedThrough)
	}
	meta := readSequenceMetaFile(t, tempDir)
	if meta.AcceptedThrough != 1 {
		t.Fatalf("disk acceptedThrough must stay 1, got %d", meta.AcceptedThrough)
	}

	q.syncHook = func(phase, target string) error {
		if phase == syncPhaseMetaDir {
			return errors.New("injected meta dir sync fail")
		}
		return nil
	}
	batch, err = q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	_ = q.CommitAccepted(batch[:1], 2)
	if q.Stats().AcceptedThrough < 1 {
		t.Fatalf("acceptedThrough must never regress below 1, got %d", q.Stats().AcceptedThrough)
	}
	meta = readSequenceMetaFile(t, tempDir)
	if meta.AcceptedThrough < 1 {
		t.Fatalf("disk acceptedThrough must never regress below 1, got %d", meta.AcceptedThrough)
	}
}

func TestDurableQueueQuarantineMustNotMaskMissingTail(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"one", "two", "three"} {
		if _, err := q.Enqueue([]byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	corruptRecordFile(t, activeRecordPath(t, tempDir, 1))
	if _, err := q.ReadBatch(10, 1024*1024); err != nil && !errors.Is(err, ErrDeliveryBlocked) && !errors.Is(err, ErrSequenceGap) {
		// quarantine of seq1 may leave later actives blocked or readable depending on path
		if q.Stats().QuarantineCount == 0 {
			t.Fatalf("expected sequence 1 quarantined, err=%v stats=%+v", err, q.Stats())
		}
	}
	if q.Stats().QuarantineCount != 1 {
		t.Fatalf("expected sequence 1 quarantined, stats=%+v", q.Stats())
	}
	if err := os.Remove(activeRecordPath(t, tempDir, 2)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(activeRecordPath(t, tempDir, 3)); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		qi, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatalf("reopen %d: %v", i, err)
		}
		batch, err := qi.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrSequenceGap) || len(batch) != 0 {
			t.Fatalf("reopen %d: expected QUEUE_SEQUENCE_GAP empty batch, got batch=%+v err=%v", i, batch, err)
		}
		stats := qi.Stats()
		if !stats.SequenceGapBlock || !stats.DeliveryBlocked {
			t.Fatalf("reopen %d: must be gap-blocked, stats=%+v", i, stats)
		}
		if stats.QuarantineCount != 1 || !recordPresent(t, tempDir, "quarantine", 1) {
			t.Fatalf("reopen %d: sequence 1 quarantine must remain", i)
		}
		if stats.AcceptedThrough != 0 {
			t.Fatalf("reopen %d: acceptedThrough must stay 0, got %d", i, stats.AcceptedThrough)
		}
	}
}

func TestDurableQueueArbitraryInitialLineageValid(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "quarantine"), 0700); err != nil {
		t.Fatal(err)
	}
	writeActiveRecord(t, tempDir, 42, "rec-42", []byte("start"))
	if err := os.WriteFile(filepath.Join(tempDir, sequenceMetaFile), []byte(`{"highestSequence":42}`), 0600); err != nil {
		t.Fatal(err)
	}
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 1 || batch[0].Sequence != 42 {
		t.Fatalf("seq 42 alone must be valid, got %+v err=%v", batch, err)
	}

	tempDir2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir2, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir2, "quarantine"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, seq := range []uint64{42, 43, 44} {
		writeActiveRecord(t, tempDir2, seq, fmt.Sprintf("rec-%d", seq), []byte("x"))
	}
	if err := os.WriteFile(filepath.Join(tempDir2, sequenceMetaFile), []byte(`{"highestSequence":44}`), 0600); err != nil {
		t.Fatal(err)
	}
	q2, err := NewDurableQueue(tempDir2, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 3 || batch[0].Sequence != 42 {
		t.Fatalf("contiguous 42..44 must be valid, got %+v err=%v", batch, err)
	}
}

func TestDurableQueueInitialLineageInteriorGap(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "quarantine"), 0700); err != nil {
		t.Fatal(err)
	}
	writeActiveRecord(t, tempDir, 42, "rec-42", []byte("a"))
	writeActiveRecord(t, tempDir, 44, "rec-44", []byte("c"))
	if err := os.WriteFile(filepath.Join(tempDir, sequenceMetaFile), []byte(`{"highestSequence":44}`), 0600); err != nil {
		t.Fatal(err)
	}
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrSequenceGap) || len(batch) != 0 {
		t.Fatalf("missing 43 must QUEUE_SEQUENCE_GAP, got batch=%+v err=%v", batch, err)
	}
	if !q.Stats().SequenceGapBlock || !q.Stats().DeliveryBlocked {
		t.Fatalf("stats must report gap block: %+v", q.Stats())
	}
}

func TestDurableQueueActiveDirSyncFailurePreservesOldAuthorization(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 2 {
		t.Fatalf("expected 2, got %+v err=%v", batch, err)
	}
	authID := batch[0].ID
	q.syncHook = func(phase, target string) error {
		if phase == syncPhaseActiveCommit {
			return errors.New("injected active commit sync fail")
		}
		return nil
	}
	if err := q.CommitAccepted(batch[:1], 1); err == nil {
		t.Fatal("expected active dirsync failure")
	}
	meta := readSequenceMetaFile(t, tempDir)
	if meta.AcceptedThrough != 1 || len(meta.AcceptedRecords) != 1 || meta.AcceptedRecords[0].ID != authID {
		t.Fatalf("seq1 auth must remain pending after dirsync failure, got %+v", meta)
	}

	batch2, err := q.ReadBatch(10, 1024*1024)
	if err == nil {
		// If ReadBatch hit pending cleanup fsync fail, err is set; otherwise empty conflict.
		_ = batch2
	}
	if err := q.CommitAccepted([]Record{{Sequence: 2, ID: batch[1].ID, Payload: []byte("two"), Checksum: batch[1].Checksum, CreatedAt: batch[1].CreatedAt}}, 2); err == nil {
		t.Fatal("seq2 ACK must not succeed while seq1 cleanup pending and dirsync still failing")
	}
	meta = readSequenceMetaFile(t, tempDir)
	if len(meta.AcceptedRecords) != 1 || meta.AcceptedRecords[0].Sequence != 1 || meta.AcceptedRecords[0].ID != authID {
		t.Fatalf("seq2 must not overwrite seq1 auth, got %+v", meta.AcceptedRecords)
	}
	if meta.AcceptedThrough != 1 {
		t.Fatalf("acceptedThrough must stay 1, got %d", meta.AcceptedThrough)
	}
}

func TestDurableQueueRetryAfterActiveDirSyncFailure(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 2 {
		t.Fatalf("expected 2, got %+v err=%v", batch, err)
	}
	auth1 := batch[0].ID
	failActive := true
	clearedSeq1 := false
	q.syncHook = func(phase, target string) error {
		if phase == syncPhaseActiveCommit && failActive {
			return errors.New("injected active commit sync fail")
		}
		return nil
	}
	if err := q.CommitAccepted(batch[:1], 1); err == nil {
		t.Fatal("expected first commit dirsync failure")
	}
	meta := readSequenceMetaFile(t, tempDir)
	if len(meta.AcceptedRecords) != 1 || meta.AcceptedRecords[0].ID != auth1 {
		t.Fatalf("pending seq1 auth required, got %+v", meta.AcceptedRecords)
	}

	failActive = false
	q.syncHook = func(phase, target string) error {
		if phase == syncPhaseActiveCommit {
			// After pending cleanup clears seq1, meta clear happens; then seq2 commit will sync again.
			return nil
		}
		if phase == syncPhaseMetaFile || phase == syncPhaseMetaDir {
			m := readSequenceMetaFile(t, tempDir)
			if len(m.AcceptedRecords) == 0 && m.AcceptedThrough >= 1 {
				clearedSeq1 = true
			}
		}
		return nil
	}
	batch2, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	meta = readSequenceMetaFile(t, tempDir)
	if len(meta.AcceptedRecords) != 0 {
		t.Fatalf("seq1 pending auth must clear before later delivery, got %+v", meta.AcceptedRecords)
	}
	if len(batch2) == 0 || batch2[0].Sequence != 2 {
		t.Fatalf("expected seq2 after cleanup, got %+v", batch2)
	}
	if err := q.CommitAccepted(batch2[:1], 2); err != nil {
		t.Fatal(err)
	}
	meta = readSequenceMetaFile(t, tempDir)
	if meta.AcceptedThrough != 2 {
		t.Fatalf("acceptedThrough must advance to 2, got %d", meta.AcceptedThrough)
	}
	if len(meta.AcceptedRecords) != 0 {
		t.Fatalf("seq2 auth must clear after successful commit, got %+v", meta.AcceptedRecords)
	}
	_ = clearedSeq1
}

func TestDurableQueueAbsentAuthorizedFileRequiresActiveDirSync(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 2 {
		t.Fatalf("expected 2, got %+v err=%v", batch, err)
	}
	authID := batch[0].ID
	q.removeHook = func(path string) error {
		if strings.Contains(path, string(filepath.Separator)+"active"+string(filepath.Separator)) && strings.HasSuffix(path, ".json") {
			return errors.New("injected delete fail")
		}
		return nil
	}
	if err := q.CommitAccepted(batch[:1], 1); err == nil {
		t.Fatal("expected delete failure")
	}
	if err := os.Remove(activeRecordPath(t, tempDir, 1)); err != nil {
		t.Fatal(err)
	}
	meta := readSequenceMetaFile(t, tempDir)
	if len(meta.AcceptedRecords) != 1 || meta.AcceptedRecords[0].ID != authID {
		t.Fatalf("pending auth required, got %+v", meta.AcceptedRecords)
	}

	fsyncSeen := 0
	failSync := true
	q.removeHook = nil
	q.syncHook = func(phase, target string) error {
		if phase == syncPhaseActiveCommit {
			fsyncSeen++
			if failSync {
				return errors.New("injected recover active sync fail")
			}
		}
		return nil
	}
	if _, err := q.ReadBatch(10, 1024*1024); !errors.Is(err, ErrPendingAckCleanup) && err == nil {
		// reconcile returns wrapped fsync error
		if err == nil {
			t.Fatal("expected pending cleanup failure while active fsync fails")
		}
	}
	meta = readSequenceMetaFile(t, tempDir)
	if len(meta.AcceptedRecords) != 1 {
		t.Fatalf("acceptedRecords must not clear without durable active fsync, got %+v", meta.AcceptedRecords)
	}
	if fsyncSeen == 0 {
		t.Fatal("active dir fsync must be attempted even when authorized file is absent")
	}

	failSync = false
	fsyncSeen = 0
	batch2, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if fsyncSeen == 0 {
		t.Fatal("successful recovery must fsync active dir with zero deletions")
	}
	meta = readSequenceMetaFile(t, tempDir)
	if len(meta.AcceptedRecords) != 0 {
		t.Fatalf("acceptedRecords must clear after durable fsync, got %+v", meta.AcceptedRecords)
	}
	if len(batch2) == 0 || batch2[0].Sequence != 2 {
		t.Fatalf("later delivery may continue, got %+v", batch2)
	}
}

func TestDurableQueueTwoConsecutiveFailedCleanupsPreserveAuth(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 2 {
		t.Fatalf("expected 2, got %+v err=%v", batch, err)
	}
	authID := batch[0].ID
	q.syncHook = func(phase, target string) error {
		if phase == syncPhaseActiveCommit {
			return errors.New("injected active commit sync fail")
		}
		return nil
	}
	if err := q.CommitAccepted(batch[:1], 1); err == nil {
		t.Fatal("expected first cleanup fsync failure")
	}
	for i := 0; i < 2; i++ {
		if err := q.CommitAccepted([]Record{batch[1]}, 2); err == nil {
			t.Fatalf("attempt %d: seq2 ACK must fail while seq1 cleanup pending", i)
		}
		meta := readSequenceMetaFile(t, tempDir)
		if meta.AcceptedThrough != 1 || len(meta.AcceptedRecords) != 1 || meta.AcceptedRecords[0].ID != authID || meta.AcceptedRecords[0].Sequence != 1 {
			t.Fatalf("attempt %d: auth must remain exactly seq1, got through=%d records=%+v", i, meta.AcceptedThrough, meta.AcceptedRecords)
		}
	}
}

func writeQuarantineRecord(t *testing.T, root string, seq uint64, id string, payload []byte) string {
	t.Helper()
	now := time.Now().UTC()
	checksum := computeChecksum(seq, id, now, payload)
	rec := Record{
		Sequence:  seq,
		ID:        id,
		CreatedAt: now,
		Checksum:  checksum,
		Payload:   payload,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	name := recordFileName(seq, id)
	path := filepath.Join(root, "quarantine", name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func writeCorruptActiveNamed(t *testing.T, root string, seq uint64, id string) string {
	t.Helper()
	name := recordFileName(seq, id)
	path := filepath.Join(root, "active", name)
	// Valid filename encoding a different ID, but corrupt payload/checksum.
	body := fmt.Sprintf(`{"sequence":%d,"id":%q,"checksum":"deadbeef","payload":"dGVzdA=="}`, seq, id)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

// A: first-record loss with durable lineageStart stays QUEUE_SEQUENCE_GAP across restarts.
func TestDurableQueueFirstRecordLossBlocksWithLineageStart(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"one", "two", "three"} {
		if _, err := q.Enqueue([]byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	meta := readSequenceMetaFile(t, tempDir)
	if meta.Version != sequenceMetaVersion2 || meta.LineageStart != 1 || meta.HighestSequence != 3 {
		t.Fatalf("expected v2 lineageStart=1 highest=3, got %+v", meta)
	}
	if q.Stats().LineageStart != 1 {
		t.Fatalf("stats lineageStart must be 1, got %d", q.Stats().LineageStart)
	}
	if err := os.Remove(activeRecordPath(t, tempDir, 1)); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		qi, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
		if err != nil {
			t.Fatalf("reopen %d: %v", i, err)
		}
		batch, err := qi.ReadBatch(10, 1024*1024)
		if !errors.Is(err, ErrSequenceGap) || len(batch) != 0 {
			t.Fatalf("reopen %d: expected QUEUE_SEQUENCE_GAP, got batch=%+v err=%v", i, batch, err)
		}
		stats := qi.Stats()
		if !stats.SequenceGapBlock || stats.LineageStart != 1 {
			t.Fatalf("reopen %d: lineageStart must stay 1 and gap-blocked, stats=%+v", i, stats)
		}
		meta = readSequenceMetaFile(t, tempDir)
		if meta.Version != sequenceMetaVersion2 || meta.LineageStart != 1 {
			t.Fatalf("reopen %d: durable lineageStart must stay 1, got %+v", i, meta)
		}
	}
}

// B: v2 arbitrary start 42..44 valid; remove 42 => gap.
func TestDurableQueueV2ArbitraryStartGapOnFirstLoss(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "quarantine"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, seq := range []uint64{42, 43, 44} {
		writeActiveRecord(t, tempDir, seq, fmt.Sprintf("rec-%d", seq), []byte("x"))
	}
	metaPayload := `{"version":2,"highestSequence":44,"lineageStart":42}`
	if err := os.WriteFile(filepath.Join(tempDir, sequenceMetaFile), []byte(metaPayload), 0600); err != nil {
		t.Fatal(err)
	}
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) != 3 || batch[0].Sequence != 42 {
		t.Fatalf("contiguous v2 42..44 must be valid, got %+v err=%v", batch, err)
	}
	if err := os.Remove(activeRecordPath(t, tempDir, 42)); err != nil {
		t.Fatal(err)
	}
	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrSequenceGap) || len(batch) != 0 {
		t.Fatalf("missing lineageStart=42 must QUEUE_SEQUENCE_GAP, got batch=%+v err=%v", batch, err)
	}
	if q2.Stats().LineageStart != 42 {
		t.Fatalf("lineageStart must remain 42, got %d", q2.Stats().LineageStart)
	}
}

// C: v2 interior gap 42,44 => gap.
func TestDurableQueueV2InteriorGap(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "quarantine"), 0700); err != nil {
		t.Fatal(err)
	}
	writeActiveRecord(t, tempDir, 42, "rec-42", []byte("a"))
	writeActiveRecord(t, tempDir, 44, "rec-44", []byte("c"))
	metaPayload := `{"version":2,"highestSequence":44,"lineageStart":42}`
	if err := os.WriteFile(filepath.Join(tempDir, sequenceMetaFile), []byte(metaPayload), 0600); err != nil {
		t.Fatal(err)
	}
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrSequenceGap) || len(batch) != 0 {
		t.Fatalf("missing 43 must QUEUE_SEQUENCE_GAP, got batch=%+v err=%v", batch, err)
	}
	if !q.Stats().SequenceGapBlock || q.Stats().LineageStart != 42 {
		t.Fatalf("stats must report gap with lineageStart=42: %+v", q.Stats())
	}
}

// E: legacy highest=41 empty then enqueue 42 => v2 lineageStart=42.
func TestDurableQueueLegacyEmptyHighestEnqueuesNewLineage(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "quarantine"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, sequenceMetaFile), []byte(`{"highestSequence":41}`), 0600); err != nil {
		t.Fatal(err)
	}
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if q.Stats().LineageStart != 0 {
		t.Fatalf("legacy empty must leave lineageStart unset until next enqueue, got %d", q.Stats().LineageStart)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrSequenceGap) || len(batch) != 0 {
		t.Fatalf("empty active with legacy highest must gap, got batch=%+v err=%v", batch, err)
	}
	rec, err := q.Enqueue([]byte("fresh-lineage"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Sequence != 42 {
		t.Fatalf("expected next sequence 42, got %d", rec.Sequence)
	}
	meta := readSequenceMetaFile(t, tempDir)
	if meta.Version != sequenceMetaVersion2 || meta.LineageStart != 42 || meta.HighestSequence != 42 {
		t.Fatalf("enqueue must write v2 lineageStart=42 highest=42, got %+v", meta)
	}
	if q.Stats().LineageStart != 42 {
		t.Fatalf("stats lineageStart must be 42, got %d", q.Stats().LineageStart)
	}
}

// F: corrupt different-ID replacement after ACK delete fail => ACK conflict, auth retained, not quarantine-hidden.
func TestDurableQueueCorruptDifferentIDReplacementAckConflict(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := q.Enqueue([]byte("original-A"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("keep-later")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) < 1 {
		t.Fatalf("expected batch, got %+v err=%v", batch, err)
	}
	authorizedID := batch[0].ID
	q.removeHook = func(path string) error {
		if strings.Contains(path, string(filepath.Separator)+"active"+string(filepath.Separator)) && strings.HasSuffix(path, ".json") {
			return errors.New("injected active delete fail")
		}
		return nil
	}
	if err := q.CommitAccepted(batch[:1], rec.Sequence); err == nil {
		t.Fatal("expected post-meta delete failure")
	}
	originalPath := activeRecordPath(t, tempDir, rec.Sequence)
	if err := os.Remove(originalPath); err != nil {
		t.Fatal(err)
	}
	writeCorruptActiveNamed(t, tempDir, rec.Sequence, "rec-replacement-corrupt")

	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !recordPresent(t, tempDir, "active", rec.Sequence) {
		t.Fatal("corrupt different-ID replacement must remain in active")
	}
	if recordPresent(t, tempDir, "quarantine", rec.Sequence) {
		t.Fatal("corrupt ACK-bounded replacement must not be quarantine-hidden")
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrAckRecoveryConflict) || len(batch) != 0 {
		t.Fatalf("expected QUEUE_ACK_RECOVERY_CONFLICT, got batch=%+v err=%v", batch, err)
	}
	meta := readSequenceMetaFile(t, tempDir)
	if len(meta.AcceptedRecords) != 1 || meta.AcceptedRecords[0].ID != authorizedID {
		t.Fatalf("acceptedRecords auth must be retained, got %+v", meta.AcceptedRecords)
	}
	if !q2.Stats().AckRecoveryConflictBlock {
		t.Fatalf("must report ack recovery conflict: %+v", q2.Stats())
	}
}

// G: corrupt unauthorized old seq => ACK conflict not quarantine-first.
func TestDurableQueueCorruptUnauthorizedOldSeqAckConflict(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "quarantine"), 0700); err != nil {
		t.Fatal(err)
	}
	writeCorruptActiveNamed(t, tempDir, 1, "unexpected-old")
	writeActiveRecord(t, tempDir, 2, "later", []byte("later"))
	metaPayload := `{"version":2,"highestSequence":2,"lineageStart":1,"acceptedThrough":1}`
	if err := os.WriteFile(filepath.Join(tempDir, sequenceMetaFile), []byte(metaPayload), 0600); err != nil {
		t.Fatal(err)
	}

	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !recordPresent(t, tempDir, "active", 1) {
		t.Fatal("corrupt unauthorized old-sequence must be retained in active")
	}
	if recordPresent(t, tempDir, "quarantine", 1) {
		t.Fatal("must not quarantine-first an ACK-bounded unauthorized seq")
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrAckRecoveryConflict) || len(batch) != 0 {
		t.Fatalf("expected QUEUE_ACK_RECOVERY_CONFLICT, got batch=%+v err=%v", batch, err)
	}
	if !q.Stats().AckRecoveryConflictBlock || q.Stats().AcceptedThrough != 1 {
		t.Fatalf("must fail closed with acceptedThrough unchanged: %+v", q.Stats())
	}
}

// H: quarantine different-ID vs acceptedRecords => conflict, auth not cleared.
func TestDurableQueueQuarantineDifferentIDVsAcceptedRecordsConflict(t *testing.T) {
	tempDir := t.TempDir()
	q, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := q.Enqueue([]byte("auth-one"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil || len(batch) < 1 {
		t.Fatalf("expected batch, got %+v err=%v", batch, err)
	}
	authorizedID := batch[0].ID
	q.removeHook = func(path string) error {
		if strings.Contains(path, string(filepath.Separator)+"active"+string(filepath.Separator)) && strings.HasSuffix(path, ".json") {
			return errors.New("injected active delete fail")
		}
		return nil
	}
	if err := q.CommitAccepted(batch[:1], rec.Sequence); err == nil {
		t.Fatal("expected post-meta delete failure")
	}
	// Move authorized active aside and place a different-ID copy in quarantine.
	if err := os.Remove(activeRecordPath(t, tempDir, rec.Sequence)); err != nil {
		t.Fatal(err)
	}
	writeQuarantineRecord(t, tempDir, rec.Sequence, "rec-quarantine-other", []byte("other"))

	q2, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = q2.ReadBatch(10, 1024*1024)
	if !errors.Is(err, ErrAckRecoveryConflict) || len(batch) != 0 {
		t.Fatalf("expected QUEUE_ACK_RECOVERY_CONFLICT, got batch=%+v err=%v", batch, err)
	}
	meta := readSequenceMetaFile(t, tempDir)
	if len(meta.AcceptedRecords) != 1 || meta.AcceptedRecords[0].ID != authorizedID || meta.AcceptedRecords[0].Sequence != rec.Sequence {
		t.Fatalf("acceptedRecords auth must not be cleared, got %+v", meta.AcceptedRecords)
	}
	if !recordPresent(t, tempDir, "quarantine", rec.Sequence) {
		t.Fatal("conflicting quarantine record must be retained")
	}
	if !q2.Stats().AckRecoveryConflictBlock {
		t.Fatalf("must report ack recovery conflict: %+v", q2.Stats())
	}
}
