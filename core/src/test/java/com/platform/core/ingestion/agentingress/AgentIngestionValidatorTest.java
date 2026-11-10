package com.platform.core.ingestion.agentingress;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.platform.core.agentcontrol.AgentAssignmentStatus;
import com.platform.core.agentcontrol.AgentControlRepository;
import com.platform.core.agentcontrol.AgentSensorAssignment;
import com.platform.core.agentcontrol.EntityAgentBinding;
import com.platform.core.domain.inventory.EntityIdentifier;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.IngestionMetrics;
import com.platform.core.ingestion.KafkaTelemetryProducer;
import com.platform.core.ingestion.agentingress.dto.AgentIngressRecord;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryAckResponse;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryBatchRequest;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryObservationBatch;
import com.platform.core.ingestion.replay.PublisherClaimResult;
import com.platform.core.ingestion.replay.ReplayReservation;
import com.platform.core.persistence.EntityIdentifierRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;

import java.math.BigDecimal;
import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.List;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

class AgentIngestionValidatorTest {

    private static final String STREAM = "stream-00112233445566778899aabbccddeeff";

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
                2, sensorId, STREAM, "batch-1", record.sensorIngressSequence(), record.sensorIngressSequence(),
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
        when(agentRepo.findActiveEntityBinding(tenantId, "agent-1"))
                .thenReturn(Optional.of(new EntityAgentBinding(tenantId, entityId, "agent-1",
                        "a".repeat(64), "host-abc", "linux", Instant.now(), Instant.now(), "ACTIVE", 1)));
        when(identifierRepo.findByResolutionKey(
                tenantId, "platform.agent", "HOST_ID", "host-abc"
        )).thenReturn(Optional.of(new EntityIdentifier(
                UUID.randomUUID(), tenantId, entityId, "HOST_ID", "platform.agent",
                "host-abc", BigDecimal.ONE, Instant.now(), Instant.now()
        )));
    }

    @Test
    void knownConflictingHostRejectedBeforeReservationAndPublication() throws Exception {
        UUID otherEntity = UUID.fromString("018f0e0a-0000-7000-8000-000000000022");
        when(identifierRepo.findByResolutionKey(tenantId, "platform.agent", "HOST_ID", "host-other"))
                .thenReturn(Optional.of(new EntityIdentifier(UUID.randomUUID(), tenantId, otherEntity,
                        "HOST_ID", "platform.agent", "host-other", BigDecimal.ONE, Instant.now(), Instant.now())));
        when(identifierRepo.hasConflictingHostEvidence(eq(tenantId), eq(entityId), anyList())).thenReturn(true);
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 1);
        ObjectNode payload = record.records().get(0).path("payload").deepCopy();
        payload.put("hostId", "host-other");
        assertRejectedBeforeCoreAck(List.of(withPayload(record, payload)),
                "ERR_AGENT_IDENTITY_CONFLICT", HttpStatus.CONFLICT);
    }

    @Test
    void conflictingObservationCannotHideBehindValidCollectionHost() throws Exception {
        when(identifierRepo.findByResolutionKey(tenantId, "platform.agent", "HOST_ID", "host-other"))
                .thenReturn(Optional.of(new EntityIdentifier(UUID.randomUUID(), tenantId, UUID.randomUUID(),
                        "HOST_ID", "platform.agent", "host-other", BigDecimal.ONE, Instant.now(), Instant.now())));
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 1);
        ObjectNode payload = record.records().get(0).path("payload").deepCopy();
        ((ObjectNode) payload.path("observations").get(0)).put("hostId", "host-other");
        when(identifierRepo.hasConflictingHostEvidence(eq(tenantId), eq(entityId), anyList())).thenReturn(true);
        assertRejectedBeforeCoreAck(List.of(withPayload(record, payload)),
                "ERR_AGENT_IDENTITY_CONFLICT", HttpStatus.CONFLICT);
    }

    @Test
    void unknownHostAndForgedEntityCannotChangeCanonicalBinding() throws Exception {
        stubAssignment(1, "sensor-infra-01");
        stubEntity();
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 1);
        ObjectNode payload = record.records().get(0).path("payload").deepCopy();
        payload.put("hostId", "new-dhcp-host").put("entityId", UUID.randomUUID().toString());
        ((ObjectNode) payload.path("observations").get(0))
                .put("hostId", "new-dhcp-host").put("entityId", UUID.randomUUID().toString());
        var observations = validator.validateAndExtract(new byte[0],
                batchWith(withPayload(record, payload), "sensor-infra-01"), scope);
        assertEquals(entityId.toString(), observations.get(0).entityId());
        verify(agentRepo).findActiveEntityBinding(tenantId, "agent-1");
        verify(agentRepo).findAssignmentByGeneration(tenantId, "agent-1", 1);
        verifyNoMoreInteractions(agentRepo);
    }

    @Test
    void manyDistinctHostClaimsUseOneBoundedQueryNotPerObservationQueries() throws Exception {
        stubAssignment(1, "sensor-infra-01");
        stubEntity();
        var record = sampleRecord(1, "sensor-infra-01", 1);
        ObjectNode payload = record.records().get(0).path("payload").deepCopy();
        var original = payload.path("observations").get(0).deepCopy();
        var items = payload.putArray("observations");
        for (int i = 0; i < 128; i++) {
            items.add(((ObjectNode) original.deepCopy()).put("hostId", "unknown-" + i));
        }
        var output = validator.validateAndExtract(new byte[0],
                batchWith(withPayload(record, payload), "sensor-infra-01"), scope);
        assertEquals(128, output.size());
        verify(identifierRepo).hasConflictingHostEvidence(eq(tenantId), eq(entityId), argThat(ids -> ids.size() == 129));
        verifyNoMoreInteractions(identifierRepo);
    }

    @Test
    void missingActiveBindingFailsClosedEvenWhenHostResolves() throws Exception {
        stubAssignment(1, "sensor-infra-01");
        stubEntity();
        when(agentRepo.findActiveEntityBinding(tenantId, "agent-1")).thenReturn(Optional.empty());
        var batch = batchWith(sampleRecord(1, "sensor-infra-01", 1), "sensor-infra-01");
        var ex = assertThrows(IngestionException.class,
                () -> validator.validateAndExtract(new byte[0], batch, scope));
        assertEquals("ERR_AGENT_ENTITY_BINDING_INACTIVE", ex.getCode());
        verifyNoInteractions(identifierRepo);
    }

    // Deliberately unsupported logical evidence, not an approved BUILD-016 wire schema.
    private JsonNode processNetworkPayload(boolean withHostId) throws Exception {
        JsonNode payload = objectMapper.readTree("""
                {"evidenceType":"ProcessNetworkEvidence","schemaVersion":1,
                 "sourceProvenance":"agent.linux.process_network.ebpf",
                 "bucketStartAt":"2026-09-11T10:00:00Z",
                 "bucketEndAt":"2026-09-11T10:00:30Z",
                 "workload":{"serviceName":"nginx.service"},
                 "remoteIP":"192.0.2.10","remotePort":443,"protocol":"TCP",
                 "bytesTx":1024,"quality":"COMPLETE"}
                """);
        if (withHostId) {
            ((ObjectNode) payload).put("hostId", "host-abc");
        }
        return payload;
    }

    private AgentIngressRecord withPayload(AgentIngressRecord scalar, JsonNode payload) {
        JsonNode delivery = scalar.records().get(0).deepCopy();
        ((ObjectNode) delivery).set("payload", payload);
        return new AgentIngressRecord(
                scalar.sensorIngressSequence(), scalar.sensorIngressRecordId(), scalar.tenantId(), scalar.agentId(),
                scalar.acceptingSensorId(), scalar.assignmentGeneration(), scalar.agentFirstSequence(),
                scalar.agentLastSequence(), scalar.agentBatchId(), scalar.correlationId(), scalar.acceptedAt(),
                List.of(delivery)
        );
    }

    private void assertRejectedBeforeCoreAck(List<AgentIngressRecord> records, String code) throws Exception {
        assertRejectedBeforeCoreAck(records, code, HttpStatus.BAD_REQUEST);
    }

    private void assertRejectedBeforeCoreAck(List<AgentIngressRecord> records, String code,
                                            HttpStatus status) throws Exception {
        stubAssignment(1, "sensor-infra-01");
        stubEntity();
        AgentTelemetryBatchRequest batch = new AgentTelemetryBatchRequest(
                2, "sensor-infra-01", STREAM, "batch-1", 1, records.size(), records.size(),
                AgentIngestionValidator.calculateStableChecksum(records), "corr-batch", records
        );
        SensorAuthorityResolver authority = mock(SensorAuthorityResolver.class);
        X509Certificate cert = mock(X509Certificate.class);
        when(authority.resolve(cert)).thenReturn(scope);
        AgentIngestionValidator realValidator = new AgentIngestionValidator(
                objectMapper, authority, agentRepo, identifierRepo, metrics
        );
        AgentIngressWatermarkStore watermarks = mock(AgentIngressWatermarkStore.class);
        KafkaTelemetryProducer kafka = mock(KafkaTelemetryProducer.class);
        AgentIngestionService service = new AgentIngestionService(realValidator, watermarks, kafka, metrics);
        byte[] rawBody = objectMapper.writeValueAsBytes(batch);
        UUID pendingReservationId = UUID.randomUUID();

        when(watermarks.reserveBatchWithBody(any(), anyString(), anyString(), anyLong(), anyLong(),
                anyString(), anyString(), anyString(), anyString()))
                .thenReturn(ReplayReservation.reserved(0, pendingReservationId));
        when(watermarks.releaseUnpublishedPending(any(), anyString(), anyString(), anyLong(), anyLong(),
                anyString(), anyString(), anyString(), anyString(), eq(pendingReservationId)))
                .thenReturn(true);
        if ("ERR_AGENT_IDENTITY_CONFLICT".equals(code)) {
            UUID dispositionId = UUID.randomUUID();
            when(watermarks.terminallyRejectIdentityConflict(any(), anyString(), anyString(), anyLong(),
                    anyString(), anyString(), anyString(), any(), any(), any()))
                    .thenReturn(ReplayReservation.terminallyRejected(0, 1,
                            dispositionId, code));
            AgentTelemetryAckResponse result = service.ingestAgentTelemetry(
                    rawBody, batch, batch.batchId(), cert);
            assertEquals("TERMINALLY_REJECTED", result.status());
            assertEquals(dispositionId, result.dispositionId());
        } else {
            IngestionException ex = assertThrows(IngestionException.class, () ->
                    service.ingestAgentTelemetry(rawBody, batch, batch.batchId(), cert));
            assertEquals(code, ex.getCode());
            assertEquals(status, ex.getStatus());
            verify(watermarks).releaseUnpublishedPending(any(), anyString(), anyString(), anyLong(),
                    anyLong(), anyString(), anyString(), anyString(), anyString(), eq(pendingReservationId));
        }

        verify(authority).resolve(cert);
        verify(agentRepo, atLeastOnce()).findAssignmentByGeneration(tenantId, "agent-1", 1);
        verify(watermarks).reserveBatchWithBody(any(), anyString(), anyString(), anyLong(), anyLong(),
                anyString(), anyString(), anyString(), anyString());
        verifyNoInteractions(kafka);
    }

    @Test
    void typedProcessNetworkWithoutHostIdRejectedBeforeCoreAck() throws Exception {
        assertRejectedBeforeCoreAck(List.of(withPayload(
                sampleRecord(1, "sensor-infra-01", 1), processNetworkPayload(false)
        )), "ERR_NON_NUMERIC_RECORD");
        verifyNoInteractions(identifierRepo);
    }

    @Test
    void typedProcessNetworkWithResolvedHostButNoObservationsRejectedBeforeCoreAck() throws Exception {
        assertRejectedBeforeCoreAck(List.of(withPayload(
                sampleRecord(1, "sensor-infra-01", 1), processNetworkPayload(true)
        )), "ERR_NON_NUMERIC_RECORD");
        verify(identifierRepo).findByResolutionKey(tenantId, "platform.agent", "HOST_ID", "host-abc");
    }

    @Test
    void typedProcessNetworkWithEmptyObservationsRejectedBeforeCoreAck() throws Exception {
        JsonNode typed = processNetworkPayload(true);
        ((ObjectNode) typed).putArray("observations");
        assertRejectedBeforeCoreAck(List.of(withPayload(
                sampleRecord(1, "sensor-infra-01", 1), typed
        )), "ERR_NON_NUMERIC_RECORD");
    }

    @Test
    void typedObjectCannotMasqueradeAsNumericObservation() throws Exception {
        JsonNode payload = objectMapper.readTree("""
                {"hostId":"host-abc","observations":[{
                 "metricKey":"process.network","observedAt":"2026-09-11T10:00:00Z"}]}
                """);
        ((ObjectNode) payload.path("observations").get(0))
                .set("value", processNetworkPayload(true));
        assertRejectedBeforeCoreAck(List.of(withPayload(
                sampleRecord(1, "sensor-infra-01", 1), payload
        )), "ERR_NON_NUMERIC_RECORD");
    }

    @Test
    void mixedScalarThenTypedIngressRejectsWholeBatchBeforeCoreAck() throws Exception {
        assertRejectedBeforeCoreAck(List.of(
                sampleRecord(1, "sensor-infra-01", 1),
                withPayload(sampleRecord(2, "sensor-infra-01", 1), processNetworkPayload(true))
        ), "ERR_NON_NUMERIC_RECORD");
    }

    @Test
    void mixedTypedThenScalarIngressRejectsWholeBatchBeforeCoreAck() throws Exception {
        assertRejectedBeforeCoreAck(List.of(
                withPayload(sampleRecord(1, "sensor-infra-01", 1), processNetworkPayload(true)),
                sampleRecord(2, "sensor-infra-01", 1)
        ), "ERR_NON_NUMERIC_RECORD");
    }

    @Test
    void mixedScalarAndTypedDeliveriesInsideOneIngressRejectBeforeCoreAck() throws Exception {
        AgentIngressRecord scalar = sampleRecord(1, "sensor-infra-01", 1);
        JsonNode typedDelivery = withPayload(scalar, processNetworkPayload(true)).records().get(0);
        ((ObjectNode) typedDelivery).put("sequence", 11).put("id", "rec-11");
        AgentIngressRecord mixed = new AgentIngressRecord(
                scalar.sensorIngressSequence(), scalar.sensorIngressRecordId(), scalar.tenantId(), scalar.agentId(),
                scalar.acceptingSensorId(), scalar.assignmentGeneration(), 10, 11,
                scalar.agentBatchId(), scalar.correlationId(), scalar.acceptedAt(),
                List.of(scalar.records().get(0), typedDelivery)
        );
        assertRejectedBeforeCoreAck(List.of(mixed), "ERR_NON_NUMERIC_RECORD");
    }

    @Test
    void scalarCollectionWithTypedSiblingCurrentlyAcksOnlyScalarSemantics() throws Exception {
        // Existing extraction ignores unrecognized collection fields. Document this
        // unsafe future-enablement shortcut rather than asserting an invented rejection.
        AgentIngressRecord scalar = sampleRecord(1, "sensor-infra-01", 1);
        JsonNode payload = scalar.records().get(0).path("payload").deepCopy();
        ((ObjectNode) payload).set("processNetworkEvidence", processNetworkPayload(true));
        AgentTelemetryBatchRequest batch = batchWith(withPayload(scalar, payload), "sensor-infra-01");
        stubAssignment(1, "sensor-infra-01");
        stubEntity();
        SensorAuthorityResolver authority = mock(SensorAuthorityResolver.class);
        X509Certificate cert = mock(X509Certificate.class);
        when(authority.resolve(cert)).thenReturn(scope);
        AgentIngestionValidator realValidator = new AgentIngestionValidator(
                objectMapper, authority, agentRepo, identifierRepo, metrics
        );
        AgentIngressWatermarkStore watermarks = mock(AgentIngressWatermarkStore.class);
        KafkaTelemetryProducer kafka = mock(KafkaTelemetryProducer.class);
        UUID owner = UUID.fromString("018f0e0a-0000-7000-8000-00000000beef");
        when(watermarks.reserveBatchWithBody(any(), anyString(), anyString(), anyLong(), anyLong(),
                anyString(), anyString(), anyString(), anyString()))
                .thenReturn(ReplayReservation.reserved(0, UUID.randomUUID()));
        when(watermarks.checkpointAuthority(any(), anyString(), anyString(), anyLong(), anyLong(),
                anyString(), anyString(), anyString(), anyString(), any(), anyList())).thenReturn(true);
        when(watermarks.claimPublisherLeaseExact(any(), anyString(), anyString(), anyLong(), anyLong(),
                anyString(), anyString(), anyString(), anyString(), any()))
                .thenReturn(PublisherClaimResult.claimed(owner, 1L));
        AgentIngestionService service = new AgentIngestionService(realValidator, watermarks, kafka, metrics);

        var ack = service.ingestAgentTelemetry(objectMapper.writeValueAsBytes(batch), batch, batch.batchId(), cert);
        assertEquals("ACCEPTED", ack.status());
        assertEquals(1L, ack.acceptedThroughIngressSequence());
        var published = org.mockito.ArgumentCaptor.forClass(TelemetryObservationBatch.class);
        verify(kafka).publishBatchTransactionally(published.capture());
        assertEquals(1, published.getValue().numericObservations().size());
        assertTrue(published.getValue().counterObservations().isEmpty());
        assertEquals("cpu.util", published.getValue().numericObservations().get(0).metricId());
        assertEquals(42.5, published.getValue().numericObservations().get(0).value());
        verify(watermarks).finalizeReservedBatch(tenantId, "sensor-infra-01", STREAM, 1L, 1L,
                batch.checksum(), batch.batchId(), batch.correlationId(), owner, 1L);
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
                2, "sensor-infra-01", STREAM, "batch-1", 1, 1, 1, checksum, "corr", List.of(record)
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
    void legacyRevokedAssignmentWithinAcceptanceIntervalCheckpointsOriginalEntity() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 2);
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        Instant created = Instant.parse("2026-09-11T09:00:00Z");
        Instant revoked = Instant.parse("2026-09-11T11:00:00Z");
        when(agentRepo.findAssignmentByGeneration(tenantId, "agent-1", 2))
                .thenReturn(Optional.of(new AgentSensorAssignment(UUID.randomUUID(), tenantId,
                        "agent-1", "sensor-infra-01", 2, AgentAssignmentStatus.REVOKED,
                        created, created, revoked)));
        when(agentRepo.findActiveEntityBinding(tenantId, "agent-1"))
                .thenReturn(Optional.of(new EntityAgentBinding(tenantId, entityId, "agent-1",
                        "a".repeat(64), "host-abc", "linux", created, revoked, "ACTIVE", 1)));
        AgentIngestionValidator.ValidatedBatch validated = validator.validateLegacyAndSnapshot(batch, scope);
        assertEquals(List.of(entityId), validated.entityIds());
        assertEquals(1, validated.observations().size());
        assertEquals(entityId.toString(), validated.observations().get(0).entityId());
    }

    @Test
    void legacyPendingWithoutHistoricalEntityProofRequiresReconciliation() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 2);
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        Instant created = Instant.parse("2026-09-11T09:00:00Z");
        Instant revoked = Instant.parse("2026-09-11T11:00:00Z");
        when(agentRepo.findAssignmentByGeneration(tenantId, "agent-1", 2))
                .thenReturn(Optional.of(new AgentSensorAssignment(UUID.randomUUID(), tenantId,
                        "agent-1", "sensor-infra-01", 2, AgentAssignmentStatus.REVOKED,
                        created, created, revoked)));
        when(agentRepo.findActiveEntityBinding(tenantId, "agent-1"))
                .thenReturn(Optional.of(new EntityAgentBinding(tenantId, entityId, "agent-1",
                        "a".repeat(64), "host-abc", "linux", revoked, revoked, "ACTIVE", 1)));
        IngestionException failure = assertThrows(IngestionException.class,
                () -> validator.validateLegacyAndSnapshot(batch, scope));
        assertEquals("ERR_AGENT_LEGACY_ENTITY_UNPROVEN", failure.getCode());
    }

    @Test
    void validatedSnapshotSurvivesCurrentBindingRevocationAndRetainsLogicalIdentity() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 2);
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        List<CanonicalObservation> replay = validator.extractValidatedSnapshot(batch, scope, List.of(entityId));
        assertEquals(1, replay.size());
        assertEquals(entityId.toString(), replay.get(0).entityId());
        assertTrue(replay.get(0).sequenceId().contains("agent-1:2:10:0:cpu.util"));
        verifyNoInteractions(agentRepo);
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
    void missingBindingCannotFallBackToHostResolution() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 1);
        AgentTelemetryBatchRequest batch = batchWith(record, "sensor-infra-01");
        stubAssignment(1, "sensor-infra-01");
        when(identifierRepo.findByResolutionKey(any(), any(), any(), any())).thenReturn(Optional.empty());
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_AGENT_ENTITY_BINDING_INACTIVE", ex.getCode());
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
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
                2, "sensor-infra-01", STREAM, "batch-1", 1, 1, 1, checksum, "corr", List.of(record)
        );
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_TENANT_SCOPE_MISMATCH", ex.getCode());
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
    }

    @Test
    void unsupportedSchemaVersionRejected() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 1);
        String checksum = AgentIngestionValidator.calculateStableChecksum(List.of(record));
        AgentTelemetryBatchRequest batch = new AgentTelemetryBatchRequest(
                1, "sensor-infra-01", STREAM, "batch-1", 1, 1, 1, checksum, "corr", List.of(record)
        );
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_UNSUPPORTED_SCHEMA_VERSION", ex.getCode());
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
    }

    @Test
    void invalidAgentStreamIdRejected() throws Exception {
        AgentIngressRecord record = sampleRecord(1, "sensor-infra-01", 1);
        String checksum = AgentIngestionValidator.calculateStableChecksum(List.of(record));
        AgentTelemetryBatchRequest batch = new AgentTelemetryBatchRequest(
                2, "sensor-infra-01", "not-a-stream", "batch-1", 1, 1, 1, checksum, "corr", List.of(record)
        );
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(), batch, scope));
        assertEquals("ERR_AGENT_STREAM_ID_INVALID", ex.getCode());
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
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
