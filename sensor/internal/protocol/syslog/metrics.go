package syslog

import "sync/atomic"

type ReceiverState string

const (
	ReceiverStopped  ReceiverState = "STOPPED"
	ReceiverStarting ReceiverState = "STARTING"
	ReceiverRunning  ReceiverState = "RUNNING"
	ReceiverDraining ReceiverState = "DRAINING"
	ReceiverFailed   ReceiverState = "FAILED"
)

type receiverMetrics struct {
	datagramsReceived atomic.Uint64
	parsedRFC3164     atomic.Uint64
	parsedRFC5424     atomic.Uint64
	parsedPartial     atomic.Uint64
	malformed         atomic.Uint64
	oversized         atomic.Uint64
	queueFullDrops    atomic.Uint64
	sinkSuccess       atomic.Uint64
	sinkFailure       atomic.Uint64
}

type ReceiverSnapshot struct {
	State             ReceiverState
	DatagramsReceived uint64
	ParsedRFC3164     uint64
	ParsedRFC5424     uint64
	ParsedPartial     uint64
	Malformed         uint64
	Oversized         uint64
	QueueFullDrops    uint64
	SinkSuccess       uint64
	SinkFailure       uint64
	QueueDepth        int
	QueueCapacity     int
	WorkerCount       int
	LastErrorCode     ErrorCode
}
