package com.platform.core.security.http;

import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.crypto.RSASSASigner;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;
import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.core.read.ListAppender;
import com.platform.core.api.EntityController;
import com.platform.core.api.PageResult;
import com.platform.core.api.TenantController;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.controller.VersionController;
import com.platform.core.domain.inventory.Entity;
import com.platform.core.domain.inventory.EntityService;
import com.platform.core.domain.inventory.LifecycleState;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.logging.CorrelationIdFilter;
import com.platform.core.security.authorization.JdbcPermissionGrantDirectory;
import com.platform.core.security.authorization.PermissionGrantDirectory;
import com.platform.core.security.identity.IdentityDirectory;
import com.platform.core.security.identity.JdbcIdentityDirectory;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
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
import org.springframework.security.oauth2.jwt.JwtDecoder;
import org.springframework.security.oauth2.jwt.NimbusJwtDecoder;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.transaction.PlatformTransactionManager;

import javax.sql.DataSource;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.interfaces.RSAPrivateKey;
import java.security.interfaces.RSAPublicKey;
import java.time.Instant;
import java.util.Date;
import java.util.List;
import java.util.Map;
import java.util.UUID;

import static org.hamcrest.Matchers.containsString;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.content;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.header;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

/**
 * Real JDBC directories on existing main HTTP paths (tenant read / entity read). Not PR246.
 * SOURCE_DRAFT / NOT_RUN.
 */
@Tag("postgres")
@SpringBootTest(classes = JdbcSecurityDirectoryHttpIntegrationTest.TestApplication.class)
@AutoConfigureMockMvc
class JdbcSecurityDirectoryHttpIntegrationTest {

    private static final String ISSUER = "https://idp.platform.test/realms/platform";
    private static final String AUDIENCE = "platform-core-api";
    private static final String PROVIDER = "platform-oidc";
    private static final KeyPair KEY_PAIR = rsaKeyPair();

    private static final UUID TENANT_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID TENANT_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b1");
    private static final UUID SUBJECT_GRANTED = UUID.fromString("018f0e0a-0000-7000-8000-0000000000aa");
    private static final UUID SUBJECT_NO_GRANTS = UUID.fromString("018f0e0a-0000-7000-8000-0000000000ab");
    private static final UUID SUBJECT_DISABLED = UUID.fromString("018f0e0a-0000-7000-8000-0000000000ac");
    private static final UUID SUBJECT_NAMED = UUID.fromString("018f0e0a-0000-7000-8000-0000000000ad");
    private static final UUID GRANT_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c1");
    private static final UUID GRANT_NAMED = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c2");
    private static final UUID ENTITY_E1 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");
    private static final UUID ENTITY_E2 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e2");

    private static final PostgresTestSupport PG = startPostgres();

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private IdentityDirectory identityDirectory;

    @Autowired
    private PermissionGrantDirectory permissionGrantDirectory;

    @Autowired
    private JdbcTemplate jdbcTemplate;

    @MockBean
    private TenantService tenantService;

    @MockBean
    private EntityService entityService;

    @MockBean
    private org.springframework.boot.actuate.health.HealthEndpoint healthEndpoint;

    @DynamicPropertySource
    static void directoryProps(DynamicPropertyRegistry registry) {
        registry.add("platform.security.directories.persistence-enabled", () -> "true");
        registry.add("platform.security.directories.query-timeout", () -> "2s");
        registry.add("platform.security.oidc.issuer-uri", () -> ISSUER);
        registry.add("platform.security.oidc.audience", () -> AUDIENCE);
        registry.add("platform.security.oidc.provider-instance-id", () -> PROVIDER);
    }

    @BeforeAll
    static void seedImmutableProviderTrust() {
        // Preserve provider trust across tests: never DELETE identity_provider_instance
        // (trigger forbids delete/retarget). Seed once if absent.
        Integer count = PG.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.identity_provider_instance WHERE provider_instance_id=?",
                Integer.class, PROVIDER);
        if (count != null && count == 0) {
            PG.jdbc().update("""
                    INSERT INTO platform.identity_provider_instance
                        (provider_instance_id, provider_type, issuer_uri, audience, status)
                    VALUES (?, 'OIDC', ?, ?, 'ACTIVE')
                    """, PROVIDER, ISSUER, AUDIENCE);
        }
    }

    @AfterAll
    static void stopPostgres() {
        PG.stop();
    }

    @BeforeEach
    void seedMutableDataAndStub() {
        when(healthEndpoint.health()).thenReturn(org.springframework.boot.actuate.health.Health.up().build());
        Tenant tenantA = new Tenant(
                TENANT_A, "Tenant A", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC",
                Instant.parse("2026-01-01T00:00:00Z"), Instant.parse("2026-01-01T00:00:00Z")
        );
        Tenant tenantB = new Tenant(
                TENANT_B, "Tenant B", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC",
                Instant.parse("2026-01-01T00:00:00Z"), Instant.parse("2026-01-01T00:00:00Z")
        );
        when(tenantService.get(TENANT_A)).thenReturn(tenantA);
        when(tenantService.get(TENANT_B)).thenReturn(tenantB);
        Instant t0 = Instant.parse("2026-01-01T00:00:00Z");
        Entity entityE1 = new Entity(
                ENTITY_E1, TENANT_A, "DEVICE", "E1", null, null, LifecycleState.ACTIVE,
                null, null, null, null, null, t0, t0, "test", t0, t0
        );
        Entity entityE2 = new Entity(
                ENTITY_E2, TENANT_A, "DEVICE", "E2", null, null, LifecycleState.ACTIVE,
                null, null, null, null, null, t0, t0, "test", t0, t0
        );
        when(entityService.get(eq(TENANT_A), eq(ENTITY_E1))).thenReturn(entityE1);
        when(entityService.get(eq(TENANT_A), eq(ENTITY_E2))).thenReturn(entityE2);
        // Otherwise-successful nonempty collection result — collection denial must not depend on missing data.
        when(entityService.list(eq(TENANT_A), anyInt(), anyInt())).thenReturn(
                new PageResult<>(List.of(entityE1), 1, 50, 1L, false));

        // Mutable tables only — do not touch identity_provider_instance (immutable trust seed).
        jdbcTemplate.update("DELETE FROM platform.permission_grant_resource");
        jdbcTemplate.update("DELETE FROM platform.permission_grant");
        jdbcTemplate.update("DELETE FROM platform.subject_tenant_scope");
        jdbcTemplate.update("DELETE FROM platform.identity_mapping");
        jdbcTemplate.update("DELETE FROM platform.platform_subject");
        jdbcTemplate.update("DELETE FROM platform.tenant WHERE tenant_id IN (?, ?)", TENANT_A, TENANT_B);

        jdbcTemplate.update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?, 'Tenant A', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW()),
                       (?, 'Tenant B', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                """, TENANT_A, TENANT_B);
        jdbcTemplate.update("""
                INSERT INTO platform.platform_subject (subject_id, status)
                VALUES (?, 'ACTIVE'), (?, 'ACTIVE'), (?, 'ACTIVE'), (?, 'ACTIVE')
                """, SUBJECT_GRANTED, SUBJECT_NO_GRANTS, SUBJECT_DISABLED, SUBJECT_NAMED);
        jdbcTemplate.update("""
                INSERT INTO platform.identity_mapping
                    (provider_type, provider_instance_id, external_subject, subject_id, disabled)
                VALUES ('OIDC', ?, 'user-granted', ?, FALSE),
                       ('OIDC', ?, 'user-no-grants', ?, FALSE),
                       ('OIDC', ?, 'user-disabled', ?, TRUE),
                       ('OIDC', ?, 'user-named', ?, FALSE)
                """, PROVIDER, SUBJECT_GRANTED, PROVIDER, SUBJECT_NO_GRANTS,
                PROVIDER, SUBJECT_DISABLED, PROVIDER, SUBJECT_NAMED);

        jdbcTemplate.update("""
                INSERT INTO platform.subject_tenant_scope (subject_id, tenant_id, status)
                VALUES (?, ?, 'ACTIVE'), (?, ?, 'ACTIVE')
                """, SUBJECT_GRANTED, TENANT_A, SUBJECT_NAMED, TENANT_A);
        jdbcTemplate.update("""
                INSERT INTO platform.permission_grant
                    (grant_id, subject_id, tenant_id, permission, resource_type, scope_kind)
                VALUES (?, ?, ?, 'tenant.read', 'TENANT', 'TENANT_WIDE')
                """, GRANT_A, SUBJECT_GRANTED, TENANT_A);
        jdbcTemplate.update("""
                INSERT INTO platform.permission_grant
                    (grant_id, subject_id, tenant_id, permission, resource_type, scope_kind)
                VALUES (?, ?, ?, 'entity.read', 'ENTITY', 'NAMED_RESOURCES')
                """, GRANT_NAMED, SUBJECT_NAMED, TENANT_A);
        jdbcTemplate.update("""
                INSERT INTO platform.permission_grant_resource (grant_id, resource_id)
                VALUES (?, ?)
                """, GRANT_NAMED, ENTITY_E1);

        assertInstanceOf(JdbcIdentityDirectory.class, identityDirectory);
        assertInstanceOf(JdbcPermissionGrantDirectory.class, permissionGrantDirectory);
        Integer providers = jdbcTemplate.queryForObject(
                "SELECT COUNT(*) FROM platform.identity_provider_instance WHERE provider_instance_id=?",
                Integer.class, PROVIDER);
        assertTrue(providers != null && providers == 1);
    }

    @Test
    void mappedPrincipalCanReadTenantViaRealJdbcPath() throws Exception {
        String token = signedJwt("user-granted", Map.of());
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.name").value("Tenant A"));
        verify(tenantService).get(TENANT_A);
    }

    @Test
    void signedJwtRoleClaimsProvideNoGrantsOnRealJdbcFilterPath() throws Exception {
        String token = signedJwt("user-no-grants", Map.of(
                "roles", List.of("ADMIN"),
                "permissions", List.of("tenant.read")
        ));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));
        verify(tenantService, never()).get(any());
    }

    @Test
    void disabledMappingRejected() throws Exception {
        String token = signedJwt("user-disabled", Map.of());
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value(containsString("AUTH")));
        verify(tenantService, never()).get(any());
    }

    @Test
    void committedRevocationObservedOnLaterRequest() throws Exception {
        String token = signedJwt("user-granted", Map.of());
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isOk());

        jdbcTemplate.update(
                "UPDATE platform.permission_grant SET revoked_at = NOW() WHERE grant_id = ?", GRANT_A);

        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));
    }

    @Test
    void crossTenantTenantReadDeniedWithoutServiceInvocation() throws Exception {
        // Tenant B exists and would succeed if authorized — deny must be authz, not missing resource.
        String token = signedJwt("user-granted", Map.of());
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_B)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));
        verify(tenantService, never()).get(TENANT_B);
    }

    @Test
    void namedEntityE1AllowedE2AndCollectionDeniedWithoutServiceInvocation() throws Exception {
        String token = signedJwt("user-named", Map.of());
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A + "/entities/" + ENTITY_E1)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isOk());
        verify(entityService).get(TENANT_A, ENTITY_E1);

        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A + "/entities/" + ENTITY_E2)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));
        verify(entityService, never()).get(TENANT_A, ENTITY_E2);

        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A + "/entities")
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));
        verify(entityService, never()).list(any(), anyInt(), anyInt());
    }

    @Test
    void directoryCorruptionReturnsOpaque500WithoutPartialAuthority() throws Exception {
        jdbcTemplate.update("""
                UPDATE platform.permission_grant
                   SET resource_type = 'ENTITY'
                 WHERE grant_id = ?
                """, GRANT_A);
        String token = signedJwt("user-granted", Map.of());

        Logger auditLogger = (Logger) LoggerFactory.getLogger(SecurityAuditEmitter.class);
        ListAppender<ILoggingEvent> appender = new ListAppender<>();
        appender.start();
        auditLogger.addAppender(appender);
        SecurityContextHolder.clearContext();
        try {
            String correlation = "jdbc-directory-corruption";
            mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                            .header("X-Correlation-ID", correlation)
                            .header("Authorization", "Bearer " + token))
                    .andExpect(status().isInternalServerError())
                    .andExpect(content().contentTypeCompatibleWith(MediaType.APPLICATION_JSON))
                    .andExpect(header().string("X-Correlation-ID", correlation))
                    .andExpect(jsonPath("$.code").value("INTERNAL_ERROR"))
                    .andExpect(jsonPath("$.message").value("Unexpected internal error"))
                    .andExpect(jsonPath("$.correlationId").value(correlation))
                    .andExpect(jsonPath("$.timestamp").isNotEmpty());
            assertTrue(
                    SecurityContextHolder.getContext().getAuthentication() == null
                            || !SecurityContextHolder.getContext().getAuthentication().isAuthenticated(),
                    "no partial authenticated SecurityContext after directory corruption");
            String audit = appender.list.stream()
                    .map(ILoggingEvent::getFormattedMessage)
                    .reduce("", (a, b) -> a + "\n" + b);
            assertTrue(audit.contains("reason=SECURITY_DIRECTORY_CORRUPT"), audit);
            assertFalse(audit.toLowerCase().contains("permission_grant"));
            assertFalse(audit.toLowerCase().contains("password"));
            verify(tenantService, never()).get(any());
        } finally {
            auditLogger.detachAppender(appender);
            appender.stop();
            SecurityContextHolder.clearContext();
        }
    }

    @Test
    void unmappedSubjectUnauthorizedWithoutSqlLeak() throws Exception {
        String token = signedJwt("user-unknown", Map.of());
        String body = mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isUnauthorized())
                .andReturn()
                .getResponse()
                .getContentAsString();
        assertFalse(body.toLowerCase().contains("identity_mapping"));
        assertFalse(body.toLowerCase().contains("sql"));
        assertFalse(body.toLowerCase().contains("stacktrace"));
        verify(tenantService, never()).get(any());
    }

    private static String signedJwt(String subject, Map<String, Object> extra) throws Exception {
        JWTClaimsSet.Builder builder = new JWTClaimsSet.Builder()
                .issuer(ISSUER)
                .subject(subject)
                .audience(AUDIENCE)
                .issueTime(Date.from(Instant.now().minusSeconds(30)))
                .notBeforeTime(Date.from(Instant.now().minusSeconds(30)))
                .expirationTime(Date.from(Instant.now().plusSeconds(600)));
        extra.forEach(builder::claim);
        SignedJWT jwt = new SignedJWT(new JWSHeader(JWSAlgorithm.RS256), builder.build());
        jwt.sign(new RSASSASigner((RSAPrivateKey) KEY_PAIR.getPrivate()));
        return jwt.serialize();
    }

    private static PostgresTestSupport startPostgres() {
        try {
            PostgresTestSupport pg = new PostgresTestSupport();
            pg.start();
            pg.migrate();
            return pg;
        } catch (Exception ex) {
            throw new IllegalStateException("Failed to start PostgresTestSupport for JDBC HTTP IT", ex);
        }
    }

    private static KeyPair rsaKeyPair() {
        try {
            KeyPairGenerator generator = KeyPairGenerator.getInstance("RSA");
            generator.initialize(2048);
            return generator.generateKeyPair();
        } catch (Exception ex) {
            throw new IllegalStateException(ex);
        }
    }

    @SpringBootConfiguration
    @EnableAutoConfiguration(exclude = {DataSourceAutoConfiguration.class, KafkaAutoConfiguration.class})
    @Import({
            HttpSecurityConfiguration.class,
            PlatformJwtAuthenticationConverter.class,
            AuthorizationRouteMapper.class,
            AuthorizationEnforcementFilter.class,
            SecurityErrorResponseWriter.class,
            SecurityAuditEmitter.class,
            SecurityDirectoryConfiguration.class,
            ApiExceptionHandler.class,
            CorrelationIdFilter.class,
            TenantController.class,
            EntityController.class,
            VersionController.class,
            JdbcHttpPersistenceBeans.class
    })
    static class TestApplication {
    }

    @Configuration
    static class JdbcHttpPersistenceBeans {
        @Bean
        @Primary
        DataSource dataSource() {
            return PG.dataSource();
        }

        @Bean
        JdbcTemplate jdbcTemplate(DataSource dataSource) {
            return new JdbcTemplate(dataSource);
        }

        @Bean(name = "transactionManager")
        PlatformTransactionManager transactionManager(DataSource dataSource) {
            return new DataSourceTransactionManager(dataSource);
        }

        @Bean
        @Primary
        OidcResourceServerProperties oidcResourceServerProperties() {
            OidcResourceServerProperties props = new OidcResourceServerProperties();
            props.setIssuerUri(ISSUER);
            props.setAudience(AUDIENCE);
            props.setProviderInstanceId(PROVIDER);
            return props;
        }

        @Bean
        @Primary
        JwtDecoder jwtDecoder() {
            NimbusJwtDecoder decoder = NimbusJwtDecoder.withPublicKey((RSAPublicKey) KEY_PAIR.getPublic()).build();
            decoder.setJwtValidator(token -> {
                var result = org.springframework.security.oauth2.jwt.JwtValidators.createDefault().validate(token);
                if (result.hasErrors()) {
                    return result;
                }
                if (!ISSUER.equals(token.getIssuer().toString())) {
                    return org.springframework.security.oauth2.core.OAuth2TokenValidatorResult.failure(
                            new org.springframework.security.oauth2.core.OAuth2Error("invalid_token", "issuer", null));
                }
                if (token.getAudience() == null || !token.getAudience().contains(AUDIENCE)) {
                    return org.springframework.security.oauth2.core.OAuth2TokenValidatorResult.failure(
                            new org.springframework.security.oauth2.core.OAuth2Error("invalid_token", "audience", null));
                }
                return org.springframework.security.oauth2.core.OAuth2TokenValidatorResult.success();
            });
            return decoder;
        }
    }
}
