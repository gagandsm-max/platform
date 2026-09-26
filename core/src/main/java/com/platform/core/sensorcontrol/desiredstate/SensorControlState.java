package com.platform.core.sensorcontrol.desiredstate;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

public record SensorControlState(
        UUID tenantId,
        String sensorId,
        int desiredSchemaVersion,
        long desiredGeneration,
        String desiredChecksum,
        SensorRuntimeDesiredConfig desiredConfig,
        String desiredBy,
        Instant desiredAt,
        long appliedGeneration,
        String appliedChecksum,
        Instant appliedAt,
        long observedGeneration,
        String observedChecksum,
        SensorControlObservationOutcome observedOutcome,
        String rejectionCode,
        String rejectionMessage,
        Instant observedAt,
        Instant updatedAt
) {
    public SensorControlState {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(sensorId, "sensorId");
        Objects.requireNonNull(desiredChecksum, "desiredChecksum");
        Objects.requireNonNull(desiredConfig, "desiredConfig");
        Objects.requireNonNull(desiredBy, "desiredBy");
        Objects.requireNonNull(desiredAt, "desiredAt");
        Objects.requireNonNull(updatedAt, "updatedAt");
    }

    public SensorControlReconciliationState reconciliationState() {
        return SensorControlReconciliationState.derive(this);
    }
}
