package com.platform.core.domain.recurring;

import java.util.List;
import java.util.Objects;

/** Non-secret SNMP collection parameters mirrored by Sensor recurring.SNMPParams. */
public record SnmpParams(
        String target,
        int port,
        String version,
        String operation,
        List<String> oids,
        long timeoutMs,
        SnmpLimits limits
) {
    public SnmpParams {
        Objects.requireNonNull(target, "target");
        Objects.requireNonNull(version, "version");
        Objects.requireNonNull(operation, "operation");
        Objects.requireNonNull(oids, "oids");
        oids = List.copyOf(oids);
        Objects.requireNonNull(limits, "limits");
    }

    public record SnmpLimits(
            int maxOids,
            int maxWalkEntries,
            long maxBulkRepetitions,
            int maxResults,
            int maxResultBytes
    ) {
    }
}
