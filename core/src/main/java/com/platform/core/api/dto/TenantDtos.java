package com.platform.core.api.dto;

import java.time.Instant;
import java.util.UUID;

public final class TenantDtos {
    private TenantDtos() {}

    public record CreateTenantRequest(String name, String mode, String status, String timezone) {}

    public record UpdateTenantRequest(String name, String mode, String status, String timezone) {}

    public record TenantResponse(
            UUID tenantId,
            String name,
            String mode,
            String status,
            String timezone,
            Instant createdAt,
            Instant updatedAt
    ) {}
}
