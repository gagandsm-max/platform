package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.kafka.support.SendResult;

import java.time.Instant;
import java.util.List;
import java.util.concurrent.CompletableFuture;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.*;

class KafkaTelemetryProducerTest {

    private KafkaTemplate<String, String> kafkaTemplate;
    private ObjectMapper objectMapper;
    private IngestionMetrics metrics;
    private KafkaTelemetryProducer producer;

    @BeforeEach
    void setUp() {
        kafkaTemplate = mock(KafkaTemplate.class);
        objectMapper = new ObjectMapper().findAndRegisterModules();
        metrics = new IngestionMetrics();
        producer = new KafkaTelemetryProducer(kafkaTemplate, objectMapper, metrics, 2);
    }

    @Test
    void testPublishSynchronouslySuccess() {
        CanonicalObservation obs = new CanonicalObservation(
                "default", "entity-1", "cpu.util", "sensor-net-01",
                Instant.now(), 50.0, 1, 0, "1", "snmp"
        );

        CompletableFuture<SendResult<String, String>> future = CompletableFuture.completedFuture(mock(SendResult.class));
        when(kafkaTemplate.send(eq("telemetry.numeric.v1"), eq("default:entity-1"), anyString()))
                .thenReturn(future);

        assertDoesNotThrow(() -> producer.publishSynchronously(List.of(obs)));
        verify(kafkaTemplate, times(1)).send(eq("telemetry.numeric.v1"), eq("default:entity-1"), anyString());
    }

    @Test
    void testPublishSynchronouslyFailureThrowsIngestionException() {
        CanonicalObservation obs = new CanonicalObservation(
                "default", "entity-1", "cpu.util", "sensor-net-01",
                Instant.now(), 50.0, 1, 0, "1", "snmp"
        );

        CompletableFuture<SendResult<String, String>> future = new CompletableFuture<>();
        future.completeExceptionally(new RuntimeException("Broker unreachable"));
        when(kafkaTemplate.send(eq("telemetry.numeric.v1"), eq("default:entity-1"), anyString()))
                .thenReturn(future);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                producer.publishSynchronously(List.of(obs)));
        assertEquals(HttpStatus.SERVICE_UNAVAILABLE, ex.getStatus());
        assertEquals("ERR_KAFKA_PUBLICATION_FAILED", ex.getCode());
    }
}
