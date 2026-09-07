package com.platform.core.security.authorization;

import java.util.Collection;
import java.util.Objects;
import java.util.Set;
import java.util.UUID;

/**
 * Scope for one permission grant. There is deliberately no global or cross-tenant wildcard.
 */
public record ResourceScope(ResourceType resourceType, Kind kind, Set<UUID> resourceIds) {
    public enum Kind {
        TENANT_WIDE,
        NAMED_RESOURCES
    }

    public ResourceScope {
        Objects.requireNonNull(resourceType, "resourceType must not be null");
        Objects.requireNonNull(kind, "kind must not be null");
        Objects.requireNonNull(resourceIds, "resourceIds must not be null");
        if (resourceIds.stream().anyMatch(Objects::isNull)) {
            throw new IllegalArgumentException("resourceIds must not contain null");
        }
        resourceIds = Set.copyOf(resourceIds);
        if (kind == Kind.TENANT_WIDE && !resourceIds.isEmpty()) {
            throw new IllegalArgumentException("tenant-wide scope must not contain resource IDs");
        }
        if (kind == Kind.NAMED_RESOURCES && resourceIds.isEmpty()) {
            throw new IllegalArgumentException("named-resource scope must contain at least one resource ID");
        }
    }

    public static ResourceScope tenantWide(ResourceType resourceType) {
        return new ResourceScope(resourceType, Kind.TENANT_WIDE, Set.of());
    }

    public static ResourceScope named(ResourceType resourceType, Collection<UUID> resourceIds) {
        Objects.requireNonNull(resourceIds, "resourceIds must not be null");
        return new ResourceScope(resourceType, Kind.NAMED_RESOURCES, Set.copyOf(resourceIds));
    }

    boolean allows(ResourceType requestedType, UUID requestedResourceId) {
        if (resourceType != requestedType) {
            return false;
        }
        if (kind == Kind.TENANT_WIDE) {
            return true;
        }
        return requestedResourceId != null && resourceIds.contains(requestedResourceId);
    }
}
