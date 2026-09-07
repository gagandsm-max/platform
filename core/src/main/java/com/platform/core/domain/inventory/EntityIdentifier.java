package com.platform.core.domain.inventory;

import java.math.BigDecimal;
import java.time.Instant;
import java.util.UUID;

public record EntityIdentifier(
        UUID identifierId,
        UUID tenantId,
        UUID entityId,
        String identifierType,
        String namespace,
        String identifierValue,
        BigDecimal confidence,
        Instant firstSeenAt,
        Instant lastSeenAt
) {}
