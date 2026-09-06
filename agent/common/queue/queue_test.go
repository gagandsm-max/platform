package queue

import (
	"testing"
)

func TestMemoryQueueOperations(t *testing.T) {
	q := NewMemoryQueue(2)

	if err := q.Push(&QueueItem{ID: "1", Timestamp: 100}); err != nil {
		t.Fatalf("unexpected error pushing item 1: %v", err)
	}
	if err := q.Push(&QueueItem{ID: "2", Timestamp: 200}); err != nil {
		t.Fatalf("unexpected error pushing item 2: %v", err)
	}

	// Queue full test
	if err := q.Push(&QueueItem{ID: "3", Timestamp: 300}); err != ErrQueueFull {
		t.Fatalf("expected ErrQueueFull, got: %v", err)
	}

	if q.Len() != 2 {
		t.Fatalf("expected queue length 2, got %d", q.Len())
	}

	batch := q.PopBatch(1)
	if len(batch) != 1 || batch[0].ID != "1" {
		t.Fatalf("expected item 1, got %v", batch)
	}

	if q.Len() != 1 {
		t.Fatalf("expected queue length 1, got %d", q.Len())
	}
}
