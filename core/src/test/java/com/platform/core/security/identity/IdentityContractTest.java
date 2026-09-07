package com.platform.core.security.identity;

import org.junit.jupiter.api.Test;

import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.lang.reflect.Modifier;
import java.time.Instant;
import java.util.Arrays;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;

class IdentityContractTest {
    private static final Instant AUTH = Instant.parse("2026-09-07T09:00:00Z");
    private static final Instant EXP = Instant.parse("2026-09-07T10:00:00Z");
    private static final Instant NOW = Instant.parse("2026-09-07T09:30:00Z");

    @Test
    void providerFamiliesAndProviderIdentityKeySemantics() {
        assertEquals(IdentityProviderType.LOCAL, id(IdentityProviderType.LOCAL, "local", "s", false).providerType());
        assertEquals(IdentityProviderType.OIDC, id(IdentityProviderType.OIDC, "corp-oidc", "s", false).providerType());
        assertEquals(IdentityProviderType.SAML, id(IdentityProviderType.SAML, "partner-saml", "s", false).providerType());
        assertEquals(IdentityProviderType.LDAP, id(IdentityProviderType.LDAP, "ad-primary", "s", false).providerType());
        ProviderIdentityKey a = id(IdentityProviderType.OIDC, "corp-oidc", "same", false).providerIdentityKey();
        ProviderIdentityKey b = id(IdentityProviderType.OIDC, "partner-oidc", "same", false).providerIdentityKey();
        ProviderIdentityKey c = id(IdentityProviderType.SAML, "corp-oidc", "same", false).providerIdentityKey();
        assertNotEquals(a, b);
        assertNotEquals(a, c);
        assertEquals(a, id(IdentityProviderType.OIDC, "corp-oidc", "same", false).providerIdentityKey());
        assertEquals(a.hashCode(), id(IdentityProviderType.OIDC, "corp-oidc", "same", false).providerIdentityKey().hashCode());
    }

    @Test
    void providerInstanceValidation() {
        assertThrows(IllegalArgumentException.class, () -> new ProviderInstanceId(" "));
        for (String bad : Set.of("-corp", "corp-", "corp..oidc", "corp/oidc", "corp oidc", "CORP", "corp\noidc")) {
            assertThrows(IllegalArgumentException.class, () -> new ProviderInstanceId(bad));
        }
        String max = "a".repeat(IdentityLimits.PROVIDER_INSTANCE_ID_MAX_LENGTH);
        assertEquals(max, new ProviderInstanceId(max).value());
        assertThrows(IllegalArgumentException.class, () -> new ProviderInstanceId(max + "a"));
    }

    @Test
    void externalSubjectValidation() {
        for (String bad : new String[]{"   ", " subject", "subject ", "subject\u0000value", "subject\nvalue"}) {
            assertThrows(IllegalArgumentException.class, () -> new ExternalSubject(bad));
        }
        String max = "x".repeat(IdentityLimits.EXTERNAL_SUBJECT_MAX_LENGTH);
        assertEquals(max, new ExternalSubject(max).value());
        assertThrows(IllegalArgumentException.class, () -> new ExternalSubject(max + "x"));
    }

    @Test
    void authenticationEvidenceTimeAndAssurance() {
        assertThrows(NullPointerException.class, () -> new AuthenticatedExternalIdentity(
                IdentityProviderType.OIDC, new ProviderInstanceId("corp-oidc"), new ExternalSubject("s"),
                null, AuthenticationAssurance.withoutVerifiedMfa(), EXP));
        assertThrows(IllegalArgumentException.class, () -> new AuthenticatedExternalIdentity(
                IdentityProviderType.OIDC, new ProviderInstanceId("corp-oidc"), new ExternalSubject("s"),
                AUTH, AuthenticationAssurance.withoutVerifiedMfa(), AUTH.minusNanos(1)));
        assertThrows(NullPointerException.class, () -> new AuthenticatedExternalIdentity(
                null, new ProviderInstanceId("corp-oidc"), new ExternalSubject("s"),
                AUTH, AuthenticationAssurance.withoutVerifiedMfa(), EXP));
        assertFalse(id(IdentityProviderType.OIDC, "corp-oidc", "s", false).assurance().mfaSatisfied());
        assertTrue(id(IdentityProviderType.OIDC, "corp-oidc", "s", true).assurance().mfaSatisfied());
        AuthenticatedExternalIdentity evidence = id(IdentityProviderType.OIDC, "corp-oidc", "s", true);
        assertTrue(evidence.isCurrentAt(EXP.minusNanos(1)));
        assertFalse(evidence.isCurrentAt(EXP));
        assertFalse(evidence.isCurrentAt(AUTH.minusNanos(1)));
    }

    @Test
    void platformAndResolvedSubjectContract() throws Exception {
        UUID uuid = UUID.fromString("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa");
        assertEquals(new PlatformSubject(uuid), new PlatformSubject(uuid));
        assertThrows(NullPointerException.class, () -> new PlatformSubject(null));
        PlatformSubject subject = new PlatformSubject(uuid);
        AuthenticatedExternalIdentity evidence = id(IdentityProviderType.LDAP, "ad-primary", "directory-id", true);
        ResolvedAuthenticatedIdentity resolved = ResolvedAuthenticatedIdentity.current(subject, evidence, NOW);
        assertEquals(subject, resolved.platformSubject());
        assertEquals(evidence, resolved.authenticationEvidence());
        assertTrue(resolved.assurance().mfaSatisfied());
        IdentityResolutionException expired = assertThrows(IdentityResolutionException.class,
                () -> ResolvedAuthenticatedIdentity.current(subject, evidence, EXP));
        assertEquals(IdentityResolutionFailure.AUTHENTICATION_EXPIRED, expired.failure());
        Method method = IdentityResolver.class.getMethod("resolve", AuthenticatedExternalIdentity.class, Instant.class);
        assertEquals(ResolvedAuthenticatedIdentity.class, method.getReturnType());
    }

    @Test
    void authorityModelContainsNoProfileClaimsAuthorizationOrMutableCollections() {
        Set<String> fields = Arrays.stream(AuthenticatedExternalIdentity.class.getDeclaredFields())
                .map(Field::getName).collect(java.util.stream.Collectors.toSet());
        for (String forbidden : Set.of("email", "displayName", "username", "upn")) {
            assertFalse(fields.contains(forbidden));
        }
        for (Class<?> type : apiTypes()) {
            for (Field field : type.getDeclaredFields()) {
                if (!Modifier.isStatic(field.getModifiers())) {
                    String name = field.getName().toLowerCase(Locale.ROOT);
                    assertTrue(Set.of("tenant", "role", "permission", "grant", "resource", "admin")
                            .stream().noneMatch(name::contains), type.getSimpleName() + "." + field.getName());
                }
                assertFalse(Map.class.isAssignableFrom(field.getType()));
            }
        }
        for (Class<?> type : immutableTypes()) {
            assertTrue(type.isRecord() || Modifier.isFinal(type.getModifiers()));
            for (Field field : type.getDeclaredFields()) {
                if (!Modifier.isStatic(field.getModifiers())) assertTrue(Modifier.isFinal(field.getModifiers()));
            }
        }
    }

    @Test
    void resolutionFailuresAndStringsAreNonSensitive() {
        for (IdentityResolutionFailure failure : IdentityResolutionFailure.values()) {
            IdentityResolutionException error = new IdentityResolutionException(failure);
            assertEquals(failure, error.failure());
            assertFalse(error.getMessage().isBlank());
        }
        String external = "provider-subject-123";
        AuthenticatedExternalIdentity evidence = id(IdentityProviderType.OIDC, "corp-oidc", external, false);
        assertFalse(evidence.toString().contains(external));
        assertFalse(evidence.providerIdentityKey().toString().contains(external));
    }

    private static AuthenticatedExternalIdentity id(IdentityProviderType type, String provider, String subject, boolean mfa) {
        return new AuthenticatedExternalIdentity(type, new ProviderInstanceId(provider), new ExternalSubject(subject),
                AUTH, new AuthenticationAssurance(mfa), EXP);
    }

    private static Class<?>[] apiTypes() {
        return new Class<?>[]{IdentityProviderType.class, IdentityLimits.class, ProviderInstanceId.class,
                ExternalSubject.class, ProviderIdentityKey.class, AuthenticationAssurance.class,
                AuthenticatedExternalIdentity.class, PlatformSubject.class, IdentityResolutionFailure.class,
                IdentityResolutionException.class, ResolvedAuthenticatedIdentity.class, IdentityResolver.class};
    }

    private static Class<?>[] immutableTypes() {
        return new Class<?>[]{ProviderInstanceId.class, ExternalSubject.class, ProviderIdentityKey.class,
                AuthenticationAssurance.class, AuthenticatedExternalIdentity.class, PlatformSubject.class,
                ResolvedAuthenticatedIdentity.class};
    }
}
