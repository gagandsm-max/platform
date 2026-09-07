package com.platform.core.domain.tenant;

import java.time.Instant;
import java.util.UUID;

public record Tenant(
        UUID tenantId,
        String name,
        TenantMode mode,
        TenantStatus status,
        String timezone,
        Instant createdAt,
        Instant updatedAt
) {}
