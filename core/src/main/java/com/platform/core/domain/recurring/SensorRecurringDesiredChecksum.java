package com.platform.core.domain.recurring;

import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HexFormat;
import java.util.List;
import java.util.Objects;

/**
 * Deterministic desired-state checksum over Sensor recurring schedule assignments.
 *
 * <p>Must match Go {@code sensor/internal/recurring.ComputeDesiredChecksum}.
 *
 * <p>Assignments are sorted by {@code assignmentId} ascending. Each assignment contributes
 * length-prefixed UTF-8 fields (4-byte big-endian length + bytes) in this exact order:
 * <ol>
 *   <li>{@code assignmentId}</li>
 *   <li>{@code tenantId}</li>
 *   <li>{@code entityId}</li>
 *   <li>{@code monitorDefinitionId}</li>
 *   <li>{@code schedulingClass}</li>
 *   <li>{@code executionClass}</li>
 *   <li>{@code cadenceMs} (decimal)</li>
 *   <li>{@code freshnessMs} (decimal)</li>
 *   <li>{@code phaseVersion} (decimal)</li>
 *   <li>{@code executionTimeoutMs} (decimal)</li>
 *   <li>{@code credentialProfileRef}</li>
 *   <li>{@code protocol}</li>
 *   <li>canonical SNMP params as strings when present:
 *       {@code target}, {@code port}, {@code version}, {@code operation},
 *       each {@code oid} in order, {@code timeoutMs},
 *       {@code maxOids}, {@code maxWalkEntries}, {@code maxBulkRepetitions},
 *       {@code maxResults}, {@code maxResultBytes}</li>
 * </ol>
 * No timestamps, actors, or JSON formatting noise. Empty assignment set → {@code null}.
 */
public final class SensorRecurringDesiredChecksum {

    private SensorRecurringDesiredChecksum() {
    }

    public record AssignmentDigest(
            String assignmentId,
            String tenantId,
            String entityId,
            String monitorDefinitionId,
            String schedulingClass,
            String executionClass,
            long cadenceMs,
            long freshnessMs,
            int phaseVersion,
            long executionTimeoutMs,
            String credentialProfileRef,
            String protocol,
            SnmpParams snmp
    ) {
        public AssignmentDigest {
            Objects.requireNonNull(assignmentId, "assignmentId");
            Objects.requireNonNull(tenantId, "tenantId");
            Objects.requireNonNull(entityId, "entityId");
            Objects.requireNonNull(monitorDefinitionId, "monitorDefinitionId");
            Objects.requireNonNull(schedulingClass, "schedulingClass");
            Objects.requireNonNull(executionClass, "executionClass");
            Objects.requireNonNull(credentialProfileRef, "credentialProfileRef");
            Objects.requireNonNull(protocol, "protocol");
            // snmp may be null only for checksum parity with Go when SNMP block absent;
            // Slice A service validation requires snmp for protocol SNMP.
        }
    }

    /**
     * @return lowercase 64-hex SHA-256, or {@code null} when the assignment set is empty
     */
    public static String compute(List<AssignmentDigest> assignments) {
        Objects.requireNonNull(assignments, "assignments");
        if (assignments.isEmpty()) {
            return null;
        }
        List<AssignmentDigest> sorted = new ArrayList<>(assignments);
        sorted.sort(Comparator.comparing(AssignmentDigest::assignmentId));
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            for (AssignmentDigest row : sorted) {
                writeLengthPrefixed(digest, row.assignmentId());
                writeLengthPrefixed(digest, row.tenantId());
                writeLengthPrefixed(digest, row.entityId());
                writeLengthPrefixed(digest, row.monitorDefinitionId());
                writeLengthPrefixed(digest, row.schedulingClass());
                writeLengthPrefixed(digest, row.executionClass());
                writeLengthPrefixed(digest, Long.toString(row.cadenceMs()));
                writeLengthPrefixed(digest, Long.toString(row.freshnessMs()));
                writeLengthPrefixed(digest, Integer.toString(row.phaseVersion()));
                writeLengthPrefixed(digest, Long.toString(row.executionTimeoutMs()));
                writeLengthPrefixed(digest, row.credentialProfileRef());
                writeLengthPrefixed(digest, row.protocol());
                SnmpParams snmp = row.snmp();
                if (snmp != null) {
                    writeLengthPrefixed(digest, snmp.target());
                    writeLengthPrefixed(digest, Integer.toString(snmp.port()));
                    writeLengthPrefixed(digest, snmp.version());
                    writeLengthPrefixed(digest, snmp.operation());
                    for (String oid : snmp.oids()) {
                        writeLengthPrefixed(digest, oid);
                    }
                    writeLengthPrefixed(digest, Long.toString(snmp.timeoutMs()));
                    SnmpParams.SnmpLimits limits = snmp.limits();
                    writeLengthPrefixed(digest, Integer.toString(limits.maxOids()));
                    writeLengthPrefixed(digest, Integer.toString(limits.maxWalkEntries()));
                    writeLengthPrefixed(digest, Long.toString(limits.maxBulkRepetitions()));
                    writeLengthPrefixed(digest, Integer.toString(limits.maxResults()));
                    writeLengthPrefixed(digest, Integer.toString(limits.maxResultBytes()));
                }
            }
            return HexFormat.of().formatHex(digest.digest());
        } catch (NoSuchAlgorithmException ex) {
            throw new IllegalStateException("SHA-256 unavailable", ex);
        }
    }

    private static void writeLengthPrefixed(MessageDigest digest, String value) {
        byte[] utf8 = value.getBytes(StandardCharsets.UTF_8);
        ByteBuffer len = ByteBuffer.allocate(4);
        len.putInt(utf8.length);
        digest.update(len.array());
        digest.update(utf8);
    }
}
