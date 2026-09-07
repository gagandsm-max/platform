package com.platform.core.security.authorization;

import java.util.UUID;

/**
 * One authorization question. resourceId is optional for tenant-wide operations such as list/create.
 */
public record AuthorizationRequest(
        UUID tenantId,
        Permission permission,
        ResourceType resourceType,
        UUID resourceId
) {
}
