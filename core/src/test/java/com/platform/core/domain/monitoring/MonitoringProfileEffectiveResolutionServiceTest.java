package com.platform.core.domain.monitoring;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.AssignmentMode;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.ProfileRef;
import com.platform.core.persistence.MonitoringProfileRepository;
import com.platform.core.persistence.MonitoringProfileRepository.Assignment;
import com.platform.core.persistence.MonitoringProfileRepository.Version;
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
import static org.mockito.Mockito.verifyNoMoreInteractions;
import static org.mockito.Mockito.when;

class MonitoringProfileEffectiveResolutionServiceTest {
    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID OTHER_TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b2");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c3");
    private static final UUID OTHER_ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000d4");
    private static final UUID PROFILE_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e5");
    private static final UUID PROFILE_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000f6");
    private static final Instant NOW = Instant.parse("2026-11-18T00:00:00Z");

    private MonitoringProfileRepository repository;
    private MonitoringProfileEffectiveResolutionService service;

    @BeforeEach
    void setUp() {
        repository = mock(MonitoringProfileRepository.class);
        service = new MonitoringProfileEffectiveResolutionService(repository);
    }

    @Test
    void resolvesValidatedAutomaticSuggestion() {
        when(repository.findAssignment(TENANT, ENTITY)).thenReturn(Optional.of(assignment(
                TENANT, ENTITY, PROFILE_A, 1, AssignmentMode.AUTO_FROM_CRITICALITY)));
        when(repository.findVersion(TENANT, PROFILE_B, 3)).thenReturn(Optional.of(version(TENANT, PROFILE_B, 3)));

        var result = service.resolve(reader(TENANT, ENTITY), TENANT, ENTITY,
                ref(TENANT, PROFILE_B, 3)).orElseThrow();

        assertEquals(AssignmentMode.AUTO_FROM_CRITICALITY, result.assignmentMode());
        assertEquals(ref(TENANT, PROFILE_B, 3), result.suggestedProfile());
        assertEquals(ref(TENANT, PROFILE_B, 3), result.effectiveProfile());
        verify(repository).findAssignment(TENANT, ENTITY);
        verify(repository).findVersion(TENANT, PROFILE_B, 3);
        verifyNoMoreInteractions(repository);
    }

    @Test
    void explicitAssignmentRemainsPinnedWhenSuggestionChanges() {
        when(repository.findAssignment(TENANT, ENTITY)).thenReturn(Optional.of(assignment(
                TENANT, ENTITY, PROFILE_A, 7, AssignmentMode.EXPLICIT_PROFILE)));
        when(repository.findVersion(TENANT, PROFILE_B, 4)).thenReturn(Optional.of(version(TENANT, PROFILE_B, 4)));
        when(repository.findVersion(TENANT, PROFILE_A, 7)).thenReturn(Optional.of(version(TENANT, PROFILE_A, 7)));

        var result = service.resolve(reader(TENANT, ENTITY), TENANT, ENTITY,
                ref(TENANT, PROFILE_B, 4)).orElseThrow();

        assertEquals(ref(TENANT, PROFILE_B, 4), result.suggestedProfile());
        assertEquals(ref(TENANT, PROFILE_A, 7), result.effectiveProfile());
        verify(repository).findAssignment(TENANT, ENTITY);
        verify(repository).findVersion(TENANT, PROFILE_B, 4);
        verify(repository).findVersion(TENANT, PROFILE_A, 7);
        verifyNoMoreInteractions(repository);
    }

    @Test
    void rejectsWrongTenantSuggestionBeforeVersionLookup() {
        when(repository.findAssignment(TENANT, ENTITY)).thenReturn(Optional.of(assignment(
                TENANT, ENTITY, PROFILE_A, 1, AssignmentMode.AUTO_FROM_CRITICALITY)));

        DomainException ex = assertThrows(DomainException.class, () -> service.resolve(
                reader(TENANT, ENTITY), TENANT, ENTITY, ref(OTHER_TENANT, PROFILE_B, 2)));

        assertEquals(MonitoringProfileSelection.ERR_VALIDATION, ex.getCode());
        verify(repository).findAssignment(TENANT, ENTITY);
        verifyNoMoreInteractions(repository);
    }

    @Test
    void deniesWithoutPermissionBeforeAnyRepositoryAccess() {
        PrincipalContext noPermission = new PrincipalContext("viewer", AuthenticationState.AUTHENTICATED,
                List.of(TENANT), List.of());

        assertMasked(() -> service.resolve(noPermission, TENANT, ENTITY, ref(TENANT, PROFILE_A, 1)));
        verifyNoInteractions(repository);
    }

    @Test
    void namedEntityGrantCannotReadAnotherEntity() {
        assertMasked(() -> service.resolve(reader(TENANT, ENTITY), TENANT, OTHER_ENTITY,
                ref(TENANT, PROFILE_A, 1)));
        verifyNoInteractions(repository);
    }

    @Test
    void absentAssignmentReturnsEmptyWithoutRequiringSuggestion() {
        when(repository.findAssignment(TENANT, ENTITY)).thenReturn(Optional.empty());

        assertTrue(service.resolve(reader(TENANT, ENTITY), TENANT, ENTITY, null).isEmpty());

        verify(repository).findAssignment(TENANT, ENTITY);
        verifyNoMoreInteractions(repository);
    }

    @Test
    void missingSuppliedSuggestionFailsClosed() {
        when(repository.findAssignment(TENANT, ENTITY)).thenReturn(Optional.of(assignment(
                TENANT, ENTITY, PROFILE_A, 1, AssignmentMode.AUTO_FROM_CRITICALITY)));

        DomainException ex = assertThrows(DomainException.class,
                () -> service.resolve(reader(TENANT, ENTITY), TENANT, ENTITY, null));

        assertEquals(MonitoringProfileSelection.ERR_VALIDATION, ex.getCode());
        verify(repository).findAssignment(TENANT, ENTITY);
        verifyNoMoreInteractions(repository);
    }

    @Test
    void missingProfileVersionFailsClosed() {
        when(repository.findAssignment(TENANT, ENTITY)).thenReturn(Optional.of(assignment(
                TENANT, ENTITY, PROFILE_A, 1, AssignmentMode.AUTO_FROM_CRITICALITY)));
        when(repository.findVersion(TENANT, PROFILE_B, 2)).thenReturn(Optional.empty());

        assertThrows(DomainException.class, () -> service.resolve(reader(TENANT, ENTITY), TENANT, ENTITY,
                ref(TENANT, PROFILE_B, 2)));

        verify(repository).findAssignment(TENANT, ENTITY);
        verify(repository).findVersion(TENANT, PROFILE_B, 2);
        verifyNoMoreInteractions(repository);
    }

    @Test
    void tenantCorruptAssignmentRecordFailsClosed() {
        when(repository.findAssignment(TENANT, ENTITY)).thenReturn(Optional.of(assignment(
                OTHER_TENANT, ENTITY, PROFILE_A, 1, AssignmentMode.AUTO_FROM_CRITICALITY)));

        assertThrows(IllegalStateException.class, () -> service.resolve(reader(TENANT, ENTITY),
                TENANT, ENTITY, ref(TENANT, PROFILE_A, 1)));

        verify(repository).findAssignment(TENANT, ENTITY);
        verifyNoMoreInteractions(repository);
    }

    @Test
    void validatesPersistedVersionIdentityAndDoesNotMutateRepository() {
        when(repository.findAssignment(TENANT, ENTITY)).thenReturn(Optional.of(assignment(
                TENANT, ENTITY, PROFILE_A, 1, AssignmentMode.AUTO_FROM_CRITICALITY)));
        when(repository.findVersion(TENANT, PROFILE_B, 3)).thenReturn(Optional.of(version(OTHER_TENANT, PROFILE_B, 3)));

        assertThrows(IllegalStateException.class, () -> service.resolve(reader(TENANT, ENTITY), TENANT, ENTITY,
                ref(TENANT, PROFILE_B, 3)));

        verify(repository).findAssignment(TENANT, ENTITY);
        verify(repository).findVersion(TENANT, PROFILE_B, 3);
        verifyNoMoreInteractions(repository);
    }

    private static Assignment assignment(UUID tenant, UUID entity, UUID profile, int version, AssignmentMode mode) {
        return new Assignment(tenant, entity, profile, version, mode, UUID.randomUUID(), NOW, 1L, null, NOW);
    }

    private static Version version(UUID tenant, UUID profile, int version) {
        return new Version(tenant, profile, version, "{}", "00", "{}", UUID.randomUUID(), NOW, null);
    }

    private static ProfileRef ref(UUID tenant, UUID profile, int version) {
        return new ProfileRef(tenant, profile.toString(), version);
    }

    private static PrincipalContext reader(UUID tenant, UUID entity) {
        return new PrincipalContext("viewer", AuthenticationState.AUTHENTICATED, List.of(tenant),
                List.of(new PermissionGrant(tenant, PlatformPermissions.ENTITY_READ,
                        ResourceScope.named(ResourceType.ENTITY, List.of(entity)))));
    }

    private static void assertMasked(org.junit.jupiter.api.function.Executable action) {
        UnauthorizedAccessException ex = assertThrows(UnauthorizedAccessException.class, action);
        assertEquals(404, ex.httpStatus());
        assertEquals("RESOURCE_NOT_FOUND", ex.code());
    }
}
