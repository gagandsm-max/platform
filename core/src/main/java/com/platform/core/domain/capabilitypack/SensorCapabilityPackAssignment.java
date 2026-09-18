package com.platform.core.domain.capabilitypack;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/** Desired Sensor Capability Pack assignment row (no manifest bytes). */
public record SensorCapabilityPackAssignment(
        UUID tenantId,
        String sensorId,
        String packId,
        String packVersion,
        String assignedBy,
        Instant assignedAt
) {
    public SensorCapabilityPackAssignment {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(sensorId, "sensorId");
        Objects.requireNonNull(packId, "packId");
        Objects.requireNonNull(packVersion, "packVersion");
        Objects.requireNonNull(assignedBy, "assignedBy");
        Objects.requireNonNull(assignedAt, "assignedAt");
    }
}
