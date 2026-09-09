package com.platform.core.security.http;

import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.crypto.MACSigner;
import com.nimbusds.jose.crypto.RSASSASigner;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;
import com.platform.core.api.TenantController;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.controller.SensorIngestionController;
import com.platform.core.controller.VersionController;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.logging.CorrelationIdFilter;
import com.platform.core.security.authorization.PermissionGrant;
import com.platform.core.security.authorization.PermissionGrantDirectory;
import com.platform.core.security.authorization.PlatformPermissions;
import com.platform.core.security.authorization.ResourceScope;
import com.platform.core.security.authorization.ResourceType;
import com.platform.core.security.identity.DirectoryIdentityResolver;
import com.platform.core.security.identity.IdentityDirectory;
import com.platform.core.security.identity.IdentityProviderType;
import com.platform.core.security.identity.IdentityResolver;
import com.platform.core.security.identity.PlatformSubject;
import org.junit.jupiter.api.BeforeEach;
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
import org.springframework.security.oauth2.jwt.JwtDecoder;
import org.springframework.security.oauth2.jwt.NimbusJwtDecoder;
import org.springframework.test.web.servlet.MockMvc;

import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.interfaces.RSAPrivateKey;
import java.security.interfaces.RSAPublicKey;
import java.time.Instant;
import java.util.Date;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;

import static org.hamcrest.Matchers.containsString;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest(classes = CoreHttpSecurityWebTest.TestApplication.class)
@AutoConfigureMockMvc
class CoreHttpSecurityWebTest {

    private static final String ISSUER = "https://idp.platform.test/realms/platform";
    private static final String AUDIENCE = "platform-core-api";
    private static final String PROVIDER = "platform-oidc";

    private static final KeyPair KEY_PAIR = rsaKeyPair();
    private static final UUID SUBJECT_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000aa");
    private static final UUID TENANT_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID TENANT_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b1");

    @Autowired
    private MockMvc mockMvc;

    @MockBean
    private TenantService tenantService;

    @MockBean
    private org.springframework.jdbc.core.JdbcTemplate jdbcTemplate;

    @MockBean
    private org.springframework.boot.actuate.health.HealthEndpoint healthEndpoint;

    @BeforeEach
    void stubTenant() {
        Tenant tenantA = new Tenant(
                TENANT_A, "Tenant A", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC",
                Instant.parse("2026-01-01T00:00:00Z"), Instant.parse("2026-01-01T00:00:00Z")
        );
        when(tenantService.get(TENANT_A)).thenReturn(tenantA);
        when(tenantService.get(TENANT_B)).thenThrow(new com.platform.core.domain.DomainException("TENANT_NOT_FOUND", "Tenant not found"));
        when(healthEndpoint.health()).thenReturn(org.springframework.boot.actuate.health.Health.up().build());
    }

    @Test
    void businessApiWithoutCredentialsIsUnauthorized() throws Exception {
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value(containsString("AUTH")))
                .andExpect(jsonPath("$.stackTrace").doesNotExist());
    }

    @Test
    void malformedAuthorizationHeaderRejected() throws Exception {
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "NotBearer abc"))
                .andExpect(status().isUnauthorized());
    }

    @Test
    void malformedJwtRejected() throws Exception {
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer not-a-jwt"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("AUTH_TOKEN_INVALID"));
    }

    @Test
    void invalidSignatureRejected() throws Exception {
        KeyPair other = rsaKeyPair();
        String token = signedJwt((RSAPrivateKey) other.getPrivate(), ISSUER, AUDIENCE, "user-a",
                Instant.now().minusSeconds(60), Instant.now().plusSeconds(600));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isUnauthorized());
    }

    @Test
    void expiredTokenRejected() throws Exception {
        String token = signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), ISSUER, AUDIENCE, "user-a",
                Instant.now().minusSeconds(3600), Instant.now().minusSeconds(60));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isUnauthorized());
    }

    @Test
    void notYetValidTokenRejected() throws Exception {
        String token = signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), ISSUER, AUDIENCE, "user-a",
                Instant.now().plusSeconds(600), Instant.now().plusSeconds(1200));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isUnauthorized());
    }

    @Test
    void wrongIssuerRejected() throws Exception {
        String token = signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), "https://evil.example/issuer", AUDIENCE, "user-a",
                Instant.now().minusSeconds(60), Instant.now().plusSeconds(600));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isUnauthorized());
    }

    @Test
    void wrongAudienceRejected() throws Exception {
        String token = signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), ISSUER, "other-audience", "user-a",
                Instant.now().minusSeconds(60), Instant.now().plusSeconds(600));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isUnauthorized());
    }

    @Test
    void hs256UnsupportedAlgorithmRejected() throws Exception {
        byte[] secret = new byte[32];
        for (int i = 0; i < secret.length; i++) {
            secret[i] = (byte) i;
        }
        JWTClaimsSet claims = new JWTClaimsSet.Builder()
                .issuer(ISSUER)
                .subject("user-a")
                .audience(AUDIENCE)
                .issueTime(Date.from(Instant.now().minusSeconds(60)))
                .notBeforeTime(Date.from(Instant.now().minusSeconds(60)))
                .expirationTime(Date.from(Instant.now().plusSeconds(600)))
                .build();
        SignedJWT jwt = new SignedJWT(new JWSHeader(JWSAlgorithm.HS256), claims);
        jwt.sign(new MACSigner(secret));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + jwt.serialize()))
                .andExpect(status().isUnauthorized());
    }

    @Test
    void unresolvedIdentityRejected() throws Exception {
        String token = signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), ISSUER, AUDIENCE, "unknown-user",
                Instant.now().minusSeconds(60), Instant.now().plusSeconds(600));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isUnauthorized());
    }

    @Test
    void disabledIdentityRejected() throws Exception {
        String token = signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), ISSUER, AUDIENCE, "disabled-user",
                Instant.now().minusSeconds(60), Instant.now().plusSeconds(600));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isUnauthorized());
    }

    @Test
    void validPrincipalCanReadOwnTenant() throws Exception {
        String token = signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), ISSUER, AUDIENCE, "user-a",
                Instant.now().minusSeconds(60), Instant.now().plusSeconds(600));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.name").value("Tenant A"));
    }

    @Test
    void tenantACannotReadTenantB() throws Exception {
        String token = signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), ISSUER, AUDIENCE, "user-a",
                Instant.now().minusSeconds(60), Instant.now().plusSeconds(600));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_B)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));
    }

    @Test
    void authenticatedWithoutPermissionDeniedAsNotFound() throws Exception {
        String token = signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), ISSUER, AUDIENCE, "user-readonly-mismatch",
                Instant.now().minusSeconds(60), Instant.now().plusSeconds(600));
        mockMvc.perform(post("/api/v1/tenants")
                        .header("Authorization", "Bearer " + token)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"name\":\"X\",\"mode\":\"SINGLE\",\"status\":\"ACTIVE\",\"timezone\":\"UTC\"}"))
                .andExpect(status().isNotFound());
    }

    @Test
    void clientRoleClaimsDoNotGrantAccess() throws Exception {
        String token = signedJwtWithExtraClaims(
                (RSAPrivateKey) KEY_PAIR.getPrivate(),
                ISSUER,
                AUDIENCE,
                "user-no-grants",
                Instant.now().minusSeconds(60),
                Instant.now().plusSeconds(600),
                Map.of("roles", java.util.List.of("ADMIN"), "permissions", java.util.List.of("tenant.read"))
        );
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A)
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));
    }

    @Test
    void versionRemainsPublic() throws Exception {
        mockMvc.perform(get("/api/v1/version"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.product").value("Platform"));
    }

    @Test
    void anonymousActuatorHealthIsAllowedWithoutComponentDetails() throws Exception {
        // Without a live datastore in this harness health may be DOWN (503); probe chain must still allow it.
        int status = mockMvc.perform(get("/actuator/health"))
                .andReturn()
                .getResponse()
                .getStatus();
        org.junit.jupiter.api.Assertions.assertTrue(
                status == 200 || status == 503,
                "anonymous /actuator/health must be reachable, got HTTP " + status
        );
        String body = mockMvc.perform(get("/actuator/health"))
                .andReturn()
                .getResponse()
                .getContentAsString();
        org.junit.jupiter.api.Assertions.assertFalse(
                body.contains("\"components\""),
                "anonymous health must not expose component details: " + body
        );
        org.junit.jupiter.api.Assertions.assertFalse(body.contains("\"db\""));
        org.junit.jupiter.api.Assertions.assertFalse(body.contains("\"clickHouse\""));
    }

    @Test
    void anonymousActuatorLivenessIsAllowed() throws Exception {
        int status = mockMvc.perform(get("/actuator/health/liveness"))
                .andReturn()
                .getResponse()
                .getStatus();
        org.junit.jupiter.api.Assertions.assertTrue(
                status == 200 || status == 503,
                "anonymous /actuator/health/liveness must be reachable, got HTTP " + status
        );
    }

    @Test
    void anonymousActuatorReadinessIsAllowed() throws Exception {
        int status = mockMvc.perform(get("/actuator/health/readiness"))
                .andReturn()
                .getResponse()
                .getStatus();
        org.junit.jupiter.api.Assertions.assertTrue(
                status == 200 || status == 503,
                "anonymous /actuator/health/readiness must be reachable, got HTTP " + status
        );
    }

    @Test
    void anonymousActuatorMetricsDenied() throws Exception {
        mockMvc.perform(get("/actuator/metrics"))
                .andExpect(status().isForbidden());
        mockMvc.perform(get("/actuator/metrics/jvm.memory.used"))
                .andExpect(status().isForbidden());
    }

    @Test
    void anonymousActuatorInfoDenied() throws Exception {
        mockMvc.perform(get("/actuator/info"))
                .andExpect(status().isForbidden());
    }

    @Test
    void anonymousUnknownActuatorEndpointDenied() throws Exception {
        mockMvc.perform(get("/actuator/env"))
                .andExpect(status().isForbidden());
        mockMvc.perform(get("/actuator/beans"))
                .andExpect(status().isForbidden());
    }

    @Test
    void anonymousConnectivityHealthDenied() throws Exception {
        mockMvc.perform(get("/api/v1/health/connectivity"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value(containsString("AUTH")));
    }

    @Test
    void anonymousUnknownHealthPathCannotBypassAuth() throws Exception {
        mockMvc.perform(get("/api/v1/health/secret-probe"))
                .andExpect(status().isUnauthorized());
        mockMvc.perform(get("/api/v1/health/components"))
                .andExpect(status().isUnauthorized());
    }

    @Test
    void authenticatedPrincipalDoesNotImplicitlyReceiveConnectivity() throws Exception {
        String token = signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), ISSUER, AUDIENCE, "user-a",
                Instant.now().minusSeconds(60), Instant.now().plusSeconds(600));
        mockMvc.perform(get("/api/v1/health/connectivity")
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));
    }

    @Test
    void businessJwtDoesNotGrantActuatorAdministration() throws Exception {
        String token = signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), ISSUER, AUDIENCE, "user-a",
                Instant.now().minusSeconds(60), Instant.now().plusSeconds(600));
        mockMvc.perform(get("/actuator/metrics")
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isForbidden());
        mockMvc.perform(get("/actuator/info")
                        .header("Authorization", "Bearer " + token))
                .andExpect(status().isForbidden());
    }

    @Test
    void sensorWithoutClientCertificateRemainsUnauthorized() throws Exception {
        mockMvc.perform(post("/api/v1/sensor/heartbeat")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"sensor_id\":\"x\"}"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("ERR_AUTH_CLIENT_CERT_REQUIRED"));
    }

    @Test
    void telemetryWithoutClientCertificateRemainsUnauthorized() throws Exception {
        mockMvc.perform(post("/api/v1/telemetry")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"sensor_id\":\"x\"}"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("ERR_AUTH_CLIENT_CERT_REQUIRED"));
    }

    @Test
    void bearerTokenCannotImpersonateSensorWithoutCertificate() throws Exception {
        String token = signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), ISSUER, AUDIENCE, "user-a",
                Instant.now().minusSeconds(60), Instant.now().plusSeconds(600));
        mockMvc.perform(post("/api/v1/sensor/heartbeat")
                        .header("Authorization", "Bearer " + token)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"sensor_id\":\"x\"}"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("ERR_AUTH_CLIENT_CERT_REQUIRED"));
        mockMvc.perform(post("/api/v1/telemetry")
                        .header("Authorization", "Bearer " + token)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"sensor_id\":\"x\"}"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("ERR_AUTH_CLIENT_CERT_REQUIRED"));
    }

    @Test
    void errorBodiesDoNotLeakTokenMaterial() throws Exception {
        String token = "Bearer FAKESECRET_a1b2c3d4e5f6g7h8i9j0";
        String body = mockMvc.perform(get("/api/v1/tenants/" + TENANT_A).header("Authorization", token))
                .andExpect(status().isUnauthorized())
                .andReturn()
                .getResponse()
                .getContentAsString();
        org.junit.jupiter.api.Assertions.assertFalse(body.contains("header."));
        org.junit.jupiter.api.Assertions.assertFalse(body.toLowerCase().contains("stacktrace"));
    }

    @SpringBootConfiguration
    @EnableAutoConfiguration(exclude = {
            DataSourceAutoConfiguration.class,
            KafkaAutoConfiguration.class
    })
    @Import({
            HttpSecurityConfiguration.class,
            PlatformJwtAuthenticationConverter.class,
            AuthorizationRouteMapper.class,
            AuthorizationEnforcementFilter.class,
            SecurityErrorResponseWriter.class,
            SecurityAuditEmitter.class,
            ApiExceptionHandler.class,
            CorrelationIdFilter.class,
            TenantController.class,
            SensorIngestionController.class,
            VersionController.class,
            TestSecurityBeans.class
    })
    static class TestApplication {
    }

    @Configuration
    static class TestSecurityBeans {
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
        JwtDecoder jwtDecoder(OidcResourceServerProperties properties) {
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

        @Bean
        IdentityDirectory identityDirectory() {
            Map<String, IdentityDirectory.MappedIdentity> map = new ConcurrentHashMap<>();
            map.put("user-a", new IdentityDirectory.MappedIdentity(new PlatformSubject(SUBJECT_A), false));
            map.put("disabled-user", new IdentityDirectory.MappedIdentity(new PlatformSubject(UUID.randomUUID()), true));
            map.put("user-readonly-mismatch", new IdentityDirectory.MappedIdentity(new PlatformSubject(UUID.randomUUID()), false));
            map.put("user-no-grants", new IdentityDirectory.MappedIdentity(new PlatformSubject(UUID.randomUUID()), false));
            return key -> {
                if (key.providerType() != IdentityProviderType.OIDC
                        || !PROVIDER.equals(key.providerInstanceId().value())) {
                    return Optional.empty();
                }
                return Optional.ofNullable(map.get(key.externalSubject().value()));
            };
        }

        @Bean
        IdentityResolver identityResolver(IdentityDirectory identityDirectory) {
            return new DirectoryIdentityResolver(identityDirectory);
        }

        @Bean
        PermissionGrantDirectory permissionGrantDirectory() {
            return subject -> {
                if (SUBJECT_A.equals(subject.subjectId())) {
                    return new PermissionGrantDirectory.PrincipalAuthorization(
                            Set.of(TENANT_A),
                            Set.of(new PermissionGrant(
                                    TENANT_A,
                                    PlatformPermissions.TENANT_READ,
                                    ResourceScope.tenantWide(ResourceType.TENANT)
                            ))
                    );
                }
                return new PermissionGrantDirectory.PrincipalAuthorization(Set.of(), Set.of());
            };
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

    private static String signedJwt(
            RSAPrivateKey privateKey,
            String issuer,
            String audience,
            String subject,
            Instant nbf,
            Instant exp
    ) throws Exception {
        return signedJwtWithExtraClaims(privateKey, issuer, audience, subject, nbf, exp, Map.of());
    }

    private static String signedJwtWithExtraClaims(
            RSAPrivateKey privateKey,
            String issuer,
            String audience,
            String subject,
            Instant nbf,
            Instant exp,
            Map<String, Object> extra
    ) throws Exception {
        JWTClaimsSet.Builder builder = new JWTClaimsSet.Builder()
                .issuer(issuer)
                .subject(subject)
                .audience(audience)
                .issueTime(Date.from(nbf))
                .notBeforeTime(Date.from(nbf))
                .expirationTime(Date.from(exp));
        extra.forEach(builder::claim);
        SignedJWT jwt = new SignedJWT(new JWSHeader(JWSAlgorithm.RS256), builder.build());
        jwt.sign(new RSASSASigner(privateKey));
        return jwt.serialize();
    }
}
