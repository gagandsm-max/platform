package com.platform.core.security.http;

import com.platform.core.security.authorization.AuthenticationState;
import com.platform.core.security.authorization.AuthorizationDecision;
import com.platform.core.security.authorization.PermissionGrant;
import com.platform.core.security.authorization.PlatformPermissions;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.authorization.ResourceScope;
import com.platform.core.security.authorization.ResourceType;
import org.junit.jupiter.api.Test;

import java.util.Optional;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class AuthorizationRouteMapperIdentifierTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID OTHER_TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b1");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");
    private static final UUID OTHER_ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e2");
    private static final UUID IDENTIFIER = UUID.fromString("018f0e0a-0000-7000-8000-0000000000d1");
    private static final UUID OTHER_IDENTIFIER = UUID.fromString("018f0e0a-0000-7000-8000-0000000000d2");

    private final AuthorizationRouteMapper mapper = new AuthorizationRouteMapper();

    @Test
    void tenantWideIdentifierReadAllowsCollectionUnderEntity() {
        PrincipalContext principal = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_IDENTIFIER_READ,
                        ResourceScope.tenantWide(ResourceType.ENTITY)
                )
        );
        assertAllowed(principal, "GET",
                "/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/identifiers");
    }

    @Test
    void namedParentEntityGrantAllowsCollectionAndCreate() {
        PrincipalContext principal = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_IDENTIFIER_READ,
                        ResourceScope.named(ResourceType.ENTITY, Set.of(ENTITY))
                ),
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_IDENTIFIER_CREATE,
                        ResourceScope.named(ResourceType.ENTITY, Set.of(ENTITY))
                )
        );
        assertAllowed(principal, "GET",
                "/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/identifiers");
        assertAllowed(principal, "POST",
                "/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/identifiers");
    }

    @Test
    void wrongParentEntityDoesNotAuthorizeIdentifierCollection() {
        PrincipalContext principal = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_IDENTIFIER_READ,
                        ResourceScope.named(ResourceType.ENTITY, Set.of(ENTITY))
                )
        );
        assertDenied(principal, "GET",
                "/api/v1/tenants/" + TENANT + "/entities/" + OTHER_ENTITY + "/identifiers");
    }

    @Test
    void namedIdentifierGrantAllowsDeleteOfThatIdentifierOnly() {
        PrincipalContext principal = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_IDENTIFIER_REMOVE,
                        ResourceScope.named(ResourceType.ENTITY_IDENTIFIER, Set.of(IDENTIFIER))
                )
        );
        assertAllowed(principal, "DELETE",
                "/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/identifiers/" + IDENTIFIER);
        assertDenied(principal, "DELETE",
                "/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/identifiers/" + OTHER_IDENTIFIER);
    }

    @Test
    void parentEntityUuidIsNotTreatedAsIdentifierUuidForRemove() {
        PrincipalContext principal = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_IDENTIFIER_REMOVE,
                        ResourceScope.named(ResourceType.ENTITY_IDENTIFIER, Set.of(ENTITY))
                )
        );
        // Grant names the parent entity UUID as if it were an identifier — must not authorize DELETE.
        assertDenied(principal, "DELETE",
                "/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/identifiers/" + IDENTIFIER);
    }

    @Test
    void crossTenantIdentifierAccessIsDenied() {
        PrincipalContext principal = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_IDENTIFIER_REMOVE,
                        ResourceScope.named(ResourceType.ENTITY_IDENTIFIER, Set.of(IDENTIFIER))
                ),
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_IDENTIFIER_READ,
                        ResourceScope.tenantWide(ResourceType.ENTITY)
                )
        );
        assertDenied(principal, "DELETE",
                "/api/v1/tenants/" + OTHER_TENANT + "/entities/" + ENTITY + "/identifiers/" + IDENTIFIER);
        assertDenied(principal, "GET",
                "/api/v1/tenants/" + OTHER_TENANT + "/entities/" + ENTITY + "/identifiers");
    }

    @Test
    void resolveRequiresTenantWideEntityIdentifierRead() {
        PrincipalContext tenantWide = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_IDENTIFIER_READ,
                        ResourceScope.tenantWide(ResourceType.ENTITY)
                )
        );
        assertAllowed(tenantWide, "GET",
                "/api/v1/tenants/" + TENANT + "/entities/identifiers/resolve");

        PrincipalContext namedOnly = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_IDENTIFIER_READ,
                        ResourceScope.named(ResourceType.ENTITY, Set.of(ENTITY))
                )
        );
        // Resolve has no parent entity id; named-only entity grants do not authorize it.
        assertDenied(namedOnly, "GET",
                "/api/v1/tenants/" + TENANT + "/entities/identifiers/resolve");
    }

    /**
     * Post-#26: customer Discovery APIs remain unmapped. Existing tenant grants must not
     * unlock Discovery until formal discovery.* permissions are defined. Do not invent them here.
     */
    @Test
    void customerDiscoveryRoutesRemainDefaultDeniedUntilPermissionsDefined() {
        PrincipalContext principal = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.TENANT_READ,
                        ResourceScope.tenantWide(ResourceType.TENANT)
                ),
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_READ,
                        ResourceScope.tenantWide(ResourceType.ENTITY)
                )
        );
        UUID requestId = UUID.fromString("018f0e0a-0000-7000-8000-000000000321");
        assertDenied(principal, "POST", "/api/v1/tenants/" + TENANT + "/discovery");
        assertDenied(principal, "GET", "/api/v1/tenants/" + TENANT + "/discovery");
        assertDenied(principal, "GET",
                "/api/v1/tenants/" + TENANT + "/discovery/" + requestId);
        assertDenied(principal, "GET",
                "/api/v1/tenants/" + TENANT + "/discovery/" + requestId + "/candidates");
        assertDenied(principal, "GET",
                "/api/v1/tenants/" + TENANT + "/discovery/" + requestId + "/evidence");
        assertDenied(principal, "POST",
                "/api/v1/tenants/" + TENANT + "/discovery/" + requestId + "/cancel");
    }

    @Test
    void capabilityPackRoutesRequireDedicatedPermissions() {
        PrincipalContext reader = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.CAPABILITY_PACK_READ,
                        ResourceScope.tenantWide(ResourceType.TENANT)
                )
        );
        PrincipalContext manager = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.CAPABILITY_PACK_MANAGE,
                        ResourceScope.tenantWide(ResourceType.TENANT)
                )
        );
        PrincipalContext wrongTenant = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.CAPABILITY_PACK_MANAGE,
                        ResourceScope.tenantWide(ResourceType.TENANT)
                )
        );
        PrincipalContext tenantPathOnly = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.TENANT_READ,
                        ResourceScope.tenantWide(ResourceType.TENANT)
                )
        );

        String base = "/api/v1/tenants/" + TENANT + "/capability-packs";
        assertAllowed(reader, "GET", base);
        assertAllowed(reader, "GET", base + "/fixture.fictional.base/versions/1.0.0");
        assertAllowed(reader, "GET", base + "/fixture.fictional.base/active-version");
        assertAllowed(reader, "GET", base + "/assignments/sensors/sensor-01");
        assertDenied(reader, "PUT", base + "/fixture.fictional.base/versions/1.0.0");
        assertDenied(reader, "PUT", base + "/fixture.fictional.base/active-version");
        assertDenied(reader, "DELETE", base + "/fixture.fictional.base/active-version");
        assertDenied(reader, "PUT", base + "/assignments/sensors/sensor-01/fixture.pack");
        assertDenied(reader, "DELETE", base + "/assignments/sensors/sensor-01/fixture.pack");

        assertAllowed(manager, "PUT", base + "/fixture.fictional.base/versions/1.0.0");
        assertAllowed(manager, "PUT", base + "/fixture.fictional.base/active-version");
        assertAllowed(manager, "DELETE", base + "/fixture.fictional.base/active-version");
        assertAllowed(manager, "PUT", base + "/assignments/sensors/sensor-01/fixture.pack");
        assertAllowed(manager, "DELETE", base + "/assignments/sensors/sensor-01/fixture.pack");
        assertDenied(manager, "POST", base);

        assertDenied(wrongTenant, "PUT",
                "/api/v1/tenants/" + OTHER_TENANT + "/capability-packs/fixture.fictional.base/versions/1.0.0");
        assertDenied(tenantPathOnly, "GET", base);
        assertDenied(tenantPathOnly, "PUT", base + "/fixture.fictional.base/versions/1.0.0");
    }

    @Test
    void managementEndpointsUseEntityReadAndUpdate() {
        PrincipalContext reader = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_READ,
                        ResourceScope.named(ResourceType.ENTITY, Set.of(ENTITY))
                )
        );
        PrincipalContext updater = principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_UPDATE,
                        ResourceScope.named(ResourceType.ENTITY, Set.of(ENTITY))
                )
        );
        PrincipalContext noPerms = principal(TENANT);

        String base = "/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/management-endpoints";
        assertAllowed(reader, "GET", base);
        assertAllowed(reader, "GET", base + "/primary");
        assertDenied(reader, "POST", base);
        assertDenied(reader, "PUT", base + "/" + IDENTIFIER);

        assertAllowed(updater, "POST", base);
        assertAllowed(updater, "PUT", base + "/" + IDENTIFIER);
        assertDenied(updater, "GET", base);

        assertDenied(noPerms, "GET", base);
        assertDenied(noPerms, "POST", base);

        assertDenied(reader, "GET",
                "/api/v1/tenants/" + TENANT + "/entities/" + OTHER_ENTITY + "/management-endpoints");
    }

    private static PrincipalContext principal(UUID tenantScope, PermissionGrant... grants) {
        return new PrincipalContext(
                "subject",
                AuthenticationState.AUTHENTICATED,
                Set.of(tenantScope),
                Set.of(grants)
        );
    }

    private void assertAllowed(PrincipalContext principal, String method, String path) {
        Optional<AuthorizationDecision> decision = mapper.evaluate(principal, method, path);
        assertTrue(decision.isPresent());
        assertTrue(decision.get().allowed(), () -> "expected allow for " + method + " " + path
                + " got " + decision.get().denyReason());
    }

    private void assertDenied(PrincipalContext principal, String method, String path) {
        Optional<AuthorizationDecision> decision = mapper.evaluate(principal, method, path);
        assertTrue(decision.isPresent());
        assertEquals(false, decision.get().allowed(), () -> "expected deny for " + method + " " + path);
        // Concealment: caller must not learn existence via distinct allow/deny codes at this layer.
        assertTrue(
                decision.get().denyReason() == AuthorizationDecision.DenyReason.TENANT_SCOPE_DENIED
                        || decision.get().denyReason() == AuthorizationDecision.DenyReason.PERMISSION_DENIED
                        || decision.get().denyReason() == AuthorizationDecision.DenyReason.RESOURCE_SCOPE_DENIED
        );
    }
}
