package com.platform.core.security.audit;

import org.junit.jupiter.api.Test;

import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.Callable;
import java.util.concurrent.Executors;

import static org.junit.jupiter.api.Assertions.*;

class SecurityAuditEventTest {
    private static final Instant NOW = Instant.parse("2026-09-07T10:00:00Z");
    private static final UUID ACTOR = UUID.fromString("11111111-1111-4111-8111-111111111111");
    private static final UUID TENANT_A = UUID.fromString("22222222-2222-4222-8222-222222222222");
    private static final UUID TENANT_B = UUID.fromString("33333333-3333-4333-8333-333333333333");
    private static final UUID RESOURCE = UUID.fromString("44444444-4444-4444-8444-444444444444");
    private static final CorrelationId CORR = new CorrelationId("req-01:abc.def_2");

    @Test
    void authenticationAuthorizationAndCredentialActionsArePrecise() {
        var authOk = event(UUID.randomUUID(), AuditCategory.AUTHENTICATION, AuditActions.AUTHENTICATION_LOGIN,
                AuditOutcome.SUCCESS, ActorType.USER, ACTOR, null, null, null, null, Map.of());
        var authFail = event(UUID.randomUUID(), AuditCategory.AUTHENTICATION, AuditActions.AUTHENTICATION_LOGIN,
                AuditOutcome.FAILURE, ActorType.USER, null, null, null, null,
                new AuditReasonCode("AUTHENTICATION_FAILED"), Map.of());
        var deny = event(UUID.randomUUID(), AuditCategory.AUTHORIZATION, AuditActions.AUTHORIZATION_DENY,
                AuditOutcome.DENIED, ActorType.USER, ACTOR, TENANT_A, new AuditResourceType("entity"), RESOURCE,
                new AuditReasonCode("PERMISSION_DENIED"), Map.of());
        assertEquals(ACTOR, authOk.actorSubjectId());
        assertNull(authFail.actorSubjectId());
        assertEquals(AuditOutcome.DENIED, deny.outcome());
        assertNotEquals(AuditActions.CREDENTIAL_USE, AuditActions.CREDENTIAL_EDIT);
        assertNotEquals(AuditActions.CREDENTIAL_USE, AuditActions.CREDENTIAL_REVEAL);
        assertNotEquals(AuditActions.CREDENTIAL_EDIT, AuditActions.CREDENTIAL_REVEAL);
        for (AuditAction a : new AuditAction[]{
                AuditActions.CREDENTIAL_USE, AuditActions.CREDENTIAL_EDIT, AuditActions.CREDENTIAL_REVEAL}) {
            assertEquals(a, event(UUID.randomUUID(), AuditCategory.CREDENTIAL, a, AuditOutcome.SUCCESS,
                    ActorType.USER, ACTOR, TENANT_A, null, null, null, Map.of()).action());
        }
        assertThrows(IllegalArgumentException.class, () -> new AuditAction("credential.*"));
        assertThrows(IllegalArgumentException.class, () -> new AuditAction("credential"));
    }

    @Test
    void eventIdentityIsIndependentOfCorrelationAndEqualsByEventId() {
        UUID sharedId = UUID.randomUUID();
        var a = event(UUID.randomUUID(), AuditCategory.SESSION, new AuditAction("session.logout"), AuditOutcome.SUCCESS,
                ActorType.USER, ACTOR, TENANT_A, null, null, null, Map.of());
        var b = event(UUID.randomUUID(), AuditCategory.SESSION, new AuditAction("session.logout"), AuditOutcome.SUCCESS,
                ActorType.USER, ACTOR, TENANT_A, null, null, null, Map.of());
        assertEquals(a.correlationId(), b.correlationId());
        assertNotEquals(a.eventId(), b.eventId());
        assertNotEquals(a, b);

        var sameIdLeft = event(sharedId, AuditCategory.CREDENTIAL, AuditActions.CREDENTIAL_USE, AuditOutcome.SUCCESS,
                ActorType.USER, ACTOR, TENANT_A, null, null, null, Map.of("path", "a"));
        var sameIdRight = event(sharedId, AuditCategory.CREDENTIAL, AuditActions.CREDENTIAL_REVEAL, AuditOutcome.FAILURE,
                ActorType.USER, ACTOR, TENANT_B, null, null, null, Map.of("path", "b"));
        assertEquals(sameIdLeft, sameIdRight);
        assertEquals(sameIdLeft.hashCode(), sameIdRight.hashCode());

        var credOne = event(UUID.randomUUID(), AuditCategory.CREDENTIAL, AuditActions.CREDENTIAL_USE, AuditOutcome.SUCCESS,
                ActorType.USER, ACTOR, TENANT_A, null, null, null, Map.of());
        var credTwo = event(UUID.randomUUID(), AuditCategory.CREDENTIAL, AuditActions.CREDENTIAL_USE, AuditOutcome.SUCCESS,
                ActorType.USER, ACTOR, TENANT_A, null, null, null, Map.of());
        assertNotEquals(credOne, credTwo);
    }

    @Test
    void actorModelRejectsNonsensicalSystemSubjectAndAllowsPreAuthUser() {
        assertDoesNotThrow(() -> event(UUID.randomUUID(), AuditCategory.AUTHENTICATION,
                AuditActions.AUTHENTICATION_LOGIN, AuditOutcome.FAILURE, ActorType.USER, null, null, null, null,
                new AuditReasonCode("AUTHENTICATION_FAILED"), Map.of()));
        assertDoesNotThrow(() -> event(UUID.randomUUID(), AuditCategory.SECURITY_ADMINISTRATION,
                new AuditAction("security.admin.rotate"), AuditOutcome.SUCCESS, ActorType.SYSTEM, null, TENANT_A,
                null, null, null, Map.of()));
        assertThrows(IllegalArgumentException.class, () -> event(UUID.randomUUID(), AuditCategory.SECURITY_ADMINISTRATION,
                new AuditAction("security.admin.rotate"), AuditOutcome.SUCCESS, ActorType.SYSTEM, ACTOR, TENANT_A,
                null, null, null, Map.of()));
        assertDoesNotThrow(() -> event(UUID.randomUUID(), AuditCategory.AUTHENTICATION,
                AuditActions.AUTHENTICATION_LOGIN, AuditOutcome.SUCCESS, ActorType.SERVICE, ACTOR, null, null, null,
                null, Map.of()));
        assertEquals(3, ActorType.values().length);
        assertThrows(IllegalArgumentException.class, () -> ActorType.valueOf("POLICY_ENGINE"));
    }

    @Test
    void requiredFieldsFailClosed() {
        UUID id = UUID.randomUUID();
        assertThrows(NullPointerException.class, () -> new SecurityAuditEvent(null, NOW, CORR, AuditCategory.AUTHENTICATION,
                AuditActions.AUTHENTICATION_LOGIN, AuditOutcome.SUCCESS, ActorType.USER, null, null, null, null, null, null, AuditMetadata.empty()));
        assertThrows(NullPointerException.class, () -> new SecurityAuditEvent(id, null, CORR, AuditCategory.AUTHENTICATION,
                AuditActions.AUTHENTICATION_LOGIN, AuditOutcome.SUCCESS, ActorType.USER, null, null, null, null, null, null, AuditMetadata.empty()));
        assertThrows(NullPointerException.class, () -> new SecurityAuditEvent(id, NOW, null, AuditCategory.AUTHENTICATION,
                AuditActions.AUTHENTICATION_LOGIN, AuditOutcome.SUCCESS, ActorType.USER, null, null, null, null, null, null, AuditMetadata.empty()));
        assertThrows(NullPointerException.class, () -> new SecurityAuditEvent(id, NOW, CORR, null,
                AuditActions.AUTHENTICATION_LOGIN, AuditOutcome.SUCCESS, ActorType.USER, null, null, null, null, null, null, AuditMetadata.empty()));
        assertThrows(NullPointerException.class, () -> new SecurityAuditEvent(id, NOW, CORR, AuditCategory.AUTHENTICATION,
                null, AuditOutcome.SUCCESS, ActorType.USER, null, null, null, null, null, null, AuditMetadata.empty()));
        assertThrows(NullPointerException.class, () -> new SecurityAuditEvent(id, NOW, CORR, AuditCategory.AUTHENTICATION,
                AuditActions.AUTHENTICATION_LOGIN, null, ActorType.USER, null, null, null, null, null, null, AuditMetadata.empty()));
        assertThrows(NullPointerException.class, () -> new SecurityAuditEvent(id, NOW, CORR, AuditCategory.AUTHENTICATION,
                AuditActions.AUTHENTICATION_LOGIN, AuditOutcome.SUCCESS, null, null, null, null, null, null, null, AuditMetadata.empty()));
    }

    @Test
    void correlationActionAndReasonGrammarsAreBounded() {
        assertEquals("req-01:abc.def_2", CORR.value());
        assertEquals(128, AuditLimits.CORRELATION_ID_MAX);
        assertDoesNotThrow(() -> new CorrelationId("A".repeat(AuditLimits.CORRELATION_ID_MAX)));
        assertThrows(IllegalArgumentException.class, () -> new CorrelationId(" "));
        assertThrows(IllegalArgumentException.class, () -> new CorrelationId(" leading"));
        assertThrows(IllegalArgumentException.class, () -> new CorrelationId("trailing "));
        assertThrows(IllegalArgumentException.class, () -> new CorrelationId("bad\nvalue"));
        assertThrows(IllegalArgumentException.class, () -> new CorrelationId("bad\u200Bvalue"));
        assertThrows(IllegalArgumentException.class, () -> new CorrelationId("x".repeat(AuditLimits.CORRELATION_ID_MAX + 1)));

        assertThrows(IllegalArgumentException.class, () -> new AuditAction("Authentication.Login"));
        assertThrows(IllegalArgumentException.class, () -> new AuditAction("login"));
        assertThrows(IllegalArgumentException.class, () -> new AuditAction(".authentication.login"));
        assertThrows(IllegalArgumentException.class, () -> new AuditAction("authentication.login."));
        assertThrows(IllegalArgumentException.class, () -> new AuditAction("authentication..login"));
        assertThrows(IllegalArgumentException.class, () -> new AuditAction("authentication login"));
        assertThrows(IllegalArgumentException.class, () -> new AuditAction("authentication.\tlogin"));
        assertThrows(IllegalArgumentException.class, () -> new AuditAction("authentication.\u200Blogin"));
        assertThrows(IllegalArgumentException.class, () -> new AuditAction("auth.*"));
        String action = "a." + "b".repeat(AuditLimits.ACTION_MAX - 2);
        assertEquals(AuditLimits.ACTION_MAX, new AuditAction(action).value().length());
        assertThrows(IllegalArgumentException.class, () -> new AuditAction(action + "b"));

        assertEquals("AUTHENTICATION_FAILED", new AuditReasonCode("AUTHENTICATION_FAILED").value());
        assertThrows(IllegalArgumentException.class, () -> new AuditReasonCode("authentication failed"));
        assertThrows(IllegalArgumentException.class, () -> new AuditReasonCode("AUTH\nFAIL"));
        assertThrows(IllegalArgumentException.class, () -> new AuditReasonCode("AUTH\u2028FAIL"));
        assertThrows(IllegalArgumentException.class, () -> new AuditReasonCode(" AUTH_FAIL"));
    }

    @Test
    void tenantResourceAndNetworkSourceStayExplicitAndNonAuthoritative() {
        var preAuth = event(UUID.randomUUID(), AuditCategory.AUTHENTICATION, AuditActions.AUTHENTICATION_LOGIN,
                AuditOutcome.FAILURE, ActorType.USER, null, null, null, null, null, Map.of());
        assertNull(preAuth.tenantId());
        assertNull(preAuth.resourceType());
        assertNull(preAuth.resourceId());
        assertThrows(IllegalArgumentException.class, () -> event(UUID.randomUUID(), AuditCategory.AUTHORIZATION,
                AuditActions.AUTHORIZATION_DENY, AuditOutcome.DENIED, ActorType.USER, ACTOR, TENANT_A,
                new AuditResourceType("entity"), null, null, Map.of()));
        assertThrows(IllegalArgumentException.class, () -> event(UUID.randomUUID(), AuditCategory.AUTHORIZATION,
                AuditActions.AUTHORIZATION_DENY, AuditOutcome.DENIED, ActorType.USER, ACTOR, TENANT_A,
                null, RESOURCE, null, Map.of()));
        var a = event(UUID.randomUUID(), AuditCategory.AUTHORIZATION, AuditActions.AUTHORIZATION_ALLOW, AuditOutcome.SUCCESS,
                ActorType.USER, ACTOR, TENANT_A, new AuditResourceType("entity"), RESOURCE, null, Map.of());
        var b = event(UUID.randomUUID(), AuditCategory.AUTHORIZATION, AuditActions.AUTHORIZATION_ALLOW, AuditOutcome.SUCCESS,
                ActorType.USER, ACTOR, TENANT_B, new AuditResourceType("entity"), RESOURCE, null, Map.of());
        assertNotEquals(a.tenantId(), b.tenantId());
        assertEquals(a.resourceId(), b.resourceId());

        var source = new NetworkSourceAddress("2001:db8::10");
        var sourceEvent = new SecurityAuditEvent(UUID.randomUUID(), NOW, CORR, AuditCategory.AUTHENTICATION,
                AuditActions.AUTHENTICATION_LOGIN, AuditOutcome.FAILURE, ActorType.USER, null, null, source,
                null, null, new AuditReasonCode("AUTHENTICATION_FAILED"), AuditMetadata.empty());
        assertEquals(source, sourceEvent.sourceAddress());
        assertNull(sourceEvent.actorSubjectId());
        assertDoesNotThrow(() -> new NetworkSourceAddress("192.0.2.10"));
        assertDoesNotThrow(() -> new NetworkSourceAddress("::ffff:192.0.2.10"));
        assertThrows(IllegalArgumentException.class, () -> new NetworkSourceAddress("bad\naddress"));
        assertThrows(IllegalArgumentException.class, () -> new NetworkSourceAddress("localhost"));
        assertThrows(IllegalArgumentException.class, () -> new NetworkSourceAddress("example.com"));
        assertThrows(IllegalArgumentException.class, () -> new NetworkSourceAddress("not-an-ip"));
        assertFalse(NetworkSourceAddress.isIpLiteralShape("example.com"));
        assertTrue(NetworkSourceAddress.isIpLiteralShape("127.0.0.1"));
    }

    @Test
    void metadataIsImmutableBoundedAndUtf8Aware() {
        var input = new LinkedHashMap<String, String>();
        input.put("auth.method", "oidc");
        var metadata = new AuditMetadata(input);
        input.put("later", "mutation");
        assertEquals(Map.of("auth.method", "oidc"), metadata.values());
        assertThrows(UnsupportedOperationException.class, () -> metadata.values().put("x", "y"));

        var tooMany = new LinkedHashMap<String, String>();
        for (int i = 0; i <= AuditLimits.METADATA_MAX_ENTRIES; i++) {
            tooMany.put("key" + i, "v");
        }
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(tooMany));

        assertThrows(IllegalArgumentException.class,
                () -> new AuditMetadata(Map.of("k".repeat(AuditLimits.METADATA_KEY_MAX + 1), "v")));
        assertDoesNotThrow(() -> new AuditMetadata(Map.of("k".repeat(AuditLimits.METADATA_KEY_MAX), "v")));

        byte[] exactly512 = new byte[AuditLimits.METADATA_VALUE_MAX];
        java.util.Arrays.fill(exactly512, (byte) 'x');
        assertDoesNotThrow(() -> new AuditMetadata(Map.of("exact", new String(exactly512, StandardCharsets.UTF_8))));
        assertThrows(IllegalArgumentException.class,
                () -> new AuditMetadata(Map.of("over", new String(exactly512, StandardCharsets.UTF_8) + "y")));

        // Multibyte Unicode: char-count under limit can still exceed UTF-8 byte limit.
        String multi = "é".repeat(300); // 300 chars, 600 UTF-8 bytes
        assertTrue(multi.length() < AuditLimits.METADATA_VALUE_MAX);
        assertTrue(multi.getBytes(StandardCharsets.UTF_8).length > AuditLimits.METADATA_VALUE_MAX);
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of("unicode", multi)));

        var nullKey = new LinkedHashMap<String, String>();
        nullKey.put(null, "v");
        var nullValue = new LinkedHashMap<String, String>();
        nullValue.put("key", null);
        assertThrows(NullPointerException.class, () -> new AuditMetadata(nullKey));
        assertThrows(NullPointerException.class, () -> new AuditMetadata(nullValue));

        // Aggregate UTF-8 bytes across many entries (each value within per-value max).
        var under = new LinkedHashMap<String, String>();
        for (int i = 0; i < 16; i++) {
            under.put("k" + i, "v".repeat(500));
        }
        // keys ~32 bytes + 16*500 = 8032-ish; ensure under 8192
        assertDoesNotThrow(() -> new AuditMetadata(under));
        var over = new LinkedHashMap<String, String>(under);
        over.put("overflow", "z".repeat(200));
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(over));

        var exact = new LinkedHashMap<String, String>();
        // 15 * (2-char key + 500 value) approx; craft exact total
        int remaining = AuditLimits.METADATA_TOTAL_UTF8_BYTES_MAX;
        int index = 0;
        while (remaining > 0 && index < AuditLimits.METADATA_MAX_ENTRIES) {
            String key = "e" + index;
            int keyBytes = key.getBytes(StandardCharsets.UTF_8).length;
            int valueBudget = Math.min(AuditLimits.METADATA_VALUE_MAX, remaining - keyBytes);
            if (valueBudget <= 0) {
                break;
            }
            exact.put(key, "x".repeat(valueBudget));
            remaining -= keyBytes + valueBudget;
            index++;
        }
        assertEquals(0, remaining);
        assertDoesNotThrow(() -> new AuditMetadata(exact));
        exact.put("plus", "1");
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(exact));

        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of("safe", "bad\nvalue")));
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of("safe", "bad\u2028value")));
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of("safe", "bad\u200Bvalue")));
        assertDoesNotThrow(() -> new AuditMetadata(Map.of("note", "café résumé")));
    }

    @Test
    void forbiddenSecretMetadataKeysFailClosedWithoutOverclaimingValues() {
        for (String key : new String[]{
                "password", "passwd", "auth.password", "auth_password", "auth-password", "user.password.value",
                "access_token", "access.token", "access-token", "accesstoken",
                "refresh_token", "id_token", "http.authorization", "authorization.header",
                "session.cookie", "set.cookie", "set_cookie", "cookie.value",
                "client_secret", "private_key", "private.key", "api_key", "credential_value", "auth.token"
        }) {
            assertTrue(AuditMetadata.isForbidden(key), key);
            assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of(key, "never-store-this")), key);
        }
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of("Password", "x")));
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of("TOKEN", "x")));
        // Innocent measurement key: deliberately allowed (not startsWith token_ alone).
        assertFalse(AuditMetadata.isForbidden("token_count"));
        assertDoesNotThrow(() -> new AuditMetadata(Map.of("token_count", "3")));
        // Denylist does not inspect values; emitters must not put secrets in values.
        assertDoesNotThrow(() -> new AuditMetadata(Map.of("note", "password=Hunter2")));
    }

    @Test
    void toStringExcludesMetadataValuesAndAuthoritySurfaceHasNoRawPayload() throws Exception {
        var audit = event(UUID.randomUUID(), AuditCategory.AUTHENTICATION, AuditActions.AUTHENTICATION_LOGIN,
                AuditOutcome.SUCCESS, ActorType.USER, ACTOR, TENANT_A, null, null, null,
                Map.of("auth.method", "highly-sensitive-diagnostic-value"));
        assertFalse(audit.toString().contains("highly-sensitive-diagnostic-value"));
        assertTrue(audit.toString().contains("metadataEntries=1"));
        assertEquals("AuditMetadata[entries=1]", audit.metadata().toString());

        for (Field field : SecurityAuditEvent.class.getDeclaredFields()) {
            String name = field.getName().toLowerCase(Locale.ROOT);
            assertFalse(name.contains("password") || name.contains("token") || name.contains("requestbody")
                    || name.contains("responsebody") || name.contains("role") || name.contains("permission")
                    || name.contains("grant") || name.contains("jwt") || name.contains("cookie"), field.getName());
            assertNotEquals(Object.class, field.getType());
            assertFalse(Map.class.isAssignableFrom(field.getType()) && field.getName().equals("headers"));
        }
        Method values = AuditMetadata.class.getMethod("values");
        assertEquals(Map.class, values.getReturnType());
        assertEquals(String.class, ((java.lang.reflect.ParameterizedType) values.getGenericReturnType())
                .getActualTypeArguments()[1]);
    }

    @Test
    void actorUuidIsExplicitAndNotDisplayIdentity() {
        var audit = event(UUID.randomUUID(), AuditCategory.IDENTITY, AuditActions.IDENTITY_MAPPING_RESOLVE,
                AuditOutcome.SUCCESS, ActorType.USER, ACTOR, null, null, null, null,
                Map.of("identity.source", "provider"));
        assertEquals(ACTOR, audit.actorSubjectId());
        assertFalse(audit.metadata().values().containsKey("email"));
        assertFalse(audit.metadata().values().containsKey("username"));
    }

    @Test
    void concurrentReadsAreDeterministic() throws Exception {
        var audit = event(UUID.randomUUID(), AuditCategory.AUTHORIZATION, AuditActions.AUTHORIZATION_ALLOW,
                AuditOutcome.SUCCESS, ActorType.USER, ACTOR, TENANT_A, new AuditResourceType("entity"), RESOURCE,
                null, Map.of("decision.source", "kernel"));
        try (var executor = Executors.newVirtualThreadPerTaskExecutor()) {
            Callable<String> read = () -> audit.eventId() + ":" + audit.correlationId() + ":"
                    + audit.metadata().values().get("decision.source");
            String expected = read.call();
            for (var future : executor.invokeAll(java.util.Collections.nCopies(32, read))) {
                assertEquals(expected, future.get());
            }
        }
    }

    @Test
    void createFactoryUsesRepositoryUuidV7Convention() {
        var audit = SecurityAuditEvent.create(NOW, CORR, AuditCategory.SESSION, new AuditAction("session.login"),
                AuditOutcome.SUCCESS, ActorType.USER, ACTOR, TENANT_A, null, null, null, null, Map.of());
        assertEquals(7, audit.eventId().version());
    }

    @Test
    void mfaChallengeUsesOutcomeRatherThanActionExplosion() {
        var challenged = event(UUID.randomUUID(), AuditCategory.MFA, AuditActions.MFA_CHALLENGE,
                AuditOutcome.FAILURE, ActorType.USER, ACTOR, null, null, null,
                new AuditReasonCode("MFA_REJECTED"), Map.of("factor", "totp"));
        assertEquals(AuditActions.MFA_CHALLENGE, challenged.action());
        assertEquals(AuditOutcome.FAILURE, challenged.outcome());
    }

    private static SecurityAuditEvent event(UUID id, AuditCategory category, AuditAction action, AuditOutcome outcome,
                                            ActorType actorType, UUID actor, UUID tenant, AuditResourceType resourceType,
                                            UUID resourceId, AuditReasonCode reason, Map<String, String> metadata) {
        return new SecurityAuditEvent(id, NOW, CORR, category, action, outcome, actorType, actor, tenant, null,
                resourceType, resourceId, reason, new AuditMetadata(metadata));
    }
}
