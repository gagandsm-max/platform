package uploader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/spool"
)

type State string

const (
	StateIdle                   State = "IDLE"
	StateUploading              State = "UPLOADING"
	StateBackoff                State = "BACKOFF"
	StateCoreUnavailable        State = "CORE_UNAVAILABLE"
	StateAckContractUnavailable State = "ACK_CONTRACT_UNAVAILABLE"
	StateDegraded               State = "DEGRADED"
	StateStopped                State = "STOPPED"
)

type ErrorCode string

const (
	CodeInvalidConfig          ErrorCode = "UPLOADER_INVALID_CONFIG"
	CodeSpool                  ErrorCode = "UPLOADER_SPOOL"
	CodeTransport              ErrorCode = "UPLOADER_TRANSPORT"
	CodeAckInvalid             ErrorCode = "UPLOADER_ACK_INVALID"
	CodeAckContractUnavailable ErrorCode = "UPLOADER_ACK_CONTRACT_UNAVAILABLE"
	CodeCancelled              ErrorCode = "UPLOADER_CANCELLED"
	CodeDeadline               ErrorCode = "UPLOADER_DEADLINE_EXCEEDED"
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

type Config struct {
	SensorID             string
	StreamID             string
	MaxBatchRecords      int
	MaxBatchPayloadBytes int
	MaxRequestBytes      int
	MaxAttempts          int
	InitialBackoff       time.Duration
	MaxBackoff           time.Duration
	JitterFraction       float64
	IdleInterval         time.Duration
}

type Transport interface {
	Upload(context.Context, coreclient.Batch) (coreclient.UploadAck, error)
}
type Spool interface {
	ReadOrReserveOutboundBatch(int, int) (spool.Batch, error)
	CompleteOutboundBatch(uint64) error
	Stats() (spool.Stats, error)
}

// Optional DiskSpool capabilities used to prove and recover acknowledged
// cleanup without requiring directory Stats on the healthy upload path.
type ackedThroughReporter interface{ AckedThrough() (uint64, error) }
type unresolvedCleanupReporter interface {
	UnresolvedAckedCleanup() (bool, uint64, error)
}
type ackedCleanupReconciler interface{ ReconcileAckedCleanup() error }

type Snapshot struct {
	State                State
	Uploads              uint64
	Retries              uint64
	AcknowledgedThrough  uint64
	LastSuccessfulUpload time.Time
	NextAttempt          time.Time
	BacklogRecords       int
	BacklogBytes         int64
	OldestBacklogAge     time.Duration
	UploadLag            time.Duration
	LastErrorCode        ErrorCode
}

type Uploader struct {
	cfg               Config
	spool             Spool
	transport         Transport
	mu                sync.Mutex
	state             State
	lastSuccess       time.Time
	nextAttempt       time.Time
	lastErr           ErrorCode
	acked             uint64
	cleanupFault      bool
	cleanupRetryAfter time.Time
	cleanupWatermark  uint64
	cleanupProbed     bool
	pendingAckSeq     uint64
	pendingAckCounted bool
	pendingAckProven  bool
	uploads           atomic.Uint64
	retries           atomic.Uint64
	wait              func(context.Context, time.Duration) error
	jitter            func(time.Duration, float64) time.Duration
	cancel            context.CancelFunc
	started           bool
	done              chan struct{}
	startOnce         sync.Once
	stopOnce          sync.Once
}

func New(cfg Config, sp Spool, tr Transport) (*Uploader, error) {
	// !(f>=0 && f<=1) rejects NaN/±Inf; the prior f<0 || f>1 form let NaN through.
	if cfg.SensorID == "" || cfg.StreamID == "" || cfg.MaxBatchRecords <= 0 || cfg.MaxBatchPayloadBytes <= 0 || cfg.MaxRequestBytes <= 0 || cfg.MaxAttempts <= 0 || cfg.InitialBackoff <= 0 || cfg.MaxBackoff < cfg.InitialBackoff || !(cfg.JitterFraction >= 0 && cfg.JitterFraction <= 1) || cfg.IdleInterval <= 0 || sp == nil || tr == nil {
		return nil, codeErr(CodeInvalidConfig, "uploader requires explicit identity, bounded batch/retry timings, spool and transport", nil, false)
	}
	u := &Uploader{cfg: cfg, spool: sp, transport: tr, state: StateIdle, done: make(chan struct{})}
	u.wait = waitContext
	u.jitter = jitterDuration
	return u, nil
}

func (u *Uploader) Start(parent context.Context) error {
	if parent == nil {
		return codeErr(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	u.mu.Lock()
	if u.state == StateStopped {
		u.mu.Unlock()
		return codeErr(CodeInvalidConfig, "uploader is stopped", nil, false)
	}
	u.mu.Unlock()
	startedNow := false
	u.startOnce.Do(func() {
		startedNow = true
		ctx, cancel := context.WithCancel(parent)
		u.mu.Lock()
		u.cancel = cancel
		u.started = true
		u.mu.Unlock()
		go u.loop(ctx)
	})
	if !startedNow {
		return codeErr(CodeInvalidConfig, "uploader already started", nil, false)
	}
	return nil
}
func (u *Uploader) loop(ctx context.Context) {
	defer close(u.done)
	for {
		_, err := u.RunOnce(ctx)
		if ctx.Err() != nil {
			u.setState(StateStopped, "", time.Time{})
			return
		}
		delay := u.outerCycleDelay(err)
		u.mu.Lock()
		u.nextAttempt = time.Now().Add(delay)
		u.mu.Unlock()
		if err := u.wait(ctx, delay); err != nil {
			u.setState(StateStopped, "", time.Time{})
			return
		}
	}
}

// outerCycleDelay selects the wait before the next outer RunOnce.
// Successful or empty cycles use IdleInterval. Any RunOnce error — permanent
// rejection, invalid/missing ACK, spool fault, or transient attempt exhaustion —
// uses MaxBackoff as the deterministic outer failure cadence. Inner transient
// retries remain separately jittered via backoff(); this outer delay is not
// jittered. Policy when IdleInterval > MaxBackoff: failure pacing stays at
// MaxBackoff and is not raised to IdleInterval; IdleInterval is only the
// success/empty poll cadence. Production validated timings use IdleInterval
// (1s) < MaxBackoff (5s); the prior bug applied IdleInterval to permanent
// rejection and resubmitted the frozen batch at the 1s idle cadence.
func (u *Uploader) outerCycleDelay(err error) time.Duration {
	if err != nil {
		return u.cfg.MaxBackoff
	}
	return u.cfg.IdleInterval
}

func (u *Uploader) RunOnce(ctx context.Context) (bool, error) {
	// RunOnce is serialized by the owning uploader loop. Direct callers must
	// preserve the same single-flight invariant because reservation is mutable.
	if err := ctxErr(ctx); err != nil {
		return false, err
	}
	if err := u.resolvePendingAck(ctx); err != nil {
		return false, err
	}
	cleanupErr := u.recoverAckedCleanup(ctx)
	if cleanupErr != nil {
		u.mu.Lock()
		canDrain := u.cleanupFault && u.cleanupWatermark != 0 && u.pendingAckSeq == 0
		u.mu.Unlock()
		if !canDrain {
			return false, cleanupErr
		}
	}
	// Spool reservation mutates the durable outbound window. Recheck cancellation
	// immediately before crossing that boundary.
	if err := ctxErr(ctx); err != nil {
		return false, err
	}
	batch, err := u.spool.ReadOrReserveOutboundBatch(u.cfg.MaxBatchRecords, u.cfg.MaxBatchPayloadBytes)
	if err != nil {
		u.setState(StateDegraded, CodeSpool, time.Time{})
		return false, codeErr(CodeSpool, "failed reading durable spool batch", err, false)
	}
	if len(batch.Records) == 0 {
		if cleanupErr != nil {
			return false, cleanupErr
		}
		u.setState(StateIdle, "", time.Time{})
		return false, nil
	}
	u.mu.Lock()
	cleanupWatermark := u.cleanupWatermark
	cleanupFault := u.cleanupFault
	u.mu.Unlock()
	if cleanupFault && (cleanupWatermark == 0 || batch.FirstSequence <= cleanupWatermark) {
		u.paceCleanupFault()
		return false, codeErr(CodeSpool, "spool reservation is not authoritatively beyond acknowledged cleanup watermark", nil, false)
	}
	request, err := u.makeBatch(batch)
	if err != nil {
		u.setState(StateDegraded, CodeSpool, time.Time{})
		return false, err
	}
	var last error
	for attempt := 1; attempt <= u.cfg.MaxAttempts; attempt++ {
		if err := ctxErr(ctx); err != nil {
			return false, err
		}
		u.setState(StateUploading, "", time.Time{})
		u.uploads.Add(1)
		ack, err := u.transport.Upload(ctx, request)
		if err == nil {
			if err := validateUploaderAck(request, ack); err != nil {
				if isCode(err, CodeAckContractUnavailable) || coreclient.IsCode(err, coreclient.CodeAckContractMissing) {
					u.setState(StateAckContractUnavailable, CodeAckContractUnavailable, time.Time{})
					return false, err
				}
				u.setState(StateDegraded, CodeAckInvalid, time.Time{})
				return false, err
			} else if aerr := u.spool.CompleteOutboundBatch(*ack.AcceptedThroughSequence); aerr != nil {
				u.markPendingAck(*ack.AcceptedThroughSequence)
				_ = u.resolvePendingAck(ctx)
				u.setState(StateDegraded, CodeSpool, time.Now().Add(u.cfg.MaxBackoff))
				return false, codeErr(CodeSpool, "failed applying durable acknowledgement to spool; reconciliation remains pending", aerr, false)
			} else {
				u.recordDelivery(*ack.AcceptedThroughSequence)
				return true, nil
			}
		}
		last = err
		if coreclient.IsCode(err, coreclient.CodeAckContractMissing) || isCode(err, CodeAckContractUnavailable) {
			u.setState(StateAckContractUnavailable, CodeAckContractUnavailable, time.Time{})
			return false, codeErr(CodeAckContractUnavailable, "Core durable acknowledgement contract is unavailable; spool retained", err, false)
		}
		if isCode(err, CodeAckInvalid) {
			u.setState(StateDegraded, CodeAckInvalid, time.Time{})
			return false, err
		}
		if !isTransient(err) {
			u.setState(StateDegraded, CodeTransport, time.Time{})
			return false, codeErr(CodeTransport, "permanent Core upload failure; spool retained", err, false)
		}
		if attempt == u.cfg.MaxAttempts {
			break
		}
		delay := backoff(u.cfg.InitialBackoff, u.cfg.MaxBackoff, attempt, u.jitter, u.cfg.JitterFraction)
		u.retries.Add(1)
		u.setState(StateBackoff, CodeTransport, time.Now().Add(delay))
		if err := u.wait(ctx, delay); err != nil {
			return false, ctxErr(ctx)
		}
	}
	u.setState(StateCoreUnavailable, CodeTransport, time.Now().Add(u.cfg.MaxBackoff))
	return false, codeErr(CodeTransport, "transient Core upload attempts exhausted; spool retained", last, true)
}

func (u *Uploader) recordDelivery(seq uint64) {
	u.mu.Lock()
	if seq > u.acked {
		u.acked = seq
		u.lastSuccess = time.Now().UTC()
	}
	if u.cleanupFault {
		u.state = StateDegraded
		u.lastErr = CodeSpool
		u.nextAttempt = u.cleanupRetryAfter
	} else {
		u.state = StateIdle
		u.lastErr = ""
		u.nextAttempt = time.Time{}
	}
	u.mu.Unlock()
}

func (u *Uploader) markPendingAck(seq uint64) {
	u.mu.Lock()
	if u.pendingAckSeq == 0 {
		u.pendingAckSeq = seq
		u.pendingAckCounted = false
		u.pendingAckProven = false
	}
	u.cleanupFault = true
	u.cleanupRetryAfter = time.Now().Add(u.cfg.MaxBackoff)
	u.cleanupProbed = false
	u.state = StateDegraded
	u.lastErr = CodeSpool
	u.nextAttempt = u.cleanupRetryAfter
	u.mu.Unlock()
}

// resolvePendingAck preserves the attempted completion as an in-memory intent
// until an authoritative exact watermark settles whether it committed. No new
// batch may be reserved while that intent is unresolved.
func (u *Uploader) resolvePendingAck(ctx context.Context) error {
	u.mu.Lock()
	seq, counted := u.pendingAckSeq, u.pendingAckCounted
	paced := u.cleanupFault && u.cleanupProbed && time.Now().Before(u.cleanupRetryAfter)
	u.mu.Unlock()
	if seq == 0 {
		return nil
	}
	if paced {
		return codeErr(CodeSpool, "ambiguous spool acknowledgement remains unresolved", nil, false)
	}
	acked, proven := u.provenAckedThrough()
	if ctxErr(ctx) != nil {
		u.paceCleanupFault()
		return ctxErr(ctx)
	}
	if !proven {
		u.paceCleanupFault()
		return codeErr(CodeSpool, "ambiguous spool acknowledgement remains unresolved", nil, false)
	}
	if acked < seq {
		if counted {
			u.paceCleanupFault()
			return codeErr(CodeSpool, "previously proven acknowledgement now has a contradictory lower watermark", nil, false)
		}
		u.mu.Lock()
		if u.pendingAckSeq == seq {
			u.pendingAckSeq = 0
			u.pendingAckCounted = false
		}
		u.cleanupFault = false
		u.cleanupRetryAfter = time.Time{}
		u.cleanupWatermark = 0
		u.mu.Unlock()
		return nil
	}
	if acked != seq {
		u.paceCleanupFault()
		return codeErr(CodeSpool, "spool acknowledgement watermark does not exactly match pending completion", nil, false)
	}
	if !counted {
		u.recordDelivery(seq)
		u.mu.Lock()
		if u.pendingAckSeq == seq {
			u.pendingAckCounted = true
			u.pendingAckProven = true
		}
		u.cleanupWatermark = seq
		u.mu.Unlock()
	}
	if _, ok := u.spool.(unresolvedCleanupReporter); !ok {
		u.paceCleanupFault()
		return codeErr(CodeSpool, "exact acknowledgement is proven but cleanup recovery cannot be established", nil, false)
	}
	if err := u.recoverAckedCleanup(ctx); err != nil {
		return err
	}
	u.mu.Lock()
	if u.pendingAckSeq == seq {
		u.pendingAckSeq = 0
		u.pendingAckCounted = false
		u.pendingAckProven = false
	}
	u.mu.Unlock()
	return nil
}

// Only O(1) spool evidence can settle a failed completion. Stats scans are
// deliberately excluded here because they cannot establish custody of the
// exact attempted completion.
func (u *Uploader) provenAckedThrough() (uint64, bool) {
	hasO1 := false
	if r, ok := u.spool.(ackedThroughReporter); ok {
		hasO1 = true
		if acked, err := r.AckedThrough(); err == nil {
			return acked, true
		}
	}
	if r, ok := u.spool.(unresolvedCleanupReporter); ok {
		hasO1 = true
		if _, acked, err := r.UnresolvedAckedCleanup(); err == nil {
			return acked, true
		}
	}
	if hasO1 {
		return 0, false
	}
	return 0, false
}

// recoverAckedCleanup inspects the O(1) latch on every capable spool. It calls
// the directory-scanning reconciler only when the spool reports real unresolved
// ACK cleanup, and paces failures even when RunOnce is called directly.
func (u *Uploader) recoverAckedCleanup(ctx context.Context) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	u.mu.Lock()
	if u.cleanupFault && u.cleanupProbed && time.Now().Before(u.cleanupRetryAfter) {
		u.mu.Unlock()
		return codeErr(CodeSpool, "durable acknowledged spool cleanup remains unresolved", nil, false)
	}
	u.mu.Unlock()
	r, ok := u.spool.(unresolvedCleanupReporter)
	if !ok {
		u.mu.Lock()
		fault := u.cleanupFault
		u.mu.Unlock()
		if fault {
			u.paceCleanupFault()
			return codeErr(CodeSpool, "durable acknowledged cleanup cannot be inspected", nil, false)
		}
		return nil
	}
	unresolved, watermark, err := r.UnresolvedAckedCleanup()
	if ctxErr(ctx) != nil {
		u.markCleanupFailure()
		return ctxErr(ctx)
	}
	if err != nil {
		u.markCleanupFailure()
		return codeErr(CodeSpool, "failed inspecting durable acknowledged cleanup", err, false)
	}
	u.mu.Lock()
	fault := u.cleanupFault
	if unresolved {
		u.cleanupWatermark = watermark
	}
	if u.cleanupProbed && time.Now().Before(u.cleanupRetryAfter) {
		u.mu.Unlock()
		return codeErr(CodeSpool, "durable acknowledged spool cleanup remains unresolved", nil, false)
	}
	u.mu.Unlock()
	if !unresolved && !fault {
		return nil
	}
	if err := ctxErr(ctx); err != nil {
		return err
	}
	reconciler, ok := u.spool.(ackedCleanupReconciler)
	if !unresolved {
		watermark = u.cleanupWatermark
	}
	if !ok || watermark == 0 {
		u.markCleanupFailure()
		return codeErr(CodeSpool, "durable acknowledged spool cleanup cannot be reconciled", nil, false)
	}
	if err := reconciler.ReconcileAckedCleanup(); err != nil {
		u.markCleanupFailure()
		return codeErr(CodeSpool, "failed reconciling durable acknowledged spool cleanup", err, false)
	}
	if ctxErr(ctx) != nil {
		u.markCleanupFailure()
		return ctxErr(ctx)
	}
	u.mu.Lock()
	u.cleanupFault = false
	u.cleanupRetryAfter = time.Time{}
	u.cleanupWatermark = 0
	u.cleanupProbed = false
	u.mu.Unlock()
	return nil
}

func (u *Uploader) paceCleanupFault() {
	u.mu.Lock()
	if !u.cleanupFault {
		u.cleanupFault = true
		u.cleanupRetryAfter = time.Now().Add(u.cfg.MaxBackoff)
	}
	u.cleanupProbed = true
	u.state = StateDegraded
	u.lastErr = CodeSpool
	u.nextAttempt = u.cleanupRetryAfter
	u.mu.Unlock()
}

func (u *Uploader) markCleanupFailure() {
	u.mu.Lock()
	u.cleanupFault = true
	u.cleanupRetryAfter = time.Now().Add(u.cfg.MaxBackoff)
	u.cleanupProbed = true
	u.state = StateDegraded
	u.lastErr = CodeSpool
	u.nextAttempt = u.cleanupRetryAfter
	u.mu.Unlock()
}

func (u *Uploader) makeBatch(b spool.Batch) (coreclient.Batch, error) {
	if b.FirstSequence == 0 || b.LastSequence < b.FirstSequence {
		return coreclient.Batch{}, codeErr(CodeInvalidConfig, "spool batch sequence bounds are invalid", nil, false)
	}
	if b.FirstSequence > spool.MaxSafeSequence || b.LastSequence > spool.MaxSafeSequence {
		return coreclient.Batch{}, codeErr(CodeInvalidConfig, "spool batch sequence exceeds MAX_SAFE_SEQUENCE", nil, false)
	}
	records := make([]json.RawMessage, 0, len(b.Records))
	h := sha256.New()
	for _, r := range b.Records {
		if r.Sequence > spool.MaxSafeSequence {
			return coreclient.Batch{}, codeErr(CodeInvalidConfig, "spool record sequence exceeds MAX_SAFE_SEQUENCE", nil, false)
		}
		enc, err := json.Marshal(r)
		if err != nil {
			return coreclient.Batch{}, codeErr(CodeSpool, "failed encoding spool record for upload", err, false)
		}
		records = append(records, enc)
		_, _ = h.Write(enc)
	}
	checksum := hex.EncodeToString(h.Sum(nil))
	batchID := fmt.Sprintf("%s-%s-%020d-%020d-%s", u.cfg.SensorID, u.cfg.StreamID, b.FirstSequence, b.LastSequence, checksum[:16])
	out := coreclient.Batch{SchemaVersion: 1, SensorID: u.cfg.SensorID, BootEpoch: u.cfg.StreamID, BatchID: batchID, FirstSequence: b.FirstSequence, LastSequence: b.LastSequence, Checksum: checksum, Records: records, CorrelationID: "upload-" + batchID}
	encoded, err := json.Marshal(out)
	if err != nil {
		return coreclient.Batch{}, codeErr(CodeSpool, "failed encoding upload batch", err, false)
	}
	if len(encoded) > u.cfg.MaxRequestBytes {
		return coreclient.Batch{}, codeErr(CodeInvalidConfig, "serialized upload batch exceeds configured request byte bound", nil, false)
	}
	return out, nil
}

// validateUploaderAck re-checks the frozen telemetry ACK contract before spool deletion.
func validateUploaderAck(batch coreclient.Batch, ack coreclient.UploadAck) error {
	if ack.Status != "ACCEPTED" {
		return codeErr(CodeAckInvalid, "Core telemetry acknowledgement status is not ACCEPTED", nil, false)
	}
	if strings.TrimSpace(ack.SensorIdentity) == "" {
		return codeErr(CodeAckContractUnavailable, "Core did not provide authoritative sensor_identity", nil, false)
	}
	if ack.SensorIdentity != batch.SensorID {
		return codeErr(CodeAckInvalid, "Core telemetry acknowledgement sensor_identity does not match submitted Sensor", nil, false)
	}
	if ack.AcceptedThroughSequence == nil {
		return codeErr(CodeAckContractUnavailable, "Core did not provide authoritative durable acknowledgement watermark", nil, false)
	}
	if *ack.AcceptedThroughSequence != batch.LastSequence {
		return codeErr(CodeAckInvalid, "Core acknowledgement is not the submitted batch terminal sequence", nil, false)
	}
	return nil
}

func (u *Uploader) Snapshot() (Snapshot, error) {
	st, err := u.spool.Stats()
	if err != nil {
		return Snapshot{}, codeErr(CodeSpool, "failed reading spool statistics", err, false)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return Snapshot{State: u.state, Uploads: u.uploads.Load(), Retries: u.retries.Load(), AcknowledgedThrough: u.acked, LastSuccessfulUpload: u.lastSuccess, NextAttempt: u.nextAttempt, BacklogRecords: st.RecordCount, BacklogBytes: st.SpoolBytes, OldestBacklogAge: st.OldestRecordAge, UploadLag: st.OldestRecordAge, LastErrorCode: u.lastErr}, nil
}
func (u *Uploader) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return codeErr(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	u.mu.Lock()
	started := u.started
	cancel := u.cancel
	if !started {
		u.state = StateStopped
	}
	u.mu.Unlock()
	if !started {
		return nil
	}
	u.stopOnce.Do(func() {
		if cancel != nil {
			cancel()
		}
	})
	select {
	case <-u.done:
		return nil
	case <-ctx.Done():
		return ctxErr(ctx)
	}
}
func (u *Uploader) setState(s State, code ErrorCode, next time.Time) {
	u.mu.Lock()
	u.state = s
	u.lastErr = code
	u.nextAttempt = next
	u.mu.Unlock()
}
func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return codeErr(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return codeErr(CodeDeadline, "uploader deadline exceeded", err, false)
		}
		return codeErr(CodeCancelled, "uploader cancelled", err, false)
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
func backoff(initial, max time.Duration, attempt int, j func(time.Duration, float64) time.Duration, f float64) time.Duration {
	d := initial
	for i := 1; i < attempt; i++ {
		if d >= max/2 {
			d = max
			break
		}
		d *= 2
	}
	if d > max {
		d = max
	}
	// Clamp the post-jitter delay: jitter must not escape the selected max or
	// collapse to a non-positive wait (edge configs / adversarial injectors).
	return clampBackoffDelay(j(d, f), max)
}

// clampBackoffDelay enforces a positive delay that never exceeds max after jitter.
func clampBackoffDelay(d, max time.Duration) time.Duration {
	if max <= 0 {
		return time.Nanosecond
	}
	if d > max {
		return max
	}
	if d <= 0 {
		return time.Nanosecond
	}
	return d
}

func jitterDuration(d time.Duration, f float64) time.Duration {
	if f <= 0 {
		return d
	}
	delta := float64(d) * f
	v := float64(d) - delta + rand.Float64()*2*delta
	// Saturate BEFORE float64→Duration: float64(MaxInt64) is 2^63, so MaxInt64-scale
	// (and NaN/Inf) intermediates overflow the cast into a non-positive wait that the
	// positive max clamp would otherwise floor to 1ns (tight retry).
	if math.IsNaN(v) || math.IsInf(v, 0) || v >= float64(math.MaxInt64) {
		return time.Duration(math.MaxInt64)
	}
	if v < 0 {
		return 0
	}
	return time.Duration(v)
}
func isCode(err error, c ErrorCode) bool { var e *Error; return errors.As(err, &e) && e.Code == c }

func isTransient(err error) bool {
	if coreclient.IsTransient(err) {
		return true
	}
	var t interface{ Transient() bool }
	return errors.As(err, &t) && t.Transient()
}
