package com.platform.core.security.authorization;

import org.junit.jupiter.api.Test;

import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class AuthorizationEvaluatorTest {
    private final AuthorizationEvaluator evaluator = new AuthorizationEvaluator();
    private final UUID tenantA = UUID.fromString("10000000-0000-0000-0000-000000000001");
    private final UUID tenantB = UUID.fromString("20000000-0000-0000-0000-000000000002");
    private final UUID resourceA = UUID.fromString("30000000-0000-0000-0000-000000000003");
    private final UUID resourceB = UUID.fromString("40000000-0000-0000-0000-000000000004");

    @Test
    void authenticatedMatchingTenantAndPermissionAllows() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.tenantWide(ResourceType.ENTITY))
        ));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)));
    }

    @Test
    void unauthenticatedPrincipalIsDenied() {
        PrincipalContext principal = new PrincipalContext(
                "subject-1", AuthenticationState.UNAUTHENTICATED, Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.tenantWide(ResourceType.ENTITY))
        ));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.NOT_AUTHENTICATED);
    }

    @Test
    void tenantBoundaryDeniesCrossTenantRequest() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.tenantWide(ResourceType.ENTITY))
        ));
        assertDeny(evaluator.authorize(principal,
                request(tenantB, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.TENANT_SCOPE_DENIED);
    }

    @Test
    void sameResourceUuidAcrossTenantsDoesNotBypassTenantBoundary() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.named(ResourceType.ENTITY, Set.of(resourceA)))
        ));
        assertDeny(evaluator.authorize(principal,
                request(tenantB, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.TENANT_SCOPE_DENIED);
    }

    @Test
    void missingPermissionIsDenied() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of());
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
    }

    @Test
    void unknownAndMalformedPermissionsAreRejectedSafely() {
        Permission unknown = Permission.of("entity.unknown_action");
        PrincipalContext principal = principal(Set.of(tenantA), Set.of());
        assertDeny(evaluator.authorize(principal,
                request(tenantA, unknown, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity..read"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity.*"));
    }

    @Test
    void tenantWideGrantAllowsResourceInAuthorizedTenant() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.SITE_READ, ResourceScope.tenantWide(ResourceType.SITE))
        ));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.SITE_READ, ResourceType.SITE, resourceA)));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.SITE_READ, ResourceType.SITE, null)));
    }

    @Test
    void resourceRestrictedGrantAllowsNamedResource() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_UPDATE, ResourceScope.named(ResourceType.ENTITY, Set.of(resourceA)))
        ));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_UPDATE, ResourceType.ENTITY, resourceA)));
    }

    @Test
    void resourceRestrictedGrantDeniesDifferentResource() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_UPDATE, ResourceScope.named(ResourceType.ENTITY, Set.of(resourceA)))
        ));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_UPDATE, ResourceType.ENTITY, resourceB)),
                AuthorizationDecision.DenyReason.RESOURCE_SCOPE_DENIED);
    }

    @Test
    void resourceIdNeverOverridesTenantScope() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_UPDATE, ResourceScope.named(ResourceType.ENTITY, Set.of(resourceA)))
        ));
        assertDeny(evaluator.authorize(principal,
                request(tenantB, PlatformPermissions.ENTITY_UPDATE, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.TENANT_SCOPE_DENIED);
    }

    @Test
    void credentialUseOnlyAllowsUseAndDeniesReveal() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.CREDENTIAL_USE, ResourceScope.tenantWide(ResourceType.CREDENTIAL))
        ));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_USE, ResourceType.CREDENTIAL, resourceA)));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_REVEAL, ResourceType.CREDENTIAL, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
    }

    @Test
    void credentialRevealOnlyAllowsRevealAndDeniesUse() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.CREDENTIAL_REVEAL, ResourceScope.tenantWide(ResourceType.CREDENTIAL))
        ));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_REVEAL, ResourceType.CREDENTIAL, resourceA)));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_USE, ResourceType.CREDENTIAL, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
    }

    @Test
    void bothCredentialPermissionsAreIndependentAndAllowed() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.CREDENTIAL_USE, ResourceScope.tenantWide(ResourceType.CREDENTIAL)),
                grant(tenantA, PlatformPermissions.CREDENTIAL_REVEAL, ResourceScope.tenantWide(ResourceType.CREDENTIAL))
        ));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_USE, ResourceType.CREDENTIAL, resourceA)));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_REVEAL, ResourceType.CREDENTIAL, resourceA)));
    }

    @Test
    void noCredentialPermissionsDenyBoth() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of());
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_USE, ResourceType.CREDENTIAL, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_REVEAL, ResourceType.CREDENTIAL, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
    }

    @Test
    void roleNamesHaveNoSpecialBehavior() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, Permission.of("role.admin"), ResourceScope.tenantWide(ResourceType.ENTITY))
        ));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
    }

    @Test
    void mspMasterTenantAloneDoesNotGrantChildTenant() {
        UUID mspMaster = tenantA;
        UUID mspChild = tenantB;
        PrincipalContext principal = principal(Set.of(mspMaster), Set.of(
                grant(mspMaster, PlatformPermissions.TENANT_READ, ResourceScope.tenantWide(ResourceType.TENANT))
        ));
        assertDeny(evaluator.authorize(principal,
                request(mspChild, PlatformPermissions.TENANT_READ, ResourceType.TENANT, mspChild)),
                AuthorizationDecision.DenyReason.TENANT_SCOPE_DENIED);
    }

    @Test
    void duplicateGrantsDoNotChangeDecisionBehavior() {
        PermissionGrant grant = grant(tenantA, PlatformPermissions.ENTITY_READ,
                ResourceScope.named(ResourceType.ENTITY, Set.of(resourceA)));
        PrincipalContext one = principal(Set.of(tenantA), List.of(grant));
        PrincipalContext duplicated = principal(Set.of(tenantA), List.of(grant, grant, grant));
        AuthorizationRequest request = request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA);
        assertEquals(evaluator.authorize(one, request), evaluator.authorize(duplicated, request));
        assertEquals(1, duplicated.permissionGrants().size());
    }

    @Test
    void principalCollectionsAreImmutableAndDefensivelyCopied() {
        Set<UUID> tenantScopes = new HashSet<>();
        tenantScopes.add(tenantA);
        Set<PermissionGrant> grants = new HashSet<>();
        grants.add(grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.tenantWide(ResourceType.ENTITY)));
        PrincipalContext principal = principal(tenantScopes, grants);

        tenantScopes.add(tenantB);
        grants.clear();

        assertEquals(Set.of(tenantA), principal.tenantScopes());
        assertEquals(1, principal.permissionGrants().size());
        assertThrows(UnsupportedOperationException.class, () -> principal.tenantScopes().add(tenantB));
        assertThrows(UnsupportedOperationException.class, principal.permissionGrants()::clear);
    }

    @Test
    void permissionValuesAreBounded() {
        String value = "a." + "b".repeat(Permission.MAX_LENGTH - 2);
        assertEquals(Permission.MAX_LENGTH, value.length());
        assertEquals(value, Permission.of(value).value());
        assertThrows(IllegalArgumentException.class, () -> Permission.of(value + "c"));
    }

    @Test
    void permissionControlCharactersAreRejected() {
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity.re\nad"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity.re\u0000ad"));
    }

    @Test
    void permissionCaseAndNormalizationAreDeterministic() {
        assertEquals("entity.read", Permission.of("entity.read").value());
        assertThrows(IllegalArgumentException.class, () -> Permission.of("Entity.Read"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of(" entity.read"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity.read "));
    }

    @Test
    void nullAndBlankSubjectsAreRejected() {
        assertThrows(NullPointerException.class, () -> new PrincipalContext(
                null, AuthenticationState.AUTHENTICATED, Set.of(), Set.of()));
        assertThrows(IllegalArgumentException.class, () -> new PrincipalContext(
                " ", AuthenticationState.AUTHENTICATED, Set.of(), Set.of()));
        assertThrows(IllegalArgumentException.class, () -> new PrincipalContext(
                "subject\nvalue", AuthenticationState.AUTHENTICATED, Set.of(), Set.of()));
    }

    @Test
    void nullRequestFieldsAreRejectedSafely() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of());
        assertDeny(evaluator.authorize(principal, null), AuthorizationDecision.DenyReason.INVALID_REQUEST);
        assertDeny(evaluator.authorize(principal,
                new AuthorizationRequest(null, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.INVALID_REQUEST);
        assertDeny(evaluator.authorize(principal,
                new AuthorizationRequest(tenantA, null, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.INVALID_REQUEST);
        assertDeny(evaluator.authorize(principal,
                new AuthorizationRequest(tenantA, PlatformPermissions.ENTITY_READ, null, resourceA)),
                AuthorizationDecision.DenyReason.INVALID_REQUEST);
    }

    @Test
    void resourceTypeMismatchIsDenied() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.named(ResourceType.ENTITY, Set.of(resourceA)))
        ));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.SITE, resourceA)),
                AuthorizationDecision.DenyReason.RESOURCE_SCOPE_DENIED);
    }

    @Test
    void sameEntityIdUnderAnotherTenantCannotBeAuthorized() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.named(ResourceType.ENTITY, Set.of(resourceA)))
        ));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)));
        assertDeny(evaluator.authorize(principal,
                request(tenantB, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.TENANT_SCOPE_DENIED);
    }

    private PrincipalContext principal(Set<UUID> tenants, Set<PermissionGrant> grants) {
        return new PrincipalContext("subject-1", AuthenticationState.AUTHENTICATED, tenants, grants);
    }

    private PrincipalContext principal(Set<UUID> tenants, List<PermissionGrant> grants) {
        return new PrincipalContext("subject-1", AuthenticationState.AUTHENTICATED, tenants, grants);
    }

    private static PermissionGrant grant(UUID tenant, Permission permission, ResourceScope scope) {
        return new PermissionGrant(tenant, permission, scope);
    }

    private static AuthorizationRequest request(UUID tenant, Permission permission, ResourceType type, UUID resource) {
        return new AuthorizationRequest(tenant, permission, type, resource);
    }

    private static void assertAllow(AuthorizationDecision decision) {
        assertTrue(decision.allowed());
        assertEquals(AuthorizationDecision.Outcome.ALLOW, decision.outcome());
        assertEquals(null, decision.denyReason());
    }

    private static void assertDeny(AuthorizationDecision decision, AuthorizationDecision.DenyReason reason) {
        assertFalse(decision.allowed());
        assertEquals(AuthorizationDecision.Outcome.DENY, decision.outcome());
        assertEquals(reason, decision.denyReason());
    }
}
