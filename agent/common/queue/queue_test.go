package queue

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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

	// 3. Delete through sequence 1
	if err := q.DeleteThrough(1); err != nil {
		t.Fatalf("delete through failed: %v", err)
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

func TestDurableQueueDeleteThroughOK(t *testing.T) {
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
	if err := q.DeleteThrough(1); err != nil {
		t.Fatal(err)
	}
	batch, err := q.ReadBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 1 || batch[0].Sequence != 2 {
		t.Fatalf("partial delete failed: %+v", batch)
	}
}

func TestDurableQueueSequenceLineageSurvivesDrain(t *testing.T) {
	tempDir := t.TempDir()

	q1, err := NewDurableQueue(tempDir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	var last uint64
	for i := 0; i < 3; i++ {
		rec, err := q1.Enqueue([]byte("payload"))
		if err != nil {
			t.Fatal(err)
		}
		last = rec.Sequence
	}
	if err := q1.DeleteThrough(last); err != nil {
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
