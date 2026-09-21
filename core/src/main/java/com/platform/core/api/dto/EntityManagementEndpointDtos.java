package com.platform.core.api.dto;

import java.time.Instant;
import java.util.UUID;

public final class EntityManagementEndpointDtos {
    private EntityManagementEndpointDtos() {
    }

    public record CreateManagementEndpointRequest(
            String addressType,
            String address,
            Boolean primary,
            String source
    ) {
    }

    public record UpdateManagementEndpointRequest(
            String state,
            Boolean primary
    ) {
    }

    public record ManagementEndpointResponse(
            UUID endpointId,
            UUID tenantId,
            UUID entityId,
            String addressType,
            String address,
            String state,
            String source,
            boolean primary,
            String createdBy,
            Instant createdAt,
            String updatedBy,
            Instant updatedAt
    ) {
    }
}
