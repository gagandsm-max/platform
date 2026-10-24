package com.platform.core.telemetry.read;

import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.core.read.ListAppender;
import org.slf4j.LoggerFactory;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.crypto.RSASSASigner;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;
import com.platform.core.api.EntityMetricObservationController;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.inventory.Entity;
import com.platform.core.domain.inventory.EntityService;
import com.platform.core.domain.inventory.LifecycleState;
import com.platform.core.logging.CorrelationIdFilter;
import com.platform.core.security.authorization.JdbcPermissionGrantDirectory;
import com.platform.core.security.authorization.PermissionGrantDirectory;
import com.platform.core.security.http.*;
import com.platform.core.security.identity.IdentityDirectory;
import com.platform.core.security.identity.JdbcIdentityDirectory;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.SpringBootConfiguration;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.autoconfigure.jdbc.DataSourceAutoConfiguration;
import org.springframework.boot.autoconfigure.kafka.KafkaAutoConfiguration;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Import;
import org.springframework.context.annotation.Primary;
import org.springframework.http.MediaType;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.security.oauth2.core.DelegatingOAuth2TokenValidator;
import org.springframework.security.oauth2.core.OAuth2Error;
import org.springframework.security.oauth2.core.OAuth2TokenValidatorResult;
import org.springframework.security.oauth2.jwt.JwtDecoder;
import org.springframework.security.oauth2.jwt.JwtValidators;
import org.springframework.security.oauth2.jwt.NimbusJwtDecoder;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.MvcResult;
import org.springframework.test.web.servlet.request.MockHttpServletRequestBuilder;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.support.TransactionTemplate;

import javax.sql.DataSource;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.interfaces.RSAPrivateKey;
import java.security.interfaces.RSAPublicKey;
import java.time.Instant;
import java.util.Date;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.content;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.header;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

/**
 * Signed bearer -> real Nimbus/validators -> production Spring security/converter ->
 * real PostgreSQL identity/grant directories -> production metric controller/service.
 * Only EntityService and ClickHouseMetricObservationReader are test boundary doubles.
 * This is NOT physical ClickHouse, customer IdP login, browser or full-stack evidence.
 * SOURCE_ONLY / NOT_RUN until separately admitted execution.
 */
@Tag("postgres")
@SpringBootTest(classes = JdbcMetricObservationSecurityIntegrationTest.TestApplication.class)
@AutoConfigureMockMvc
class JdbcMetricObservationSecurityIntegrationTest {
    private static final String ISSUER = "https://idp.platform.test/realms/platform";
    private static final String AUDIENCE = "platform-core-api";
    private static final String PROVIDER = "platform-oidc";
    private static final String COUNTER = "18446744073709551615";
    private static final String METRIC = ".1.3.6.1.2.1.31.1.1.1.6.1";
    private static final Instant FROM = Instant.parse("2026-10-16T00:00:00Z");
    private static final Instant TO = FROM.plusSeconds(3600);
    private static final UUID TENANT = id("a1");
    private static final UUID OTHER_TENANT = id("b1");
    private static final UUID ENTITY = id("e1");
    private static final UUID OTHER_ENTITY = id("e2");
    private static final UUID GRANTED = id("aa");
    private static final UUID NO_GRANT = id("ab");
    private static final UUID DISABLED = id("ac");
    private static final UUID NO_MEMBERSHIP = id("ad");
    private static final UUID GRANT = id("c1");
    private static final UUID MEMBERSHIP_GRANT = id("c2");
    private static final KeyPair KEY = rsaKeyPair();
    private static final KeyPair WRONG_KEY = rsaKeyPair();
    private static final PostgresTestSupport PG = startPostgres();

    private boolean fixtureOwned;

    @Autowired private MockMvc mvc;
    @Autowired private ObjectMapper mapper;
    @Autowired private JdbcTemplate jdbc;
    @Autowired private PlatformTransactionManager transactionManager;
    @Autowired private IdentityDirectory identities;
    @Autowired private PermissionGrantDirectory grants;
    @Autowired private JwtDecoder decoder;
    // Deliberately labelled data boundaries, not identity/permission/authentication doubles.
    @MockBean private EntityService entities;
    @MockBean private ClickHouseMetricObservationReader reader;

    @DynamicPropertySource
    static void properties(DynamicPropertyRegistry registry) {
        registry.add("platform.security.directories.persistence-enabled", () -> "true");
        registry.add("platform.security.directories.query-timeout", () -> "2s");
        registry.add("platform.security.oidc.issuer-uri", () -> ISSUER);
        registry.add("platform.security.oidc.audience", () -> AUDIENCE);
        registry.add("platform.security.oidc.provider-instance-id", () -> PROVIDER);
    }

    @BeforeAll
    static void immutableTrust() {
        PG.jdbc().update("""
                INSERT INTO platform.identity_provider_instance
                    (provider_instance_id, provider_type, issuer_uri, audience, status)
                VALUES (?, 'OIDC', ?, ?, 'ACTIVE')
                ON CONFLICT (provider_instance_id) DO NOTHING
                """, PROVIDER, ISSUER, AUDIENCE);
        var providers = PG.jdbc().queryForList("""
                SELECT provider_type, issuer_uri, audience, status
                FROM platform.identity_provider_instance WHERE provider_instance_id=?
                """, PROVIDER);
        assertEquals(1, providers.size());
        var provider = providers.get(0);
        assertEquals("OIDC", provider.get("provider_type"));
        assertEquals(ISSUER, provider.get("issuer_uri"));
        assertEquals(AUDIENCE, provider.get("audience"));
        assertEquals("ACTIVE", provider.get("status"));
    }

    @AfterAll
    static void stopPostgres() {
        PG.stop();
    }

    @BeforeEach
    void seed() {
        assertInstanceOf(JdbcIdentityDirectory.class, identities);
        assertInstanceOf(JdbcPermissionGrantDirectory.class, grants);
        assertInstanceOf(NimbusJwtDecoder.class, decoder);
        assertFalse(mockingDetails(identities).isMock());
        assertFalse(mockingDetails(grants).isMock());
        assertFalse(mockingDetails(decoder).isMock());
        new TransactionTemplate(transactionManager).executeWithoutResult(transaction -> {
            refuseExistingFixture();
            jdbc.update("""
                    INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                    VALUES (?, 'Metric A', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW()),
                           (?, 'Metric B', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                    """, TENANT, OTHER_TENANT);
            for (UUID subject : List.of(GRANTED, NO_GRANT, DISABLED, NO_MEMBERSHIP)) {
                jdbc.update("INSERT INTO platform.platform_subject (subject_id, status) VALUES (?, ?)",
                        subject, subject.equals(DISABLED) ? "DISABLED" : "ACTIVE");
                jdbc.update("""
                        INSERT INTO platform.identity_mapping
                            (provider_type, provider_instance_id, external_subject, subject_id, disabled)
                        VALUES ('OIDC', ?, ?, ?, ?)
                        """, PROVIDER, subject.toString(), subject, false);
                // Keep the required membership FK, but make this subject's membership inactive.
                jdbc.update("""
                        INSERT INTO platform.subject_tenant_scope (subject_id, tenant_id, status)
                        VALUES (?, ?, ?)
                        """, subject, TENANT, subject.equals(NO_MEMBERSHIP) ? "REVOKED" : "ACTIVE");
            }
            namedGrant(GRANT, GRANTED);
            // A grant alone is insufficient without active tenant membership.
            namedGrant(MEMBERSHIP_GRANT, NO_MEMBERSHIP);
        });
        fixtureOwned = true; // Claim only the successfully committed seed.
        when(entities.get(TENANT, ENTITY)).thenReturn(new Entity(
                ENTITY, TENANT, "DEVICE", "Metric fixture", null, null, LifecycleState.ACTIVE,
                null, null, null, null, null, FROM, FROM, "test", FROM, FROM));
        when(reader.read(any())).thenReturn(new MetricObservationPage(List.of(
                new MetricObservationRow(FROM.plusSeconds(1), null, COUNTER, 1, 0,
                        "sensor-test", "7", "SNMP")), false));
    }

    private void refuseExistingFixture() {
        for (UUID tenant : List.of(TENANT, OTHER_TENANT)) {
            assertEquals(0, jdbc.queryForObject(
                    "SELECT COUNT(*) FROM platform.tenant WHERE tenant_id=?", Integer.class, tenant));
        }
        for (UUID subject : List.of(GRANTED, NO_GRANT, DISABLED, NO_MEMBERSHIP)) {
            assertEquals(0, jdbc.queryForObject(
                    "SELECT COUNT(*) FROM platform.platform_subject WHERE subject_id=?", Integer.class, subject));
            assertEquals(0, jdbc.queryForObject("""
                    SELECT COUNT(*) FROM platform.identity_mapping
                    WHERE subject_id=? OR (provider_type='OIDC' AND provider_instance_id=? AND external_subject=?)
                    """, Integer.class, subject, PROVIDER, subject.toString()));
            assertEquals(0, jdbc.queryForObject(
                    "SELECT COUNT(*) FROM platform.subject_tenant_scope WHERE subject_id=?", Integer.class, subject));
            assertEquals(0, jdbc.queryForObject(
                    "SELECT COUNT(*) FROM platform.permission_grant WHERE subject_id=?", Integer.class, subject));
        }
        for (UUID grant : List.of(GRANT, MEMBERSHIP_GRANT)) {
            assertEquals(0, jdbc.queryForObject(
                    "SELECT COUNT(*) FROM platform.permission_grant WHERE grant_id=?", Integer.class, grant));
            assertEquals(0, jdbc.queryForObject(
                    "SELECT COUNT(*) FROM platform.permission_grant_resource WHERE grant_id=?", Integer.class, grant));
        }
    }

    @AfterEach
    void cleanOwnedFixture() {
        if (!fixtureOwned) {
            return;
        }
        new TransactionTemplate(transactionManager).executeWithoutResult(transaction -> {
            for (UUID grant : List.of(GRANT, MEMBERSHIP_GRANT)) {
                jdbc.update("DELETE FROM platform.permission_grant_resource WHERE grant_id=? AND resource_id=?", grant, ENTITY);
                jdbc.update("DELETE FROM platform.permission_grant WHERE grant_id=?", grant);
            }
            for (UUID subject : List.of(GRANTED, NO_GRANT, DISABLED, NO_MEMBERSHIP)) {
                jdbc.update("DELETE FROM platform.subject_tenant_scope WHERE subject_id=? AND tenant_id=?", subject, TENANT);
                jdbc.update("""
                        DELETE FROM platform.identity_mapping
                        WHERE provider_type='OIDC' AND provider_instance_id=? AND external_subject=? AND subject_id=?
                        """, PROVIDER, subject.toString(), subject);
                jdbc.update("DELETE FROM platform.platform_subject WHERE subject_id=?", subject);
            }
            jdbc.update("DELETE FROM platform.tenant WHERE tenant_id IN (?, ?)", TENANT, OTHER_TENANT);
        });
        fixtureOwned = false;
    }

    private void namedGrant(UUID grant, UUID subject) {
        jdbc.update("""
                INSERT INTO platform.permission_grant
                    (grant_id, subject_id, tenant_id, permission, resource_type, scope_kind)
                VALUES (?, ?, ?, 'entity.read', 'ENTITY', 'NAMED_RESOURCES')
                """, grant, subject, TENANT);
        jdbc.update("INSERT INTO platform.permission_grant_resource (grant_id, resource_id) VALUES (?, ?)",
                grant, ENTITY);
    }

    @Test
    void signedMappedV7IdentityReachesExactMetricScopeAndPreservesCounter64() throws Exception {
        assertEquals(7, TENANT.version());
        assertEquals(7, ENTITY.version());
        allowed(token(GRANTED.toString()));
    }

    private void allowed(String token) throws Exception {
        mvc.perform(request(TENANT, ENTITY, "100").header("Authorization", "Bearer " + token))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.tenantId").value(TENANT.toString()))
                .andExpect(jsonPath("$.entityId").value(ENTITY.toString()))
                .andExpect(jsonPath("$.returned").value(1))
                .andExpect(jsonPath("$.observations[0].counterValue").value(COUNTER))
                .andExpect(jsonPath("$.observations[0].value").doesNotExist());
        verify(entities).get(TENANT, ENTITY);
        verify(reader).read(new MetricObservationQuery(TENANT, ENTITY,
                AllowedMetricSelector.find(METRIC, MetricValueKind.COUNTER64).orElseThrow(), FROM, TO, 100));
        verifyNoMoreInteractions(reader, entities);
    }

    @Test
    void missingMalformedAndBadSignatureBearerNeverReachData() throws Exception {
        denied(null, TENANT, ENTITY, 401, "AUTH_REQUIRED");
        denied("not-a-jwt", TENANT, ENTITY, 401, "AUTH_TOKEN_INVALID");
        denied(signed(GRANTED.toString(), WRONG_KEY, ISSUER, AUDIENCE, Instant.now().plusSeconds(600)),
                TENANT, ENTITY, 401, "AUTH_TOKEN_INVALID");
    }

    @Test
    void configuredIssuerAudienceAndTimeValidatorsRejectOtherwiseSignedTokens() throws Exception {
        denied(signed(GRANTED.toString(), KEY, "https://wrong.invalid", AUDIENCE, Instant.now().plusSeconds(600)),
                TENANT, ENTITY, 401, "AUTH_TOKEN_INVALID");
        denied(signed(GRANTED.toString(), KEY, ISSUER, "wrong-audience", Instant.now().plusSeconds(600)),
                TENANT, ENTITY, 401, "AUTH_TOKEN_INVALID");
        denied(signed(GRANTED.toString(), KEY, ISSUER, AUDIENCE, Instant.now().minusSeconds(300)),
                TENANT, ENTITY, 401, "AUTH_TOKEN_INVALID");
    }

    @Test
    void unmappedDisabledAndMappedWithoutGrantCannotRead() throws Exception {
        denied(token("unmapped'; SELECT identity_mapping --"), TENANT, ENTITY, 401, "AUTH_TOKEN_INVALID");
        denied(token(DISABLED.toString()), TENANT, ENTITY, 401, "AUTH_TOKEN_INVALID");
        denied(token(NO_GRANT.toString()), TENANT, ENTITY, 404, "RESOURCE_NOT_FOUND");
    }

    @Test
    void disabledMappingCannotUseAnOtherwiseActiveSubject() throws Exception {
        assertEquals(1, jdbc.update("""
                UPDATE platform.identity_mapping SET disabled=TRUE
                WHERE provider_type='OIDC' AND provider_instance_id=? AND external_subject=?
                """, PROVIDER, GRANTED.toString()));
        denied(token(GRANTED.toString()), TENANT, ENTITY, 401, "AUTH_TOKEN_INVALID");
    }

    @Test
    void wrongTenantAndWrongNamedEntityHaveTheSameOpaqueDenial() throws Exception {
        String bearer = token(GRANTED.toString());
        String tenantBody = denied(bearer, OTHER_TENANT, ENTITY, 404, "RESOURCE_NOT_FOUND");
        String entityBody = denied(bearer, TENANT, OTHER_ENTITY, 404, "RESOURCE_NOT_FOUND");
        JsonNode tenantError = mapper.readTree(tenantBody);
        JsonNode entityError = mapper.readTree(entityBody);
        assertEquals(tenantError.get("message"), entityError.get("message"));
        assertEquals("Resource not found", tenantError.get("message").asText());
    }

    @Test
    void inactiveMembershipWithoutActiveGrantsReceivesOpaqueDenial() throws Exception {
        assertEquals(1, jdbc.update("""
                UPDATE platform.permission_grant SET revoked_at=NOW()
                WHERE subject_id=? AND tenant_id=? AND revoked_at IS NULL
                """, NO_MEMBERSHIP, TENANT));
        denied(token(NO_MEMBERSHIP.toString()), TENANT, ENTITY, 404, "RESOURCE_NOT_FOUND");
    }

    @Test
    void activeGrantAgainstInactiveMembershipIsDirectoryCorruption() throws Exception {
        directoryCorruption(token(NO_MEMBERSHIP.toString()));
    }

    @Test
    void atomicGrantAndMembershipRevocationDeniesTheSameSignedToken() throws Exception {
        String bearer = token(GRANTED.toString());
        allowed(bearer);
        clearInvocations(reader, entities);
        new TransactionTemplate(transactionManager).executeWithoutResult(transaction -> {
            assertEquals(1, jdbc.update("""
                    UPDATE platform.permission_grant SET revoked_at=NOW()
                    WHERE subject_id=? AND tenant_id=? AND revoked_at IS NULL
                    """, GRANTED, TENANT));
            assertEquals(1, jdbc.update("""
                    UPDATE platform.subject_tenant_scope SET status='REVOKED'
                    WHERE subject_id=? AND tenant_id=? AND status='ACTIVE'
                    """, GRANTED, TENANT));
        }); // Both authority changes are committed before the same bearer is replayed.
        denied(bearer, TENANT, ENTITY, 404, "RESOURCE_NOT_FOUND");
    }

    @Test
    void membershipOnlyRevocationLeavingActiveGrantIsDirectoryCorruption() throws Exception {
        String bearer = token(GRANTED.toString());
        allowed(bearer);
        clearInvocations(reader, entities);
        assertEquals(1, jdbc.update("""
                UPDATE platform.subject_tenant_scope SET status='REVOKED'
                WHERE subject_id=? AND tenant_id=?
                """, GRANTED, TENANT));
        directoryCorruption(bearer);
    }

    private void directoryCorruption(String bearer) throws Exception {
        Logger auditLogger = (Logger) LoggerFactory.getLogger(SecurityAuditEmitter.class);
        ListAppender<ILoggingEvent> appender = new ListAppender<>();
        appender.start();
        auditLogger.addAppender(appender);
        SecurityContextHolder.clearContext();
        try {
            String correlation = "metric-directory-corruption";
            MvcResult result = mvc.perform(request(TENANT, ENTITY, "100")
                            .header("X-Correlation-ID", correlation)
                            .header("Authorization", "Bearer " + bearer))
                    .andExpect(status().isInternalServerError())
                    .andExpect(content().contentTypeCompatibleWith(MediaType.APPLICATION_JSON))
                    .andExpect(header().string("X-Correlation-ID", correlation))
                    .andExpect(jsonPath("$.code").value("INTERNAL_ERROR"))
                    .andExpect(jsonPath("$.message").value("Unexpected internal error"))
                    .andExpect(jsonPath("$.correlationId").value(correlation))
                    .andExpect(jsonPath("$.timestamp").isNotEmpty())
                    .andReturn();
            assertSanitized(result, bearer);
            var authentication = SecurityContextHolder.getContext().getAuthentication();
            assertTrue(authentication == null || !authentication.isAuthenticated(),
                    "directory corruption must not leave a partially authenticated context");
            String audit = appender.list.stream().map(ILoggingEvent::getFormattedMessage)
                    .reduce("", (left, right) -> left + "\n" + right);
            assertTrue(audit.contains("reason=SECURITY_DIRECTORY_CORRUPT"));
            assertFalse(audit.contains(bearer));
            for (String forbidden : List.of("permission_grant", "subject_tenant_scope", "identity_mapping",
                    "select ", "password", "stacktrace", "authenticationserviceexception")) {
                assertFalse(audit.toLowerCase(java.util.Locale.ROOT).contains(forbidden),
                        "audit must exclude sensitive directory/exception detail: " + forbidden);
            }
            assertTrue(appender.list.stream().allMatch(event -> event.getThrowableProxy() == null),
                    "sanitized audit must not carry a throwable");
            verifyNoInteractions(reader, entities);
        } finally {
            auditLogger.detachAppender(appender);
            appender.stop();
            SecurityContextHolder.clearContext();
        }
    }

    @Test
    void grantRevocationDeniesNextRequestWithTheSameSignedToken() throws Exception {
        String bearer = token(GRANTED.toString());
        allowed(bearer);
        clearInvocations(reader, entities);
        assertEquals(1, jdbc.update("UPDATE platform.permission_grant SET revoked_at=NOW() WHERE grant_id=?", GRANT));
        denied(bearer, TENANT, ENTITY, 404, "RESOURCE_NOT_FOUND");
    }

    @Test
    void validAuthorityDoesNotBypassQueryLimitValidation() throws Exception {
        String bearer = token(GRANTED.toString());
        MvcResult result = mvc.perform(request(TENANT, ENTITY, "501")
                        .header("Authorization", "Bearer " + bearer))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("QUERY_LIMIT_EXCEEDED"))
                .andReturn();
        assertSanitized(result, bearer);
        verifyNoInteractions(reader, entities);
    }

    private String denied(String bearer, UUID tenant, UUID entity, int status, String code) throws Exception {
        MockHttpServletRequestBuilder request = request(tenant, entity, "100");
        if (bearer != null) {
            request.header("Authorization", "Bearer " + bearer);
        }
        MvcResult result = mvc.perform(request).andExpect(status().is(status))
                .andExpect(jsonPath("$.code").value(code)).andReturn();
        assertSanitized(result, bearer);
        verifyNoInteractions(reader, entities);
        return result.getResponse().getContentAsString();
    }

    private void assertSanitized(MvcResult result, String bearer) throws Exception {
        String body = result.getResponse().getContentAsString();
        JsonNode error = mapper.readTree(body);
        Set<String> fields = new HashSet<>();
        error.fieldNames().forEachRemaining(fields::add);
        assertEquals(Set.of("code", "message", "correlationId", "timestamp"), fields);
        if (bearer != null) {
            assertFalse(body.contains(bearer));
        }
        for (String forbidden : List.of("Bearer ", "SELECT ", "identity_mapping", "permission_grant",
                "stackTrace", "stacktrace", "org.postgresql", "java.lang", "unmapped'")) {
            assertFalse(body.contains(forbidden), "error must not expose bearer, SQL or stack details");
        }
    }

    private static MockHttpServletRequestBuilder request(UUID tenant, UUID entity, String limit) {
        return get("/api/v1/tenants/" + tenant + "/entities/" + entity + "/metric-observations")
                .param("metricId", METRIC).param("valueKind", "COUNTER64")
                .param("from", FROM.toString()).param("to", TO.toString()).param("limit", limit);
    }

    private static String token(String subject) throws Exception {
        return signed(subject, KEY, ISSUER, AUDIENCE, Instant.now().plusSeconds(600));
    }

    private static String signed(String subject, KeyPair key, String issuer, String audience, Instant expiry)
            throws Exception {
        SignedJWT jwt = new SignedJWT(new JWSHeader(JWSAlgorithm.RS256), new JWTClaimsSet.Builder()
                .issuer(issuer).subject(subject).audience(audience)
                .issueTime(Date.from(Instant.now().minusSeconds(600)))
                .notBeforeTime(Date.from(Instant.now().minusSeconds(600)))
                .expirationTime(Date.from(expiry)).build());
        jwt.sign(new RSASSASigner((RSAPrivateKey) key.getPrivate()));
        return jwt.serialize();
    }

    private static UUID id(String suffix) {
        return UUID.fromString("018f2542-0000-7000-8000-0000000000" + suffix);
    }

    private static KeyPair rsaKeyPair() {
        try {
            KeyPairGenerator generator = KeyPairGenerator.getInstance("RSA");
            generator.initialize(2048);
            return generator.generateKeyPair();
        } catch (Exception ex) {
            throw new IllegalStateException("Cannot create test-only signing key", ex);
        }
    }

    private static PostgresTestSupport startPostgres() {
        PostgresTestSupport pg = new PostgresTestSupport();
        try {
            pg.start();
            pg.migrate();
            return pg;
        } catch (Exception ex) {
            pg.stop();
            throw new IllegalStateException("Cannot initialize isolated metric security PostgreSQL fixture", ex);
        }
    }

    @SpringBootConfiguration
    @EnableAutoConfiguration(exclude = {DataSourceAutoConfiguration.class, KafkaAutoConfiguration.class})
    @Import({HttpSecurityConfiguration.class, PlatformJwtAuthenticationConverter.class,
            AuthorizationRouteMapper.class, AuthorizationEnforcementFilter.class,
            SecurityErrorResponseWriter.class, SecurityAuditEmitter.class, SecurityDirectoryConfiguration.class,
            ApiExceptionHandler.class, CorrelationIdFilter.class, EntityMetricObservationController.class,
            EntityMetricObservationService.class, PersistenceAndJwtFixtures.class})
    static class TestApplication { }

    @Configuration
    static class PersistenceAndJwtFixtures {
        @Bean @Primary DataSource dataSource() { return PG.dataSource(); }
        @Bean JdbcTemplate jdbcTemplate(DataSource dataSource) { return new JdbcTemplate(dataSource); }
        @Bean(name = "transactionManager")
        PlatformTransactionManager transactionManager(DataSource dataSource) {
            return new DataSourceTransactionManager(dataSource);
        }
        @Bean @Primary
        OidcResourceServerProperties oidcResourceServerProperties() {
            OidcResourceServerProperties props = new OidcResourceServerProperties();
            props.setIssuerUri(ISSUER);
            props.setAudience(AUDIENCE);
            props.setProviderInstanceId(PROVIDER);
            return props;
        }
        @Bean @Primary
        JwtDecoder jwtDecoder(OidcResourceServerProperties properties) {
            // Existing JDBC HTTP convention: real RS256 Nimbus, local test key instead of network JWKS.
            // Same timestamp/issuer/audience policy as JwtDecoderConfiguration; negatives above exercise it.
            NimbusJwtDecoder decoder = NimbusJwtDecoder.withPublicKey((RSAPublicKey) KEY.getPublic()).build();
            decoder.setJwtValidator(new DelegatingOAuth2TokenValidator<>(
                    JwtValidators.createDefaultWithIssuer(properties.getIssuerUri()),
                    jwt -> jwt.getAudience().contains(properties.getAudience())
                            ? OAuth2TokenValidatorResult.success()
                            : OAuth2TokenValidatorResult.failure(new OAuth2Error(
                                    "invalid_token", "Required audience is missing", null))));
            return decoder;
        }
    }
}
