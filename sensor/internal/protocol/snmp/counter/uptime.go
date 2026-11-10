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

// AssessSysUpTime compares TimeTicks progression with wall-clock elapsed time.
//
// A TimeTicks decrease is ambiguous: the same observation pair can be produced
// by a natural TimeTicks wrap or by a host reboot. Modular wall-clock agreement
// alone is therefore insufficient evidence of continuity. Any current < previous
// transition returns ContinuityUnknown; this leaf package does not invent
// corroborating continuity evidence and does not authorize wrap continuity from
// TimeTicks alone.
//
// Ordinary non-decreasing TimeTicks that match wall-clock elapsed time within
// the caller-provided tolerance may return ContinuityContinuous.
func AssessSysUpTime(in SysUpTimeInput) (Continuity, string) {
	if !in.CurrentWall.After(in.PreviousWall) {
		return ContinuityUnknown, "wall-clock timestamps are not strictly increasing"
	}
	if in.Tolerance < 0 {
		return ContinuityUnknown, "sysUpTime tolerance must not be negative"
	}

	// TimeTicks decrease: wrap versus reboot cannot be distinguished from
	// TimeTicks + wall-clock modular agreement alone. Fail safe to UNKNOWN.
	if in.CurrentTicks < in.PreviousTicks {
		return ContinuityUnknown, "sysUpTime TimeTicks decreased; wrap versus reboot unresolved without external continuity corroboration"
	}

	wallElapsed, ok := checkedElapsed(in.PreviousWall, in.CurrentWall)
	if !ok {
		return ContinuityUnknown, "wall-clock elapsed interval is not representable for sysUpTime comparison"
	}
	wallTicks := uint64(wallElapsed / (time.Second / timeTicksPerSecond))
	// Non-decreasing path: modular advance equals simple difference.
	modAdvance := uint64(in.CurrentTicks) - uint64(in.PreviousTicks)

	var diff uint64
	if modAdvance >= wallTicks {
		diff = modAdvance - wallTicks
	} else {
		diff = wallTicks - modAdvance
	}
	toleranceTicks := uint64(in.Tolerance / (time.Second / timeTicksPerSecond))
	if diff <= toleranceTicks {
		return ContinuityContinuous, "sysUpTime progression matches wall-clock elapsed time within tolerance"
	}
	return ContinuityUnknown, "sysUpTime progression does not match wall-clock elapsed time within tolerance"
}
