package agentreceiver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"platform/sensor/internal/spool"
)

const (
	agentBatchRecordType   = "agent.delivery.batch"
	batchIndexDirName      = "batch-index"
	streamStateDirName     = "stream-state"
	defaultMaxSparseRanges = 4096
)

// DurableReplayConflictError is returned when durable Agent sequence identity
// is reused with different immutable batch contents or overlaps an accepted range.
type DurableReplayConflictError struct {
	TenantID      string
	AgentID       string
	FirstSequence uint64
	LastSequence  uint64
}

func (e *DurableReplayConflictError) Error() string {
	return ErrCodeReplayConflict + ": durable Agent sequence conflict"
}

func IsDurableReplayConflict(err error) bool {
	var target *DurableReplayConflictError
	return errors.As(err, &target)
}

type batchIndexEntry struct {
	TenantID        string `json:"tenantId"`
	AgentID         string `json:"agentId"`
	BatchID         string `json:"batchId"`
	FirstSequence   uint64 `json:"firstSequence"`
	LastSequence    uint64 `json:"lastSequence"`
	BatchChecksum   string `json:"batchChecksum"`
	AcceptedThrough uint64 `json:"acceptedThrough"`
	SpoolSequence   uint64 `json:"spoolSequence"`
	AssignmentGen   int64  `json:"assignmentGeneration,omitempty"`
	ReceivingSensor string `json:"receivingSensorId,omitempty"`
}

type streamStateEntry struct {
	TenantID                  string    `json:"tenantId"`
	AgentID                   string    `json:"agentId"`
	ContiguousAcceptedThrough uint64    `json:"contiguousAcceptedThrough"`
	UpdatedAt                 time.Time `json:"updatedAt"`
}

type durableBatchPayload struct {
	TenantID      string           `json:"tenantId"`
	AgentID       string           `json:"agentId"`
	SensorID      string           `json:"sensorId"`
	Generation    int64            `json:"assignmentGeneration"`
	FirstSequence uint64           `json:"firstSequence"`
	LastSequence  uint64           `json:"lastSequence"`
	RecordIDs     []string         `json:"recordIds"`
	BatchChecksum string           `json:"batchChecksum"`
	BatchID       string           `json:"batchId"`
	CorrelationID string           `json:"correlationId"`
	Timestamp     time.Time        `json:"timestamp"`
	Envelope      DeliveryEnvelope `json:"envelope"`
}

// SpoolAcceptanceSink durably accepts Agent batches into a dedicated DiskSpool
// directory. Pending spool records hold undrained payloads. The batch-index is
// the durable acceptance receipt: it survives AcknowledgeThrough (drain) and
// Sensor restart so lost-ACK retries remain idempotent.
//
// Replay protection combines a durable per-stream contiguous acceptance watermark
// (stream-state/) with exact sparse receipts (batch-index/). Drained exact receipts
// are safely compacted only when wholly covered by the durable persisted contiguous
// watermark.
type SpoolAcceptanceSink struct {
	spool              *spool.DiskSpool
	indexDir           string
	stateDir           string
	mu                 sync.Mutex
	reconcileRequired  bool
	ranges             map[string][]batchIndexEntry
	streams            map[string]*streamStateEntry
	maxSparseRanges    int
	maxDrainedReceipts int
	// indexPublisher is an unexported test seam for exercising the critical
	// append-succeeds/index-publication-fails boundary. Production leaves it nil.
	indexPublisher func(string, batchIndexEntry) error
}

func NewSpoolAcceptanceSink(agentSpool *spool.DiskSpool, agentSpoolDir string) (*SpoolAcceptanceSink, error) {
	if agentSpool == nil {
		return nil, errors.New("ERR_INVALID_CONFIG: agent DiskSpool is required")
	}
	dir := filepath.Clean(strings.TrimSpace(agentSpoolDir))
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("ERR_INVALID_CONFIG: agent spool directory must be absolute")
	}
	indexDir := filepath.Join(dir, batchIndexDirName)
	if err := os.MkdirAll(indexDir, 0750); err != nil {
		return nil, fmt.Errorf("ERR_ACCEPTANCE_SINK_UNAVAILABLE: failed to create batch index directory: %w", err)
	}
	if fi, err := os.Lstat(indexDir); err != nil {
		return nil, fmt.Errorf("ERR_ACCEPTANCE_SINK_UNAVAILABLE: failed to inspect batch index directory: %w", err)
	} else if fi.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("ERR_ACCEPTANCE_SINK_UNAVAILABLE: batch index directory is a symlink")
	}

	stateDir := filepath.Join(dir, streamStateDirName)
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		return nil, fmt.Errorf("ERR_ACCEPTANCE_SINK_UNAVAILABLE: failed to create stream state directory: %w", err)
	}
	if fi, err := os.Lstat(stateDir); err != nil {
		return nil, fmt.Errorf("ERR_ACCEPTANCE_SINK_UNAVAILABLE: failed to inspect stream state directory: %w", err)
	} else if fi.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("ERR_ACCEPTANCE_SINK_UNAVAILABLE: stream state directory is a symlink")
	}

	sink := &SpoolAcceptanceSink{
		spool:              agentSpool,
		indexDir:           indexDir,
		stateDir:           stateDir,
		ranges:             make(map[string][]batchIndexEntry),
		streams:            make(map[string]*streamStateEntry),
		maxSparseRanges:    defaultMaxSparseRanges,
		maxDrainedReceipts: defaultMaxSparseRanges,
	}
	if err := sink.rebuildIndexFromSpool(); err != nil {
		return nil, err
	}
	return sink, nil
}

func (s *SpoolAcceptanceSink) effectiveMaxSparseRanges() int {
	if s.maxSparseRanges > 0 {
		return s.maxSparseRanges
	}
	if s.maxDrainedReceipts > 0 {
		return s.maxDrainedReceipts
	}
	return defaultMaxSparseRanges
}

func (s *SpoolAcceptanceSink) AcceptBatch(ctx context.Context, env DeliveryEnvelope) (AcceptanceResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return AcceptanceResult{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.reconcileRequired {
		if err := s.reconcileAllLocked(); err != nil {
			return AcceptanceResult{}, err
		}
	}

	if env.FirstSequence == 0 || env.LastSequence < env.FirstSequence {
		return AcceptanceResult{}, fmt.Errorf("%s: sequence range [%d..%d] is invalid", ErrCodeSinkFailed, env.FirstSequence, env.LastSequence)
	}

	streamKey := sourceRangeStreamKey(env.TenantID, env.AgentID)
	state := s.getOrCreateStreamStateLocked(env.TenantID, env.AgentID)
	checksum := ComputeBatchChecksum(env.Records)
	key := batchIndexKey(env.TenantID, env.AgentID, env.FirstSequence, env.LastSequence)

	// 1. Exact receipt lookup (if receipt is still present on disk)
	if existing, ok, err := s.readVerifiedIndexLocked(key); err != nil {
		return AcceptanceResult{}, err
	} else if ok {
		if existing.BatchChecksum == checksum {
			return AcceptanceResult{AcceptedThrough: existing.AcceptedThrough}, nil
		}
		return AcceptanceResult{}, durableSequenceConflict(env.TenantID, env.AgentID, env.FirstSequence, env.LastSequence)
	}

	// 2. Replay below compacted contiguous watermark (Section 6.5)
	if env.LastSequence <= state.ContiguousAcceptedThrough {
		return AcceptanceResult{AcceptedThrough: env.LastSequence}, nil
	}

	// 3. Partial overlap with contiguous watermark (Section 6.6)
	if env.FirstSequence <= state.ContiguousAcceptedThrough && env.LastSequence > state.ContiguousAcceptedThrough {
		return AcceptanceResult{}, durableSequenceConflict(env.TenantID, env.AgentID, env.FirstSequence, env.LastSequence)
	}

	// 4. Overlap with existing sparse ranges
	if s.overlapsAcceptedRangeLocked(env.TenantID, env.AgentID, env.FirstSequence, env.LastSequence) {
		return AcceptanceResult{}, durableSequenceConflict(env.TenantID, env.AgentID, env.FirstSequence, env.LastSequence)
	}

	// 5. Sparse state capacity guard (Section 6.8)
	// Contiguous gap-closing traffic (FirstSequence == ContiguousAcceptedThrough + 1)
	// is allowed even if sparse limit is reached because it advances the watermark.
	// A new disjoint sparse range (FirstSequence > ContiguousAcceptedThrough + 1)
	// must fail closed if the number of sparse ranges reaches the limit.
	isGapClosing := env.FirstSequence == state.ContiguousAcceptedThrough+1
	if !isGapClosing {
		limit := s.effectiveMaxSparseRanges()
		sparseCount := s.countSparseRangesLocked(streamKey)
		if sparseCount >= limit {
			return AcceptanceResult{}, fmt.Errorf("%s: sparse accepted range capacity exceeded for agent stream", ErrCodeSinkFailed)
		}
	}

	recordIDs := make([]string, 0, len(env.Records))
	for _, r := range env.Records {
		recordIDs = append(recordIDs, r.ID)
	}
	payload := durableBatchPayload{
		TenantID:      env.TenantID,
		AgentID:       env.AgentID,
		SensorID:      env.ReceivingSensorID,
		Generation:    env.AssignmentGeneration,
		FirstSequence: env.FirstSequence,
		LastSequence:  env.LastSequence,
		RecordIDs:     recordIDs,
		BatchChecksum: checksum,
		BatchID:       env.BatchID,
		CorrelationID: env.CorrelationID,
		Timestamp:     time.Now().UTC(),
		Envelope:      env,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return AcceptanceResult{}, fmt.Errorf("%s: failed to encode durable agent batch: %w", ErrCodeSinkFailed, err)
	}

	rec, err := s.spool.Append(spool.AppendRequest{
		Class:     spool.ClassOperational,
		Type:      agentBatchRecordType,
		Payload:   encoded,
		CreatedAt: payload.Timestamp,
	})
	if err != nil {
		return AcceptanceResult{}, err
	}

	entry := batchIndexEntry{
		TenantID:        env.TenantID,
		AgentID:         env.AgentID,
		BatchID:         env.BatchID,
		FirstSequence:   env.FirstSequence,
		LastSequence:    env.LastSequence,
		BatchChecksum:   checksum,
		AcceptedThrough: env.LastSequence,
		SpoolSequence:   rec.Sequence,
		AssignmentGen:   env.AssignmentGeneration,
		ReceivingSensor: env.ReceivingSensorID,
	}
	if err := s.publishIndexLocked(key, entry); err != nil {
		s.reconcileRequired = true
		return AcceptanceResult{}, err
	}
	s.addRangeLocked(entry)

	// Advance contiguous coverage and persist watermark if advanced
	if _, err := s.advanceContiguousCoverageLocked(streamKey); err != nil {
		s.reconcileRequired = true
		return AcceptanceResult{}, err
	}

	// Compact eligible drained historical receipts
	if err := s.compactDrainedReceiptsLocked(streamKey); err != nil {
		s.reconcileRequired = true
		return AcceptanceResult{}, err
	}

	return AcceptanceResult{AcceptedThrough: env.LastSequence}, nil
}

func durableSequenceConflict(tenantID, agentID string, firstSequence, lastSequence uint64) error {
	return &DurableReplayConflictError{
		TenantID:      tenantID,
		AgentID:       agentID,
		FirstSequence: firstSequence,
		LastSequence:  lastSequence,
	}
}

func sourceRangeStreamKey(tenantID, agentID string) string {
	var b []byte
	b = appendLenPrefixed(b, tenantID)
	b = appendLenPrefixed(b, agentID)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func rangesOverlap(firstA, lastA, firstB, lastB uint64) bool {
	return firstA <= lastB && firstB <= lastA
}

func (s *SpoolAcceptanceSink) overlapsAcceptedRangeLocked(tenantID, agentID string, firstSequence, lastSequence uint64) bool {
	for _, existing := range s.ranges[sourceRangeStreamKey(tenantID, agentID)] {
		if rangesOverlap(firstSequence, lastSequence, existing.FirstSequence, existing.LastSequence) {
			return true
		}
	}
	return false
}

func (s *SpoolAcceptanceSink) addRangeLocked(entry batchIndexEntry) {
	stream := sourceRangeStreamKey(entry.TenantID, entry.AgentID)
	s.ranges[stream] = append(s.ranges[stream], entry)
}

// batchIndexKey intentionally excludes BatchID. Agent sequence/range is the
// correctness identity; BatchID remains correlation metadata only.
func batchIndexKey(tenantID, agentID string, firstSequence, lastSequence uint64) string {
	var b []byte
	b = appendLenPrefixed(b, tenantID)
	b = appendLenPrefixed(b, agentID)
	b = appendLenPrefixed(b, fmt.Sprintf("%d", firstSequence))
	b = appendLenPrefixed(b, fmt.Sprintf("%d", lastSequence))
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *SpoolAcceptanceSink) indexPath(key string) string {
	return filepath.Join(s.indexDir, key+".json")
}

func (s *SpoolAcceptanceSink) statePath(key string) string {
	return filepath.Join(s.stateDir, key+".json")
}

func (s *SpoolAcceptanceSink) getOrCreateStreamStateLocked(tenantID, agentID string) *streamStateEntry {
	streamKey := sourceRangeStreamKey(tenantID, agentID)
	state, ok := s.streams[streamKey]
	if !ok || state == nil {
		state = &streamStateEntry{
			TenantID:                  tenantID,
			AgentID:                   agentID,
			ContiguousAcceptedThrough: 0,
			UpdatedAt:                 time.Now().UTC(),
		}
		s.streams[streamKey] = state
	}
	return state
}

func (s *SpoolAcceptanceSink) countSparseRangesLocked(streamKey string) int {
	state := s.streams[streamKey]
	watermark := uint64(0)
	if state != nil {
		watermark = state.ContiguousAcceptedThrough
	}
	count := 0
	for _, entry := range s.ranges[streamKey] {
		if entry.FirstSequence > watermark {
			count++
		}
	}
	return count
}

func (s *SpoolAcceptanceSink) advanceContiguousCoverageLocked(streamKey string) (bool, error) {
	state := s.streams[streamKey]
	if state == nil {
		return false, nil
	}
	ranges := s.ranges[streamKey]
	if len(ranges) == 0 {
		return false, nil
	}
	sorted := make([]batchIndexEntry, len(ranges))
	copy(sorted, ranges)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].FirstSequence == sorted[j].FirstSequence {
			return sorted[i].LastSequence < sorted[j].LastSequence
		}
		return sorted[i].FirstSequence < sorted[j].FirstSequence
	})

	c := state.ContiguousAcceptedThrough
	for {
		foundNext := false
		for _, r := range sorted {
			if r.FirstSequence <= c+1 && r.LastSequence > c {
				c = r.LastSequence
				foundNext = true
				break
			}
		}
		if !foundNext {
			break
		}
	}

	if c > state.ContiguousAcceptedThrough {
		state.ContiguousAcceptedThrough = c
		state.UpdatedAt = time.Now().UTC()
		if err := s.persistStreamStateLocked(streamKey); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (s *SpoolAcceptanceSink) persistStreamStateLocked(streamKey string) error {
	state, ok := s.streams[streamKey]
	if !ok || state == nil {
		return nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("%s: failed to encode stream state: %w", ErrCodeSinkFailed, err)
	}
	path := s.statePath(streamKey)
	tmp, err := os.CreateTemp(s.stateDir, ".stream-state-*.tmp")
	if err != nil {
		return fmt.Errorf("%s: failed to create stream state temp file: %w", ErrCodeSinkFailed, err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0640); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%s: failed to chmod stream state temp file: %w", ErrCodeSinkFailed, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%s: failed to write stream state: %w", ErrCodeSinkFailed, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%s: failed to fsync stream state: %w", ErrCodeSinkFailed, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%s: failed to close stream state: %w", ErrCodeSinkFailed, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("%s: failed to publish stream state: %w", ErrCodeSinkFailed, err)
	}
	cleanup = false
	return syncIndexDir(s.stateDir)
}

func (s *SpoolAcceptanceSink) compactDrainedReceiptsLocked(streamKey string) error {
	state := s.streams[streamKey]
	if state == nil {
		return nil
	}
	acked, err := s.spool.AckedThrough()
	if err != nil {
		return fmt.Errorf("%s: failed to read spool acknowledgement watermark: %w", ErrCodeSinkFailed, err)
	}

	watermark := state.ContiguousAcceptedThrough
	var retained []batchIndexEntry
	compactedAny := false

	for _, entry := range s.ranges[streamKey] {
		// Compaction rule:
		// 1. Fully covered by contiguous acceptance watermark: entry.LastSequence <= watermark
		// 2. Spool payload has already been drained: entry.SpoolSequence <= acked
		if entry.LastSequence <= watermark && entry.SpoolSequence > 0 && entry.SpoolSequence <= acked {
			key := batchIndexKey(entry.TenantID, entry.AgentID, entry.FirstSequence, entry.LastSequence)
			if err := s.removeIndexLocked(key); err != nil {
				return err
			}
			compactedAny = true
			continue
		}
		retained = append(retained, entry)
	}
	s.ranges[streamKey] = retained
	if compactedAny {
		return syncIndexDir(s.indexDir)
	}
	return nil
}

func (s *SpoolAcceptanceSink) readIndexLocked(key string) (batchIndexEntry, bool, error) {
	path := s.indexPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return batchIndexEntry{}, false, nil
		}
		return batchIndexEntry{}, false, fmt.Errorf("%s: failed to read batch index: %w", ErrCodeSinkFailed, err)
	}
	var entry batchIndexEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return batchIndexEntry{}, false, fmt.Errorf("%s: corrupt batch index entry: %w", ErrCodeSinkFailed, err)
	}
	return entry, true, nil
}

func (s *SpoolAcceptanceSink) readVerifiedIndexLocked(key string) (batchIndexEntry, bool, error) {
	entry, ok, err := s.readIndexLocked(key)
	if err != nil || !ok {
		return entry, ok, err
	}
	if err := validateIndexReceipt(key, entry); err != nil {
		return batchIndexEntry{}, false, err
	}
	acked, err := s.spool.AckedThrough()
	if err != nil {
		return batchIndexEntry{}, false, fmt.Errorf("%s: failed to read spool acknowledgement watermark: %w", ErrCodeSinkFailed, err)
	}
	if entry.SpoolSequence <= acked {
		// Pending payload has been drained. The fsynced index entry is the
		// durable acceptance receipt used for lost-ACK replay.
		return entry, true, nil
	}
	rec, err := s.spool.ReadPendingRecord(entry.SpoolSequence)
	if err != nil {
		return batchIndexEntry{}, false, fmt.Errorf("%s: batch index durable record unavailable: %w", ErrCodeSinkFailed, err)
	}
	if rec.Type != agentBatchRecordType {
		return batchIndexEntry{}, false, fmt.Errorf("%s: batch index references wrong spool record type", ErrCodeSinkFailed)
	}
	var payload durableBatchPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		return batchIndexEntry{}, false, fmt.Errorf("%s: indexed durable payload is corrupt: %w", ErrCodeSinkFailed, err)
	}
	payloadKey := batchIndexKey(payload.TenantID, payload.AgentID, payload.FirstSequence, payload.LastSequence)
	if payloadKey != key || payload.TenantID != entry.TenantID || payload.AgentID != entry.AgentID ||
		payload.FirstSequence != entry.FirstSequence || payload.LastSequence != entry.LastSequence ||
		payload.BatchChecksum != entry.BatchChecksum || entry.AcceptedThrough != payload.LastSequence {
		return batchIndexEntry{}, false, fmt.Errorf("%s: batch index does not match durable spool payload", ErrCodeSinkFailed)
	}
	return entry, true, nil
}

func validateIndexReceipt(key string, entry batchIndexEntry) error {
	if entry.SpoolSequence == 0 {
		return fmt.Errorf("%s: batch index missing durable spool sequence", ErrCodeSinkFailed)
	}
	if strings.TrimSpace(entry.TenantID) == "" || strings.TrimSpace(entry.AgentID) == "" ||
		entry.FirstSequence == 0 || entry.LastSequence < entry.FirstSequence ||
		strings.TrimSpace(entry.BatchChecksum) == "" || entry.AcceptedThrough != entry.LastSequence {
		return fmt.Errorf("%s: incomplete batch index receipt", ErrCodeSinkFailed)
	}
	if batchIndexKey(entry.TenantID, entry.AgentID, entry.FirstSequence, entry.LastSequence) != key {
		return fmt.Errorf("%s: batch index key does not match receipt identity", ErrCodeSinkFailed)
	}
	return nil
}

func (s *SpoolAcceptanceSink) publishIndexLocked(key string, entry batchIndexEntry) error {
	if s.indexPublisher != nil {
		return s.indexPublisher(key, entry)
	}
	return s.writeIndexLocked(key, entry)
}

func (s *SpoolAcceptanceSink) writeIndexLocked(key string, entry batchIndexEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("%s: failed to encode batch index: %w", ErrCodeSinkFailed, err)
	}
	path := s.indexPath(key)
	tmp, err := os.CreateTemp(s.indexDir, ".batch-index-*.tmp")
	if err != nil {
		return fmt.Errorf("%s: failed to create batch index temp file: %w", ErrCodeSinkFailed, err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0640); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%s: failed to chmod batch index temp file: %w", ErrCodeSinkFailed, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%s: failed to write batch index: %w", ErrCodeSinkFailed, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%s: failed to fsync batch index: %w", ErrCodeSinkFailed, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%s: failed to close batch index: %w", ErrCodeSinkFailed, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("%s: failed to publish batch index: %w", ErrCodeSinkFailed, err)
	}
	cleanup = false
	return syncIndexDir(s.indexDir)
}

func syncIndexDir(dir string) error {
	dirFile, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("%s: failed to open directory for sync: %w", ErrCodeSinkFailed, err)
	}
	defer dirFile.Close()
	if err := dirFile.Sync(); err != nil {
		return fmt.Errorf("%s: failed to fsync directory: %w", ErrCodeSinkFailed, err)
	}
	return nil
}

func (s *SpoolAcceptanceSink) rebuildIndexFromSpool() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reconcileAllLocked()
}

func (s *SpoolAcceptanceSink) loadStreamStateFilesLocked() (map[string]*streamStateEntry, error) {
	entries, err := os.ReadDir(s.stateDir)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to list stream state directory: %w", ErrCodeSinkFailed, err)
	}
	loaded := make(map[string]*streamStateEntry)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.stateDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to inspect stream state entry: %w", ErrCodeSinkFailed, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: unsafe stream state entry type", ErrCodeSinkFailed)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to read stream state: %w", ErrCodeSinkFailed, err)
		}
		var state streamStateEntry
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, fmt.Errorf("%s: corrupt stream state entry: %w", ErrCodeSinkFailed, err)
		}
		key := strings.TrimSuffix(entry.Name(), ".json")
		if strings.TrimSpace(state.TenantID) == "" || strings.TrimSpace(state.AgentID) == "" {
			return nil, fmt.Errorf("%s: stream state entry missing tenant or agent identity", ErrCodeSinkFailed)
		}
		expectedKey := sourceRangeStreamKey(state.TenantID, state.AgentID)
		if key != expectedKey {
			return nil, fmt.Errorf("%s: stream state file key does not match identity", ErrCodeSinkFailed)
		}
		loaded[key] = &state
	}
	return loaded, nil
}

func (s *SpoolAcceptanceSink) reconcileAllLocked() error {
	acked, err := s.spool.AckedThrough()
	if err != nil {
		s.reconcileRequired = true
		return fmt.Errorf("%s: failed to read spool acknowledgement watermark: %w", ErrCodeSinkFailed, err)
	}

	loadedStates, err := s.loadStreamStateFilesLocked()
	if err != nil {
		s.reconcileRequired = true
		return err
	}
	s.streams = loadedStates

	loaded, err := s.loadIndexFilesLocked()
	if err != nil {
		s.reconcileRequired = true
		return err
	}

	pendingKeys := make(map[string]struct{})
	err = s.spool.VisitPending(func(rec spool.Record) error {
		if rec.Type != agentBatchRecordType {
			return nil
		}
		var payload durableBatchPayload
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			return fmt.Errorf("%s: corrupt durable Agent payload during recovery: %w", ErrCodeSinkFailed, err)
		}
		if strings.TrimSpace(payload.TenantID) == "" || strings.TrimSpace(payload.AgentID) == "" ||
			payload.FirstSequence == 0 || payload.LastSequence < payload.FirstSequence ||
			strings.TrimSpace(payload.BatchChecksum) == "" {
			return fmt.Errorf("%s: incomplete durable Agent payload during recovery", ErrCodeSinkFailed)
		}
		key := batchIndexKey(payload.TenantID, payload.AgentID, payload.FirstSequence, payload.LastSequence)
		entry := batchIndexEntry{
			TenantID:        payload.TenantID,
			AgentID:         payload.AgentID,
			BatchID:         payload.BatchID,
			FirstSequence:   payload.FirstSequence,
			LastSequence:    payload.LastSequence,
			BatchChecksum:   payload.BatchChecksum,
			AcceptedThrough: payload.LastSequence,
			SpoolSequence:   rec.Sequence,
			AssignmentGen:   payload.Generation,
			ReceivingSensor: payload.SensorID,
		}
		if existing, ok := loaded[key]; ok {
			if existing.BatchChecksum != payload.BatchChecksum ||
				existing.TenantID != payload.TenantID || existing.AgentID != payload.AgentID ||
				existing.FirstSequence != payload.FirstSequence || existing.LastSequence != payload.LastSequence {
				return durableSequenceConflict(payload.TenantID, payload.AgentID, payload.FirstSequence, payload.LastSequence)
			}
			if existing.SpoolSequence != rec.Sequence {
				return fmt.Errorf("%s: accepted identity has conflicting pending and receipt spool sequences", ErrCodeSinkFailed)
			}
			pendingKeys[key] = struct{}{}
			return nil
		}
		if err := s.publishIndexLocked(key, entry); err != nil {
			return err
		}
		loaded[key] = entry
		pendingKeys[key] = struct{}{}
		return nil
	})
	if err != nil {
		s.reconcileRequired = true
		return err
	}

	ranges := make(map[string][]batchIndexEntry)
	for key, entry := range loaded {
		if err := validateIndexReceipt(key, entry); err != nil {
			s.reconcileRequired = true
			return err
		}
		if _, pending := pendingKeys[key]; !pending {
			if entry.SpoolSequence > acked {
				s.reconcileRequired = true
				return fmt.Errorf("%s: batch index durable record unavailable", ErrCodeSinkFailed)
			}
		}
		stream := sourceRangeStreamKey(entry.TenantID, entry.AgentID)
		for _, existing := range ranges[stream] {
			if rangesOverlap(entry.FirstSequence, entry.LastSequence, existing.FirstSequence, existing.LastSequence) {
				if existing.BatchChecksum != entry.BatchChecksum ||
					existing.FirstSequence != entry.FirstSequence || existing.LastSequence != entry.LastSequence {
					s.reconcileRequired = true
					return durableSequenceConflict(entry.TenantID, entry.AgentID, entry.FirstSequence, entry.LastSequence)
				}
			}
		}
		ranges[stream] = append(ranges[stream], entry)
		_ = s.getOrCreateStreamStateLocked(entry.TenantID, entry.AgentID)
	}

	s.ranges = ranges
	for stream := range s.ranges {
		if _, err := s.advanceContiguousCoverageLocked(stream); err != nil {
			s.reconcileRequired = true
			return err
		}
		if err := s.compactDrainedReceiptsLocked(stream); err != nil {
			s.reconcileRequired = true
			return err
		}
	}
	s.reconcileRequired = false
	return nil
}

func (s *SpoolAcceptanceSink) loadIndexFilesLocked() (map[string]batchIndexEntry, error) {
	entries, err := os.ReadDir(s.indexDir)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to list batch index directory: %w", ErrCodeSinkFailed, err)
	}
	loaded := make(map[string]batchIndexEntry)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.indexDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to inspect batch index entry: %w", ErrCodeSinkFailed, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: unsafe batch index entry type", ErrCodeSinkFailed)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to read batch index: %w", ErrCodeSinkFailed, err)
		}
		var receipt batchIndexEntry
		if err := json.Unmarshal(data, &receipt); err != nil {
			return nil, fmt.Errorf("%s: corrupt batch index entry: %w", ErrCodeSinkFailed, err)
		}
		key := strings.TrimSuffix(entry.Name(), ".json")
		if err := validateIndexReceipt(key, receipt); err != nil {
			return nil, err
		}
		loaded[key] = receipt
	}
	return loaded, nil
}

func (s *SpoolAcceptanceSink) removeIndexLocked(key string) error {
	path := s.indexPath(key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%s: failed to evict drained batch index: %w", ErrCodeSinkFailed, err)
	}
	return nil
}

// ContiguousWatermark returns the durable contiguous accepted sequence watermark
// for the given tenant and agent stream.
func (s *SpoolAcceptanceSink) ContiguousWatermark(tenantID, agentID string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.streams[sourceRangeStreamKey(tenantID, agentID)]
	if state == nil {
		return 0
	}
	return state.ContiguousAcceptedThrough
}

// SparseRangeCount returns the number of active sparse out-of-order ranges
// for the given tenant and agent stream.
func (s *SpoolAcceptanceSink) SparseRangeCount(tenantID, agentID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.countSparseRangesLocked(sourceRangeStreamKey(tenantID, agentID))
}

// SetMaxSparseRangesForTest allows tests to set a smaller sparse state capacity limit.
func (s *SpoolAcceptanceSink) SetMaxSparseRangesForTest(limit int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxSparseRanges = limit
	s.maxDrainedReceipts = limit
}
