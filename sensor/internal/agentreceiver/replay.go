package agentreceiver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ReplayDecision classifies an incoming delivery against previously seen batch keys.
type ReplayDecision int

const (
	ReplayNone ReplayDecision = iota
	ReplayExact
	ReplayConflict
)

type cachedBatchEntry struct {
	firstSequence uint64
	lastSequence  uint64
	batchChecksum string
	ack           DeliveryAck
	recordedAt    time.Time
}

// ComputeBatchChecksum produces a deterministic SHA-256 fingerprint over the sequence,
// ID, and checksum of all records in an envelope.
func ComputeBatchChecksum(records []DeliveryRecord) string {
	h := sha256.New()
	for _, r := range records {
		_, _ = fmt.Fprintf(h, "%d\n%s\n%s\n", r.Sequence, r.ID, r.Checksum)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ReplayTracker is a thread-safe, bounded in-memory cache that prevents duplicate
// processing and rejects conflicting batch replays.
type ReplayTracker struct {
	mu         sync.RWMutex
	maxEntries int
	entries    map[string]cachedBatchEntry
	keys       []string
}

func NewReplayTracker(maxEntries int) *ReplayTracker {
	if maxEntries <= 0 {
		maxEntries = 5000
	}
	return &ReplayTracker{
		maxEntries: maxEntries,
		entries:    make(map[string]cachedBatchEntry, maxEntries),
		keys:       make([]string, 0, maxEntries),
	}
}

// Check evaluates whether a batch key (typically "agentID:batchID") has been seen before.
func (t *ReplayTracker) Check(key string, firstSeq, lastSeq uint64, batchChecksum string) (ReplayDecision, DeliveryAck) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	cached, exists := t.entries[key]
	if !exists {
		return ReplayNone, DeliveryAck{}
	}

	// Exact replay: same sequences and exact matching record checksums
	if cached.firstSequence == firstSeq &&
		cached.lastSequence == lastSeq &&
		cached.batchChecksum == batchChecksum {
		return ReplayExact, cached.ack
	}

	// Conflict: same batch identifier with divergent sequence bounds or payload checksums
	return ReplayConflict, DeliveryAck{}
}

// Record stores a successfully accepted batch and its authoritative ACK response.
func (t *ReplayTracker) Record(key string, firstSeq, lastSeq uint64, batchChecksum string, ack DeliveryAck) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.entries[key]; exists {
		// Update existing
		t.entries[key] = cachedBatchEntry{
			firstSequence: firstSeq,
			lastSequence:  lastSeq,
			batchChecksum: batchChecksum,
			ack:           ack,
			recordedAt:    time.Now().UTC(),
		}
		return
	}

	// Evict oldest if capacity reached
	if len(t.keys) >= t.maxEntries {
		oldest := t.keys[0]
		t.keys = t.keys[1:]
		delete(t.entries, oldest)
	}

	t.entries[key] = cachedBatchEntry{
		firstSequence: firstSeq,
		lastSequence:  lastSeq,
		batchChecksum: batchChecksum,
		ack:           ack,
		recordedAt:    time.Now().UTC(),
	}
	t.keys = append(t.keys, key)
}

func (t *ReplayTracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = make(map[string]cachedBatchEntry, t.maxEntries)
	t.keys = t.keys[:0]
}

func (t *ReplayTracker) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.entries)
}
