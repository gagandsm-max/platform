package com.platform.core.domain.capabilitypack;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Tenant Capability Pack activation history row. ACTIVE generations are monotonic per packId.
 */
public record CapabilityPackActivation(
        UUID activationId,
        UUID tenantId,
        String packId,
        String packVersion,
        long generation,
        String status,
        String activatedBy,
        Instant activatedAt,
        String revokedBy,
        Instant revokedAt,
        String correlationId
) {
    public static final String STATUS_ACTIVE = "ACTIVE";
    public static final String STATUS_REVOKED = "REVOKED";

    public CapabilityPackActivation {
        Objects.requireNonNull(activationId, "activationId");
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(packId, "packId");
        Objects.requireNonNull(packVersion, "packVersion");
        Objects.requireNonNull(status, "status");
        Objects.requireNonNull(activatedBy, "activatedBy");
        Objects.requireNonNull(activatedAt, "activatedAt");
        if (generation < 1) {
            throw new IllegalArgumentException("generation must be >= 1");
        }
    }

    public boolean active() {
        return STATUS_ACTIVE.equals(status);
    }
}
