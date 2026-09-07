package com.platform.core.security.authorization;

import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.Callable;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class AuthorizationEvaluatorTest {
    private final AuthorizationEvaluator evaluator = new AuthorizationEvaluator();
    private final UUID tenantA = UUID.fromString("10000000-0000-0000-0000-000000000001");
    private final UUID tenantB = UUID.fromString("20000000-0000-0000-0000-000000000002");
    private final UUID resourceA = UUID.fromString("30000000-0000-0000-0000-000000000003");
    private final UUID resourceB = UUID.fromString("40000000-0000-0000-0000-000000000004");
    private final UUID resourceX = UUID.fromString("50000000-0000-0000-0000-000000000005");
    private final UUID resourceY = UUID.fromString("60000000-0000-0000-0000-000000000006");

    @Test
    void authenticatedMatchingTenantAndPermissionAllows() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.tenantWide(ResourceType.ENTITY))
        ));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)));
    }

    @Test
    void unauthenticatedPrincipalWithGrantsIsRejectedAtConstruction() {
        assertThrows(IllegalArgumentException.class, () -> new PrincipalContext(
                "subject-1",
                AuthenticationState.UNAUTHENTICATED,
                Set.of(tenantA),
                Set.of(grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.tenantWide(ResourceType.ENTITY)))
        ));
    }

    @Test
    void unauthenticatedPrincipalWithoutGrantsIsDenied() {
        PrincipalContext principal = new PrincipalContext(
                "subject-1", AuthenticationState.UNAUTHENTICATED, Set.of(), Set.of());
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
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity..read"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity.*"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of(".entity"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity."));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity read"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("ENTITY.READ"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity.\tread"));
        Permission unknown = Permission.of("entity.unknown_action");
        assertThrows(IllegalArgumentException.class,
                () -> grant(tenantA, unknown, ResourceScope.tenantWide(ResourceType.ENTITY)));
        assertThrows(IllegalArgumentException.class,
                () -> request(tenantA, unknown, ResourceType.ENTITY, resourceA));
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
    void emptyNamedResourceSetCannotBecomeTenantWide() {
        assertThrows(IllegalArgumentException.class,
                () -> ResourceScope.named(ResourceType.ENTITY, Set.of()));
        assertThrows(IllegalArgumentException.class,
                () -> new ResourceScope(ResourceType.ENTITY, ResourceScope.Kind.NAMED_RESOURCES, Set.of()));
    }

    @Test
    void nullResourceIdForNamedGrantIsDenied() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.named(ResourceType.ENTITY, Set.of(resourceA)))
        ));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, null)),
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
    void credentialUseOnlyAllowsUseAndDeniesRevealAndEdit() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.CREDENTIAL_USE, ResourceScope.tenantWide(ResourceType.CREDENTIAL))
        ));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_USE, ResourceType.CREDENTIAL, resourceA)));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_REVEAL, ResourceType.CREDENTIAL, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_EDIT, ResourceType.CREDENTIAL, resourceA)),
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
    void credentialEditIsIndependentOfUseAndReveal() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.CREDENTIAL_EDIT, ResourceScope.tenantWide(ResourceType.CREDENTIAL))
        ));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_EDIT, ResourceType.CREDENTIAL, resourceA)));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_USE, ResourceType.CREDENTIAL, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_REVEAL, ResourceType.CREDENTIAL, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
    }

    @Test
    void credentialPermissionPrefixAndWildcardLookalikesNeverImplyAuthority() {
        assertThrows(IllegalArgumentException.class, () -> Permission.of("credential"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("credential.*"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("*.reveal"));
        Permission useExtra = Permission.of("credential.use.extra");
        assertThrows(IllegalArgumentException.class,
                () -> grant(tenantA, useExtra, ResourceScope.tenantWide(ResourceType.CREDENTIAL)));
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.CREDENTIAL_USE, ResourceScope.tenantWide(ResourceType.CREDENTIAL))
        ));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.CREDENTIAL_REVEAL, ResourceType.CREDENTIAL, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
        assertNotEquals(PlatformPermissions.CREDENTIAL_USE, PlatformPermissions.CREDENTIAL_REVEAL);
        assertNotEquals(PlatformPermissions.CREDENTIAL_USE, PlatformPermissions.CREDENTIAL_EDIT);
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
    void unboundRoleLikePermissionsCannotBeGrantedAndDoNotAuthorize() {
        Permission roleAdmin = Permission.of("role.admin");
        assertThrows(IllegalArgumentException.class,
                () -> grant(tenantA, roleAdmin, ResourceScope.tenantWide(ResourceType.ENTITY)));
        PrincipalContext principal = principal(Set.of(tenantA), Set.of());
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
    }

    @Test
    void permissionResourceTypeMismatchIsRejectedAtGrantAndRequest() {
        assertThrows(IllegalArgumentException.class,
                () -> grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.tenantWide(ResourceType.SITE)));
        assertThrows(IllegalArgumentException.class,
                () -> grant(tenantA, PlatformPermissions.CREDENTIAL_USE, ResourceScope.tenantWide(ResourceType.ENTITY)));
        assertThrows(IllegalArgumentException.class,
                () -> grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.tenantWide(ResourceType.CREDENTIAL)));
        assertThrows(IllegalArgumentException.class,
                () -> request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.SITE, resourceA));
        assertThrows(IllegalArgumentException.class,
                () -> request(tenantA, PlatformPermissions.CREDENTIAL_USE, ResourceType.ENTITY, resourceA));
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
    void grantTenantOutsidePrincipalScopesIsRejected() {
        assertThrows(IllegalArgumentException.class, () -> principal(Set.of(tenantA), Set.of(
                grant(tenantB, PlatformPermissions.ENTITY_READ, ResourceScope.tenantWide(ResourceType.ENTITY))
        )));
    }

    @Test
    void duplicateAndCombinedRestrictedGrantsAreDeterministic() {
        PermissionGrant grantX = grant(tenantA, PlatformPermissions.ENTITY_READ,
                ResourceScope.named(ResourceType.ENTITY, Set.of(resourceX)));
        PermissionGrant grantY = grant(tenantA, PlatformPermissions.ENTITY_READ,
                ResourceScope.named(ResourceType.ENTITY, Set.of(resourceY)));
        PrincipalContext principal = principal(Set.of(tenantA), List.of(grantX, grantY, grantX));
        assertEquals(2, principal.permissionGrants().size());
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceX)));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceY)));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.RESOURCE_SCOPE_DENIED);
    }

    @Test
    void tenantWideGrantDominatesRestrictedGrantForSamePermission() {
        PrincipalContext principal = principal(Set.of(tenantA), List.of(
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.named(ResourceType.ENTITY, Set.of(resourceA))),
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.tenantWide(ResourceType.ENTITY))
        ));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceB)));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, null)));
    }

    @Test
    void principalAndResourceScopeCollectionsAreImmutableAndDefensivelyCopied() {
        Set<UUID> tenantScopes = new HashSet<>();
        tenantScopes.add(tenantA);
        Set<UUID> namedIds = new HashSet<>();
        namedIds.add(resourceA);
        ResourceScope scope = ResourceScope.named(ResourceType.ENTITY, namedIds);
        Set<PermissionGrant> grants = new HashSet<>();
        grants.add(grant(tenantA, PlatformPermissions.ENTITY_READ, scope));
        PrincipalContext principal = principal(tenantScopes, grants);

        tenantScopes.add(tenantB);
        grants.clear();
        namedIds.add(resourceB);

        assertEquals(Set.of(tenantA), principal.tenantScopes());
        assertEquals(1, principal.permissionGrants().size());
        assertEquals(Set.of(resourceA), scope.resourceIds());
        assertThrows(UnsupportedOperationException.class, () -> principal.tenantScopes().add(tenantB));
        assertThrows(UnsupportedOperationException.class, principal.permissionGrants()::clear);
        assertThrows(UnsupportedOperationException.class, () -> scope.resourceIds().add(resourceB));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceB)),
                AuthorizationDecision.DenyReason.RESOURCE_SCOPE_DENIED);
    }

    @Test
    void permissionValuesAreBounded() {
        String value = "a." + "b".repeat(Permission.MAX_LENGTH - 2);
        assertEquals(Permission.MAX_LENGTH, value.length());
        assertEquals(value, Permission.of(value).value());
        assertThrows(IllegalArgumentException.class, () -> Permission.of(value + "c"));
    }

    @Test
    void permissionControlAndUnicodeSurprisesAreRejected() {
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity.re\nad"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity.re\u0000ad"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity.re\u200Bad"));
        Locale previous = Locale.getDefault();
        try {
            Locale.setDefault(Locale.forLanguageTag("tr-TR"));
            assertThrows(IllegalArgumentException.class, () -> Permission.of("ENTITY.READ"));
            assertEquals("entity.read", Permission.of("entity.read").value());
        } finally {
            Locale.setDefault(previous);
        }
    }

    @Test
    void permissionCaseAndNormalizationAreDeterministic() {
        assertEquals("entity.read", Permission.of("entity.read").value());
        assertThrows(IllegalArgumentException.class, () -> Permission.of("Entity.Read"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of(" entity.read"));
        assertThrows(IllegalArgumentException.class, () -> Permission.of("entity.read "));
    }

    @Test
    void subjectValidationRejectsNullBlankControlFormatAndOverlength() {
        assertThrows(NullPointerException.class, () -> new PrincipalContext(
                null, AuthenticationState.AUTHENTICATED, Set.of(), Set.of()));
        assertThrows(IllegalArgumentException.class, () -> new PrincipalContext(
                " ", AuthenticationState.AUTHENTICATED, Set.of(), Set.of()));
        assertThrows(IllegalArgumentException.class, () -> new PrincipalContext(
                "subject\nvalue", AuthenticationState.AUTHENTICATED, Set.of(), Set.of()));
        assertThrows(IllegalArgumentException.class, () -> new PrincipalContext(
                "subject\u200Bvalue", AuthenticationState.AUTHENTICATED, Set.of(), Set.of()));
        String max = "s".repeat(PrincipalContext.MAX_SUBJECT_LENGTH);
        new PrincipalContext(max, AuthenticationState.AUTHENTICATED, Set.of(), Set.of());
        assertThrows(IllegalArgumentException.class, () -> new PrincipalContext(
                max + "x", AuthenticationState.AUTHENTICATED, Set.of(), Set.of()));
    }

    @Test
    void requestConstructionRejectsNullRequiredFields() {
        assertThrows(NullPointerException.class,
                () -> new AuthorizationRequest(null, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA));
        assertThrows(NullPointerException.class,
                () -> new AuthorizationRequest(tenantA, null, ResourceType.ENTITY, resourceA));
        assertThrows(NullPointerException.class,
                () -> new AuthorizationRequest(tenantA, PlatformPermissions.ENTITY_READ, null, resourceA));
        assertDeny(evaluator.authorize(null,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA)),
                AuthorizationDecision.DenyReason.INVALID_REQUEST);
        assertDeny(evaluator.authorize(principal(Set.of(tenantA), Set.of()), null),
                AuthorizationDecision.DenyReason.INVALID_REQUEST);
    }

    @Test
    void resourceTypeMismatchOnExistingGrantPathIsDenied() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.named(ResourceType.ENTITY, Set.of(resourceA)))
        ));
        assertDeny(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.SITE_READ, ResourceType.SITE, resourceA)),
                AuthorizationDecision.DenyReason.PERMISSION_DENIED);
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

    @Test
    void uuidEqualityIsCaseInsensitiveOfStringFormatting() {
        UUID upper = UUID.fromString("30000000-0000-0000-0000-000000000003");
        UUID lower = UUID.fromString("30000000-0000-0000-0000-000000000003");
        assertEquals(upper, lower);
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.named(ResourceType.ENTITY, Set.of(upper)))
        ));
        assertAllow(evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, lower)));
    }

    @Test
    void evaluatorIsDeterministicUnderConcurrentReuse() throws Exception {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of(
                grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.named(ResourceType.ENTITY, Set.of(resourceA)))
        ));
        AuthorizationRequest allowReq = request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA);
        AuthorizationRequest denyReq = request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceB);
        AuthorizationEvaluator shared = new AuthorizationEvaluator();
        ExecutorService pool = Executors.newFixedThreadPool(8);
        try {
            List<Callable<AuthorizationDecision>> tasks = new ArrayList<>();
            for (int i = 0; i < 200; i++) {
                tasks.add(() -> shared.authorize(principal, allowReq));
                tasks.add(() -> shared.authorize(principal, denyReq));
            }
            List<Future<AuthorizationDecision>> futures = pool.invokeAll(tasks);
            int allows = 0;
            int denies = 0;
            for (Future<AuthorizationDecision> future : futures) {
                AuthorizationDecision decision = future.get(5, TimeUnit.SECONDS);
                if (decision.allowed()) {
                    allows++;
                } else {
                    assertEquals(AuthorizationDecision.DenyReason.RESOURCE_SCOPE_DENIED, decision.denyReason());
                    denies++;
                }
            }
            assertEquals(200, allows);
            assertEquals(200, denies);
        } finally {
            pool.shutdownNow();
        }
    }

    @Test
    void denyDecisionDoesNotContainSecretLikeData() {
        PrincipalContext principal = principal(Set.of(tenantA), Set.of());
        AuthorizationDecision decision = evaluator.authorize(principal,
                request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA));
        String text = decision.toString();
        assertFalse(text.toLowerCase(Locale.ROOT).contains("password"));
        assertFalse(text.toLowerCase(Locale.ROOT).contains("token"));
        assertFalse(text.contains("subject-1"));
        assertFalse(text.contains(tenantA.toString()));
        assertTrue(text.contains("DENY"));
    }

    @Test
    void equalsAndHashCodeAreValueBased() {
        Permission p1 = Permission.of("entity.read");
        Permission p2 = Permission.of("entity.read");
        assertEquals(p1, p2);
        assertEquals(p1.hashCode(), p2.hashCode());
        PermissionGrant g1 = grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.tenantWide(ResourceType.ENTITY));
        PermissionGrant g2 = grant(tenantA, PlatformPermissions.ENTITY_READ, ResourceScope.tenantWide(ResourceType.ENTITY));
        assertEquals(g1, g2);
        assertEquals(g1.hashCode(), g2.hashCode());
        AuthorizationRequest r1 = request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA);
        AuthorizationRequest r2 = request(tenantA, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, resourceA);
        assertEquals(r1, r2);
        assertEquals(Set.of(g1), Set.of(g2));
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
