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
	"strings"
	"sync"
	"time"

	"platform/sensor/internal/spool"
)

const (
	agentBatchRecordType = "agent.delivery.batch"
	batchIndexDirName    = "batch-index"
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
	TenantID         string `json:"tenantId"`
	AgentID          string `json:"agentId"`
	BatchID          string `json:"batchId"`
	FirstSequence    uint64 `json:"firstSequence"`
	LastSequence     uint64 `json:"lastSequence"`
	BatchChecksum    string `json:"batchChecksum"`
	AcceptedThrough  uint64 `json:"acceptedThrough"`
	SpoolSequence    uint64 `json:"spoolSequence"`
	AssignmentGen    int64  `json:"assignmentGeneration,omitempty"`
	ReceivingSensor  string `json:"receivingSensorId,omitempty"`
}

type durableBatchPayload struct {
	TenantID             string           `json:"tenantId"`
	AgentID              string           `json:"agentId"`
	SensorID             string           `json:"sensorId"`
	Generation           int64            `json:"assignmentGeneration"`
	FirstSequence        uint64           `json:"firstSequence"`
	LastSequence         uint64           `json:"lastSequence"`
	RecordIDs            []string         `json:"recordIds"`
	BatchChecksum        string           `json:"batchChecksum"`
	BatchID              string           `json:"batchId"`
	CorrelationID        string           `json:"correlationId"`
	Timestamp            time.Time        `json:"timestamp"`
	Envelope             DeliveryEnvelope `json:"envelope"`
}

// SpoolAcceptanceSink durably accepts Agent batches into a dedicated DiskSpool
// directory. The spool is the durable source of truth; the batch index is only
// a derived lookup accelerator and is rebuilt from the complete pending spool.
type SpoolAcceptanceSink struct {
	spool             *spool.DiskSpool
	indexDir          string
	mu                sync.Mutex
	reconcileRequired bool
	ranges            map[string][]batchIndexEntry
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
	sink := &SpoolAcceptanceSink{
		spool: agentSpool,
		indexDir: indexDir,
		ranges: make(map[string][]batchIndexEntry),
	}
	if err := sink.rebuildIndexFromSpool(); err != nil {
		return nil, err
	}
	return sink, nil
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

	checksum := ComputeBatchChecksum(env.Records)
	key := batchIndexKey(env.TenantID, env.AgentID, env.FirstSequence, env.LastSequence)
	if existing, ok, err := s.readVerifiedIndexLocked(key); err != nil {
		return AcceptanceResult{}, err
	} else if ok {
		if existing.BatchChecksum == checksum {
			return AcceptanceResult{AcceptedThrough: existing.AcceptedThrough}, nil
		}
		return AcceptanceResult{}, durableSequenceConflict(env.TenantID, env.AgentID, env.FirstSequence, env.LastSequence)
	}

	if s.overlapsAcceptedRangeLocked(env.TenantID, env.AgentID, env.FirstSequence, env.LastSequence) {
		return AcceptanceResult{}, durableSequenceConflict(env.TenantID, env.AgentID, env.FirstSequence, env.LastSequence)
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
		// The payload is already durable. Block subsequent acceptance until the
		// complete spool has been reconciled so retries cannot accumulate orphans.
		s.reconcileRequired = true
		return AcceptanceResult{}, err
	}
	s.addRangeLocked(entry)
	return AcceptanceResult{AcceptedThrough: env.LastSequence}, nil
}

func durableSequenceConflict(tenantID, agentID string, firstSequence, lastSequence uint64) error {
	return &DurableReplayConflictError{
		TenantID: tenantID,
		AgentID: agentID,
		FirstSequence: firstSequence,
		LastSequence: lastSequence,
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
	if entry.SpoolSequence == 0 {
		return batchIndexEntry{}, false, fmt.Errorf("%s: batch index missing durable spool sequence", ErrCodeSinkFailed)
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
		return fmt.Errorf("%s: failed to open batch index dir: %w", ErrCodeSinkFailed, err)
	}
	defer dirFile.Close()
	if err := dirFile.Sync(); err != nil {
		return fmt.Errorf("%s: failed to fsync batch index dir: %w", ErrCodeSinkFailed, err)
	}
	return nil
}

func (s *SpoolAcceptanceSink) rebuildIndexFromSpool() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reconcileAllLocked()
}

func (s *SpoolAcceptanceSink) reconcileAllLocked() error {
	// The spool is authoritative. Remove only derived JSON index entries, then
	// rebuild the complete index from every pending durable Agent record.
	entries, err := os.ReadDir(s.indexDir)
	if err != nil {
		return fmt.Errorf("%s: failed to list batch index directory: %w", ErrCodeSinkFailed, err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.indexDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("%s: failed to inspect batch index entry: %w", ErrCodeSinkFailed, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s: unsafe batch index entry type", ErrCodeSinkFailed)
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("%s: failed to clear derived batch index: %w", ErrCodeSinkFailed, err)
		}
	}
	if err := syncIndexDir(s.indexDir); err != nil {
		return err
	}

	seen := make(map[string]batchIndexEntry)
	ranges := make(map[string][]batchIndexEntry)
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
		if existing, ok := seen[key]; ok {
			if existing.BatchChecksum != payload.BatchChecksum {
				return durableSequenceConflict(payload.TenantID, payload.AgentID, payload.FirstSequence, payload.LastSequence)
			}
			// Exact duplicate already exists durably. Keep the earliest spool
			// record as canonical and do not create another index entry.
			return nil
		}

		stream := sourceRangeStreamKey(payload.TenantID, payload.AgentID)
		for _, existing := range ranges[stream] {
			if rangesOverlap(payload.FirstSequence, payload.LastSequence, existing.FirstSequence, existing.LastSequence) {
				return durableSequenceConflict(payload.TenantID, payload.AgentID, payload.FirstSequence, payload.LastSequence)
			}
		}

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
		seen[key] = entry
		ranges[stream] = append(ranges[stream], entry)
		if err := s.publishIndexLocked(key, entry); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		s.reconcileRequired = true
		return err
	}
	s.ranges = ranges
	s.reconcileRequired = false
	return nil
}
