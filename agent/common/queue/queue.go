package queue

import (
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

	// Verify or create root queue dir
	if fi, err := os.Lstat(absDir); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("QUEUE_SYMLINK_REJECTED: queue directory is a symlink")
		}
		if !fi.IsDir() {
			return nil, ErrInvalidDirectory
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(absDir, 0700); err != nil {
			return nil, fmt.Errorf("QUEUE_MKDIR_FAILED: %w", err)
		}
	} else {
		return nil, fmt.Errorf("QUEUE_STAT_FAILED: %w", err)
	}

	activeDir := filepath.Join(absDir, "active")
	if err := os.MkdirAll(activeDir, 0700); err != nil {
		return nil, fmt.Errorf("QUEUE_MKDIR_ACTIVE_FAILED: %w", err)
	}

	quarantineDir := filepath.Join(absDir, "quarantine")
	if err := os.MkdirAll(quarantineDir, 0700); err != nil {
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

func (q *DurableQueue) recoverState() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 1. Clean up stray temp files in queue dir and active dir
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

	// 2. Count existing quarantine records
	qEntries, _ := os.ReadDir(q.quarantineDir)
	q.quarantineCount = 0
	for _, e := range qEntries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			q.quarantineCount++
		}
	}

	// 3. Scan active records
	entries, err := os.ReadDir(q.activeDir)
	if err != nil {
		return fmt.Errorf("QUEUE_SCAN_FAILED: %w", err)
	}

	q.activeCount = 0
	q.activeBytes = 0
	var highestSeq uint64 = 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		q.activeCount++
		q.activeBytes += fi.Size()

		// Parse sequence from filename: rec_0000000000000000001_xxx.json
		parts := strings.Split(e.Name(), "_")
		if len(parts) >= 2 {
			if seq, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
				if seq > highestSeq {
					highestSeq = seq
				}
			}
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

	// Enforce capacity bounds
	if q.activeCount >= q.maxRecords {
		return nil, ErrQueueFull
	}
	if q.activeBytes+int64(len(payload)) > q.maxBytes {
		return nil, ErrQueueFull
	}

	q.lastSeq++
	seq := q.lastSeq

	recID := generateRecordID()
	now := time.Now().UTC()

	h := sha256.New()
	h.Write(payload)
	checksum := hex.EncodeToString(h.Sum(nil))

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

	fileName := fmt.Sprintf("rec_%019d_%s.json", seq, recID)
	targetPath := filepath.Join(q.activeDir, fileName)

	// Atomic write: write to temp file then rename
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

	// Filter and sort by filename (which starts with zero-padded sequence)
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

		var rec Record
		if err := json.Unmarshal(data, &rec); err != nil {
			// Corrupt JSON: quarantine
			q.quarantineRecord(name)
			continue
		}

		// Verify SHA-256 checksum
		h := sha256.New()
		h.Write(rec.Payload)
		computedCheck := hex.EncodeToString(h.Sum(nil))
		if computedCheck != rec.Checksum {
			// Corrupt payload checksum: quarantine
			q.quarantineRecord(name)
			continue
		}

		batch = append(batch, rec)
		currentBytes += int64(len(data))
	}

	return batch, nil
}

func (q *DurableQueue) quarantineRecord(name string) {
	oldPath := filepath.Join(q.activeDir, name)
	newPath := filepath.Join(q.quarantineDir, name)
	if fi, err := os.Stat(oldPath); err == nil {
		q.activeCount--
		q.activeBytes -= fi.Size()
		if q.activeCount < 0 {
			q.activeCount = 0
		}
		if q.activeBytes < 0 {
			q.activeBytes = 0
		}
	}
	_ = os.Rename(oldPath, newPath)
	q.quarantineCount++
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

		parts := strings.Split(e.Name(), "_")
		if len(parts) >= 2 {
			if s, err := strconv.ParseUint(parts[1], 10, 64); err == nil && s <= seq {
				filePath := filepath.Join(q.activeDir, e.Name())
				if fi, err := e.Info(); err == nil {
					q.activeCount--
					q.activeBytes -= fi.Size()
				}
				_ = os.Remove(filePath)
			}
		}
	}

	if q.activeCount < 0 {
		q.activeCount = 0
	}
	if q.activeBytes < 0 {
		q.activeBytes = 0
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

	return QueueStats{
		ActiveCount:     q.activeCount,
		ActiveBytes:     q.activeBytes,
		QuarantineCount: q.quarantineCount,
		OldestAge:       oldestAge,
		LastSequence:    q.lastSeq,
	}
}

func (q *DurableQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.activeCount
}

func generateRecordID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("rec-%d", time.Now().UnixNano())
	}
	return "rec-" + hex.EncodeToString(b)
}
