package queue

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrQueueFull                  = errors.New("QUEUE_FULL: capacity limit reached")
	ErrRecordTooLarge             = errors.New("QUEUE_RECORD_TOO_LARGE: payload exceeds max allowed record size")
	ErrRecordCorrupted            = errors.New("QUEUE_RECORD_CORRUPT: record checksum mismatch")
	ErrInvalidDirectory           = errors.New("QUEUE_DIR_INVALID: queue directory is invalid or unsafe")
	ErrQuarantineFull             = errors.New("QUEUE_QUARANTINE_FULL: quarantine capacity limit reached")
	ErrDeliveryBlocked            = errors.New("QUEUE_DELIVERY_BLOCKED: quarantined sequence blocks later delivery")
	ErrQuarantineDestExists       = errors.New("QUEUE_QUARANTINE_DEST_EXISTS: refusing to overwrite quarantine record")
	ErrUnknownSequence            = errors.New("QUEUE_UNKNOWN_SEQUENCE_BLOCK: queue record filename has no safe sequence")
	ErrQuarantineUncertain        = errors.New("QUEUE_QUARANTINE_UNCERTAIN: durable record location cannot be confirmed")
	ErrQuarantineSourceSyncFailed = errors.New("QUEUE_QUARANTINE_SOURCE_SYNC_FAILED: quarantine destination durable; source dirsync failed")
)

const (
	recordSequenceWidth     = 19
	uncertainMarkerFile     = "location.uncertain"
	syncPhaseQuarantineDest = "quarantine-dest"
	syncPhaseActiveSource   = "active-source"
	syncPhaseRollbackDest   = "rollback-dest"
	syncPhaseRollbackSource = "rollback-source"
)

// syncHookFunc is a per-queue test seam for directory fsync phases. Production
// leaves this nil. Hooks must not be process-global mutable state.
type syncHookFunc func(phase, dir string) error

type Record struct {
	Sequence  uint64    `json:"sequence"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Checksum  string    `json:"checksum"`
	Payload   []byte    `json:"payload"`
}

type QueueStats struct {
	ActiveCount          int
	ActiveBytes          int64
	QuarantineCount      int
	QuarantineBytes      int64
	OldestAge            time.Duration
	LastSequence         uint64
	MaxRecords           int
	MaxBytes             int64
	MaxQuarantineRecords int
	MaxQuarantineBytes   int64
	AtCapacity           bool
	QuarantineAtCapacity bool
	BlockedFromSequence  uint64
	DeliveryBlocked      bool
	UnknownSequenceBlock bool
	LocationUncertain    bool
}

type DurableQueue struct {
	dir                  string
	activeDir            string
	quarantineDir        string
	metaPath             string
	maxRecords           int
	maxBytes             int64
	maxRecordBytes       int
	maxQuarantineRecords int
	maxQuarantineBytes   int64
	mu                   sync.Mutex
	lastSeq              uint64
	activeCount          int
	activeBytes          int64
	quarantineCount      int
	quarantineBytes      int64
	blockedFromSequence  uint64
	unknownSequenceBlock bool
	locationUncertain    bool
	syncHook             syncHookFunc
}

const sequenceMetaFile = "sequence.meta"

func NewDurableQueue(dir string, maxRecords int, maxBytes int64, maxRecordBytes int) (*DurableQueue, error) {
	if dir == "" {
		return nil, errors.New("QUEUE_DIR_REQUIRED: queue directory must be specified")
	}
	if maxRecords <= 0 {
		maxRecords = 1000
	}
	if maxBytes <= 0 {
		maxBytes = 50 * 1024 * 1024 // 50MB
	}
	if maxRecordBytes <= 0 {
		maxRecordBytes = 64 * 1024 // 64KB
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("QUEUE_PATH_INVALID: %w", err)
	}

	if err := ensureSafeDir(absDir); err != nil {
		return nil, err
	}

	activeDir := filepath.Join(absDir, "active")
	if err := ensureSafeDir(activeDir); err != nil {
		return nil, fmt.Errorf("QUEUE_MKDIR_ACTIVE_FAILED: %w", err)
	}

	quarantineDir := filepath.Join(absDir, "quarantine")
	if err := ensureSafeDir(quarantineDir); err != nil {
		return nil, fmt.Errorf("QUEUE_MKDIR_QUARANTINE_FAILED: %w", err)
	}

	q := &DurableQueue{
		dir:                  absDir,
		activeDir:            activeDir,
		quarantineDir:        quarantineDir,
		metaPath:             filepath.Join(absDir, sequenceMetaFile),
		maxRecords:           maxRecords,
		maxBytes:             maxBytes,
		maxRecordBytes:       maxRecordBytes,
		maxQuarantineRecords: maxRecords,
		maxQuarantineBytes:   maxBytes,
	}

	if err := q.recoverState(); err != nil {
		return nil, err
	}

	return q, nil
}

func ensureSafeDir(path string) error {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return errors.New("QUEUE_SYMLINK_REJECTED: queue directory is a symlink")
		}
		if !fi.IsDir() {
			return ErrInvalidDirectory
		}
		if fi.Mode().Perm()&0077 != 0 {
			if err := os.Chmod(path, 0700); err != nil {
				return fmt.Errorf("QUEUE_CHMOD_FAILED: %w", err)
			}
		}
		return nil
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0700); err != nil {
			return fmt.Errorf("QUEUE_MKDIR_FAILED: %w", err)
		}
		return nil
	} else {
		return fmt.Errorf("QUEUE_STAT_FAILED: %w", err)
	}
}

func (q *DurableQueue) recoverState() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	cleanTemp := func(d string) {
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tmp") || strings.Contains(e.Name(), ".tmp.") {
				_ = os.Remove(filepath.Join(d, e.Name()))
			}
		}
	}
	cleanTemp(q.dir)
	cleanTemp(q.activeDir)
	cleanTemp(q.quarantineDir)

	q.unknownSequenceBlock = false
	q.locationUncertain = false
	if err := q.loadUncertainMarkerLocked(); err != nil {
		return err
	}

	qHigh, err := q.scanQuarantineLocked()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(q.activeDir)
	if err != nil {
		return fmt.Errorf("QUEUE_SCAN_FAILED: %w", err)
	}

	q.activeCount = 0
	q.activeBytes = 0
	var highestSeq uint64
	if qHigh > highestSeq {
		highestSeq = qHigh
	}

	for _, e := range entries {
		if e.IsDir() || !isRecordJSON(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			return fmt.Errorf("QUEUE_RECOVER_STAT_FAILED: %w", err)
		}
		fileSeq, _, nameErr := parseRecordFileName(e.Name())
		if nameErr != nil {
			q.unknownSequenceBlock = true
			q.activeCount++
			q.activeBytes += fi.Size()
			continue
		}
		filePath := filepath.Join(q.activeDir, e.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("QUEUE_RECOVER_READ_FAILED: %w", err)
		}
		rec, _, parseErr := decodeRecordFile(e.Name(), data)
		if parseErr != nil {
			if _, destErr := os.Lstat(filepath.Join(q.quarantineDir, e.Name())); destErr == nil {
				q.activeCount++
				q.activeBytes += fi.Size()
				if fileSeq > highestSeq {
					highestSeq = fileSeq
				}
				continue
			}
			if qErr := q.quarantineRecordLocked(e.Name(), "QUEUE_RECORD_CORRUPT"); qErr != nil {
				if _, stillActive := os.Lstat(filePath); stillActive == nil {
					q.activeCount++
					q.activeBytes += fi.Size()
				}
				if fileSeq > highestSeq {
					highestSeq = fileSeq
				}
				if isFailClosedQuarantineErr(qErr) {
					continue
				}
				return qErr
			}
			if fileSeq > highestSeq {
				highestSeq = fileSeq
			}
			continue
		}
		_ = rec
		q.activeCount++
		q.activeBytes += fi.Size()
		if fileSeq > highestSeq {
			highestSeq = fileSeq
		}
	}

	if err := q.reconcileDuplicatesLocked(); err != nil {
		return err
	}

	durableWM, err := q.readSequenceWatermarkLocked()
	if err != nil {
		return err
	}

	// lastSeq = max(file highest, durable watermark) so drained queues retain lineage.
	q.lastSeq = highestSeq
	if durableWM > q.lastSeq {
		q.lastSeq = durableWM
	}
	return nil
}

type sequenceMeta struct {
	HighestSequence uint64 `json:"highestSequence"`
}

func (q *DurableQueue) readSequenceWatermarkLocked() (uint64, error) {
	data, err := os.ReadFile(q.metaPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("QUEUE_META_READ_FAILED: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return 0, errors.New("QUEUE_META_CORRUPT: sequence metadata is empty")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var meta sequenceMeta
	if err := dec.Decode(&meta); err != nil {
		return 0, fmt.Errorf("QUEUE_META_CORRUPT: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return 0, errors.New("QUEUE_META_CORRUPT: trailing data in sequence metadata")
	}
	return meta.HighestSequence, nil
}

func (q *DurableQueue) persistSequenceWatermarkLocked(seq uint64) error {
	meta := sequenceMeta{HighestSequence: seq}
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("QUEUE_META_MARSHAL_FAILED: %w", err)
	}
	tmp, err := os.CreateTemp(q.dir, "sequence.*.tmp")
	if err != nil {
		return fmt.Errorf("QUEUE_META_TMP_FAILED: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("QUEUE_META_CHMOD_FAILED: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("QUEUE_META_WRITE_FAILED: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("QUEUE_META_SYNC_FAILED: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("QUEUE_META_CLOSE_FAILED: %w", err)
	}
	if err := os.Rename(tmpName, q.metaPath); err != nil {
		return fmt.Errorf("QUEUE_META_RENAME_FAILED: %w", err)
	}
	if err := syncDir(q.dir); err != nil {
		return fmt.Errorf("QUEUE_META_DIR_SYNC_FAILED: %w", err)
	}
	return nil
}

func (q *DurableQueue) Enqueue(payload []byte) (*Record, error) {
	if len(payload) > q.maxRecordBytes {
		return nil, ErrRecordTooLarge
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.activeCount >= q.maxRecords {
		return nil, ErrQueueFull
	}

	q.lastSeq++
	seq := q.lastSeq

	recID := generateRecordID()
	now := time.Now().UTC()
	checksum := computeChecksum(seq, recID, now, payload)

	rec := &Record{
		Sequence:  seq,
		ID:        recID,
		CreatedAt: now,
		Checksum:  checksum,
		Payload:   payload,
	}

	data, err := json.Marshal(rec)
	if err != nil {
		q.lastSeq--
		return nil, fmt.Errorf("QUEUE_MARSHAL_FAILED: %w", err)
	}

	// Capacity is measured by serialized on-disk size, not payload-only length.
	if q.activeBytes+int64(len(data)) > q.maxBytes {
		q.lastSeq--
		return nil, ErrQueueFull
	}

	fileName := fmt.Sprintf("rec_%019d_%s.json", seq, recID)
	targetPath := filepath.Join(q.activeDir, fileName)

	tmpFile, err := os.CreateTemp(q.activeDir, "enqueue.*.tmp")
	if err != nil {
		q.lastSeq--
		return nil, fmt.Errorf("QUEUE_TMP_CREATE_FAILED: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		q.lastSeq--
		return nil, fmt.Errorf("QUEUE_CHMOD_FAILED: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		q.lastSeq--
		return nil, fmt.Errorf("QUEUE_WRITE_FAILED: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		q.lastSeq--
		return nil, fmt.Errorf("QUEUE_SYNC_FAILED: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		q.lastSeq--
		return nil, fmt.Errorf("QUEUE_CLOSE_FAILED: %w", err)
	}

	if err := os.Rename(tmpName, targetPath); err != nil {
		q.lastSeq--
		return nil, fmt.Errorf("QUEUE_RENAME_FAILED: %w", err)
	}

	if err := syncDir(q.activeDir); err != nil {
		// Record exists; surface durability warning as hard failure so callers
		// do not assume durability that the platform could not confirm.
		q.lastSeq--
		_ = os.Remove(targetPath)
		return nil, fmt.Errorf("QUEUE_DIR_SYNC_FAILED: %w", err)
	}

	if err := q.persistSequenceWatermarkLocked(seq); err != nil {
		q.lastSeq--
		_ = os.Remove(targetPath)
		return nil, err
	}

	q.activeCount++
	q.activeBytes += int64(len(data))

	return rec, nil
}

func (q *DurableQueue) ReadBatch(maxRecords int, maxBytes int64) ([]Record, error) {
	if maxRecords <= 0 {
		maxRecords = 100
	}
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.locationUncertain {
		return nil, ErrQuarantineUncertain
	}

	entries, err := os.ReadDir(q.activeDir)
	if err != nil {
		return nil, fmt.Errorf("QUEUE_READ_DIR_FAILED: %w", err)
	}

	var fileNames []string
	for _, e := range entries {
		if !e.IsDir() && isRecordJSON(e.Name()) {
			fileNames = append(fileNames, e.Name())
		}
	}
	sort.Strings(fileNames)

	for _, name := range fileNames {
		if _, _, nameErr := parseRecordFileName(name); nameErr != nil {
			q.unknownSequenceBlock = true
			return nil, ErrUnknownSequence
		}
	}
	if q.unknownSequenceBlock {
		return nil, ErrUnknownSequence
	}

	var batch []Record
	var currentBytes int64
	blocked := q.blockedFromSequence

	for _, name := range fileNames {
		if len(batch) >= maxRecords {
			break
		}

		fileSeq, _, nameErr := parseRecordFileName(name)
		if nameErr != nil {
			q.unknownSequenceBlock = true
			return nil, ErrUnknownSequence
		}
		if blocked > 0 && fileSeq > blocked {
			break
		}

		dup, dupErr := q.duplicateStateLocked(name)
		if dupErr != nil {
			return nil, dupErr
		}
		if dup {
			q.noteBlockedLocked(fileSeq)
			break
		}

		filePath := filepath.Join(q.activeDir, name)
		data, err := os.ReadFile(filePath)
		if err != nil {
			if len(batch) > 0 {
				return batch, nil
			}
			return nil, fmt.Errorf("QUEUE_READ_FAILED: %w", err)
		}

		if currentBytes+int64(len(data)) > maxBytes && len(batch) > 0 {
			break
		}

		rec, _, parseErr := decodeRecordFile(name, data)
		if parseErr != nil {
			if qErr := q.quarantineRecordLocked(name, "QUEUE_RECORD_CORRUPT"); qErr != nil {
				if q.locationUncertain || errors.Is(qErr, ErrQuarantineUncertain) || errors.Is(qErr, ErrUnknownSequence) {
					return nil, qErr
				}
				if len(batch) > 0 {
					return batch, nil
				}
				return nil, qErr
			}
			break
		}

		if len(batch) > 0 && rec.Sequence != batch[len(batch)-1].Sequence+1 {
			// Missing intermediate sequence in active/. Do not assemble a gapped
			// batch and do not invent a skip of the hole.
			break
		}

		batch = append(batch, *rec)
		currentBytes += int64(len(data))
	}

	if err := q.errIfDeliveryBlockedLocked(len(batch) > 0); err != nil {
		return nil, err
	}
	return batch, nil
}

func decodeRecordFile(name string, data []byte) (*Record, uint64, error) {
	fileSeq, idFromName, err := parseRecordFileName(name)
	if err != nil {
		return nil, 0, err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	var rec Record
	if err := dec.Decode(&rec); err != nil {
		return nil, fileSeq, fmt.Errorf("%w: %v", ErrRecordCorrupted, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fileSeq, fmt.Errorf("%w: trailing JSON garbage", ErrRecordCorrupted)
	}

	if rec.Sequence != fileSeq {
		return nil, fileSeq, fmt.Errorf("%w: sequence/filename mismatch", ErrRecordCorrupted)
	}
	if rec.ID != idFromName {
		return nil, fileSeq, fmt.Errorf("%w: id/filename mismatch", ErrRecordCorrupted)
	}

	computed := computeChecksum(rec.Sequence, rec.ID, rec.CreatedAt, rec.Payload)
	if computed != rec.Checksum {
		return nil, fileSeq, ErrRecordCorrupted
	}
	if len(rec.Payload) == 0 {
		return nil, fileSeq, fmt.Errorf("%w: empty payload", ErrRecordCorrupted)
	}
	if rec.CreatedAt.IsZero() {
		return nil, fileSeq, fmt.Errorf("%w: zero createdAt", ErrRecordCorrupted)
	}
	return &rec, fileSeq, nil
}

func parseRecordFileName(name string) (uint64, string, error) {
	// Canonical: rec_<19-digit-seq>_<id>.json
	if !strings.HasPrefix(name, "rec_") || !strings.HasSuffix(name, ".json") {
		return 0, "", fmt.Errorf("%w: bad filename", ErrRecordCorrupted)
	}
	body := strings.TrimSuffix(strings.TrimPrefix(name, "rec_"), ".json")
	parts := strings.SplitN(body, "_", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, "", fmt.Errorf("%w: bad filename parts", ErrRecordCorrupted)
	}
	if len(parts[0]) != recordSequenceWidth {
		return 0, "", fmt.Errorf("%w: noncanonical sequence width", ErrRecordCorrupted)
	}
	for i := 0; i < len(parts[0]); i++ {
		c := parts[0][i]
		if c < '0' || c > '9' {
			return 0, "", fmt.Errorf("%w: bad sequence", ErrRecordCorrupted)
		}
	}
	seq, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("%w: bad sequence", ErrRecordCorrupted)
	}
	return seq, parts[1], nil
}

func computeChecksum(seq uint64, id string, createdAt time.Time, payload []byte) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%d\n%s\n%s\n", seq, id, createdAt.UTC().Format(time.RFC3339Nano))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func (q *DurableQueue) scanQuarantineLocked() (uint64, error) {
	qEntries, err := os.ReadDir(q.quarantineDir)
	if err != nil {
		return 0, fmt.Errorf("QUEUE_SCAN_QUARANTINE_FAILED: %w", err)
	}
	q.quarantineCount = 0
	q.quarantineBytes = 0
	q.blockedFromSequence = 0
	var highest uint64
	for _, e := range qEntries {
		if e.IsDir() || !isRecordJSON(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			return 0, fmt.Errorf("QUEUE_SCAN_QUARANTINE_STAT_FAILED: %w", err)
		}
		fileSeq, _, nameErr := parseRecordFileName(e.Name())
		if nameErr != nil {
			q.unknownSequenceBlock = true
			q.quarantineCount++
			q.quarantineBytes += fi.Size()
			continue
		}
		q.quarantineCount++
		q.quarantineBytes += fi.Size()
		q.noteBlockedLocked(fileSeq)
		if fileSeq > highest {
			highest = fileSeq
		}
	}
	return highest, nil
}

func (q *DurableQueue) noteBlockedLocked(seq uint64) {
	if seq == 0 {
		return
	}
	if q.blockedFromSequence == 0 || seq < q.blockedFromSequence {
		q.blockedFromSequence = seq
	}
}

func (q *DurableQueue) errIfDeliveryBlockedLocked(hasPrefix bool) error {
	if q.locationUncertain {
		return ErrQuarantineUncertain
	}
	if q.unknownSequenceBlock {
		return ErrUnknownSequence
	}
	if hasPrefix || q.blockedFromSequence == 0 {
		return nil
	}
	if q.hasActiveAfterBlockedLocked() {
		return ErrDeliveryBlocked
	}
	return nil
}

func (q *DurableQueue) hasActiveAfterBlockedLocked() bool {
	if q.blockedFromSequence == 0 {
		return false
	}
	entries, err := os.ReadDir(q.activeDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "rec_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		seq, _, err := parseRecordFileName(e.Name())
		if err != nil {
			continue
		}
		if seq > q.blockedFromSequence {
			return true
		}
	}
	return false
}

func (q *DurableQueue) quarantineCapacityLocked(additionalBytes int64) error {
	if q.quarantineCount >= q.maxQuarantineRecords {
		return ErrQuarantineFull
	}
	if q.quarantineBytes+additionalBytes > q.maxQuarantineBytes {
		return ErrQuarantineFull
	}
	return nil
}

func sanitizeQuarantineCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return "QUEUE_RECORD_CORRUPT"
	}
	var b strings.Builder
	for _, r := range code {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "QUEUE_RECORD_CORRUPT"
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func (q *DurableQueue) writeQuarantineReason(recordName, code string) {
	reasonName := strings.TrimSuffix(recordName, ".json") + ".reason"
	if !strings.HasPrefix(reasonName, "rec_") || strings.HasSuffix(reasonName, ".json") {
		return
	}
	path := filepath.Join(q.quarantineDir, reasonName)
	payload := []byte(`{"code":"` + sanitizeQuarantineCode(code) + `"}`)
	tmp, err := os.CreateTemp(q.quarantineDir, "reason.*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(tmpName, path)
}

func (q *DurableQueue) quarantineRecordLocked(name, code string) error {
	oldPath := filepath.Join(q.activeDir, name)
	newPath := filepath.Join(q.quarantineDir, name)

	fi, err := os.Lstat(oldPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("QUEUE_QUARANTINE_STAT_FAILED: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return errors.New("QUEUE_QUARANTINE_SYMLINK_REJECTED: active record path is a symlink")
	}

	destInfo, destErr := os.Lstat(newPath)
	if destErr == nil {
		if destInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("QUEUE_QUARANTINE_SYMLINK_REJECTED: quarantine record path is a symlink")
		}
		return ErrQuarantineDestExists
	}
	if !errors.Is(destErr, os.ErrNotExist) {
		return fmt.Errorf("QUEUE_QUARANTINE_DEST_STAT_FAILED: %w", destErr)
	}

	size := fi.Size()
	if err := q.quarantineCapacityLocked(size); err != nil {
		return err
	}

	fileSeq, _, parseErr := parseRecordFileName(name)
	if parseErr != nil {
		q.unknownSequenceBlock = true
		return ErrUnknownSequence
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("QUEUE_QUARANTINE_FAILED: %w", err)
	}

	if err := q.syncPhase(syncPhaseQuarantineDest, q.quarantineDir); err != nil {
		return q.rollbackQuarantineMoveLocked(name, err)
	}
	if err := q.syncPhase(syncPhaseActiveSource, q.activeDir); err != nil {
		// Destination directory entry is durable. Do not reverse: a rollback
		// could drop the only durable copy. Quarantine is authoritative.
		q.applyQuarantineCountersLocked(size, fileSeq)
		return fmt.Errorf("%w: %v", ErrQuarantineSourceSyncFailed, err)
	}

	q.applyQuarantineCountersLocked(size, fileSeq)
	q.writeQuarantineReason(name, code)
	return nil
}

func (q *DurableQueue) applyQuarantineCountersLocked(size int64, fileSeq uint64) {
	q.activeCount--
	q.activeBytes -= size
	if q.activeCount < 0 {
		q.activeCount = 0
	}
	if q.activeBytes < 0 {
		q.activeBytes = 0
	}
	q.quarantineCount++
	q.quarantineBytes += size
	q.noteBlockedLocked(fileSeq)
}

func (q *DurableQueue) rollbackQuarantineMoveLocked(name string, destSyncErr error) error {
	activePath := filepath.Join(q.activeDir, name)
	quarantinePath := filepath.Join(q.quarantineDir, name)
	if err := os.Rename(quarantinePath, activePath); err != nil {
		return q.enterUncertainLocked(fmt.Errorf("rollback rename failed: %w", err))
	}
	if err := q.syncPhase(syncPhaseRollbackDest, q.activeDir); err != nil {
		return q.enterUncertainLocked(fmt.Errorf("rollback dest sync failed: %w", err))
	}
	if err := q.syncPhase(syncPhaseRollbackSource, q.quarantineDir); err != nil {
		// Rollback destination (active/) is durable. Do not reverse again.
		return fmt.Errorf("QUEUE_QUARANTINE_DIR_SYNC_FAILED: %w", destSyncErr)
	}
	return fmt.Errorf("QUEUE_QUARANTINE_DIR_SYNC_FAILED: %w", destSyncErr)
}

func (q *DurableQueue) syncPhase(phase, dir string) error {
	if q.syncHook != nil {
		if err := q.syncHook(phase, dir); err != nil {
			return err
		}
	}
	return syncDir(dir)
}

func (q *DurableQueue) enterUncertainLocked(cause error) error {
	q.locationUncertain = true
	_ = q.persistUncertainMarkerLocked()
	return fmt.Errorf("%w: %v", ErrQuarantineUncertain, cause)
}

func (q *DurableQueue) persistUncertainMarkerLocked() error {
	path := filepath.Join(q.dir, uncertainMarkerFile)
	payload := []byte(`{"code":"QUEUE_QUARANTINE_UNCERTAIN"}`)
	tmp, err := os.CreateTemp(q.dir, "uncertain.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return syncDir(q.dir)
}

func (q *DurableQueue) loadUncertainMarkerLocked() error {
	path := filepath.Join(q.dir, uncertainMarkerFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("QUEUE_UNCERTAIN_MARKER_READ_FAILED: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		q.locationUncertain = true
		return nil
	}
	var meta map[string]string
	if err := json.Unmarshal(data, &meta); err != nil {
		q.locationUncertain = true
		return nil
	}
	if meta["code"] != "QUEUE_QUARANTINE_UNCERTAIN" {
		q.locationUncertain = true
		return nil
	}
	q.locationUncertain = true
	return nil
}

func (q *DurableQueue) duplicateStateLocked(name string) (bool, error) {
	qPath := filepath.Join(q.quarantineDir, name)
	qInfo, err := os.Lstat(qPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("QUEUE_DUPLICATE_STAT_FAILED: %w", err)
	}
	if qInfo.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("QUEUE_QUARANTINE_SYMLINK_REJECTED: quarantine record path is a symlink")
	}
	aPath := filepath.Join(q.activeDir, name)
	aData, err := os.ReadFile(aPath)
	if err != nil {
		return false, fmt.Errorf("QUEUE_DUPLICATE_ACTIVE_READ_FAILED: %w", err)
	}
	qData, err := os.ReadFile(qPath)
	if err != nil {
		return false, fmt.Errorf("QUEUE_DUPLICATE_QUARANTINE_READ_FAILED: %w", err)
	}
	if !bytes.Equal(aData, qData) {
		return false, q.enterUncertainLocked(errors.New("active and quarantine copies conflict"))
	}
	return true, nil
}

func (q *DurableQueue) reconcileDuplicatesLocked() error {
	entries, err := os.ReadDir(q.activeDir)
	if err != nil {
		return fmt.Errorf("QUEUE_SCAN_FAILED: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !isRecordJSON(e.Name()) {
			continue
		}
		if _, destErr := os.Lstat(filepath.Join(q.quarantineDir, e.Name())); destErr != nil {
			continue
		}
		dup, dupErr := q.duplicateStateLocked(e.Name())
		if dupErr != nil {
			if errors.Is(dupErr, ErrQuarantineUncertain) {
				continue
			}
			return dupErr
		}
		if dup {
			if fileSeq, _, parseErr := parseRecordFileName(e.Name()); parseErr == nil {
				q.noteBlockedLocked(fileSeq)
			}
		}
	}
	return nil
}

func isRecordJSON(name string) bool {
	return strings.HasPrefix(name, "rec_") && strings.HasSuffix(name, ".json")
}

func isFailClosedQuarantineErr(err error) bool {
	return errors.Is(err, ErrQuarantineUncertain) ||
		errors.Is(err, ErrQuarantineSourceSyncFailed) ||
		errors.Is(err, ErrQuarantineDestExists) ||
		errors.Is(err, ErrQuarantineFull) ||
		errors.Is(err, ErrUnknownSequence)
}

func (q *DurableQueue) DeleteThrough(seq uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	entries, err := os.ReadDir(q.activeDir)
	if err != nil {
		return fmt.Errorf("QUEUE_DELETE_SCAN_FAILED: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "rec_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		fileSeq, _, err := parseRecordFileName(e.Name())
		if err != nil || fileSeq > seq {
			continue
		}

		filePath := filepath.Join(q.activeDir, e.Name())
		fi, err := e.Info()
		if err != nil {
			return fmt.Errorf("QUEUE_DELETE_STAT_FAILED: %w", err)
		}
		if err := os.Remove(filePath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("QUEUE_DELETE_FAILED: %w", err)
		}
		q.activeCount--
		q.activeBytes -= fi.Size()
	}

	if q.activeCount < 0 {
		q.activeCount = 0
	}
	if q.activeBytes < 0 {
		q.activeBytes = 0
	}

	if err := syncDir(q.activeDir); err != nil {
		return fmt.Errorf("QUEUE_DELETE_DIR_SYNC_FAILED: %w", err)
	}

	return nil
}

func (q *DurableQueue) Stats() QueueStats {
	q.mu.Lock()
	defer q.mu.Unlock()

	var oldestAge time.Duration
	entries, err := os.ReadDir(q.activeDir)
	if err == nil {
		var minMod time.Time
		for _, e := range entries {
			if !e.IsDir() && strings.HasPrefix(e.Name(), "rec_") {
				if fi, err := e.Info(); err == nil {
					if minMod.IsZero() || fi.ModTime().Before(minMod) {
						minMod = fi.ModTime()
					}
				}
			}
		}
		if !minMod.IsZero() {
			oldestAge = time.Since(minMod)
		}
	}

	atCapacity := q.activeCount >= q.maxRecords || q.activeBytes >= q.maxBytes
	quarantineAtCapacity := q.quarantineCount >= q.maxQuarantineRecords || q.quarantineBytes >= q.maxQuarantineBytes
	return QueueStats{
		ActiveCount:          q.activeCount,
		ActiveBytes:          q.activeBytes,
		QuarantineCount:      q.quarantineCount,
		QuarantineBytes:      q.quarantineBytes,
		OldestAge:            oldestAge,
		LastSequence:         q.lastSeq,
		MaxRecords:           q.maxRecords,
		MaxBytes:             q.maxBytes,
		MaxQuarantineRecords: q.maxQuarantineRecords,
		MaxQuarantineBytes:   q.maxQuarantineBytes,
		AtCapacity:           atCapacity,
		QuarantineAtCapacity: quarantineAtCapacity,
		BlockedFromSequence:  q.blockedFromSequence,
		DeliveryBlocked:      q.unknownSequenceBlock || q.locationUncertain || q.hasActiveAfterBlockedLocked(),
		UnknownSequenceBlock: q.unknownSequenceBlock,
		LocationUncertain:    q.locationUncertain,
	}
}

func (q *DurableQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.activeCount
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func generateRecordID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("rec-%d", time.Now().UnixNano())
	}
	return "rec-" + hex.EncodeToString(b)
}
