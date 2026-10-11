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

func currentWindowSynthetic(t *testing.T) collector.ProcessNetworkSyntheticEvent {
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
	// Realistic current-window timestamp — do not backdate to hide FlushDue defects.
	syn.ObservedAt = time.Now().UTC()
	return syn
}

func startIngestWithBufferedRaw(t *testing.T, b *Backend, n int) (context.Context, context.CancelFunc) {
	t.Helper()
	runtimeCtx, cancel := armRuntime(b)
	b.mu.Lock()
	b.rawCh = make(chan []byte, n)
	b.bootWall = time.Now().UTC()
	b.workerRemain.Store(1)
	b.mu.Unlock()
	for i := 0; i < n; i++ {
		b.rawCh <- putEventV1(t, nil)
	}
	go b.ingestLoop(runtimeCtx)
	return runtimeCtx, cancel
}

func TestShutdownDrainsBufferedRawChannel(t *testing.T) {
	agg, sink := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, runtimeCancel := startIngestWithBufferedRaw(t, b, 3)
	defer runtimeCancel()

	// Give ingest a moment to start selecting; records must remain buffered or
	// partially batched — Stop must drain/account, not discard.
	time.Sleep(20 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := b.StopContext(stopCtx); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if got := len(sink.ProcessNetwork) + len(sink.ProcessNetworkAccounting); got == 0 {
		t.Fatal("buffered rawCh records must be drained into sink evidence on clean shutdown")
	}
}

func TestShutdownFlushesCurrentAggregationWindow(t *testing.T) {
	agg, sink := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimeCtx, runtimeCancel := armRuntime(b)
	defer runtimeCancel()
	b.mu.Lock()
	b.rawCh = make(chan []byte)
	b.bootWall = time.Now().UTC()
	b.workerRemain.Store(1)
	b.mu.Unlock()
	go b.ingestLoop(runtimeCtx)

	syn := currentWindowSynthetic(t)
	if err := b.aggregator.Ingest(runtimeCtx, []collector.ProcessNetworkSyntheticEvent{syn}); err != nil &&
		!errors.Is(err, collector.ErrStructuredTransportNotWired) {
		t.Fatalf("ingest current window: %v", err)
	}
	// Empty local batch; aggregator holds an open current window. Prior FlushDue
	// skipped it — shutdown must FlushAll.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := b.StopContext(stopCtx); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if got := len(sink.ProcessNetwork) + len(sink.ProcessNetworkAccounting); got == 0 {
		t.Fatal("current-time aggregation window must flush on clean shutdown")
	}
}

func TestShutdownEmptyBatchStillFlushesAggregatorWindows(t *testing.T) {
	agg, sink := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimeCtx, runtimeCancel := armRuntime(b)
	defer runtimeCancel()

	syn := currentWindowSynthetic(t)
	if err := b.aggregator.Ingest(runtimeCtx, []collector.ProcessNetworkSyntheticEvent{syn}); err != nil &&
		!errors.Is(err, collector.ErrStructuredTransportNotWired) {
		t.Fatalf("ingest: %v", err)
	}
	batch := []collector.ProcessNetworkSyntheticEvent{}
	if fatal := b.flushBatch(runtimeCtx, runtimeCtx, &batch, true); fatal {
		t.Fatal("shutdown flush of empty batch must not be fatal for TRANSPORT_NOT_WIRED sink")
	}
	if got := len(sink.ProcessNetwork) + len(sink.ProcessNetworkAccounting); got == 0 {
		t.Fatal("empty local batch with nonempty aggregator must still FlushAll")
	}
}

func TestStopDeadlineIncompleteFlushEvidence(t *testing.T) {
	sink := newContextBlockingSink(nil)
	b := testBackendWithSink(t, sink)
	runtimeCtx, runtimeCancel := armRuntime(b)
	defer runtimeCancel()

	syn := currentWindowSynthetic(t)
	if err := b.aggregator.Ingest(runtimeCtx, []collector.ProcessNetworkSyntheticEvent{syn}); err != nil {
		t.Fatalf("pre-stop ingest: %v", err)
	}
	b.mu.Lock()
	b.rawCh = make(chan []byte)
	b.workerRemain.Store(1)
	b.mu.Unlock()
	go b.ingestLoop(runtimeCtx)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer stopCancel()
	err := b.StopContext(stopCtx)
	if err == nil {
		t.Fatal("budget-exhausted incomplete flush must not report successful Stop")
	}
	if !errors.Is(err, ErrBackendShutdownFlush) && !errors.Is(err, ErrBackendStopDeadline) {
		t.Fatalf("want incomplete/deadline evidence, got %v", err)
	}
}

func TestFinalKernelLossReconcileBeforeMapClose(t *testing.T) {
	b := testBackendInactive(t)
	runtimeCtx, runtimeCancel := armRuntime(b)
	defer runtimeCancel()

	var snapCalls atomic.Uint32
	b.kernelLossSnapshotFn = func() (uint64, uint64, bool) {
		snapCalls.Add(1)
		// First periodic-style observation, then a larger final absolute.
		if snapCalls.Load() == 1 {
			return 2, 0, true
		}
		return 5, 1, true
	}
	_, closed := trackHandleCleanup(b)
	b.mu.Lock()
	b.rawCh = make(chan []byte)
	b.workerRemain.Store(1)
	b.mu.Unlock()
	go b.ingestLoop(runtimeCtx)

	// Production poll path: invoke via pollKernelLoss (not only helpers).
	if fatal := b.pollKernelLoss(runtimeCtx); fatal {
		t.Fatal("initial poll must not be fatal")
	}
	if b.LossSnapshot()[LossClassMapUpdateFail] != 2 {
		t.Fatalf("after poll map loss=%d", b.LossSnapshot()[LossClassMapUpdateFail])
	}
	if b.Capability().State != collector.ProcessNetworkCapabilityResourceGuarded {
		t.Fatalf("poll must latch RESOURCE_GUARDED, got %s", b.Capability().State)
	}
	if !b.Capability().Resource.Guarded {
		t.Fatal("RESOURCE_GUARDED must set Resource.Guarded=true")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := b.StopContext(stopCtx); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if closed.Load() != 1 {
		t.Fatalf("handles closed=%d", closed.Load())
	}
	if b.LossSnapshot()[LossClassMapUpdateFail] != 5 {
		t.Fatalf("final reconcile map loss=%d want 5", b.LossSnapshot()[LossClassMapUpdateFail])
	}
	if b.LossSnapshot()[LossClassRingReserveFail] != 1 {
		t.Fatalf("final reconcile ring loss=%d want 1", b.LossSnapshot()[LossClassRingReserveFail])
	}
	if snapCalls.Load() < 2 {
		t.Fatal("final shutdown reconcile must observe kernel loss after prior poll")
	}
}

func TestFinalKernelLossLookupFailureNotZeroSuccess(t *testing.T) {
	b := testBackendInactive(t)
	_, runtimeCancel := armRuntime(b)
	defer runtimeCancel()
	b.kernelLossSnapshotFn = func() (uint64, uint64, bool) {
		return 0, 0, false
	}
	b.mu.Lock()
	b.stopRequested = true
	b.mu.Unlock()
	b.reconcileFinalKernelLoss(context.Background(), true)
	b.mu.Lock()
	err := b.shutdownFlushErr
	b.mu.Unlock()
	if !errors.Is(err, ErrBackendLossAccountingIncomplete) {
		t.Fatalf("failed lookup must not claim zero-loss success: %v", err)
	}
	if b.LossSnapshot()[LossClassMapUpdateFail] != 0 {
		t.Fatal("must not fabricate loss counts from failed snapshot")
	}
}

func TestKernelLossReconcileExactUnderConcurrentObservation(t *testing.T) {
	var loss lossCounters
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for abs := uint64(1); abs <= 20; abs++ {
				_, _ = loss.reconcileKernelAbs(abs, abs)
			}
		}()
	}
	wg.Wait()
	if got := loss.Snapshot()[LossClassMapUpdateFail]; got != 20 {
		t.Fatalf("map=%d want 20", got)
	}
	if got := loss.Snapshot()[LossClassRingReserveFail]; got != 20 {
		t.Fatalf("ring=%d want 20", got)
	}
	d1, d2 := loss.reconcileKernelAbs(20, 20)
	if d1 != 0 || d2 != 0 {
		t.Fatalf("repeat deltas map=%d ring=%d", d1, d2)
	}
}

func TestPollKernelLossProductionPathGuardsResource(t *testing.T) {
	b := testBackendInactive(t)
	ctx, cancel := armRuntime(b)
	defer cancel()
	var n atomic.Uint64
	b.kernelLossSnapshotFn = func() (uint64, uint64, bool) {
		return n.Add(1), 0, true
	}
	if fatal := b.pollKernelLoss(ctx); fatal {
		t.Fatal("pollKernelLoss must not be fatal for BACKEND_DROP")
	}
	cap := b.Capability()
	if cap.State != collector.ProcessNetworkCapabilityResourceGuarded || !cap.Resource.Guarded {
		t.Fatalf("production poll path must set consistent RESOURCE_GUARDED: %+v", cap)
	}
	if b.LossSnapshot()[LossClassMapUpdateFail] != 1 {
		t.Fatalf("loss=%d", b.LossSnapshot()[LossClassMapUpdateFail])
	}
}

func TestResourceGuardedFlagConsistentOnTransition(t *testing.T) {
	b := testBackendInactive(t)
	_, cancel := armRuntime(b)
	defer cancel()
	b.guardCapabilityForKernelLoss()
	cap := b.Capability()
	if cap.State != collector.ProcessNetworkCapabilityResourceGuarded {
		t.Fatalf("state=%s", cap.State)
	}
	if !cap.Resource.Guarded {
		t.Fatal("State=RESOURCE_GUARDED requires Resource.Guarded=true")
	}
}

func TestFatalOverridesResourceGuardedConsistently(t *testing.T) {
	b := testBackendInactive(t)
	ctx, cancel := armRuntime(b)
	defer cancel()
	b.guardCapabilityForKernelLoss()
	if !b.handleRingReadError(ctx, errors.New("fatal while guarded")) {
		t.Fatal("expected stop")
	}
	cap := b.Capability()
	if cap.State != collector.ProcessNetworkCapabilityUnavailable {
		t.Fatalf("fatal must override to UNAVAILABLE, got %s", cap.State)
	}
	if !hasReason(cap, collector.ReasonReaderFailed) {
		t.Fatalf("want READER_FAILED: %+v", cap)
	}
	// Guarded evidence may remain as supportability; state must not stay RESOURCE_GUARDED.
	if cap.State == collector.ProcessNetworkCapabilityResourceGuarded && !cap.Resource.Guarded {
		t.Fatal("contradictory RESOURCE_GUARDED/Guarded=false")
	}
}

func TestStopFromResourceGuardedConsistent(t *testing.T) {
	b := testBackendInactive(t)
	runtimeCtx, runtimeCancel := armRuntime(b)
	defer runtimeCancel()
	b.guardCapabilityForKernelLoss()
	b.mu.Lock()
	b.rawCh = make(chan []byte)
	b.workerRemain.Store(1)
	b.mu.Unlock()
	go b.ingestLoop(runtimeCtx)

	if err := b.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	cap := b.Capability()
	if cap.State != collector.ProcessNetworkCapabilityUnavailable || !hasReason(cap, collector.ReasonBackendStopped) {
		t.Fatalf("want BACKEND_STOPPED got %+v", cap)
	}
	if !cap.Resource.Guarded {
		t.Fatal("stop from RESOURCE_GUARDED must retain Resource.Guarded evidence")
	}
	for _, forbidden := range []string{"DOWN", "HOST_DOWN", "DEVICE_DOWN"} {
		if string(cap.State) == forbidden {
			t.Fatalf("must not assert %s", forbidden)
		}
	}
}

func TestReadLoopClosesRawChAsSoleProducer(t *testing.T) {
	b := testBackendInactive(t)
	b.mu.Lock()
	b.rawCh = make(chan []byte)
	b.mu.Unlock()
	// Production readLoop defers closeRawCh only when a ring reader is armed.
	// Prove the sole-producer close contract without requiring a live BPF map.
	b.closeRawCh()
	select {
	case _, ok := <-b.rawCh:
		if ok {
			t.Fatal("rawCh must be closed by sole producer close path")
		}
	default:
		t.Fatal("rawCh close not observed")
	}
	// Idempotent: second close must not panic.
	b.closeRawCh()
}

func TestReadLoopNilReaderDoesNotCloseRawCh(t *testing.T) {
	b := testBackendInactive(t)
	runtimeCtx, cancel := armRuntime(b)
	defer cancel()
	b.mu.Lock()
	b.rawCh = make(chan []byte, 1)
	b.workerRemain.Store(1)
	b.mu.Unlock()
	done := make(chan struct{})
	go func() {
		b.readLoop(runtimeCtx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("nil-reader readLoop did not exit")
	}
	select {
	case b.rawCh <- []byte{1}:
		// Fixture producers may still enqueue when no reader was armed.
	default:
		t.Fatal("nil-reader readLoop must not close rawCh")
	}
}

func TestTransportNotWiredRemainsExplicitOnShutdownFlush(t *testing.T) {
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
	_, runtimeCancel := startIngestLoopWithPendingOldEvent(t, b)
	defer runtimeCancel()
	if err := b.StopContext(context.Background()); err != nil {
		t.Fatalf("TRANSPORT_NOT_WIRED shutdown must not fail Stop: %v", err)
	}
	if b.TransportNotWiredCount() == 0 {
		t.Fatal("TRANSPORT_NOT_WIRED must remain explicit")
	}
	if b.Capability().State != collector.ProcessNetworkCapabilityUnavailable {
		t.Fatalf("stopped capability=%s", b.Capability().State)
	}
}
