package com.platform.core.security.authorization;

import java.util.List;

/**
 * Deterministic deny-by-default Platform resource authorization kernel.
 */
public final class AuthorizationEvaluator {

    public AuthorizationDecision authorize(PrincipalContext principal, AuthorizationRequest request) {
        if (principal == null || request == null
                || request.tenantId() == null
                || request.permission() == null
                || request.resourceType() == null) {
            return AuthorizationDecision.deny(AuthorizationDecision.DenyReason.INVALID_REQUEST);
        }

        if (principal.authenticationState() != AuthenticationState.AUTHENTICATED) {
            return AuthorizationDecision.deny(AuthorizationDecision.DenyReason.NOT_AUTHENTICATED);
        }

        if (!principal.tenantScopes().contains(request.tenantId())) {
            return AuthorizationDecision.deny(AuthorizationDecision.DenyReason.TENANT_SCOPE_DENIED);
        }

        List<PermissionGrant> matchingGrants = principal.permissionGrants().stream()
                .filter(grant -> grant.tenantId().equals(request.tenantId()))
                .filter(grant -> grant.permission().equals(request.permission()))
                .toList();

        if (matchingGrants.isEmpty()) {
            return AuthorizationDecision.deny(AuthorizationDecision.DenyReason.PERMISSION_DENIED);
        }

        boolean scopeAllowed = matchingGrants.stream()
                .anyMatch(grant -> grant.resourceScope().allows(request.resourceType(), request.resourceId()));

        if (!scopeAllowed) {
            return AuthorizationDecision.deny(AuthorizationDecision.DenyReason.RESOURCE_SCOPE_DENIED);
        }

        return AuthorizationDecision.allow();
    }
}
