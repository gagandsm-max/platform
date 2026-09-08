package com.platform.core.ingestion;

import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;

import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

class SensorIngestionServiceTest {

    private IngestionValidator validator;
    private IdempotencyTracker idempotencyTracker;
    private KafkaTelemetryProducer kafkaProducer;
    private IngestionMetrics metrics;
    private SensorIngestionService service;

    @BeforeEach
    void setUp() {
        validator = mock(IngestionValidator.class);
        idempotencyTracker = new IdempotencyTracker();
        kafkaProducer = mock(KafkaTelemetryProducer.class);
        metrics = new IngestionMetrics();
        service = new SensorIngestionService(validator, idempotencyTracker, kafkaProducer, metrics);
    }

    @Test
    void testSuccessfulIngestionPublishesToKafkaAndReturnsDurableAck() {
        X509Certificate cert = mock(X509Certificate.class);
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-1", 100, 105, "sum-123", List.of());
        byte[] rawBody = "{}".getBytes();

        List<CanonicalObservation> observations = List.of(
                new CanonicalObservation("default", "sensor-net-01", "cpu.util", "sensor-net-01", Instant.now(), 42.0, 1, 0, "105", "snmp")
        );
        when(validator.validateAndExtract(rawBody, req, cert)).thenReturn(observations);

        TelemetryAckResponse ack = service.ingestTelemetry(rawBody, req, "b-1", cert);

        assertNotNull(ack);
        assertEquals("ACCEPTED", ack.status());
        assertEquals(105L, ack.acceptedThroughSequence());
        assertEquals("sensor-net-01", ack.sensorIdentity());

        verify(kafkaProducer, times(1)).publishSynchronously(observations);

        // Verify idempotency cache
        TelemetryAckResponse replay = service.ingestTelemetry(rawBody, req, "b-1", cert);
        assertEquals(105L, replay.acceptedThroughSequence());
        // Verify kafkaProducer was not called again
        verify(kafkaProducer, times(1)).publishSynchronously(observations);
    }

    @Test
    void testKafkaFailurePropagatesAndDoesNotAck() {
        X509Certificate cert = mock(X509Certificate.class);
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-2", 1, 1, "sum-fail", List.of());
        byte[] rawBody = "{}".getBytes();

        List<CanonicalObservation> observations = List.of();
        when(validator.validateAndExtract(rawBody, req, cert)).thenReturn(observations);
        doThrow(new IngestionException(HttpStatus.SERVICE_UNAVAILABLE, "ERR_KAFKA_PUBLICATION_FAILED", "Kafka unavailable"))
                .when(kafkaProducer).publishSynchronously(observations);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestTelemetry(rawBody, req, "b-2", cert));
        assertEquals(HttpStatus.SERVICE_UNAVAILABLE, ex.getStatus());
        assertEquals("ERR_KAFKA_PUBLICATION_FAILED", ex.getCode());

        // Ensure not recorded in idempotency tracker
        assertNull(idempotencyTracker.checkDuplicate("b-2", "sum-fail"));
    }
}
