package com.platform.core.security.http;

import com.platform.core.security.authorization.AuthorizationDecision;
import com.platform.core.security.authorization.AuthorizationEvaluator;
import com.platform.core.security.authorization.AuthorizationRequest;
import com.platform.core.security.authorization.Permission;
import com.platform.core.security.authorization.PlatformPermissions;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.authorization.ResourceType;
import org.springframework.http.HttpMethod;
import org.springframework.stereotype.Component;

import java.util.Locale;
import java.util.Optional;
import java.util.UUID;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * Maps business API routes to Platform AuthorizationRequest values.
 * Tenant IDs in the path are resource selectors, never proof of authority.
 */
@Component
public class AuthorizationRouteMapper {

    private static final Pattern TENANT_SCOPED = Pattern.compile(
            "^/api/v1/tenants/([0-9a-fA-F-]{36})(/.*)?$"
    );

    private final AuthorizationEvaluator evaluator = new AuthorizationEvaluator();

    public Optional<AuthorizationDecision> evaluate(PrincipalContext principal, String method, String path) {
        String normalized = normalize(path);
        Matcher tenantScoped = TENANT_SCOPED.matcher(normalized);
        if (!tenantScoped.matches()) {
            if ("/api/v1/tenants".equals(normalized)) {
                return Optional.of(evaluateTenantCollection(principal, method));
            }
            // Unknown / unclassified authenticated business path: deny by default.
            if (normalized.startsWith("/api/v1/")
                    && !normalized.startsWith("/api/v1/sensor/")
                    && !normalized.equals("/api/v1/telemetry")
                    && !normalized.equals("/api/v1/version")
                    && !normalized.startsWith("/api/v1/health/")) {
                return Optional.of(AuthorizationDecision.deny(AuthorizationDecision.DenyReason.PERMISSION_DENIED));
            }
            return Optional.empty();
        }

        UUID tenantId = UUID.fromString(tenantScoped.group(1));
        String remainder = tenantScoped.group(2) == null ? "" : tenantScoped.group(2);
        Permission permission = mapTenantScopedPermission(method, remainder);
        if (permission == null) {
            return Optional.of(AuthorizationDecision.deny(AuthorizationDecision.DenyReason.PERMISSION_DENIED));
        }
        ResourceType resourceType = com.platform.core.security.authorization.PermissionResourceBindings
                .requiredResourceType(permission);
        UUID resourceId = extractResourceId(remainder, resourceType, tenantId);
        AuthorizationRequest request = new AuthorizationRequest(tenantId, permission, resourceType, resourceId);
        return Optional.of(evaluator.authorize(principal, request));
    }

    private AuthorizationDecision evaluateTenantCollection(PrincipalContext principal, String method) {
        Permission permission;
        if (HttpMethod.POST.matches(method)) {
            permission = PlatformPermissions.TENANT_CREATE;
        } else if (HttpMethod.GET.matches(method)) {
            permission = PlatformPermissions.TENANT_READ;
        } else {
            return AuthorizationDecision.deny(AuthorizationDecision.DenyReason.PERMISSION_DENIED);
        }
        boolean allowed = principal.permissionGrants().stream()
                .anyMatch(grant -> grant.permission().equals(permission));
        if (!allowed) {
            return AuthorizationDecision.deny(AuthorizationDecision.DenyReason.PERMISSION_DENIED);
        }
        return AuthorizationDecision.allow();
    }

    private static Permission mapTenantScopedPermission(String method, String remainder) {
        boolean isGet = HttpMethod.GET.matches(method);
        boolean isPost = HttpMethod.POST.matches(method);
        boolean isPut = HttpMethod.PUT.matches(method);

        if (remainder.isEmpty() || remainder.equals("/")) {
            // GET/PUT /api/v1/tenants/{id}
            if (isGet) {
                return PlatformPermissions.TENANT_READ;
            }
            if (isPut) {
                return PlatformPermissions.TENANT_UPDATE;
            }
            return null;
        }
        if (remainder.startsWith("/sites")) {
            if (isGet) {
                return PlatformPermissions.SITE_READ;
            }
            if (isPost) {
                return PlatformPermissions.SITE_CREATE;
            }
            if (isPut) {
                return PlatformPermissions.SITE_UPDATE;
            }
            return null;
        }
        if (remainder.startsWith("/groups")) {
            if (isGet) {
                return PlatformPermissions.GROUP_READ;
            }
            if (isPost) {
                return PlatformPermissions.GROUP_CREATE;
            }
            if (isPut) {
                return PlatformPermissions.GROUP_UPDATE;
            }
            return null;
        }
        if (remainder.startsWith("/tags")) {
            if (isGet) {
                return PlatformPermissions.TAG_READ;
            }
            if (isPost) {
                return PlatformPermissions.TAG_CREATE;
            }
            return null;
        }
        if (remainder.startsWith("/entities")) {
            if (remainder.contains("/identifiers")) {
                if (isGet) {
                    return PlatformPermissions.ENTITY_IDENTIFIER_READ;
                }
                if (isPost) {
                    return PlatformPermissions.ENTITY_IDENTIFIER_CREATE;
                }
                if (HttpMethod.DELETE.matches(method)) {
                    return PlatformPermissions.ENTITY_IDENTIFIER_REMOVE;
                }
                return null;
            }
            if (remainder.contains("/groups")) {
                if (isGet) {
                    return PlatformPermissions.ENTITY_GROUP_READ;
                }
                if (isPost || isPut) {
                    return PlatformPermissions.ENTITY_GROUP_ASSIGN;
                }
                if (HttpMethod.DELETE.matches(method)) {
                    return PlatformPermissions.ENTITY_GROUP_REMOVE;
                }
                return null;
            }
            if (remainder.contains("/tags")) {
                if (isGet) {
                    return PlatformPermissions.ENTITY_TAG_READ;
                }
                if (isPost || isPut) {
                    return PlatformPermissions.ENTITY_TAG_ASSIGN;
                }
                if (HttpMethod.DELETE.matches(method)) {
                    return PlatformPermissions.ENTITY_TAG_REMOVE;
                }
                return null;
            }
            if (isGet) {
                return PlatformPermissions.ENTITY_READ;
            }
            if (isPost) {
                return PlatformPermissions.ENTITY_CREATE;
            }
            if (isPut) {
                return PlatformPermissions.ENTITY_UPDATE;
            }
            return null;
        }
        return null;
    }

    private static final Pattern ENTITY_IDENTIFIER_NAMED = Pattern.compile(
            "^/entities/[0-9a-fA-F-]{36}/identifiers/([0-9a-fA-F-]{36})$"
    );
    private static final Pattern NAMED_PARENT = Pattern.compile(
            "^/(sites|groups|tags|entities)/([0-9a-fA-F-]{36})(?:/.*)?$"
    );

    private static UUID extractResourceId(String remainder, ResourceType resourceType, UUID tenantId) {
        if (resourceType == ResourceType.TENANT) {
            return tenantId;
        }
        // ENTITY_IDENTIFIER decisions must use the identifier UUID, never the parent entity UUID.
        if (resourceType == ResourceType.ENTITY_IDENTIFIER) {
            Matcher identifier = ENTITY_IDENTIFIER_NAMED.matcher(remainder);
            if (identifier.matches()) {
                return UUID.fromString(identifier.group(1));
            }
            return null;
        }
        // /sites/{uuid}, /groups/{uuid}, /tags/{uuid}, /entities/{uuid} (parent for nested ops)
        Matcher m = NAMED_PARENT.matcher(remainder);
        if (m.matches()) {
            return UUID.fromString(m.group(2));
        }
        return null;
    }

    private static String normalize(String path) {
        if (path == null || path.isBlank()) {
            return "/";
        }
        String value = path;
        int q = value.indexOf('?');
        if (q >= 0) {
            value = value.substring(0, q);
        }
        if (value.length() > 1 && value.endsWith("/")) {
            value = value.substring(0, value.length() - 1);
        }
        return value.toLowerCase(Locale.ROOT).contains("%") ? value : value;
    }
}
