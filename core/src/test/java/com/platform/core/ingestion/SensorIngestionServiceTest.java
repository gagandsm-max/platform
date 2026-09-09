package com.platform.core.ingestion;

import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import com.platform.core.ingestion.replay.TelemetryWatermarkStore;
import com.platform.core.ingestion.replay.WatermarkState;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;

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
    }

    private X509Certificate createMockCert(String cn) {
        X509Certificate cert = mock(X509Certificate.class);
        when(cert.getSubjectX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal("CN=" + cn));
        return cert;
    }

    @Test
    void testClaimedSensorIdMismatchingCertCnThrowsIdentityMismatch() {
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-02", "epoch-1", "b-1", 100, 105, "sum-123", List.of());
        byte[] rawBody = "{}".getBytes();

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestTelemetry(rawBody, req, "b-1", cert));
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
        assertEquals("ERR_AUTH_IDENTITY_MISMATCH", ex.getCode());
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
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-net-01")).thenReturn(null);

        TelemetryAckResponse ack = service.ingestTelemetry(rawBody, req, "b-1", cert);

        assertNotNull(ack);
        assertEquals("ACCEPTED", ack.status());
        assertEquals(105L, ack.acceptedThroughSequence());
        assertEquals("sensor-net-01", ack.sensorIdentity());

        verify(kafkaProducer, times(1)).publishBatchTransactionally(observations);
        verify(watermarkStore, times(1)).updateWatermark(
                eq(tenantId), eq("sensor-net-01"), eq(105L), eq(100L), eq(105L), eq("sum-123"), eq("b-1")
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

        WatermarkState existingWatermark = new WatermarkState(
                "sensor-net-01", tenantId, 105L, 100L, 105L, "sum-123", "b-1", Instant.now()
        );
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-net-01")).thenReturn(existingWatermark);

        TelemetryAckResponse ack = service.ingestTelemetry(rawBody, req, "b-1", cert);

        assertNotNull(ack);
        assertEquals("ACCEPTED", ack.status());
        assertEquals(105L, ack.acceptedThroughSequence());
        assertEquals("sensor-net-01", ack.sensorIdentity());

        // Zero additional Kafka publishes, zero watermark updates
        verify(kafkaProducer, never()).publishBatchTransactionally(anyList());
        verify(watermarkStore, never()).updateWatermark(any(), anyString(), anyLong(), anyLong(), anyLong(), anyString(), anyString());
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

        WatermarkState existingWatermark = new WatermarkState(
                "sensor-net-01", tenantId, 105L, 100L, 105L, "sum-ORIGINAL", "b-1", Instant.now()
        );
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-net-01")).thenReturn(existingWatermark);

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

        WatermarkState existingWatermark = new WatermarkState(
                "sensor-net-01", tenantId, 105L, 100L, 105L, "sum-105", "b-105", Instant.now()
        );
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-net-01")).thenReturn(existingWatermark);

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

        WatermarkState existingWatermark = new WatermarkState(
                "sensor-net-01", tenantId, 105L, 100L, 105L, "sum-105", "b-105", Instant.now()
        );
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-net-01")).thenReturn(existingWatermark);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestTelemetry(rawBody, req, "b-gap", cert));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_SEQUENCE_GAP", ex.getCode());

        verify(kafkaProducer, never()).publishBatchTransactionally(anyList());
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
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-net-01")).thenReturn(null);

        doThrow(new IngestionException(HttpStatus.SERVICE_UNAVAILABLE, "ERR_KAFKA_PUBLICATION_FAILED", "Kafka down"))
                .when(kafkaProducer).publishBatchTransactionally(observations);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestTelemetry(rawBody, req, "b-2", cert));
        assertEquals(HttpStatus.SERVICE_UNAVAILABLE, ex.getStatus());
        assertEquals("ERR_KAFKA_PUBLICATION_FAILED", ex.getCode());

        // Watermark MUST NOT be updated
        verify(watermarkStore, never()).updateWatermark(any(), anyString(), anyLong(), anyLong(), anyLong(), anyString(), anyString());
    }

    @Test
    void testKafkaCommitSucceedsButPostgresWatermarkThrowsNoAckAndRetryRepublishesCleanly() {
        // Fault injection: Kafka publication succeeds, but Postgres crashes / rolls back before watermark update.
        // In at-least-once architecture:
        // 1. Core throws exception (no ACK returned to sensor).
        // 2. Sensor retries sending the same batch.
        // 3. Since watermark was never updated, retry is re-processed and republishes to Kafka with identical deterministic sequence IDs.
        // 4. Postgres succeeds on retry and returns ACK.
        X509Certificate cert = createMockCert("sensor-net-01");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-fault", 100, 105, "sum-fault", List.of());
        byte[] rawBody = "{}".getBytes();

        List<CanonicalObservation> observations = List.of(
                new CanonicalObservation(tenantId.toString(), entityId.toString(), "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "105", "snmp")
        );
        when(validator.resolveAuthority(cert)).thenReturn(testScope);
        when(validator.validateAndExtract(rawBody, req, testScope)).thenReturn(observations);
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-net-01")).thenReturn(null);

        // Attempt 1: Kafka succeeds, but Postgres watermark store throws
        doThrow(new RuntimeException("PostgreSQL connection terminated unexpectedly"))
                .doNothing()
                .when(watermarkStore).updateWatermark(eq(tenantId), eq("sensor-net-01"), eq(105L), eq(100L), eq(105L), eq("sum-fault"), eq("b-fault"));

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
        verify(watermarkStore, times(2)).updateWatermark(
                eq(tenantId), eq("sensor-net-01"), eq(105L), eq(100L), eq(105L), eq("sum-fault"), eq("b-fault")
        );
    }
}
