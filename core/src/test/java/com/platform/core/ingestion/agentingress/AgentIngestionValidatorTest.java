package com.platform.core.ingestion.agentingress;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.agentcontrol.AgentAssignmentStatus;
import com.platform.core.agentcontrol.AgentControlRepository;
import com.platform.core.agentcontrol.AgentSensorAssignment;
import com.platform.core.domain.inventory.EntityIdentifier;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.IngestionMetrics;
import com.platform.core.ingestion.agentingress.dto.AgentIngressRecord;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryBatchRequest;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.persistence.EntityIdentifierRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;

import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

class AgentIngestionValidatorTest {

    private ObjectMapper objectMapper;
    private AgentControlRepository agentRepo;
    private EntityIdentifierRepository identifierRepo;
    private AgentIngestionValidator validator;
    private IngestionMetrics metrics;

    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private final UUID entityId = UUID.fromString("018f0e0a-0000-7000-8000-000000000011");
    private final SensorScope scope = new SensorScope("sensor-infra-01", tenantId, null, Set.of(entityId), true);

    @BeforeEach
    void setUp() {
        objectMapper = new ObjectMapper().findAndRegisterModules();
        agentRepo = mock(AgentControlRepository.class);
        identifierRepo = mock(EntityIdentifierRepository.class);
        metrics = new IngestionMetrics();
        validator = new AgentIngestionValidator(
                objectMapper, null, agentRepo, identifierRepo, metrics
        );
    }

    private AgentIngressRecord sampleRecord(long ingressSeq, String acceptingSensor, long generation) throws Exception {
        return sampleRecordWithAcceptedAt(ingressSeq, acceptingSensor, generation, "2026-09-11T10:00:00Z");
    }

    private AgentIngressRecord sampleRecordWithAcceptedAt(
            long ingressSeq, String acceptingSensor, long generation, String acceptedAt
    ) throws Exception {
        String payload = """
                {"hostId":"host-abc","collectedAt":"2026-09-11T10:00:00Z","observations":[{
                  "observationId":"obs-1","metricKey":"cpu.util","value":42.5,
                  "observedAt":"2026-09-11T10:00:00Z","hostId":"host-abc"
                }]}
                """;
        JsonNode delivery = objectMapper.readTree("""
                {"sequence":10,"id":"rec-10","createdAt":"2026-09-11T10:00:00Z",
                 "checksum":"abc","payload":%s}
                """.formatted(payload));
        return new AgentIngressRecord(
                ingressSeq, "ingress-" + ingressSeq, tenantId.toString(), "agent-1",
                acceptingSensor, generation, 10, 10, "agent-batch-1", "corr-1",
                acceptedAt, List.of(delivery)
        );
    }

    private AgentTelemetryBatchRequest batchWith(AgentIngressRecord record, String sensorId) throws Exception {
        String checksum = AgentIngestionValidator.calculateStableChecksum(List.of(record));
        return new AgentTelemetryBatchRequest(
                1, sensorId, "batch-1", record.sensorIngressSequence(), record.sensorIngressSequence(),
                1, checksum, "corr-batch", List.of(record)
        );
    }

    private void stubAssignment(long generation, String sensorId) {
        when(agentRepo.findAssignmentByGeneration(eq(tenantId), eq("agent-1"), eq(generation)))
                .thenReturn(Optional.of(new AgentSensorAssignment(
                        UUID.randomUUID(), tenantId, "agent-1", sensorId, generation,
                        AgentAssignmentStatus.ACTIVE, Instant.now(), Instant.now(), null
                )));
    }

    private void stubEntity() {
        when(identifierRepo.findByResolutionKey(
                tenantId, "platform.agent", "HOST_ID", "host-abc"
        )).thenReturn(Optional.of(new EntityIdentifier(
                UUID.randomUUID(), tenantId, entityId, "HOST_ID", "platform.agent",
                "host-abc", BigDecimal.ONE, Instant.now(), Instant.now()
        )));
    }

    @Test
    void authorityMismatchFailClosed() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-other", 1);
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-other");
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_AUTH_IDENTITY_MISMATCH", ex.getCode());
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
    }

    @Test
    void acceptingSensorMismatchFailClosed() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-other", 1);
        // Batch sensor matches scope, but acceptingSensorId does not
        String checksum = AgentIngestionValidator.calculateStableChecksum(List.of(record));
        AgentTelemetryBatchRequest batch = new AgentTelemetryBatchRequest(
                1, "sensor-infra-01", "batch-1", 1, 1, 1, checksum, "corr", List.of(record)
        );
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_AUTH_IDENTITY_MISMATCH", ex.getCode());
    }

    @Test
    void unknownGenerationFailClosed() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 99);
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        when(agentRepo.findAssignmentByGeneration(tenantId, "agent-1", 99)).thenReturn(Optional.empty());
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_AGENT_ASSIGNMENT_UNKNOWN", ex.getCode());
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
    }

    @Test
    void historicalRevokedGenerationAccepted() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 2);
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        Instant created = Instant.parse("2026-09-11T09:00:00Z");
        Instant revoked = Instant.parse("2026-09-11T11:00:00Z");
        when(agentRepo.findAssignmentByGeneration(tenantId, "agent-1", 2))
                .thenReturn(Optional.of(new AgentSensorAssignment(
                        UUID.randomUUID(), tenantId, "agent-1", "sensor-infra-01", 2L,
                        AgentAssignmentStatus.REVOKED, created, created, revoked
                )));
        stubEntity();
        List<CanonicalObservation> obs = validator.validateAndExtract("{}".getBytes(), batch, scope);
        assertEquals(1, obs.size());
        assertEquals("AGENT_HOST", obs.get(0).sourceProtocol());
        assertEquals("AGENT", obs.get(0).sourceType());
        assertEquals("agent-1", obs.get(0).agentId());
        assertEquals(2L, obs.get(0).assignmentGeneration());
        assertEquals(entityId.toString(), obs.get(0).entityId());
        assertTrue(obs.get(0).sequenceId().contains("agent-1:2:10:0:cpu.util"));
    }

    @Test
    void historicalRevokedGenerationAcceptedExactlyOnActivationBoundary() throws Exception {
        AgentIngressRecord record = sampleRecordWithAcceptedAt(1, "sensor-infra-01", 2, "2026-09-11T10:00:00Z");
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        Instant created = Instant.parse("2026-09-11T10:00:00Z");
        Instant revoked = Instant.parse("2026-09-11T11:00:00Z");
        when(agentRepo.findAssignmentByGeneration(tenantId, "agent-1", 2))
                .thenReturn(Optional.of(new AgentSensorAssignment(
                        UUID.randomUUID(), tenantId, "agent-1", "sensor-infra-01", 2L,
                        AgentAssignmentStatus.REVOKED, created, created, revoked
                )));
        stubEntity();
        List<CanonicalObservation> obs = validator.validateAndExtract("{}".getBytes(), batch, scope);
        assertEquals(1, obs.size());
    }

    @Test
    void historicalRevokedGenerationAcceptedExactlyOnRevocationBoundary() throws Exception {
        AgentIngressRecord record = sampleRecordWithAcceptedAt(1, "sensor-infra-01", 2, "2026-09-11T10:00:00Z");
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        Instant created = Instant.parse("2026-09-11T09:00:00Z");
        Instant revoked = Instant.parse("2026-09-11T10:00:00Z");
        when(agentRepo.findAssignmentByGeneration(tenantId, "agent-1", 2))
                .thenReturn(Optional.of(new AgentSensorAssignment(
                        UUID.randomUUID(), tenantId, "agent-1", "sensor-infra-01", 2L,
                        AgentAssignmentStatus.REVOKED, created, created, revoked
                )));
        stubEntity();
        List<CanonicalObservation> obs = validator.validateAndExtract("{}".getBytes(), batch, scope);
        assertEquals(1, obs.size());
    }

    @Test
    void historicalRevokedGenerationRejectedAcceptedAfterRevocation() throws Exception {
        AgentIngressRecord record = sampleRecordWithAcceptedAt(1, "sensor-infra-01", 2, "2026-09-11T10:00:01Z");
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        Instant created = Instant.parse("2026-09-11T09:00:00Z");
        Instant revoked = Instant.parse("2026-09-11T10:00:00Z");
        when(agentRepo.findAssignmentByGeneration(tenantId, "agent-1", 2))
                .thenReturn(Optional.of(new AgentSensorAssignment(
                        UUID.randomUUID(), tenantId, "agent-1", "sensor-infra-01", 2L,
                        AgentAssignmentStatus.REVOKED, created, created, revoked
                )));
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION", ex.getCode());
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
    }

    @Test
    void historicalRevokedGenerationRejectedAcceptedBeforeActivation() throws Exception {
        AgentIngressRecord record = sampleRecordWithAcceptedAt(1, "sensor-infra-01", 2, "2026-09-11T08:59:59Z");
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        Instant created = Instant.parse("2026-09-11T09:00:00Z");
        Instant revoked = Instant.parse("2026-09-11T10:00:00Z");
        when(agentRepo.findAssignmentByGeneration(tenantId, "agent-1", 2))
                .thenReturn(Optional.of(new AgentSensorAssignment(
                        UUID.randomUUID(), tenantId, "agent-1", "sensor-infra-01", 2L,
                        AgentAssignmentStatus.REVOKED, created, created, revoked
                )));
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_REVOKED_ASSIGNMENT_ACCEPTED_BEFORE_ACTIVATION", ex.getCode());
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
    }

    @Test
    void historicalRevokedGenerationRejectedMissingAcceptanceTime() throws Exception {
        AgentIngressRecord record = sampleRecordWithAcceptedAt(1, "sensor-infra-01", 2, null);
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        Instant created = Instant.parse("2026-09-11T09:00:00Z");
        Instant revoked = Instant.parse("2026-09-11T10:00:00Z");
        when(agentRepo.findAssignmentByGeneration(tenantId, "agent-1", 2))
                .thenReturn(Optional.of(new AgentSensorAssignment(
                        UUID.randomUUID(), tenantId, "agent-1", "sensor-infra-01", 2L,
                        AgentAssignmentStatus.REVOKED, created, created, revoked
                )));
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_REQUIRED", ex.getCode());
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
    }

    @Test
    void historicalRevokedGenerationRejectedMalformedAcceptanceTime() throws Exception {
        AgentIngressRecord record = sampleRecordWithAcceptedAt(1, "sensor-infra-01", 2, "not-a-timestamp");
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        Instant created = Instant.parse("2026-09-11T09:00:00Z");
        Instant revoked = Instant.parse("2026-09-11T10:00:00Z");
        when(agentRepo.findAssignmentByGeneration(tenantId, "agent-1", 2))
                .thenReturn(Optional.of(new AgentSensorAssignment(
                        UUID.randomUUID(), tenantId, "agent-1", "sensor-infra-01", 2L,
                        AgentAssignmentStatus.REVOKED, created, created, revoked
                )));
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_INVALID", ex.getCode());
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
    }

    @Test
    void currentActiveAssignmentAccepted() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 1);
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        stubAssignment(1, "sensor-infra-01");
        stubEntity();
        List<CanonicalObservation> obs = validator.validateAndExtract("{}".getBytes(), batch, scope);
        assertEquals(1, obs.size());
        assertEquals("cpu.util", obs.get(0).metricId());
    }

    @Test
    void entityUnresolvedTerminal() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 1);
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        stubAssignment(1, "sensor-infra-01");
        when(identifierRepo.findByResolutionKey(any(), any(), any(), any())).thenReturn(Optional.empty());
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_AGENT_ENTITY_UNRESOLVED", ex.getCode());
        assertEquals(HttpStatus.UNPROCESSABLE_ENTITY, ex.getStatus());
    }

    @Test
    void tenantScopeMismatchFailClosed() throws Exception {
        UUID otherTenant = UUID.fromString("018f0e0a-0000-7000-8000-000000000099");
        AgentIngressRecord record = new AgentIngressRecord(
                1, "ingress-1", otherTenant.toString(), "agent-1",
                "sensor-infra-01", 1, 10, 10, "agent-batch-1", "corr-1",
                "2026-09-11T10:00:00Z", List.of()
        );
        String checksum = AgentIngestionValidator.calculateStableChecksum(List.of(record));
        AgentTelemetryBatchRequest batch = new AgentTelemetryBatchRequest(
                1, "sensor-infra-01", "batch-1", 1, 1, 1, checksum, "corr", List.of(record)
        );
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_TENANT_SCOPE_MISMATCH", ex.getCode());
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
    }

    @Test
    void entityResolveHappyPath() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 1);
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        stubAssignment(1, "sensor-infra-01");
        stubEntity();
        List<CanonicalObservation> obs = validator.validateAndExtract("{}".getBytes(), batch, scope);
        assertEquals(1, obs.size());
        assertEquals("cpu.util", obs.get(0).metricId());
        assertEquals(42.5, obs.get(0).value());
        assertEquals("sensor-infra-01", obs.get(0).sensorId());
    }
}
