package com.platform.core.domain.discovery;

import java.time.Instant;
import java.util.UUID;

public record DiscoveryTarget(
        UUID targetId,
        UUID tenantId,
        UUID requestId,
        String targetAddress,
        TargetType targetType,
        String status,
        Instant createdAt,
        Instant updatedAt
) {
}
