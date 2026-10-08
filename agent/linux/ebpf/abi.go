package ebpf

import "time"

// Versioned fixed ABI for kernel→userspace process-network records.
const (
	ProcessNetABIV1     uint16 = 1
	ProcessNetCommLen          = 16
	ProcessNetRecordV1Size     = 88

	EventTypeInetState uint8 = 1
	EventTypeTCPRexmit uint8 = 2

	FlagOptionalProbe uint32 = 0x1

	LossMapUpdateFail      = 0
	LossRingReserveFail    = 1
	LossDecoderReject      = 2
	LossUserspaceChanFull  = 3
	LossCounterSlots       = 4

	StateMapMaxEntries = 4096
	RingBufferBytes    = 1 * 1024 * 1024
	RawRecordChannel   = 256
	IngestBatchSize    = 256
	BackendGoroutines  = 2
	AggregationFlush   = time.Second
	AggregationWindow  = 60 * time.Second

	ProgInetSockSetState = "handle_inet_sock_set_state"
	ProgTCPRetransmit    = "handle_tcp_retransmit_skb"
	MapConnState         = "conn_state"
	MapEvents            = "events"
	MapLossCounters      = "loss_counters"

	// RequestedMapBytesEstimate is an engineering safety estimate for the
	// HASH state map (key+value+per-entry overhead), not a certified SKU.
	RequestedMapBytesEstimate = uint64(StateMapMaxEntries * (8 + 16 + 64))
	RequestedRingBytesExact   = uint64(RingBufferBytes)
)

// rawEventV1 is the fixed little-endian on-wire layout. Do not unsafe-cast
// kernel bytes into this type; decoder copies field-by-field.
type rawEventV1 struct {
	ABIVersion uint16
	RecordSize uint16
	EventType  uint8
	Protocol   uint8
	Family     uint16
	TsNs       uint64
	TGID       uint32
	PID        uint32
	Sport      uint16
	Dport      uint16
	OldState   int32
	NewState   int32
	Saddr      [16]byte
	Daddr      [16]byte
	Comm       [ProcessNetCommLen]byte
	Flags      uint32
}
