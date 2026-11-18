package com.platform.core.domain.monitoring;

import com.platform.core.persistence.MonitoringProfileRepository;
import com.platform.core.persistence.MonitoringProfileRepository.Assignment;
import com.platform.core.security.authorization.AuthenticationState;
import com.platform.core.security.authorization.PermissionGrant;
import com.platform.core.security.authorization.PlatformPermissions;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.authorization.ResourceScope;
import com.platform.core.security.authorization.ResourceType;
import com.platform.core.security.http.UnauthorizedAccessException;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.List;
import java.util.Optional;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;

class MonitoringProfileEntityReadServiceTest {
    private final UUID tenantA = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private final UUID tenantB = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b2");
    private final UUID entityA = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c3");
    private final UUID entityB = UUID.fromString("018f0e0a-0000-7000-8000-0000000000d4");
    private final UUID profileA = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e5");

    private MonitoringProfileRepository repo;
    private MonitoringProfileEntityReadService service;

    @BeforeEach
    void setUp() {
        repo = mock(MonitoringProfileRepository.class);
        service = new MonitoringProfileEntityReadService(repo);
    }

    @Test
    void nullPrincipalFailsBeforeAnyRepositoryLookup() {
        assertMasked(() -> service.findEntityAssignment(null, tenantA, entityA));
        verifyNoInteractions(repo);
    }

    @Test
    void authenticatedPrincipalWithoutEntityReadGrantIsDenied() {
        PrincipalContext principal = new PrincipalContext("viewer", AuthenticationState.AUTHENTICATED,
                List.of(tenantA), List.of());
        assertMasked(() -> service.findEntityAssignment(principal, tenantA, entityA));
        verifyNoInteractions(repo);
    }

    @Test
    void tenantScopeMismatchIsDeniedBeforeDatabaseLookup() {
        assertMasked(() -> service.findEntityAssignment(namedEntityReader(tenantA, entityA), tenantB, entityA));
        verifyNoInteractions(repo);
    }

    @Test
    void namedResourceGrantCannotReadAnotherEntityInSameTenant() {
        assertMasked(() -> service.findEntityAssignment(namedEntityReader(tenantA, entityA), tenantA, entityB));
        verifyNoInteractions(repo);
    }

    @Test
    void authorizedEntityReceivesOnlyBoundedAssignmentIdentity() {
        Instant now = Instant.parse("2026-11-18T00:00:00Z");
        Assignment a = new Assignment(tenantA, entityA, profileA, 3,
                MonitoringProfileSelection.AssignmentMode.EXPLICIT_PROFILE,
                UUID.randomUUID(), now, 9L, null, now);
        when(repo.findAssignment(tenantA, entityA)).thenReturn(Optional.of(a));

        var view = service.findEntityAssignment(namedEntityReader(tenantA, entityA), tenantA, entityA)
                .orElseThrow();
        assertEquals(entityA, view.entityId());
        assertEquals(profileA, view.profileId());
        assertEquals(3, view.profileVersionNo());
        assertEquals(MonitoringProfileSelection.AssignmentMode.EXPLICIT_PROFILE, view.assignmentMode());
        assertEquals(9L, view.rowVersion());
        assertEquals(now, view.effectiveFrom());
        verify(repo).findAssignment(tenantA, entityA);
    }

    @Test
    void authorizedEntityWithoutAssignmentReturnsEmpty() {
        when(repo.findAssignment(tenantA, entityA)).thenReturn(Optional.empty());
        assertTrue(service.findEntityAssignment(namedEntityReader(tenantA, entityA), tenantA, entityA)
                .isEmpty());
        verify(repo).findAssignment(tenantA, entityA);
    }

    @Test
    void mismatchedTenantRowIsRejectedWithoutDisclosure() {
        Instant now = Instant.parse("2026-11-18T00:00:00Z");
        when(repo.findAssignment(tenantA, entityA)).thenReturn(Optional.of(new Assignment(
                tenantB, entityA, profileA, 1, MonitoringProfileSelection.AssignmentMode.AUTO_FROM_CRITICALITY,
                UUID.randomUUID(), now, 1, null, now)));
        assertThrows(IllegalStateException.class, () ->
                service.findEntityAssignment(namedEntityReader(tenantA, entityA), tenantA, entityA));
    }

    @Test
    void mismatchedEntityRowIsRejectedWithoutDisclosure() {
        Instant now = Instant.parse("2026-11-18T00:00:00Z");
        when(repo.findAssignment(tenantA, entityA)).thenReturn(Optional.of(new Assignment(
                tenantA, entityB, profileA, 1, MonitoringProfileSelection.AssignmentMode.AUTO_FROM_CRITICALITY,
                UUID.randomUUID(), now, 1, null, now)));
        assertThrows(IllegalStateException.class, () ->
                service.findEntityAssignment(namedEntityReader(tenantA, entityA), tenantA, entityA));
    }

    private static PrincipalContext namedEntityReader(UUID tenant, UUID entity) {
        return new PrincipalContext("viewer", AuthenticationState.AUTHENTICATED,
                List.of(tenant), List.of(new PermissionGrant(
                        tenant, PlatformPermissions.ENTITY_READ,
                        ResourceScope.named(ResourceType.ENTITY, List.of(entity)))));
    }

    private static void assertMasked(org.junit.jupiter.api.function.Executable call) {
        UnauthorizedAccessException ex = assertThrows(UnauthorizedAccessException.class, call);
        assertEquals(404, ex.httpStatus());
        assertEquals("RESOURCE_NOT_FOUND", ex.code());
    }
}
