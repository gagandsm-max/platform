package com.platform.core.domain.inventory;

import java.time.Instant;
import java.util.UUID;

public record EntityTagAssignment(
        UUID tenantId,
        UUID entityId,
        UUID tagId,
        Instant createdAt
) {}
