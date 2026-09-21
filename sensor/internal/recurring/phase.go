package recurring

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"time"
)

// ComputePhase returns the deterministic phase offset in [0, cadence).
// Inputs (length-prefixed FNV-1a 64-bit) are exactly:
//
//	phaseVersion, tenantId, assignmentId, entityId, monitorDefinitionId
//
// Sensor identity MUST NOT be included so reassignment does not reshuffle the estate.
// Result is stable across Sensor restart.
func ComputePhase(def RecurringDefinition) (time.Duration, error) {
	if def.CadenceMs < MinCadenceMs {
		return 0, fmt.Errorf("%s: cadenceMs required for phase", ErrCodeInvalidDesired)
	}
	cadence := time.Duration(def.CadenceMs) * time.Millisecond
	cadenceNs := uint64(cadence.Nanoseconds())
	if cadenceNs == 0 {
		return 0, fmt.Errorf("%s: cadence nanoseconds underflow", ErrCodeInvalidDesired)
	}

	h := fnv.New64a()
	var lenBuf [4]byte
	writeLengthPrefixed(h, lenBuf[:], strconv.FormatUint(uint64(def.PhaseVersion), 10))
	writeLengthPrefixed(h, lenBuf[:], def.TenantID)
	writeLengthPrefixed(h, lenBuf[:], def.AssignmentID)
	writeLengthPrefixed(h, lenBuf[:], def.EntityID)
	writeLengthPrefixed(h, lenBuf[:], def.MonitorDefinitionID)

	phaseNs := h.Sum64() % cadenceNs
	return time.Duration(phaseNs), nil
}
