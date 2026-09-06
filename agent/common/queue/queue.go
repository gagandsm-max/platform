package queue

import (
	"errors"
	"sync"
)

var ErrQueueFull = errors.New("agent queue capacity reached")

type QueueItem struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"timestamp"`
	Payload   []byte `json:"payload"`
}

type MemoryQueue struct {
	items    []*QueueItem
	capacity int
	mu       sync.Mutex
}

func NewMemoryQueue(capacity int) *MemoryQueue {
	if capacity <= 0 {
		capacity = 1000
	}
	return &MemoryQueue{
		items:    make([]*QueueItem, 0, capacity),
		capacity: capacity,
	}
}

func (q *MemoryQueue) Push(item *QueueItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) >= q.capacity {
		return ErrQueueFull
	}
	q.items = append(q.items, item)
	return nil
}

func (q *MemoryQueue) PopBatch(batchSize int) []*QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil
	}

	if batchSize > len(q.items) {
		batchSize = len(q.items)
	}

	batch := make([]*QueueItem, batchSize)
	copy(batch, q.items[:batchSize])
	q.items = q.items[batchSize:]
	return batch
}

func (q *MemoryQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
