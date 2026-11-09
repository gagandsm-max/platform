package agentdrain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/spool"
)

const (
	agentBatchRecordType = "agent.delivery.batch"
	poisonDirName        = "agent-poison"
	// reasonSpoolCleanupFailed is reported when durable AckedThrough advanced but
	// record removal or final spool-directory fsync did not complete. Distinct from
	// Core delivery blockage (e.g. ERR_BATCH_TOO_LARGE).
	reasonSpoolCleanupFailed = "ERR_SPOOL_CLEANUP_FAILED"
	// reasonSpoolAckUncertain is reported while a bounded pending partial-ack
	// reconcile intent exists and watermark commitment has not yet been proved
	// or disproved. Distinct from proven cleanup failure and from Core 413 blocks.
	reasonSpoolAckUncertain = "ERR_SPOOL_ACK_UNCERTAIN"
	identityConflictCode    = "ERR_AGENT_IDENTITY_CONFLICT"
	identityConflictMarker  = "identity-conflict.quarantined"
)

type ErrorCode string

const (
	CodeInvalidConfig ErrorCode = "AGENTDRAIN_INVALID_CONFIG"
	CodeSpool         ErrorCode = "AGENTDRAIN_SPOOL"
	CodeTransport     ErrorCode = "AGENTDRAIN_TRANSPORT"
	CodeAckInvalid    ErrorCode = "AGENTDRAIN_ACK_INVALID"
	CodePoison        ErrorCode = "AGENTDRAIN_POISON"
	CodeCancelled     ErrorCode = "AGENTDRAIN_CANCELLED"
	CodeDeadline      ErrorCode = "AGENTDRAIN_DEADLINE_EXCEEDED"
)

type Error struct {
	Code      ErrorCode
	msg       string
	cause     error
	transient bool
}

func (e *Error) Error() string   { return string(e.Code) + ": " + e.msg }
func (e *Error) Unwrap() error   { return e.cause }
func (e *Error) Transient() bool { return e != nil && e.transient }

func codeErr(code ErrorCode, msg string, cause error, transient bool) error {
	return &Error{Code: code, msg: msg, cause: cause, transient: transient}
}

// durableBatchPayload mirrors agentreceiver.durableBatchPayload for decode-only use.
type durableBatchPayload struct {
	TenantID      string          `json:"tenantId"`
	AgentID       string          `json:"agentId"`
	SensorID      string          `json:"sensorId"`
	Generation    int64           `json:"assignmentGeneration"`
	FirstSequence uint64          `json:"firstSequence"`
	LastSequence  uint64          `json:"lastSequence"`
	RecordIDs     []string        `json:"recordIds"`
	BatchChecksum string          `json:"batchChecksum"`
	BatchID       string          `json:"batchId"`
	CorrelationID string          `json:"correlationId"`
	Timestamp     time.Time       `json:"timestamp"`
	Envelope      json.RawMessage `json:"envelope"`
}

type envelopeLite struct {
	Records json.RawMessage `json:"records"`
}

type Config struct {
	SensorID             string
	StreamID             string
	AgentSpoolDir        string
	MaxBatchRecords      int
	MaxBatchPayloadBytes int
	MaxRequestBytes      int
	RequestTimeout       time.Duration
	InitialBackoff       time.Duration
	MaxBackoff           time.Duration
	JitterFraction       float64
	IdleInterval         time.Duration
	ShutdownGrace        time.Duration
	Logger               *slog.Logger
}

type Transport interface {
	UploadAgentTelemetry(context.Context, coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error)
}

type Spool interface {
	ReadOrReserveOutboundBatch(int, int) (spool.Batch, error)
	CompleteOutboundBatch(uint64) error
	AbandonOutboundBatch() error
	AcknowledgeThrough(uint64) error
	Stats() (spool.Stats, error)
}

type Metrics struct {
	PendingRecords       atomic.Int64
	PendingBytes         atomic.Int64
	OldestAgeSeconds     atomic.Int64
	Attempts             atomic.Uint64
	Successes            atomic.Uint64
	FailuresTransient    atomic.Uint64
	FailuresPermanent    atomic.Uint64
	DurableAcks          atomic.Uint64
	PoisonCount          atomic.Uint64
	AuthorityPoisonCount atomic.Uint64
	Backpressure         atomic.Uint64
	LastSuccessUnix      atomic.Int64
}

func (m *Metrics) LastSuccessAge() time.Duration {
	sec := m.LastSuccessUnix.Load()
	if sec == 0 {
		return 0
	}
	return time.Since(time.Unix(sec, 0))
}

// Delivery health states for Sensor supportability. Distinct from agent_spool
// filesystem capacity: blocked Core delivery can coexist with a writable spool.
const (
	DeliveryStateUp       = "UP"
	DeliveryStateDegraded = "DEGRADED"
)

// DeliveryStatus is a bounded Agent drain delivery snapshot (no payloads/secrets).
type DeliveryStatus struct {
	State           string
	ReasonCode      string
	Blocked         bool
	Quarantined     bool
	PendingRecords  int64
	PendingBytes    int64
	OldestAgeSecs   int64
	FailuresPerm    uint64
	DurableAcks     uint64
	BlockedSince    time.Time
	LastSuccessTime time.Time
	FirstSequence   uint64
	LastSequence    uint64
}

type sizeRejectedReporter interface {
	OutboundBatchSizeRejected() (bool, error)
}

// ackedCleanupReconciler is implemented by DiskSpool. Drain never deletes
// record-like files by filename; cleanup stays under spool lock/validation.
type ackedCleanupReconciler interface {
	ReconcileAckedCleanup() error
}

// unresolvedCleanupReporter is an O(1) spool signal for unresolved post-ACK
// cleanup/fsync failure. Snapshot must not scan the spool directory.
type unresolvedCleanupReporter interface {
	UnresolvedAckedCleanup() (bool, uint64, error)
}

// ackedThroughReporter is an O(1) durable watermark reader. Prefer this over
// Stats() when proving post-commit AcknowledgeThrough advancement: Stats scans
// pending records and filesystem capacity and can fail transiently after the
// watermark is already durable.
type ackedThroughReporter interface {
	AckedThrough() (uint64, error)
}

type Drain struct {
	cfg       Config
	spool     Spool
	transport Transport
	metrics   *Metrics
	logger    *slog.Logger
	poisonDir string
	syncDir   func(string) error
	// Optional test-only poison publication seams. Nil uses production os.File / os.Rename.
	poisonWrite  func(f *os.File, p []byte) (int, error)
	poisonSync   func(f *os.File) error
	poisonRename func(oldpath, newpath string) error

	runMu sync.Mutex // Serializes selection, upload and ACK within this drain.
	// A Core identity conflict rejects the whole request before replay reservation.
	// Select singleton Sensor records through the rejected range. A valid
	// prefix must not cause the remaining range to be batched/rejected again.
	identityIsolationUntil uint64 // guarded by runMu; zero means inactive
	mu                     sync.Mutex
	cancel                 context.CancelFunc
	started                bool
	done                   chan struct{}
	startOnce              sync.Once
	stopOnce               sync.Once
	wait                   func(context.Context, time.Duration) error
	jitter                 func(time.Duration, float64) time.Duration

	deliveryMu          sync.RWMutex
	deliveryBlocked     bool
	identityQuarantined bool
	deliveryReason      string
	blockedSince        time.Time
	blockedFirstSeq     uint64
	blockedLastSeq      uint64
	// deliveryGen advances on every durable-ACK clear so a Snapshot that
	// observed a pre-ACK legacy sizeRejected sidecar cannot reassert DEGRADED
	// after recovery. Observations sample gen before spool I/O and re-check
	// under deliveryMu before marking.
	deliveryGen uint64

	// cleanupFault is independent of deliveryBlocked. It remains set after a
	// partial AcknowledgeThrough (watermark durable, cleanup/fsync failed) until
	// orphan removal + spool-directory sync succeed. Empty backlog and unrelated
	// Core uploads must not clear it.
	cleanupFault      bool
	cleanupFaultSince time.Time
	cleanupFaultSeq   uint64
	// cleanupGen advances on every clearCleanupFault so a Snapshot that read a
	// positive unresolved-cleanup flag cannot reassert DEGRADED after recovery.
	// Observations sample gen before spool I/O and re-check under deliveryMu.
	cleanupGen uint64
	// cleanupRetryAfter / cleanupRetryBackoff pace expensive watermark inspection,
	// ReconcileAckedCleanup + directory fsync under persistent cleanup or pending
	// partial-ack reconcile failure. Skipping a paced attempt must never clear the
	// fault/intent; pending telemetry delivery continues.
	cleanupRetryAfter   time.Time
	cleanupRetryBackoff time.Duration

	// pendingPartialAckSeq retains one bounded reconciliation intent when a
	// partial AcknowledgeThrough/CompleteOutboundBatch error could not be
	// classified because every watermark inspector failed. Fail closed
	// immediately (retain prior delivery block when present); later successful
	// inspection recovers health, including with an empty logical backlog.
	// Zero means no pending intent. While set, RunOnce must not select another
	// batch — only unresolved watermark proof is a delivery barrier; a proven
	// watermark with cleanupFault alone must still allow later delivery.
	// pendingPartialAckCoreACK / pendingPartialAckAuthorityPoison classify the
	// original operation for one-shot deferred counting (never overwritten).
	pendingPartialAckSeq             uint64
	pendingPartialAckCoreACK         bool // true => Core ACK path; false => poison park
	pendingPartialAckAuthorityPoison bool // poison park only; derived from isAuthorityPoisonCode

	// Optional test-only seam. Nil in production. Invoked after a positive
	// legacy sizeRejected+pending observation and before the generation-gated
	// mark attempt, so barrier tests can interleave a durable ACK.
	afterLegacySizeObserve func()
	// Optional test-only seam. Nil in production. Invoked after a positive
	// unresolved-cleanup observation and before the generation-gated mark,
	// so barrier tests can interleave clearCleanupFault.
	afterCleanupUnresolvedObserve func()
	// Optional test-only seam. Nil in production. Invoked while deliveryMu is
	// held inside installProvenCleanupAfterPartialAck after clearing obsolete
	// delivery block / pending intent and before latching cleanupFault, so
	// barrier tests can prove Snapshot never observes a transient UP window.
	duringBlockedToCleanupTransition func()
}

func New(cfg Config, sp Spool, tr Transport) (*Drain, error) {
	if strings.TrimSpace(cfg.SensorID) == "" || strings.TrimSpace(cfg.StreamID) == "" || sp == nil || tr == nil {
		return nil, codeErr(CodeInvalidConfig, "sensor id, stream id, spool and transport are required", nil, false)
	}
	if !validAgentSpoolStreamID(cfg.StreamID) {
		return nil, codeErr(CodeInvalidConfig, "agent spool stream id grammar invalid", nil, false)
	}
	if cfg.MaxBatchRecords <= 0 {
		cfg.MaxBatchRecords = 32
	}
	if cfg.MaxBatchPayloadBytes <= 0 {
		cfg.MaxBatchPayloadBytes = 1024 * 1024
	}
	if cfg.MaxRequestBytes <= 0 {
		cfg.MaxRequestBytes = 2 * 1024 * 1024
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 15 * time.Second
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 500 * time.Millisecond
	}
	if cfg.MaxBackoff < cfg.InitialBackoff {
		cfg.MaxBackoff = 30 * time.Second
	}
	if cfg.JitterFraction < 0 || cfg.JitterFraction > 1 {
		cfg.JitterFraction = 0.1
	}
	if cfg.IdleInterval <= 0 {
		cfg.IdleInterval = time.Second
	}
	if cfg.ShutdownGrace <= 0 {
		cfg.ShutdownGrace = 10 * time.Second
	}
	dir := filepath.Clean(strings.TrimSpace(cfg.AgentSpoolDir))
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, codeErr(CodeInvalidConfig, "agent spool directory must be absolute", nil, false)
	}
	poisonDir := filepath.Join(dir, poisonDirName)
	if err := os.MkdirAll(poisonDir, 0750); err != nil {
		return nil, codeErr(CodeInvalidConfig, "failed to create agent-poison directory", err, false)
	}
	markerInfo, markerErr := os.Lstat(filepath.Join(poisonDir, identityConflictMarker))
	if markerErr != nil && !os.IsNotExist(markerErr) {
		return nil, codeErr(CodeInvalidConfig, "failed inspecting identity quarantine marker", markerErr, false)
	}
	if markerErr == nil && !markerInfo.Mode().IsRegular() {
		return nil, codeErr(CodeInvalidConfig, "identity quarantine marker is not a regular file", nil, false)
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	d := &Drain{
		cfg:                 cfg,
		spool:               sp,
		transport:           tr,
		metrics:             &Metrics{},
		logger:              logger,
		poisonDir:           poisonDir,
		syncDir:             defaultSyncDir,
		identityQuarantined: markerErr == nil,
		done:                make(chan struct{}),
		wait:                waitContext,
		jitter:              jitterDuration,
	}
	return d, nil
}

func (d *Drain) Metrics() *Metrics { return d.metrics }

// Snapshot returns the current Agent delivery-health view for Sensor supportability.
// It never infers monitored-device failure and never exposes payloads or Core bodies.
func (d *Drain) Snapshot() DeliveryStatus {
	if d == nil {
		return DeliveryStatus{State: DeliveryStateUp}
	}
	d.refreshStats()
	d.observeLegacySizeBlock()
	d.observeOrphanedCleanupFault()

	d.deliveryMu.RLock()
	defer d.deliveryMu.RUnlock()
	st := DeliveryStatus{
		State:          DeliveryStateUp,
		PendingRecords: d.metrics.PendingRecords.Load(),
		PendingBytes:   d.metrics.PendingBytes.Load(),
		OldestAgeSecs:  d.metrics.OldestAgeSeconds.Load(),
		FailuresPerm:   d.metrics.FailuresPermanent.Load(),
		DurableAcks:    d.metrics.DurableAcks.Load(),
		Quarantined:    d.identityQuarantined,
	}
	if sec := d.metrics.LastSuccessUnix.Load(); sec > 0 {
		st.LastSuccessTime = time.Unix(sec, 0).UTC()
	}
	if d.deliveryBlocked {
		st.State = DeliveryStateDegraded
		st.Blocked = true
		st.ReasonCode = d.deliveryReason
		st.BlockedSince = d.blockedSince
		st.FirstSequence = d.blockedFirstSeq
		st.LastSequence = d.blockedLastSeq
		return st
	}
	if d.pendingPartialAckSeq != 0 {
		// Pending watermark proof alone is truthful DEGRADED uncertainty — not a
		// Core delivery block and not a proven cleanup failure.
		st.State = DeliveryStateDegraded
		st.Blocked = false
		st.ReasonCode = reasonSpoolAckUncertain
		st.FirstSequence = d.pendingPartialAckSeq
		st.LastSequence = d.pendingPartialAckSeq
		return st
	}
	if d.cleanupFault {
		// Storage cleanup failure is not a Core delivery block: Blocked stays false
		// so operators can distinguish obsolete 413 blockage from unresolved I/O.
		st.State = DeliveryStateDegraded
		st.Blocked = false
		st.ReasonCode = reasonSpoolCleanupFailed
		st.BlockedSince = d.cleanupFaultSince
		st.FirstSequence = d.cleanupFaultSeq
		st.LastSequence = d.cleanupFaultSeq
	}
	if d.identityQuarantined && st.State == DeliveryStateUp {
		st.State = DeliveryStateDegraded
		st.ReasonCode = identityConflictCode
	}
	return st
}

func (d *Drain) markDeliveryBlocked(reason string, batch spool.Batch) {
	if d == nil {
		return
	}
	sanitized := sanitizeReason(reason)
	d.deliveryMu.Lock()
	defer d.deliveryMu.Unlock()
	if !d.deliveryBlocked {
		d.blockedSince = time.Now().UTC()
	}
	d.deliveryBlocked = true
	d.deliveryReason = sanitized
	d.blockedFirstSeq = batch.FirstSequence
	d.blockedLastSeq = batch.LastSequence
}

func (d *Drain) clearDeliveryBlocked() {
	if d == nil {
		return
	}
	d.deliveryMu.Lock()
	defer d.deliveryMu.Unlock()
	d.deliveryBlocked = false
	d.deliveryReason = ""
	d.blockedSince = time.Time{}
	d.blockedFirstSeq = 0
	d.blockedLastSeq = 0
	d.deliveryGen++
	// Intentionally does not clear cleanupFault: durable Core ACK / poison park
	// success must not hide an unresolved spool cleanup failure.
}

func (d *Drain) markCleanupFault(seq uint64) {
	if d == nil {
		return
	}
	d.deliveryMu.Lock()
	defer d.deliveryMu.Unlock()
	if !d.cleanupFault {
		d.cleanupFaultSince = time.Now().UTC()
		// Fresh fault: allow an immediate first recovery attempt.
		d.cleanupRetryAfter = time.Time{}
		d.cleanupRetryBackoff = 0
	}
	d.cleanupFault = true
	d.cleanupFaultSeq = seq
}

// installProvenCleanupAfterPartialAck atomically consumes pending reconcile
// intent (if any), clears an obsolete Core delivery block, and latches
// cleanupFault under one deliveryMu critical section so a concurrent Snapshot
// cannot observe a transient UP window (pending-only DEGRADED → UP → cleanup,
// or blocked → UP → cleanup). Advances deliveryGen for legacy-size observation
// fencing. Does not fabricate Core ACK, performs no spool filesystem I/O, and
// must not be called while holding deliveryMu.
func (d *Drain) installProvenCleanupAfterPartialAck(seq uint64) {
	if d == nil {
		return
	}
	d.deliveryMu.Lock()
	defer d.deliveryMu.Unlock()
	d.pendingPartialAckSeq = 0
	d.pendingPartialAckCoreACK = false
	d.pendingPartialAckAuthorityPoison = false
	d.deliveryBlocked = false
	d.deliveryReason = ""
	d.blockedSince = time.Time{}
	d.blockedFirstSeq = 0
	d.blockedLastSeq = 0
	d.deliveryGen++
	if d.duringBlockedToCleanupTransition != nil {
		d.duringBlockedToCleanupTransition()
	}
	if !d.cleanupFault {
		d.cleanupFaultSince = time.Now().UTC()
		d.cleanupRetryAfter = time.Time{}
		d.cleanupRetryBackoff = 0
	}
	d.cleanupFault = true
	d.cleanupFaultSeq = seq
}

// consumePendingPartialAckInstallCleanup consumes a matching pending intent and
// installs proven cleanup degradation atomically. Returns the original operation
// classification for one-shot deferred counting. consumed=false when no matching
// intent remains (lost/cleared). No spool I/O under deliveryMu.
func (d *Drain) consumePendingPartialAckInstallCleanup(seq uint64) (coreACK bool, authorityPoison bool, consumed bool) {
	if d == nil {
		return false, false, false
	}
	d.deliveryMu.Lock()
	defer d.deliveryMu.Unlock()
	if d.pendingPartialAckSeq == 0 || d.pendingPartialAckSeq != seq {
		return false, false, false
	}
	coreACK = d.pendingPartialAckCoreACK
	authorityPoison = d.pendingPartialAckAuthorityPoison
	d.pendingPartialAckSeq = 0
	d.pendingPartialAckCoreACK = false
	d.pendingPartialAckAuthorityPoison = false
	d.deliveryBlocked = false
	d.deliveryReason = ""
	d.blockedSince = time.Time{}
	d.blockedFirstSeq = 0
	d.blockedLastSeq = 0
	d.deliveryGen++
	if d.duringBlockedToCleanupTransition != nil {
		d.duringBlockedToCleanupTransition()
	}
	if !d.cleanupFault {
		d.cleanupFaultSince = time.Now().UTC()
		d.cleanupRetryAfter = time.Time{}
		d.cleanupRetryBackoff = 0
	}
	d.cleanupFault = true
	d.cleanupFaultSeq = seq
	return coreACK, authorityPoison, true
}

// markObservedCleanupFault applies a Snapshot unresolved-cleanup observation only
// when cleanupGen is unchanged since the observation began. A concurrent recovery
// that called clearCleanupFault advances cleanupGen and wins.
func (d *Drain) markObservedCleanupFault(gen uint64, seq uint64) {
	if d == nil {
		return
	}
	d.deliveryMu.Lock()
	defer d.deliveryMu.Unlock()
	if d.cleanupGen != gen || d.cleanupFault {
		return
	}
	d.cleanupFaultSince = time.Now().UTC()
	d.cleanupFault = true
	d.cleanupFaultSeq = seq
	d.cleanupRetryAfter = time.Time{}
	d.cleanupRetryBackoff = 0
}

func (d *Drain) clearCleanupFault() {
	if d == nil {
		return
	}
	d.deliveryMu.Lock()
	defer d.deliveryMu.Unlock()
	d.cleanupFault = false
	d.cleanupFaultSince = time.Time{}
	d.cleanupFaultSeq = 0
	d.cleanupRetryAfter = time.Time{}
	d.cleanupRetryBackoff = 0
	d.cleanupGen++
}

// scheduleCleanupRetry paces the next expensive cleanup / watermark-inspection
// recovery under deliveryMu. It never clears cleanupFault or pending reconcile
// intent. Callers must not hold deliveryMu.
func (d *Drain) scheduleCleanupRetry() {
	if d == nil {
		return
	}
	d.deliveryMu.Lock()
	defer d.deliveryMu.Unlock()
	if !d.cleanupFault && d.pendingPartialAckSeq == 0 {
		return
	}
	base := d.cleanupRetryBackoff
	if base <= 0 {
		base = d.cfg.InitialBackoff
	}
	jitterFn := d.jitter
	if jitterFn == nil {
		jitterFn = jitterDuration
	}
	backoff := jitterFn(base, d.cfg.JitterFraction)
	if backoff > d.cfg.MaxBackoff {
		backoff = d.cfg.MaxBackoff
	}
	if backoff < d.cfg.InitialBackoff {
		backoff = d.cfg.InitialBackoff
	}
	d.cleanupRetryAfter = time.Now().Add(backoff)
	// Overflow-safe doubling: duration multiply can wrap negative near max int64.
	next := base * 2
	if next < base || next > d.cfg.MaxBackoff {
		next = d.cfg.MaxBackoff
	}
	if next < d.cfg.InitialBackoff {
		next = d.cfg.InitialBackoff
	}
	d.cleanupRetryBackoff = next
}

// observeLegacySizeBlock surfaces durable pre-65fe619 sizeRejected sidecars as
// delivery degradation before the next upload attempt, without suppressing replay.
// Spool observation stays outside deliveryMu (lock order: spool then delivery;
// never hold deliveryMu across spool I/O). A sampled deliveryGen must still
// match under the write lock or the observation is discarded as stale vs ACK.
func (d *Drain) observeLegacySizeBlock() {
	reporter, ok := d.spool.(sizeRejectedReporter)
	if !ok {
		return
	}
	d.deliveryMu.RLock()
	gen := d.deliveryGen
	already := d.deliveryBlocked
	d.deliveryMu.RUnlock()
	if already {
		return
	}
	blocked, err := reporter.OutboundBatchSizeRejected()
	if err != nil || !blocked {
		return
	}
	if d.metrics.PendingRecords.Load() <= 0 {
		return
	}
	if d.afterLegacySizeObserve != nil {
		d.afterLegacySizeObserve()
	}
	d.markLegacyDeliveryBlocked(gen, "ERR_BATCH_TOO_LARGE")
}

// markLegacyDeliveryBlocked applies a legacy sizeRejected observation only when
// deliveryGen is unchanged since the observation began. A concurrent durable ACK
// that called clearDeliveryBlocked advances deliveryGen and wins.
func (d *Drain) markLegacyDeliveryBlocked(gen uint64, reason string) {
	if d == nil {
		return
	}
	sanitized := sanitizeReason(reason)
	d.deliveryMu.Lock()
	defer d.deliveryMu.Unlock()
	if d.deliveryGen != gen || d.deliveryBlocked {
		return
	}
	// Pending may already be zero after an ACK that has not yet advanced gen in
	// an alternate interleaving; never reassert a block with an empty backlog.
	if d.metrics.PendingRecords.Load() <= 0 {
		return
	}
	if !d.deliveryBlocked {
		d.blockedSince = time.Now().UTC()
	}
	d.deliveryBlocked = true
	d.deliveryReason = sanitized
	d.blockedFirstSeq = 0
	d.blockedLastSeq = 0
}

func (d *Drain) Start(parent context.Context) error {
	if parent == nil {
		return codeErr(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	startedNow := false
	d.startOnce.Do(func() {
		startedNow = true
		ctx, cancel := context.WithCancel(parent)
		d.mu.Lock()
		d.cancel = cancel
		d.started = true
		d.mu.Unlock()
		go d.loop(ctx)
	})
	if !startedNow {
		return codeErr(CodeInvalidConfig, "agent drain already started", nil, false)
	}
	return nil
}

func (d *Drain) loop(ctx context.Context) {
	defer close(d.done)
	for {
		_, err := d.RunOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		delay := d.cfg.IdleInterval
		if err != nil {
			var te *Error
			if errors.As(err, &te) && te.transient {
				delay = d.cfg.MaxBackoff
			}
		}
		if err := d.wait(ctx, delay); err != nil {
			return
		}
	}
}

func (d *Drain) RunOnce(ctx context.Context) (bool, error) {
	d.runMu.Lock()
	defer d.runMu.Unlock()
	if err := ctxErr(ctx); err != nil {
		return false, err
	}
	d.refreshStats()
	// Attempt deferred partial-ack health reconcile (when inspectors previously
	// failed) and cleanup recovery before selecting work. Empty backlog alone
	// does not clear an unresolved storage-cleanup fault. Unresolved watermark
	// proof is a hard barrier: do not select/upload/ack/park another batch while
	// intent remains. Proven cleanupFault alone must still allow delivery.
	d.tryReconcilePendingPartialAck()
	d.tryRecoverCleanupFault()
	if d.hasUnresolvedPendingPartialAck() {
		return false, nil
	}
	maxRecords := d.cfg.MaxBatchRecords
	if d.identityIsolationUntil != 0 {
		maxRecords = 1
	}
	batch, err := d.spool.ReadOrReserveOutboundBatch(maxRecords, d.cfg.MaxBatchPayloadBytes)
	if err != nil {
		return false, codeErr(CodeSpool, "failed reading agent spool batch", err, false)
	}
	if len(batch.Records) == 0 {
		return false, nil
	}

	// Under spool protection (80%) or high FS use, keep draining so ACK→delete can free space.
	// Pausing here would wedge the Agent spool (uploader does not pause either). Observe only.
	if st, serr := d.spool.Stats(); serr == nil {
		if st.FilesystemUsedPct >= 95 || st.ProtectionState == spool.ProtectionTriggered {
			d.metrics.Backpressure.Add(1)
		}
	}

	req, err := d.makeRequest(batch)
	if err != nil {
		// Only a locally identified malformed singleton can authorize parking.
		// A failure elsewhere in a frozen batch (or a request/configuration limit)
		// does not identify the head as poison.
		var malformed *Error
		isMalformed := errors.As(err, &malformed) && malformed.Code == CodePoison
		if len(batch.Records) != 1 || !isMalformed {
			reason := "ERR_AGENT_REQUEST_BUILD_FAILED"
			if isMalformed {
				reason = "ERR_MALFORMED_DURABLE_PAYLOAD"
			}
			return false, d.retainBlocked(ctx, batch, reason, err)
		}
		if aerr := d.spool.AbandonOutboundBatch(); aerr != nil {
			return false, codeErr(CodeSpool, "failed abandoning outbound window before poison park", aerr, false)
		}
		if parkErr := d.parkHead(batch, "ERR_MALFORMED_DURABLE_PAYLOAD", err); parkErr != nil {
			return false, parkErr
		}
		// Durable quarantine is not a Core ACK, but the obsolete delivery-block
		// reason no longer applies once the poisoned head is parked and removed.
		d.clearDeliveryBlocked()
		d.refreshStats()
		return false, codeErr(CodePoison, "parked malformed durable agent batch", err, false)
	}

	uploadCtx, cancel := context.WithTimeout(ctx, d.cfg.RequestTimeout)
	defer cancel()
	d.metrics.Attempts.Add(1)
	ack, err := d.transport.UploadAgentTelemetry(uploadCtx, req)
	if err == nil {
		accepted := ack.AcceptedThroughIngressSequence
		if aerr := d.spool.CompleteOutboundBatch(accepted); aerr != nil {
			// CompleteOutboundBatch may fail after durably advancing AckedThrough
			// (record cleanup or outbound-window sidecar unlink/fsync). Reconcile
			// only from authoritative durable proof; never invent a second Core ACK.
			if d.reconcileAfterPartialSpoolCommit(accepted, true, false) {
				d.metrics.Successes.Add(1)
				d.metrics.DurableAcks.Add(1)
				d.metrics.LastSuccessUnix.Store(time.Now().Unix())
				d.logger.Warn("Core agent ACK durable in spool but cleanup failed",
					"event_code", reasonSpoolCleanupFailed,
					"safe_context", map[string]any{
						"accepted_through":  accepted,
						"partial_spool_ack": true,
					})
				return false, codeErr(CodeSpool, "durable agent ACK applied but spool cleanup failed", aerr, false)
			}
			return false, codeErr(CodeSpool, "failed applying durable agent ACK to spool", aerr, false)
		}
		// Full CompleteOutboundBatch success proves commitment for accepted; clear
		// only a matching pending intent (never erase a different older intent).
		d.clearPendingPartialAckReconcileMatching(accepted)
		d.metrics.Successes.Add(1)
		d.metrics.DurableAcks.Add(1)
		d.metrics.LastSuccessUnix.Store(time.Now().Unix())
		if accepted >= d.identityIsolationUntil {
			d.identityIsolationUntil = 0
		}
		d.clearDeliveryBlocked()
		d.refreshStats()
		return true, nil
	}

	// A current 413 cannot disprove earlier acceptance with a lost ACK. Keep
	// the exact frozen request eligible for replay, including legacy singleton
	// SizeRejected windows. Request-size errors use the paced retainBlocked path.
	if isIdentityConflictRejection(err) {
		// Core validates identity before watermark reservation and publication.
		// Its batch-level rejection does not identify which Sensor record failed.
		if len(batch.Records) != 1 {
			if aerr := d.spool.AbandonOutboundBatch(); aerr != nil {
				return false, codeErr(CodeSpool, "failed isolating identity-conflict window", aerr, false)
			}
			d.identityIsolationUntil = batch.LastSequence
			return false, codeErr(CodeTransport, "identity-conflict batch narrowed without ACK", err, false)
		}
		if aerr := d.spool.AbandonOutboundBatch(); aerr != nil {
			return false, codeErr(CodeSpool, "failed abandoning identity-conflict window", aerr, false)
		}
		if d.identityIsolationUntil == 0 {
			d.identityIsolationUntil = batch.LastSequence
		}
		if parkErr := d.parkHead(batch, identityConflictCode, err); parkErr != nil {
			return false, parkErr
		}
		if batch.LastSequence >= d.identityIsolationUntil {
			d.identityIsolationUntil = 0
		}
		d.metrics.FailuresPermanent.Add(1)
		d.clearDeliveryBlocked()
		d.refreshStats()
		return false, codeErr(CodePoison, "quarantined identity-conflict agent record", err, false)
	}

	if isRecordAttributableCoreError(err) {
		code := terminalCode(err)
		// Core rejects the whole request without identifying an offending record.
		// Even one Sensor record may contain several independent Agent deliveries.
		if len(req.Records) != 1 || !singleDelivery(req.Records[0].Records) {
			return false, d.retainBlocked(ctx, batch, code, err)
		}
		// Only a record-attributable singleton rejection may abandon and durably park.
		if aerr := d.spool.AbandonOutboundBatch(); aerr != nil {
			return false, codeErr(CodeSpool, "failed abandoning outbound window before poison park", aerr, false)
		}
		if parkErr := d.parkHead(batch, code, err); parkErr != nil {
			return false, parkErr
		}
		d.metrics.FailuresPermanent.Add(1)
		// Clear only after durable poison publish + spool removal succeeded.
		// Poison quarantine must not increment DurableAcks.
		d.clearDeliveryBlocked()
		d.refreshStats()
		return false, codeErr(CodePoison, "parked terminal agent-telemetry failure", err, false)
	}

	if isTransient(err) {
		d.metrics.FailuresTransient.Add(1)
		backoff := jitterDuration(d.cfg.InitialBackoff, d.cfg.JitterFraction)
		if backoff > d.cfg.MaxBackoff {
			backoff = d.cfg.MaxBackoff
		}
		_ = d.wait(ctx, backoff)
		return false, codeErr(CodeTransport, "transient Core agent-telemetry failure; spool retained", err, true)
	}

	return false, d.retainBlocked(ctx, batch, terminalCode(err), err)
}

// singleDelivery deliberately fails closed on missing, empty or malformed arrays.
func singleDelivery(raw json.RawMessage) bool {
	var records []json.RawMessage
	return json.Unmarshal(raw, &records) == nil && len(records) == 1
}

// retainBlocked preserves the frozen range and reports the original cause without
// logging payloads or untrusted Core messages. Pace RunOnce as well as the loop:
// direct callers must not spin on an ambiguous permanent rejection.
func (d *Drain) retainBlocked(ctx context.Context, batch spool.Batch, reason string, cause error) error {
	d.metrics.FailuresPermanent.Add(1)
	d.metrics.Backpressure.Add(1)
	d.markDeliveryBlocked(reason, batch)
	d.logger.Warn("Agent-ingress delivery blocked; spool retained",
		"event_code", sanitizeReason(reason),
		"safe_context", map[string]any{
			"first_sequence": batch.FirstSequence, "last_sequence": batch.LastSequence,
			"record_count":         len(batch.Records),
			"singleton_size_block": reason == "ERR_BATCH_TOO_LARGE" && len(batch.Records) == 1,
			"retry_delay_ms":       d.cfg.MaxBackoff.Milliseconds(),
			"delivery_health":      DeliveryStateDegraded,
		})
	if reason == "ERR_BATCH_TOO_LARGE" || reason == "ERR_REQUEST_TOO_LARGE" {
		d.logger.Warn("Restore Core request-size compatibility; exact frozen request will be retried",
			"event_code", sanitizeReason(reason), "retry_delay_ms", d.cfg.MaxBackoff.Milliseconds())
	}
	if err := d.wait(ctx, d.cfg.MaxBackoff); err != nil {
		if cerr := ctxErr(ctx); cerr != nil {
			return cerr
		}
	}
	return codeErr(CodeTransport, "agent-telemetry delivery blocked; spool retained", cause, false)
}

func (d *Drain) makeRequest(batch spool.Batch) (coreclient.AgentTelemetryBatch, error) {
	records := make([]coreclient.AgentIngressRecord, 0, len(batch.Records))
	h := sha256.New()
	for _, rec := range batch.Records {
		if rec.Type != agentBatchRecordType {
			return coreclient.AgentTelemetryBatch{}, codeErr(CodePoison, "unexpected durable spool record type", nil, false)
		}
		var payload durableBatchPayload
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			return coreclient.AgentTelemetryBatch{}, codeErr(CodePoison, "corrupt durable agent payload", err, false)
		}
		if strings.TrimSpace(payload.TenantID) == "" || strings.TrimSpace(payload.AgentID) == "" ||
			strings.TrimSpace(payload.SensorID) == "" || payload.Generation < 1 ||
			payload.FirstSequence == 0 || payload.LastSequence < payload.FirstSequence {
			return coreclient.AgentTelemetryBatch{}, codeErr(CodePoison, "incomplete durable agent payload", nil, false)
		}
		var env envelopeLite
		deliveryRecords := json.RawMessage("[]")
		if len(payload.Envelope) > 0 {
			if err := json.Unmarshal(payload.Envelope, &env); err == nil && len(env.Records) > 0 {
				deliveryRecords = env.Records
			}
		}
		acceptedAt := payload.Timestamp.UTC().Format(time.RFC3339Nano)
		ingress := coreclient.AgentIngressRecord{
			SensorIngressSequence: rec.Sequence,
			SensorIngressRecordID: fmt.Sprintf("agent-spool-%d", rec.Sequence),
			TenantID:              payload.TenantID,
			AgentID:               payload.AgentID,
			AcceptingSensorID:     payload.SensorID,
			AssignmentGeneration:  payload.Generation,
			AgentFirstSequence:    payload.FirstSequence,
			AgentLastSequence:     payload.LastSequence,
			AgentBatchID:          payload.BatchID,
			CorrelationID:         payload.CorrelationID,
			AcceptedAt:            acceptedAt,
			Records:               deliveryRecords,
		}
		// Stable checksum shared with Core AgentIngestionValidator (scalar identity fields).
		var line = fmt.Sprintf("%d|%s|%d|%d|%d|%s|%s|%s",
			ingress.SensorIngressSequence, ingress.AgentBatchID,
			ingress.AgentFirstSequence, ingress.AgentLastSequence, ingress.AssignmentGeneration,
			ingress.TenantID, ingress.AgentID, ingress.AcceptingSensorID)
		var deliveries []struct {
			Sequence uint64 `json:"sequence"`
			ID       string `json:"id"`
			Checksum string `json:"checksum"`
		}
		_ = json.Unmarshal(deliveryRecords, &deliveries)
		for _, dr := range deliveries {
			line += fmt.Sprintf("|%d|%s|%s", dr.Sequence, dr.ID, dr.Checksum)
		}
		_, _ = h.Write([]byte(line))
		records = append(records, ingress)
	}
	checksum := hex.EncodeToString(h.Sum(nil))
	if batch.FirstSequence == 0 || batch.LastSequence < batch.FirstSequence ||
		batch.FirstSequence > spool.MaxSafeSequence || batch.LastSequence > spool.MaxSafeSequence {
		return coreclient.AgentTelemetryBatch{}, fmt.Errorf("agent spool batch sequence exceeds MAX_SAFE_SEQUENCE or is invalid")
	}
	batchID := fmt.Sprintf("agent-ingress-%s-%s-%020d-%020d-%s",
		d.cfg.SensorID, d.cfg.StreamID, batch.FirstSequence, batch.LastSequence, checksum[:16])
	corr := fmt.Sprintf("agentdrain-%s", batchID)
	out := coreclient.AgentTelemetryBatch{
		SchemaVersion:            2,
		SensorID:                 d.cfg.SensorID,
		SensorAgentSpoolStreamID: d.cfg.StreamID,
		BatchID:                  batchID,
		FirstIngressSequence:     batch.FirstSequence,
		LastIngressSequence:      batch.LastSequence,
		RecordCount:              len(records),
		Checksum:                 checksum,
		CorrelationID:            corr,
		Records:                  records,
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return coreclient.AgentTelemetryBatch{}, err
	}
	if len(encoded) > d.cfg.MaxRequestBytes {
		return coreclient.AgentTelemetryBatch{}, fmt.Errorf("serialized agent-telemetry batch exceeds MaxRequestBytes")
	}
	return out, nil
}

func defaultSyncDir(dirPath string) error {
	dir, err := os.Open(dirPath)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

// persistIdentityConflictMarker keeps the security hold visible across Sensor
// restarts and later successful deliveries. The marker is never cleared by a
// telemetry ACK; an authorized reconciliation workflow must clear it.
func (d *Drain) persistIdentityConflictMarker() error {
	path := filepath.Join(d.poisonDir, identityConflictMarker)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil && !os.IsExist(err) {
		return err
	}
	if err == nil {
		if syncErr := f.Sync(); syncErr != nil {
			_ = f.Close()
			return syncErr
		}
		if closeErr := f.Close(); closeErr != nil {
			return closeErr
		}
	} else {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return statErr
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("identity quarantine marker is not a regular file")
		}
	}
	if err := d.syncDir(d.poisonDir); err != nil {
		return err
	}
	d.deliveryMu.Lock()
	d.identityQuarantined = true
	d.deliveryMu.Unlock()
	return nil
}

func (d *Drain) parkHead(batch spool.Batch, reason string, cause error) error {
	if len(batch.Records) == 0 {
		return nil
	}
	head := batch.Records[0]
	name := fmt.Sprintf("%020d-%s.json", head.Sequence, sanitizeReason(reason))
	path := filepath.Join(d.poisonDir, name)
	meta := map[string]any{
		"reason":       reason,
		"sequence":     head.Sequence,
		"type":         head.Type,
		"parkedAt":     time.Now().UTC().Format(time.RFC3339Nano),
		"cause":        fmt.Sprintf("%v", cause),
		"payloadBytes": len(head.Payload),
	}
	if reason == identityConflictCode {
		var payload durableBatchPayload
		if err := json.Unmarshal(head.Payload, &payload); err != nil {
			return codeErr(CodePoison, "failed decoding identity-conflict custody metadata", err, false)
		}
		digest := sha256.Sum256(head.Payload)
		meta["cause"] = identityConflictCode // never persist an untrusted Core message
		meta["tenantId"] = payload.TenantID
		meta["agentId"] = payload.AgentID
		meta["batchChecksum"] = payload.BatchChecksum
		meta["correlationId"] = payload.CorrelationID
		meta["payloadSHA256"] = hex.EncodeToString(digest[:])
		meta["replayPolicy"] = "AUTHORITATIVE_RECONCILIATION_REQUIRED"
	}
	metaBytes, _ := json.Marshal(meta)
	tmp, err := os.CreateTemp(d.poisonDir, ".poison-*.tmp")
	if err != nil {
		return codeErr(CodePoison, "failed creating poison temp file", err, true)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	writeFn := d.poisonWrite
	if writeFn == nil {
		writeFn = func(f *os.File, p []byte) (int, error) { return f.Write(p) }
	}
	fileSyncFn := d.poisonSync
	if fileSyncFn == nil {
		fileSyncFn = func(f *os.File) error { return f.Sync() }
	}
	renameFn := d.poisonRename
	if renameFn == nil {
		renameFn = os.Rename
	}

	if _, err := writeFn(tmp, metaBytes); err != nil {
		_ = tmp.Close()
		return codeErr(CodePoison, "failed writing poison metadata", err, true)
	}
	if _, err := writeFn(tmp, []byte("\n")); err != nil {
		_ = tmp.Close()
		return codeErr(CodePoison, "failed writing poison separator", err, true)
	}
	if _, err := writeFn(tmp, head.Payload); err != nil {
		_ = tmp.Close()
		return codeErr(CodePoison, "failed writing poison payload", err, true)
	}
	if err := fileSyncFn(tmp); err != nil {
		_ = tmp.Close()
		return codeErr(CodePoison, "failed syncing poison file", err, true)
	}
	if err := tmp.Close(); err != nil {
		return codeErr(CodePoison, "failed closing poison file", err, true)
	}
	if err := renameFn(tmpName, path); err != nil {
		return codeErr(CodePoison, "failed publishing poison file", err, true)
	}
	cleanup = false
	syncFn := d.syncDir
	if syncFn == nil {
		syncFn = defaultSyncDir
	}
	if err := syncFn(d.poisonDir); err != nil {
		d.metrics.Backpressure.Add(1)
		return codeErr(CodePoison, "failed syncing poison directory", err, true)
	}
	if reason == identityConflictCode {
		if err := d.persistIdentityConflictMarker(); err != nil {
			return codeErr(CodePoison, "failed publishing identity-conflict health marker", err, true)
		}
	}
	if err := d.spool.AcknowledgeThrough(head.Sequence); err != nil {
		// DiskSpool may return after durably advancing AckedThrough when later
		// record cleanup or final directory fsync fails. Reconcile delivery
		// health from authoritative spool state; never invent a Core ACK.
		authorityPoison := isAuthorityPoisonCode(reason)
		if d.reconcileAfterPartialSpoolCommit(head.Sequence, false, authorityPoison) {
			d.metrics.PoisonCount.Add(1)
			if authorityPoison {
				d.metrics.AuthorityPoisonCount.Add(1)
			}
			d.logger.Warn("Parked terminal agent-ingress record; spool watermark durable but cleanup failed",
				"event_code", string(reason),
				"safe_context", map[string]any{"sequence": head.Sequence, "partial_spool_ack": true})
			return codeErr(CodeSpool, "parked record watermark durable but spool cleanup failed", err, false)
		}
		return codeErr(CodeSpool, "failed removing parked record from spool", err, false)
	}
	d.metrics.PoisonCount.Add(1)
	if isAuthorityPoisonCode(reason) {
		d.metrics.AuthorityPoisonCount.Add(1)
	}
	d.logger.Warn("Parked terminal agent-ingress record",
		"event_code", string(reason),
		"safe_context", map[string]any{"sequence": head.Sequence})
	return nil
}

// reconcileDeliveryAfterPartialParkAck is the generic poison-park entry point for
// post-commit AcknowledgeThrough reconciliation (no Core ACK; no authority class).
func (d *Drain) reconcileDeliveryAfterPartialParkAck(parkedSeq uint64) bool {
	return d.reconcileAfterPartialSpoolCommit(parkedSeq, false, false)
}

// reconcileAfterPartialSpoolCommit clears obsolete delivery-block health only
// when durable spool state proves AcknowledgeThrough/CompleteOutboundBatch
// committed seq even though a post-commit cleanup/sidecar/fsync error was
// returned. Pre-commit failures retain the prior block. When every watermark
// inspector fails, fail closed but retain one bounded reconciliation intent
// (including authority-poison classification) so a later successful inspection
// can recover — including with zero pending records — without retaining
// obsolete 413 forever and without allowing a second delivery to overwrite the
// intent. After clearing the obsolete Core delivery reason, an unresolved
// storage-cleanup fault remains visible as DEGRADED (ERR_SPOOL_CLEANUP_FAILED)
// until orphan/sidecar cleanup + directory sync succeed. Callers that own a
// real Core ACK must count DurableAcks themselves; this helper never fabricates
// one. authorityPoison is meaningful only for poison-park paths.
func (d *Drain) reconcileAfterPartialSpoolCommit(seq uint64, coreACK bool, authorityPoison bool) bool {
	acked, proven := d.provenAckedThrough()
	if !proven {
		d.markPendingPartialAckReconcile(seq, coreACK, authorityPoison)
		return false
	}
	if acked < seq {
		// Authoritative non-commit proof: clear matching intent without counting.
		d.clearPendingPartialAckReconcileMatching(seq)
		return false
	}
	// Logical pending excludes sequences at or below AckedThrough even if an
	// orphaned record file or stale outbound-window sidecar remains.
	d.installProvenCleanupAfterPartialAck(seq)
	d.refreshStats()
	return true
}

func (d *Drain) markPendingPartialAckReconcile(seq uint64, coreACK bool, authorityPoison bool) {
	if d == nil || seq == 0 {
		return
	}
	d.deliveryMu.Lock()
	defer d.deliveryMu.Unlock()
	if d.pendingPartialAckSeq != 0 {
		// Never overwrite a retained bounded intent (including classification).
		return
	}
	// Fresh intent: allow an immediate first recovery attempt.
	d.cleanupRetryAfter = time.Time{}
	d.cleanupRetryBackoff = 0
	d.pendingPartialAckSeq = seq
	d.pendingPartialAckCoreACK = coreACK
	d.pendingPartialAckAuthorityPoison = authorityPoison
}

func (d *Drain) clearPendingPartialAckReconcile() {
	if d == nil {
		return
	}
	d.deliveryMu.Lock()
	defer d.deliveryMu.Unlock()
	d.pendingPartialAckSeq = 0
	d.pendingPartialAckCoreACK = false
	d.pendingPartialAckAuthorityPoison = false
}

func (d *Drain) clearPendingPartialAckReconcileMatching(seq uint64) {
	if d == nil || seq == 0 {
		return
	}
	d.deliveryMu.Lock()
	defer d.deliveryMu.Unlock()
	if d.pendingPartialAckSeq != seq {
		return
	}
	d.pendingPartialAckSeq = 0
	d.pendingPartialAckCoreACK = false
	d.pendingPartialAckAuthorityPoison = false
}

func (d *Drain) hasUnresolvedPendingPartialAck() bool {
	if d == nil {
		return false
	}
	d.deliveryMu.RLock()
	defer d.deliveryMu.RUnlock()
	return d.pendingPartialAckSeq != 0
}

// tryReconcilePendingPartialAck applies deferred health reconciliation after a
// prior partial acknowledgement where watermark inspectors all failed. Paced
// like cleanup recovery so IdleInterval does not re-run expensive Stats scans.
func (d *Drain) tryReconcilePendingPartialAck() {
	if d == nil {
		return
	}
	d.deliveryMu.RLock()
	seq := d.pendingPartialAckSeq
	retryAfter := d.cleanupRetryAfter
	d.deliveryMu.RUnlock()
	if seq == 0 {
		return
	}
	if !retryAfter.IsZero() && time.Now().Before(retryAfter) {
		return
	}
	acked, proven := d.provenAckedThrough()
	if !proven {
		d.scheduleCleanupRetry()
		return
	}
	if acked < seq {
		// Authoritative proof that the attempted commit never landed.
		// Preserve applicable delivery blocks; permit retry of the same work.
		d.clearPendingPartialAckReconcile()
		return
	}
	coreACK, authorityPoison, consumed := d.consumePendingPartialAckInstallCleanup(seq)
	if !consumed {
		return
	}
	if coreACK {
		// Count the original Core ACK once after durable proof; not a second ACK.
		d.metrics.Successes.Add(1)
		d.metrics.DurableAcks.Add(1)
		d.metrics.LastSuccessUnix.Store(time.Now().Unix())
	} else {
		d.metrics.PoisonCount.Add(1)
		if authorityPoison {
			d.metrics.AuthorityPoisonCount.Add(1)
		}
	}
	d.refreshStats()
}

// provenAckedThrough returns the durable AckedThrough watermark using O(1)
// reporters when available. Stats() is only a compatibility fallback for test
// spools that lack those reporters. When O(1) reporters exist but fail, do not
// fall back to an expensive Stats directory scan — callers pace retry instead.
// A transient Stats() failure must not be treated as disproof when AckedThrough
// or UnresolvedAckedCleanup succeeds.
func (d *Drain) provenAckedThrough() (uint64, bool) {
	if d == nil || d.spool == nil {
		return 0, false
	}
	hasO1 := false
	if reporter, ok := d.spool.(ackedThroughReporter); ok {
		hasO1 = true
		if acked, err := reporter.AckedThrough(); err == nil {
			return acked, true
		}
		// Fall through: AckedThrough inspect can fail independently of the
		// durable watermark exposed by UnresolvedAckedCleanup.
	}
	if reporter, ok := d.spool.(unresolvedCleanupReporter); ok {
		hasO1 = true
		if _, acked, err := reporter.UnresolvedAckedCleanup(); err == nil {
			return acked, true
		}
	}
	if hasO1 {
		return 0, false
	}
	st, err := d.spool.Stats()
	if err != nil {
		return 0, false
	}
	return st.AckedThrough, true
}

// observeOrphanedCleanupFault reasserts cleanup degradation from the spool's
// O(1) unresolved-cleanup flag (set when AcknowledgeThrough advances the
// watermark but removal/fsync fails). Snapshot must not scan the spool directory.
// Spool observation stays outside deliveryMu (never hold deliveryMu across spool
// I/O). A sampled cleanupGen must still match under the write lock or the
// observation is discarded as stale vs a concurrent clearCleanupFault.
func (d *Drain) observeOrphanedCleanupFault() {
	if d == nil {
		return
	}
	d.deliveryMu.RLock()
	gen := d.cleanupGen
	already := d.cleanupFault
	d.deliveryMu.RUnlock()
	if already {
		return
	}
	reporter, ok := d.spool.(unresolvedCleanupReporter)
	if !ok {
		return
	}
	unresolved, seq, err := reporter.UnresolvedAckedCleanup()
	if err != nil || !unresolved {
		return
	}
	if d.afterCleanupUnresolvedObserve != nil {
		d.afterCleanupUnresolvedObserve()
	}
	d.markObservedCleanupFault(gen, seq)
}

// tryRecoverCleanupFault clears ERR_SPOOL_CLEANUP_FAILED only after durable
// Stats prove the fault sequence is covered and DiskSpool-owned reconcile plus
// spool directory fsync succeed. Unrelated upload success does not clear it.
// Under persistent failure, recovery is paced via InitialBackoff/MaxBackoff so
// RunOnce does not rescan the spool directory every IdleInterval; skipped
// attempts leave the fault set and do not block pending telemetry delivery.
func (d *Drain) tryRecoverCleanupFault() {
	if d == nil {
		return
	}
	d.deliveryMu.RLock()
	faulted := d.cleanupFault
	faultSeq := d.cleanupFaultSeq
	retryAfter := d.cleanupRetryAfter
	d.deliveryMu.RUnlock()
	if !faulted {
		return
	}
	if !retryAfter.IsZero() && time.Now().Before(retryAfter) {
		// Paced skip: fault remains DEGRADED; delivery path continues.
		return
	}
	acked, proven := d.provenAckedThrough()
	if !proven {
		// Watermark inspection/Stats failed: keep DEGRADED and pace retries so
		// IdleInterval does not repeat expensive directory Stats every loop.
		d.scheduleCleanupRetry()
		return
	}
	if acked < faultSeq {
		return
	}
	reconciler, ok := d.spool.(ackedCleanupReconciler)
	if !ok {
		return
	}
	if err := reconciler.ReconcileAckedCleanup(); err != nil {
		d.scheduleCleanupRetry()
		return
	}
	syncFn := d.syncDir
	if syncFn == nil {
		syncFn = defaultSyncDir
	}
	if err := syncFn(d.cfg.AgentSpoolDir); err != nil {
		d.scheduleCleanupRetry()
		return
	}
	d.clearCleanupFault()
	d.refreshStats()
}

func (d *Drain) refreshStats() {
	st, err := d.spool.Stats()
	if err != nil {
		return
	}
	d.metrics.PendingRecords.Store(int64(st.RecordCount))
	d.metrics.PendingBytes.Store(st.SpoolBytes)
	d.metrics.OldestAgeSeconds.Store(int64(st.OldestRecordAge.Seconds()))
}

func (d *Drain) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return codeErr(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	d.mu.Lock()
	started := d.started
	cancel := d.cancel
	d.mu.Unlock()
	if !started {
		return nil
	}
	d.stopOnce.Do(func() {
		if cancel != nil {
			cancel()
		}
	})
	select {
	case <-d.done:
		return nil
	case <-ctx.Done():
		return ctxErr(ctx)
	}
}

func isRecordAttributableCoreError(err error) bool {
	// Poison parking requires an explicit record-attributable Core contract plus
	// a singleton Sensor record and Agent delivery. Request JSON, schema, batch
	// checksum and batch/request size failures cannot identify record poison,
	// even for a singleton; they retain the frozen window via retainBlocked.
	// Unknown/application/authority/assignment/state errors also fail closed.
	// HTTP status alone must never authorize AcknowledgeThrough.
	// BUILD-004 Slice 4 retains the four revoked-temporal authority codes.
	return coreclient.IsCoreAPICode(err, "ERR_MALFORMED_RECORD") ||
		coreclient.IsCoreAPICode(err, "ERR_MALFORMED_RECORD_PAYLOAD") ||
		coreclient.IsCoreAPICode(err, "ERR_AGENT_HOST_ID_REQUIRED") ||
		coreclient.IsCoreAPICode(err, "ERR_RECORD_TOO_LARGE") ||
		isAuthorityPoisonError(err)
}

func isIdentityConflictRejection(err error) bool {
	var api *coreclient.CoreAPIError
	return errors.As(err, &api) && api.Code == identityConflictCode &&
		(api.HTTPStatus == 403 || api.HTTPStatus == 409) && !api.Transient()
}

func isAuthorityPoisonError(err error) bool {
	return coreclient.IsCoreAPICode(err, "ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_REQUIRED") ||
		coreclient.IsCoreAPICode(err, "ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_INVALID") ||
		coreclient.IsCoreAPICode(err, "ERR_REVOKED_ASSIGNMENT_ACCEPTED_BEFORE_ACTIVATION") ||
		coreclient.IsCoreAPICode(err, "ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION")
}

func isAuthorityPoisonCode(code string) bool {
	switch code {
	case "ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_REQUIRED",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_INVALID",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTED_BEFORE_ACTIVATION",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION":
		return true
	default:
		return false
	}
}

func terminalCode(err error) string {
	var api *coreclient.CoreAPIError
	if errors.As(err, &api) && api.Code != "" {
		return api.Code
	}
	return "ERR_TERMINAL"
}

func isTransient(err error) bool {
	if coreclient.IsTransient(err) {
		return true
	}
	var t interface{ Transient() bool }
	return errors.As(err, &t) && t.Transient()
}

func sanitizeReason(reason string) string {
	r := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, reason)
	if len(r) > 64 {
		return r[:64]
	}
	return r
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return codeErr(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return codeErr(CodeDeadline, "agent drain deadline exceeded", err, false)
		}
		return codeErr(CodeCancelled, "agent drain cancelled", err, false)
	}
	return nil
}

func waitContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func jitterDuration(d time.Duration, f float64) time.Duration {
	if f <= 0 {
		return d
	}
	delta := float64(d) * f
	v := float64(d) - delta + rand.Float64()*2*delta
	if v < 0 {
		return 0
	}
	return time.Duration(v)
}

func validAgentSpoolStreamID(streamID string) bool {
	const prefix = "stream-"
	if len(streamID) != len(prefix)+32 || !strings.HasPrefix(streamID, prefix) {
		return false
	}
	for i := len(prefix); i < len(streamID); i++ {
		c := streamID[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
