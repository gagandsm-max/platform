package com.platform.core.domain.inventory;

import java.time.Instant;
import java.util.UUID;

public record EntityGroup(
        UUID groupId,
        UUID tenantId,
        UUID parentGroupId,
        String name,
        String description,
        Instant createdAt,
        Instant updatedAt
) {}
