package com.platform.core.security.identity;

import org.junit.jupiter.api.Test;

import java.lang.annotation.Annotation;
import java.lang.reflect.Constructor;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.lang.reflect.Modifier;
import java.lang.reflect.RecordComponent;
import java.time.Instant;
import java.util.Arrays;
import java.util.Collection;
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
    void providerFamiliesAndUnknownProviderValues() {
        assertArrayEquals(
                new IdentityProviderType[]{
                        IdentityProviderType.LOCAL,
                        IdentityProviderType.OIDC,
                        IdentityProviderType.SAML,
                        IdentityProviderType.LDAP
                },
                IdentityProviderType.values());
        assertThrows(IllegalArgumentException.class, () -> IdentityProviderType.valueOf("KEYCLOAK"));
        assertThrows(IllegalArgumentException.class, () -> IdentityProviderType.valueOf("oidc"));
        assertThrows(NullPointerException.class, () -> IdentityProviderType.valueOf(null));
    }

    @Test
    void providerIdentityKeyUsesTypeInstanceAndExactOpaqueSubject() {
        ProviderIdentityKey a = id(IdentityProviderType.OIDC, "corp-oidc", "same", false).providerIdentityKey();
        ProviderIdentityKey differentInstance =
                id(IdentityProviderType.OIDC, "partner-oidc", "same", false).providerIdentityKey();
        ProviderIdentityKey differentType =
                id(IdentityProviderType.SAML, "corp-oidc", "same", false).providerIdentityKey();
        ProviderIdentityKey equal =
                id(IdentityProviderType.OIDC, "corp-oidc", "same", false).providerIdentityKey();

        assertNotEquals(a, differentInstance);
        assertNotEquals(a, differentType);
        assertEquals(a, equal);
        assertEquals(a.hashCode(), equal.hashCode());
        assertNotEquals(new ExternalSubject("Subject"), new ExternalSubject("subject"));
        assertNotEquals(new ExternalSubject("\u00e9"), new ExternalSubject("e\u0301"));
    }

    @Test
    void providerInstanceValidationUsesStrictAsciiGrammar() {
        assertThrows(NullPointerException.class, () -> new ProviderInstanceId(null));
        for (String bad : Set.of(
                "", " ", ".", "..", "-foo", "foo-", ".foo", "foo.",
                "foo..bar", "foo--bar", "foo.-bar", "foo/bar", "foo\\bar",
                "foo bar", "FOO", "foo\nbar", "foo\u200Bbar")) {
            assertThrows(IllegalArgumentException.class, () -> new ProviderInstanceId(bad), bad);
        }
        for (String valid : Set.of("a", "local", "corp-oidc", "corp_oidc", "corp.oidc", "oidc2")) {
            assertEquals(valid, new ProviderInstanceId(valid).value());
        }
        String max = "a".repeat(IdentityLimits.PROVIDER_INSTANCE_ID_MAX_LENGTH);
        assertEquals(max, new ProviderInstanceId(max).value());
        assertThrows(IllegalArgumentException.class, () -> new ProviderInstanceId(max + "a"));
    }

    @Test
    void externalSubjectRejectsAmbiguousUnicodeWithoutNormalizing() {
        assertThrows(NullPointerException.class, () -> new ExternalSubject(null));
        for (String bad : new String[]{
                "", "   ", " subject", "subject ", "\u00A0subject", "subject\u00A0",
                "\u2003subject", "subject\u202F", "subject\u0000value", "subject\nvalue",
                "subject\u200Bvalue", "subject\u2066value", "subject\u2028value",
                "subject\u2029value", "subject\uD800value"
        }) {
            assertThrows(IllegalArgumentException.class, () -> new ExternalSubject(bad));
        }

        for (String valid : Set.of(
                "opaque:user/with+punc=@example",
                "\u7528\u6237-\u0394\u03BF\u03BA\u03B9\u03BC\u03AE",
                "internal space",
                "\u00e9",
                "e\u0301")) {
            assertEquals(valid, new ExternalSubject(valid).value());
        }
    }

    @Test
    void externalSubjectBoundIsMeasuredInUnicodeCodePoints() {
        String asciiMax = "x".repeat(IdentityLimits.EXTERNAL_SUBJECT_MAX_LENGTH);
        assertEquals(asciiMax, new ExternalSubject(asciiMax).value());
        assertThrows(IllegalArgumentException.class, () -> new ExternalSubject(asciiMax + "x"));

        String supplementaryMax = "\uD83D\uDE00".repeat(IdentityLimits.EXTERNAL_SUBJECT_MAX_LENGTH);
        assertEquals(supplementaryMax, new ExternalSubject(supplementaryMax).value());
        assertThrows(IllegalArgumentException.class,
                () -> new ExternalSubject(supplementaryMax + "\uD83D\uDE00"));
    }

    @Test
    void authenticationEvidenceHasDeterministicTimeAndAssuranceSemantics() {
        assertThrows(NullPointerException.class, () -> new AuthenticatedExternalIdentity(
                IdentityProviderType.OIDC, provider(), subject(), null,
                AuthenticationAssurance.withoutVerifiedMfa(), EXP));
        assertThrows(IllegalArgumentException.class, () -> new AuthenticatedExternalIdentity(
                IdentityProviderType.OIDC, provider(), subject(), AUTH,
                AuthenticationAssurance.withoutVerifiedMfa(), AUTH.minusNanos(1)));

        AuthenticatedExternalIdentity zeroLifetime = new AuthenticatedExternalIdentity(
                IdentityProviderType.OIDC, provider(), subject(), AUTH,
                AuthenticationAssurance.withoutVerifiedMfa(), AUTH);
        assertFalse(zeroLifetime.isCurrentAt(AUTH));

        AuthenticatedExternalIdentity evidence = id(IdentityProviderType.OIDC, "corp-oidc", "s", true);
        assertFalse(evidence.isCurrentAt(AUTH.minusNanos(1)));
        assertTrue(evidence.isCurrentAt(AUTH));
        assertTrue(evidence.isCurrentAt(EXP.minusNanos(1)));
        assertFalse(evidence.isCurrentAt(EXP));
        assertFalse(evidence.isCurrentAt(EXP.plusNanos(1)));
        assertThrows(NullPointerException.class, () -> evidence.isCurrentAt(null));

        assertFalse(AuthenticationAssurance.withoutVerifiedMfa().mfaSatisfied());
        assertTrue(AuthenticationAssurance.withVerifiedMfa().mfaSatisfied());
        assertEquals(new AuthenticationAssurance(false), evidenceWithMfa(false).assurance());
        assertEquals(new AuthenticationAssurance(true), evidenceWithMfa(true).assurance());
    }

    @Test
    void resolvedIdentityRejectsFutureAndExpiredEvidencePrecisely() {
        PlatformSubject platformSubject = platformSubject();
        AuthenticatedExternalIdentity evidence = id(IdentityProviderType.OIDC, "corp-oidc", "s", false);

        IdentityResolutionException future = assertThrows(IdentityResolutionException.class,
                () -> ResolvedAuthenticatedIdentity.current(platformSubject, evidence, AUTH.minusNanos(1)));
        assertEquals(IdentityResolutionFailure.INVALID_IDENTITY, future.failure());

        IdentityResolutionException exactExpiry = assertThrows(IdentityResolutionException.class,
                () -> ResolvedAuthenticatedIdentity.current(platformSubject, evidence, EXP));
        assertEquals(IdentityResolutionFailure.AUTHENTICATION_EXPIRED, exactExpiry.failure());

        IdentityResolutionException afterExpiry = assertThrows(IdentityResolutionException.class,
                () -> ResolvedAuthenticatedIdentity.current(platformSubject, evidence, EXP.plusNanos(1)));
        assertEquals(IdentityResolutionFailure.AUTHENTICATION_EXPIRED, afterExpiry.failure());
        assertDoesNotThrow(() -> ResolvedAuthenticatedIdentity.current(platformSubject, evidence, EXP.minusNanos(1)));
    }

    @Test
    void platformSubjectIsOnlyAStableUuidValue() {
        UUID uuid = UUID.fromString("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa");
        PlatformSubject a = new PlatformSubject(uuid);
        PlatformSubject equal = new PlatformSubject(uuid);
        PlatformSubject different = new PlatformSubject(
                UUID.fromString("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"));

        assertEquals(a, equal);
        assertEquals(a.hashCode(), equal.hashCode());
        assertNotEquals(a, different);
        assertEquals(uuid, a.subjectId());
        assertThrows(NullPointerException.class, () -> new PlatformSubject(null));
        assertArrayEquals(new Class<?>[]{UUID.class},
                Arrays.stream(PlatformSubject.class.getRecordComponents())
                        .map(RecordComponent::getType).toArray(Class<?>[]::new));
    }

    @Test
    void resolverIsProviderNeutralAndAlwaysAppliesMapping() throws Exception {
        Method method = IdentityResolver.class.getMethod(
                "resolve", AuthenticatedExternalIdentity.class, Instant.class);
        assertEquals(ResolvedAuthenticatedIdentity.class, method.getReturnType());
        assertArrayEquals(
                new Class<?>[]{AuthenticatedExternalIdentity.class, Instant.class},
                method.getParameterTypes());

        PlatformSubject sharedSubject = platformSubject();
        IdentityResolver mapping = (identity, evaluatedAt) ->
                ResolvedAuthenticatedIdentity.current(sharedSubject, identity, evaluatedAt);
        AuthenticatedExternalIdentity local = id(IdentityProviderType.LOCAL, "local", "local-user", false);
        AuthenticatedExternalIdentity oidc = id(IdentityProviderType.OIDC, "corp-oidc", "oidc-user", true);

        ResolvedAuthenticatedIdentity resolvedLocal = mapping.resolve(local, NOW);
        ResolvedAuthenticatedIdentity resolvedOidc = mapping.resolve(oidc, NOW);
        assertEquals(sharedSubject, resolvedLocal.platformSubject());
        assertEquals(sharedSubject, resolvedOidc.platformSubject());
        assertNotEquals(local.providerIdentityKey(), oidc.providerIdentityKey());
        assertThrows(IdentityResolutionException.class, () -> mapping.resolve(local, EXP));
    }

    @Test
    void disabledMappingIsDistinctFromProviderAuthenticationFailure() {
        IdentityResolver disabled = (identity, evaluatedAt) -> {
            throw new IdentityResolutionException(IdentityResolutionFailure.IDENTITY_DISABLED);
        };
        IdentityResolutionException error = assertThrows(IdentityResolutionException.class,
                () -> disabled.resolve(id(IdentityProviderType.LDAP, "ad-primary", "directory-id", true), NOW));
        assertEquals(IdentityResolutionFailure.IDENTITY_DISABLED, error.failure());
    }

    @Test
    void nullBehaviorIsConsistentAtDomainBoundaries() {
        assertThrows(NullPointerException.class,
                () -> new ProviderIdentityKey(null, provider(), subject()));
        assertThrows(NullPointerException.class,
                () -> new ProviderIdentityKey(IdentityProviderType.OIDC, null, subject()));
        assertThrows(NullPointerException.class,
                () -> new ProviderIdentityKey(IdentityProviderType.OIDC, provider(), null));
        assertThrows(NullPointerException.class,
                () -> new AuthenticatedExternalIdentity(null, provider(), subject(), AUTH,
                        AuthenticationAssurance.withoutVerifiedMfa(), EXP));
        assertThrows(NullPointerException.class,
                () -> new AuthenticatedExternalIdentity(IdentityProviderType.OIDC, null, subject(), AUTH,
                        AuthenticationAssurance.withoutVerifiedMfa(), EXP));
        assertThrows(NullPointerException.class,
                () -> new AuthenticatedExternalIdentity(IdentityProviderType.OIDC, provider(), null, AUTH,
                        AuthenticationAssurance.withoutVerifiedMfa(), EXP));
        assertThrows(NullPointerException.class,
                () -> new AuthenticatedExternalIdentity(IdentityProviderType.OIDC, provider(), subject(), AUTH,
                        null, EXP));
        assertThrows(NullPointerException.class,
                () -> ResolvedAuthenticatedIdentity.current(null, evidenceWithMfa(false), NOW));
        assertThrows(NullPointerException.class,
                () -> ResolvedAuthenticatedIdentity.current(platformSubject(), null, NOW));
        assertThrows(NullPointerException.class,
                () -> ResolvedAuthenticatedIdentity.current(platformSubject(), evidenceWithMfa(false), null));
        assertThrows(NullPointerException.class, () -> new IdentityResolutionException(null));
    }

    @Test
    void authorityModelContainsNoAuthorizationSecretsClaimsOrMutableCarriers() {
        Set<String> forbiddenAuthorityNames = Set.of(
                "tenant", "role", "permission", "grant", "resource", "admin", "scope");
        Set<String> forbiddenSecretNames = Set.of(
                "password", "token", "refresh", "jwt", "assertion", "credential", "secret",
                "privatekey", "clientsecret", "authorizationheader", "cookie", "claim", "attribute");

        for (Class<?> type : apiTypes()) {
            for (Field field : type.getDeclaredFields()) {
                if (Modifier.isStatic(field.getModifiers())) {
                    continue;
                }
                assertSafeName(type, field.getName(), forbiddenAuthorityNames, forbiddenSecretNames);
                assertNotMutableCarrier(type, field.getName(), field.getType());
                assertTrue(Modifier.isFinal(field.getModifiers()), type.getSimpleName() + "." + field.getName());
            }
            for (RecordComponent component : type.getRecordComponents() == null
                    ? new RecordComponent[0] : type.getRecordComponents()) {
                assertSafeName(type, component.getName(), forbiddenAuthorityNames, forbiddenSecretNames);
                assertNotMutableCarrier(type, component.getName(), component.getType());
            }
            if (!type.isEnum()) {
                for (Method method : type.getDeclaredMethods()) {
                    if (!Modifier.isPublic(method.getModifiers()) || method.isSynthetic()) {
                        continue;
                    }
                    assertNotMutableCarrier(type, method.getName(), method.getReturnType());
                    for (Class<?> parameterType : method.getParameterTypes()) {
                        assertNotMutableCarrier(type, method.getName(), parameterType);
                    }
                }
                for (Constructor<?> constructor : type.getDeclaredConstructors()) {
                    if (!Modifier.isPublic(constructor.getModifiers())) {
                        continue;
                    }
                    for (Class<?> parameterType : constructor.getParameterTypes()) {
                        assertNotMutableCarrier(type, "constructor", parameterType);
                    }
                }
            }
            assertNoFrameworkSerializationAnnotations(type);
            assertTrue(type.isRecord() || type.isEnum() || type.isInterface()
                    || Modifier.isFinal(type.getModifiers()), type.getName());
        }
    }

    @Test
    void resolutionFailuresAndStringRepresentationsAreNonSensitive() {
        Set<String> messages = Arrays.stream(IdentityResolutionFailure.values())
                .map(IdentityResolutionException::new)
                .peek(error -> assertNotNull(error.failure()))
                .map(Throwable::getMessage)
                .collect(java.util.stream.Collectors.toSet());
        assertEquals(Set.of("identity resolution failed"), messages);

        String external = "provider-subject-123";
        AuthenticatedExternalIdentity evidence = id(
                IdentityProviderType.OIDC, "corp-oidc", external, false);
        ResolvedAuthenticatedIdentity resolved =
                ResolvedAuthenticatedIdentity.current(platformSubject(), evidence, NOW);

        assertFalse(new ExternalSubject(external).toString().contains(external));
        assertFalse(evidence.providerIdentityKey().toString().contains(external));
        assertFalse(evidence.toString().contains(external));
        assertFalse(resolved.toString().contains(external));
        assertFalse(new IdentityResolutionException(IdentityResolutionFailure.IDENTITY_DISABLED)
                .toString().contains(external));
    }

    private static AuthenticatedExternalIdentity id(
            IdentityProviderType type, String provider, String externalSubject, boolean mfa) {
        return new AuthenticatedExternalIdentity(
                type,
                new ProviderInstanceId(provider),
                new ExternalSubject(externalSubject),
                AUTH,
                new AuthenticationAssurance(mfa),
                EXP);
    }

    private static ProviderInstanceId provider() {
        return new ProviderInstanceId("corp-oidc");
    }

    private static ExternalSubject subject() {
        return new ExternalSubject("subject");
    }

    private static PlatformSubject platformSubject() {
        return new PlatformSubject(UUID.fromString("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"));
    }

    private static AuthenticatedExternalIdentity evidenceWithMfa(boolean mfa) {
        return id(IdentityProviderType.OIDC, "corp-oidc", "subject", mfa);
    }

    private static void assertSafeName(
            Class<?> owner, String value, Set<String> forbiddenAuthorityNames, Set<String> forbiddenSecretNames) {
        String name = value.toLowerCase(Locale.ROOT);
        assertTrue(forbiddenAuthorityNames.stream().noneMatch(name::contains),
                owner.getSimpleName() + "." + value);
        assertTrue(forbiddenSecretNames.stream().noneMatch(name::contains),
                owner.getSimpleName() + "." + value);
    }

    private static void assertNotMutableCarrier(Class<?> owner, String member, Class<?> valueType) {
        assertFalse(Map.class.isAssignableFrom(valueType), owner.getSimpleName() + "." + member);
        assertFalse(Collection.class.isAssignableFrom(valueType), owner.getSimpleName() + "." + member);
        assertFalse(valueType.isArray(), owner.getSimpleName() + "." + member);
    }

    private static void assertNoFrameworkSerializationAnnotations(Class<?> type) {
        for (Annotation annotation : type.getAnnotations()) {
            String annotationType = annotation.annotationType().getName();
            assertFalse(annotationType.startsWith("com.fasterxml.jackson"), type.getName());
            assertFalse(annotationType.startsWith("org.springframework"), type.getName());
            assertFalse(annotationType.startsWith("jakarta.persistence"), type.getName());
        }
    }

    private static Class<?>[] apiTypes() {
        return new Class<?>[]{
                IdentityProviderType.class,
                IdentityLimits.class,
                ProviderInstanceId.class,
                ExternalSubject.class,
                ProviderIdentityKey.class,
                AuthenticationAssurance.class,
                AuthenticatedExternalIdentity.class,
                PlatformSubject.class,
                IdentityResolutionFailure.class,
                IdentityResolutionException.class,
                ResolvedAuthenticatedIdentity.class,
                IdentityResolver.class
        };
    }
}
