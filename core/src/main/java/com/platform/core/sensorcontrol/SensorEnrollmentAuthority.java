package com.platform.core.sensorcontrol;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Persisted Sensor enrollment bootstrap authority (digest only — never plaintext token).
 */
public record SensorEnrollmentAuthority(
        UUID authorityId,
        String tokenHashSha256,
        UUID tenantId,
        String deploymentId,
        UUID siteId,
        SensorRole role,
        Instant issuedAt,
        Instant expiresAt,
        Instant consumedAt,
        Instant revokedAt,
        Instant createdAt,
        Instant updatedAt
) {
    public SensorEnrollmentAuthority {
        Objects.requireNonNull(authorityId, "authorityId");
        Objects.requireNonNull(tokenHashSha256, "tokenHashSha256");
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(deploymentId, "deploymentId");
        Objects.requireNonNull(issuedAt, "issuedAt");
        Objects.requireNonNull(expiresAt, "expiresAt");
        Objects.requireNonNull(createdAt, "createdAt");
        Objects.requireNonNull(updatedAt, "updatedAt");
    }

    public boolean isConsumed() {
        return consumedAt != null;
    }

    public boolean isRevoked() {
        return revokedAt != null;
    }

    public boolean isExpired(Instant now) {
        return !expiresAt.isAfter(now);
    }
}
