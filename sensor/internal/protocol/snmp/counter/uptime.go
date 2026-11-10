package counter

import "time"

const (
	// timeTicksPerSecond is the SNMP TimeTicks resolution (hundredths of a second).
	timeTicksPerSecond = 100
	timeTicksModulus   = uint64(1) << 32
)

// SysUpTimeInput is a caller-supplied pair of sysUpTime (TimeTicks) observations
// plus wall-clock times and an explicit tolerance. No global/customer default
// tolerance is invented by this helper.
type SysUpTimeInput struct {
	PreviousTicks uint32
	CurrentTicks  uint32
	PreviousWall  time.Time
	CurrentWall   time.Time
	// Tolerance is the maximum absolute disagreement allowed between modular
	// TimeTicks progression and wall-clock elapsed time.
	Tolerance time.Duration
}

// AssessSysUpTime compares modular TimeTicks progression with wall-clock
// elapsed time. A sysUpTime decrease is not automatically a reboot: TimeTicks
// itself can wrap. When wrap versus reboot cannot be distinguished under the
// caller-provided tolerance, ContinuityUnknown is returned.
func AssessSysUpTime(in SysUpTimeInput) (Continuity, string) {
	if !in.CurrentWall.After(in.PreviousWall) {
		return ContinuityUnknown, "wall-clock timestamps are not strictly increasing"
	}
	if in.Tolerance < 0 {
		return ContinuityUnknown, "sysUpTime tolerance must not be negative"
	}

	wallElapsed := in.CurrentWall.Sub(in.PreviousWall)
	wallTicks := uint64(wallElapsed / (time.Second / timeTicksPerSecond))
	// Truncation to hundredths matches TimeTicks resolution; leftover sub-tick
	// wall time is absorbed by the caller-provided tolerance.
	modAdvance := modularTimeTicksDelta(in.PreviousTicks, in.CurrentTicks)

	var diff uint64
	if modAdvance >= wallTicks {
		diff = modAdvance - wallTicks
	} else {
		diff = wallTicks - modAdvance
	}
	toleranceTicks := uint64(in.Tolerance / (time.Second / timeTicksPerSecond))
	if diff <= toleranceTicks {
		return ContinuityContinuous, "modular sysUpTime progression matches wall-clock elapsed time within tolerance"
	}
	return ContinuityUnknown, "sysUpTime progression does not match wall-clock elapsed time within tolerance; wrap versus reboot unresolved"
}

func modularTimeTicksDelta(previous, current uint32) uint64 {
	return (uint64(current) + timeTicksModulus - uint64(previous)) % timeTicksModulus
}
