package recurring

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"sort"
	"strconv"
)

// ComputeDesiredChecksum mirrors Core SensorRecurringDesiredChecksum.
// Empty definition sets yield "" (Core returns null; JSON/Go empty string).
// Definitions are sorted by AssignmentID ascending. Each row contributes
// length-prefixed UTF-8 fields in this exact order:
//
//	assignmentId, tenantId, entityId, monitorDefinitionId, schedulingClass,
//	executionClass, cadenceMs, freshnessMs, phaseVersion, executionTimeoutMs,
//	credentialProfileRef, protocol, then canonical SNMP params as strings
//	(target, port, version, operation, each oid, timeoutMs, each limit field).
//
// Output is lowercase hex SHA-256.
func ComputeDesiredChecksum(defs []RecurringDefinition) string {
	if len(defs) == 0 {
		return ""
	}
	sorted := append([]RecurringDefinition(nil), defs...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].AssignmentID < sorted[j].AssignmentID
	})
	h := sha256.New()
	var lenBuf [4]byte
	for _, row := range sorted {
		writeLengthPrefixed(h, lenBuf[:], row.AssignmentID)
		writeLengthPrefixed(h, lenBuf[:], row.TenantID)
		writeLengthPrefixed(h, lenBuf[:], row.EntityID)
		writeLengthPrefixed(h, lenBuf[:], row.MonitorDefinitionID)
		writeLengthPrefixed(h, lenBuf[:], string(row.SchedulingClass))
		writeLengthPrefixed(h, lenBuf[:], row.ExecutionClass)
		writeLengthPrefixed(h, lenBuf[:], strconv.FormatInt(row.CadenceMs, 10))
		writeLengthPrefixed(h, lenBuf[:], strconv.FormatInt(row.FreshnessMs, 10))
		writeLengthPrefixed(h, lenBuf[:], strconv.FormatUint(uint64(row.PhaseVersion), 10))
		writeLengthPrefixed(h, lenBuf[:], strconv.FormatInt(row.ExecutionTimeoutMs, 10))
		writeLengthPrefixed(h, lenBuf[:], row.CredentialProfileRef)
		writeLengthPrefixed(h, lenBuf[:], row.Protocol)
		if row.SNMP != nil {
			writeLengthPrefixed(h, lenBuf[:], row.SNMP.Target)
			writeLengthPrefixed(h, lenBuf[:], strconv.Itoa(row.SNMP.Port))
			writeLengthPrefixed(h, lenBuf[:], row.SNMP.Version)
			writeLengthPrefixed(h, lenBuf[:], row.SNMP.Operation)
			for _, oid := range row.SNMP.OIDs {
				writeLengthPrefixed(h, lenBuf[:], oid)
			}
			writeLengthPrefixed(h, lenBuf[:], strconv.FormatInt(row.SNMP.TimeoutMs, 10))
			writeLengthPrefixed(h, lenBuf[:], strconv.Itoa(row.SNMP.Limits.MaxOIDs))
			writeLengthPrefixed(h, lenBuf[:], strconv.Itoa(row.SNMP.Limits.MaxWalkEntries))
			writeLengthPrefixed(h, lenBuf[:], strconv.FormatUint(uint64(row.SNMP.Limits.MaxBulkRepetitions), 10))
			writeLengthPrefixed(h, lenBuf[:], strconv.Itoa(row.SNMP.Limits.MaxResults))
			writeLengthPrefixed(h, lenBuf[:], strconv.Itoa(row.SNMP.Limits.MaxResultBytes))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeLengthPrefixed(h hash.Hash, lenBuf []byte, value string) {
	b := []byte(value)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(b)))
	_, _ = h.Write(lenBuf)
	_, _ = h.Write(b)
}
