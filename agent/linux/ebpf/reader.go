package ebpf

import (
	"context"
	"errors"
	"os"
	"time"

	"platform/agent/common/collector"

	"github.com/cilium/ebpf/ringbuf"
)

func (b *Backend) readLoop(ctx context.Context) {
	defer b.workerExit()
	rd := b.reader
	if rd == nil {
		return
	}
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
			b.flushBatch(ctx, b.shutdownFlushContext(ctx), &batch)
			return
		case <-ticker.C:
			if b.flushBatch(ctx, ctx, &batch) {
				return
			}
			if b.pollKernelLoss(ctx) {
				return
			}
		case raw, ok := <-b.rawCh:
			if !ok {
				flushCtx, cancel := boundedBackendStopContext(ctx)
				b.flushBatch(ctx, flushCtx, &batch)
				cancel()
				return
			}
			ev, err := decodeRawRecord(raw, b.bootWall, b.monoBaseNs)
			if err != nil {
				b.loss.Add(LossClassDecoderReject, 1)
				if b.emitDrop(ctx, LossClassDecoderReject, 1) {
					return
				}
				continue
			}
			syn, err := toSynthetic(ev)
			if err != nil {
				b.loss.Add(LossClassDecoderReject, 1)
				if b.emitDrop(ctx, LossClassDecoderReject, 1) {
					return
				}
				continue
			}
			batch = append(batch, syn)
			if len(batch) >= IngestBatchSize {
				if b.flushBatch(ctx, ctx, &batch) {
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

// flushBatch ingests and flushes a pending batch. Returns true when a fatal
// local aggregator/runtime fault stopped the backend. operationCtx is passed to
// aggregator/sink work; runtimeCtx alone decides whether a context error is a
// lifecycle shutdown or an active-runtime fault.
func (b *Backend) flushBatch(runtimeCtx, operationCtx context.Context, batch *[]collector.ProcessNetworkSyntheticEvent) (fatal bool) {
	if len(*batch) == 0 || b.aggregator == nil {
		return false
	}
	toSend := *batch
	*batch = (*batch)[:0]
	if err := b.aggregator.Ingest(operationCtx, toSend); err != nil {
		return b.observeAggregatorErr(runtimeCtx, err)
	}
	if _, err := b.aggregator.FlushDue(operationCtx, time.Now().UTC()); err != nil {
		return b.observeAggregatorErr(runtimeCtx, err)
	}
	return false
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
// Returns true when a fatal aggregator fault stopped the backend.
func (b *Backend) pollKernelLoss(ctx context.Context) (fatal bool) {
	b.mu.Lock()
	handles := b.handles
	b.mu.Unlock()
	if handles == nil || handles.primary.LossCounters == nil {
		return false
	}
	var key uint32
	var mapFail, ringFail uint64
	key = LossMapUpdateFail
	_ = handles.primary.LossCounters.Lookup(&key, &mapFail)
	key = LossRingReserveFail
	_ = handles.primary.LossCounters.Lookup(&key, &ringFail)

	mapDelta, ringDelta := b.loss.reconcileKernelAbs(mapFail, ringFail)
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
