package com.platform.core.domain.recurring;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Desired/applied recurring schedule distribution state for one Sensor.
 * Generation 0 means never mutated (no state row, or never assigned).
 */
public record RecurringState(
        UUID tenantId,
        String sensorId,
        long desiredGeneration,
        String desiredChecksum,
        long appliedGeneration,
        String appliedChecksum,
        Instant appliedAt,
        String updatedBy,
        Instant updatedAt
) {
    public RecurringState {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(sensorId, "sensorId");
        Objects.requireNonNull(updatedBy, "updatedBy");
        Objects.requireNonNull(updatedAt, "updatedAt");
        if (desiredGeneration < 0 || appliedGeneration < 0) {
            throw new IllegalArgumentException("generations must be >= 0");
        }
        if (appliedGeneration > desiredGeneration) {
            throw new IllegalArgumentException("applied generation cannot exceed desired");
        }
    }

    public static RecurringState neverMutated(UUID tenantId, String sensorId) {
        Instant epoch = Instant.EPOCH;
        return new RecurringState(
                tenantId, sensorId, 0L, null, 0L, null, null, "system", epoch);
    }
}
