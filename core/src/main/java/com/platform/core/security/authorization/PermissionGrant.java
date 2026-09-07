package com.platform.core.security.authorization;

import java.util.Objects;
import java.util.UUID;

/**
 * One explicit permission grant for exactly one tenant.
 * Permission and resource-scope type must be compatible; unbound permissions cannot be granted.
 */
public record PermissionGrant(UUID tenantId, Permission permission, ResourceScope resourceScope) {
    public PermissionGrant {
        Objects.requireNonNull(tenantId, "tenantId must not be null");
        Objects.requireNonNull(permission, "permission must not be null");
        Objects.requireNonNull(resourceScope, "resourceScope must not be null");
        PermissionResourceBindings.requireCompatible(permission, resourceScope.resourceType());
    }
}
