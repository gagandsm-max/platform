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
	"regexp"
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
	ErrSequenceGap                = errors.New("QUEUE_SEQUENCE_GAP: missing unacknowledged sequence blocks delivery")
	ErrSequenceConflict           = errors.New("QUEUE_SEQUENCE_CONFLICT: multiple active records share a sequence")
	ErrMetaUncertain              = errors.New("QUEUE_META_UNCERTAIN: durable sequence metadata state cannot be confirmed")
	ErrAckRecoveryConflict        = errors.New("QUEUE_ACK_RECOVERY_CONFLICT: unauthorized or conflicting record at or below acceptedThrough")
	ErrPendingAckCleanup          = errors.New("QUEUE_PENDING_ACK_CLEANUP: prior ACK cleanup not yet durable")
	ErrSequenceExhausted          = errors.New("QUEUE_SEQUENCE_EXHAUSTED: sequence space exhausted at MAX_SAFE_SEQUENCE")
)

var agentStreamIDPattern = regexp.MustCompile(`^stream-[0-9a-f]{32}$`)

const (
	recordSequenceWidth     = 19
	uncertainMarkerFile     = "location.uncertain"
	transitionIntentFile    = "quarantine.transition"
	maxIntentBytes          = 4096
	syncPhaseIntentFile     = "intent-file"
	syncPhaseIntentDir      = "intent-dir"
	syncPhaseQuarantineDest = "quarantine-dest"
	syncPhaseActiveSource   = "active-source"
	syncPhaseRollbackDest   = "rollback-dest"
	syncPhaseRollbackSource = "rollback-source"
	syncPhaseIntentClearDir = "intent-clear-dir"
	syncPhaseMetaFile       = "meta-file"
	syncPhaseMetaDir        = "meta-dir"
	syncPhaseActiveCommit   = "active-commit"
)

// syncHookFunc is a per-queue test seam for directory and file fsync phases. Production
// leaves this nil. Hooks must not be process-global mutable state.
type syncHookFunc func(phase, target string) error

// pathHookFunc is a per-queue test seam for path operations (e.g. ReadDir, Lstat, Remove).
type pathHookFunc func(path string) error

type quarantineReasonSidecar struct {
	Code string `json:"code"`
}

type uncertainMarkerPayload struct {
	Code string `json:"code"`
}

type transitionIntent struct {
	Version  int    `json:"version"`
	Record   string `json:"record"`
	Sequence uint64 `json:"sequence"`
	Code     string `json:"code,omitempty"`
}

type Record struct {
	Sequence  uint64    `json:"sequence"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Checksum  string    `json:"checksum"`
	Payload   []byte    `json:"payload"`
}

type QueueStats struct {
	ActiveCount              int
	ActiveBytes              int64
	QuarantineCount          int
	QuarantineBytes          int64
	OldestAge                time.Duration
	LastSequence             uint64
	LineageStart             uint64
	AcceptedThrough          uint64
	AgentStreamID            string
	LegacyCutoverSequence    uint64
	MaxRecords               int
	MaxBytes                 int64
	MaxQuarantineRecords     int
	MaxQuarantineBytes       int64
	AtCapacity               bool
	QuarantineAtCapacity     bool
	BlockedFromSequence      uint64
	DeliveryBlocked          bool
	UnknownSequenceBlock     bool
	LocationUncertain        bool
	MetaUncertain            bool
	SequenceGapBlock         bool
	SequenceConflictBlock    bool
	AckRecoveryConflictBlock bool
}

type DurableQueue struct {
	dir                      string
	activeDir                string
	quarantineDir            string
	metaPath                 string
	maxRecords               int
	maxBytes                 int64
	maxRecordBytes           int
	maxQuarantineRecords     int
	maxQuarantineBytes       int64
	mu                       sync.Mutex
	lastSeq                  uint64
	lineageStart             uint64
	acceptedThrough          uint64
	acceptedRecords          []acceptedRecordRef
	agentStreamId            string
	legacyCutoverSequence    uint64
	activeCount              int
	activeBytes              int64
	quarantineCount          int
	quarantineBytes          int64
	blockedFromSequence      uint64
	unknownSequenceBlock     bool
	locationUncertain        bool
	metaUncertain            bool
	sequenceGapBlock         bool
	sequenceConflictBlock    bool
	ackRecoveryConflictBlock bool
	syncHook                 syncHookFunc
	readDirHook              pathHookFunc
	lstatHook                pathHookFunc
	removeHook               pathHookFunc
}

const (
	// MAX_SAFE_SEQUENCE is the JSON-safe delivery/replay sequence ceiling (2^53 - 1).
	MAX_SAFE_SEQUENCE     = 9007199254740991
	sequenceMetaFile      = "sequence.meta"
	sequenceMetaVersion2  = 2
	sequenceMetaVersion3  = 3
)

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
	// 1. Clean temps
	cleanTemp(q.dir)
	cleanTemp(q.activeDir)
	cleanTemp(q.quarantineDir)

	q.unknownSequenceBlock = false
	q.locationUncertain = false
	q.ackRecoveryConflictBlock = false

	// 2. Load uncertain marker
	if err := q.loadUncertainMarkerLocked(); err != nil {
		return err
	}

	// 3. Read/validate sequence.meta FIRST so ACK context is known before mutating active.
	meta, err := q.readSequenceMetaLocked()
	if err != nil {
		return err
	}
	if meta.HighestSequence > MAX_SAFE_SEQUENCE {
		return errors.New("QUEUE_META_CORRUPT: highestSequence exceeds MAX_SAFE_SEQUENCE")
	}
	q.lastSeq = meta.HighestSequence
	q.acceptedThrough = meta.AcceptedThrough
	q.acceptedRecords = cloneAcceptedRecords(meta.AcceptedRecords)
	q.agentStreamId = meta.AgentStreamID
	q.legacyCutoverSequence = meta.LegacyCutoverSequence
	if meta.Version == sequenceMetaVersion2 || meta.Version == sequenceMetaVersion3 {
		q.lineageStart = meta.LineageStart
	} else {
		q.lineageStart = 0
	}
	if q.agentStreamId != "" && !validAgentStreamID(q.agentStreamId) {
		return errors.New("QUEUE_META_CORRUPT: invalid agentStreamId")
	}

	// 4. Scan quarantine / recover transition intent (existing A-E)
	qHigh, err := q.scanQuarantineLocked()
	if err != nil {
		return err
	}
	intentSeq, hasIntent, _ := q.recoverTransitionIntentLocked()

	// 5. Scan active WITH ACK context known
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
	if hasIntent && intentSeq > highestSeq {
		highestSeq = intentSeq
	}

	authBySeq := make(map[uint64]string, len(q.acceptedRecords))
	for _, r := range q.acceptedRecords {
		authBySeq[r.Sequence] = r.ID
	}

	for _, e := range entries {
		if e.IsDir() || !isRecordJSON(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			return fmt.Errorf("QUEUE_RECOVER_STAT_FAILED: %w", err)
		}
		fileSeq, fileID, nameErr := parseRecordFileName(e.Name())
		if nameErr != nil {
			q.unknownSequenceBlock = true
			q.activeCount++
			q.activeBytes += fi.Size()
			continue
		}
		if fileSeq > highestSeq {
			highestSeq = fileSeq
		}

		// ACK-bounded sequences: never quarantine/delete before ACK reconcile.
		if q.acceptedThrough > 0 && fileSeq <= q.acceptedThrough {
			authID, ok := authBySeq[fileSeq]
			if !ok || fileID != authID || e.Name() != recordFileName(fileSeq, authID) {
				q.ackRecoveryConflictBlock = true
			}
			// Exact-match leftovers are cleanup candidates; corrupt payload must not
			// quarantine-hide them before reconcile. Retain all ACK-bounded actives.
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
				continue
			}
			if qErr := q.quarantineRecordLocked(e.Name(), "QUEUE_RECORD_CORRUPT"); qErr != nil {
				if _, stillActive := os.Lstat(filePath); stillActive == nil {
					q.activeCount++
					q.activeBytes += fi.Size()
				}
				if isFailClosedQuarantineErr(qErr) {
					continue
				}
				return qErr
			}
			continue
		}
		_ = rec
		q.activeCount++
		q.activeBytes += fi.Size()
	}

	// lastSeq = max(file highest, durable watermark) so drained queues retain lineage.
	if highestSeq > q.lastSeq {
		q.lastSeq = highestSeq
	}
	if q.acceptedThrough > q.lastSeq {
		return errors.New("QUEUE_META_CORRUPT: acceptedThrough exceeds highestSequence lineage")
	}
	if q.lastSeq > MAX_SAFE_SEQUENCE {
		return errors.New("QUEUE_META_CORRUPT: recovered sequence exceeds MAX_SAFE_SEQUENCE")
	}

	// Migrate legacy/v2 meta (or initialize empty queue) to v3 stream identity once.
	// Never regenerate agentStreamId on later restarts.
	if q.agentStreamId == "" {
		if err := q.migrateOrInitStreamLocked(); err != nil {
			return err
		}
	}

	// 6. Reconcile duplicates
	if err := q.reconcileDuplicatesLocked(); err != nil {
		return err
	}
	// 7. Reconcile accepted leftovers
	if err := q.reconcileAcceptedLeftoversLocked(); err != nil {
		return err
	}
	// 8. Refresh integrity
	if err := q.refreshSequenceIntegrityLocked(); err != nil {
		return err
	}
	return nil
}

type acceptedRecordRef struct {
	Sequence uint64 `json:"sequence"`
	ID       string `json:"id"`
}

type sequenceMeta struct {
	Version               uint                `json:"version,omitempty"`
	AgentStreamID         string              `json:"agentStreamId,omitempty"`
	LegacyCutoverSequence uint64              `json:"legacyCutoverSequence,omitempty"`
	HighestSequence       uint64              `json:"highestSequence"`
	LineageStart          uint64              `json:"lineageStart,omitempty"`
	AcceptedThrough       uint64              `json:"acceptedThrough,omitempty"`
	AcceptedRecords       []acceptedRecordRef `json:"acceptedRecords,omitempty"`
}

func generateAgentStreamID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("QUEUE_STREAM_ID_FAILED: %w", err)
	}
	id := "stream-" + hex.EncodeToString(b)
	if !validAgentStreamID(id) {
		return "", errors.New("QUEUE_STREAM_ID_FAILED: generated agentStreamId failed validation")
	}
	return id, nil
}

func validAgentStreamID(id string) bool {
	return agentStreamIDPattern.MatchString(id)
}

// migrateOrInitStreamLocked assigns a durable agentStreamId exactly once.
// Empty queues (highest=0) persist cutover=0 and lineageStart=0 — never lineageStart=1 while highest=0.
// Legacy/v2 opens preserve sequence watermarks and set cutover to highest+1 when highest>0.
func (q *DurableQueue) migrateOrInitStreamLocked() error {
	if q.agentStreamId != "" {
		return nil
	}
	id, err := generateAgentStreamID()
	if err != nil {
		return err
	}
	q.agentStreamId = id
	if q.lastSeq > 0 {
		if q.lastSeq >= MAX_SAFE_SEQUENCE {
			// Internal only; never emitted on the wire.
			q.legacyCutoverSequence = MAX_SAFE_SEQUENCE + 1
		} else {
			q.legacyCutoverSequence = q.lastSeq + 1
		}
	} else {
		q.legacyCutoverSequence = 0
	}
	// Do not invent lineageStart during stream migration/init.
	return q.persistSequenceMetaLockedOpts(q.lastSeq, q.acceptedThrough, q.acceptedRecords, false)
}

func (q *DurableQueue) AgentStreamID() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.agentStreamId
}

func (q *DurableQueue) LegacyCutoverSequence() uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.legacyCutoverSequence
}

func (q *DurableQueue) IsLegacySequence(seq uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.legacyCutoverSequence > 0 && seq < q.legacyCutoverSequence
}

func cloneAcceptedRecords(in []acceptedRecordRef) []acceptedRecordRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]acceptedRecordRef, len(in))
	copy(out, in)
	return out
}

func validateAcceptedRecords(acceptedThrough uint64, records []acceptedRecordRef, bound int) error {
	if len(records) == 0 {
		return nil
	}
	if bound > 0 && len(records) > bound {
		return errors.New("QUEUE_META_CORRUPT: acceptedRecords exceed bound")
	}
	seen := make(map[uint64]struct{}, len(records))
	for _, r := range records {
		if r.Sequence == 0 || strings.TrimSpace(r.ID) == "" {
			return errors.New("QUEUE_META_CORRUPT: acceptedRecords entry missing sequence or id")
		}
		if r.Sequence > acceptedThrough {
			return errors.New("QUEUE_META_CORRUPT: acceptedRecords sequence exceeds acceptedThrough")
		}
		if _, ok := seen[r.Sequence]; ok {
			return errors.New("QUEUE_META_CORRUPT: acceptedRecords duplicate sequence")
		}
		seen[r.Sequence] = struct{}{}
	}
	return nil
}

func (q *DurableQueue) readSequenceMetaLocked() (sequenceMeta, error) {
	data, err := os.ReadFile(q.metaPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sequenceMeta{}, nil
		}
		return sequenceMeta{}, fmt.Errorf("QUEUE_META_READ_FAILED: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return sequenceMeta{}, errors.New("QUEUE_META_CORRUPT: sequence metadata is empty")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var meta sequenceMeta
	if err := dec.Decode(&meta); err != nil {
		return sequenceMeta{}, fmt.Errorf("QUEUE_META_CORRUPT: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return sequenceMeta{}, errors.New("QUEUE_META_CORRUPT: trailing data in sequence metadata")
	}
	// version absent/0 = legacy; version 2 = lineage format; version 3 = stream lineage;
	// unknown nonzero = fail closed. Migration to v3 happens in recover, not here.
	if meta.Version != 0 && meta.Version != sequenceMetaVersion2 && meta.Version != sequenceMetaVersion3 {
		return sequenceMeta{}, errors.New("QUEUE_META_CORRUPT: unknown sequence metadata version")
	}
	if meta.AcceptedThrough > meta.HighestSequence {
		return sequenceMeta{}, errors.New("QUEUE_META_CORRUPT: acceptedThrough exceeds highestSequence")
	}
	if meta.LineageStart > meta.HighestSequence {
		return sequenceMeta{}, errors.New("QUEUE_META_CORRUPT: lineageStart exceeds highestSequence")
	}
	if meta.HighestSequence > MAX_SAFE_SEQUENCE {
		return sequenceMeta{}, errors.New("QUEUE_META_CORRUPT: highestSequence exceeds MAX_SAFE_SEQUENCE")
	}
	if meta.Version == sequenceMetaVersion3 {
		if !validAgentStreamID(meta.AgentStreamID) {
			return sequenceMeta{}, errors.New("QUEUE_META_CORRUPT: version 3 requires valid agentStreamId")
		}
	}
	if meta.Version == sequenceMetaVersion2 && meta.HighestSequence > 0 && meta.AcceptedThrough == 0 {
		if meta.LineageStart == 0 || meta.LineageStart > meta.HighestSequence {
			return sequenceMeta{}, errors.New("QUEUE_META_CORRUPT: version 2 requires lineageStart when highestSequence is set without acceptedThrough")
		}
	}
	if err := validateAcceptedRecords(meta.AcceptedThrough, meta.AcceptedRecords, q.maxRecords); err != nil {
		return sequenceMeta{}, err
	}
	return meta, nil
}

func (q *DurableQueue) readSequenceWatermarkLocked() (uint64, error) {
	meta, err := q.readSequenceMetaLocked()
	if err != nil {
		return 0, err
	}
	return meta.HighestSequence, nil
}

func (q *DurableQueue) persistSequenceWatermarkLocked(seq uint64) error {
	return q.persistSequenceMetaLocked(seq, q.acceptedThrough, q.acceptedRecords)
}

func (q *DurableQueue) persistSequenceMetaLocked(highest, accepted uint64, records []acceptedRecordRef) error {
	return q.persistSequenceMetaLockedOpts(highest, accepted, records, true)
}

func (q *DurableQueue) persistSequenceMetaLockedOpts(highest, accepted uint64, records []acceptedRecordRef, establishLineage bool) error {
	if q.metaUncertain {
		return ErrMetaUncertain
	}
	if q.agentStreamId == "" || !validAgentStreamID(q.agentStreamId) {
		return errors.New("QUEUE_META_CORRUPT: agentStreamId required before meta persist")
	}
	if highest > MAX_SAFE_SEQUENCE {
		return ErrSequenceExhausted
	}
	if accepted > highest {
		return errors.New("QUEUE_META_CORRUPT: acceptedThrough exceeds highestSequence")
	}
	if accepted < q.acceptedThrough {
		return errors.New("QUEUE_META_REGRESS: acceptedThrough must never regress")
	}
	if err := validateAcceptedRecords(accepted, records, q.maxRecords); err != nil {
		return err
	}

	// lineageStart: once nonzero never move forward/change.
	// Establish only when still unset and this write has no accepted boundary
	// (first durable enqueue / watermark establish). Do not invent lineage from
	// an ACK-only highest watermark. Never set lineageStart=1 while highest=0.
	// Migration/init stream writes must not invent lineageStart (establishLineage=false).
	lineageStart := q.lineageStart
	if q.lineageStart != 0 {
		lineageStart = q.lineageStart
	} else if establishLineage && accepted == 0 && highest > 0 {
		lineageStart = highest
	}
	if lineageStart > highest {
		return errors.New("QUEUE_META_CORRUPT: lineageStart exceeds highestSequence")
	}

	meta := sequenceMeta{
		Version:               sequenceMetaVersion3,
		AgentStreamID:         q.agentStreamId,
		LegacyCutoverSequence: q.legacyCutoverSequence,
		HighestSequence:       highest,
		LineageStart:          lineageStart,
		AcceptedThrough:       accepted,
		AcceptedRecords:       cloneAcceptedRecords(records),
	}
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
	if err := q.syncFilePhase(syncPhaseMetaFile, tmp); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("QUEUE_META_SYNC_FAILED: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("QUEUE_META_CLOSE_FAILED: %w", err)
	}
	if err := os.Rename(tmpName, q.metaPath); err != nil {
		return fmt.Errorf("QUEUE_META_RENAME_FAILED: %w", err)
	}
	if err := q.syncDirPhase(syncPhaseMetaDir, q.dir); err != nil {
		// Rename succeeded: durable meta may already be the new value. Fail closed —
		// adopt disk/intended watermarks (never regress) and block further meta writes.
		q.adoptMetaAfterUncertainLocked(highest, accepted, records, lineageStart)
		q.metaUncertain = true
		return fmt.Errorf("%w: %v", ErrMetaUncertain, err)
	}
	if q.lineageStart == 0 {
		q.lineageStart = lineageStart
	}
	return nil
}

// earliestNonzeroLineage returns the earliest nonzero lineage among candidates.
// Zero means unset. Disagreeing nonzero values never pick the later/larger.
func earliestNonzeroLineage(candidates ...uint64) uint64 {
	var chosen uint64
	for _, c := range candidates {
		if c == 0 {
			continue
		}
		if chosen == 0 || c < chosen {
			chosen = c
		}
	}
	return chosen
}

// adoptMetaAfterUncertainLocked raises in-memory watermarks to the max of current,
// intended write, and a best-effort disk reread so a later write cannot regress.
// Lineage adopts the earliest nonzero among current/intended/disk (never later).
func (q *DurableQueue) adoptMetaAfterUncertainLocked(highest, accepted uint64, records []acceptedRecordRef, intendedLineage uint64) {
	if accepted > q.acceptedThrough {
		q.acceptedThrough = accepted
	}
	if highest > q.lastSeq {
		q.lastSeq = highest
	}
	// Intended records are the floor (including an explicit clear to nil).
	q.acceptedRecords = cloneAcceptedRecords(records)
	disk, err := q.readSequenceMetaLocked()
	diskLineage := uint64(0)
	if err == nil {
		if disk.AcceptedThrough > q.acceptedThrough {
			q.acceptedThrough = disk.AcceptedThrough
		}
		if disk.HighestSequence > q.lastSeq {
			q.lastSeq = disk.HighestSequence
		}
		// Only trust disk acceptedRecords when the reread reflects this write
		// (or a later one). A stale lower acceptedThrough must not drop new auth.
		if disk.AcceptedThrough >= accepted {
			q.acceptedRecords = cloneAcceptedRecords(disk.AcceptedRecords)
		}
		if disk.Version == sequenceMetaVersion2 || disk.Version == sequenceMetaVersion3 {
			diskLineage = disk.LineageStart
		}
		// Preserve stream identity / cutover; never regenerate after uncertain rename.
		if q.agentStreamId == "" && validAgentStreamID(disk.AgentStreamID) {
			q.agentStreamId = disk.AgentStreamID
		}
		if q.legacyCutoverSequence == 0 && disk.LegacyCutoverSequence > 0 {
			q.legacyCutoverSequence = disk.LegacyCutoverSequence
		}
	}
	q.lineageStart = earliestNonzeroLineage(q.lineageStart, intendedLineage, diskLineage)
}

func (q *DurableQueue) Enqueue(payload []byte) (*Record, error) {
	if len(payload) > q.maxRecordBytes {
		return nil, ErrRecordTooLarge
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.metaUncertain {
		return nil, ErrMetaUncertain
	}

	if q.activeCount >= q.maxRecords {
		return nil, ErrQueueFull
	}

	if q.lastSeq >= MAX_SAFE_SEQUENCE {
		return nil, ErrSequenceExhausted
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

	// First durable enqueue establishes lineageStart in the same meta write as
	// highestSequence=seq. Once nonzero, lineageStart never moves forward.
	if err := q.persistSequenceWatermarkLocked(seq); err != nil {
		if errors.Is(err, ErrMetaUncertain) {
			// Meta rename likely durable; retain the record so watermark and active agree.
			q.activeCount++
			q.activeBytes += int64(len(data))
			return nil, err
		}
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

	if q.metaUncertain {
		return nil, ErrMetaUncertain
	}
	if q.locationUncertain {
		return nil, ErrQuarantineUncertain
	}

	// Resolve pending ACK cleanup before delivering later records.
	if len(q.acceptedRecords) > 0 {
		if err := q.reconcileAcceptedLeftoversLocked(); err != nil {
			return nil, err
		}
		if len(q.acceptedRecords) > 0 {
			// Conflict retains auth and blocks cleanup; do not mask as temporary pending.
			if q.ackRecoveryConflictBlock {
				return nil, ErrAckRecoveryConflict
			}
			if q.sequenceConflictBlock {
				return nil, ErrSequenceConflict
			}
			return nil, ErrPendingAckCleanup
		}
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

	if err := q.refreshSequenceIntegrityLocked(); err != nil {
		return nil, err
	}
	if q.sequenceConflictBlock {
		return nil, ErrSequenceConflict
	}
	if q.ackRecoveryConflictBlock {
		return nil, ErrAckRecoveryConflict
	}
	if q.sequenceGapBlock {
		return nil, ErrSequenceGap
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
		if fileSeq <= q.acceptedThrough {
			// Durable ACK leftovers are skipped for delivery; recovery cleans them.
			continue
		}
		if blocked > 0 && fileSeq >= blocked {
			break
		}
		// Never cross LegacyCutoverSequence inside one batch.
		if q.legacyCutoverSequence > 0 && len(batch) > 0 &&
			batch[0].Sequence < q.legacyCutoverSequence &&
			fileSeq >= q.legacyCutoverSequence {
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

		if q.acceptedThrough > 0 && len(batch) == 0 && rec.Sequence != q.acceptedThrough+1 {
			expected := q.acceptedThrough + 1
			if q.blockedFromSequence == 0 || q.blockedFromSequence > expected {
				q.sequenceGapBlock = true
				return nil, ErrSequenceGap
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
	if q.metaUncertain {
		return ErrMetaUncertain
	}
	if q.locationUncertain {
		return ErrQuarantineUncertain
	}
	if q.unknownSequenceBlock {
		return ErrUnknownSequence
	}
	if q.sequenceConflictBlock {
		return ErrSequenceConflict
	}
	if q.ackRecoveryConflictBlock {
		return ErrAckRecoveryConflict
	}
	if q.sequenceGapBlock {
		return ErrSequenceGap
	}
	if hasPrefix || q.blockedFromSequence == 0 {
		return nil
	}
	hasActive, err := q.hasActiveAfterBlockedLocked()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrQuarantineUncertain, err)
	}
	if hasActive {
		return ErrDeliveryBlocked
	}
	return nil
}

func (q *DurableQueue) hasActiveAfterBlockedLocked() (bool, error) {
	if q.blockedFromSequence == 0 {
		return false, nil
	}
	if q.readDirHook != nil {
		if err := q.readDirHook(q.activeDir); err != nil {
			return true, fmt.Errorf("QUEUE_ACTIVE_DIR_SCAN_FAILED: %w", err)
		}
	}
	entries, err := os.ReadDir(q.activeDir)
	if err != nil {
		return true, fmt.Errorf("QUEUE_ACTIVE_DIR_SCAN_FAILED: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "rec_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		seq, _, err := parseRecordFileName(e.Name())
		if err != nil {
			continue
		}
		if seq >= q.blockedFromSequence {
			return true, nil
		}
	}
	return false, nil
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
	payload, err := json.Marshal(quarantineReasonSidecar{Code: sanitizeQuarantineCode(code)})
	if err != nil {
		return
	}
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
	if err := os.Rename(tmpName, path); err != nil {
		return
	}
	_ = syncDir(q.quarantineDir)
}

func (q *DurableQueue) quarantineRecordLocked(name, code string) error {
	if q.locationUncertain {
		return ErrQuarantineUncertain
	}

	oldPath := filepath.Join(q.activeDir, name)
	newPath := filepath.Join(q.quarantineDir, name)

	fi, err := q.lstatFile(oldPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("QUEUE_QUARANTINE_STAT_FAILED: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return errors.New("QUEUE_QUARANTINE_SYMLINK_REJECTED: active record path is a symlink")
	}
	if !fi.Mode().IsRegular() {
		return errors.New("QUEUE_QUARANTINE_RECORD_NOT_REGULAR: active record path is not a regular file")
	}

	destInfo, destErr := q.lstatFile(newPath)
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

	// 1. Establish durable PRE-TRANSITION INTENT before any rename
	if err := q.persistTransitionIntentLocked(name, fileSeq, code); err != nil {
		return q.enterUncertainLocked(fmt.Errorf("transition intent failed: %w", err))
	}

	// 2. Perform active -> quarantine rename
	if err := os.Rename(oldPath, newPath); err != nil {
		return q.enterUncertainLocked(fmt.Errorf("quarantine rename failed: %w", err))
	}

	// 3. Sync quarantine destination directory
	if err := q.syncDirPhase(syncPhaseQuarantineDest, q.quarantineDir); err != nil {
		return q.rollbackQuarantineMoveLocked(name, err)
	}

	// 4. Sync active source directory
	if err := q.syncDirPhase(syncPhaseActiveSource, q.activeDir); err != nil {
		// Destination directory entry is durable. Do not reverse: a rollback
		// could drop the only durable copy. Quarantine is authoritative.
		q.applyQuarantineCountersLocked(size, fileSeq)
		if merr := q.persistUncertainStateLocked(); merr != nil {
			return fmt.Errorf("%w: marker persistence failed (%v) after source sync failed: %v", ErrQuarantineUncertain, merr, err)
		}
		return fmt.Errorf("%w: %v", ErrQuarantineSourceSyncFailed, err)
	}

	// 5. Final quarantine state is durably established
	q.applyQuarantineCountersLocked(size, fileSeq)
	q.writeQuarantineReason(name, code)

	// 6. Clear transition intent durably
	if err := q.clearTransitionIntentLocked(); err != nil {
		return q.enterUncertainLocked(fmt.Errorf("clear transition intent failed: %w", err))
	}

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

	// Attempt rollback rename: quarantine -> active
	if err := os.Rename(quarantinePath, activePath); err != nil {
		return q.enterUncertainLocked(fmt.Errorf("rollback rename failed: %w", err))
	}

	// Step 1: Sync rollback destination (active directory)
	if err := q.syncDirPhase(syncPhaseRollbackDest, q.activeDir); err != nil {
		return q.enterUncertainLocked(fmt.Errorf("rollback dest sync failed: %w", err))
	}

	// Step 2: Sync rollback source (quarantine directory)
	if err := q.syncDirPhase(syncPhaseRollbackSource, q.quarantineDir); err != nil {
		// Rollback destination (active/) is durable. Do not reverse again.
		return q.enterUncertainLocked(fmt.Errorf("rollback source sync failed after dest sync error: %w", destSyncErr))
	}

	// Rollback durability established: attempt to clear transition intent
	if err := q.clearTransitionIntentLocked(); err != nil {
		return q.enterUncertainLocked(fmt.Errorf("clear transition intent after rollback failed: %w", err))
	}

	return fmt.Errorf("QUEUE_QUARANTINE_DIR_SYNC_FAILED: %w", destSyncErr)
}

func (q *DurableQueue) syncDirPhase(phase, dir string) error {
	if q.syncHook != nil {
		if err := q.syncHook(phase, dir); err != nil {
			return err
		}
	}
	return syncDir(dir)
}

func (q *DurableQueue) syncFilePhase(phase string, f *os.File) error {
	if q.syncHook != nil {
		if err := q.syncHook(phase, f.Name()); err != nil {
			return err
		}
	}
	return f.Sync()
}

func (q *DurableQueue) lstatFile(path string) (os.FileInfo, error) {
	if q.lstatHook != nil {
		if err := q.lstatHook(path); err != nil {
			return nil, err
		}
	}
	return os.Lstat(path)
}

func (q *DurableQueue) persistTransitionIntentLocked(recordName string, seq uint64, code string) error {
	intent := transitionIntent{
		Version:  1,
		Record:   recordName,
		Sequence: seq,
		Code:     sanitizeQuarantineCode(code),
	}
	data, err := json.Marshal(intent)
	if err != nil {
		return fmt.Errorf("QUEUE_INTENT_MARSHAL_FAILED: %w", err)
	}

	intentPath := filepath.Join(q.dir, transitionIntentFile)
	if fi, err := q.lstatFile(intentPath); err == nil {
		return fmt.Errorf("QUEUE_INTENT_EXISTS: transition intent already present (mode=%v)", fi.Mode())
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("QUEUE_INTENT_STAT_FAILED: %w", err)
	}

	// O_EXCL: never overwrite an existing intent lease.
	f, err := os.OpenFile(intentPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("QUEUE_INTENT_OPEN_FAILED: %w", err)
	}
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return fmt.Errorf("QUEUE_INTENT_CHMOD_FAILED: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("QUEUE_INTENT_WRITE_FAILED: %w", err)
	}
	if err := q.syncFilePhase(syncPhaseIntentFile, f); err != nil {
		_ = f.Close()
		return fmt.Errorf("QUEUE_INTENT_SYNC_FAILED: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("QUEUE_INTENT_CLOSE_FAILED: %w", err)
	}
	if err := q.syncDirPhase(syncPhaseIntentDir, q.dir); err != nil {
		return fmt.Errorf("QUEUE_INTENT_DIR_SYNC_FAILED: %w", err)
	}
	return nil
}

func (q *DurableQueue) clearTransitionIntentLocked() error {
	intentPath := filepath.Join(q.dir, transitionIntentFile)
	if q.removeHook != nil {
		if err := q.removeHook(intentPath); err != nil {
			return err
		}
	}
	if err := os.Remove(intentPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("QUEUE_INTENT_REMOVE_FAILED: %w", err)
	}
	if err := q.syncDirPhase(syncPhaseIntentClearDir, q.dir); err != nil {
		return fmt.Errorf("QUEUE_INTENT_CLEAR_DIR_SYNC_FAILED: %w", err)
	}
	return nil
}

func (q *DurableQueue) recoverTransitionIntentLocked() (uint64, bool, error) {
	intentPath := filepath.Join(q.dir, transitionIntentFile)
	fi, err := q.lstatFile(intentPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		q.locationUncertain = true
		return 0, true, nil
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		q.locationUncertain = true
		return 0, true, nil
	}
	if !fi.Mode().IsRegular() {
		q.locationUncertain = true
		return 0, true, nil
	}
	if fi.Size() == 0 || fi.Size() > maxIntentBytes {
		q.locationUncertain = true
		return 0, true, nil
	}

	data, err := os.ReadFile(intentPath)
	if err != nil {
		q.locationUncertain = true
		return 0, true, nil
	}
	var intent transitionIntent
	if err := json.Unmarshal(data, &intent); err != nil {
		q.locationUncertain = true
		return 0, true, nil
	}

	if intent.Version != 1 || filepath.Base(intent.Record) != intent.Record || strings.Contains(intent.Record, "..") {
		q.locationUncertain = true
		return 0, true, nil
	}

	fileSeq, _, parseErr := parseRecordFileName(intent.Record)
	if parseErr != nil || fileSeq == 0 || fileSeq != intent.Sequence {
		q.locationUncertain = true
		return 0, true, nil
	}

	aPath := filepath.Join(q.activeDir, intent.Record)
	qPath := filepath.Join(q.quarantineDir, intent.Record)

	aInfo, aErr := q.lstatFile(aPath)
	if aErr != nil && !errors.Is(aErr, os.ErrNotExist) {
		q.locationUncertain = true
		return intent.Sequence, true, nil
	}
	if aErr == nil {
		if aInfo.Mode()&os.ModeSymlink != 0 || !aInfo.Mode().IsRegular() {
			q.locationUncertain = true
			return intent.Sequence, true, nil
		}
	}

	qInfo, qErr := q.lstatFile(qPath)
	if qErr != nil && !errors.Is(qErr, os.ErrNotExist) {
		q.locationUncertain = true
		return intent.Sequence, true, nil
	}
	if qErr == nil {
		if qInfo.Mode()&os.ModeSymlink != 0 || !qInfo.Mode().IsRegular() {
			q.locationUncertain = true
			return intent.Sequence, true, nil
		}
	}

	hasActive := (aErr == nil)
	hasQuarantine := (qErr == nil)

	switch {
	case !hasActive && !hasQuarantine:
		// Case E: intent + neither active nor quarantine -> QUEUE_QUARANTINE_UNCERTAIN
		q.noteBlockedLocked(intent.Sequence)
		q.locationUncertain = true
		return intent.Sequence, true, nil

	case !hasActive && hasQuarantine:
		// Case A: intent + quarantine only
		// Quarantine is authoritative. Record is blocked.
		// Later records cannot be delivered.
		q.noteBlockedLocked(intent.Sequence)
		return intent.Sequence, true, nil

	case hasActive && !hasQuarantine:
		// Case B: intent + active only
		// Active record retained. Fail closed / reconciliation required. No later delivery.
		q.noteBlockedLocked(intent.Sequence)
		return intent.Sequence, true, nil

	case hasActive && hasQuarantine:
		// Both exist: check identical vs conflicting bytes
		aData, err := os.ReadFile(aPath)
		if err != nil {
			q.noteBlockedLocked(intent.Sequence)
			q.locationUncertain = true
			return intent.Sequence, true, nil
		}
		qData, err := os.ReadFile(qPath)
		if err != nil {
			q.noteBlockedLocked(intent.Sequence)
			q.locationUncertain = true
			return intent.Sequence, true, nil
		}

		if bytes.Equal(aData, qData) {
			// Case C: intent + active + quarantine with identical bytes
			// Quarantine authoritative. Active duplicate must NEVER be delivered.
			// Fail closed / blocked until reconciliation.
			q.noteBlockedLocked(intent.Sequence)
			return intent.Sequence, true, nil
		}

		// Case D: intent + active + quarantine with conflicting bytes -> QUEUE_QUARANTINE_UNCERTAIN
		q.noteBlockedLocked(intent.Sequence)
		q.locationUncertain = true
		return intent.Sequence, true, nil
	}

	return intent.Sequence, true, nil
}

func (q *DurableQueue) persistUncertainStateLocked() error {
	q.locationUncertain = true
	return q.persistUncertainMarkerLocked()
}

func (q *DurableQueue) enterUncertainLocked(cause error) error {
	if err := q.persistUncertainStateLocked(); err != nil {
		return fmt.Errorf("%w: marker persistence failed (%v) after cause: %v", ErrQuarantineUncertain, err, cause)
	}
	return fmt.Errorf("%w: %v", ErrQuarantineUncertain, cause)
}

func (q *DurableQueue) persistUncertainMarkerLocked() error {
	path := filepath.Join(q.dir, uncertainMarkerFile)
	payload, err := json.Marshal(uncertainMarkerPayload{Code: "QUEUE_QUARANTINE_UNCERTAIN"})
	if err != nil {
		return fmt.Errorf("QUEUE_UNCERTAIN_MARSHAL_FAILED: %w", err)
	}
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
	fi, err := q.lstatFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("QUEUE_UNCERTAIN_MARKER_STAT_FAILED: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		q.locationUncertain = true
		return errors.New("QUEUE_UNCERTAIN_MARKER_SYMLINK: uncertain marker is a symlink")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("QUEUE_UNCERTAIN_MARKER_READ_FAILED: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		q.locationUncertain = true
		return nil
	}
	var meta uncertainMarkerPayload
	if err := json.Unmarshal(data, &meta); err != nil {
		q.locationUncertain = true
		return nil
	}
	if meta.Code != "QUEUE_QUARANTINE_UNCERTAIN" {
		q.locationUncertain = true
		return nil
	}
	q.locationUncertain = true
	return nil
}

func (q *DurableQueue) duplicateStateLocked(name string) (bool, error) {
	qPath := filepath.Join(q.quarantineDir, name)
	qInfo, err := q.lstatFile(qPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("QUEUE_DUPLICATE_STAT_FAILED: %w", err)
	}
	if qInfo.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("QUEUE_QUARANTINE_SYMLINK_REJECTED: quarantine record path is a symlink")
	}
	if !qInfo.Mode().IsRegular() {
		return false, errors.New("QUEUE_QUARANTINE_RECORD_NOT_REGULAR: quarantine record path is not a regular file")
	}

	aPath := filepath.Join(q.activeDir, name)
	aInfo, err := q.lstatFile(aPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("QUEUE_DUPLICATE_ACTIVE_STAT_FAILED: %w", err)
	}
	if aInfo.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("QUEUE_ACTIVE_SYMLINK_REJECTED: active record path is a symlink")
	}
	if !aInfo.Mode().IsRegular() {
		return false, errors.New("QUEUE_ACTIVE_RECORD_NOT_REGULAR: active record path is not a regular file")
	}

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
		destPath := filepath.Join(q.quarantineDir, e.Name())
		destInfo, destErr := q.lstatFile(destPath)
		if destErr != nil {
			if errors.Is(destErr, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("QUEUE_RECONCILE_DEST_STAT_FAILED: %w", destErr)
		}
		if destInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("QUEUE_QUARANTINE_SYMLINK_REJECTED: quarantine record path is a symlink")
		}
		if !destInfo.Mode().IsRegular() {
			return errors.New("QUEUE_QUARANTINE_RECORD_NOT_REGULAR: quarantine record path is not a regular file")
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

func recordFileName(seq uint64, id string) string {
	return fmt.Sprintf("rec_%019d_%s.json", seq, id)
}

func (q *DurableQueue) refreshSequenceIntegrityLocked() error {
	q.sequenceGapBlock = false
	q.sequenceConflictBlock = false
	q.ackRecoveryConflictBlock = false

	covered := make(map[uint64]struct{})
	bySeq := make(map[uint64][]string)

	entries, err := os.ReadDir(q.activeDir)
	if err != nil {
		return fmt.Errorf("QUEUE_SCAN_FAILED: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !isRecordJSON(e.Name()) {
			continue
		}
		fileSeq, _, nameErr := parseRecordFileName(e.Name())
		if nameErr != nil {
			continue
		}
		bySeq[fileSeq] = append(bySeq[fileSeq], e.Name())
		covered[fileSeq] = struct{}{}
	}
	for _, names := range bySeq {
		if len(names) > 1 {
			q.sequenceConflictBlock = true
			return nil
		}
	}

	// Any active record at sequence <= acceptedThrough must exactly match
	// persisted acceptedRecords authorization, or the queue is blocked.
	authBySeq := make(map[uint64]string, len(q.acceptedRecords))
	for _, r := range q.acceptedRecords {
		authBySeq[r.Sequence] = r.ID
	}
	if q.acceptedThrough > 0 {
		for fileSeq, names := range bySeq {
			if fileSeq > q.acceptedThrough {
				continue
			}
			for _, name := range names {
				_, fileID, nameErr := parseRecordFileName(name)
				if nameErr != nil {
					q.ackRecoveryConflictBlock = true
					continue
				}
				authID, ok := authBySeq[fileSeq]
				if !ok || fileID != authID || name != recordFileName(fileSeq, authID) {
					q.ackRecoveryConflictBlock = true
				}
			}
		}
	}

	qEntries, err := os.ReadDir(q.quarantineDir)
	if err != nil {
		return fmt.Errorf("QUEUE_SCAN_QUARANTINE_FAILED: %w", err)
	}
	for _, e := range qEntries {
		if e.IsDir() || !isRecordJSON(e.Name()) {
			continue
		}
		fileSeq, fileID, nameErr := parseRecordFileName(e.Name())
		if nameErr != nil {
			continue
		}
		covered[fileSeq] = struct{}{}
		// Different-ID quarantine at/below acceptedThrough must not clear auth.
		if q.acceptedThrough > 0 && fileSeq <= q.acceptedThrough {
			authID, ok := authBySeq[fileSeq]
			if !ok || fileID != authID || e.Name() != recordFileName(fileSeq, authID) {
				q.ackRecoveryConflictBlock = true
			}
		}
	}
	// Intent / block holds cover the blocked sequence even when neither copy is present.
	if q.blockedFromSequence > 0 {
		covered[q.blockedFromSequence] = struct{}{}
	}
	for _, r := range q.acceptedRecords {
		covered[r.Sequence] = struct{}{}
	}

	if q.lastSeq > q.acceptedThrough {
		if q.acceptedThrough > 0 {
			for seq := q.acceptedThrough + 1; seq <= q.lastSeq; seq++ {
				if _, ok := covered[seq]; !ok {
					q.sequenceGapBlock = true
					break
				}
			}
		} else {
			// acceptedThrough == 0: continuity depends on durable or inferred lineage.
			if q.lastSeq == 0 {
				return nil
			}
			var start uint64
			if q.lineageStart > 0 {
				// version 2 (or inferred): require disposition for lineageStart..lastSeq.
				start = q.lineageStart
			} else {
				// Legacy (version 0): empty covered with lastSeq>0 is a gap.
				// Otherwise use lowest covered as one-time in-memory lineage for gap
				// check; next legitimate meta write persists it as v2 (do not write on open).
				if len(covered) == 0 {
					q.sequenceGapBlock = true
					return nil
				}
				var lowest uint64
				first := true
				for seq := range covered {
					if first || seq < lowest {
						lowest = seq
						first = false
					}
				}
				start = lowest
				q.lineageStart = lowest
			}
			for seq := start; seq <= q.lastSeq; seq++ {
				if _, ok := covered[seq]; !ok {
					q.sequenceGapBlock = true
					break
				}
			}
		}
	}
	return nil
}

func (q *DurableQueue) reconcileAcceptedLeftoversLocked() error {
	if q.acceptedThrough == 0 {
		return nil
	}
	if err := q.refreshSequenceIntegrityLocked(); err != nil {
		return err
	}
	if q.sequenceConflictBlock {
		// Ambiguous same-sequence leftovers: fail closed without deleting.
		return nil
	}
	if q.ackRecoveryConflictBlock {
		// Unauthorized / ID-mismatched leftovers at or below acceptedThrough:
		// retain records, retain acceptedRecords, do not treat as healthy skip.
		return nil
	}
	if len(q.acceptedRecords) == 0 {
		return nil
	}

	authBySeq := make(map[uint64]string, len(q.acceptedRecords))
	for _, r := range q.acceptedRecords {
		authBySeq[r.Sequence] = r.ID
	}

	entries, err := os.ReadDir(q.activeDir)
	if err != nil {
		return fmt.Errorf("QUEUE_RECOVER_SCAN_FAILED: %w", err)
	}

	type leftoverDelete struct {
		name string
		size int64
	}
	var deletes []leftoverDelete
	for _, e := range entries {
		if e.IsDir() || !isRecordJSON(e.Name()) {
			continue
		}
		fileSeq, fileID, nameErr := parseRecordFileName(e.Name())
		if nameErr != nil {
			continue
		}
		if fileSeq > q.acceptedThrough {
			continue
		}
		authID, ok := authBySeq[fileSeq]
		if !ok || fileID != authID || e.Name() != recordFileName(fileSeq, authID) {
			// Should already be flagged by refresh; never delete unauthorized.
			q.ackRecoveryConflictBlock = true
			return nil
		}
		fi, err := e.Info()
		if err != nil {
			return fmt.Errorf("QUEUE_RECOVER_LEFTOVER_STAT_FAILED: %w", err)
		}
		deletes = append(deletes, leftoverDelete{name: e.Name(), size: fi.Size()})
	}

	for _, d := range deletes {
		filePath := filepath.Join(q.activeDir, d.name)
		if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("QUEUE_RECOVER_LEFTOVER_DELETE_FAILED: %w", err)
		}
		q.activeCount--
		q.activeBytes -= d.size
	}
	if q.activeCount < 0 {
		q.activeCount = 0
	}
	if q.activeBytes < 0 {
		q.activeBytes = 0
	}

	// Pending acceptedRecords requires active-dir fsync before auth may be cleared,
	// even when authorized files are already absent (unlink may not have been durable).
	if err := q.syncDirPhase(syncPhaseActiveCommit, q.activeDir); err != nil {
		return fmt.Errorf("QUEUE_RECOVER_LEFTOVER_DIR_SYNC_FAILED: %w", err)
	}

	highest := q.lastSeq
	if q.acceptedThrough > highest {
		highest = q.acceptedThrough
	}
	if err := q.persistSequenceMetaLocked(highest, q.acceptedThrough, nil); err != nil {
		return err
	}
	q.acceptedRecords = nil
	return q.refreshSequenceIntegrityLocked()
}

// CommitAccepted durably advances acceptedThrough for the exact submitted ACK
// prefix, then deletes only those authorized active records.
func (q *DurableQueue) CommitAccepted(records []Record, acceptedThrough uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.metaUncertain {
		return ErrMetaUncertain
	}
	if q.locationUncertain {
		return ErrQuarantineUncertain
	}
	if acceptedThrough == 0 {
		return errors.New("QUEUE_COMMIT_EMPTY: empty ACK does not authorize deletion")
	}
	if len(records) == 0 {
		return errors.New("QUEUE_COMMIT_EMPTY_BATCH: ACK without submitted records")
	}

	// Pending acceptedRecords is a durable cleanup barrier: reconcile it fully
	// before any later ACK may overwrite authorization metadata.
	if len(q.acceptedRecords) > 0 {
		if err := q.reconcileAcceptedLeftoversLocked(); err != nil {
			return err
		}
		if len(q.acceptedRecords) > 0 {
			if q.ackRecoveryConflictBlock {
				return ErrAckRecoveryConflict
			}
			if q.sequenceConflictBlock {
				return ErrSequenceConflict
			}
			return ErrPendingAckCleanup
		}
	}

	var prefix []Record
	found := false
	for _, r := range records {
		if r.Sequence == 0 || strings.TrimSpace(r.ID) == "" {
			return errors.New("QUEUE_COMMIT_INVALID_RECORD: submitted record missing sequence or id")
		}
		prefix = append(prefix, r)
		if r.Sequence == acceptedThrough {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("QUEUE_COMMIT_NOT_IN_BATCH: acceptedThrough %d was not in the submitted batch", acceptedThrough)
	}
	for i := 1; i < len(prefix); i++ {
		if prefix[i].Sequence != prefix[i-1].Sequence+1 {
			return errors.New("QUEUE_COMMIT_GAP: submitted ACK prefix is not contiguous")
		}
	}
	if acceptedThrough < q.acceptedThrough {
		return errors.New("QUEUE_META_REGRESS: acceptedThrough must never regress")
	}
	if q.acceptedThrough > 0 && prefix[0].Sequence != q.acceptedThrough+1 {
		q.sequenceGapBlock = true
		return ErrSequenceGap
	}

	authBySeq := make(map[uint64]string, len(prefix))
	authRefs := make([]acceptedRecordRef, 0, len(prefix))
	for _, r := range prefix {
		authBySeq[r.Sequence] = r.ID
		authRefs = append(authRefs, acceptedRecordRef{Sequence: r.Sequence, ID: r.ID})
	}

	entries, err := os.ReadDir(q.activeDir)
	if err != nil {
		return fmt.Errorf("QUEUE_COMMIT_SCAN_FAILED: %w", err)
	}

	bySeq := make(map[uint64][]string)
	for _, e := range entries {
		if e.IsDir() || !isRecordJSON(e.Name()) {
			continue
		}
		fileSeq, _, nameErr := parseRecordFileName(e.Name())
		if nameErr != nil {
			q.unknownSequenceBlock = true
			return ErrUnknownSequence
		}
		bySeq[fileSeq] = append(bySeq[fileSeq], e.Name())
	}
	for seq, names := range bySeq {
		if len(names) > 1 {
			q.sequenceConflictBlock = true
			return ErrSequenceConflict
		}
		// Blocker B: any active record with seq <= proposed acceptedThrough must be
		// an exact member of the ACK-authorized submitted prefix (including when
		// prior acceptedThrough was 0).
		if seq <= acceptedThrough {
			authID, ok := authBySeq[seq]
			if !ok || names[0] != recordFileName(seq, authID) {
				return fmt.Errorf("QUEUE_COMMIT_UNEXPECTED: unexpected active record sequence %d in ACK range", seq)
			}
		}
	}

	type authorizedDelete struct {
		name string
		size int64
	}
	var deletes []authorizedDelete
	for _, r := range prefix {
		expectedName := recordFileName(r.Sequence, r.ID)
		names := bySeq[r.Sequence]
		if len(names) == 0 {
			if r.Sequence <= q.acceptedThrough {
				continue
			}
			return fmt.Errorf("QUEUE_COMMIT_MISSING: expected active record %s", expectedName)
		}
		if len(names) != 1 || names[0] != expectedName {
			q.sequenceConflictBlock = true
			return ErrSequenceConflict
		}
		filePath := filepath.Join(q.activeDir, expectedName)
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("QUEUE_COMMIT_READ_FAILED: %w", err)
		}
		rec, _, parseErr := decodeRecordFile(expectedName, data)
		if parseErr != nil {
			return fmt.Errorf("QUEUE_COMMIT_RECORD_INVALID: %w", parseErr)
		}
		if rec.Sequence != r.Sequence || rec.ID != r.ID {
			q.sequenceConflictBlock = true
			return ErrSequenceConflict
		}
		fi, err := os.Lstat(filePath)
		if err != nil {
			return fmt.Errorf("QUEUE_COMMIT_STAT_FAILED: %w", err)
		}
		if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
			return errors.New("QUEUE_COMMIT_RECORD_NOT_REGULAR: active record path is not a regular file")
		}
		deletes = append(deletes, authorizedDelete{name: expectedName, size: fi.Size()})
	}

	highest := q.lastSeq
	if acceptedThrough > highest {
		highest = acceptedThrough
	}
	// Commit order: validate → reject unexpected → persist acceptedThrough+acceptedRecords
	// → meta fsync → rename → meta dir fsync → delete exact authorized → active dir fsync
	// → durably clear acceptedRecords.
	if err := q.persistSequenceMetaLocked(highest, acceptedThrough, authRefs); err != nil {
		return err
	}
	q.acceptedThrough = acceptedThrough
	q.acceptedRecords = cloneAcceptedRecords(authRefs)
	q.sequenceGapBlock = false

	for _, d := range deletes {
		filePath := filepath.Join(q.activeDir, d.name)
		if q.removeHook != nil {
			if err := q.removeHook(filePath); err != nil {
				return fmt.Errorf("QUEUE_COMMIT_DELETE_FAILED: %w", err)
			}
		}
		if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("QUEUE_COMMIT_DELETE_FAILED: %w", err)
		}
		q.activeCount--
		q.activeBytes -= d.size
	}
	if q.activeCount < 0 {
		q.activeCount = 0
	}
	if q.activeBytes < 0 {
		q.activeBytes = 0
	}

	if err := q.syncDirPhase(syncPhaseActiveCommit, q.activeDir); err != nil {
		return fmt.Errorf("QUEUE_COMMIT_DIR_SYNC_FAILED: %w", err)
	}

	if err := q.persistSequenceMetaLocked(highest, acceptedThrough, nil); err != nil {
		return err
	}
	q.acceptedRecords = nil
	return q.refreshSequenceIntegrityLocked()
}

func (q *DurableQueue) Stats() QueueStats {
	q.mu.Lock()
	defer q.mu.Unlock()

	var oldestAge time.Duration
	entries, err := os.ReadDir(q.activeDir)
	if err == nil {
		var minMod time.Time
		for _, e := range entries {
			if e.IsDir() || !isRecordJSON(e.Name()) {
				continue
			}
			fi, infoErr := e.Info()
			if infoErr != nil {
				continue
			}
			if minMod.IsZero() || fi.ModTime().Before(minMod) {
				minMod = fi.ModTime()
			}
		}
		if !minMod.IsZero() {
			oldestAge = time.Since(minMod)
		}
	}

	atCapacity := q.activeCount >= q.maxRecords || q.activeBytes >= q.maxBytes
	quarantineAtCapacity := q.quarantineCount >= q.maxQuarantineRecords || q.quarantineBytes >= q.maxQuarantineBytes
	hasActiveBlocked, hasActiveErr := q.hasActiveAfterBlockedLocked()
	return QueueStats{
		ActiveCount:              q.activeCount,
		ActiveBytes:              q.activeBytes,
		QuarantineCount:          q.quarantineCount,
		QuarantineBytes:          q.quarantineBytes,
		OldestAge:                oldestAge,
		LastSequence:             q.lastSeq,
		LineageStart:             q.lineageStart,
		AcceptedThrough:          q.acceptedThrough,
		AgentStreamID:            q.agentStreamId,
		LegacyCutoverSequence:    q.legacyCutoverSequence,
		MaxRecords:               q.maxRecords,
		MaxBytes:                 q.maxBytes,
		MaxQuarantineRecords:     q.maxQuarantineRecords,
		MaxQuarantineBytes:       q.maxQuarantineBytes,
		AtCapacity:               atCapacity,
		QuarantineAtCapacity:     quarantineAtCapacity,
		BlockedFromSequence:      q.blockedFromSequence,
		DeliveryBlocked:          q.unknownSequenceBlock || q.locationUncertain || q.metaUncertain || q.sequenceGapBlock || q.sequenceConflictBlock || q.ackRecoveryConflictBlock || hasActiveBlocked || hasActiveErr != nil,
		UnknownSequenceBlock:     q.unknownSequenceBlock,
		LocationUncertain:        q.locationUncertain,
		MetaUncertain:            q.metaUncertain,
		SequenceGapBlock:         q.sequenceGapBlock,
		SequenceConflictBlock:    q.sequenceConflictBlock,
		AckRecoveryConflictBlock: q.ackRecoveryConflictBlock,
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
