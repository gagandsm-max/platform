package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.authority.TestSensorAuthorityResolver;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryObservationBatch;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import com.platform.core.ingestion.replay.PublisherClaimResult;
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
    private final UUID ownerToken = UUID.fromString("018f0e0a-0000-7000-8000-00000000cafe");
    private static final long PUBLISHER_EPOCH = 1L;

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
        when(watermarkStore.claimPublisherLease(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString()))
                .thenReturn(PublisherClaimResult.claimed(ownerToken, PUBLISHER_EPOCH));
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

        TelemetryObservationBatch observations = TelemetryObservationBatch.ofNumeric(List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "105", "snmp")
        ));
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

        TelemetryObservationBatch observations = TelemetryObservationBatch.ofNumeric(List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "105", "snmp")
        ));
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
        verify(watermarkStore, times(1)).claimPublisherLease(
                eq(tenantId), eq("sensor-net-01"), eq("epoch-1"), eq(100L), eq(105L), eq("sum-123"), eq("b-1")
        );
        verify(watermarkStore, times(1)).finalizeReservedBatch(
                eq(tenantId), eq("sensor-net-01"), eq("epoch-1"), eq(100L), eq(105L), eq("sum-123"), eq("b-1"),
                eq(ownerToken), eq(PUBLISHER_EPOCH)
        );
        verify(watermarkStore, never()).releasePublisherLease(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), any(), anyLong());
    }

    @Test
    void testIdempotencyKeyMismatchRejectsBeforeReserve() {
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-1", 100, 105, "sum-123", List.of());
        when(validator.resolveAuthority(cert)).thenReturn(testScope);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestTelemetry("{}".getBytes(), req, "other-key", cert));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_IDEMPOTENCY_KEY_MISMATCH", ex.getCode());

        verify(watermarkStore, never()).reserveBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString());
        verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
    }

    @Test
    void testExactRetryReturnsCachedWatermarkWithoutRepublishing() {
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-1", 100, 105, "sum-123", List.of());
        byte[] rawBody = "{}".getBytes();

        TelemetryObservationBatch observations = TelemetryObservationBatch.ofNumeric(List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "105", "snmp")
        ));
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

        // Exact accepted replay does not claim, republish, or finalize again.
        verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        verify(watermarkStore, never()).claimPublisherLease(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString());
        verify(watermarkStore, never()).finalizeReservedBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), any(), anyLong());
    }

    @Test
    void testConflictingRetryRejectsWithConflict() {
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-1", 100, 105, "sum-DIFFERENT", List.of());
        byte[] rawBody = "{}".getBytes();

        TelemetryObservationBatch observations = TelemetryObservationBatch.ofNumeric(List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "105", "snmp")
        ));
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

        verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        verify(watermarkStore, never()).claimPublisherLease(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString());
    }

    @Test
    void testSequenceRegressionRejects() {
        X509Certificate cert = createMockCert("sensor-net-01");
        // Regressed batch: firstSequence 95 is <= watermark 105
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-old", 95, 99, "sum-old", List.of());
        byte[] rawBody = "{}".getBytes();

        TelemetryObservationBatch observations = TelemetryObservationBatch.ofNumeric(List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "99", "snmp")
        ));
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

        verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
    }

    @Test
    void testSequenceGapRejects() {
        X509Certificate cert = createMockCert("sensor-net-01");
        // Gapped batch: watermark is 105, but incoming firstSequence is 120 (expected 106)
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-gap", 120, 125, "sum-gap", List.of());
        byte[] rawBody = "{}".getBytes();

        TelemetryObservationBatch observations = TelemetryObservationBatch.ofNumeric(List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "125", "snmp")
        ));
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

        verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
    }

    @Test
    void testUnexpectedDurableStreamChangeRejectsBeforeKafka() {
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(
                1, "sensor-net-01", "stream-new", "b-stream-change",
                1, 1, "sum-stream-change", List.of());
        byte[] rawBody = "{}".getBytes();

        TelemetryObservationBatch observations = TelemetryObservationBatch.ofNumeric(List.of(
                new CanonicalObservation(
                        tenantId.toString(), entityId.toString(), "cpu.util",
                        "sensor-net-01", Instant.now(), 42.0, 1, 0,
                        "stream-new:1", "snmp")
        ));
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
        verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        verify(watermarkStore, never()).finalizeReservedBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), any(), anyLong());
    }

    @Test
    void testPublisherLeaseBusyRejectsWithoutKafka() {
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-busy", 1, 1, "sum-busy", List.of());
        byte[] rawBody = "{}".getBytes();
        TelemetryObservationBatch observations = TelemetryObservationBatch.ofNumeric(List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "1", "snmp")
        ));
        when(validator.resolveAuthority(cert)).thenReturn(testScope);
        when(validator.validateAndExtract(rawBody, req, testScope)).thenReturn(observations);
        when(watermarkStore.claimPublisherLease(
                tenantId, "sensor-net-01", "epoch-1", 1L, 1L, "sum-busy", "b-busy"))
                .thenReturn(PublisherClaimResult.busy());

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestTelemetry(rawBody, req, "b-busy", cert));
        assertEquals(HttpStatus.TOO_MANY_REQUESTS, ex.getStatus());
        assertEquals("ERR_BATCH_PUBLICATION_IN_PROGRESS", ex.getCode());
        verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        verify(watermarkStore, never()).finalizeReservedBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), any(), anyLong());
    }

    @Test
    void testKafkaFailurePreventsWatermarkUpdateAndThrows() {
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-2", 1, 1, "sum-fail", List.of());
        byte[] rawBody = "{}".getBytes();

        TelemetryObservationBatch observations = TelemetryObservationBatch.ofNumeric(List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "1", "snmp")
        ));
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
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), any(), anyLong());
        verify(watermarkStore, times(1)).releasePublisherLease(
                eq(tenantId), eq("sensor-net-01"), eq("epoch-1"), eq(1L), eq(1L), eq("sum-fail"), eq("b-2"),
                eq(ownerToken), eq(PUBLISHER_EPOCH)
        );
    }

    @Test
    void testKafkaCommitSucceedsButPostgresWatermarkThrowsNoAckAndRetryRepublishesCleanly() {
        // Fault injection: the durable reservation is committed, Kafka publication succeeds,
        // but PostgreSQL finalize fails before ACK. The exact retry sees RETRY_PENDING,
        // republishes the same stable observation identity, then finalizes successfully.
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-fault", 100, 105, "sum-fault", List.of());
        byte[] rawBody = "{}".getBytes();

        TelemetryObservationBatch observations = TelemetryObservationBatch.ofNumeric(List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "105", "snmp")
        ));
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
                        eq("sum-fault"), eq("b-fault"), eq(ownerToken), eq(PUBLISHER_EPOCH));

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
        verify(watermarkStore, times(2)).claimPublisherLease(
                eq(tenantId), eq("sensor-net-01"), eq("epoch-1"), eq(100L), eq(105L), eq("sum-fault"), eq("b-fault")
        );
        verify(watermarkStore, times(2)).finalizeReservedBatch(
                eq(tenantId), eq("sensor-net-01"), eq("epoch-1"), eq(100L), eq(105L), eq("sum-fault"), eq("b-fault"),
                eq(ownerToken), eq(PUBLISHER_EPOCH)
        );
        verify(watermarkStore, times(1)).releasePublisherLease(
                eq(tenantId), eq("sensor-net-01"), eq("epoch-1"), eq(100L), eq(105L), eq("sum-fault"), eq("b-fault"),
                eq(ownerToken), eq(PUBLISHER_EPOCH)
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
        verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        // Prove 9: invalid source data never reaches durable replay reservation/finalization.
        verify(watermarkStore, never()).reserveBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString());
        verify(watermarkStore, never()).claimPublisherLease(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString());
        verify(watermarkStore, never()).finalizeReservedBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), any(), anyLong());
    }

    // =========================================================================
    // ISSUE #218 — BUILD-020 icmp_observation_v1 CHARACTERIZATION (service path)
    //
    // CHARACTERIZATION ONLY of current numeric-only Core limitation. Validation
    // fails before reserve/finalize/Kafka/ACK. When typed ICMP ingestion is
    // approved and implemented, replace these expected-rejection tests with
    // durable typed-acceptance + complete-accounting coverage.
    // =========================================================================

    private SensorIngestionService serviceWithRealValidator() {
        TestSensorAuthorityResolver realResolver = new TestSensorAuthorityResolver();
        realResolver.registerScope("sensor-net-01", testScope);
        IngestionValidator realValidator = new IngestionValidator(
                new ObjectMapper().findAndRegisterModules(), realResolver, metrics);
        return new SensorIngestionService(realValidator, realResolver, watermarkStore, kafkaProducer, metrics);
    }

    /**
     * Source-faithful BUILD-020 Envelope+Evidence as produced by
     * sensor/internal/integration/icmp/executor.go (icmp_observation_v1).
     */
    private String build020IcmpRecordJson(long sequence) {
        String envelope = "{"
                + "\"schemaVersion\":1,"
                + "\"batchId\":\"icmp-aabbccddeeff00112233445566778899\","
                + "\"recordType\":\"icmp_observation_v1\","
                + "\"collectedAt\":\"2026-10-11T14:00:00Z\","
                + "\"sensorId\":\"sensor-net-01\","
                + "\"tenantId\":\"" + tenantId + "\","
                + "\"entityId\":\"" + entityId + "\","
                + "\"job\":{\"jobId\":\"icmp-job-1\",\"assignmentKey\":\"icmp-assign-1\",\"assignmentVersion\":1,\"attempt\":1},"
                + "\"protocol\":\"ICMP\","
                + "\"protocolVersion\":\"4\","
                + "\"operation\":\"ECHO\","
                + "\"status\":\"COLLECTED\","
                + "\"quality\":\"GOOD\","
                + "\"protocolSchemaVersion\":1,"
                + "\"protocolPayload\":{"
                + "\"schemaVersion\":1,"
                + "\"target\":\"127.0.0.1\","
                + "\"ipVersion\":4,"
                + "\"source\":\"SENSOR_PING_SOCKET\","
                + "\"startedAt\":\"2026-10-11T13:59:59.900Z\","
                + "\"completedAt\":\"2026-10-11T14:00:00Z\","
                + "\"requestedSamples\":2,"
                + "\"received\":2,"
                + "\"packetLossPct\":0.0,"
                + "\"minRttMicros\":120,"
                + "\"avgRttMicros\":150,"
                + "\"maxRttMicros\":180,"
                + "\"samples\":["
                + "{\"sequence\":1,\"success\":true,\"rttMicros\":120},"
                + "{\"sequence\":2,\"success\":true,\"rttMicros\":180}"
                + "]"
                + "}"
                + "}";
        return "{\"sequence\":" + sequence + ",\"type\":\"icmp_observation_v1\",\"payload\":" + envelope + "}";
    }

    private String snmpNumericRecordJson(long sequence, long unsignedValue) {
        return "{\"sequence\":" + sequence + ",\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{"
                + "\"tenantId\":\"" + tenantId + "\","
                + "\"entityId\":\"" + entityId + "\","
                + "\"collectedAt\":\"2026-10-11T14:00:00Z\","
                + "\"protocol\":\"SNMP\","
                + "\"quality\":\"GOOD\","
                + "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":"
                + unsignedValue + "}}]}}}";
    }

    private String sha256Of(String... recordJsons) throws Exception {
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        for (String recordJson : recordJsons) {
            digest.update(recordJson.getBytes(StandardCharsets.UTF_8));
        }
        StringBuilder sb = new StringBuilder();
        for (byte b : digest.digest()) {
            sb.append(String.format("%02x", b));
        }
        return sb.toString();
    }

    private void assertNoDurableSideEffects() {
        // MANDATORY #4/#5/#6: no durable ACK path, no PG reserve/finalize, no Kafka.
        verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        verify(watermarkStore, never()).reserveBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString());
        verify(watermarkStore, never()).claimPublisherLease(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString());
        verify(watermarkStore, never()).finalizeReservedBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), any(), anyLong());
        verify(watermarkStore, never()).releasePublisherLease(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), any(), anyLong());
    }

    @Test
    void issue218_sourceFaithfulIcmpBatchRejectedWithNoAckNoPgAdvanceNoKafka() throws Exception {
        SensorIngestionService realService = serviceWithRealValidator();
        X509Certificate cert = createMockCert("sensor-net-01");

        String icmp = build020IcmpRecordJson(1);
        String checksum = sha256Of(icmp);
        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\","
                + "\"batchId\":\"b-icmp-svc\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + checksum + "\",\"records\":[" + icmp + "]}";
        TelemetryBatchRequest req = new ObjectMapper().readValue(json, TelemetryBatchRequest.class);
        byte[] rawBody = json.getBytes(StandardCharsets.UTF_8);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                realService.ingestTelemetry(rawBody, req, "b-icmp-svc", cert));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_UNSUPPORTED_RECORD_TYPE", ex.getCode(),
                "CHARACTERIZATION: no Core durable ACK for BUILD-020 icmp_observation_v1 under numeric-only Core");
        assertNoDurableSideEffects();
    }

    @Test
    void issue218_mixedAndReverseOrderIcmpBatchesRejectWithoutPartialPublishOrDurableAdvance() throws Exception {
        SensorIngestionService realService = serviceWithRealValidator();
        X509Certificate cert = createMockCert("sensor-net-01");
        ObjectMapper mapper = new ObjectMapper();

        // Mixed: SNMP numeric sibling first, ICMP second — whole batch rejected.
        String snmpFirst = snmpNumericRecordJson(1, 12345);
        String icmpSecond = build020IcmpRecordJson(2);
        String mixedChecksum = sha256Of(snmpFirst, icmpSecond);
        String mixedJson = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\","
                + "\"batchId\":\"b-icmp-mixed\",\"firstSequence\":1,\"lastSequence\":2,\"checksum\":\""
                + mixedChecksum + "\",\"records\":[" + snmpFirst + "," + icmpSecond + "]}";
        TelemetryBatchRequest mixedReq = mapper.readValue(mixedJson, TelemetryBatchRequest.class);

        IngestionException mixedEx = assertThrows(IngestionException.class, () ->
                realService.ingestTelemetry(mixedJson.getBytes(StandardCharsets.UTF_8), mixedReq, "b-icmp-mixed", cert));
        assertEquals("ERR_UNSUPPORTED_RECORD_TYPE", mixedEx.getCode());

        // Reverse sequence order on the wire (ICMP seq=1 after SNMP seq=2 in array).
        String icmpFirst = build020IcmpRecordJson(1);
        String snmpSecond = snmpNumericRecordJson(2, 67890);
        String reverseChecksum = sha256Of(snmpSecond, icmpFirst);
        String reverseJson = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\","
                + "\"batchId\":\"b-icmp-rev\",\"firstSequence\":1,\"lastSequence\":2,\"checksum\":\""
                + reverseChecksum + "\",\"records\":[" + snmpSecond + "," + icmpFirst + "]}";
        TelemetryBatchRequest reverseReq = mapper.readValue(reverseJson, TelemetryBatchRequest.class);

        IngestionException reverseEx = assertThrows(IngestionException.class, () ->
                realService.ingestTelemetry(reverseJson.getBytes(StandardCharsets.UTF_8), reverseReq, "b-icmp-rev", cert));
        assertEquals("ERR_UNSUPPORTED_RECORD_TYPE", reverseEx.getCode());

        assertNoDurableSideEffects();
    }
}
