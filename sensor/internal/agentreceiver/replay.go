package agentreceiver

import (
	"crypto/sha256"
	"encoding/binary"
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

// ReplayIdentity uniquely identifies a batch for process-local replay tracking.
// Encoding is length-prefixed to avoid delimiter-injection between fields.
type ReplayIdentity struct {
	TenantID string
	AgentID  string
	BatchID  string
}

// Key returns a length-prefixed map key safe against embedded separators.
func (id ReplayIdentity) Key() string {
	var b []byte
	b = appendLenPrefixed(b, id.TenantID)
	b = appendLenPrefixed(b, id.AgentID)
	b = appendLenPrefixed(b, id.BatchID)
	return string(b)
}

func appendLenPrefixed(dst []byte, s string) []byte {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(s)))
	dst = append(dst, lenBuf[:]...)
	dst = append(dst, s...)
	return dst
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

// ReplayTracker is a thread-safe, bounded in-memory cache that prevents conflicting
// reuse of a BatchID within a single Sensor process lifetime.
//
// IMPORTANT: ReplayTracker is a process-local OPTIMIZATION only — not correctness
// authority across restart, eviction, or durable-storage failure. Exact cache hits
// deliberately return ReplayNone so the Receiver must re-enter the durable acceptance
// sink and verify spool truth before emitting an ACK. Eviction or process restart does
// NOT authorize Agent-side queue deletion; the Agent MUST retain records until it
// receives a valid durable batch-bound application ACK.
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

// Check evaluates whether a ReplayIdentity has been seen before in this process.
// Exact matches intentionally fall through to durable verification; only conflicting
// reuse is rejected from the process-local cache.
func (t *ReplayTracker) Check(id ReplayIdentity, firstSeq, lastSeq uint64, batchChecksum string) (ReplayDecision, DeliveryAck) {
	key := id.Key()
	t.mu.RLock()
	defer t.mu.RUnlock()

	cached, exists := t.entries[key]
	if !exists {
		return ReplayNone, DeliveryAck{}
	}

	if cached.firstSequence == firstSeq &&
		cached.lastSequence == lastSeq &&
		cached.batchChecksum == batchChecksum {
		return ReplayNone, DeliveryAck{}
	}

	return ReplayConflict, DeliveryAck{}
}

// Record stores a successfully accepted batch and its authoritative ACK response.
// The stored ACK is diagnostic/cache state only and is never sufficient by itself to
// authorize a future ACK; the durable sink remains authoritative.
func (t *ReplayTracker) Record(id ReplayIdentity, firstSeq, lastSeq uint64, batchChecksum string, ack DeliveryAck) {
	key := id.Key()
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.entries[key]; exists {
		t.entries[key] = cachedBatchEntry{
			firstSequence: firstSeq,
			lastSequence:  lastSeq,
			batchChecksum: batchChecksum,
			ack:           ack,
			recordedAt:    time.Now().UTC(),
		}
		return
	}

	// Evict oldest if capacity reached. Eviction is local optimization only and
	// must never be interpreted as authorization for Agent-side record deletion.
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
