package com.platform.core.security.http;

import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.crypto.RSASSASigner;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;
import com.platform.core.api.MonitoringProfileAssignmentController;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.monitoring.MonitoringProfileEntityReadService;
import com.platform.core.domain.monitoring.MonitoringProfileEntityReadService.EntityAssignmentView;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.AssignmentMode;
import com.platform.core.logging.CorrelationIdFilter;
import com.platform.core.security.authorization.PermissionGrant;
import com.platform.core.security.authorization.PermissionGrantDirectory;
import com.platform.core.security.authorization.PlatformPermissions;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.authorization.ResourceScope;
import com.platform.core.security.authorization.ResourceType;
import com.platform.core.security.identity.DirectoryIdentityResolver;
import com.platform.core.security.identity.IdentityDirectory;
import com.platform.core.security.identity.IdentityProviderType;
import com.platform.core.security.identity.IdentityResolver;
import com.platform.core.security.identity.PlatformSubject;
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

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.delete;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.put;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

/**
 * Real Spring Security filter-chain gate for the BUILD-070 entity assignment read API.
 * Uses RSA-signed JWTs and the production authorization mapper/filter; the read
 * service is mocked so the gate cannot modify or depend on a production database.
 */
@SpringBootTest(classes = MonitoringProfileAssignmentHttpSecurityWebTest.TestApplication.class)
@AutoConfigureMockMvc
class MonitoringProfileAssignmentHttpSecurityWebTest {

    private static final String ISSUER = "https://idp.platform.test/realms/platform";
    private static final String AUDIENCE = "platform-core-api";
    private static final String PROVIDER = "platform-oidc";
    private static final KeyPair KEY_PAIR = rsaKeyPair();

    private static final UUID TENANT_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID TENANT_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b1");
    private static final UUID ENTITY_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");
    private static final UUID ENTITY_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e2");
    private static final UUID PROFILE = UUID.fromString("018f0e0a-0000-7000-8000-0000000000d1");

    private static final UUID SUBJECT_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c1");
    private static final UUID SUBJECT_OTHER = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c2");
    private static final UUID SUBJECT_NO_GRANTS = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c3");
    private static final UUID SUBJECT_TENANT_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c4");

    private static final String PATH = "/api/v1/tenants/" + TENANT_A + "/entities/" + ENTITY_A
            + "/monitoring-profile-assignment";

    @Autowired
    private MockMvc mvc;

    @MockBean
    private MonitoringProfileEntityReadService reader;

    @MockBean
    private org.springframework.jdbc.core.JdbcTemplate jdbcTemplate;

    @MockBean
    private org.springframework.boot.actuate.health.HealthEndpoint healthEndpoint;

    @Test
    void anonymousRequestRequiresBearerAuthentication() throws Exception {
        mvc.perform(get(PATH))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").exists());
        verifyNoInteractions(reader);
    }

    @Test
    void invalidBearerTokenIsRejectedBeforeService() throws Exception {
        mvc.perform(get(PATH).header("Authorization", "Bearer not-a-jwt"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("AUTH_TOKEN_INVALID"));
        verifyNoInteractions(reader);
    }

    @Test
    void wrongJwtSignatureIsRejectedBeforeService() throws Exception {
        String forged = signedJwt((RSAPrivateKey) rsaKeyPair().getPrivate(), "entity-viewer");
        mvc.perform(get(PATH).header("Authorization", "Bearer " + forged))
                .andExpect(status().isUnauthorized());
        verifyNoInteractions(reader);
    }

    @Test
    void authenticatedButMissingEntityReadGrantIsConcealed() throws Exception {
        mvc.perform(get(PATH).header("Authorization", "Bearer " + token("entity-no-grants")))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));
        verifyNoInteractions(reader);
    }

    @Test
    void namedGrantForDifferentEntityCannotReadAssignment() throws Exception {
        mvc.perform(get(PATH).header("Authorization", "Bearer " + token("entity-other-only")))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));
        verifyNoInteractions(reader);
    }

    @Test
    void namedGrantForOtherTenantCannotReadAssignment() throws Exception {
        mvc.perform(get(PATH).header("Authorization", "Bearer " + token("entity-tenant-b")))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));
        verifyNoInteractions(reader);
    }

    @Test
    void authorizedReadReturnsOnlyAssignmentMetadata() throws Exception {
        Instant effectiveFrom = Instant.parse("2026-11-18T00:00:00Z");
        when(reader.findEntityAssignment(any(PrincipalContext.class), eq(TENANT_A), eq(ENTITY_A)))
                .thenReturn(Optional.of(new EntityAssignmentView(
                        ENTITY_A, PROFILE, 3, AssignmentMode.EXPLICIT_PROFILE, 4L, effectiveFrom)));

        mvc.perform(get(PATH).header("Authorization", "Bearer " + token("entity-viewer")))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.entityId").value(ENTITY_A.toString()))
                .andExpect(jsonPath("$.profileId").value(PROFILE.toString()))
                .andExpect(jsonPath("$.profileVersionNo").value(3))
                .andExpect(jsonPath("$.assignmentMode").value("EXPLICIT_PROFILE"))
                .andExpect(jsonPath("$.rowVersion").value(4))
                .andExpect(jsonPath("$.definition").doesNotExist())
                .andExpect(jsonPath("$.capacityEstimate").doesNotExist())
                .andExpect(jsonPath("$.assignedBy").doesNotExist());
        verify(reader).findEntityAssignment(any(PrincipalContext.class), eq(TENANT_A), eq(ENTITY_A));
    }

    @Test
    void authorizedReadWithNoAssignmentReturnsNoContent() throws Exception {
        when(reader.findEntityAssignment(any(PrincipalContext.class), eq(TENANT_A), eq(ENTITY_A)))
                .thenReturn(Optional.empty());
        mvc.perform(get(PATH).header("Authorization", "Bearer " + token("entity-viewer")))
                .andExpect(status().isNoContent());
        verify(reader).findEntityAssignment(any(PrincipalContext.class), eq(TENANT_A), eq(ENTITY_A));
    }

    @Test
    void readOnlyGrantDoesNotPermitAnyAssignmentMutationMethod() throws Exception {
        String header = "Bearer " + token("entity-viewer");
        mvc.perform(post(PATH).header("Authorization", header).contentType("application/json").content("{}"))
                .andExpect(status().isNotFound());
        mvc.perform(put(PATH).header("Authorization", header).contentType("application/json").content("{}"))
                .andExpect(status().isNotFound());
        mvc.perform(delete(PATH).header("Authorization", header))
                .andExpect(status().isNotFound());
        verifyNoInteractions(reader);
    }

    private static String token(String subject) throws Exception {
        return signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), subject);
    }

    private static String signedJwt(RSAPrivateKey key, String subject) throws Exception {
        Instant now = Instant.now();
        JWTClaimsSet claims = new JWTClaimsSet.Builder()
                .issuer(ISSUER)
                .subject(subject)
                .audience(AUDIENCE)
                .issueTime(Date.from(now.minusSeconds(60)))
                .notBeforeTime(Date.from(now.minusSeconds(60)))
                .expirationTime(Date.from(now.plusSeconds(600)))
                .build();
        SignedJWT jwt = new SignedJWT(new JWSHeader(JWSAlgorithm.RS256), claims);
        jwt.sign(new RSASSASigner(key));
        return jwt.serialize();
    }

    private static KeyPair rsaKeyPair() {
        try {
            KeyPairGenerator generator = KeyPairGenerator.getInstance("RSA");
            generator.initialize(2048);
            return generator.generateKeyPair();
        } catch (Exception ex) {
            throw new IllegalStateException("RSA test key initialization failed", ex);
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
            ApiExceptionHandler.class,
            CorrelationIdFilter.class,
            MonitoringProfileAssignmentController.class,
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

        @Bean
        IdentityDirectory identityDirectory() {
            Map<String, UUID> subjects = Map.of(
                    "entity-viewer", SUBJECT_A,
                    "entity-other-only", SUBJECT_OTHER,
                    "entity-no-grants", SUBJECT_NO_GRANTS,
                    "entity-tenant-b", SUBJECT_TENANT_B
            );
            return key -> {
                if (key.providerType() != IdentityProviderType.OIDC
                        || !PROVIDER.equals(key.providerInstanceId().value())) {
                    return Optional.empty();
                }
                UUID subjectId = subjects.get(key.externalSubject().value());
                return subjectId == null ? Optional.empty() : Optional.of(
                        new IdentityDirectory.MappedIdentity(new PlatformSubject(subjectId), false));
            };
        }

        @Bean
        IdentityResolver identityResolver(IdentityDirectory identityDirectory) {
            return new DirectoryIdentityResolver(identityDirectory);
        }

        @Bean
        PermissionGrantDirectory permissionGrantDirectory() {
            return subject -> {
                UUID subjectId = subject.subjectId();
                if (SUBJECT_A.equals(subjectId)) {
                    return new PermissionGrantDirectory.PrincipalAuthorization(
                            Set.of(TENANT_A), Set.of(new PermissionGrant(
                                    TENANT_A, PlatformPermissions.ENTITY_READ,
                                    ResourceScope.named(ResourceType.ENTITY, Set.of(ENTITY_A)))));
                }
                if (SUBJECT_OTHER.equals(subjectId)) {
                    return new PermissionGrantDirectory.PrincipalAuthorization(
                            Set.of(TENANT_A), Set.of(new PermissionGrant(
                                    TENANT_A, PlatformPermissions.ENTITY_READ,
                                    ResourceScope.named(ResourceType.ENTITY, Set.of(ENTITY_B)))));
                }
                if (SUBJECT_TENANT_B.equals(subjectId)) {
                    return new PermissionGrantDirectory.PrincipalAuthorization(
                            Set.of(TENANT_B), Set.of(new PermissionGrant(
                                    TENANT_B, PlatformPermissions.ENTITY_READ,
                                    ResourceScope.named(ResourceType.ENTITY, Set.of(ENTITY_A)))));
                }
                return new PermissionGrantDirectory.PrincipalAuthorization(Set.of(TENANT_A), Set.of());
            };
        }
    }
}
