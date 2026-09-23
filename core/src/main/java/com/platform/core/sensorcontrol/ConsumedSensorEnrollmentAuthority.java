package com.platform.core.sensorcontrol;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Authoritative scope returned after a successful one-time authority consumption.
 * Contains no token material.
 */
public record ConsumedSensorEnrollmentAuthority(
        UUID authorityId,
        UUID tenantId,
        String deploymentId,
        UUID siteId,
        SensorRole role,
        Instant expiresAt,
        Instant consumedAt
) {
    public ConsumedSensorEnrollmentAuthority {
        Objects.requireNonNull(authorityId, "authorityId");
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(deploymentId, "deploymentId");
        Objects.requireNonNull(role, "role");
        Objects.requireNonNull(expiresAt, "expiresAt");
        Objects.requireNonNull(consumedAt, "consumedAt");
    }
}
