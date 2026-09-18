package com.platform.core.security.http;

import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.crypto.RSASSASigner;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;
import com.platform.core.api.CapabilityPackController;
import com.platform.core.api.PageResult;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.capabilitypack.CapabilityPackCatalogService;
import com.platform.core.domain.capabilitypack.CapabilityPackTestSupport;
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

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.delete;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.put;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest(classes = CapabilityPackHttpSecurityWebTest.TestApplication.class)
@AutoConfigureMockMvc
class CapabilityPackHttpSecurityWebTest {

    private static final String ISSUER = "https://idp.platform.test/realms/platform";
    private static final String AUDIENCE = "platform-core-api";
    private static final String PROVIDER = "platform-oidc";
    private static final KeyPair KEY_PAIR = rsaKeyPair();

    private static final UUID SUBJECT_READER = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c1");
    private static final UUID SUBJECT_MANAGER = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c2");
    private static final UUID TENANT_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID TENANT_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b1");
    private static final UUID SUBJECT_MANAGER_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c3");

    @Autowired
    private MockMvc mockMvc;

    @MockBean
    private CapabilityPackCatalogService catalog;

    @MockBean
    private org.springframework.jdbc.core.JdbcTemplate jdbcTemplate;

    @MockBean
    private org.springframework.boot.actuate.health.HealthEndpoint healthEndpoint;

    @Test
    void getRequiresCapabilityPackRead() throws Exception {
        when(catalog.list(eq(TENANT_A), anyInt(), anyInt()))
                .thenReturn(new PageResult<>(java.util.List.of(), 1, 50, 0, false));
        String reader = token("pack-reader");
        String manager = token("pack-manager");

        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A + "/capability-packs")
                        .header("Authorization", "Bearer " + reader))
                .andExpect(status().isOk());

        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A + "/capability-packs")
                        .header("Authorization", "Bearer " + manager))
                .andExpect(status().isNotFound());
    }

    @Test
    void putAndDeleteRequireCapabilityPackManage() throws Exception {
        byte[] body = CapabilityPackTestSupport.minimalManifestBytes("fixture.fictional.base", "1.0.0", "Base");
        String reader = token("pack-reader");
        String manager = token("pack-manager");

        mockMvc.perform(put("/api/v1/tenants/" + TENANT_A + "/capability-packs/fixture.fictional.base/versions/1.0.0")
                        .header("Authorization", "Bearer " + reader)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(body))
                .andExpect(status().isNotFound());

        mockMvc.perform(put("/api/v1/tenants/" + TENANT_A + "/capability-packs/fixture.fictional.base/active-version")
                        .header("Authorization", "Bearer " + reader)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"packVersion\":\"1.0.0\",\"expectedGeneration\":0}"))
                .andExpect(status().isNotFound());

        mockMvc.perform(delete("/api/v1/tenants/" + TENANT_A + "/capability-packs/fixture.fictional.base/active-version")
                        .header("Authorization", "Bearer " + reader)
                        .param("expectedGeneration", "1"))
                .andExpect(status().isNotFound());

        when(catalog.register(any(), any(), any(), any())).thenThrow(
                new com.platform.core.domain.DomainException("VALIDATION_FAILED", "stub"));
        mockMvc.perform(put("/api/v1/tenants/" + TENANT_A + "/capability-packs/fixture.fictional.base/versions/1.0.0")
                        .header("Authorization", "Bearer " + manager)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(body))
                .andExpect(status().isBadRequest());
    }

    @Test
    void wrongTenantGrantAndPathAloneDoNotAuthorize() throws Exception {
        String wrongTenantManager = token("pack-manager-b");
        mockMvc.perform(get("/api/v1/tenants/" + TENANT_A + "/capability-packs")
                        .header("Authorization", "Bearer " + wrongTenantManager))
                .andExpect(status().isNotFound());
    }

    @Test
    void unknownCapabilityPackMethodDenied() throws Exception {
        String manager = token("pack-manager");
        mockMvc.perform(post("/api/v1/tenants/" + TENANT_A + "/capability-packs")
                        .header("Authorization", "Bearer " + manager)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{}"))
                .andExpect(status().isNotFound());
    }

    private String token(String subject) throws Exception {
        return signedJwt((RSAPrivateKey) KEY_PAIR.getPrivate(), subject,
                Instant.now().minusSeconds(60), Instant.now().plusSeconds(600));
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
            CapabilityPackController.class,
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
            Map<String, IdentityDirectory.MappedIdentity> map = new ConcurrentHashMap<>();
            map.put("pack-reader", new IdentityDirectory.MappedIdentity(new PlatformSubject(SUBJECT_READER), false));
            map.put("pack-manager", new IdentityDirectory.MappedIdentity(new PlatformSubject(SUBJECT_MANAGER), false));
            map.put("pack-manager-b", new IdentityDirectory.MappedIdentity(
                    new PlatformSubject(SUBJECT_MANAGER_B), false));
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
                if (SUBJECT_READER.equals(subject.subjectId())) {
                    return new PermissionGrantDirectory.PrincipalAuthorization(
                            Set.of(TENANT_A),
                            Set.of(new PermissionGrant(
                                    TENANT_A,
                                    PlatformPermissions.CAPABILITY_PACK_READ,
                                    ResourceScope.tenantWide(ResourceType.TENANT)
                            ))
                    );
                }
                if (SUBJECT_MANAGER.equals(subject.subjectId())) {
                    return new PermissionGrantDirectory.PrincipalAuthorization(
                            Set.of(TENANT_A),
                            Set.of(new PermissionGrant(
                                    TENANT_A,
                                    PlatformPermissions.CAPABILITY_PACK_MANAGE,
                                    ResourceScope.tenantWide(ResourceType.TENANT)
                            ))
                    );
                }
                if (SUBJECT_MANAGER_B.equals(subject.subjectId())) {
                    return new PermissionGrantDirectory.PrincipalAuthorization(
                            Set.of(TENANT_B),
                            Set.of(new PermissionGrant(
                                    TENANT_B,
                                    PlatformPermissions.CAPABILITY_PACK_MANAGE,
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

    private static String signedJwt(RSAPrivateKey privateKey, String subject, Instant nbf, Instant exp)
            throws Exception {
        JWTClaimsSet claims = new JWTClaimsSet.Builder()
                .issuer(ISSUER)
                .subject(subject)
                .audience(AUDIENCE)
                .issueTime(Date.from(nbf))
                .notBeforeTime(Date.from(nbf))
                .expirationTime(Date.from(exp))
                .build();
        SignedJWT jwt = new SignedJWT(new JWSHeader(JWSAlgorithm.RS256), claims);
        jwt.sign(new RSASSASigner(privateKey));
        return jwt.serialize();
    }
}
