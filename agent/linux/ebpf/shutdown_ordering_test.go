package ebpf

import (
	"context"
	"errors"
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

// backendLossNotInSink returns LossSnapshot totals for classes that have not
// already been represented as sink DroppedRecordsCount (TRANSPORT_NOT_WIRED or
// failed emit paths). When emitDrop succeeded, sink dropped already covers them.
func backendLossNotInSink(b *Backend, sinkDropped uint64) uint64 {
	snap := b.LossSnapshot()
	backend := snap[LossClassMapUpdateFail] + snap[LossClassRingReserveFail] +
		snap[LossClassUserspaceChanFull] + snap[LossClassDecoderReject]
	if backend <= sinkDropped {
		return 0
	}
	return backend - sinkDropped
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
	unrepresented := backendLossNotInSink(b, dropped)
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
func TestDetachFailureTypedIncompleteEvidence(t *testing.T) {
	agg, _ := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	rd := &fakeEventReader{records: [][]byte{putEventV1(t, nil)}}
	b.detachProbesFn = func(*kernelHandles) error {
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
	err = b.StopContext(stopCtx)
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
	unrepresented := backendLossNotInSink(b, dropped)
	if accepted+dropped+shed+unrepresented != n {
		t.Fatalf("DISJOINT conservation failed: accepted=%d dropped=%d shed=%d unrepresented=%d want=%d (len(evidence)=%d)",
			accepted, dropped, shed, unrepresented, n, len(sink.ProcessNetwork))
	}
	if accepted < 1 {
		t.Fatal("expected at least one accepted represented event")
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
