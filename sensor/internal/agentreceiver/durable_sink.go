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

// DurableReplayConflictError is returned when a durable batch index detects a
// conflicting replay for the same (tenant, agent, batchID) key.
type DurableReplayConflictError struct {
	TenantID string
	AgentID  string
	BatchID  string
}

func (e *DurableReplayConflictError) Error() string {
	return ErrCodeReplayConflict + ": durable batch index conflict"
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
	SpoolSequence    uint64 `json:"spoolSequence,omitempty"`
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
// directory. It is isolated from the observation telemetry spool/uploader path.
type SpoolAcceptanceSink struct {
	spool    *spool.DiskSpool
	indexDir string
	mu       sync.Mutex
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
	sink := &SpoolAcceptanceSink{spool: agentSpool, indexDir: indexDir}
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

	checksum := ComputeBatchChecksum(env.Records)
	key := batchIndexKey(env.TenantID, env.AgentID, env.BatchID)
	if existing, ok, err := s.readIndexLocked(key); err != nil {
		return AcceptanceResult{}, err
	} else if ok {
		if existing.FirstSequence == env.FirstSequence &&
			existing.LastSequence == env.LastSequence &&
			existing.BatchChecksum == checksum {
			return AcceptanceResult{AcceptedThrough: existing.AcceptedThrough}, nil
		}
		return AcceptanceResult{}, &DurableReplayConflictError{
			TenantID: env.TenantID,
			AgentID:  env.AgentID,
			BatchID:  env.BatchID,
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
	if err := s.writeIndexLocked(key, entry); err != nil {
		// Payload is durable but index is not; refuse ACK so Agent retries.
		return AcceptanceResult{}, err
	}
	return AcceptanceResult{AcceptedThrough: env.LastSequence}, nil
}

func batchIndexKey(tenantID, agentID, batchID string) string {
	var b []byte
	b = appendLenPrefixed(b, tenantID)
	b = appendLenPrefixed(b, agentID)
	b = appendLenPrefixed(b, batchID)
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
	dirFile, err := os.Open(s.indexDir)
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
	// Best-effort rebuild for restart safety: scan pending spool records and
	// ensure index entries exist for agent delivery batches.
	const maxRecords = 1024
	const maxBytes = 64 * 1024 * 1024
	batch, err := s.spool.ReadBatch(maxRecords, maxBytes)
	if err != nil {
		// Empty / no pending is fine; other errors surface.
		if spool.CodeOf(err) == spool.CodeSequence || spool.CodeOf(err) == spool.CodeCorruption {
			return fmt.Errorf("%s: agent spool recovery failed: %w", ErrCodeSinkFailed, err)
		}
		// ReadBatch with no pending records returns empty batch + nil.
	}
	for _, rec := range batch.Records {
		if rec.Type != agentBatchRecordType {
			continue
		}
		var payload durableBatchPayload
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			continue
		}
		key := batchIndexKey(payload.TenantID, payload.AgentID, payload.BatchID)
		if _, ok, err := s.readIndexLocked(key); err != nil {
			return err
		} else if ok {
			continue
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
		if err := s.writeIndexLocked(key, entry); err != nil {
			return err
		}
	}
	return nil
}
