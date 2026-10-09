package collector

// ProcessNetworkSupportSnapshot is a backend-neutral, bounded operator support
// view of optional process-network enrichment counters. It is not a readiness
// input and never asserts host/network/device DOWN.
type ProcessNetworkSupportSnapshot struct {
	Present               bool
	MapUpdateFail         uint64
	RingReserveFail       uint64
	DecoderReject         uint64
	UserspaceChannelFull  uint64
	TransportNotWired     uint64
	AggregatorFaults      uint64
	ReaderFaults          uint64
	OptionalProbeAttached bool
}

// Copy returns a value copy suitable for crossing health/support boundaries.
func (s ProcessNetworkSupportSnapshot) Copy() ProcessNetworkSupportSnapshot {
	return s
}
