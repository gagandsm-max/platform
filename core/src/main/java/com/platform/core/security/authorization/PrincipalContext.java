package com.platform.core.security.authorization;

import java.util.Collection;
import java.util.Objects;
import java.util.Set;
import java.util.UUID;

/**
 * Identity-provider-agnostic principal context consumed by Platform authorization.
 * It intentionally carries no passwords, tokens, certificates, JWTs, assertions, or role names.
 *
 * <p>Lean invariant: an UNAUTHENTICATED principal must not carry tenant scopes or grants.
 * Authenticated principals may only carry grants whose tenants are in explicit tenantScopes.
 */
public final class PrincipalContext {
    public static final int MAX_SUBJECT_LENGTH = 256;

    private final String subjectId;
    private final AuthenticationState authenticationState;
    private final Set<UUID> tenantScopes;
    private final Set<PermissionGrant> permissionGrants;

    public PrincipalContext(
            String subjectId,
            AuthenticationState authenticationState,
            Collection<UUID> tenantScopes,
            Collection<PermissionGrant> permissionGrants
    ) {
        this.subjectId = validateSubject(subjectId);
        this.authenticationState = Objects.requireNonNull(authenticationState, "authenticationState must not be null");
        Objects.requireNonNull(tenantScopes, "tenantScopes must not be null");
        Objects.requireNonNull(permissionGrants, "permissionGrants must not be null");

        if (tenantScopes.stream().anyMatch(Objects::isNull)) {
            throw new IllegalArgumentException("tenantScopes must not contain null");
        }
        if (permissionGrants.stream().anyMatch(Objects::isNull)) {
            throw new IllegalArgumentException("permissionGrants must not contain null");
        }

        this.tenantScopes = Set.copyOf(tenantScopes);
        this.permissionGrants = Set.copyOf(permissionGrants);

        if (this.authenticationState == AuthenticationState.UNAUTHENTICATED) {
            if (!this.tenantScopes.isEmpty()) {
                throw new IllegalArgumentException("unauthenticated principal must not carry tenant scopes");
            }
            if (!this.permissionGrants.isEmpty()) {
                throw new IllegalArgumentException("unauthenticated principal must not carry permission grants");
            }
        }

        for (PermissionGrant grant : this.permissionGrants) {
            if (!this.tenantScopes.contains(grant.tenantId())) {
                throw new IllegalArgumentException("permission grant tenant must be in explicit tenant scopes");
            }
        }
    }

    public String subjectId() {
        return subjectId;
    }

    public AuthenticationState authenticationState() {
        return authenticationState;
    }

    public Set<UUID> tenantScopes() {
        return tenantScopes;
    }

    public Set<PermissionGrant> permissionGrants() {
        return permissionGrants;
    }

    private static String validateSubject(String subjectId) {
        Objects.requireNonNull(subjectId, "subjectId must not be null");
        if (subjectId.isBlank()) {
            throw new IllegalArgumentException("subjectId must not be blank");
        }
        if (subjectId.length() > MAX_SUBJECT_LENGTH) {
            throw new IllegalArgumentException("subjectId exceeds maximum length");
        }
        if (!subjectId.equals(subjectId.strip())) {
            throw new IllegalArgumentException("subjectId must not contain leading or trailing whitespace");
        }
        if (subjectId.codePoints().anyMatch(PrincipalContext::isDangerousCodePoint)) {
            throw new IllegalArgumentException("subjectId must not contain control or format characters");
        }
        return subjectId;
    }

    private static boolean isDangerousCodePoint(int codePoint) {
        if (Character.isISOControl(codePoint)) {
            return true;
        }
        int type = Character.getType(codePoint);
        return type == Character.FORMAT
                || type == Character.PRIVATE_USE
                || type == Character.SURROGATE
                || type == Character.LINE_SEPARATOR
                || type == Character.PARAGRAPH_SEPARATOR;
    }
}
