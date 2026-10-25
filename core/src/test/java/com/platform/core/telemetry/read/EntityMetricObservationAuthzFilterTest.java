package com.platform.core.telemetry.read;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.api.EntityMetricObservationController;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.inventory.EntityService;
import com.platform.core.logging.CorrelationIdFilter;
import com.platform.core.security.authorization.PermissionGrant;
import com.platform.core.security.authorization.PlatformPermissions;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.authorization.ResourceScope;
import com.platform.core.security.authorization.ResourceType;
import com.platform.core.security.http.AuthorizationEnforcementFilter;
import com.platform.core.security.http.AuthorizationRouteMapper;
import com.platform.core.security.http.PlatformAuthenticationToken;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.security.http.SecurityErrorResponseWriter;
import com.platform.core.security.identity.AuthenticatedExternalIdentity;
import com.platform.core.security.identity.AuthenticationAssurance;
import com.platform.core.security.identity.ExternalSubject;
import com.platform.core.security.identity.IdentityProviderType;
import com.platform.core.security.identity.PlatformSubject;
import com.platform.core.security.identity.ProviderInstanceId;
import com.platform.core.security.identity.ResolvedAuthenticatedIdentity;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.format.datetime.standard.DateTimeFormatterRegistrar;
import org.springframework.format.support.DefaultFormattingConversionService;
import org.springframework.http.converter.json.MappingJackson2HttpMessageConverter;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.time.Instant;
import java.util.Set;
import java.util.UUID;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

/**
 * Proves ENTITY_READ denial on the real {@link AuthorizationEnforcementFilter} path
 * never reaches the metric service / ClickHouse reader. Standalone controller mocks alone
 * do not establish this operational identity.
 */
class EntityMetricObservationAuthzFilterTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID OTHER_TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b1");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");
    private static final UUID OTHER_ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e2");
    private static final UUID SUBJECT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000aa");

    private EntityService entities;
    private ClickHouseMetricObservationReader reader;
    private MockMvc mockMvc;

    @BeforeEach
    void setUp() {
        entities = mock(EntityService.class);
        reader = mock(ClickHouseMetricObservationReader.class);
        EntityMetricObservationService service = new EntityMetricObservationService(entities, reader, 300);

        ObjectMapper mapper = new ObjectMapper().findAndRegisterModules();
        MappingJackson2HttpMessageConverter converter = new MappingJackson2HttpMessageConverter(mapper);
        DefaultFormattingConversionService conversionService = new DefaultFormattingConversionService();
        DateTimeFormatterRegistrar registrar = new DateTimeFormatterRegistrar();
        registrar.setUseIsoFormat(true);
        registrar.registerFormatters(conversionService);

        AuthorizationEnforcementFilter authzFilter = new AuthorizationEnforcementFilter(
                new AuthorizationRouteMapper(),
                new SecurityErrorResponseWriter(mapper),
                new SecurityAuditEmitter()
        );

        mockMvc = MockMvcBuilders.standaloneSetup(new EntityMetricObservationController(service))
                .addFilters(new CorrelationIdFilter(), authzFilter)
                .setControllerAdvice(new ApiExceptionHandler())
                .setConversionService(conversionService)
                .setMessageConverters(converter)
                .build();
    }

    @AfterEach
    void clearSecurity() {
        SecurityContextHolder.clearContext();
    }

    @Test
    void crossTenantPathDeniedWithoutClickHouseCall() throws Exception {
        authenticate(principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_READ,
                        ResourceScope.named(ResourceType.ENTITY, Set.of(ENTITY))
                )
        ));

        mockMvc.perform(get("/api/v1/tenants/" + OTHER_TENANT + "/entities/" + ENTITY + "/metric-observations")
                        .param("metricId", "system.cpu.utilization")
                        .param("valueKind", "NUMERIC")
                        .param("from", "2026-10-16T00:00:00Z")
                        .param("to", "2026-10-16T01:00:00Z"))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));

        verifyNoInteractions(entities);
        verifyNoInteractions(reader);
    }

    @Test
    void otherEntityScopeDeniedWithoutClickHouseCall() throws Exception {
        authenticate(principal(
                TENANT,
                new PermissionGrant(
                        TENANT,
                        PlatformPermissions.ENTITY_READ,
                        ResourceScope.named(ResourceType.ENTITY, Set.of(OTHER_ENTITY))
                )
        ));

        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/metric-observations")
                        .param("metricId", "system.cpu.utilization")
                        .param("valueKind", "NUMERIC")
                        .param("from", "2026-10-16T00:00:00Z")
                        .param("to", "2026-10-16T01:00:00Z"))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));

        verifyNoInteractions(entities);
        verifyNoInteractions(reader);
    }

    @Test
    void missingEntityReadPermissionDeniedWithoutClickHouseCall() throws Exception {
        authenticate(principal(TENANT));

        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/metric-observations")
                        .param("metricId", ".1.3.6.1.2.1.31.1.1.1.6.1")
                        .param("valueKind", "COUNTER64")
                        .param("from", "2026-10-16T00:00:00Z")
                        .param("to", "2026-10-16T01:00:00Z"))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("RESOURCE_NOT_FOUND"));

        verify(entities, never()).get(any(), any());
        verifyNoInteractions(reader);
    }

    private static void authenticate(PrincipalContext principal) {
        Instant now = Instant.parse("2026-10-16T12:00:00Z");
        AuthenticatedExternalIdentity evidence = new AuthenticatedExternalIdentity(
                IdentityProviderType.OIDC,
                new ProviderInstanceId("platform-oidc"),
                new ExternalSubject("user-a"),
                now.minusSeconds(60),
                AuthenticationAssurance.withoutVerifiedMfa(),
                now.plusSeconds(3600)
        );
        ResolvedAuthenticatedIdentity identity = ResolvedAuthenticatedIdentity.current(
                new PlatformSubject(SUBJECT), evidence, now);
        SecurityContextHolder.getContext().setAuthentication(
                new PlatformAuthenticationToken(identity, principal));
    }

    private static PrincipalContext principal(UUID tenantScope, PermissionGrant... grants) {
        return new PrincipalContext(
                "subject",
                com.platform.core.security.authorization.AuthenticationState.AUTHENTICATED,
                Set.of(tenantScope),
                Set.of(grants)
        );
    }
}
