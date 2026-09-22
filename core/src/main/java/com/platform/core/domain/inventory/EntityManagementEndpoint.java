package com.platform.core.domain.inventory;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Authoritative management/poll host binding for a canonical entity.
 * Distinct from observational {@link EntityIdentifier} and discovery target addresses.
 * Never carries credentials or protocol secrets.
 */
public record EntityManagementEndpoint(
        UUID endpointId,
        UUID tenantId,
        UUID entityId,
        ManagementEndpointAddressType addressType,
        String address,
        ManagementEndpointState state,
        ManagementEndpointSource source,
        boolean primary,
        String createdBy,
        Instant createdAt,
        String updatedBy,
        Instant updatedAt
) {
    public EntityManagementEndpoint {
        Objects.requireNonNull(endpointId, "endpointId");
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(entityId, "entityId");
        Objects.requireNonNull(addressType, "addressType");
        Objects.requireNonNull(address, "address");
        Objects.requireNonNull(state, "state");
        Objects.requireNonNull(source, "source");
        Objects.requireNonNull(createdBy, "createdBy");
        Objects.requireNonNull(createdAt, "createdAt");
        Objects.requireNonNull(updatedBy, "updatedBy");
        Objects.requireNonNull(updatedAt, "updatedAt");
        if (address.isBlank()) {
            throw new IllegalArgumentException("address must not be blank");
        }
        if (primary && state != ManagementEndpointState.ACTIVE) {
            throw new IllegalArgumentException("primary endpoint must be ACTIVE");
        }
    }
}
