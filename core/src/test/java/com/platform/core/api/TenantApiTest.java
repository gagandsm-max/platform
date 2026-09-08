package com.platform.core.api;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.api.dto.TenantDtos;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.logging.CorrelationIdFilter;
import com.platform.core.security.authorization.AuthenticationState;
import com.platform.core.security.authorization.PermissionGrant;
import com.platform.core.security.authorization.PlatformPermissions;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.authorization.ResourceScope;
import com.platform.core.security.authorization.ResourceType;
import com.platform.core.security.http.PlatformAuthenticationToken;
import com.platform.core.security.identity.AuthenticatedExternalIdentity;
import com.platform.core.security.identity.AuthenticationAssurance;
import com.platform.core.security.identity.ExternalSubject;
import com.platform.core.security.identity.IdentityProviderType;
import com.platform.core.security.identity.PlatformSubject;
import com.platform.core.security.identity.ProviderInstanceId;
import com.platform.core.security.identity.ResolvedAuthenticatedIdentity;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.MediaType;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.time.Instant;
import java.util.Set;
import java.util.UUID;

import static org.hamcrest.Matchers.notNullValue;
import static org.hamcrest.Matchers.not;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.header;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

class TenantApiTest {

    private MockMvc mockMvc;
    private TenantService tenantService;
    private final ObjectMapper mapper = new ObjectMapper().findAndRegisterModules();

    @BeforeEach
    void setUp() {
        tenantService = mock(TenantService.class);
        mockMvc = MockMvcBuilders.standaloneSetup(new TenantController(tenantService))
                .addFilters(new CorrelationIdFilter())
                .setControllerAdvice(new ApiExceptionHandler())
                .build();

        // Controller list path now requires an authenticated Platform principal.
        UUID tenantScope = UUID.fromString("018f0e0a-0000-7000-8000-000000000099");
        PrincipalContext principal = new PrincipalContext(
                "subject-test",
                AuthenticationState.AUTHENTICATED,
                Set.of(tenantScope),
                Set.of(new PermissionGrant(
                        tenantScope,
                        PlatformPermissions.TENANT_READ,
                        ResourceScope.tenantWide(ResourceType.TENANT)
                ))
        );
        SecurityContextHolder.getContext().setAuthentication(
                new PlatformAuthenticationToken(
                        ResolvedAuthenticatedIdentity.current(
                                new PlatformSubject(tenantScope),
                                new AuthenticatedExternalIdentity(
                                        IdentityProviderType.OIDC,
                                        new ProviderInstanceId("platform-oidc"),
                                        new ExternalSubject("user-1"),
                                        Instant.parse("2026-01-01T00:00:00Z"),
                                        AuthenticationAssurance.withoutVerifiedMfa(),
                                        Instant.parse("2099-01-01T00:00:00Z")
                                ),
                                Instant.parse("2026-06-01T00:00:00Z")
                        ),
                        principal
                )
        );
    }

    @org.junit.jupiter.api.AfterEach
    void clearSecurity() {
        SecurityContextHolder.clearContext();
    }

    @Test
    void createAndGetTenantReturnExpectedStatuses() throws Exception {
        Tenant tenant = new Tenant(
                UUID.fromString("018f0e0a-0000-7000-8000-000000000099"),
                "Acme",
                TenantMode.SINGLE,
                TenantStatus.ACTIVE,
                "UTC",
                Instant.parse("2026-01-01T00:00:00Z"),
                Instant.parse("2026-01-01T00:00:00Z")
        );
        when(tenantService.create(eq("Acme"), eq(TenantMode.SINGLE), eq(TenantStatus.ACTIVE), eq("UTC")))
                .thenReturn(tenant);
        when(tenantService.get(tenant.tenantId())).thenReturn(tenant);

        mockMvc.perform(post("/api/v1/tenants")
                        .contentType(MediaType.APPLICATION_JSON)
                        .header(CorrelationIdFilter.CORRELATION_ID_HEADER, "corr-day02")
                        .content(mapper.writeValueAsString(new TenantDtos.CreateTenantRequest(
                                "Acme", "SINGLE", "ACTIVE", "UTC"))))
                .andExpect(status().isCreated())
                .andExpect(header().string(CorrelationIdFilter.CORRELATION_ID_HEADER, "corr-day02"))
                .andExpect(jsonPath("$.tenantId").value(tenant.tenantId().toString()));

        mockMvc.perform(get("/api/v1/tenants/" + tenant.tenantId()))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.name").value("Acme"));
    }

    @Test
    void notFoundIncludesCorrelationIdWithoutStackTrace() throws Exception {
        UUID missing = UUID.fromString("018f0e0a-0000-7000-8000-000000000404");
        when(tenantService.get(missing)).thenThrow(new DomainException("TENANT_NOT_FOUND", "Tenant not found"));

        mockMvc.perform(get("/api/v1/tenants/" + missing)
                        .header(CorrelationIdFilter.CORRELATION_ID_HEADER, "corr-404"))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("TENANT_NOT_FOUND"))
                .andExpect(jsonPath("$.correlationId").value("corr-404"))
                .andExpect(jsonPath("$.stackTrace").doesNotExist())
                .andExpect(jsonPath("$.message", notNullValue()));
    }

    @Test
    void badPaginationRejected() throws Exception {
        TenantService realish = mock(TenantService.class);
        when(realish.listScoped(any(), eq(0), anyInt()))
                .thenThrow(new DomainException("QUERY_LIMIT_EXCEEDED", "page must be >= 1"));
        MockMvc mvc = MockMvcBuilders.standaloneSetup(new TenantController(realish))
                .setControllerAdvice(new ApiExceptionHandler())
                .addFilters(new CorrelationIdFilter())
                .build();
        mvc.perform(get("/api/v1/tenants").param("page", "0").param("limit", "50"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("QUERY_LIMIT_EXCEEDED"));
    }

    @Test
    void malformedPathAndQueryValuesReturnStableValidationErrors() throws Exception {
        mockMvc.perform(get("/api/v1/tenants/not-a-uuid"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"))
                .andExpect(jsonPath("$.stackTrace").doesNotExist());

        mockMvc.perform(get("/api/v1/tenants").param("page", "not-an-integer"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));
    }

    @Test
    void unsafeCorrelationIdIsNotReflected() throws Exception {
        mockMvc.perform(get("/api/v1/tenants/not-a-uuid")
                        .header(CorrelationIdFilter.CORRELATION_ID_HEADER, "unsafe value"))
                .andExpect(status().isBadRequest())
                .andExpect(header().string(CorrelationIdFilter.CORRELATION_ID_HEADER, not("unsafe value")))
                .andExpect(jsonPath("$.correlationId", not("unsafe value")));
    }
}
