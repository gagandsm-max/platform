package com.platform.core.agentcontrol;

import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.SensorScope;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class AgentAuthorityQueryServiceTest {

    private static final Instant NOW = Instant.parse("2026-09-10T06:00:00Z");
    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private static final UUID OTHER_TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000002");
    private static final UUID SITE = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final String FP =
            "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

    private AgentControlRepository repository;
    private AgentAuthorityMetrics metrics;
    private AgentAuthorityQueryService service;
    private SensorScope callingSensor;

    @BeforeEach
    void setUp() {
        repository = mock(AgentControlRepository.class);
        metrics = new AgentAuthorityMetrics();
        service = new AgentAuthorityQueryService(repository, metrics, Clock.fixed(NOW, ZoneOffset.UTC));
        callingSensor = new SensorScope("sensor-infra-01", TENANT, SITE, Set.of(), true);
    }

    private AgentCertificateCredential credential(
            AgentCredentialStatus status, Instant notBefore, Instant notAfter, String agentId
    ) {
        return new AgentCertificateCredential(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000c1"),
                TENANT,
                agentId,
                FP,
                null,
                "1",
                "CN=Platform Test CA",
                null,
                notBefore,
                notAfter,
                status,
                notBefore,
                status == AgentCredentialStatus.ACTIVE ? null : NOW,
                notBefore,
                NOW
        );
    }

    private AgentPrincipal principal(String agentId, AgentLifecycleStatus status) {
        return new AgentPrincipal(agentId, TENANT, SITE, status, NOW, NOW);
    }

    private AgentSensorAssignment assignment(String agentId, String sensorId, long generation) {
        return new AgentSensorAssignment(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1"),
                TENANT,
                agentId,
                sensorId,
                generation,
                AgentAssignmentStatus.ACTIVE,
                NOW,
                NOW,
                null
        );
    }

    private void stubHappyPath() {
        when(repository.findCredentialByFingerprint(FP))
                .thenReturn(Optional.of(credential(AgentCredentialStatus.ACTIVE,
                        NOW.minusSeconds(60), NOW.plusSeconds(3600), "agent-01")));
        when(repository.findPrincipalByTenantAndId(TENANT, "agent-01"))
                .thenReturn(Optional.of(principal("agent-01", AgentLifecycleStatus.ACTIVE)));
        when(repository.findActiveAssignmentForAgent(TENANT, "agent-01"))
                .thenReturn(Optional.of(assignment("agent-01", "sensor-infra-01", 3)));
    }

    @Test
    void resolveSuccessReturnsBoundedView() {
        stubHappyPath();
        AgentAuthorityView view = service.resolveForSensor(callingSensor, FP);
        assertEquals("agent-01", view.agentId());
        assertEquals(TENANT, view.tenantId());
        assertEquals(SITE, view.siteId());
        assertEquals(3L, view.assignmentGeneration());
        assertEquals("sensor-infra-01", view.sensorId());
        assertEquals(1, metrics.getResolveSuccesses());
    }

    @Test
    void unknownFingerprintFailsClosed() {
        when(repository.findCredentialByFingerprint(any())).thenReturn(Optional.empty());
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP));
        assertEquals("ERR_AGENT_CREDENTIAL_UNKNOWN", ex.getCode());
        assertEquals(1, metrics.getReasonCount(AgentAuthorityMetrics.REASON_UNKNOWN_FINGERPRINT));
    }

    @Test
    void revokedCredentialFailsClosed() {
        when(repository.findCredentialByFingerprint(FP))
                .thenReturn(Optional.of(credential(AgentCredentialStatus.REVOKED,
                        NOW.minusSeconds(60), NOW.plusSeconds(3600), "agent-01")));
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP));
        assertEquals("ERR_AGENT_CREDENTIAL_REVOKED", ex.getCode());
    }

    @Test
    void supersededCredentialFailsClosed() {
        when(repository.findCredentialByFingerprint(FP))
                .thenReturn(Optional.of(credential(AgentCredentialStatus.SUPERSEDED,
                        NOW.minusSeconds(60), NOW.plusSeconds(3600), "agent-01")));
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP));
        assertEquals("ERR_AGENT_CREDENTIAL_REVOKED", ex.getCode());
        assertEquals(1, metrics.getReasonCount(AgentAuthorityMetrics.REASON_CREDENTIAL_SUPERSEDED));
    }

    @Test
    void expiredCredentialFailsClosed() {
        when(repository.findCredentialByFingerprint(FP))
                .thenReturn(Optional.of(credential(AgentCredentialStatus.ACTIVE,
                        NOW.minusSeconds(7200), NOW.minusSeconds(60), "agent-01")));
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP));
        assertEquals("ERR_AGENT_CREDENTIAL_EXPIRED", ex.getCode());
        assertEquals(1, metrics.getReasonCount(AgentAuthorityMetrics.REASON_CREDENTIAL_EXPIRED));
    }

    @Test
    void notYetValidCredentialFailsClosed() {
        when(repository.findCredentialByFingerprint(FP))
                .thenReturn(Optional.of(credential(AgentCredentialStatus.ACTIVE,
                        NOW.plusSeconds(60), NOW.plusSeconds(3600), "agent-01")));
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP));
        assertEquals("ERR_AGENT_CREDENTIAL_EXPIRED", ex.getCode());
        assertEquals(1, metrics.getReasonCount(AgentAuthorityMetrics.REASON_CREDENTIAL_NOT_YET_VALID));
    }

    @Test
    void disabledAgentFailsClosed() {
        when(repository.findCredentialByFingerprint(FP))
                .thenReturn(Optional.of(credential(AgentCredentialStatus.ACTIVE,
                        NOW.minusSeconds(60), NOW.plusSeconds(3600), "agent-01")));
        when(repository.findPrincipalByTenantAndId(TENANT, "agent-01"))
                .thenReturn(Optional.of(principal("agent-01", AgentLifecycleStatus.DISABLED)));
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP));
        assertEquals("ERR_AGENT_DISABLED", ex.getCode());
    }

    @Test
    void revokedAgentFailsClosed() {
        when(repository.findCredentialByFingerprint(FP))
                .thenReturn(Optional.of(credential(AgentCredentialStatus.ACTIVE,
                        NOW.minusSeconds(60), NOW.plusSeconds(3600), "agent-01")));
        when(repository.findPrincipalByTenantAndId(TENANT, "agent-01"))
                .thenReturn(Optional.of(principal("agent-01", AgentLifecycleStatus.REVOKED)));
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP));
        assertEquals("ERR_AGENT_DISABLED", ex.getCode());
        assertEquals(1, metrics.getReasonCount(AgentAuthorityMetrics.REASON_AGENT_REVOKED));
    }

    @Test
    void missingAssignmentFailsClosed() {
        when(repository.findCredentialByFingerprint(FP))
                .thenReturn(Optional.of(credential(AgentCredentialStatus.ACTIVE,
                        NOW.minusSeconds(60), NOW.plusSeconds(3600), "agent-01")));
        when(repository.findPrincipalByTenantAndId(TENANT, "agent-01"))
                .thenReturn(Optional.of(principal("agent-01", AgentLifecycleStatus.ACTIVE)));
        when(repository.findActiveAssignmentForAgent(TENANT, "agent-01")).thenReturn(Optional.empty());
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP));
        assertEquals("ERR_AGENT_ASSIGNMENT_MISSING", ex.getCode());
    }

    @Test
    void wrongSensorExactMismatchFailsClosed() {
        stubHappyPath();
        when(repository.findActiveAssignmentForAgent(TENANT, "agent-01"))
                .thenReturn(Optional.of(assignment("agent-01", "sensor-OTHER", 3)));
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP));
        assertEquals("ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH", ex.getCode());
    }

    @Test
    void sensorIdCaseMismatchFailsClosed() {
        stubHappyPath();
        when(repository.findActiveAssignmentForAgent(TENANT, "agent-01"))
                .thenReturn(Optional.of(assignment("agent-01", "Sensor-Infra-01", 3)));
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP));
        assertEquals("ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH", ex.getCode());
        assertEquals(1, metrics.getReasonCount(AgentAuthorityMetrics.REASON_SENSOR_MISMATCH));
    }

    @Test
    void tenantMismatchFailsClosed() {
        stubHappyPath();
        SensorScope otherTenant = new SensorScope("sensor-infra-01", OTHER_TENANT, SITE, Set.of(), true);
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(otherTenant, FP));
        assertEquals("ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH", ex.getCode());
        assertEquals(1, metrics.getReasonCount(AgentAuthorityMetrics.REASON_TENANT_MISMATCH));
    }

    @Test
    void staleGenerationIsNotAcceptedWhenAssignmentMoved() {
        // Calling sensor still thinks generation 3, but ACTIVE assignment is now on another sensor gen 4.
        when(repository.findCredentialByFingerprint(FP))
                .thenReturn(Optional.of(credential(AgentCredentialStatus.ACTIVE,
                        NOW.minusSeconds(60), NOW.plusSeconds(3600), "agent-01")));
        when(repository.findPrincipalByTenantAndId(TENANT, "agent-01"))
                .thenReturn(Optional.of(principal("agent-01", AgentLifecycleStatus.ACTIVE)));
        when(repository.findActiveAssignmentForAgent(eq(TENANT), eq("agent-01")))
                .thenReturn(Optional.of(assignment("agent-01", "sensor-infra-02", 4)));

        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP));
        assertEquals("ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH", ex.getCode());
    }

    @Test
    void claimedGenerationMismatchFailsClosed() {
        stubHappyPath();
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP, 2L));
        assertEquals("ERR_AGENT_ASSIGNMENT_GENERATION_MISMATCH", ex.getCode());
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
        assertEquals(1, metrics.getReasonCount(AgentAuthorityMetrics.REASON_STALE_GENERATION));
    }

    @Test
    void claimedGenerationExactMatchAccepted() {
        stubHappyPath();
        AgentAuthorityView view = service.resolveForSensor(callingSensor, FP, 3L);
        assertEquals(3L, view.assignmentGeneration());
        assertEquals(1, metrics.getResolveSuccesses());
        assertEquals(0, metrics.getReasonCount(AgentAuthorityMetrics.REASON_STALE_GENERATION));
    }

    @Test
    void nullClaimedGenerationSkipsFenceClaimCheck() {
        stubHappyPath();
        AgentAuthorityView view = service.resolveForSensor(callingSensor, FP, null);
        assertEquals(3L, view.assignmentGeneration());
    }

    @Test
    void dbFailureFailsClosed() {
        when(repository.findCredentialByFingerprint(any())).thenThrow(new RuntimeException("connection reset"));
        IngestionException ex = assertThrows(IngestionException.class,
                () -> service.resolveForSensor(callingSensor, FP));
        assertEquals("ERR_AGENT_AUTHORITY_UNAVAILABLE", ex.getCode());
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
        assertEquals(1, metrics.getReasonCount(AgentAuthorityMetrics.REASON_DB_ERROR));
    }
}
