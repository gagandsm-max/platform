package com.platform.core.domain.inventory;

import java.time.Instant;
import java.util.UUID;

public record EntityAttribute(
        UUID attributeId,
        UUID tenantId,
        UUID entityId,
        String schemaKey,
        String valueJson,
        String source,
        Instant observedAt
) {}
