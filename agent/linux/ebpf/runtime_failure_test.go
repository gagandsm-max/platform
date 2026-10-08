package ebpf

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"platform/agent/common/collector"
)

func testBackendInactive(t *testing.T) *Backend {
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

func armRuntime(b *Backend) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	b.mu.Lock()
	b.cancel = cancel
	b.started = true
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
	fatal := b.flushBatch(ctx, &batch)
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
	fatal := b.flushBatch(ctx, &batch)
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
	fatal := b.flushBatch(ctx, &batch)
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
		t.Fatal("best-effort capability transition still required")
	}
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

	// Quiesced workers: Stop succeeds with nil.
	b2 := testBackendInactive(t)
	b2.mu.Lock()
	b2.started = true
	b2.stopped = false
	done := make(chan struct{})
	close(done)
	b2.workersDone = done
	b2.capability = collector.ProcessNetworkCapabilitySnapshot{State: collector.ProcessNetworkCapabilityActive}
	b2.mu.Unlock()
	if err := b2.StopContext(ctx); err != nil {
		t.Fatalf("quiesced stop: %v", err)
	}
	if err := b2.StopContext(ctx); err != nil {
		t.Fatalf("idempotent: %v", err)
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

func TestObserveAggregatorErrShutdownCancelNotFatal(t *testing.T) {
	b := testBackendInactive(t)
	_, cancel := armRuntime(b)
	defer cancel()
	if b.observeAggregatorErr(context.Canceled) {
		t.Fatal("shutdown cancel must not be fatal")
	}
	if b.AggregatorFaultCount() != 0 {
		t.Fatal()
	}
	if b.Capability().State != collector.ProcessNetworkCapabilityActive {
		t.Fatal()
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
