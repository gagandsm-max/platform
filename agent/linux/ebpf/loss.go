package ebpf

import (
	"sync/atomic"

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

// mergeKernelLoss reads saturating kernel ARRAY counters into userspace once.
func (l *lossCounters) noteKernel(mapUpdate, ringReserve uint64) {
	if mapUpdate > 0 {
		l.mapUpdateFail.Store(mapUpdate)
	}
	if ringReserve > 0 {
		l.ringReserveFail.Store(ringReserve)
	}
}

func lossToQuality(total uint64) collector.ProcessNetworkQualityState {
	if total == 0 {
		return collector.ProcessNetworkQualityValid
	}
	return collector.ProcessNetworkQualityPartial
}
