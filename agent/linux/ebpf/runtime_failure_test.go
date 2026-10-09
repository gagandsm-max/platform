package ebpf

import (
	"context"
	"errors"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/agent/common/collector"
)

func testBackendInactive(t testing.TB) *Backend {
	t.Helper()
	agg, _ := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config: Config{
			Enabled:          false,
			PreflightTimeout: time.Second,
			RecheckMin:       time.Minute,
		},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testBackendWithSink(t *testing.T, sink collector.StructuredEvidenceSink) *Backend {
	t.Helper()
	agg, err := collector.NewProcessNetworkAggregator(collector.ProcessNetworkAggregatorConfig{
		SourceProvenance:     collector.ProcessNetworkSyntheticSourceProvenance,
		Window:               time.Minute,
		TestAllowShortWindow: true,
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func oldSyntheticEvent(t *testing.T) collector.ProcessNetworkSyntheticEvent {
	t.Helper()
	raw := putEventV1(t, nil)
	ev, err := decodeRawRecord(raw, time.Now().UTC(), 0)
	if err != nil {
		t.Fatal(err)
	}
	syn, err := toSynthetic(ev)
	if err != nil {
		t.Fatal(err)
	}
	syn.ObservedAt = time.Now().UTC().Add(-2 * time.Minute)
	return syn
}

func trackHandleCleanup(b *Backend) (*kernelHandles, *atomic.Int32) {
	handles := &kernelHandles{}
	closed := &atomic.Int32{}
	b.mu.Lock()
	b.handles = handles
	b.closeHandles = func(got *kernelHandles) {
		if got == handles {
			closed.Add(1)
			return
		}
		closed.Add(1000)
	}
	b.mu.Unlock()
	return handles, closed
}

func armRuntime(b *Backend) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	b.mu.Lock()
	b.cancel = cancel
	b.started = true
	b.stopRequested = false
	b.stopped = false
	b.workersDone = make(chan struct{})
	b.workerRemain.Store(int32(BackendGoroutines))
	b.capability = collector.ProcessNetworkCapabilitySnapshot{
		State:       collector.ProcessNetworkCapabilityActive,
		ReasonCodes: nil,
		ObservedAt:  time.Now().UTC(),
	}
	b.mu.Unlock()
	return ctx, cancel
}

func hasReason(cap collector.ProcessNetworkCapabilitySnapshot, want collector.ProcessNetworkCapabilityReason) bool {
	for _, r := range cap.ReasonCodes {
		if r == want {
			return true
		}
	}
	return false
}

func TestRingReadUnexpectedErrorNotDecoderReject(t *testing.T) {
	b := testBackendInactive(t)
	ctx, cancel := armRuntime(b)
	defer cancel()

	stop := b.handleRingReadError(ctx, errors.New("ring I/O boom"))
	if !stop {
		t.Fatal("unexpected reader error must stop the read loop")
	}
	if b.LossSnapshot()[LossClassDecoderReject] != 0 {
		t.Fatalf("must not classify as DECODER_REJECT: %v", b.LossSnapshot())
	}
	if b.ReaderFaultCount() != 1 {
		t.Fatalf("readerFaults=%d", b.ReaderFaultCount())
	}
	cap := b.Capability()
	if cap.State == collector.ProcessNetworkCapabilityActive {
		t.Fatalf("must leave ACTIVE: %+v", cap)
	}
	if !hasReason(cap, collector.ReasonReaderFailed) {
		t.Fatalf("want READER_FAILED got %+v", cap)
	}
	if ctx.Err() == nil {
		t.Fatal("runtime must be cancelled")
	}
	// No busy-loop: second identical classification while already failed stays terminal.
	if !b.handleRingReadError(ctx, errors.New("ring I/O boom")) {
		t.Fatal("still terminal")
	}
	if b.LossSnapshot()[LossClassDecoderReject] != 0 {
		t.Fatal("still must not accumulate DECODER_REJECT")
	}
}

func TestRingReadDeadlineContinuesWithoutDecoderReject(t *testing.T) {
	b := testBackendInactive(t)
	ctx, cancel := armRuntime(b)
	defer cancel()
	if b.handleRingReadError(ctx, os.ErrDeadlineExceeded) {
		t.Fatal("deadline must not stop read loop")
	}
	if b.Capability().State != collector.ProcessNetworkCapabilityActive {
		t.Fatal("deadline must not leave ACTIVE")
	}
	if b.LossSnapshot()[LossClassDecoderReject] != 0 {
		t.Fatal("deadline must not count as DECODER_REJECT")
	}
	if b.ReaderFaultCount() != 0 {
		t.Fatal("deadline is not READER_FAILED")
	}
}

func TestMalformedRawRecordRemainsDecoderReject(t *testing.T) {
	b := testBackendInactive(t)
	b.mu.Lock()
	b.capability = collector.ProcessNetworkCapabilitySnapshot{
		State:      collector.ProcessNetworkCapabilityActive,
		ObservedAt: time.Now().UTC(),
	}
	b.rawCh = make(chan []byte, 1)
	b.bootWall = time.Now().UTC()
	b.workersDone = make(chan struct{})
	b.workerRemain.Store(1)
	b.mu.Unlock()

	ctx, cancelIngest := context.WithCancel(context.Background())
	go b.ingestLoop(ctx)

	b.rawCh <- []byte{0x01, 0x02} // too short / malformed
	deadline := time.After(2 * time.Second)
	for {
		if b.LossSnapshot()[LossClassDecoderReject] >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected DECODER_REJECT, snap=%v", b.LossSnapshot())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if b.Capability().State != collector.ProcessNetworkCapabilityActive {
		t.Fatalf("decoder reject must not kill backend: %+v", b.Capability())
	}
	cancelIngest()
	select {
	case <-b.workersDone:
	case <-time.After(2 * time.Second):
		t.Fatal("ingest worker did not exit")
	}
}

func TestKernelLossReconcileExactOnceSequence(t *testing.T) {
	var l lossCounters
	abs := []uint64{0, 1, 1, 3}
	want := []uint64{0, 1, 0, 2}
	var got []uint64
	for _, a := range abs {
		d, _ := l.reconcileKernelAbs(a, 0)
		got = append(got, d)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("deltas got %v want %v", got, want)
		}
	}
	if l.Snapshot()[LossClassMapUpdateFail] != 3 {
		t.Fatalf("absolute=%d", l.Snapshot()[LossClassMapUpdateFail])
	}
}

func TestKernelLossReconcileConcurrentNoDoubleCount(t *testing.T) {
	var l lossCounters
	const goroutines = 32
	var wg sync.WaitGroup
	deltas := make(chan uint64, goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			d, _ := l.reconcileKernelAbs(5, 0)
			deltas <- d
		}()
	}
	wg.Wait()
	close(deltas)
	var sum uint64
	var nonzero int
	for d := range deltas {
		sum += d
		if d != 0 {
			nonzero++
		}
	}
	if sum != 5 || nonzero != 1 {
		t.Fatalf("sum=%d nonzero=%d absolute=%d", sum, nonzero, l.Snapshot()[LossClassMapUpdateFail])
	}
	// Regression / reorder: lower absolute must not underflow or emit.
	d, _ := l.reconcileKernelAbs(3, 0)
	if d != 0 || l.Snapshot()[LossClassMapUpdateFail] != 5 {
		t.Fatalf("regression fabricate: d=%d abs=%d", d, l.Snapshot()[LossClassMapUpdateFail])
	}
	// Repeat same absolute: zero delta.
	d, _ = l.reconcileKernelAbs(5, 0)
	if d != 0 {
		t.Fatalf("repeat delta=%d", d)
	}
}

func TestKernelLossUint64RepresentationBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		delta uint64
	}{
		{name: "max_uint32_exact", delta: uint64(math.MaxUint32)},
		{name: "max_uint32_plus_one_saturates", delta: uint64(math.MaxUint32) + 1},
		{name: "one_shift_32_never_zero", delta: uint64(1) << 32},
		{name: "max_uint64_bounded", delta: uint64(math.MaxUint64)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var loss lossCounters
			delta, _ := loss.reconcileKernelAbs(tc.delta, 0)
			if delta != tc.delta {
				t.Fatalf("delta=%d want=%d", delta, tc.delta)
			}
			if got := loss.Snapshot()[LossClassMapUpdateFail]; got != tc.delta {
				t.Fatalf("uint64 authority=%d want=%d", got, tc.delta)
			}

			ev := backendDropEvent64(string(LossClassMapUpdateFail), delta, time.Now().UTC())
			if ev.DroppedCount == nil {
				t.Fatal("missing bounded drop representation")
			}
			wantCount := uint32(tc.delta)
			wantGuarded := false
			if tc.delta > uint64(math.MaxUint32) {
				wantCount = math.MaxUint32
				wantGuarded = true
			}
			if got := *ev.DroppedCount; got != wantCount {
				t.Fatalf("represented=%d want=%d", got, wantCount)
			}
			if ev.ResourceGuarded != wantGuarded {
				t.Fatalf("resource_guarded=%v want=%v", ev.ResourceGuarded, wantGuarded)
			}
			if tc.delta > uint64(math.MaxUint32) && *ev.DroppedCount == 0 {
				t.Fatal("oversized uint64 loss wrapped to zero")
			}

			repeat, _ := loss.reconcileKernelAbs(tc.delta, 0)
			if repeat != 0 || loss.Snapshot()[LossClassMapUpdateFail] != tc.delta {
				t.Fatalf("repeat changed authority: delta=%d snapshot=%v", repeat, loss.Snapshot())
			}
		})
	}
}

func TestAggregatorIngestErrorObservable(t *testing.T) {
	b := testBackendInactive(t)
	ctx, cancel := armRuntime(b)
	defer cancel()
	if err := b.aggregator.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	batch := []collector.ProcessNetworkSyntheticEvent{
		backendDropEvent(string(LossClassDecoderReject), 1, time.Now().UTC()),
	}
	fatal := b.flushBatch(ctx, ctx, &batch)
	if !fatal {
		t.Fatal("expected fatal aggregator fault")
	}
	if b.AggregatorFaultCount() != 1 {
		t.Fatalf("faults=%d", b.AggregatorFaultCount())
	}
	cap := b.Capability()
	if cap.State == collector.ProcessNetworkCapabilityActive {
		t.Fatalf("must leave ACTIVE: %+v", cap)
	}
	if !hasReason(cap, collector.ReasonSourceUnavailable) {
		t.Fatalf("want SOURCE_UNAVAILABLE got %+v", cap)
	}
	if ctx.Err() == nil {
		t.Fatal("must cancel runtime")
	}
}

func TestAggregatorFlushDueErrorObservable(t *testing.T) {
	sink := &failingSink{err: errors.New("AGGREGATOR_SINK_BROKEN")}
	agg, err := collector.NewProcessNetworkAggregator(collector.ProcessNetworkAggregatorConfig{
		SourceProvenance:     collector.ProcessNetworkSyntheticSourceProvenance,
		Window:               time.Minute,
		TestAllowShortWindow: true,
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := armRuntime(b)
	defer cancel()

	raw := putEventV1(t, nil)
	ev, err := decodeRawRecord(raw, time.Now().UTC(), 0)
	if err != nil {
		t.Fatal(err)
	}
	syn, err := toSynthetic(ev)
	if err != nil {
		t.Fatal(err)
	}
	// Force window end into the past so FlushDue delivers immediately.
	syn.ObservedAt = time.Now().UTC().Add(-2 * time.Minute)
	batch := []collector.ProcessNetworkSyntheticEvent{syn}
	fatal := b.flushBatch(ctx, ctx, &batch)
	if !fatal {
		t.Fatal("expected FlushDue sink failure to be fatal")
	}
	if b.AggregatorFaultCount() == 0 {
		t.Fatal("FlushDue error must be observable")
	}
	if b.Capability().State == collector.ProcessNetworkCapabilityActive {
		t.Fatal("must leave ACTIVE")
	}
}

type failingSink struct {
	err error
}

func (f *failingSink) AcceptSoftwareInventory(context.Context, collector.SoftwareInventorySnapshot) error {
	return f.err
}
func (f *failingSink) AcceptProcessServiceCatalogue(context.Context, collector.ProcessServiceDiscoveryCatalogue) error {
	return f.err
}
func (f *failingSink) AcceptProcessNetworkEvidence(context.Context, collector.ProcessNetworkEvidence) error {
	return f.err
}
func (f *failingSink) AcceptProcessNetworkWindowAccounting(context.Context, collector.ProcessNetworkWindowAccounting) error {
	return f.err
}
func (f *failingSink) DeliveryState() collector.StructuredEvidenceDeliveryState {
	return collector.StructuredEvidenceLocalOnly
}

type contextBlockingSink struct {
	entered      chan context.Context
	canceled     chan struct{}
	release      <-chan struct{}
	enteredOnce  sync.Once
	canceledOnce sync.Once
}

func newContextBlockingSink(release <-chan struct{}) *contextBlockingSink {
	return &contextBlockingSink{
		entered:  make(chan context.Context, 1),
		canceled: make(chan struct{}),
		release:  release,
	}
}

func (s *contextBlockingSink) block(ctx context.Context) error {
	s.enteredOnce.Do(func() { s.entered <- ctx })
	<-ctx.Done()
	s.canceledOnce.Do(func() { close(s.canceled) })
	if s.release != nil {
		<-s.release
	}
	return ctx.Err()
}

func (s *contextBlockingSink) AcceptSoftwareInventory(ctx context.Context, _ collector.SoftwareInventorySnapshot) error {
	return s.block(ctx)
}

func (s *contextBlockingSink) AcceptProcessServiceCatalogue(ctx context.Context, _ collector.ProcessServiceDiscoveryCatalogue) error {
	return s.block(ctx)
}

func (s *contextBlockingSink) AcceptProcessNetworkEvidence(ctx context.Context, _ collector.ProcessNetworkEvidence) error {
	return s.block(ctx)
}

func (s *contextBlockingSink) AcceptProcessNetworkWindowAccounting(ctx context.Context, _ collector.ProcessNetworkWindowAccounting) error {
	return s.block(ctx)
}

func (s *contextBlockingSink) DeliveryState() collector.StructuredEvidenceDeliveryState {
	return collector.StructuredEvidenceLocalOnly
}

func startIngestLoopWithPendingOldEvent(t *testing.T, b *Backend) (context.Context, context.CancelFunc) {
	t.Helper()
	raw := putEventV1(t, nil)
	runtimeCtx, cancel := armRuntime(b)
	b.mu.Lock()
	b.rawCh = make(chan []byte)
	b.bootWall = time.Now().UTC().Add(-2 * time.Minute)
	b.workerRemain.Store(1)
	b.mu.Unlock()

	go b.ingestLoop(runtimeCtx)
	sent := make(chan struct{})
	go func() {
		b.rawCh <- raw
		close(sent)
	}()
	select {
	case <-sent:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("ingest worker did not receive pending shutdown record")
	}
	return runtimeCtx, cancel
}

func TestTransportNotWiredPreservedNotKernelFailure(t *testing.T) {
	sink := collector.UnwiredStructuredEvidenceSink{}
	agg, err := collector.NewProcessNetworkAggregator(collector.ProcessNetworkAggregatorConfig{
		SourceProvenance:     collector.ProcessNetworkSyntheticSourceProvenance,
		Window:               time.Minute,
		TestAllowShortWindow: true,
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := armRuntime(b)
	defer cancel()

	raw := putEventV1(t, nil)
	ev, err := decodeRawRecord(raw, time.Now().UTC(), 0)
	if err != nil {
		t.Fatal(err)
	}
	syn, err := toSynthetic(ev)
	if err != nil {
		t.Fatal(err)
	}
	syn.ObservedAt = time.Now().UTC().Add(-2 * time.Minute)
	batch := []collector.ProcessNetworkSyntheticEvent{syn}
	fatal := b.flushBatch(ctx, ctx, &batch)
	if fatal {
		t.Fatal("TRANSPORT_NOT_WIRED must not be fatal")
	}
	if b.TransportNotWiredCount() == 0 {
		t.Fatal("non-delivery must be observable")
	}
	if b.AggregatorFaultCount() != 0 {
		t.Fatal("must not count as aggregator fault")
	}
	if b.LossSnapshot()[LossClassMapUpdateFail] != 0 || b.LossSnapshot()[LossClassDecoderReject] != 0 {
		t.Fatalf("must not misclassify as kernel/decoder loss: %v", b.LossSnapshot())
	}
	cap := b.Capability()
	if cap.State != collector.ProcessNetworkCapabilityActive {
		t.Fatalf("collection-capable ACTIVE preserved: %+v", cap)
	}
	if hasReason(cap, collector.ReasonReaderFailed) || hasReason(cap, collector.ReasonSourceUnavailable) {
		t.Fatalf("must not look like kernel/source failure: %+v", cap)
	}
	if ctx.Err() != nil {
		t.Fatal("must not cancel runtime for TRANSPORT_NOT_WIRED")
	}
}

func TestStopContextDeadlineVisible(t *testing.T) {
	b := testBackendInactive(t)
	b.mu.Lock()
	b.started = true
	b.stopped = false
	b.workersDone = make(chan struct{}) // never closed → hung workers
	b.capability = collector.ProcessNetworkCapabilitySnapshot{
		State: collector.ProcessNetworkCapabilityActive,
	}
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := b.StopContext(ctx)
	if err == nil {
		t.Fatal("expected non-nil stop deadline error")
	}
	if !errors.Is(err, ErrBackendStopDeadline) {
		t.Fatalf("want ErrBackendStopDeadline got %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want wrapped DeadlineExceeded got %v", err)
	}
	// No permanent Wait-goroutine was spawned by StopContext; hung workersDone
	// remains unclosed by design of this fixture.
	if b.Capability().State == collector.ProcessNetworkCapabilityActive {
		t.Fatal("stop-request capability transition required")
	}
}

func TestStopContextDeadlineDoesNotAccumulateWaiters(t *testing.T) {
	b := testBackendInactive(t)
	b.mu.Lock()
	b.started = true
	b.workersDone = make(chan struct{}) // deliberately never quiesces
	b.capability = collector.ProcessNetworkCapabilitySnapshot{State: collector.ProcessNetworkCapabilityActive}
	b.mu.Unlock()

	baseline := runtime.NumGoroutine()
	const attempts = 32
	for i := 0; i < attempts; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		err := b.StopContext(ctx)
		cancel()
		if !errors.Is(err, ErrBackendStopDeadline) {
			t.Fatalf("attempt %d: want ErrBackendStopDeadline got %v", i, err)
		}
	}

	// Runtime/test harness activity can move by a few goroutines. A leaked
	// helper per timed-out stop would retain all 32 and exceed this tolerance.
	deadline := time.Now().Add(500 * time.Millisecond)
	for runtime.NumGoroutine() > baseline+4 && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > baseline+4 {
		t.Fatalf("timed-out stops accumulated waiter goroutines: before=%d after=%d", baseline, got)
	}
}

func TestStopDeadlineRetainsHandlesUntilFinalWorkerExit(t *testing.T) {
	release := make(chan struct{})
	sink := newContextBlockingSink(release)
	b := testBackendWithSink(t, sink)
	handles, closed := trackHandleCleanup(b)
	_, runtimeCancel := startIngestLoopWithPendingOldEvent(t, b)
	defer runtimeCancel()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	err := b.StopContext(stopCtx)
	stopCancel()
	if !errors.Is(err, ErrBackendStopDeadline) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want typed stop deadline, got %v", err)
	}

	var sinkCtx context.Context
	select {
	case sinkCtx = <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("shutdown flush never reached context-aware sink")
	}
	if _, bounded := sinkCtx.Deadline(); !bounded {
		t.Fatal("shutdown flush context must be bounded")
	}
	select {
	case <-sink.canceled:
	case <-time.After(time.Second):
		t.Fatal("blocked sink did not observe stop deadline")
	}

	b.mu.Lock()
	retained := b.handles == handles
	requested := b.stopRequested
	stopped := b.stopped
	b.mu.Unlock()
	if !retained || closed.Load() != 0 {
		t.Fatalf("handles closed while worker remained live: retained=%v closes=%d", retained, closed.Load())
	}
	if !requested || stopped {
		t.Fatalf("stop state not truthful while cleanup pending: requested=%v stopped=%v", requested, stopped)
	}

	retryCtx, retryCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	retryErr := b.StopContext(retryCtx)
	retryCancel()
	if !errors.Is(retryErr, ErrBackendStopDeadline) {
		t.Fatalf("repeated stop must not hide non-quiesced worker: %v", retryErr)
	}
	if closed.Load() != 0 {
		t.Fatalf("repeated stop force-closed handles: %d", closed.Load())
	}

	close(release)
	select {
	case <-b.workersDone:
	case <-time.After(time.Second):
		t.Fatal("final worker did not perform eventual cleanup")
	}
	if closed.Load() != 1 {
		t.Fatalf("final handle close count=%d", closed.Load())
	}
	b.mu.Lock()
	finalHandles := b.handles
	finalStopped := b.stopped
	b.mu.Unlock()
	if finalHandles != nil || !finalStopped {
		t.Fatalf("final cleanup incomplete: handles=%p stopped=%v", finalHandles, finalStopped)
	}
	if err := b.StopContext(context.Background()); err != nil {
		t.Fatalf("post-cleanup idempotent stop: %v", err)
	}
	if closed.Load() != 1 {
		t.Fatalf("idempotent stop double-closed handles: %d", closed.Load())
	}
}

func TestContextAwareBlockedSinkExitsOnStopDeadline(t *testing.T) {
	sink := newContextBlockingSink(nil)
	b := testBackendWithSink(t, sink)
	_, closed := trackHandleCleanup(b)
	_, runtimeCancel := startIngestLoopWithPendingOldEvent(t, b)
	defer runtimeCancel()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	err := b.StopContext(stopCtx)
	stopCancel()
	if err != nil && !errors.Is(err, ErrBackendStopDeadline) {
		t.Fatalf("unexpected stop result: %v", err)
	}

	var sinkCtx context.Context
	select {
	case sinkCtx = <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("shutdown flush never reached blocked sink")
	}
	if _, bounded := sinkCtx.Deadline(); !bounded {
		t.Fatal("blocked sink did not receive bounded shutdown context")
	}
	select {
	case <-sink.canceled:
	case <-time.After(time.Second):
		t.Fatal("context-aware blocked sink did not observe cancellation")
	}
	select {
	case <-b.workersDone:
	case <-time.After(time.Second):
		t.Fatal("context-aware blocked sink did not let worker quiesce")
	}
	if closed.Load() != 1 {
		t.Fatalf("eventual close count=%d", closed.Load())
	}
}

func TestFatalRuntimeFailureFinalWorkerCleanup(t *testing.T) {
	t.Run("reader_failure", func(t *testing.T) {
		b := testBackendInactive(t)
		runtimeCtx, runtimeCancel := armRuntime(b)
		defer runtimeCancel()
		_, closed := trackHandleCleanup(b)

		go func() {
			defer b.workerExit()
			b.handleRingReadError(runtimeCtx, errors.New("ring I/O boom"))
		}()
		go func() {
			defer b.workerExit()
			<-runtimeCtx.Done()
		}()

		select {
		case <-b.workersDone:
		case <-time.After(time.Second):
			t.Fatal("fatal reader failure did not quiesce both workers")
		}
		if runtimeCtx.Err() == nil {
			t.Fatal("fatal reader failure did not cancel runtime")
		}
		if closed.Load() != 1 {
			t.Fatalf("fatal reader cleanup count=%d", closed.Load())
		}
		b.mu.Lock()
		handles := b.handles
		b.mu.Unlock()
		if handles != nil {
			t.Fatal("fatal reader failure retained kernel handles")
		}
		if !hasReason(b.Capability(), collector.ReasonReaderFailed) {
			t.Fatalf("want READER_FAILED got %+v", b.Capability())
		}
	})

	t.Run("aggregator_failure", func(t *testing.T) {
		b := testBackendWithSink(t, &failingSink{err: errors.New("AGGREGATOR_SINK_BROKEN")})
		runtimeCtx, runtimeCancel := armRuntime(b)
		defer runtimeCancel()
		_, closed := trackHandleCleanup(b)
		raw := putEventV1(t, nil)
		b.mu.Lock()
		b.rawCh = make(chan []byte, IngestBatchSize)
		b.bootWall = time.Now().UTC().Add(-2 * time.Minute)
		b.mu.Unlock()

		go b.readLoop(runtimeCtx)
		go b.ingestLoop(runtimeCtx)
		for i := 0; i < IngestBatchSize; i++ {
			b.rawCh <- raw
		}

		select {
		case <-b.workersDone:
		case <-time.After(2 * time.Second):
			t.Fatal("fatal aggregator failure did not quiesce both workers")
		}
		if runtimeCtx.Err() == nil {
			t.Fatal("fatal aggregator failure did not cancel runtime")
		}
		if b.AggregatorFaultCount() == 0 {
			t.Fatal("fatal aggregator failure was not observable")
		}
		if closed.Load() != 1 {
			t.Fatalf("fatal aggregator cleanup count=%d", closed.Load())
		}
		b.mu.Lock()
		handles := b.handles
		b.mu.Unlock()
		if handles != nil {
			t.Fatal("fatal aggregator failure retained kernel handles")
		}
		if !hasReason(b.Capability(), collector.ReasonSourceUnavailable) {
			t.Fatalf("want SOURCE_UNAVAILABLE got %+v", b.Capability())
		}
	})
}

func TestStopContextNormalIdempotent(t *testing.T) {
	b := testBackendInactive(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := b.StopContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.StopContext(ctx); err != nil {
		t.Fatal(err)
	}

	// Live happy workers: Stop waits for final-worker cleanup and is idempotent.
	b2 := testBackendInactive(t)
	runtimeCtx, runtimeCancel := armRuntime(b2)
	defer runtimeCancel()
	b2.mu.Lock()
	b2.rawCh = make(chan []byte)
	b2.mu.Unlock()
	_, closed := trackHandleCleanup(b2)
	go b2.readLoop(runtimeCtx)
	go b2.ingestLoop(runtimeCtx)
	if err := b2.StopContext(ctx); err != nil {
		t.Fatalf("live stop: %v", err)
	}
	if closed.Load() != 1 {
		t.Fatalf("normal stop close count=%d", closed.Load())
	}
	if err := b2.StopContext(ctx); err != nil {
		t.Fatalf("idempotent: %v", err)
	}
	if closed.Load() != 1 {
		t.Fatalf("idempotent stop double close count=%d", closed.Load())
	}
}

func TestOptionalProbeDetachedAfterFinalCleanup(t *testing.T) {
	b := testBackendInactive(t)
	runtimeCtx, runtimeCancel := armRuntime(b)
	defer runtimeCancel()
	b.mu.Lock()
	b.optionalAvailable = true
	b.rawCh = make(chan []byte)
	b.mu.Unlock()
	_, closed := trackHandleCleanup(b)
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	if !b.SupportSnapshot().OptionalProbeAttached {
		t.Fatal("precondition: optional probe must report attached")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := b.StopContext(ctx); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if closed.Load() != 1 {
		t.Fatalf("final cleanup close count=%d", closed.Load())
	}
	cap := b.Capability()
	if cap.State != collector.ProcessNetworkCapabilityUnavailable || !hasReason(cap, collector.ReasonBackendStopped) {
		t.Fatalf("want BACKEND_STOPPED got %+v", cap)
	}
	if b.SupportSnapshot().OptionalProbeAttached {
		t.Fatal("optional_probe_attached must be false after final cleanup")
	}
	if b.OptionalProbeAttached() {
		t.Fatal("OptionalProbeAttached must be false after final cleanup")
	}
	if closed.Load() != 1 {
		t.Fatalf("no double-close: close count=%d", closed.Load())
	}
}

func TestCapabilityCallerMutationIsolated(t *testing.T) {
	b := testBackendInactive(t)
	b.mu.Lock()
	b.capability = collector.ProcessNetworkCapabilitySnapshot{
		State:       collector.ProcessNetworkCapabilityUnavailable,
		ReasonCodes: []collector.ProcessNetworkCapabilityReason{collector.ReasonReaderFailed},
	}
	b.mu.Unlock()

	snap := b.Capability()
	if len(snap.ReasonCodes) != 1 {
		t.Fatalf("%+v", snap)
	}
	snap.ReasonCodes[0] = collector.ReasonFeatureDisabled
	snap.ReasonCodes = append(snap.ReasonCodes, collector.ReasonBackendStopped)

	again := b.Capability()
	if again.ReasonCodes[0] != collector.ReasonReaderFailed {
		t.Fatalf("caller mutated backend-owned ReasonCodes: %+v", again)
	}
	if len(again.ReasonCodes) != 1 {
		t.Fatalf("caller append leaked into backend: %+v", again)
	}
}

func TestObserveAggregatorErrActiveContextErrorsFatal(t *testing.T) {
	for _, sinkErr := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(sinkErr.Error(), func(t *testing.T) {
			b := testBackendWithSink(t, &failingSink{err: sinkErr})
			ctx, cancel := armRuntime(b)
			defer cancel()
			batch := []collector.ProcessNetworkSyntheticEvent{oldSyntheticEvent(t)}

			if !b.flushBatch(ctx, ctx, &batch) {
				t.Fatalf("active runtime sink error %v must be fatal", sinkErr)
			}
			if b.AggregatorFaultCount() != 1 {
				t.Fatalf("faults=%d", b.AggregatorFaultCount())
			}
			if b.Capability().State == collector.ProcessNetworkCapabilityActive {
				t.Fatal("active sink-local context error must leave ACTIVE")
			}
			if !hasReason(b.Capability(), collector.ReasonSourceUnavailable) {
				t.Fatalf("want SOURCE_UNAVAILABLE got %+v", b.Capability())
			}
			if ctx.Err() == nil {
				t.Fatal("fatal sink-local context error must cancel runtime")
			}
		})
	}
}

func TestObserveAggregatorErrCanceledRuntimeContextErrorsNonFatal(t *testing.T) {
	for _, sinkErr := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(sinkErr.Error(), func(t *testing.T) {
			b := testBackendInactive(t)
			ctx, cancel := armRuntime(b)
			cancel()

			if b.observeAggregatorErr(ctx, sinkErr) {
				t.Fatalf("shutdown context error %v must not be fatal", sinkErr)
			}
			if b.AggregatorFaultCount() != 0 {
				t.Fatalf("faults=%d", b.AggregatorFaultCount())
			}
			if b.Capability().State != collector.ProcessNetworkCapabilityActive {
				t.Fatalf("classification alone must not rewrite capability: %+v", b.Capability())
			}
		})
	}
}

func TestStartAfterRuntimeFailureRejected(t *testing.T) {
	b := testBackendInactive(t)
	ctx, cancel := armRuntime(b)
	defer cancel()
	_ = b.handleRingReadError(ctx, errors.New("ring I/O boom"))
	if err := b.Start(context.Background()); err == nil || err.Error() != "BACKEND_RUNTIME_FAILED" {
		t.Fatalf("got %v", err)
	}
}
