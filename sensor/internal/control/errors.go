package control

// Stable Sensor control error codes (BUILD-004 Slice 3).
const (
	CodeInvalidDesired          = "SENSOR_CONTROL_INVALID_DESIRED"
	CodeUnsupportedSchema       = "SENSOR_CONTROL_UNSUPPORTED_SCHEMA"
	CodeChecksumMismatch        = "SENSOR_CONTROL_CHECKSUM_MISMATCH"
	CodeCoreRegression          = "SENSOR_CONTROL_CORE_REGRESSION"
	CodeGenerationConflict      = "SENSOR_CONTROL_GENERATION_CONFLICT"
	CodeConfigInvalid           = "SENSOR_CONTROL_CONFIG_INVALID"
	CodeApplyFailed             = "SENSOR_CONTROL_APPLY_FAILED"
	CodeRestartRequired         = "SENSOR_CONTROL_RESTART_REQUIRED"
	CodeStateDurabilityFailed   = "SENSOR_CONTROL_STATE_DURABILITY_FAILED"
	CodeStateCorrupt            = "SENSOR_CONTROL_STATE_CORRUPT"
	CodeAckFailed               = "SENSOR_CONTROL_ACK_FAILED"
	CodeCoreUnavailable         = "SENSOR_CONTROL_CORE_UNAVAILABLE"
)

// MaxSafeControlGeneration is the JSON-safe control counter ceiling (2^53 - 1).
const MaxSafeControlGeneration = uint64(9007199254740991)

// SchemaVersionV1 is the only supported Sensor control schema version in Slice 3.
const SchemaVersionV1 = 1

// Outcome values for ACK / observed state.
const (
	OutcomeApplied  = "APPLIED"
	OutcomeRejected = "REJECTED"
)

// Limits mirrored from Core SensorControlDesiredLimits (operational ceilings).
const (
	MaxWorkers             = 256
	MaxQueueDepth          = 10_000
	MaxTrackedAssignments  = 50_000
	MaxTrackedJobs         = 50_000
	MaxPayloadBytes        = 4 * 1024 * 1024
	MaxExecutionTimeoutMs  = 3_600_000
	MaxShutdownGraceMs     = 600_000
	MaxRetryAttempts       = 100
	MaxRetryBackoffMs      = 3_600_000
	MaxJitterBasisPoints   = 10_000
	MaxRequestBytes        = 16 * 1024 * 1024
	MaxResponseBytes       = 16 * 1024 * 1024
	MaxUploadBatchRecords  = 10_000
	MaxUploadBatchBytes    = 16 * 1024 * 1024
	MaxPoolCount           = 6
	MaxRejectionCodeLen    = 128
	MaxRejectionMessageLen = 512
	MaxAckRequestBytes     = 4096
)

// Error is a stable coded control-plane error without secret-bearing payloads.
type Error struct {
	Code    string
	Message string
	cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func newCodedError(code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, cause: cause}
}

// NewDurabilityTestError returns a CodeStateDurabilityFailed error for tests.
func NewDurabilityTestError() error {
	return newCodedError(CodeStateDurabilityFailed, "injected durable commit failure", nil)
}

// NewRestartRequiredError returns a typed restart-required apply rejection.
func NewRestartRequiredError(message string) error {
	if message == "" {
		message = "desired configuration requires process restart"
	}
	return newCodedError(CodeRestartRequired, message, nil)
}

// NewApplyFailedError returns a typed apply failure without leaking arbitrary codes.
func NewApplyFailedError(message string, cause error) error {
	if message == "" {
		message = "runtime apply failed"
	}
	return newCodedError(CodeApplyFailed, message, cause)
}

// RejectCodeForApply maps an Applier error to the only ACK-safe rejection codes.
func RejectCodeForApply(err error) string {
	if CodeOf(err) == CodeRestartRequired {
		return CodeRestartRequired
	}
	return CodeApplyFailed
}

// CodeOf returns the stable control code when err is a control Error.
func CodeOf(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := err.(*Error); ok && e != nil {
		return e.Code
	}
	return ""
}
