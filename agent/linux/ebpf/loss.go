package ebpf

import (
	"math"
	"sync/atomic"
	"time"

	"platform/agent/common/collector"
)

// LossClass is a bounded, observable loss reason. Counted once and translated
// into ProcessNetworkEvidence quality semantics via BACKEND_DROP events.
type LossClass string

const (
	LossClassMapUpdateFail     LossClass = "MAP_UPDATE_FAIL"
	LossClassRingReserveFail   LossClass = "RING_RESERVE_FAIL"
	LossClassDecoderReject     LossClass = "DECODER_REJECT"
	LossClassUserspaceChanFull LossClass = "USERSPACE_CHANNEL_FULL"
)

// lossCounters tracks discrete backend losses. Each increment is exactly once.
type lossCounters struct {
	mapUpdateFail     atomic.Uint64
	ringReserveFail   atomic.Uint64
	decoderReject     atomic.Uint64
	userspaceChanFull atomic.Uint64
}

func (l *lossCounters) Add(class LossClass, n uint64) {
	if n == 0 {
		return
	}
	switch class {
	case LossClassMapUpdateFail:
		l.mapUpdateFail.Add(n)
	case LossClassRingReserveFail:
		l.ringReserveFail.Add(n)
	case LossClassDecoderReject:
		l.decoderReject.Add(n)
	case LossClassUserspaceChanFull:
		l.userspaceChanFull.Add(n)
	}
}

func (l *lossCounters) Snapshot() map[LossClass]uint64 {
	return map[LossClass]uint64{
		LossClassMapUpdateFail:     l.mapUpdateFail.Load(),
		LossClassRingReserveFail:   l.ringReserveFail.Load(),
		LossClassDecoderReject:     l.decoderReject.Load(),
		LossClassUserspaceChanFull: l.userspaceChanFull.Load(),
	}
}

func (l *lossCounters) Total() uint64 {
	return l.mapUpdateFail.Load() + l.ringReserveFail.Load() +
		l.decoderReject.Load() + l.userspaceChanFull.Load()
}

// reconcileKernelAbs advances absolute kernel ARRAY counters monotonically and
// returns the exact deltas to emit. Concurrent or repeated callers cannot
// double-count: only a successful CAS on an increase yields a non-zero delta.
// Decreases/resets never regress userspace counters and never unsigned-underflow.
func (l *lossCounters) reconcileKernelAbs(mapUpdate, ringReserve uint64) (mapDelta, ringDelta uint64) {
	mapDelta = advanceAbs(&l.mapUpdateFail, mapUpdate)
	ringDelta = advanceAbs(&l.ringReserveFail, ringReserve)
	return mapDelta, ringDelta
}

func advanceAbs(counter *atomic.Uint64, absolute uint64) uint64 {
	for {
		prev := counter.Load()
		if absolute <= prev {
			return 0
		}
		if counter.CompareAndSwap(prev, absolute) {
			return absolute - prev
		}
	}
}

// backendDropEvent64 preserves the exact uint64 loss total in lossCounters
// while producing one bounded event for the existing uint32 evidence schema.
// Oversized deltas saturate visibly instead of wrapping or expanding into an
// unbounded number of events.
func backendDropEvent64(reason string, n uint64, at time.Time) collector.ProcessNetworkSyntheticEvent {
	count, guarded := backendDropRepresentation(n)
	ev := backendDropEvent(reason, count, at)
	ev.ResourceGuarded = guarded
	return ev
}

func backendDropRepresentation(n uint64) (uint32, bool) {
	if n > math.MaxUint32 {
		return math.MaxUint32, true
	}
	return uint32(n), false
}

func lossToQuality(total uint64) collector.ProcessNetworkQualityState {
	if total == 0 {
		return collector.ProcessNetworkQualityValid
	}
	return collector.ProcessNetworkQualityPartial
}
