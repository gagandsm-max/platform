package spool

import (
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const (
	storageWatermarkPercent = 80
	stateSchemaVersion      = 1
	recordSchemaVersion     = 1
	stateFileName           = "state.json"
	streamIDFileName        = "stream-id"
	outboundWindowFileName  = "outbound-window.json"
	quarantineDirName       = "quarantine"
	outboundWindowSchema    = 1
	maxOutboundWindowBytes  = 4096

	// MaxSafeSequence is the JSON-safe transport sequence ceiling (2^53 - 1).
	MaxSafeSequence = uint64(9007199254740991)
)

type ErrorCode string

const (
	CodeInvalidConfig   ErrorCode = "SPOOL_INVALID_CONFIG"
	CodeInvalidRecord   ErrorCode = "SPOOL_INVALID_RECORD"
	CodeClosed          ErrorCode = "SPOOL_CLOSED"
	CodeProtection      ErrorCode = "SPOOL_STORAGE_PROTECTION"
	CodeCorruption      ErrorCode = "SPOOL_CORRUPTION"
	CodeSequence        ErrorCode = "SPOOL_SEQUENCE_INVALID"
	CodeIO              ErrorCode = "SPOOL_IO"
	CodeUnsafePath      ErrorCode = "SPOOL_UNSAFE_PATH"
	CodeUnsafeLegacyAPI ErrorCode = "SPOOL_UNSAFE_LEGACY_API"
	CodeSequenceExhausted ErrorCode = "SPOOL_SEQUENCE_EXHAUSTED"
	CodeOutboundWindow    ErrorCode = "SPOOL_OUTBOUND_WINDOW"
)

type Error struct {
	Code ErrorCode
	msg  string
	err  error
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.msg }
func (e *Error) Unwrap() error { return e.err }
func newError(code ErrorCode, msg string, err error) error {
	return &Error{Code: code, msg: msg, err: err}
}
func CodeOf(err error) ErrorCode {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

type RecordClass string

const (
	ClassMetric      RecordClass = "METRIC"
	ClassEvent       RecordClass = "EVENT"
	ClassFlow        RecordClass = "FLOW"
	ClassSyslog      RecordClass = "SYSLOG"
	ClassInventory   RecordClass = "INVENTORY"
	ClassOperational RecordClass = "OPERATIONAL"
)

func validClass(c RecordClass) bool {
	switch c {
	case ClassMetric, ClassEvent, ClassFlow, ClassSyslog, ClassInventory, ClassOperational:
		return true
	}
	return false
}

type SpoolKind string

const (
	KindTelemetry    SpoolKind = "telemetry"
	KindAgentIngress SpoolKind = "agent_ingress"
)

type Config struct {
	Dir            string
	MaxRecordBytes int
	Kind           SpoolKind
	Logger         *slog.Logger
}
type AppendRequest struct {
	Class     RecordClass
	Type      string
	Payload   []byte
	CreatedAt time.Time
}
type Record struct {
	SchemaVersion int         `json:"schemaVersion"`
	Sequence      uint64      `json:"sequence"`
	ID            string      `json:"id"`
	Class         RecordClass `json:"class"`
	Type          string      `json:"type"`
	CreatedAt     time.Time   `json:"createdAt"`
	ContentLength int         `json:"contentLength"`
	Checksum      string      `json:"checksum"`
	Payload       []byte      `json:"payload"`
}
type Batch struct {
	FirstSequence uint64
	LastSequence  uint64
	Records       []Record
	PayloadBytes  int
}
type ProtectionState string

const (
	ProtectionSafe      ProtectionState = "SAFE"
	ProtectionTriggered ProtectionState = "PROTECTED"
	ProtectionDegraded  ProtectionState = "DEGRADED"
)

type Stats struct {
	NextSequence       uint64
	AckedThrough       uint64
	RecordCount        int
	SpoolBytes         int64
	OldestRecordAge    time.Duration
	FilesystemUsedPct  float64
	ProtectionState    ProtectionState
	QuarantinedRecords uint64
	CorruptionDetected bool
}
type diskState struct {
	SchemaVersion int    `json:"schemaVersion"`
	NextSequence  uint64 `json:"nextSequence"`
	AckedThrough  uint64 `json:"ackedThrough"`
}
type spaceStatus struct {
	totalBytes uint64
	usedBytes  uint64
	usedPct    float64
}
type DiskSpool struct {
	dir            string
	streamID       string
	quarantineDir  string
	maxRecordBytes int
	kind           SpoolKind
	logger         *slog.Logger
	mu             sync.Mutex
	state          diskState
	closed         bool
	degraded       bool
	// cleanupUnresolved is set when AckedThrough advanced but record removal or
	// final directory fsync did not complete. Cleared only by successful
	// ReconcileAckedCleanup (or Open recover that fully removes acked orphans).
	cleanupUnresolved bool
	quarantined       uint64
	space             func(int64) (spaceStatus, error)
	// removeFile is an optional test seam for post-ACK unlink failures. Nil uses os.Remove.
	removeFile func(string) error
}

// Transitional compatibility only until production main wiring is replaced.
type SpoolItem struct {
	ID        string          `json:"id"`
	SensorID  string          `json:"sensor_id"`
	Timestamp int64           `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}
