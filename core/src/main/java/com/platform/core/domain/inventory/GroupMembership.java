package com.platform.core.domain.inventory;

import java.time.Instant;
import java.util.UUID;

public record GroupMembership(
        UUID tenantId,
        UUID entityId,
        UUID groupId,
        Instant createdAt
) {}
