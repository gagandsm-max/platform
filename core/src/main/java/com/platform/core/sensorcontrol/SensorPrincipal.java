package com.platform.core.sensorcontrol;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Authoritative enrolled Sensor principal (PostgreSQL {@code platform.sensor}).
 */
public record SensorPrincipal(
        String sensorId,
        UUID tenantId,
        UUID siteId,
        SensorRole role,
        SensorLifecycleStatus status,
        boolean allowAllTenantEntities,
        Instant createdAt,
        Instant updatedAt
) {
    public SensorPrincipal {
        Objects.requireNonNull(sensorId, "sensorId");
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(role, "role");
        Objects.requireNonNull(status, "status");
        Objects.requireNonNull(createdAt, "createdAt");
        Objects.requireNonNull(updatedAt, "updatedAt");
        if (sensorId.isBlank() || sensorId.length() > 128) {
            throw new IllegalArgumentException("sensorId must be 1..128 characters");
        }
    }
}
