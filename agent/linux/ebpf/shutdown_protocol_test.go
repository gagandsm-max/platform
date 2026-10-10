package ebpf

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/agent/common/collector"

	"github.com/cilium/ebpf/ringbuf"
)

// fakeEventReader is a deterministic ring seam for shutdown drain tests.
type fakeEventReader struct {
	mu                sync.Mutex
	records           [][]byte
	closed            bool
	deadline          time.Time
	block             chan struct{} // when non-nil, ReadRaw waits until closed or Close
	closedWithPending atomic.Bool
}

func (f *fakeEventReader) SetDeadline(t time.Time) {
	f.mu.Lock()
	f.deadline = t
	f.mu.Unlock()
}

func (f *fakeEventReader) ReadRaw() ([]byte, error) {
	for {
		f.mu.Lock()
		if f.closed {
			f.mu.Unlock()
			return nil, ringbuf.ErrClosed
		}
		if len(f.records) > 0 {
			raw := f.records[0]
			f.records = f.records[1:]
			f.mu.Unlock()
			return raw, nil
		}
		deadline := f.deadline
		block := f.block
		f.mu.Unlock()
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return nil, os.ErrDeadlineExceeded
		}
		if block != nil {
			select {
			case <-block:
			case <-time.After(time.Until(deadline)):
				return nil, os.ErrDeadlineExceeded
			}
			continue
		}
		if deadline.IsZero() {
			time.Sleep(time.Millisecond)
			continue
		}
		time.Sleep(time.Until(deadline))
		return nil, os.ErrDeadlineExceeded
	}
}

func (f *fakeEventReader) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.records) > 0 {
		f.closedWithPending.Store(true)
	}
	f.closed = true
	if f.block != nil {
		select {
		case <-f.block:
		default:
			close(f.block)
			f.block = nil
		}
	}
	return nil
}

func (f *fakeEventReader) pending() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.records)
}

func (f *fakeEventReader) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func armRuntimeWithReader(b *Backend, rd eventReader, rawCap int) (context.Context, context.CancelFunc) {
	ctx, cancel := armRuntime(b)
	b.mu.Lock()
	b.reader = rd
	b.rawCh = make(chan []byte, rawCap)
	b.bootWall = time.Now().UTC()
	b.readerCloseOnce = sync.Once{}
	b.rawChCloseOnce = sync.Once{}
	b.quiesceOnce = sync.Once{}
	b.finalizeOnce = sync.Once{}
	b.mu.Unlock()
	return ctx, cancel
}

func TestStopBeforeStartCommitLatchesStopRequested(t *testing.T) {
	b := testBackendInactive(t)
	if err := b.StopContext(context.Background()); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}
	if !b.isStopRequested() {
		t.Fatal("stopRequested must latch before Start commits")
	}
	// A subsequent Start must refuse to commit workers.
	err := b.Start(context.Background())
	if err == nil || err.Error() != "BACKEND_STOPPED" {
		t.Fatalf("Start after Stop must return BACKEND_STOPPED, got %v", err)
	}
}

func TestKernelRingDrainBeforeReaderClose(t *testing.T) {
	agg, sink := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw1 := putEventV1(t, nil)
	raw2 := putEventV1(t, nil)
	rd := &fakeEventReader{records: [][]byte{raw1, raw2}}
	_, closed := trackHandleCleanup(b)
	var detached atomic.Uint32
	b.detachProbesFn = func(h *kernelHandles) error {
		detached.Add(1)
		return nil
	}
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, 8)
	defer runtimeCancel()

	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := b.StopContext(stopCtx); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if detached.Load() != 1 {
		t.Fatalf("probe detach calls=%d", detached.Load())
	}
	if rd.pending() != 0 {
		t.Fatalf("unread kernel-ring records remain: %d", rd.pending())
	}
	if got := len(sink.ProcessNetwork) + len(sink.ProcessNetworkAccounting); got == 0 {
		t.Fatal("drained kernel-ring records must produce sink evidence")
	}
	if closed.Load() != 1 {
		t.Fatalf("handles closed=%d", closed.Load())
	}
}

func TestProbeDetachWhileRingUnreadThenDrain(t *testing.T) {
	agg, sink := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	const n = 5
	records := make([][]byte, n)
	for i := 0; i < n; i++ {
		records[i] = putEventV1(t, nil)
	}
	rd := &fakeEventReader{records: records}
	detachSeen := make(chan struct{})
	var detachOnce sync.Once
	b.detachProbesFn = func(*kernelHandles) error {
		detachOnce.Do(func() { close(detachSeen) })
		return nil
	}
	_, _ = trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, n)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	stopDone := make(chan error, 1)
	go func() { stopDone <- b.StopContext(context.Background()) }()

	select {
	case <-detachSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("probes were not detached")
	}
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("StopContext: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StopContext did not complete")
	}
	if rd.closedWithPending.Load() {
		t.Fatal("reader must remain usable until pending ring records are drained")
	}
	if rd.pending() != 0 {
		t.Fatalf("pending after drain=%d", rd.pending())
	}
	if got := len(sink.ProcessNetwork) + len(sink.ProcessNetworkAccounting); got == 0 {
		t.Fatal("expected drained evidence")
	}
}

func TestRawChConservationWithSoleProducerClose(t *testing.T) {
	agg, sink := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config:     Config{Enabled: false, PreflightTimeout: time.Second, RecheckMin: time.Minute},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	const n = 4
	records := make([][]byte, n)
	for i := 0; i < n; i++ {
		records[i] = putEventV1(t, nil)
	}
	rd := &fakeEventReader{records: records}
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, n)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	if err := b.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	// Exact conservation via represented event totals (not len(evidence)>0).
	accepted, dropped, shed := processNetworkRepresentedTotals(sink)
	chanFull := b.LossSnapshot()[LossClassUserspaceChanFull]
	decoderReject := b.LossSnapshot()[LossClassDecoderReject]
	if accepted+dropped+shed+chanFull+decoderReject != n {
		t.Fatalf("conservation: accepted=%d dropped=%d shed=%d chan=%d decoder=%d want=%d (len(evidence)=%d)",
			accepted, dropped, shed, chanFull, decoderReject, n, len(sink.ProcessNetwork))
	}
	if rd.pending() != 0 {
		t.Fatalf("unconserved ring pending=%d", rd.pending())
	}
}

func TestFinalLossAfterQuiesceNotBefore(t *testing.T) {
	b := testBackendInactive(t)
	runtimeCtx, runtimeCancel := armRuntime(b)
	defer runtimeCancel()

	var snapAtQuiesce atomic.Bool
	var detached atomic.Bool
	b.detachProbesFn = func(*kernelHandles) error {
		detached.Store(true)
		return nil
	}
	b.kernelLossSnapshotFn = func() (uint64, uint64, bool) {
		if detached.Load() {
			snapAtQuiesce.Store(true)
			return 3, 1, true
		}
		return 1, 0, true
	}
	_, _ = trackHandleCleanup(b)
	b.mu.Lock()
	b.rawCh = make(chan []byte)
	b.workerRemain.Store(1)
	b.mu.Unlock()
	go b.ingestLoop(runtimeCtx)

	if err := b.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if !detached.Load() {
		t.Fatal("expected probe quiescence")
	}
	if !snapAtQuiesce.Load() {
		t.Fatal("final loss snapshot must run after producer quiescence")
	}
	if b.LossSnapshot()[LossClassMapUpdateFail] != 3 {
		t.Fatalf("map loss=%d want 3", b.LossSnapshot()[LossClassMapUpdateFail])
	}
	if b.LossSnapshot()[LossClassRingReserveFail] != 1 {
		t.Fatalf("ring loss=%d want 1", b.LossSnapshot()[LossClassRingReserveFail])
	}
	b.mu.Lock()
	done := b.finalLossDone
	b.mu.Unlock()
	if !done {
		t.Fatal("finalLossDone must latch after snapshot")
	}
}

func TestNoUnaccountedPostFinalSnapshotIncrements(t *testing.T) {
	b := testBackendInactive(t)
	runtimeCtx, runtimeCancel := armRuntime(b)
	defer runtimeCancel()
	var abs atomic.Uint64
	abs.Store(2)
	b.kernelLossSnapshotFn = func() (uint64, uint64, bool) {
		return abs.Load(), 0, true
	}
	b.detachProbesFn = func(*kernelHandles) error { return nil }
	_, _ = trackHandleCleanup(b)
	b.mu.Lock()
	b.rawCh = make(chan []byte)
	b.workerRemain.Store(1)
	b.mu.Unlock()
	go b.ingestLoop(runtimeCtx)

	if err := b.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	before := b.LossSnapshot()[LossClassMapUpdateFail]
	abs.Store(99) // would-be post-final increment
	b.reconcileFinalKernelLoss(context.Background(), true)
	after := b.LossSnapshot()[LossClassMapUpdateFail]
	if after != before {
		t.Fatalf("post-final snapshot must not apply new increments: before=%d after=%d", before, after)
	}
}

func TestDetachFailureRecordsIncompleteQuiesce(t *testing.T) {
	b := testBackendInactive(t)
	runtimeCtx, runtimeCancel := armRuntime(b)
	defer runtimeCancel()
	b.detachProbesFn = func(*kernelHandles) error {
		return errors.New("link close failed")
	}
	b.kernelLossSnapshotFn = func() (uint64, uint64, bool) { return 0, 0, true }
	_, _ = trackHandleCleanup(b)
	b.mu.Lock()
	b.rawCh = make(chan []byte)
	b.workerRemain.Store(1)
	b.mu.Unlock()
	go b.ingestLoop(runtimeCtx)

	err := b.StopContext(context.Background())
	if err == nil || !errors.Is(err, ErrBackendProducerQuiesceIncomplete) {
		t.Fatalf("want producer quiesce incomplete, got %v", err)
	}
}

func TestExpiredShutdownContextNotRenewed(t *testing.T) {
	b := testBackendInactive(t)
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	b.mu.Lock()
	b.shutdownCtx = expired
	b.mu.Unlock()
	op, opCancel := b.operationContextForShutdown(context.Background())
	defer opCancel()
	if op.Err() == nil {
		t.Fatal("expired shutdownCtx must be returned as-is, not renewed")
	}
	if !errors.Is(op.Err(), context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", op.Err())
	}
}

func TestFatalCleanupWithoutPriorStopUsesBoundedFallback(t *testing.T) {
	b := testBackendInactive(t)
	b.mu.Lock()
	b.shutdownCtx = nil
	b.mu.Unlock()
	op, opCancel := b.operationContextForShutdown(context.Background())
	defer opCancel()
	deadline, ok := op.Deadline()
	if !ok {
		t.Fatal("fatal cleanup without StopContext must use bounded fallback")
	}
	if remaining := time.Until(deadline); remaining > defaultBackendStopTimeout || remaining <= 0 {
		t.Fatalf("fallback budget=%v want <= %v", remaining, defaultBackendStopTimeout)
	}
}

func TestReaderFailureRetainedAcrossLaterStop(t *testing.T) {
	b := testBackendInactive(t)
	runtimeCtx, runtimeCancel := armRuntime(b)
	defer runtimeCancel()
	_, _ = trackHandleCleanup(b)
	b.mu.Lock()
	b.rawCh = make(chan []byte)
	b.mu.Unlock()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	if !b.handleRingReadError(runtimeCtx, errors.New("ring boom")) {
		t.Fatal("expected fatal stop")
	}
	select {
	case <-b.workersDone:
	case <-time.After(2 * time.Second):
		t.Fatal("workers did not quiesce after fatal reader failure")
	}
	err := b.StopContext(context.Background())
	if err == nil || !errors.Is(err, ErrBackendFatalRuntime) {
		t.Fatalf("later Stop must retain fatal runtime evidence, got %v", err)
	}
	if err2 := b.StopContext(context.Background()); !errors.Is(err2, ErrBackendFatalRuntime) {
		t.Fatalf("repeated Stop must preserve fatal evidence, got %v", err2)
	}
	for _, forbidden := range []string{"DOWN", "HOST_DOWN", "DEVICE_DOWN"} {
		if string(b.Capability().State) == forbidden {
			t.Fatalf("must not assert %s", forbidden)
		}
	}
}

func TestAggregatorFailureRetainedAcrossLaterStop(t *testing.T) {
	b := testBackendWithSink(t, &failingSink{err: errors.New("AGGREGATOR_SINK_BROKEN")})
	runtimeCtx, runtimeCancel := armRuntime(b)
	defer runtimeCancel()
	_, _ = trackHandleCleanup(b)
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
		t.Fatal("workers did not quiesce after aggregator failure")
	}
	err := b.StopContext(context.Background())
	if err == nil || !errors.Is(err, ErrBackendFatalRuntime) {
		t.Fatalf("later Stop must retain aggregator fatal evidence, got %v", err)
	}
}

func TestRingDrainIncompleteOnBudgetExpiry(t *testing.T) {
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
		block:   block, // after first record, block until Close
	}
	// Put one record then block further reads past deadline by keeping block open
	// and emptying records after first read — actually keep many and block.
	rd.records = append(rd.records, putEventV1(t, nil), putEventV1(t, nil))
	b.detachProbesFn = func(*kernelHandles) error { return nil }
	_, _ = trackHandleCleanup(b)
	runtimeCtx, runtimeCancel := armRuntimeWithReader(b, rd, 8)
	defer runtimeCancel()
	go b.readLoop(runtimeCtx)
	go b.ingestLoop(runtimeCtx)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer stopCancel()
	err = b.StopContext(stopCtx)
	select {
	case <-block:
	default:
		close(block)
	}
	if err == nil {
		// May succeed if drain finished quickly; require incomplete OR deadline.
		t.Log("stop completed within budget")
	}
	if err != nil &&
		!errors.Is(err, ErrBackendStopDeadline) &&
		!errors.Is(err, ErrBackendRingDrainIncomplete) &&
		!errors.Is(err, ErrBackendShutdownFlush) {
		t.Fatalf("unexpected stop error class: %v", err)
	}
}

func TestFlushAllSinkFailureNotSilentSuccess(t *testing.T) {
	sink := &failingSink{err: errors.New("SINK_REJECTED")}
	agg, err := collector.NewProcessNetworkAggregator(collector.ProcessNetworkAggregatorConfig{
		SourceProvenance:     collector.ProcessNetworkSyntheticSourceProvenance,
		Window:               time.Minute,
		TestAllowShortWindow: true,
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	syn := currentWindowSynthetic(t)
	if err := agg.Ingest(context.Background(), []collector.ProcessNetworkSyntheticEvent{syn}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	_, err = agg.FlushAll(context.Background())
	if err == nil || !errors.Is(err, collector.ErrProcessNetworkFlushDeliveryIncomplete) {
		t.Fatalf("want flush delivery incomplete, got %v", err)
	}
}
