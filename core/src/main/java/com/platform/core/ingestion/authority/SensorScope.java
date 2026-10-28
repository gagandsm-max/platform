package com.platform.core.ingestion.authority;

import java.util.Collections;
import java.util.HashSet;
import java.util.Objects;
import java.util.Set;
import java.util.UUID;

/**
 * Immutable resolved authority scope for an authenticated Sensor.
 *
 * Encapsulates the authoritative binding of:
 * - Verified Sensor machine identity
 * - Assigned Tenant
 * - Assigned Site (where applicable)
 * - Explicitly allowed entity/resource IDs
 */
public record SensorScope(
        String sensorId,
        UUID tenantId,
        UUID siteId,
        Set<UUID> allowedEntityIds,
        boolean allowAllTenantEntities
) {
    public SensorScope {
        Objects.requireNonNull(sensorId, "sensorId must not be null");
        Objects.requireNonNull(tenantId, "tenantId must not be null");
        // Defensive snapshot: unmodifiableSet alone still shares the caller's mutable backing set.
        // HashSet copy preserves historical acceptance of a null element (unlike Set.copyOf).
        allowedEntityIds = allowedEntityIds != null
                ? Collections.unmodifiableSet(new HashSet<>(allowedEntityIds))
                : Collections.emptySet();
    }

    public boolean isEntityAllowed(UUID entityId) {
        if (entityId == null) {
            return false;
        }
        if (allowAllTenantEntities) {
            return true;
        }
        return allowedEntityIds.contains(entityId);
    }

    public boolean isEntityAllowed(String entityIdStr) {
        if (entityIdStr == null || entityIdStr.isBlank()) {
            return false;
        }
        try {
            UUID entityId = UUID.fromString(entityIdStr.trim());
            return isEntityAllowed(entityId);
        } catch (IllegalArgumentException e) {
            return false;
        }
    }
}
