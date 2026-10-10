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
		// No ring reader armed: this worker is not the rawCh producer. Unit
		// fixtures that enqueue on rawCh must close it via an explicit sole-
		// producer seam; production never takes the nil-reader path after Start.
		return
	}
	// Sole producer for rawCh when a reader is armed: close exactly once on
	// exit so ingestLoop can distinguish producer quiescence from an empty
	// buffer. No other goroutine may send on or close rawCh.
	defer b.closeRawCh()
	for {
		if ctx.Err() != nil || b.isStopRequested() {
			// Stop/cancel: drain remaining kernel-ring records while the reader
			// is still usable, then exit so closeRawCh runs after the final send.
			b.drainKernelRing(ctx, rd)
			return
		}
		rd.SetDeadline(time.Now().Add(200 * time.Millisecond))
		raw, err := rd.ReadRaw()
		if err != nil {
			if b.handleRingReadError(ctx, err) {
				return
			}
			continue
		}
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

// drainKernelRing reads pending kernel-ring records after production quiesces.
// It never invents unread counts: on timeout/close it records incomplete drain.
func (b *Backend) drainKernelRing(runtimeCtx context.Context, rd eventReader) {
	if rd == nil {
		return
	}
	opCtx, opCancel := b.operationContextForShutdown(runtimeCtx)
	defer opCancel()
	for {
		if opCtx.Err() != nil {
			b.noteShutdownEvidence(fmt.Errorf("%w: kernel ring drain budget exhausted", ErrBackendRingDrainIncomplete))
			return
		}
		rd.SetDeadline(time.Now().Add(50 * time.Millisecond))
		raw, err := rd.ReadRaw()
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				// After probe quiescence an idle deadline means the ring is empty.
				return
			}
			if errors.Is(err, ringbuf.ErrClosed) || errors.Is(err, os.ErrClosed) {
				b.noteShutdownEvidence(fmt.Errorf("%w: reader closed before ring drain completed", ErrBackendRingDrainIncomplete))
				return
			}
			if b.handleRingReadError(runtimeCtx, err) {
				return
			}
			continue
		}
		select {
		case b.rawCh <- raw:
		case <-opCtx.Done():
			b.noteShutdownEvidence(fmt.Errorf("%w: rawCh backpressure during kernel ring drain", ErrBackendRingDrainIncomplete))
			return
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
	if errors.Is(err, ringbuf.ErrClosed) || errors.Is(err, os.ErrClosed) || ctx.Err() != nil {
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

// operationContextForShutdown returns the governed shutdown operation context.
// Prefer StopContext's installed shutdownCtx without extending or renewing it —
// an already-expired shutdown budget must surface incomplete flush honestly.
// A separate bounded fallback is used only for genuinely independent fatal
// cleanup when no governed StopContext exists; that fallback must not be treated
// as part of a completed Agent shutdown budget.
func (b *Backend) operationContextForShutdown(runtimeCtx context.Context) (context.Context, context.CancelFunc) {
	noop := func() {}
	b.mu.Lock()
	shutdownCtx := b.shutdownCtx
	b.mu.Unlock()
	if shutdownCtx != nil {
		return shutdownCtx, noop
	}
	// Fatal path without StopContext: independent bounded cleanup only.
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

// shutdownDrainAndFlush performs governed shutdown telemetry handling after the
// sole rawCh producer has closed (or runtime cancel forced the ingest path):
// drain remaining rawCh records, ingest the local batch, reconcile final kernel
// loss after production quiescence, then FlushAll open aggregation windows.
func (b *Backend) shutdownDrainAndFlush(operationCtx context.Context, batch *[]collector.ProcessNetworkSyntheticEvent) {
	if operationCtx == nil {
		operationCtx = context.Background()
	}
	// When no reader is armed, readLoop is not the sole producer and will not
	// close rawCh. Ingest owns completion in that case (cancel-before-Stop and
	// ingest-only fixtures). When a reader is armed, only readLoop may close.
	if !b.readerArmed() {
		b.closeRawCh()
	}
	incomplete := b.drainRawChannel(operationCtx, batch)
	if incomplete {
		b.noteShutdownEvidence(fmt.Errorf("%w: buffered raw records remain after shutdown budget", ErrBackendShutdownFlush))
	}

	// Ensure producers are quiesced before the authoritative final loss snapshot.
	if err := b.quiesceKernelProducers(); err != nil {
		b.noteShutdownEvidence(fmt.Errorf("%w: %v", ErrBackendProducerQuiesceIncomplete, err))
	}

	if len(*batch) > 0 {
		toSend := *batch
		*batch = (*batch)[:0]
		if err := b.aggregator.Ingest(operationCtx, toSend); err != nil {
			b.noteShutdownEvidence(err)
			if !errors.Is(err, collector.ErrStructuredTransportNotWired) {
				_ = b.observeAggregatorErr(operationCtx, err)
				// Continue to final loss + FlushAll so incomplete evidence is maximal.
			} else {
				b.transportNotWired.Add(1)
			}
		}
	}

	// Final loss after quiescence and before aggregator FlushAll so BACKEND_DROP
	// evidence is included in the final flush.
	b.reconcileFinalKernelLoss(operationCtx, true)

	if b.aggregator != nil {
		if _, err := b.aggregator.FlushAll(operationCtx); err != nil {
			b.noteShutdownEvidence(err)
			_ = b.observeAggregatorErr(operationCtx, err)
		}
	}
}

// drainRawChannel consumes buffered raw records until the sole producer closes
// rawCh or the shutdown operation budget expires. Returns true when the budget
// expired with the channel still open or undrained records possible.
//
// Production always waits for the sole producer close. Fixtures that enqueue
// without an armed reader must close rawCh themselves (sole-producer seam).
func (b *Backend) drainRawChannel(operationCtx context.Context, batch *[]collector.ProcessNetworkSyntheticEvent) (incomplete bool) {
	if b.rawCh == nil {
		return false
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
			b.noteShutdownEvidence(err)
			if !errors.Is(err, collector.ErrStructuredTransportNotWired) {
				_ = b.observeAggregatorErr(operationCtx, err)
				return
			}
			b.transportNotWired.Add(1)
		}
	}
	if _, err := b.aggregator.FlushAll(operationCtx); err != nil {
		b.noteShutdownEvidence(err)
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
				b.noteShutdownEvidence(err)
				return b.observeAggregatorErr(runtimeCtx, err)
			}
		}
		return false
	}
	toSend := *batch
	*batch = (*batch)[:0]
	if err := b.aggregator.Ingest(operationCtx, toSend); err != nil {
		b.noteShutdownEvidence(err)
		return b.observeAggregatorErr(runtimeCtx, err)
	}
	var err error
	if shutdown {
		_, err = b.aggregator.FlushAll(operationCtx)
	} else {
		_, err = b.aggregator.FlushDue(operationCtx, time.Now().UTC())
	}
	if err != nil {
		b.noteShutdownEvidence(err)
		return b.observeAggregatorErr(runtimeCtx, err)
	}
	return false
}

// noteShutdownEvidence records shutdown flush/accounting failures so StopContext
// cannot report success after silently discarding pending records.
// TRANSPORT_NOT_WIRED remains a truthful non-delivery boundary, not a flush failure.
// Evidence is retained independently of stopRequested so fatal-before-Stop paths
// cannot be concealed by a later nil Stop.
func (b *Backend) noteShutdownEvidence(err error) {
	if err == nil || errors.Is(err, collector.ErrStructuredTransportNotWired) {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.shutdownFlushErr = errors.Join(b.shutdownFlushErr, err)
}

// noteShutdownFlushFailure is retained as the historical name used by tests and
// call sites; it forwards to noteShutdownEvidence.
func (b *Backend) noteShutdownFlushFailure(err error) {
	b.noteShutdownEvidence(err)
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
// loss after production has quiesced and before loss-map resources are released.
// finalLossDone is latched only after the snapshot attempt so post-quiescence
// increments are not silently discarded by a premature latch.
func (b *Backend) reconcileFinalKernelLoss(ctx context.Context, emitDrops bool) {
	b.mu.Lock()
	if b.finalLossDone {
		b.mu.Unlock()
		return
	}
	stopRequested := b.stopRequested
	quiesced := b.producersQuiesced
	b.mu.Unlock()

	if !quiesced {
		if err := b.quiesceKernelProducers(); err != nil && stopRequested {
			b.noteShutdownEvidence(fmt.Errorf("%w: %v", ErrBackendProducerQuiesceIncomplete, err))
		}
	}

	mapFail, ringFail, ok, present := b.snapshotKernelLoss()

	b.mu.Lock()
	b.finalLossDone = true
	b.mu.Unlock()

	if !present {
		// No loss-map resources were ever armed — nothing to reconcile.
		return
	}
	if !ok {
		// Maps were expected but a complete snapshot could not be established.
		// Never fabricate a successful zero-loss reading.
		b.noteShutdownEvidence(ErrBackendLossAccountingIncomplete)
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
