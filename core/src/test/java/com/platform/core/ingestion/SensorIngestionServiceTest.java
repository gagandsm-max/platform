package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.authority.TestSensorAuthorityResolver;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import com.platform.core.ingestion.replay.TelemetryWatermarkStore;
import com.platform.core.ingestion.replay.ReplayReservation;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.List;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

class SensorIngestionServiceTest {

    private IngestionValidator validator;
    private SensorAuthorityResolver authorityResolver;
    private TelemetryWatermarkStore watermarkStore;
    private KafkaTelemetryProducer kafkaProducer;
    private IngestionMetrics metrics;
    private SensorIngestionService service;

    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private final UUID entityId = UUID.fromString("018f0e0a-0000-7000-8000-000000000011");
    private final SensorScope testScope = new SensorScope("sensor-net-01", tenantId, null, Set.of(entityId), true);

    @BeforeEach
    void setUp() {
        validator = mock(IngestionValidator.class);
        authorityResolver = mock(SensorAuthorityResolver.class);
        watermarkStore = mock(TelemetryWatermarkStore.class);
        kafkaProducer = mock(KafkaTelemetryProducer.class);
        metrics = new IngestionMetrics();
        service = new SensorIngestionService(validator, authorityResolver, watermarkStore, kafkaProducer, metrics);
        when(watermarkStore.reserveBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString()))
                .thenReturn(ReplayReservation.reserved(0));
    }

    private X509Certificate createMockCert(String cn) {
        X509Certificate cert = mock(X509Certificate.class);
        when(cert.getSubjectX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal("CN=" + cn));
        return cert;
    }

    @Test
    void testClaimedSensorIdMismatchingCertCnDoesNotRejectInService() {
        // CN is not authority: mismatched CN must not produce ERR_AUTH_IDENTITY_MISMATCH in the service.
        X509Certificate cert = createMockCert("totally-different-cn");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-1", 100, 105, "sum-123", List.of());
        byte[] rawBody = "{}".getBytes();

        List<CanonicalObservation> observations = List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "105", "snmp")
        );
        when(validator.resolveAuthority(cert)).thenReturn(testScope);
        when(validator.validateAndExtract(rawBody, req, testScope)).thenReturn(observations);

        TelemetryAckResponse ack = service.ingestTelemetry(rawBody, req, "b-1", cert);
        assertEquals("ACCEPTED", ack.status());
        assertEquals("sensor-net-01", ack.sensorIdentity());
        verify(validator).resolveAuthority(cert);
    }

    @Test
    void testSuccessfulIngestionPublishesAtomicallyAndUpdatesDurableWatermark() {
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-1", 100, 105, "sum-123", List.of());
        byte[] rawBody = "{}".getBytes();

        List<CanonicalObservation> observations = List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "105", "snmp")
        );
        when(validator.resolveAuthority(cert)).thenReturn(testScope);
        when(validator.validateAndExtract(rawBody, req, testScope)).thenReturn(observations);

        TelemetryAckResponse ack = service.ingestTelemetry(rawBody, req, "b-1", cert);

        assertNotNull(ack);
        assertEquals("ACCEPTED", ack.status());
        assertEquals(105L, ack.acceptedThroughSequence());
        assertEquals("sensor-net-01", ack.sensorIdentity());

        verify(kafkaProducer, times(1)).publishBatchTransactionally(observations);
        verify(watermarkStore, times(1)).reserveBatch(
                eq(tenantId), eq("sensor-net-01"), eq("epoch-1"), eq(100L), eq(105L), eq("sum-123"), eq("b-1")
        );
        verify(watermarkStore, times(1)).finalizeReservedBatch(
                eq(tenantId), eq("sensor-net-01"), eq("epoch-1"), eq(100L), eq(105L), eq("sum-123"), eq("b-1")
        );
    }

    @Test
    void testExactRetryReturnsCachedWatermarkWithoutRepublishing() {
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-1", 100, 105, "sum-123", List.of());
        byte[] rawBody = "{}".getBytes();

        List<CanonicalObservation> observations = List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "105", "snmp")
        );
        when(validator.resolveAuthority(cert)).thenReturn(testScope);
        when(validator.validateAndExtract(rawBody, req, testScope)).thenReturn(observations);

        when(watermarkStore.reserveBatch(
                tenantId, "sensor-net-01", "epoch-1", 100L, 105L, "sum-123", "b-1"))
                .thenReturn(ReplayReservation.alreadyAcked(105L));

        TelemetryAckResponse ack = service.ingestTelemetry(rawBody, req, "b-1", cert);

        assertNotNull(ack);
        assertEquals("ACCEPTED", ack.status());
        assertEquals(105L, ack.acceptedThroughSequence());
        assertEquals("sensor-net-01", ack.sensorIdentity());

        // Exact accepted replay does not republish and does not finalize again.
        verify(kafkaProducer, never()).publishBatchTransactionally(anyList());
        verify(watermarkStore, never()).finalizeReservedBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString());
    }

    @Test
    void testConflictingRetryRejectsWithConflict() {
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-1", 100, 105, "sum-DIFFERENT", List.of());
        byte[] rawBody = "{}".getBytes();

        List<CanonicalObservation> observations = List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "105", "snmp")
        );
        when(validator.resolveAuthority(cert)).thenReturn(testScope);
        when(validator.validateAndExtract(rawBody, req, testScope)).thenReturn(observations);

        when(watermarkStore.reserveBatch(
                tenantId, "sensor-net-01", "epoch-1", 100L, 105L, "sum-DIFFERENT", "b-1"))
                .thenReturn(ReplayReservation.rejected(
                        105L, "ERR_IDEMPOTENCY_CONFLICT",
                        "sequence range was accepted/reserved with a different checksum"));

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestTelemetry(rawBody, req, "b-1", cert));
        assertEquals(HttpStatus.CONFLICT, ex.getStatus());
        assertEquals("ERR_IDEMPOTENCY_CONFLICT", ex.getCode());

        verify(kafkaProducer, never()).publishBatchTransactionally(anyList());
    }

    @Test
    void testSequenceRegressionRejects() {
        X509Certificate cert = createMockCert("sensor-net-01");
        // Regressed batch: firstSequence 95 is <= watermark 105
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-old", 95, 99, "sum-old", List.of());
        byte[] rawBody = "{}".getBytes();

        List<CanonicalObservation> observations = List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "99", "snmp")
        );
        when(validator.resolveAuthority(cert)).thenReturn(testScope);
        when(validator.validateAndExtract(rawBody, req, testScope)).thenReturn(observations);

        when(watermarkStore.reserveBatch(
                tenantId, "sensor-net-01", "epoch-1", 95L, 99L, "sum-old", "b-old"))
                .thenReturn(ReplayReservation.rejected(
                        105L, "ERR_SEQUENCE_REGRESSION", "sequence already accepted"));

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestTelemetry(rawBody, req, "b-old", cert));
        assertEquals(HttpStatus.CONFLICT, ex.getStatus());
        assertEquals("ERR_SEQUENCE_REGRESSION", ex.getCode());

        verify(kafkaProducer, never()).publishBatchTransactionally(anyList());
    }

    @Test
    void testSequenceGapRejects() {
        X509Certificate cert = createMockCert("sensor-net-01");
        // Gapped batch: watermark is 105, but incoming firstSequence is 120 (expected 106)
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-gap", 120, 125, "sum-gap", List.of());
        byte[] rawBody = "{}".getBytes();

        List<CanonicalObservation> observations = List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "125", "snmp")
        );
        when(validator.resolveAuthority(cert)).thenReturn(testScope);
        when(validator.validateAndExtract(rawBody, req, testScope)).thenReturn(observations);

        when(watermarkStore.reserveBatch(
                tenantId, "sensor-net-01", "epoch-1", 120L, 125L, "sum-gap", "b-gap"))
                .thenReturn(ReplayReservation.rejected(
                        105L, "ERR_SEQUENCE_GAP", "unexpected sequence gap"));

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestTelemetry(rawBody, req, "b-gap", cert));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_SEQUENCE_GAP", ex.getCode());

        verify(kafkaProducer, never()).publishBatchTransactionally(anyList());
    }

    @Test
    void testUnexpectedDurableStreamChangeRejectsBeforeKafka() {
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(
                1, "sensor-net-01", "stream-new", "b-stream-change",
                1, 1, "sum-stream-change", List.of());
        byte[] rawBody = "{}".getBytes();

        List<CanonicalObservation> observations = List.of(
                new CanonicalObservation(
                        tenantId.toString(), entityId.toString(), "cpu.util",
                        "sensor-net-01", Instant.now(), 42.0, 1, 0,
                        "stream-new:1", "snmp")
        );
        when(validator.resolveAuthority(cert)).thenReturn(testScope);
        when(validator.validateAndExtract(rawBody, req, testScope)).thenReturn(observations);
        when(watermarkStore.reserveBatch(
                tenantId, "sensor-net-01", "stream-new",
                1L, 1L, "sum-stream-change", "b-stream-change"))
                .thenReturn(ReplayReservation.rejected(
                        105L, "ERR_SENSOR_STREAM_CHANGED",
                        "explicit continuity reset/re-enrollment is required"));

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestTelemetry(rawBody, req, "b-stream-change", cert));

        assertEquals(HttpStatus.CONFLICT, ex.getStatus());
        assertEquals("ERR_SENSOR_STREAM_CHANGED", ex.getCode());
        verify(kafkaProducer, never()).publishBatchTransactionally(anyList());
        verify(watermarkStore, never()).finalizeReservedBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString());
    }

    @Test
    void testKafkaFailurePreventsWatermarkUpdateAndThrows() {
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-2", 1, 1, "sum-fail", List.of());
        byte[] rawBody = "{}".getBytes();

        List<CanonicalObservation> observations = List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "1", "snmp")
        );
        when(validator.resolveAuthority(cert)).thenReturn(testScope);
        when(validator.validateAndExtract(rawBody, req, testScope)).thenReturn(observations);

        doThrow(new IngestionException(HttpStatus.SERVICE_UNAVAILABLE, "ERR_KAFKA_PUBLICATION_FAILED", "Kafka down"))
                .when(kafkaProducer).publishBatchTransactionally(observations);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestTelemetry(rawBody, req, "b-2", cert));
        assertEquals(HttpStatus.SERVICE_UNAVAILABLE, ex.getStatus());
        assertEquals("ERR_KAFKA_PUBLICATION_FAILED", ex.getCode());

        // Reservation remains pending for exact retry; ACK watermark MUST NOT finalize.
        verify(watermarkStore, never()).finalizeReservedBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString());
    }

    @Test
    void testKafkaCommitSucceedsButPostgresWatermarkThrowsNoAckAndRetryRepublishesCleanly() {
        // Fault injection: the durable reservation is committed, Kafka publication succeeds,
        // but PostgreSQL finalize fails before ACK. The exact retry sees RETRY_PENDING,
        // republishes the same stable observation identity, then finalizes successfully.
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-fault", 100, 105, "sum-fault", List.of());
        byte[] rawBody = "{}".getBytes();

        List<CanonicalObservation> observations = List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "105", "snmp")
        );
        when(validator.resolveAuthority(cert)).thenReturn(testScope);
        when(validator.validateAndExtract(rawBody, req, testScope)).thenReturn(observations);

        when(watermarkStore.reserveBatch(
                tenantId, "sensor-net-01", "epoch-1", 100L, 105L, "sum-fault", "b-fault"))
                .thenReturn(ReplayReservation.reserved(99L))
                .thenReturn(ReplayReservation.retryPending(99L));

        doThrow(new RuntimeException("PostgreSQL connection terminated unexpectedly"))
                .doNothing()
                .when(watermarkStore).finalizeReservedBatch(
                        eq(tenantId), eq("sensor-net-01"), eq("epoch-1"), eq(100L), eq(105L),
                        eq("sum-fault"), eq("b-fault"));

        // Ingestion call 1 fails -> NO ACK to Sensor!
        RuntimeException ex = assertThrows(RuntimeException.class, () ->
                service.ingestTelemetry(rawBody, req, "b-fault", cert));
        assertTrue(ex.getMessage().contains("PostgreSQL connection terminated"));

        // Attempt 2 (Sensor retry of same batch):
        TelemetryAckResponse ack = service.ingestTelemetry(rawBody, req, "b-fault", cert);
        assertNotNull(ack);
        assertEquals("ACCEPTED", ack.status());
        assertEquals(105L, ack.acceptedThroughSequence());

        // Kafka published twice (at-least-once replay with identical deterministic observation IDs)
        verify(kafkaProducer, times(2)).publishBatchTransactionally(observations);
        verify(watermarkStore, times(2)).reserveBatch(
                eq(tenantId), eq("sensor-net-01"), eq("epoch-1"), eq(100L), eq(105L), eq("sum-fault"), eq("b-fault")
        );
        verify(watermarkStore, times(2)).finalizeReservedBatch(
                eq(tenantId), eq("sensor-net-01"), eq("epoch-1"), eq(100L), eq(105L), eq("sum-fault"), eq("b-fault")
        );
    }

    @Test
    void testRejectedMixedContentBatchNoKafkaPublicationAndNoWatermarkUpdate() throws Exception {
        // MANDATORY P0 TEST 8 & 9:
        // Use a REAL IngestionValidator with real authority resolver to prove that a rejected
        // mixed-content batch causes ZERO Kafka publication and ZERO PostgreSQL watermark update.
        // Do not mock away the validator result.
        TestSensorAuthorityResolver realResolver = new TestSensorAuthorityResolver();
        realResolver.registerScope("sensor-net-01", testScope);
        IngestionValidator realValidator = new IngestionValidator(new ObjectMapper().findAndRegisterModules(), realResolver, metrics);
        SensorIngestionService serviceWithRealValidator = new SensorIngestionService(
                realValidator, realResolver, watermarkStore, kafkaProducer, metrics
        );

        X509Certificate cert = createMockCert("sensor-net-01");

        // Mixed observation record: 1 valid numeric observation, 1 non-numeric text observation
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\"," +
                "\"entityId\":\"" + entityId + "\"," +
                "\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\"," +
                "\"protocolPayload\":{\"observations\":[" +
                "{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":12345}}," +
                "{\"oid\":\".1.3.6.1.2.1.1.1.0\",\"status\":\"VALUE\",\"value\":{\"text\":\"Linux Router\"}}" +
                "]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-mixed\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = new ObjectMapper().readValue(json, TelemetryBatchRequest.class);
        byte[] rawBody = json.getBytes(StandardCharsets.UTF_8);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                serviceWithRealValidator.ingestTelemetry(rawBody, req, "b-mixed", cert));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_NON_NUMERIC_RECORD", ex.getCode());

        // Prove 8: No Kafka publication
        verify(kafkaProducer, never()).publishBatchTransactionally(any());
        // Prove 9: invalid source data never reaches durable replay reservation/finalization.
        verify(watermarkStore, never()).reserveBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString());
        verify(watermarkStore, never()).finalizeReservedBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString());
    }
}
