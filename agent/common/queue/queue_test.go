package queue

import (
	"bytes"
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
