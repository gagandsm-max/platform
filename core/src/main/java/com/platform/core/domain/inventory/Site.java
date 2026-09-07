package com.platform.core.domain.inventory;

import java.time.Instant;
import java.util.UUID;

public record Site(
        UUID siteId,
        UUID tenantId,
        UUID parentSiteId,
        String name,
        String description,
        String timezone,
        Instant createdAt,
        Instant updatedAt
) {}
