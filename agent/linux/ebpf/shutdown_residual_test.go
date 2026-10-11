package ebpf

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"platform/agent/common/collector"

	"github.com/cilium/ebpf/ringbuf"
)

// errRingPollerUnexpected is a typed unexpected ring fault for errors.Is proofs.
var errRingPollerUnexpected = errors.New("ring poller unexpected fault")

// errReaderCloseCause is a typed Reader.Close failure for errors.Is proofs.
var errReaderCloseCause = errors.New("reader close refused")

// TestUnexpectedRingReadDuringCanceledRuntimeNotSilent is the P1 residual:
// handleRingReadError previously treated ctx.Err()!=nil as a quiet exit, so an
// unexpected drain ReadRaw fault during Stop could discard unread-ring evidence
// and allow a later nil Stop. Drain classification must retain typed incomplete.
func TestUnexpectedRingReadDuringCanceledRuntimeNotSilent(t *testing.T) {
	b := testBackendInactive(t)
	rd := &fakeEventReader{failWith: errRingPollerUnexpected}
	runtimeCtx, runtimeCancel := context.WithCancel(context.Background())
	runtimeCancel() // already canceled — the Stop-overlap condition

	b.mu.Lock()
	b.producersQuiesced = true
	b.shutdownCtx = context.Background()
	b.mu.Unlock()

	b.drainKernelRing(runtimeCtx, rd)

	b.mu.Lock()
	flushErr := b.shutdownFlushErr
	b.mu.Unlock()
	if flushErr == nil {
		t.Fatal("unexpected ring read under canceled runtime must not be silent")
	}
	if !errors.Is(flushErr, ErrBackendRingDrainIncomplete) {
		t.Fatalf("want ErrBackendRingDrainIncomplete, got %v", flushErr)
	}
	if !errors.Is(flushErr, errRingPollerUnexpected) {
		t.Fatalf("underlying typed cause must remain visible, got %v", flushErr)
	}
}

// TestUnreadBufferedRingRecordsIncompleteWhenDrainCannotFinish proves pending
// ring records yield incomplete-drain evidence when the reader closes early.
func TestUnreadBufferedRingRecordsIncompleteWhenDrainCannotFinish(t *testing.T) {
	b := testBackendInactive(t)
	rd := &fakeEventReader{
		records: [][]byte{putEventV1(t, nil), putEventV1(t, nil)},
	}
	// Close before drain — unread records remain, ErrClosed on first ReadRaw.
	if err := rd.Close(); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	b.producersQuiesced = true
	b.shutdownCtx = context.Background()
	b.rawCh = make(chan []byte, 4)
	b.mu.Unlock()

	b.drainKernelRing(context.Background(), rd)

	b.mu.Lock()
	flushErr := b.shutdownFlushErr
	b.mu.Unlock()
	if !errors.Is(flushErr, ErrBackendRingDrainIncomplete) {
		t.Fatalf("closed reader with unread ring must be incomplete, got %v", flushErr)
	}
	if !rd.closedWithPending.Load() {
		t.Fatal("fixture must retain unread-pending evidence")
	}
}

// gatedOverlapReader blocks in ReadRaw until release, ignoring deadlines so the
// unexpected fault is delivered from the in-flight ordinary read after Stop
// latches — without racing idle-deadline continue → top-of-loop shutdown.
type gatedOverlapReader struct {
	mu       sync.Mutex
	release  chan struct{}
	entered  chan struct{}
	enterOnce sync.Once
	err      error
	closed   bool
	closeErr error
}

func (g *gatedOverlapReader) SetDeadline(time.Time) {}

func (g *gatedOverlapReader) ReadRaw() ([]byte, error) {
	g.enterOnce.Do(func() { close(g.entered) })
	<-g.release
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, ringbuf.ErrClosed
	}
	if g.err != nil {
		return nil, g.err
	}
	return nil, os.ErrDeadlineExceeded
}

func (g *gatedOverlapReader) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = true
	select {
	case <-g.release:
	default:
		close(g.release)
	}
	return g.closeErr
}

// TestUnexpectedOrdinaryReadRawOverlappingStopGoverned proves an unexpected
// ReadRaw fault overlapping Stop enters governed drain / incomplete evidence
// rather than exiting solely because the runtime context is canceled.
func TestUnexpectedOrdinaryReadRawOverlappingStopGoverned(t *testing.T) {
	agg, _ := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	rd := &gatedOverlapReader{
		release: make(chan struct{}),
		entered: make(chan struct{}),
		err:     errRingPollerUnexpected,
	}
	b.detachProbesFn = func(*kernelHandles) error { return nil }
	_, closed := trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, 4)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	select {
	case <-rd.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop never entered ReadRaw")
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- b.StopContext(context.Background()) }()
	deadline := time.Now().Add(2 * time.Second)
	for !b.isStopRequested() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !b.isStopRequested() {
		t.Fatal("Stop did not latch stopRequested")
	}
	close(rd.release) // deliver unexpected error from in-flight ordinary ReadRaw

	select {
	case err = <-stopDone:
	case <-time.After(3 * time.Second):
		t.Fatal("StopContext did not complete")
	}
	if err == nil {
		t.Fatal("unexpected ReadRaw overlapping Stop must not yield clean success")
	}
	if !errors.Is(err, ErrBackendRingDrainIncomplete) {
		t.Fatalf("want ErrBackendRingDrainIncomplete, got %v", err)
	}
	if !errors.Is(err, errRingPollerUnexpected) {
		t.Fatalf("underlying cause must remain visible, got %v", err)
	}
	select {
	case <-b.workersDone:
	case <-time.After(2 * time.Second):
		t.Fatal("workers did not finish")
	}
	if closed.Load() != 1 {
		t.Fatalf("handles closed=%d", closed.Load())
	}
	for _, forbidden := range []string{"DOWN", "HOST_DOWN", "DEVICE_DOWN"} {
		if string(b.Capability().State) == forbidden {
			t.Fatalf("must not assert %s", forbidden)
		}
	}
}

// TestExpectedIdleDeadlineAfterQuiescenceAllowsCleanShutdown is the positive
// control: confirmed quiescence + idle deadline remains a clean Stop.
func TestExpectedIdleDeadlineAfterQuiescenceAllowsCleanShutdown(t *testing.T) {
	agg, _ := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	rd := &fakeEventReader{} // empty → idle deadline after quiescence
	b.detachProbesFn = func(*kernelHandles) error { return nil }
	_, closed := trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, 2)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	if err := b.StopContext(context.Background()); err != nil {
		t.Fatalf("clean quiesced idle drain must succeed, got %v", err)
	}
	if closed.Load() != 1 {
		t.Fatalf("handles closed=%d", closed.Load())
	}
}

// TestHandleRingReadErrorCanceledRuntimeDoesNotEraseUnexpected proves the
// previous defective helper behavior is gone: ctx.Err()!=nil alone must not
// quiet-exit an unexpected fault.
func TestHandleRingReadErrorCanceledRuntimeDoesNotEraseUnexpected(t *testing.T) {
	b := testBackendInactive(t)
	ctx, cancel := armRuntime(b)
	cancel() // canceled runtime without Stop classification
	if ctx.Err() == nil {
		t.Fatal("precondition: ctx canceled")
	}
	if !b.handleRingReadError(ctx, errRingPollerUnexpected) {
		t.Fatal("unexpected fault must still terminate")
	}
	if b.ReaderFaultCount() == 0 {
		t.Fatal("canceled runtime must not erase unexpected READER_FAILED accounting")
	}
	if !hasReason(b.Capability(), collector.ReasonReaderFailed) {
		t.Fatalf("want READER_FAILED, got %+v", b.Capability())
	}
}

// TestReaderCloseErrorTypedAndVisible ensures Close failures are not ignored.
func TestReaderCloseErrorTypedAndVisible(t *testing.T) {
	agg, _ := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	rd := &fakeEventReader{closeErr: errReaderCloseCause}
	b.detachProbesFn = func(*kernelHandles) error { return nil }
	_, closed := trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, 2)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	err = b.StopContext(context.Background())
	if err == nil {
		t.Fatal("Reader.Close failure must not yield successful Stop")
	}
	if !errors.Is(err, ErrBackendRingDrainIncomplete) {
		t.Fatalf("want ErrBackendRingDrainIncomplete wrapper, got %v", err)
	}
	if !errors.Is(err, errReaderCloseCause) {
		t.Fatalf("close cause must remain errors.Is visible, got %v", err)
	}
	select {
	case <-b.workersDone:
	case <-time.After(2 * time.Second):
		t.Fatal("workers stuck")
	}
	if closed.Load() != 1 {
		t.Fatalf("handles closed=%d", closed.Load())
	}
}

// TestDetachErrorPreservesErrorsIs is a focused unit of the %w wrap fix.
func TestDetachErrorPreservesErrorsIs(t *testing.T) {
	b := testBackendInactive(t)
	_, _ = trackHandleCleanup(b)
	b.detachProbesFn = func(*kernelHandles) error { return errDetachCause }
	err := b.quiesceKernelProducers()
	if !errors.Is(err, errDetachCause) {
		t.Fatalf("quiesce return must preserve cause, got %v", err)
	}
	b.mu.Lock()
	flushErr := b.shutdownFlushErr
	b.mu.Unlock()
	if !errors.Is(flushErr, ErrBackendProducerQuiesceIncomplete) || !errors.Is(flushErr, errDetachCause) {
		t.Fatalf("noted evidence must preserve errors.Is identities, got %v", flushErr)
	}
}

// TestFirstStopDeadlineRetainedAfterAsyncCleanup is the P2 repeat-Stop freeze.
func TestFirstStopDeadlineRetainedAfterAsyncCleanup(t *testing.T) {
	agg, _ := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	rd := &fakeEventReader{}
	hold := make(chan struct{})
	entered := make(chan struct{})
	var once sync.Once
	b.detachProbesFn = func(*kernelHandles) error {
		once.Do(func() { close(entered) })
		<-hold
		return nil
	}
	_, closed := trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, 2)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	firstErr := b.StopContext(stopCtx)
	stopCancel()
	if !errors.Is(firstErr, ErrBackendStopDeadline) {
		t.Fatalf("first Stop want deadline, got %v", firstErr)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("detach barrier not reached")
	}
	close(hold)
	select {
	case <-b.workersDone:
	case <-time.After(2 * time.Second):
		t.Fatal("async cleanup did not finish")
	}
	if closed.Load() != 1 {
		t.Fatalf("closes=%d", closed.Load())
	}
	second := b.StopContext(context.Background())
	if second == nil || !errors.Is(second, ErrBackendStopDeadline) {
		t.Fatalf("repeated Stop must retain first deadline evidence, got %v", second)
	}
	// Cleanup completion is distinct from rewriting the earlier failure as success.
	if !b.isStopRequested() {
		t.Fatal("stopRequested must remain latched")
	}
}

// TestBackendDropNotDoubleCountedFromLossSnapshot proves DISJOINT accounting:
// sink-represented BACKEND_DROP must not be added again from LossSnapshot.
func TestBackendDropNotDoubleCountedFromLossSnapshot(t *testing.T) {
	agg, sink := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := armRuntime(b)
	defer cancel()
	b.kernelLossSnapshotFn = func() (uint64, uint64, bool) { return 3, 2, true }
	if fatal := b.pollKernelLoss(ctx); fatal {
		t.Fatal("poll must not be fatal")
	}
	// emitDrop ingests into open windows — flush so sink DroppedRecordsCount is
	// authoritative for DISJOINT accounting.
	if _, err := b.aggregator.FlushAll(ctx); err != nil && !errors.Is(err, collector.ErrStructuredTransportNotWired) {
		t.Fatalf("FlushAll: %v", err)
	}
	accepted, dropped, shed := processNetworkRepresentedTotals(sink)
	unrepresented, covErr := backendLossNotInSink(b, dropped)
	if covErr != nil {
		t.Fatalf("backend loss coverage: %v", covErr)
	}
	if unrepresented != 0 {
		t.Fatalf("emitDrop'd BACKEND_DROP must be fully covered by sink dropped; unrepresented=%d dropped=%d loss=%v",
			unrepresented, dropped, b.LossSnapshot())
	}
	backend := backendLossTotal(b)
	if dropped < backend {
		t.Fatalf("sink dropped=%d must cover backend loss=%d", dropped, backend)
	}
	// Naive double-count would be accepted+dropped+shed+backend.
	naive := accepted + dropped + shed + backend
	disjoint := accepted + dropped + shed + unrepresented
	if naive <= disjoint {
		t.Fatalf("precondition: naive double-count (%d) should exceed disjoint (%d)", naive, disjoint)
	}
	if disjoint != dropped+accepted+shed {
		t.Fatalf("disjoint total=%d", disjoint)
	}
}

// TestStalledDetachBarrierBoundedEvenAgainstDefectiveCode uses an explicit
// test-level deadline and forced release so a defective blocking Stop cannot
// hang the suite. The production Stop still must return within its budget.
func TestStalledDetachBarrierBoundedEvenAgainstDefectiveCode(t *testing.T) {
	agg, _ := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	rd := &fakeEventReader{}
	hold := make(chan struct{})
	entered := make(chan struct{})
	var once sync.Once
	b.detachProbesFn = func(*kernelHandles) error {
		once.Do(func() { close(entered) })
		<-hold
		return nil
	}
	_, closed := trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, 2)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	stopDone := make(chan error, 1)
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	go func() { stopDone <- b.StopContext(stopCtx) }()

	// Bound the whole test against a defective Stop that ignored the deadline.
	testDeadline := time.After(500 * time.Millisecond)
	var stopErr error
	select {
	case stopErr = <-stopDone:
		stopCancel()
	case <-testDeadline:
		stopCancel()
		close(hold) // force-release even if Stop is defective
		t.Fatal("StopContext ignored caller deadline — suite barrier fired")
	}
	if !errors.Is(stopErr, ErrBackendStopDeadline) {
		close(hold)
		t.Fatalf("want ErrBackendStopDeadline, got %v", stopErr)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(hold)
		t.Fatal("detach never entered")
	}
	close(hold)
	select {
	case <-b.workersDone:
	case <-time.After(2 * time.Second):
		t.Fatal("workers did not finish after release")
	}
	if closed.Load() != 1 {
		t.Fatalf("closes=%d", closed.Load())
	}
}

// TestCanceledRuntimeUnexpectedReadPreviousDefectDetection documents that the
// old handleRingReadError quiet-exit would have returned stop=true with zero
// incomplete evidence when only ctx was canceled.
func TestCanceledRuntimeUnexpectedReadPreviousDefectDetection(t *testing.T) {
	// Simulate the previous defective classifier locally and prove the new
	// drain path disagrees with it.
	defectiveQuietExit := func(ctx context.Context, err error) bool {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return false
		}
		if errors.Is(err, ringbuf.ErrClosed) || errors.Is(err, os.ErrClosed) || ctx.Err() != nil {
			return true // previous defect: quiet terminate
		}
		return true
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !defectiveQuietExit(ctx, errRingPollerUnexpected) {
		t.Fatal("defective classifier fixture broken")
	}

	b := testBackendInactive(t)
	b.mu.Lock()
	b.producersQuiesced = true
	b.shutdownCtx = context.Background()
	b.mu.Unlock()
	b.drainKernelRing(ctx, &fakeEventReader{failWith: io.ErrUnexpectedEOF})
	b.mu.Lock()
	flushErr := b.shutdownFlushErr
	b.mu.Unlock()
	if flushErr == nil || !errors.Is(flushErr, ErrBackendRingDrainIncomplete) {
		t.Fatalf("new drain must record incomplete where defective classifier was quiet, got %v", flushErr)
	}
	if !errors.Is(flushErr, io.ErrUnexpectedEOF) {
		t.Fatalf("want UnexpectedEOF cause, got %v", flushErr)
	}
}

// TestOperationBudgetExhaustionIncompleteEvidence covers classification #5.
func TestOperationBudgetExhaustionIncompleteEvidence(t *testing.T) {
	b := testBackendInactive(t)
	rd := &fakeEventReader{block: make(chan struct{})} // never yields records
	opCtx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	b.mu.Lock()
	b.producersQuiesced = false // idle deadline must not clean-complete
	b.shutdownCtx = opCtx
	b.mu.Unlock()
	b.drainKernelRing(context.Background(), rd)
	// Unblock fixture so Close during finalize cannot hang other tests.
	rd.mu.Lock()
	if rd.block != nil {
		close(rd.block)
		rd.block = nil
	}
	rd.mu.Unlock()
	b.mu.Lock()
	flushErr := b.shutdownFlushErr
	b.mu.Unlock()
	if !errors.Is(flushErr, ErrBackendRingDrainIncomplete) {
		t.Fatalf("budget exhaustion must be incomplete, got %v", flushErr)
	}
}

// TestNoGoroutineLeakOnResidualShutdownPaths is a coarse leak guard for the
// residual Stop / unexpected-read paths.
func TestNoGoroutineLeakOnResidualShutdownPaths(t *testing.T) {
	var lastRemain int32
	for i := 0; i < 3; i++ {
		agg, _ := testAgg(t)
		b, err := NewBackend(BackendConfig{
			Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
			Aggregator: agg,
		})
		if err != nil {
			t.Fatal(err)
		}
		rd := &fakeEventReader{failWith: errRingPollerUnexpected}
		b.detachProbesFn = func(*kernelHandles) error { return nil }
		_, closed := trackHandleCleanup(b)
		runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, 2)
		go b.readLoop(runtimeCtx)
		go b.ingestLoop(runtimeCtx)
		_ = b.StopContext(context.Background())
		select {
		case <-b.workersDone:
		case <-time.After(2 * time.Second):
			runtimeCancel()
			t.Fatal("workers stuck")
		}
		runtimeCancel()
		lastRemain = b.workerRemain.Load()
		if closed.Load() != 1 {
			t.Fatalf("iteration %d: handles closed=%d", i, closed.Load())
		}
		if lastRemain != 0 {
			t.Fatalf("iteration %d: workerRemain=%d", i, lastRemain)
		}
	}
}
