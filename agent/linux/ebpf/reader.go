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
	defer b.wg.Done()
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
			if errors.Is(err, os.ErrDeadlineExceeded) {
				b.pollKernelLoss()
				continue
			}
			if errors.Is(err, ringbuf.ErrClosed) || ctx.Err() != nil {
				return
			}
			b.loss.Add(LossClassDecoderReject, 1)
			b.emitDrop(LossClassDecoderReject, 1)
			continue
		}
		raw := make([]byte, len(rec.RawSample))
		copy(raw, rec.RawSample)
		select {
		case b.rawCh <- raw:
		default:
			b.loss.Add(LossClassUserspaceChanFull, 1)
			b.emitDrop(LossClassUserspaceChanFull, 1)
		}
	}
}

func (b *Backend) ingestLoop(ctx context.Context) {
	defer b.wg.Done()
	ticker := time.NewTicker(AggregationFlush)
	defer ticker.Stop()
	batch := make([]collector.ProcessNetworkSyntheticEvent, 0, IngestBatchSize)

	flush := func() {
		if len(batch) == 0 || b.aggregator == nil {
			return
		}
		_ = b.aggregator.Ingest(ctx, batch)
		batch = batch[:0]
		_, _ = b.aggregator.FlushDue(ctx, time.Now().UTC())
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-ticker.C:
			flush()
			b.pollKernelLoss()
		case raw, ok := <-b.rawCh:
			if !ok {
				flush()
				return
			}
			ev, err := decodeRawRecord(raw, b.bootWall, b.monoBaseNs)
			if err != nil {
				b.loss.Add(LossClassDecoderReject, 1)
				b.emitDrop(LossClassDecoderReject, 1)
				continue
			}
			syn, err := toSynthetic(ev)
			if err != nil {
				b.loss.Add(LossClassDecoderReject, 1)
				b.emitDrop(LossClassDecoderReject, 1)
				continue
			}
			batch = append(batch, syn)
			if len(batch) >= IngestBatchSize {
				flush()
			}
		}
	}
}

func (b *Backend) pollKernelLoss() {
	b.mu.Lock()
	handles := b.handles
	b.mu.Unlock()
	if handles == nil || handles.primary.LossCounters == nil {
		return
	}
	var key uint32
	var mapFail, ringFail uint64
	key = LossMapUpdateFail
	_ = handles.primary.LossCounters.Lookup(&key, &mapFail)
	key = LossRingReserveFail
	_ = handles.primary.LossCounters.Lookup(&key, &ringFail)

	prev := b.loss.Snapshot()
	b.loss.noteKernel(mapFail, ringFail)
	cur := b.loss.Snapshot()
	if d := cur[LossClassMapUpdateFail] - prev[LossClassMapUpdateFail]; d > 0 {
		b.emitDrop(LossClassMapUpdateFail, uint32(d))
	}
	if d := cur[LossClassRingReserveFail] - prev[LossClassRingReserveFail]; d > 0 {
		b.emitDrop(LossClassRingReserveFail, uint32(d))
	}
}

func (b *Backend) emitDrop(class LossClass, n uint32) {
	if n == 0 || b.aggregator == nil {
		return
	}
	ev := backendDropEvent(string(class), n, time.Now().UTC())
	_ = b.aggregator.Ingest(context.Background(), []collector.ProcessNetworkSyntheticEvent{ev})
}
