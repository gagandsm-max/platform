package com.platform.core.security.authorization;

import com.platform.core.security.identity.PlatformSubject;

/**
 * Loads Platform-owned tenant scopes and permission grants for a resolved subject.
 * Client JWT claims never populate this directory.
 */
public interface PermissionGrantDirectory {
    PrincipalAuthorization load(PlatformSubject subject);

    record PrincipalAuthorization(
            java.util.Set<java.util.UUID> tenantScopes,
            java.util.Set<PermissionGrant> permissionGrants
    ) {
        public PrincipalAuthorization {
            tenantScopes = java.util.Set.copyOf(tenantScopes);
            permissionGrants = java.util.Set.copyOf(permissionGrants);
        }
    }
}
