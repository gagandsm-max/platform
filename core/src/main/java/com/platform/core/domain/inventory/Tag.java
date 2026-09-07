package com.platform.core.domain.inventory;

import java.time.Instant;
import java.util.UUID;

public record Tag(
        UUID tagId,
        UUID tenantId,
        String tagKey,
        String tagValue,
        Instant createdAt
) {}
