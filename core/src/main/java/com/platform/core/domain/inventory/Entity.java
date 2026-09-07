package com.platform.core.domain.inventory;

import java.time.Instant;
import java.util.UUID;

public record Entity(
        UUID entityId,
        UUID tenantId,
        String entityType,
        String displayName,
        String description,
        String discoveredName,
        LifecycleState lifecycleState,
        UUID siteId,
        String criticality,
        UUID ownerTeamId,
        String devicePackId,
        String devicePackVersion,
        Instant firstSeenAt,
        Instant lastSeenAt,
        String createdBy,
        Instant createdAt,
        Instant updatedAt
) {}
