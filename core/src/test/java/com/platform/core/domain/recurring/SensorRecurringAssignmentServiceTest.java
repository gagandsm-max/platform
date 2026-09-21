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
    private static final String SENSOR = "sensor-01";

    private SensorRecurringAssignmentRepository repository;
    private SecurityAuditEmitter auditEmitter;
    private SensorRecurringAssignmentService service;

    @BeforeEach
    void setUp() {
        repository = mock(SensorRecurringAssignmentRepository.class);
        auditEmitter = mock(SecurityAuditEmitter.class);
        service = new SensorRecurringAssignmentService(repository, auditEmitter);
        authenticateAs("admin-user");
    }

    @AfterEach
    void tearDown() {
        SecurityContextHolder.clearContext();
    }

    @Test
    void replaceAssignmentsRejectsForbiddenSecretKeysInProtocolParamsJson() {
        stubActiveMutationLocks(0L);

        SensorRecurringAssignmentService.AssignmentInput input =
                new SensorRecurringAssignmentService.AssignmentInput(
                        "assign-1",
                        "entity-1",
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

        String json = "{\"target\":\"127.0.0.1\",\"port\":161,\"version\":\"3\",\"operation\":\"GET\","
                + "\"oids\":[\".1.3.6.1.2.1.1.3.0\"],\"timeoutMs\":1000,"
                + "\"limits\":{\"maxOids\":16,\"maxWalkEntries\":100,\"maxBulkRepetitions\":10,"
                + "\"maxResults\":100,\"maxResultBytes\":65536},"
                + "\"authSecret\":\"sekrit\"}";

        SensorRecurringAssignmentService.AssignmentInput input =
                new SensorRecurringAssignmentService.AssignmentInput(
                        "assign-1", "entity-1", "mon-1", "LIVENESS", "NETWORK_POLL",
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
        when(repository.listAssignmentDigests(TENANT, SENSOR)).thenReturn(List.of());
        when(repository.listAssignments(TENANT, SENSOR)).thenReturn(List.of());

        SnmpParams.SnmpLimits limits = new SnmpParams.SnmpLimits(16, 100, 10, 100, 65_536);
        SnmpParams snmp = new SnmpParams(
                "127.0.0.1", 161, "2c", "GET", List.of(".1.3.6.1.2.1.1.3.0"), 1_000L, limits);
        SensorRecurringAssignmentService.AssignmentInput input =
                new SensorRecurringAssignmentService.AssignmentInput(
                        "assign-1", "entity-1", "mon-1", "LIVENESS", "NETWORK_POLL",
                        60_000L, 90_000L, 1, 5_000L, "cred-1", "SNMP", snmp, null);

        service.replaceAssignments(TENANT, SENSOR, 0L, List.of(input));

        ArgumentCaptor<RecurringAssignment> rowCaptor = ArgumentCaptor.forClass(RecurringAssignment.class);
        verify(repository).deleteAllAssignments(TENANT, SENSOR);
        verify(repository).insertAssignment(rowCaptor.capture());
        assertEquals("assign-1", rowCaptor.getValue().assignmentId());
        assertTrue(rowCaptor.getValue().protocolParamsJson().contains("\"target\":\"127.0.0.1\""));
        verify(repository).upsertDesiredState(eq(TENANT), eq(SENSOR), eq(1L), anyString(), eq("admin-user"), any());
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
