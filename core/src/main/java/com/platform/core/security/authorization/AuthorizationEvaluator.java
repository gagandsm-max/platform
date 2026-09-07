package com.platform.core.security.authorization;

import java.util.List;
import java.util.Objects;

/**
 * Deterministic deny-by-default Platform resource authorization kernel.
 * Stateless and safe for concurrent reuse. No role, claim, or provider shortcuts.
 *
 * <p>DenyReason values are for internal audit/diagnostics. Future HTTP mapping must not
 * necessarily expose TENANT_SCOPE_DENIED vs RESOURCE_SCOPE_DENIED to callers (existence oracle).
 */
public final class AuthorizationEvaluator {

    public AuthorizationDecision authorize(PrincipalContext principal, AuthorizationRequest request) {
        if (principal == null || request == null) {
            return AuthorizationDecision.deny(AuthorizationDecision.DenyReason.INVALID_REQUEST);
        }

        // Defense in depth: records already validate, but null components must never allow.
        if (request.tenantId() == null || request.permission() == null || request.resourceType() == null) {
            return AuthorizationDecision.deny(AuthorizationDecision.DenyReason.INVALID_REQUEST);
        }

        try {
            PermissionResourceBindings.requireCompatible(request.permission(), request.resourceType());
        } catch (RuntimeException ex) {
            return AuthorizationDecision.deny(AuthorizationDecision.DenyReason.INVALID_REQUEST);
        }

        if (principal.authenticationState() != AuthenticationState.AUTHENTICATED) {
            return AuthorizationDecision.deny(AuthorizationDecision.DenyReason.NOT_AUTHENTICATED);
        }

        if (!principal.tenantScopes().contains(request.tenantId())) {
            return AuthorizationDecision.deny(AuthorizationDecision.DenyReason.TENANT_SCOPE_DENIED);
        }

        List<PermissionGrant> matchingGrants = principal.permissionGrants().stream()
                .filter(grant -> Objects.equals(grant.tenantId(), request.tenantId()))
                .filter(grant -> Objects.equals(grant.permission(), request.permission()))
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
