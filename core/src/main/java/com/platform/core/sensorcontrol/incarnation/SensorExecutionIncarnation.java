package com.platform.core.sensorcontrol.incarnation;

import java.time.Instant;
import java.util.UUID;

/**
 * Immutable V018 row snapshot for Core-authoritative Sensor execution incarnation.
 *
 * <p>Inert control authority only: does not authenticate a caller, authorize poll grants,
 * or establish telemetry/capture-time authority. Distinct from certificate credential
 * identity and from Sensor spool {@code bootEpoch}.
 */
public record SensorExecutionIncarnation(
        UUID incarnationId,
        UUID tenantId,
        String sensorId,
        long incarnationGeneration,
        Status status,
        Instant activatedAt,
        Instant supersededAt,
        Instant createdAt,
        Instant updatedAt) {

    public enum Status {
        ACTIVE,
        SUPERSEDED
    }
}
