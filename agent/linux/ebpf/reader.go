package ebpf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"platform/agent/common/collector"

	"github.com/cilium/ebpf/ringbuf"
)

func (b *Backend) readLoop(ctx context.Context) {
	defer b.workerExit()
	rd := b.reader
	if rd == nil {
		// No ring reader armed: this worker is not the rawCh producer (unit
		// fixtures may enqueue). Do not close rawCh.
		return
	}
	// Sole producer for rawCh when a reader is armed: close exactly once on
	// exit so ingestLoop can distinguish producer quiescence from an empty
	// buffer. No other goroutine may send on or close rawCh.
	defer b.closeRawCh()
	for {
		if ctx.Err() != nil {
			return
		}
		rd.SetDeadline(time.Now().Add(200 * time.Millisecond))
		rec, err := rd.Read()
		if err != nil {
			if b.handleRingReadError(ctx, err) {
				return
			}
			continue
		}
		raw := make([]byte, len(rec.RawSample))
		copy(raw, rec.RawSample)
		select {
		case b.rawCh <- raw:
		default:
			b.loss.Add(LossClassUserspaceChanFull, 1)
			if b.emitDrop(ctx, LossClassUserspaceChanFull, 1) {
				return
			}
		}
	}
}

// handleRingReadError classifies ringbuf.Reader.Read errors.
// Returns true when the read loop must terminate.
// Unexpected I/O/runtime failures are READER_FAILED (never DECODER_REJECT).
func (b *Backend) handleRingReadError(ctx context.Context, err error) (stop bool) {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		// Kernel-loss polling is owned solely by ingestLoop.
		return false
	}
	if errors.Is(err, ringbuf.ErrClosed) || ctx.Err() != nil {
		return true
	}
	b.readerFaults.Add(1)
	b.failRuntime(collector.ReasonReaderFailed)
	return true
}

func (b *Backend) ingestLoop(ctx context.Context) {
	defer b.workerExit()
	ticker := time.NewTicker(AggregationFlush)
	defer ticker.Stop()
	batch := make([]collector.ProcessNetworkSyntheticEvent, 0, IngestBatchSize)

	for {
		select {
		case <-ctx.Done():
			opCtx, opCancel := b.operationContextForShutdown(ctx)
			b.shutdownDrainAndFlush(opCtx, &batch)
			opCancel()
			return
		case <-ticker.C:
			if b.flushBatch(ctx, ctx, &batch, false) {
				return
			}
			if b.pollKernelLoss(ctx) {
				return
			}
		case raw, ok := <-b.rawCh:
			if !ok {
				// Producer quiesced: final shutdown flush of local batch + open windows.
				// Prefer the StopContext-installed shutdownCtx; never derive the flush
				// budget from an already-canceled runtime parent (that recreates
				// cancel-before-flush loss when rawCh closes after runtime cancel).
				opCtx, opCancel := b.operationContextForShutdown(ctx)
				b.shutdownDrainAndFlush(opCtx, &batch)
				opCancel()
				return
			}
			if b.appendDecodedRaw(ctx, &batch, raw) {
				return
			}
			if len(batch) >= IngestBatchSize {
				if b.flushBatch(ctx, ctx, &batch, false) {
					return
				}
			}
		}
	}
}

func (b *Backend) shutdownFlushContext(runtimeCtx context.Context) context.Context {
	b.mu.Lock()
	shutdownCtx := b.shutdownCtx
	b.mu.Unlock()
	if shutdownCtx != nil {
		return shutdownCtx
	}
	return runtimeCtx
}

// operationContextForShutdown returns a live operation context for shutdown
// flush/drain work. Prefer StopContext's shutdownCtx. If that context is missing
// or already canceled, use an independent bounded budget so producer-close races
// cannot silently discard pending records via a canceled runtime parent.
func (b *Backend) operationContextForShutdown(runtimeCtx context.Context) (context.Context, context.CancelFunc) {
	noop := func() {}
	if op := b.shutdownFlushContext(runtimeCtx); op != nil && op.Err() == nil {
		return op, noop
	}
	if op := b.shutdownFlushContext(context.Background()); op != nil && op.Err() == nil {
		return op, noop
	}
	return context.WithTimeout(context.Background(), defaultBackendStopTimeout)
}

// appendDecodedRaw decodes one raw record into the local ingest batch.
// Returns true when a fatal runtime fault stopped the backend.
func (b *Backend) appendDecodedRaw(ctx context.Context, batch *[]collector.ProcessNetworkSyntheticEvent, raw []byte) (fatal bool) {
	ev, err := decodeRawRecord(raw, b.bootWall, b.monoBaseNs)
	if err != nil {
		b.loss.Add(LossClassDecoderReject, 1)
		return b.emitDrop(ctx, LossClassDecoderReject, 1)
	}
	syn, err := toSynthetic(ev)
	if err != nil {
		b.loss.Add(LossClassDecoderReject, 1)
		return b.emitDrop(ctx, LossClassDecoderReject, 1)
	}
	*batch = append(*batch, syn)
	return false
}

// shutdownDrainAndFlush performs governed shutdown telemetry handling:
// final kernel-loss reconcile, drain of buffered rawCh records, local-batch
// ingest, and FlushAll of current open aggregation windows. Incomplete drain
// or flush is recorded explicitly — never reported as a clean successful Stop.
func (b *Backend) shutdownDrainAndFlush(operationCtx context.Context, batch *[]collector.ProcessNetworkSyntheticEvent) {
	if operationCtx == nil {
		operationCtx = context.Background()
	}
	// Capture final map/ring loss while kernel maps remain valid.
	b.reconcileFinalKernelLoss(operationCtx, true)

	incomplete := b.drainRawChannel(operationCtx, batch)
	if incomplete {
		b.noteShutdownFlushFailure(fmt.Errorf("%w: buffered raw records remain after shutdown budget", ErrBackendShutdownFlush))
	}
	b.shutdownFlushAggregator(operationCtx, batch)
}

// drainRawChannel consumes buffered raw records until the sole producer closes
// rawCh or the shutdown operation budget expires. Returns true when the budget
// expired with the channel still open or undrained records possible.
//
// When no ring reader was armed (ingest-only unit fixtures), the channel is
// drained non-blocking only — there is no producer that will close rawCh.
// When a reader exists, readLoop is the sole closer; ingest waits for that
// close so in-flight producer sends cannot race a premature empty-channel exit.
func (b *Backend) drainRawChannel(operationCtx context.Context, batch *[]collector.ProcessNetworkSyntheticEvent) (incomplete bool) {
	if b.rawCh == nil {
		return false
	}
	b.mu.Lock()
	expectProducerClose := b.reader != nil
	b.mu.Unlock()
	if !expectProducerClose {
		for {
			select {
			case raw, ok := <-b.rawCh:
				if !ok {
					return false
				}
				_ = b.appendDecodedRaw(operationCtx, batch, raw)
				if len(*batch) >= IngestBatchSize {
					_ = b.flushBatch(operationCtx, operationCtx, batch, true)
				}
			default:
				return false
			}
		}
	}
	for {
		select {
		case raw, ok := <-b.rawCh:
			if !ok {
				return false
			}
			_ = b.appendDecodedRaw(operationCtx, batch, raw)
			if len(*batch) >= IngestBatchSize {
				_ = b.flushBatch(operationCtx, operationCtx, batch, true)
			}
		case <-operationCtx.Done():
			// Best-effort non-blocking drain of whatever remains, then mark incomplete.
			for {
				select {
				case raw, ok := <-b.rawCh:
					if !ok {
						return true
					}
					_ = b.appendDecodedRaw(operationCtx, batch, raw)
				default:
					return true
				}
			}
		}
	}
}

// shutdownFlushAggregator ingests any pending local batch and flushes all open
// aggregation windows (including the current live window). Empty local batch
// with nonempty aggregator windows still flushes.
func (b *Backend) shutdownFlushAggregator(operationCtx context.Context, batch *[]collector.ProcessNetworkSyntheticEvent) {
	if b.aggregator == nil {
		return
	}
	if len(*batch) > 0 {
		toSend := *batch
		*batch = (*batch)[:0]
		if err := b.aggregator.Ingest(operationCtx, toSend); err != nil {
			b.noteShutdownFlushFailure(err)
			if !errors.Is(err, collector.ErrStructuredTransportNotWired) {
				// Do not clear without recording outcome — batch already cleared
				// only after assigning to toSend; failure is noted above.
				_ = b.observeAggregatorErr(operationCtx, err)
				return
			}
			b.transportNotWired.Add(1)
		}
	}
	if _, err := b.aggregator.FlushAll(operationCtx); err != nil {
		b.noteShutdownFlushFailure(err)
		_ = b.observeAggregatorErr(operationCtx, err)
	}
}

// flushBatch ingests and flushes a pending batch. Returns true when a fatal
// local aggregator/runtime fault stopped the backend. operationCtx is passed to
// aggregator/sink work; runtimeCtx alone decides whether a context error is a
// lifecycle shutdown or an active-runtime fault.
// When shutdown is true, FlushAll is used so current open windows are included.
func (b *Backend) flushBatch(runtimeCtx, operationCtx context.Context, batch *[]collector.ProcessNetworkSyntheticEvent, shutdown bool) (fatal bool) {
	if b.aggregator == nil {
		return false
	}
	if len(*batch) == 0 {
		if shutdown {
			if _, err := b.aggregator.FlushAll(operationCtx); err != nil {
				b.noteShutdownFlushFailure(err)
				return b.observeAggregatorErr(runtimeCtx, err)
			}
		}
		return false
	}
	toSend := *batch
	*batch = (*batch)[:0]
	if err := b.aggregator.Ingest(operationCtx, toSend); err != nil {
		b.noteShutdownFlushFailure(err)
		return b.observeAggregatorErr(runtimeCtx, err)
	}
	var err error
	if shutdown {
		_, err = b.aggregator.FlushAll(operationCtx)
	} else {
		_, err = b.aggregator.FlushDue(operationCtx, time.Now().UTC())
	}
	if err != nil {
		b.noteShutdownFlushFailure(err)
		return b.observeAggregatorErr(runtimeCtx, err)
	}
	return false
}

// noteShutdownFlushFailure records stop-requested flush failures so StopContext
// cannot report success after silently discarding pending records. TRANSPORT_NOT_WIRED
// remains a truthful non-delivery boundary, not a flush failure.
func (b *Backend) noteShutdownFlushFailure(err error) {
	if err == nil || errors.Is(err, collector.ErrStructuredTransportNotWired) {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.stopRequested {
		return
	}
	b.shutdownFlushErr = errors.Join(b.shutdownFlushErr, err)
}

// observeAggregatorErr classifies aggregator errors.
// ErrStructuredTransportNotWired remains truthful NON-DELIVERY (not fatal).
// Context cancel/deadline during shutdown is not a runtime failure.
// Other local aggregator errors degrade the backend and cancel runtime.
func (b *Backend) observeAggregatorErr(runtimeCtx context.Context, err error) (fatal bool) {
	if err == nil {
		return false
	}
	if errors.Is(err, collector.ErrStructuredTransportNotWired) {
		b.transportNotWired.Add(1)
		return false
	}
	if (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) &&
		runtimeCtx != nil && runtimeCtx.Err() != nil {
		return false
	}
	b.aggregatorFaults.Add(1)
	b.failRuntime(collector.ReasonSourceUnavailable)
	return true
}

// pollKernelLoss is owned by ingestLoop. Loss reconciliation itself is
// monotonic/CAS-exact so accidental duplicate polling cannot fabricate counts.
// Positive reconciled kernel map/ring loss while ACTIVE latches RESOURCE_GUARDED.
// Returns true when a fatal aggregator fault stopped the backend.
func (b *Backend) pollKernelLoss(ctx context.Context) (fatal bool) {
	mapFail, ringFail, ok, present := b.snapshotKernelLoss()
	if !present || !ok {
		// Absent maps or failed lookup are not a successful zero-loss claim and
		// are not fatal on the periodic path — final reconcile handles closure.
		return false
	}
	return b.applyKernelLossSnapshot(ctx, mapFail, ringFail)
}

// reconcileFinalKernelLoss captures outstanding map-update and ring-reservation
// loss before kernel loss-map resources are released. emitDrops controls whether
// BACKEND_DROP events are ingested (ingest shutdown path) versus counter-only
// reconcile (finalize last chance).
func (b *Backend) reconcileFinalKernelLoss(ctx context.Context, emitDrops bool) {
	b.mu.Lock()
	if b.finalLossDone {
		b.mu.Unlock()
		return
	}
	b.finalLossDone = true
	stopRequested := b.stopRequested
	b.mu.Unlock()

	mapFail, ringFail, ok, present := b.snapshotKernelLoss()
	if !present {
		// No loss-map resources were ever armed — nothing to reconcile.
		return
	}
	if !ok {
		// Maps were expected but a complete snapshot could not be established.
		// Never fabricate a successful zero-loss reading.
		if stopRequested {
			b.noteShutdownFlushFailure(ErrBackendLossAccountingIncomplete)
		}
		return
	}
	if !emitDrops {
		_, _ = b.loss.reconcileKernelAbs(mapFail, ringFail)
		return
	}
	_ = b.applyKernelLossSnapshot(ctx, mapFail, ringFail)
}

// snapshotKernelLoss reads absolute kernel map/ring loss counters.
// present=false means no loss-map resources are armed (not an error).
// present=true && ok=false means a complete snapshot could not be established —
// never interpret that as a successful zero-loss reading.
func (b *Backend) snapshotKernelLoss() (mapUpdate, ringReserve uint64, ok, present bool) {
	if b.kernelLossSnapshotFn != nil {
		mapUpdate, ringReserve, ok = b.kernelLossSnapshotFn()
		return mapUpdate, ringReserve, ok, true
	}
	b.mu.Lock()
	handles := b.handles
	b.mu.Unlock()
	if handles == nil || handles.primary.LossCounters == nil {
		return 0, 0, false, false
	}
	var key uint32
	var mapFail, ringFail uint64
	key = LossMapUpdateFail
	if err := handles.primary.LossCounters.Lookup(&key, &mapFail); err != nil {
		return 0, 0, false, true
	}
	key = LossRingReserveFail
	if err := handles.primary.LossCounters.Lookup(&key, &ringFail); err != nil {
		return 0, 0, false, true
	}
	return mapFail, ringFail, true, true
}

func (b *Backend) applyKernelLossSnapshot(ctx context.Context, mapFail, ringFail uint64) (fatal bool) {
	mapDelta, ringDelta := b.loss.reconcileKernelAbs(mapFail, ringFail)
	if mapDelta > 0 || ringDelta > 0 {
		b.guardCapabilityForKernelLoss()
	}
	if mapDelta > 0 {
		if b.emitDrop(ctx, LossClassMapUpdateFail, mapDelta) {
			return true
		}
	}
	if ringDelta > 0 {
		return b.emitDrop(ctx, LossClassRingReserveFail, ringDelta)
	}
	return false
}

func (b *Backend) emitDrop(ctx context.Context, class LossClass, n uint64) (fatal bool) {
	if n == 0 || b.aggregator == nil {
		return false
	}
	ev := backendDropEvent64(string(class), n, time.Now().UTC())
	if err := b.aggregator.Ingest(ctx, []collector.ProcessNetworkSyntheticEvent{ev}); err != nil {
		return b.observeAggregatorErr(ctx, err)
	}
	return false
}
