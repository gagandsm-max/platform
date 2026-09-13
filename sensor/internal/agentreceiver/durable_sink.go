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
	agentBatchRecordType      = "agent.delivery.batch"
	batchIndexDirName         = "batch-index"
	defaultMaxDrainedReceipts = 4096
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
// Sensor restart so lost-ACK retries remain idempotent. It is not rebuilt solely
// from currently pending spool files.
type SpoolAcceptanceSink struct {
	spool              *spool.DiskSpool
	indexDir           string
	mu                 sync.Mutex
	reconcileRequired  bool
	ranges             map[string][]batchIndexEntry
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
	sink := &SpoolAcceptanceSink{
		spool:              agentSpool,
		indexDir:           indexDir,
		ranges:             make(map[string][]batchIndexEntry),
		maxDrainedReceipts: defaultMaxDrainedReceipts,
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
	if err := s.enforceDrainedReceiptBoundLocked(sourceRangeStreamKey(env.TenantID, env.AgentID)); err != nil {
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
	acked, err := s.spool.AckedThrough()
	if err != nil {
		s.reconcileRequired = true
		return fmt.Errorf("%s: failed to read spool acknowledgement watermark: %w", ErrCodeSinkFailed, err)
	}

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
	}

	s.ranges = ranges
	for stream := range s.ranges {
		if err := s.enforceDrainedReceiptBoundLocked(stream); err != nil {
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

func (s *SpoolAcceptanceSink) streamHighWaterLocked(stream string) uint64 {
	var max uint64
	for _, existing := range s.ranges[stream] {
		if existing.LastSequence > max {
			max = existing.LastSequence
		}
	}
	return max
}

func (s *SpoolAcceptanceSink) enforceDrainedReceiptBoundLocked(stream string) error {
	limit := s.maxDrainedReceipts
	if limit <= 0 {
		limit = defaultMaxDrainedReceipts
	}
	acked, err := s.spool.AckedThrough()
	if err != nil {
		return fmt.Errorf("%s: failed to read spool acknowledgement watermark: %w", ErrCodeSinkFailed, err)
	}
	var drained []batchIndexEntry
	var retained []batchIndexEntry
	for _, entry := range s.ranges[stream] {
		if entry.SpoolSequence > 0 && entry.SpoolSequence <= acked {
			drained = append(drained, entry)
			continue
		}
		retained = append(retained, entry)
	}
	if len(drained) <= limit {
		return nil
	}
	sort.Slice(drained, func(i, j int) bool {
		if drained[i].FirstSequence == drained[j].FirstSequence {
			return drained[i].LastSequence < drained[j].LastSequence
		}
		return drained[i].FirstSequence < drained[j].FirstSequence
	})
	highWater := uint64(0)
	for _, entry := range append(append([]batchIndexEntry{}, retained...), drained...) {
		if entry.LastSequence > highWater {
			highWater = entry.LastSequence
		}
	}
	for len(drained) > limit {
		removed := false
		highWaterCopies := 0
		for _, entry := range drained {
			if entry.LastSequence == highWater {
				highWaterCopies++
			}
		}
		for i, entry := range drained {
			if entry.LastSequence == highWater && highWaterCopies <= 1 {
				continue
			}
			key := batchIndexKey(entry.TenantID, entry.AgentID, entry.FirstSequence, entry.LastSequence)
			if err := s.removeIndexLocked(key); err != nil {
				return err
			}
			drained = append(drained[:i], drained[i+1:]...)
			removed = true
			break
		}
		if !removed {
			break
		}
	}
	s.ranges[stream] = append(retained, drained...)
	return nil
}

func (s *SpoolAcceptanceSink) removeIndexLocked(key string) error {
	path := s.indexPath(key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%s: failed to evict drained batch index: %w", ErrCodeSinkFailed, err)
	}
	return syncIndexDir(s.indexDir)
}
