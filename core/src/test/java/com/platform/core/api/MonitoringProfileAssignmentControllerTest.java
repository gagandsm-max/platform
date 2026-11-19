package com.platform.core.api;

import com.platform.core.domain.monitoring.MonitoringProfileEntityReadService;
import com.platform.core.domain.monitoring.MonitoringProfileEntityReadService.EntityAssignmentView;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.AssignmentMode;
import com.platform.core.security.authorization.AuthenticationState;
import com.platform.core.security.authorization.PermissionGrant;
import com.platform.core.security.authorization.PlatformPermissions;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.authorization.ResourceScope;
import com.platform.core.security.authorization.ResourceType;
import com.platform.core.security.http.PlatformAuthenticationToken;
import com.platform.core.security.http.UnauthorizedAccessException;
import com.platform.core.security.identity.ResolvedAuthenticatedIdentity;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.time.Instant;
import java.util.List;
import java.util.Optional;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

class MonitoringProfileAssignmentControllerTest {
    private static final UUID TENANT_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID TENANT_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b2");
    private static final UUID ENTITY_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c3");
    private static final UUID ENTITY_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000d4");
    private static final UUID PROFILE = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e5");
    private static final String PATH = "/api/v1/tenants/" + TENANT_A + "/entities/" + ENTITY_A
            + "/monitoring-profile-assignment";

    private MonitoringProfileEntityReadService reader;
    private MonitoringProfileAssignmentController controller;
    private MockMvc mvc;

    @BeforeEach
    void setup() {
        reader = mock(MonitoringProfileEntityReadService.class);
        controller = new MonitoringProfileAssignmentController(reader);
        mvc = MockMvcBuilders.standaloneSetup(controller).build();
    }

    @AfterEach
    void cleanup() {
        SecurityContextHolder.clearContext();
    }

    @Test
    void authorizedEntityReturnsOnlyBoundedAssignmentProjection() throws Exception {
        PrincipalContext principal = namedEntityReader(TENANT_A, ENTITY_A);
        authenticate(principal);
        Instant effectiveAt = Instant.parse("2026-11-18T00:00:00Z");
        when(reader.findEntityAssignment(principal, TENANT_A, ENTITY_A))
                .thenReturn(Optional.of(new EntityAssignmentView(
                        ENTITY_A, PROFILE, 7, AssignmentMode.EXPLICIT_PROFILE, 15L, effectiveAt)));

        mvc.perform(get(PATH))
                .andExpect(status().isOk())
                .andExpect(org.springframework.test.web.servlet.result.MockMvcResultMatchers.header()
                        .string("ETag", "\"15\""))
                .andExpect(org.springframework.test.web.servlet.result.MockMvcResultMatchers.header()
                        .string("Cache-Control", "no-store"))
                .andExpect(jsonPath("$.entityId").value(ENTITY_A.toString()))
                .andExpect(jsonPath("$.profileId").value(PROFILE.toString()))
                .andExpect(jsonPath("$.profileVersionNo").value(7))
                .andExpect(jsonPath("$.assignmentMode").value("EXPLICIT_PROFILE"))
                .andExpect(jsonPath("$.rowVersion").value(15))
                .andExpect(jsonPath("$.definition").doesNotExist())
                .andExpect(jsonPath("$.capacityEstimate").doesNotExist())
                .andExpect(jsonPath("$.assignedBy").doesNotExist());
        verify(reader).findEntityAssignment(principal, TENANT_A, ENTITY_A);
    }

    @Test
    void etagPreservesLongMaxValueAsExactDecimalString() throws Exception {
        PrincipalContext principal = namedEntityReader(TENANT_A, ENTITY_A);
        authenticate(principal);
        when(reader.findEntityAssignment(principal, TENANT_A, ENTITY_A))
                .thenReturn(Optional.of(new EntityAssignmentView(
                        ENTITY_A, PROFILE, 7, AssignmentMode.EXPLICIT_PROFILE,
                        Long.MAX_VALUE, Instant.parse("2026-11-18T00:00:00Z"))));

        mvc.perform(get(PATH))
                .andExpect(status().isOk())
                .andExpect(org.springframework.test.web.servlet.result.MockMvcResultMatchers.header()
                        .string("ETag", "\"9223372036854775807\""))
                .andExpect(org.springframework.test.web.servlet.result.MockMvcResultMatchers.header()
                        .string("Cache-Control", "no-store"));
    }

    @Test
    void invalidRowVersionFailsClosed() {
        PrincipalContext principal = namedEntityReader(TENANT_A, ENTITY_A);
        authenticate(principal);
        when(reader.findEntityAssignment(principal, TENANT_A, ENTITY_A))
                .thenReturn(Optional.of(new EntityAssignmentView(
                        ENTITY_A, PROFILE, 7, AssignmentMode.EXPLICIT_PROFILE,
                        0L, Instant.parse("2026-11-18T00:00:00Z"))));

        assertThrows(IllegalStateException.class, () -> controller.get(TENANT_A, ENTITY_A));
    }

    @Test
    void authenticatedEntityWithNoAssignmentReturnsNoContent() throws Exception {
        PrincipalContext principal = namedEntityReader(TENANT_A, ENTITY_A);
        authenticate(principal);
        when(reader.findEntityAssignment(principal, TENANT_A, ENTITY_A))
                .thenReturn(Optional.empty());

        mvc.perform(get(PATH))
                .andExpect(status().isNoContent())
                .andExpect(org.springframework.test.web.servlet.result.MockMvcResultMatchers.header()
                        .doesNotExist("ETag"))
                .andExpect(org.springframework.test.web.servlet.result.MockMvcResultMatchers.header()
                        .string("Cache-Control", "no-store"));
        verify(reader).findEntityAssignment(principal, TENANT_A, ENTITY_A);
    }

    @Test
    void absentAuthenticationNeverInvokesService() {
        assertThrows(UnauthorizedAccessException.class, () -> controller.get(TENANT_A, ENTITY_A));
        verifyNoInteractions(reader);
    }

    @Test
    void existingRouteMapperAcceptsExactNamedEntityReadButNotAnotherEntity() {
        var routes = new com.platform.core.security.http.AuthorizationRouteMapper();
        PrincipalContext principal = namedEntityReader(TENANT_A, ENTITY_A);
        assertTrue(routes.evaluate(principal, "GET", PATH).orElseThrow().allowed());
        String another = "/api/v1/tenants/" + TENANT_A + "/entities/" + ENTITY_B
                + "/monitoring-profile-assignment";
        assertFalse(routes.evaluate(principal, "GET", another).orElseThrow().allowed());
    }

    @Test
    void existingRouteMapperMasksCrossTenantAndRejectsMutatingMethods() {
        var routes = new com.platform.core.security.http.AuthorizationRouteMapper();
        PrincipalContext principal = namedEntityReader(TENANT_A, ENTITY_A);
        String otherTenant = "/api/v1/tenants/" + TENANT_B + "/entities/" + ENTITY_A
                + "/monitoring-profile-assignment";
        assertFalse(routes.evaluate(principal, "GET", otherTenant).orElseThrow().allowed());
        assertFalse(routes.evaluate(principal, "POST", PATH).orElseThrow().allowed());
        assertFalse(routes.evaluate(principal, "PUT", PATH).orElseThrow().allowed());
        assertFalse(routes.evaluate(principal, "DELETE", PATH).orElseThrow().allowed());
    }

    private static PrincipalContext namedEntityReader(UUID tenant, UUID entity) {
        return new PrincipalContext("viewer", AuthenticationState.AUTHENTICATED,
                List.of(tenant), List.of(new PermissionGrant(
                        tenant, PlatformPermissions.ENTITY_READ,
                        ResourceScope.named(ResourceType.ENTITY, List.of(entity)))));
    }

    private static void authenticate(PrincipalContext principal) {
        // Only the authenticated Platform token is accepted by PlatformSecurityContext.
        SecurityContextHolder.getContext().setAuthentication(new PlatformAuthenticationToken(
                mock(ResolvedAuthenticatedIdentity.class), principal));
    }
}
