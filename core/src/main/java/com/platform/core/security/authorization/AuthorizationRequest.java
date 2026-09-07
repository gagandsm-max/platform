package com.platform.core.security.authorization;

import java.util.Objects;
import java.util.UUID;

/**
 * One authorization question. resourceId is optional for tenant-wide operations such as list/create.
 * Required fields are validated at construction; the evaluator also fail-closes on null inputs.
 */
public record AuthorizationRequest(
        UUID tenantId,
        Permission permission,
        ResourceType resourceType,
        UUID resourceId
) {
    public AuthorizationRequest {
        Objects.requireNonNull(tenantId, "tenantId must not be null");
        Objects.requireNonNull(permission, "permission must not be null");
        Objects.requireNonNull(resourceType, "resourceType must not be null");
        PermissionResourceBindings.requireCompatible(permission, resourceType);
    }
}
