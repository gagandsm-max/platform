package com.platform.core.security.audit;

import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.LinkedHashMap;
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

    @Test void authenticationAuthorizationAndCredentialActionsArePrecise() {
        var authOk = event(UUID.randomUUID(), AuditCategory.AUTHENTICATION, AuditActions.AUTHENTICATION_LOGIN,
                AuditOutcome.SUCCESS, ActorType.USER, ACTOR, null, null, null, null, Map.of());
        var authFail = event(UUID.randomUUID(), AuditCategory.AUTHENTICATION, AuditActions.AUTHENTICATION_LOGIN,
                AuditOutcome.FAILURE, ActorType.USER, null, null, null, null, new AuditReasonCode("AUTHENTICATION_FAILED"), Map.of());
        var deny = event(UUID.randomUUID(), AuditCategory.AUTHORIZATION, AuditActions.AUTHORIZATION_DENY,
                AuditOutcome.DENIED, ActorType.USER, ACTOR, TENANT_A, new AuditResourceType("entity"), RESOURCE,
                new AuditReasonCode("PERMISSION_DENIED"), Map.of());
        assertEquals(ACTOR, authOk.actorSubjectId()); assertNull(authFail.actorSubjectId());
        assertEquals(AuditOutcome.DENIED, deny.outcome());
        assertNotEquals(AuditActions.CREDENTIAL_USE, AuditActions.CREDENTIAL_EDIT);
        assertNotEquals(AuditActions.CREDENTIAL_USE, AuditActions.CREDENTIAL_REVEAL);
        assertNotEquals(AuditActions.CREDENTIAL_EDIT, AuditActions.CREDENTIAL_REVEAL);
        for (AuditAction a : new AuditAction[]{AuditActions.CREDENTIAL_USE, AuditActions.CREDENTIAL_EDIT, AuditActions.CREDENTIAL_REVEAL})
            assertEquals(a, event(UUID.randomUUID(), AuditCategory.CREDENTIAL, a, AuditOutcome.SUCCESS,
                    ActorType.USER, ACTOR, TENANT_A, null, null, null, Map.of()).action());
    }

    @Test void eventIdentityIsIndependentOfCorrelation() {
        var a = event(UUID.randomUUID(), AuditCategory.SESSION, new AuditAction("session.logout"), AuditOutcome.SUCCESS,
                ActorType.USER, ACTOR, TENANT_A, null, null, null, Map.of());
        var b = event(UUID.randomUUID(), AuditCategory.SESSION, new AuditAction("session.logout"), AuditOutcome.SUCCESS,
                ActorType.USER, ACTOR, TENANT_A, null, null, null, Map.of());
        assertEquals(a.correlationId(), b.correlationId()); assertNotEquals(a.eventId(), b.eventId()); assertNotEquals(a, b);
    }

    @Test void requiredFieldsFailClosed() {
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
                AuditActions.AUTHENTICATION_LOGIN, null, ActorType.USER, null, null, null, null, null, null, AuditMetadata.empty()));
        assertThrows(NullPointerException.class, () -> new SecurityAuditEvent(id, NOW, CORR, AuditCategory.AUTHENTICATION,
                AuditActions.AUTHENTICATION_LOGIN, AuditOutcome.SUCCESS, null, null, null, null, null, null, null, AuditMetadata.empty()));
    }

    @Test void correlationActionAndReasonGrammarsAreBounded() {
        assertEquals("req-01:abc.def_2", CORR.value());
        assertThrows(IllegalArgumentException.class, () -> new CorrelationId(" "));
        assertThrows(IllegalArgumentException.class, () -> new CorrelationId("bad\nvalue"));
        assertThrows(IllegalArgumentException.class, () -> new CorrelationId("x".repeat(AuditLimits.CORRELATION_ID_MAX + 1)));
        assertThrows(IllegalArgumentException.class, () -> new AuditAction("Authentication.Login"));
        assertThrows(IllegalArgumentException.class, () -> new AuditAction("login"));
        assertThrows(IllegalArgumentException.class, () -> new AuditAction("authentication..login"));
        String action = "a." + "b".repeat(AuditLimits.ACTION_MAX - 2);
        assertEquals(AuditLimits.ACTION_MAX, new AuditAction(action).value().length());
        assertThrows(IllegalArgumentException.class, () -> new AuditAction(action + "b"));
        assertEquals("AUTHENTICATION_FAILED", new AuditReasonCode("AUTHENTICATION_FAILED").value());
        assertThrows(IllegalArgumentException.class, () -> new AuditReasonCode("authentication failed"));
        assertThrows(IllegalArgumentException.class, () -> new AuditReasonCode("AUTH\nFAIL"));
    }

    @Test void tenantResourceAndNetworkSourceStayExplicitAndNonAuthoritative() {
        var preAuth = event(UUID.randomUUID(), AuditCategory.AUTHENTICATION, AuditActions.AUTHENTICATION_LOGIN,
                AuditOutcome.FAILURE, ActorType.USER, null, null, null, null, null, Map.of());
        assertNull(preAuth.tenantId()); assertNull(preAuth.resourceType()); assertNull(preAuth.resourceId());
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
        assertNotEquals(a.tenantId(), b.tenantId()); assertEquals(a.resourceId(), b.resourceId());
        var source = new NetworkSourceAddress("2001:db8::10");
        var sourceEvent = new SecurityAuditEvent(UUID.randomUUID(), NOW, CORR, AuditCategory.AUTHENTICATION,
                AuditActions.AUTHENTICATION_LOGIN, AuditOutcome.FAILURE, ActorType.USER, null, null, source,
                null, null, new AuditReasonCode("AUTHENTICATION_FAILED"), AuditMetadata.empty());
        assertEquals(source, sourceEvent.sourceAddress()); assertNull(sourceEvent.actorSubjectId());
        assertThrows(IllegalArgumentException.class, () -> new NetworkSourceAddress("bad\naddress"));
    }

    @Test void metadataIsImmutableBoundedAndNullSafe() {
        var input = new LinkedHashMap<String,String>(); input.put("auth.method", "oidc");
        var metadata = new AuditMetadata(input); input.put("later", "mutation");
        assertEquals(Map.of("auth.method", "oidc"), metadata.values());
        assertThrows(UnsupportedOperationException.class, () -> metadata.values().put("x", "y"));
        var tooMany = new LinkedHashMap<String,String>();
        for (int i=0; i<=AuditLimits.METADATA_MAX_ENTRIES; i++) tooMany.put("key"+i, "v");
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(tooMany));
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of("k".repeat(AuditLimits.METADATA_KEY_MAX+1), "v")));
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of("key", "v".repeat(AuditLimits.METADATA_VALUE_MAX+1))));
        var nullKey = new LinkedHashMap<String,String>(); nullKey.put(null,"v");
        var nullValue = new LinkedHashMap<String,String>(); nullValue.put("key",null);
        assertThrows(NullPointerException.class, () -> new AuditMetadata(nullKey));
        assertThrows(NullPointerException.class, () -> new AuditMetadata(nullValue));
        var big = new LinkedHashMap<String,String>(); for(int i=0;i<17;i++) big.put("safe"+i,"x".repeat(500));
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(big));
    }

    @Test void forbiddenSecretMetadataFailsClosed() {
        for (String key : new String[]{"password","access_token","auth.token","http.authorization","session.cookie",
                "client_secret","private_key","api_key","credential_value"})
            assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of(key,"never-store-this")), key);
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of("Password","x")));
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of("TOKEN","x")));
        assertThrows(IllegalArgumentException.class, () -> new AuditMetadata(Map.of("safe","bad\nvalue")));
    }

    @Test void toStringExcludesMetadataValuesAndAuthoritySurfaceHasNoRawPayload() throws Exception {
        var audit = event(UUID.randomUUID(), AuditCategory.AUTHENTICATION, AuditActions.AUTHENTICATION_LOGIN,
                AuditOutcome.SUCCESS, ActorType.USER, ACTOR, TENANT_A, null, null, null,
                Map.of("auth.method","highly-sensitive-diagnostic-value"));
        assertFalse(audit.toString().contains("highly-sensitive-diagnostic-value"));
        assertTrue(audit.toString().contains("metadataEntries=1"));
        for (var field : SecurityAuditEvent.class.getDeclaredFields()) {
            String name = field.getName().toLowerCase();
            assertFalse(name.contains("password") || name.contains("token") || name.contains("requestbody")
                    || name.contains("responsebody") || name.contains("role") || name.contains("permission"), field.getName());
            assertNotEquals(Object.class, field.getType());
        }
        assertEquals(Map.class, AuditMetadata.class.getMethod("values").getReturnType());
    }

    @Test void actorUuidIsExplicitAndNotDisplayIdentity() {
        var audit = event(UUID.randomUUID(), AuditCategory.IDENTITY, AuditActions.IDENTITY_MAPPING_RESOLVE,
                AuditOutcome.SUCCESS, ActorType.USER, ACTOR, null, null, null, null,
                Map.of("identity.source","provider"));
        assertEquals(ACTOR, audit.actorSubjectId());
        assertFalse(audit.metadata().values().containsKey("email"));
        assertFalse(audit.metadata().values().containsKey("username"));
    }

    @Test void concurrentReadsAreDeterministic() throws Exception {
        var audit = event(UUID.randomUUID(), AuditCategory.AUTHORIZATION, AuditActions.AUTHORIZATION_ALLOW,
                AuditOutcome.SUCCESS, ActorType.USER, ACTOR, TENANT_A, new AuditResourceType("entity"), RESOURCE,
                null, Map.of("decision.source","kernel"));
        try (var executor = Executors.newVirtualThreadPerTaskExecutor()) {
            Callable<String> read = () -> audit.eventId()+":"+audit.correlationId()+":"+audit.metadata().values().get("decision.source");
            String expected=read.call();
            for (var future : executor.invokeAll(java.util.Collections.nCopies(32,read))) assertEquals(expected,future.get());
        }
    }

    @Test void createFactoryUsesRepositoryUuidV7Convention() {
        var audit = SecurityAuditEvent.create(NOW, CORR, AuditCategory.SESSION, new AuditAction("session.login"),
                AuditOutcome.SUCCESS, ActorType.USER, ACTOR, TENANT_A, null, null, null, null, Map.of());
        assertEquals(7, audit.eventId().version());
    }

    private static SecurityAuditEvent event(UUID id, AuditCategory category, AuditAction action, AuditOutcome outcome,
                                            ActorType actorType, UUID actor, UUID tenant, AuditResourceType resourceType,
                                            UUID resourceId, AuditReasonCode reason, Map<String,String> metadata) {
        return new SecurityAuditEvent(id, NOW, CORR, category, action, outcome, actorType, actor, tenant, null,
                resourceType, resourceId, reason, new AuditMetadata(metadata));
    }
}
