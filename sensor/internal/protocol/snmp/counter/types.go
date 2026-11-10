// Package counter implements the BUILD-042 leaf engine for exact SNMP
// Counter32/Counter64 delta, wrap, reset/discontinuity and derived rate.
//
// Raw counter values remain authoritative exact unsigned integers. Derived
// delta/rate never replace or mutate the source observations. Counter64 must
// not pass through Float64 before exact delta/reset/discontinuity processing.
package counter

import "time"

// Width is the SNMP counter domain width in bits.
type Width uint8

const (
	Width32 Width = 32
	Width64 Width = 64
)

// DefaultContextID is the explicit canonical SNMP context identifier callers
// must supply for SNMPv1/v2c and the SNMPv3 default context. Empty is never a
// valid counter-stream identity.
const DefaultContextID = "DEFAULT"

// Continuity is an explicit assessment of whether the counter stream remained
// continuous between two observations. Callers supply this from reboot,
// discontinuity-marker, sysUpTime or equivalent evidence; the engine does not
// invent continuity from a raw decrease alone.
type Continuity string

const (
	ContinuityContinuous    Continuity = "CONTINUOUS"
	ContinuityResetOrReboot Continuity = "RESET_OR_REBOOT"
	ContinuityDiscontinuity Continuity = "DISCONTINUITY"
	ContinuityUnknown       Continuity = "UNKNOWN"
)

// Quality is the typed derivation outcome for delta/rate processing.
type Quality string

const (
	QualityValid                   Quality = "VALID"
	QualityResetOrReboot           Quality = "RESET_OR_REBOOT"
	QualityDiscontinuity           Quality = "DISCONTINUITY"
	QualityAmbiguousDecrease       Quality = "AMBIGUOUS_DECREASE"
	QualityUnknownContinuity       Quality = "UNKNOWN_CONTINUITY"
	QualityIdentityMismatch        Quality = "IDENTITY_MISMATCH"
	QualityInvalidIdentity         Quality = "INVALID_IDENTITY"
	QualityWidthMismatch           Quality = "WIDTH_MISMATCH"
	QualityOutOfDomain             Quality = "OUT_OF_DOMAIN"
	QualityInvalidWidth            Quality = "INVALID_WIDTH"
	QualityInvalidTime             Quality = "INVALID_TIME"
	QualityTimeIntervalOutOfRange  Quality = "TIME_INTERVAL_OUT_OF_RANGE"
	QualityInvalidContinuity       Quality = "INVALID_CONTINUITY"
)

// Identity binds a counter observation to the series it belongs to. A delta
// must not be computed across incompatible or incomplete identities.
//
// Minimum complete canonical counter-stream identity:
//   - SourceEntity (non-empty)
//   - MetricOID (non-empty)
//   - ContextID (non-empty; use DefaultContextID for default SNMP context)
//
// InterfaceID is optional only when MetricOID fully identifies a scalar
// stream; when present on either sample it must match exactly.
// ObservationGeneration, when present on either sample, must match exactly.
type Identity struct {
	MetricOID             string
	SourceEntity          string
	ContextID             string
	InterfaceID           string
	Width                 Width
	ObservationGeneration string
}

// Sample is one exact counter observation. Value is the authoritative raw
// unsigned counter; it is never rewritten by derivation.
type Sample struct {
	Identity   Identity
	Value      uint64
	ObservedAt time.Time
}

// Result holds exact source values plus optional derived delta/rate and the
// typed quality/reason explaining the outcome.
type Result struct {
	PreviousRaw uint64
	CurrentRaw  uint64
	// Delta is the exact unsigned increment when derivation succeeded.
	Delta *uint64
	// RatePerSecond is derived only after a valid exact Delta and a strictly
	// increasing observation timestamp pair whose elapsed interval is
	// representable by this package's rate contract. It may use floating-point.
	RatePerSecond *float64
	Continuity    Continuity
	Quality       Quality
	Reason        string
	Wrapped       bool
}

// MaxValue returns the inclusive maximum representable value for width, or
// false when width is not a supported counter domain.
func MaxValue(width Width) (uint64, bool) {
	switch width {
	case Width32:
		return 1<<32 - 1, true
	case Width64:
		return ^uint64(0), true
	default:
		return 0, false
	}
}
