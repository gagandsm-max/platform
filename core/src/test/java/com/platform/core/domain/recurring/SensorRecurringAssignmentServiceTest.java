package com.platform.core.domain.recurring;

import com.platform.core.domain.DomainException;
import com.platform.core.persistence.SensorRecurringAssignmentRepository;
import com.platform.core.security.authorization.AuthenticationState;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.http.PlatformAuthenticationToken;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.security.identity.AuthenticatedExternalIdentity;
import com.platform.core.security.identity.AuthenticationAssurance;
import com.platform.core.security.identity.ExternalSubject;
import com.platform.core.security.identity.IdentityProviderType;
import com.platform.core.security.identity.PlatformSubject;
import com.platform.core.security.identity.ProviderInstanceId;
import com.platform.core.security.identity.ResolvedAuthenticatedIdentity;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.sensorcontrol.SensorLifecycleStatus;
import com.platform.core.sensorcontrol.SensorPrincipal;
import com.platform.core.sensorcontrol.SensorRole;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.security.core.context.SecurityContextHolder;

import java.time.Instant;
import java.util.List;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class SensorRecurringAssignmentServiceTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID OTHER_TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b2");
    private static final UUID ENTITY_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");
    private static final UUID ENTITY_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e2");
    private static final UUID ENTITY_OTHER_TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e9");
    private static final UUID ENTITY_MISSING = UUID.fromString("018f0e0a-0000-7000-8000-0000000000ee");
    private static final String SENSOR = "sensor-01";

    private SensorRecurringAssignmentRepository repository;
    private SensorControlRepository sensorControlRepository;
    private SecurityAuditEmitter auditEmitter;
    private SensorRecurringAssignmentService service;

    @BeforeEach
    void setUp() {
        repository = mock(SensorRecurringAssignmentRepository.class);
        sensorControlRepository = mock(SensorControlRepository.class);
        auditEmitter = mock(SecurityAuditEmitter.class);
        service = new SensorRecurringAssignmentService(repository, sensorControlRepository, auditEmitter);
        authenticateAs("admin-user");
    }

    @AfterEach
    void tearDown() {
        SecurityContextHolder.clearContext();
    }

    @Test
    void replaceAssignmentsRejectsForbiddenSecretKeysInProtocolParamsJson() {
        stubActiveMutationLocks(0L);
        stubRestrictedSensorAllowlist(Set.of(ENTITY_A));
        when(sensorControlRepository.entityExists(TENANT, ENTITY_A)).thenReturn(true);

        SensorRecurringAssignmentService.AssignmentInput input =
                new SensorRecurringAssignmentService.AssignmentInput(
                        "assign-1",
                        ENTITY_A.toString(),
                        "mon-1",
                        "LIVENESS",
                        "NETWORK_POLL",
                        60_000L,
                        90_000L,
                        1,
                        5_000L,
                        "cred-1",
                        "SNMP",
                        null,
                        "{\"target\":\"127.0.0.1\",\"port\":161,\"version\":\"2c\",\"operation\":\"GET\","
                                + "\"oids\":[\".1.3.6.1.2.1.1.3.0\"],\"timeoutMs\":1000,"
                                + "\"limits\":{\"maxOids\":16,\"maxWalkEntries\":100,\"maxBulkRepetitions\":10,"
                                + "\"maxResults\":100,\"maxResultBytes\":65536},"
                                + "\"community\":\"public\"}"
                );

        DomainException ex = assertThrows(DomainException.class,
                () -> service.replaceAssignments(TENANT, SENSOR, 0L, List.of(input)));
        assertEquals("VALIDATION_FAILED", ex.getCode());
        assertTrue(ex.getMessage().contains("forbidden secret key"));
        verify(repository, never()).deleteAllAssignments(any(), any());
    }

    @Test
    void replaceAssignmentsRejectsAuthSecretKeyVariants() {
        stubActiveMutationLocks(0L);
        stubRestrictedSensorAllowlist(Set.of(ENTITY_A));
        when(sensorControlRepository.entityExists(TENANT, ENTITY_A)).thenReturn(true);

        String json = "{\"target\":\"127.0.0.1\",\"port\":161,\"version\":\"3\",\"operation\":\"GET\","
                + "\"oids\":[\".1.3.6.1.2.1.1.3.0\"],\"timeoutMs\":1000,"
                + "\"limits\":{\"maxOids\":16,\"maxWalkEntries\":100,\"maxBulkRepetitions\":10,"
                + "\"maxResults\":100,\"maxResultBytes\":65536},"
                + "\"authSecret\":\"sekrit\"}";

        SensorRecurringAssignmentService.AssignmentInput input =
                new SensorRecurringAssignmentService.AssignmentInput(
                        "assign-1", ENTITY_A.toString(), "mon-1", "LIVENESS", "NETWORK_POLL",
                        60_000L, 90_000L, 1, 5_000L, "cred-1", "SNMP", null, json);

        DomainException ex = assertThrows(DomainException.class,
                () -> service.replaceAssignments(TENANT, SENSOR, 0L, List.of(input)));
        assertEquals("VALIDATION_FAILED", ex.getCode());
        assertTrue(ex.getMessage().toLowerCase().contains("forbidden"));
    }

    @Test
    void replaceAssignmentsCasConflict() {
        stubActiveMutationLocks(3L);

        DomainException ex = assertThrows(DomainException.class,
                () -> service.replaceAssignments(TENANT, SENSOR, 0L, List.of()));
        assertEquals(SensorRecurringAssignmentService.ERR_GENERATION_CONFLICT, ex.getCode());
    }

    @Test
    void replaceAssignmentsPersistsCanonicalRowsAndIncrementsGeneration() {
        stubActiveMutationLocks(0L);
        stubRestrictedSensorAllowlist(Set.of(ENTITY_A));
        when(sensorControlRepository.entityExists(TENANT, ENTITY_A)).thenReturn(true);
        when(repository.listAssignmentDigests(TENANT, SENSOR)).thenReturn(List.of());
        when(repository.listAssignments(TENANT, SENSOR)).thenReturn(List.of());

        service.replaceAssignments(TENANT, SENSOR, 0L, List.of(validInput(ENTITY_A)));

        ArgumentCaptor<RecurringAssignment> rowCaptor = ArgumentCaptor.forClass(RecurringAssignment.class);
        verify(repository).deleteAllAssignments(TENANT, SENSOR);
        verify(repository).insertAssignment(rowCaptor.capture());
        assertEquals("assign-1", rowCaptor.getValue().assignmentId());
        assertEquals(ENTITY_A.toString(), rowCaptor.getValue().entityId());
        assertTrue(rowCaptor.getValue().protocolParamsJson().contains("\"target\":\"127.0.0.1\""));
        verify(repository).upsertDesiredState(eq(TENANT), eq(SENSOR), eq(1L), anyString(), eq("admin-user"), any());
    }

    @Test
    void replaceAssignmentsRejectsNonUuidEntityId() {
        stubActiveMutationLocks(0L);
        stubRestrictedSensorAllowlist(Set.of(ENTITY_A));

        DomainException ex = assertThrows(DomainException.class,
                () -> service.replaceAssignments(TENANT, SENSOR, 0L, List.of(
                        new SensorRecurringAssignmentService.AssignmentInput(
                                "assign-1", "entity-opaque", "mon-1", "LIVENESS", "NETWORK_POLL",
                                60_000L, 90_000L, 1, 5_000L, "cred-1", "SNMP", sampleSnmp(), null))));
        assertEquals(SensorRecurringAssignmentService.ERR_SENSOR_ENTITY_INVALID, ex.getCode());
        verify(repository, never()).insertAssignment(any());
    }

    @Test
    void replaceAssignmentsRejectsNonexistentEntity() {
        stubActiveMutationLocks(0L);
        stubAllowAllSensor();
        when(sensorControlRepository.entityExists(TENANT, ENTITY_MISSING)).thenReturn(false);

        DomainException ex = assertThrows(DomainException.class,
                () -> service.replaceAssignments(TENANT, SENSOR, 0L, List.of(validInput(ENTITY_MISSING))));
        assertEquals(SensorRecurringAssignmentService.ERR_SENSOR_ENTITY_INVALID, ex.getCode());
    }

    @Test
    void replaceAssignmentsRejectsCrossTenantEntity() {
        stubActiveMutationLocks(0L);
        stubAllowAllSensor();
        // Other-tenant entity is not present under this tenant_id predicate.
        when(sensorControlRepository.entityExists(TENANT, ENTITY_OTHER_TENANT)).thenReturn(false);

        DomainException ex = assertThrows(DomainException.class,
                () -> service.replaceAssignments(TENANT, SENSOR, 0L, List.of(validInput(ENTITY_OTHER_TENANT))));
        assertEquals(SensorRecurringAssignmentService.ERR_SENSOR_ENTITY_INVALID, ex.getCode());
        verify(sensorControlRepository, never()).entityExists(eq(OTHER_TENANT), any());
    }

    @Test
    void replaceAssignmentsRejectsValidTenantEntityNotOnRestrictedAllowlist() {
        stubActiveMutationLocks(0L);
        stubRestrictedSensorAllowlist(Set.of(ENTITY_A));
        when(sensorControlRepository.entityExists(TENANT, ENTITY_B)).thenReturn(true);

        DomainException ex = assertThrows(DomainException.class,
                () -> service.replaceAssignments(TENANT, SENSOR, 0L, List.of(validInput(ENTITY_B))));
        assertEquals(SensorRecurringAssignmentService.ERR_ENTITY_SCOPE_UNAUTHORIZED, ex.getCode());
        verify(repository, never()).insertAssignment(any());
    }

    @Test
    void replaceAssignmentsAllowsEntityOnRestrictedAllowlist() {
        stubActiveMutationLocks(0L);
        stubRestrictedSensorAllowlist(Set.of(ENTITY_A));
        when(sensorControlRepository.entityExists(TENANT, ENTITY_A)).thenReturn(true);
        when(repository.listAssignmentDigests(TENANT, SENSOR)).thenReturn(List.of());
        when(repository.listAssignments(TENANT, SENSOR)).thenReturn(List.of());

        service.replaceAssignments(TENANT, SENSOR, 0L, List.of(validInput(ENTITY_A)));
        verify(repository).insertAssignment(any());
    }

    @Test
    void replaceAssignmentsAllowsAnyTenantEntityWhenAllowAll() {
        stubActiveMutationLocks(0L);
        stubAllowAllSensor();
        when(sensorControlRepository.entityExists(TENANT, ENTITY_B)).thenReturn(true);
        when(repository.listAssignmentDigests(TENANT, SENSOR)).thenReturn(List.of());
        when(repository.listAssignments(TENANT, SENSOR)).thenReturn(List.of());

        service.replaceAssignments(TENANT, SENSOR, 0L, List.of(validInput(ENTITY_B)));
        verify(repository).insertAssignment(any());
        verify(sensorControlRepository, never()).listAllowedEntityIds(any(), any());
    }

    @Test
    void replaceAssignmentsRejectsInactiveSensor() {
        when(repository.lockTenantForUpdate(TENANT)).thenReturn(true);
        when(repository.findSensorStatus(TENANT, SENSOR)).thenReturn(Optional.of("DISABLED"));

        DomainException ex = assertThrows(DomainException.class,
                () -> service.replaceAssignments(TENANT, SENSOR, 0L, List.of(validInput(ENTITY_A))));
        assertEquals(SensorRecurringAssignmentService.ERR_SENSOR_NOT_ACTIVE, ex.getCode());
    }

    @Test
    void getDesiredStateReconcilesRevokedAllowlistAndBumpsGeneration() {
        when(repository.lockTenantForUpdate(TENANT)).thenReturn(true);
        when(repository.lockSensorForUpdate(TENANT, SENSOR)).thenReturn(Optional.of("ACTIVE"));
        when(repository.findStateForUpdate(TENANT, SENSOR)).thenReturn(Optional.of(
                new RecurringState(TENANT, SENSOR, 2L, "a".repeat(64), 1L, "b".repeat(64),
                        Instant.now(), "actor", Instant.now())));
        stubRestrictedSensorAllowlist(Set.of(ENTITY_A));
        when(sensorControlRepository.entityExists(TENANT, ENTITY_A)).thenReturn(true);
        when(sensorControlRepository.entityExists(TENANT, ENTITY_B)).thenReturn(true);

        RecurringAssignment keep = sampleAssignment("assign-keep", ENTITY_A);
        RecurringAssignment drop = sampleAssignment("assign-drop", ENTITY_B);
        when(repository.listAssignments(TENANT, SENSOR)).thenReturn(List.of(keep, drop));

        SensorRecurringDesiredState desired = service.getDesiredState(TENANT, SENSOR);

        verify(repository).deleteAssignment(TENANT, SENSOR, "assign-drop");
        verify(repository).upsertDesiredState(eq(TENANT), eq(SENSOR), eq(3L), anyString(),
                eq("system:entity-scope-reconcile"), any());
        assertEquals(3L, desired.generation());
        assertEquals(1, desired.definitions().size());
        assertEquals("assign-keep", desired.definitions().get(0).assignmentId());
    }

    @Test
    void ackAppliedStateRejectsStaleGeneration() {
        when(repository.lockTenantForUpdate(TENANT)).thenReturn(true);
        when(repository.lockSensorForUpdate(TENANT, SENSOR)).thenReturn(Optional.of("ACTIVE"));
        when(repository.findStateForUpdate(TENANT, SENSOR)).thenReturn(Optional.of(
                new RecurringState(TENANT, SENSOR, 2L, "a".repeat(64), 1L, "b".repeat(64),
                        Instant.now(), "actor", Instant.now())));

        DomainException ex = assertThrows(DomainException.class,
                () -> service.ackAppliedState(TENANT, SENSOR, 1L, "a".repeat(64)));
        assertEquals(SensorRecurringAssignmentService.ERR_STATE_STALE, ex.getCode());
        verify(repository, never()).updateAppliedState(any(), any(), anyLong(), any(), any(), any(), any());
    }

    private void stubActiveMutationLocks(long desiredGeneration) {
        when(repository.lockTenantForUpdate(TENANT)).thenReturn(true);
        when(repository.findSensorStatus(TENANT, SENSOR)).thenReturn(Optional.of("ACTIVE"));
        when(repository.lockSensorForUpdate(TENANT, SENSOR)).thenReturn(Optional.of("ACTIVE"));
        if (desiredGeneration == 0L) {
            when(repository.findStateForUpdate(TENANT, SENSOR))
                    .thenReturn(Optional.of(RecurringState.neverMutated(TENANT, SENSOR)));
        } else {
            when(repository.findStateForUpdate(TENANT, SENSOR)).thenReturn(Optional.of(
                    new RecurringState(TENANT, SENSOR, desiredGeneration, "a".repeat(64),
                            desiredGeneration - 1, "b".repeat(64),
                            Instant.now(), "actor", Instant.now())));
        }
    }

    private void stubRestrictedSensorAllowlist(Set<UUID> allowed) {
        Instant now = Instant.now();
        when(sensorControlRepository.findPrincipalByTenantAndId(TENANT, SENSOR)).thenReturn(Optional.of(
                new SensorPrincipal(SENSOR, TENANT, null, SensorRole.NETWORK, SensorLifecycleStatus.ACTIVE,
                        false, now, now)));
        when(sensorControlRepository.listAllowedEntityIds(TENANT, SENSOR)).thenReturn(allowed);
    }

    private void stubAllowAllSensor() {
        Instant now = Instant.now();
        when(sensorControlRepository.findPrincipalByTenantAndId(TENANT, SENSOR)).thenReturn(Optional.of(
                new SensorPrincipal(SENSOR, TENANT, null, SensorRole.NETWORK, SensorLifecycleStatus.ACTIVE,
                        true, now, now)));
    }

    private static SensorRecurringAssignmentService.AssignmentInput validInput(UUID entityId) {
        return new SensorRecurringAssignmentService.AssignmentInput(
                "assign-1", entityId.toString(), "mon-1", "LIVENESS", "NETWORK_POLL",
                60_000L, 90_000L, 1, 5_000L, "cred-1", "SNMP", sampleSnmp(), null);
    }

    private static SnmpParams sampleSnmp() {
        SnmpParams.SnmpLimits limits = new SnmpParams.SnmpLimits(16, 100, 10, 100, 65_536);
        return new SnmpParams(
                "127.0.0.1", 161, "2c", "GET", List.of(".1.3.6.1.2.1.1.3.0"), 1_000L, limits);
    }

    private static RecurringAssignment sampleAssignment(String assignmentId, UUID entityId) {
        return new RecurringAssignment(
                TENANT, SENSOR, assignmentId, entityId.toString(), "mon-1",
                "LIVENESS", "NETWORK_POLL", 60_000L, 90_000L, 1, 5_000L, "cred-1", "SNMP",
                RecurringScheduleBounds.canonicalizeSnmpJson(sampleSnmp()),
                "actor", Instant.now());
    }

    private static void authenticateAs(String subjectId) {
        Instant now = Instant.parse("2026-06-01T00:00:00Z");
        PrincipalContext principal = new PrincipalContext(
                subjectId,
                AuthenticationState.AUTHENTICATED,
                Set.of(),
                Set.of()
        );
        UUID subjectUuid = UUID.fromString("018f0e0a-0000-7000-8000-0000000000aa");
        SecurityContextHolder.getContext().setAuthentication(
                new PlatformAuthenticationToken(
                        ResolvedAuthenticatedIdentity.current(
                                new PlatformSubject(subjectUuid),
                                new AuthenticatedExternalIdentity(
                                        IdentityProviderType.OIDC,
                                        new ProviderInstanceId("platform-oidc"),
                                        new ExternalSubject(subjectId),
                                        now,
                                        AuthenticationAssurance.withoutVerifiedMfa(),
                                        Instant.parse("2099-01-01T00:00:00Z")
                                ),
                                now
                        ),
                        principal
                )
        );
    }
}
