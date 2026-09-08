package queue

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrQueueFull        = errors.New("QUEUE_FULL: capacity limit reached")
	ErrRecordTooLarge   = errors.New("QUEUE_RECORD_TOO_LARGE: payload exceeds max allowed record size")
	ErrRecordCorrupted  = errors.New("QUEUE_RECORD_CORRUPT: record checksum mismatch")
	ErrInvalidDirectory = errors.New("QUEUE_DIR_INVALID: queue directory is invalid or unsafe")
)

type Record struct {
	Sequence  uint64    `json:"sequence"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Checksum  string    `json:"checksum"`
	Payload   []byte    `json:"payload"`
}

type QueueStats struct {
	ActiveCount     int
	ActiveBytes     int64
	QuarantineCount int
	OldestAge       time.Duration
	LastSequence    uint64
	MaxRecords      int
	MaxBytes        int64
	AtCapacity      bool
}

type DurableQueue struct {
	dir             string
	activeDir       string
	quarantineDir   string
	maxRecords      int
	maxBytes        int64
	maxRecordBytes  int
	mu              sync.Mutex
	lastSeq         uint64
	activeCount     int
	activeBytes     int64
	quarantineCount int
}

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
		dir:            absDir,
		activeDir:      activeDir,
		quarantineDir:  quarantineDir,
		maxRecords:     maxRecords,
		maxBytes:       maxBytes,
		maxRecordBytes: maxRecordBytes,
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

	qEntries, err := os.ReadDir(q.quarantineDir)
	if err != nil {
		return fmt.Errorf("QUEUE_SCAN_QUARANTINE_FAILED: %w", err)
	}
	q.quarantineCount = 0
	for _, e := range qEntries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			q.quarantineCount++
		}
	}

	entries, err := os.ReadDir(q.activeDir)
	if err != nil {
		return fmt.Errorf("QUEUE_SCAN_FAILED: %w", err)
	}

	q.activeCount = 0
	q.activeBytes = 0
	var highestSeq uint64

	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "rec_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		filePath := filepath.Join(q.activeDir, e.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		rec, fileSeq, parseErr := decodeRecordFile(e.Name(), data)
		if parseErr != nil {
			if qErr := q.quarantineRecordLocked(e.Name()); qErr != nil {
				return qErr
			}
			continue
		}
		_ = rec
		fi, err := e.Info()
		if err != nil {
			continue
		}
		q.activeCount++
		q.activeBytes += fi.Size()
		if fileSeq > highestSeq {
			highestSeq = fileSeq
		}
	}

	q.lastSeq = highestSeq
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

	entries, err := os.ReadDir(q.activeDir)
	if err != nil {
		return nil, fmt.Errorf("QUEUE_READ_DIR_FAILED: %w", err)
	}

	var fileNames []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "rec_") && strings.HasSuffix(e.Name(), ".json") {
			fileNames = append(fileNames, e.Name())
		}
	}
	sort.Strings(fileNames)

	var batch []Record
	var currentBytes int64

	for _, name := range fileNames {
		if len(batch) >= maxRecords {
			break
		}

		filePath := filepath.Join(q.activeDir, name)
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		if currentBytes+int64(len(data)) > maxBytes && len(batch) > 0 {
			break
		}

		rec, _, parseErr := decodeRecordFile(name, data)
		if parseErr != nil {
			if qErr := q.quarantineRecordLocked(name); qErr != nil {
				return nil, qErr
			}
			continue
		}

		batch = append(batch, *rec)
		currentBytes += int64(len(data))
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
	if dec.More() {
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
	return &rec, fileSeq, nil
}

func parseRecordFileName(name string) (uint64, string, error) {
	// rec_<19-digit-seq>_<id>.json
	if !strings.HasPrefix(name, "rec_") || !strings.HasSuffix(name, ".json") {
		return 0, "", fmt.Errorf("%w: bad filename", ErrRecordCorrupted)
	}
	body := strings.TrimSuffix(strings.TrimPrefix(name, "rec_"), ".json")
	parts := strings.SplitN(body, "_", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, "", fmt.Errorf("%w: bad filename parts", ErrRecordCorrupted)
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

func (q *DurableQueue) quarantineRecordLocked(name string) error {
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

	size := fi.Size()
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("QUEUE_QUARANTINE_FAILED: %w", err)
	}
	if err := syncDir(q.activeDir); err != nil {
		// Best-effort restore so corrupt data is not silently vanished mid-flight.
		_ = os.Rename(newPath, oldPath)
		return fmt.Errorf("QUEUE_QUARANTINE_DIR_SYNC_FAILED: %w", err)
	}

	q.activeCount--
	q.activeBytes -= size
	if q.activeCount < 0 {
		q.activeCount = 0
	}
	if q.activeBytes < 0 {
		q.activeBytes = 0
	}
	q.quarantineCount++
	return nil
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
	return QueueStats{
		ActiveCount:     q.activeCount,
		ActiveBytes:     q.activeBytes,
		QuarantineCount: q.quarantineCount,
		OldestAge:       oldestAge,
		LastSequence:    q.lastSeq,
		MaxRecords:      q.maxRecords,
		MaxBytes:        q.maxBytes,
		AtCapacity:      atCapacity,
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
