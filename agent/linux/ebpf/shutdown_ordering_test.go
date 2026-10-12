package ebpf

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/agent/common/collector"
)

// inject appends a late kernel-ring record while the reader remains open.
func (f *fakeEventReader) inject(raw []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, raw)
}

// processNetworkRepresentedTotals sums DISJOINT sink categories:
//   - accepted: connection-lifecycle counters (never backend loss counters)
//   - dropped/shed: explicit drop/shed evidence already represented in the sink
//
// Callers must not add Backend LossSnapshot map/ring/chan/decoder totals on top
// of dropped when those BACKEND_DROP records were already emitDrop'd into the
// sink — that would double-count the same loss.
func processNetworkRepresentedTotals(sink *collector.LocalRecordingStructuredEvidenceSink) (accepted, dropped, shed uint64) {
	for _, e := range sink.ProcessNetwork {
		if e.NewConnections != nil {
			accepted += uint64(*e.NewConnections)
		}
		if e.ClosedConnections != nil {
			accepted += uint64(*e.ClosedConnections)
		}
		if e.ConnectionFailures != nil {
			accepted += uint64(*e.ConnectionFailures)
		}
		accepted += uint64(e.SampledEventsCount)
		dropped += uint64(e.DroppedRecordsCount)
		shed += uint64(e.ShedRecordsCount)
	}
	for _, a := range sink.ProcessNetworkAccounting {
		dropped += uint64(a.DroppedRecordsCount)
		shed += uint64(a.ShedRecordsCount)
	}
	return
}

// errBackendLossCoverageAmbiguous is returned when sink DroppedRecordsCount and
// LossSnapshot cannot be reconciled without LossClass-keyed sink attribution.
// Production emitDrop discards the reason string, so aggregate subtraction alone
// cannot prove per-class coverage when sinkDropped exceeds backend totals.
var errBackendLossCoverageAmbiguous = errors.New("BACKEND_LOSS_COVERAGE_AMBIGUOUS")

// backendLossTotal sums all LossClass counters (authoritative backend loss).
func backendLossTotal(b *Backend) uint64 {
	snap := b.LossSnapshot()
	return snap[LossClassMapUpdateFail] + snap[LossClassRingReserveFail] +
		snap[LossClassUserspaceChanFull] + snap[LossClassDecoderReject]
}

// backendLossNotInSink returns LossSnapshot units not covered by sink
// DroppedRecordsCount under the fixture contract:
//
//	sinkDropped is produced only by emitDrop→BACKEND_DROP for this Backend
//	(no foreign BACKEND_DROP / non-backend drop credits).
//
// Because sink drops are not LossClass-keyed (reason discarded at emit),
// sinkDropped > backendTotal is AMBIGUOUS — not “fully represented”.
// Callers must Fatal on err; do not treat (0, err) as coverage success.
func backendLossNotInSink(b *Backend, sinkDropped uint64) (unrepresented uint64, err error) {
	backend := backendLossTotal(b)
	if sinkDropped > backend {
		return 0, fmt.Errorf("%w: sinkDropped=%d backendLoss=%d (foreign/extra sink drops can mask unemitted LossSnapshot)",
			errBackendLossCoverageAmbiguous, sinkDropped, backend)
	}
	return backend - sinkDropped, nil
}

// ringPathLossNotInSink is for ring-record conservation fixtures only.
// MAP/RING kernel counters are not ring-record substitutes; their presence
// fails the fixture contract instead of entering the conservation sum.
func ringPathLossNotInSink(b *Backend, sinkDropped uint64) (unrepresented uint64, err error) {
	snap := b.LossSnapshot()
	kernel := snap[LossClassMapUpdateFail] + snap[LossClassRingReserveFail]
	if kernel != 0 {
		return 0, fmt.Errorf("%w: kernel MAP/RING loss=%d present in ring-conservation fixture",
			errBackendLossCoverageAmbiguous, kernel)
	}
	path := snap[LossClassUserspaceChanFull] + snap[LossClassDecoderReject]
	if sinkDropped > path {
		return 0, fmt.Errorf("%w: sinkDropped=%d pathLoss=%d",
			errBackendLossCoverageAmbiguous, sinkDropped, path)
	}
	return path - sinkDropped, nil
}

// TestStopRequestedBeforeDetachBegins proves StopContext publishes stopRequested
// before the owned worker initiates probe detach (barrier-controlled).
func TestStopRequestedBeforeDetachBegins(t *testing.T) {
	agg, _ := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	rd := &fakeEventReader{}
	detachBegin := make(chan struct{})
	allowDetach := make(chan struct{})
	var detachOnce sync.Once
	var sawStopBeforeDetach atomic.Bool
	b.detachProbesFn = func(*kernelHandles) error {
		if b.isStopRequested() {
			sawStopBeforeDetach.Store(true)
		}
		detachOnce.Do(func() { close(detachBegin) })
		<-allowDetach
		return nil
	}
	_, _ = trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, 4)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	stopDone := make(chan error, 1)
	go func() { stopDone <- b.StopContext(context.Background()) }()

	select {
	case <-detachBegin:
	case <-time.After(2 * time.Second):
		t.Fatal("detach did not begin")
	}
	if !sawStopBeforeDetach.Load() {
		t.Fatal("stopRequested must be published before probe detach begins")
	}
	if !b.isStopRequested() {
		t.Fatal("stopRequested must remain latched during detach")
	}
	close(allowDetach)
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("StopContext: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StopContext did not complete")
	}
}

// TestDelayedDetachLateRingEventConserved is the P1-1 regression: a late ring
// event published while detach is intentionally held must still be drained after
// quiescence. Drain completion is not accepted before detach completes.
func TestDelayedDetachLateRingEventConserved(t *testing.T) {
	agg, sink := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	const initial = 2
	const late = 3
	records := make([][]byte, initial)
	for i := 0; i < initial; i++ {
		records[i] = putEventV1(t, nil)
	}
	rd := &fakeEventReader{records: records}
	detachBegin := make(chan struct{})
	allowDetach := make(chan struct{})
	var detachOnce sync.Once
	var detachDone atomic.Bool
	b.detachProbesFn = func(*kernelHandles) error {
		detachOnce.Do(func() { close(detachBegin) })
		<-allowDetach
		detachDone.Store(true)
		return nil
	}
	_, closed := trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, initial+late)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	stopDone := make(chan error, 1)
	go func() { stopDone <- b.StopContext(context.Background()) }()

	select {
	case <-detachBegin:
	case <-time.After(2 * time.Second):
		t.Fatal("detach barrier not reached")
	}
	// Detach held: reader must not have declared clean drain / closed rawCh path.
	select {
	case <-b.workersDone:
		t.Fatal("workers completed before detach finished — drain raced ahead of quiescence")
	default:
	}
	if detachDone.Load() {
		t.Fatal("precondition: detach still held")
	}
	for i := 0; i < late; i++ {
		rd.inject(putEventV1(t, nil))
	}
	if rd.isClosed() {
		t.Fatal("reader must remain open while detach is in progress")
	}

	close(allowDetach)
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("StopContext: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StopContext did not complete after detach release")
	}
	if !detachDone.Load() {
		t.Fatal("detach must complete")
	}
	if rd.pending() != 0 {
		t.Fatalf("unread ring records remain: %d", rd.pending())
	}
	accepted, dropped, shed := processNetworkRepresentedTotals(sink)
	unrepresented, covErr := ringPathLossNotInSink(b, dropped)
	if covErr != nil {
		t.Fatalf("ring path loss coverage: %v (accepted=%d dropped=%d shed=%d loss=%v)",
			covErr, accepted, dropped, shed, b.LossSnapshot())
	}
	represented := accepted + dropped + shed + unrepresented
	want := uint64(initial + late)
	if represented != want {
		t.Fatalf("DISJOINT conservation: accepted=%d dropped=%d shed=%d unrepresentedLoss=%d total=%d want=%d sinkEvidence=%d",
			accepted, dropped, shed, unrepresented, represented, want, len(sink.ProcessNetwork))
	}
	if closed.Load() != 1 {
		t.Fatalf("handles closed=%d", closed.Load())
	}
}

// TestDrainIdleBeforeQuiesceNeverClean proves idle ring deadlines are not
// accepted as clean drain while producersQuiesced is false.
func TestDrainIdleBeforeQuiesceNeverClean(t *testing.T) {
	b := testBackendInactive(t)
	rd := &fakeEventReader{} // empty → idle deadline
	b.mu.Lock()
	b.producersQuiesced = false
	b.shutdownCtx = context.Background()
	b.mu.Unlock()

	// Bounded op context so !quiesced drain cannot spin forever.
	opCtx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	b.mu.Lock()
	b.shutdownCtx = opCtx
	b.mu.Unlock()

	b.drainKernelRing(context.Background(), rd)
	b.mu.Lock()
	flushErr := b.shutdownFlushErr
	b.mu.Unlock()
	if flushErr == nil || !errors.Is(flushErr, ErrBackendRingDrainIncomplete) {
		t.Fatalf("idle-before-quiesce must record ring drain incomplete, got %v", flushErr)
	}
}

// errDetachCause is a typed detach failure retained for errors.Is assertions.
var errDetachCause = errors.New("link close failed")

// TestDetachFailureTypedIncompleteEvidence covers device detach error → never nil Stop.
//
// The production readLoop arms a 200ms ReadRaw deadline and fakeEventReader's
// uninterruptible sleep is not woken by runtime cancel. A Stop budget equal to
// that deadline can therefore expire before detach runs, returning only
// ErrBackendStopDeadline — correct caller-bounded behavior when detach has not
// completed. This fixture therefore uses an interruptible reader block and
// waits for the injected detach failure before asserting typed quiesce evidence.
// Bounded-deadline-without-detach remains covered by
// TestStalledDetachStopReturnsWithinDeadline.
func TestDetachFailureTypedIncompleteEvidence(t *testing.T) {
	agg, _ := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	block := make(chan struct{})
	rd := &fakeEventReader{
		records: [][]byte{putEventV1(t, nil)},
		block:   block,
	}
	detachDone := make(chan struct{})
	var detachOnce sync.Once
	b.detachProbesFn = func(*kernelHandles) error {
		detachOnce.Do(func() { close(detachDone) })
		return errDetachCause
	}
	_, _ = trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, 4)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	// Short budget: failed detach leaves producersQuiesced=false so drain will
	// not accept idle-as-empty and exits via the governed shutdown deadline.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer stopCancel()
	stopDone := make(chan error, 1)
	go func() { stopDone <- b.StopContext(stopCtx) }()

	waitUntil := time.After(2 * time.Second)
	for !b.isStopRequested() {
		select {
		case <-waitUntil:
			t.Fatal("stopRequested was not published")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	// Wake the armed ReadRaw so shutdownReadPath runs while stop budget remains.
	rd.interruptRead()

	select {
	case <-detachDone:
	case <-time.After(2 * time.Second):
		t.Fatal("injected detach failure never executed")
	}

	err = <-stopDone
	if err == nil {
		t.Fatal("detach failure must not yield clean Stop")
	}
	if !errors.Is(err, ErrBackendProducerQuiesceIncomplete) {
		t.Fatalf("want ErrBackendProducerQuiesceIncomplete, got %v", err)
	}
	if !errors.Is(err, errDetachCause) {
		t.Fatalf("detach cause must remain errors.Is visible, got %v", err)
	}
	if b.kernelProducersQuiesced() {
		t.Fatal("failed detach must not latch producersQuiesced")
	}
}

// TestStalledDetachStopReturnsWithinDeadline is the P1-2 regression: blocked
// link.Close cannot pin StopContext beyond the caller budget. Cleanup ownership
// remains with workers; no force-close of maps/programs while live.
func TestStalledDetachStopReturnsWithinDeadline(t *testing.T) {
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
	detachEntered := make(chan struct{})
	var once sync.Once
	b.detachProbesFn = func(*kernelHandles) error {
		once.Do(func() { close(detachEntered) })
		<-hold // stall beyond stop budget
		return nil
	}
	handles, closed := trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, 2)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer stopCancel()
	start := time.Now()
	err = b.StopContext(stopCtx)
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("StopContext blocked too long under stalled detach: %v", elapsed)
	}
	if !errors.Is(err, ErrBackendStopDeadline) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline exhaustion identities, got %v", err)
	}
	select {
	case <-detachEntered:
	case <-time.After(time.Second):
		t.Fatal("owned worker never began detach")
	}
	b.mu.Lock()
	retained := b.handles == handles
	requested := b.stopRequested
	stopped := b.stopped
	b.mu.Unlock()
	if !retained || closed.Load() != 0 {
		t.Fatalf("handles must remain worker-owned after deadline: retained=%v closes=%d", retained, closed.Load())
	}
	if !requested || stopped {
		t.Fatalf("cleanup-pending state: requested=%v stopped=%v", requested, stopped)
	}
	// Unblock worker so the test process can exit cleanly.
	close(hold)
	select {
	case <-b.workersDone:
	case <-time.After(2 * time.Second):
		t.Fatal("workers did not finish after detach release")
	}
	if closed.Load() != 1 {
		t.Fatalf("final worker must release handles exactly once, closes=%d", closed.Load())
	}
	// P2: first unsuccessful Stop must remain observable after async cleanup.
	if err := b.StopContext(context.Background()); err == nil || !errors.Is(err, ErrBackendStopDeadline) {
		t.Fatalf("repeated Stop after stalled-detach cleanup must retain first deadline, got %v", err)
	}
}

// TestBackendCancelIsNotStopDeadline ensures explicit cancellation is not
// mislabeled as ErrBackendStopDeadline.
func TestBackendCancelIsNotStopDeadline(t *testing.T) {
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitBackendWorkers(ctx, done)
	if err == nil {
		t.Fatal("expected cancel error")
	}
	if errors.Is(err, ErrBackendStopDeadline) {
		t.Fatalf("cancellation must not become ErrBackendStopDeadline: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

// TestBackendDeadlineExhaustionPreservesTypedCauses covers actual expiry.
func TestBackendDeadlineExhaustionPreservesTypedCauses(t *testing.T) {
	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	err := waitBackendWorkers(ctx, done)
	if !errors.Is(err, ErrBackendStopDeadline) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want ErrBackendStopDeadline+DeadlineExceeded, got %v", err)
	}
}

// TestExactRingRecordConservationAcceptedPlusDrops uses represented totals.
func TestExactRingRecordConservationAcceptedPlusDrops(t *testing.T) {
	agg, sink := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	const n = 7
	records := make([][]byte, n)
	for i := 0; i < n; i++ {
		records[i] = putEventV1(t, nil)
	}
	rd := &fakeEventReader{records: records}
	b.detachProbesFn = func(*kernelHandles) error { return nil }
	_, _ = trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, n)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	if err := b.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	accepted, dropped, shed := processNetworkRepresentedTotals(sink)
	unrepresented, covErr := ringPathLossNotInSink(b, dropped)
	if covErr != nil {
		t.Fatalf("ring path loss coverage: %v (accepted=%d dropped=%d shed=%d loss=%v)",
			covErr, accepted, dropped, shed, b.LossSnapshot())
	}
	if accepted+dropped+shed+unrepresented != n {
		t.Fatalf("DISJOINT conservation failed: accepted=%d dropped=%d shed=%d unrepresented=%d want=%d (len(evidence)=%d)",
			accepted, dropped, shed, unrepresented, n, len(sink.ProcessNetwork))
	}
	if accepted < 1 {
		t.Fatal("expected at least one accepted represented event")
	}
}

// TestBackendLossNotInSinkRejectsForeignSinkMasking is the adversarial
// counterexample for the old aggregate subtraction oracle: foreign sink
// DroppedRecordsCount larger than LossSnapshot must not report unrepresented=0.
func TestBackendLossNotInSinkRejectsForeignSinkMasking(t *testing.T) {
	agg, sink := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Unemitted LossSnapshot (never emitDrop).
	b.loss.Add(LossClassMapUpdateFail, 3)

	// Foreign sink drops: BACKEND_DROP via aggregator only (no LossSnapshot bump).
	n := uint32(5)
	ctx := context.Background()
	if err := agg.Ingest(ctx, []collector.ProcessNetworkSyntheticEvent{{
		ObservedAt:   time.Now().UTC(),
		Class:        collector.ProcessNetworkEventBackendDrop,
		DroppedCount: &n,
	}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if _, err := agg.FlushAll(ctx); err != nil {
		t.Fatalf("FlushAll: %v", err)
	}
	_, dropped, _ := processNetworkRepresentedTotals(sink)
	if dropped < 5 {
		t.Fatalf("precondition: sink dropped=%d want >=5", dropped)
	}

	// Old arithmetic (inline oracle): masks MAP=3 behind foreign sink=5.
	oldBackend := backendLossTotal(b)
	var oldUnrep uint64
	if oldBackend > dropped {
		oldUnrep = oldBackend - dropped
	}
	if oldUnrep != 0 {
		t.Fatalf("adversarial precondition broken: old helper unrepresented=%d (want 0 mask)", oldUnrep)
	}

	unrep, covErr := backendLossNotInSink(b, dropped)
	if covErr == nil {
		t.Fatalf("new helper must reject ambiguous coverage; unrepresented=%d", unrep)
	}
	if !errors.Is(covErr, errBackendLossCoverageAmbiguous) {
		t.Fatalf("want errBackendLossCoverageAmbiguous, got %v", covErr)
	}
}

// TestBackendLossNotInSinkReportsUnemittedWhenSinkEmpty proves unemitted
// LossSnapshot remains visible when the sink has zero drop coverage.
func TestBackendLossNotInSinkReportsUnemittedWhenSinkEmpty(t *testing.T) {
	b := testBackendInactive(t)
	b.loss.Add(LossClassMapUpdateFail, 3)
	unrep, err := backendLossNotInSink(b, 0)
	if err != nil {
		t.Fatalf("empty sink coverage: %v", err)
	}
	if unrep != 3 {
		t.Fatalf("unrepresented=%d want 3", unrep)
	}
}

// TestDeadlinePathDoesNotForceCloseReaderWhileWorkersLive ensures removing the
// synchronous closeReader-on-deadline path: stalled detach leaves the reader
// usable for the owned worker.
func TestDeadlinePathDoesNotForceCloseReaderWhileWorkersLive(t *testing.T) {
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
	b.detachProbesFn = func(*kernelHandles) error {
		<-hold
		return nil
	}
	_, _ = trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, 2)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer stopCancel()
	_ = b.StopContext(stopCtx)
	if rd.isClosed() {
		t.Fatal("deadline path must not force-close reader while detach worker is live")
	}
	close(hold)
	select {
	case <-b.workersDone:
	case <-time.After(2 * time.Second):
		t.Fatal("workers stuck")
	}
}
