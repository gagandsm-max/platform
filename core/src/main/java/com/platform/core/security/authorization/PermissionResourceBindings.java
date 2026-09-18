package com.platform.core.security.authorization;

import java.util.Map;
import java.util.Objects;

/**
 * Canonical binding of Platform foundation permissions to exactly one resource type.
 * Prevents privilege confusion where a permission valid for one type authorizes another.
 */
public final class PermissionResourceBindings {
    private static final Map<Permission, ResourceType> BINDINGS = Map.ofEntries(
            Map.entry(PlatformPermissions.TENANT_READ, ResourceType.TENANT),
            Map.entry(PlatformPermissions.TENANT_CREATE, ResourceType.TENANT),
            Map.entry(PlatformPermissions.TENANT_UPDATE, ResourceType.TENANT),
            Map.entry(PlatformPermissions.SITE_READ, ResourceType.SITE),
            Map.entry(PlatformPermissions.SITE_CREATE, ResourceType.SITE),
            Map.entry(PlatformPermissions.SITE_UPDATE, ResourceType.SITE),
            Map.entry(PlatformPermissions.GROUP_READ, ResourceType.GROUP),
            Map.entry(PlatformPermissions.GROUP_CREATE, ResourceType.GROUP),
            Map.entry(PlatformPermissions.GROUP_UPDATE, ResourceType.GROUP),
            Map.entry(PlatformPermissions.TAG_READ, ResourceType.TAG),
            Map.entry(PlatformPermissions.TAG_CREATE, ResourceType.TAG),
            Map.entry(PlatformPermissions.ENTITY_READ, ResourceType.ENTITY),
            Map.entry(PlatformPermissions.ENTITY_CREATE, ResourceType.ENTITY),
            Map.entry(PlatformPermissions.ENTITY_UPDATE, ResourceType.ENTITY),
            // Collection/create under an entity authorize the parent ENTITY (same pattern as
            // entity.group.* / entity.tag.*). REMOVE targets a specific identifier identity.
            Map.entry(PlatformPermissions.ENTITY_IDENTIFIER_READ, ResourceType.ENTITY),
            Map.entry(PlatformPermissions.ENTITY_IDENTIFIER_CREATE, ResourceType.ENTITY),
            Map.entry(PlatformPermissions.ENTITY_IDENTIFIER_REMOVE, ResourceType.ENTITY_IDENTIFIER),
            Map.entry(PlatformPermissions.ENTITY_GROUP_READ, ResourceType.ENTITY),
            Map.entry(PlatformPermissions.ENTITY_GROUP_ASSIGN, ResourceType.ENTITY),
            Map.entry(PlatformPermissions.ENTITY_GROUP_REMOVE, ResourceType.ENTITY),
            Map.entry(PlatformPermissions.ENTITY_TAG_READ, ResourceType.ENTITY),
            Map.entry(PlatformPermissions.ENTITY_TAG_ASSIGN, ResourceType.ENTITY),
            Map.entry(PlatformPermissions.ENTITY_TAG_REMOVE, ResourceType.ENTITY),
            Map.entry(PlatformPermissions.CREDENTIAL_USE, ResourceType.CREDENTIAL),
            Map.entry(PlatformPermissions.CREDENTIAL_EDIT, ResourceType.CREDENTIAL),
            Map.entry(PlatformPermissions.CREDENTIAL_REVEAL, ResourceType.CREDENTIAL),
            Map.entry(PlatformPermissions.CAPABILITY_PACK_READ, ResourceType.TENANT),
            Map.entry(PlatformPermissions.CAPABILITY_PACK_MANAGE, ResourceType.TENANT)
    );

    private PermissionResourceBindings() {
    }

    public static boolean isFoundationPermission(Permission permission) {
        return BINDINGS.containsKey(Objects.requireNonNull(permission, "permission must not be null"));
    }

    public static ResourceType requiredResourceType(Permission permission) {
        Objects.requireNonNull(permission, "permission must not be null");
        ResourceType type = BINDINGS.get(permission);
        if (type == null) {
            throw new IllegalArgumentException("permission is not bound to a Platform foundation resource type");
        }
        return type;
    }

    public static void requireCompatible(Permission permission, ResourceType resourceType) {
        Objects.requireNonNull(resourceType, "resourceType must not be null");
        ResourceType required = requiredResourceType(permission);
        if (required != resourceType) {
            throw new IllegalArgumentException(
                    "permission is incompatible with resource type"
            );
        }
    }
}
