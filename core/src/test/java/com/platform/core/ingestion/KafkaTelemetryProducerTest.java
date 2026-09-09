package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.kafka.core.KafkaOperations;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.kafka.support.SendResult;

import java.time.Instant;
import java.util.List;
import java.util.concurrent.CompletableFuture;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
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
    void testPublishBatchTransactionallySucceedsWhenTransactional() {
        when(kafkaTemplate.isTransactional()).thenReturn(true);

        CanonicalObservation obs1 = new CanonicalObservation(
                "tenant-1", "entity-1", ".1.3.6.1.2.1.1.3.0", "sensor-net-01",
                Instant.now(), 50.0, 1, 0, "100", "snmp"
        );
        CanonicalObservation obs2 = new CanonicalObservation(
                "tenant-1", "entity-1", ".1.3.6.1.2.1.2.2.1.10.1", "sensor-net-01",
                Instant.now(), 100.0, 1, 0, "101", "snmp"
        );

        when(kafkaTemplate.executeInTransaction(any())).thenAnswer(invocation -> {
            KafkaOperations.OperationsCallback<String, String, Object> callback = invocation.getArgument(0);
            KafkaOperations<String, String> ops = mock(KafkaOperations.class);
            CompletableFuture<SendResult<String, String>> future = CompletableFuture.completedFuture(mock(SendResult.class));
            when(ops.send(anyString(), anyString(), anyString())).thenReturn(future);
            return callback.doInOperations(ops);
        });

        assertDoesNotThrow(() -> producer.publishBatchTransactionally(List.of(obs1, obs2)));
        verify(kafkaTemplate, times(1)).executeInTransaction(any());
        assertEquals(2, metrics.getRecordsAccepted());
    }

    @Test
    void testPublishFailsClosedWhenNotTransactional() {
        when(kafkaTemplate.isTransactional()).thenReturn(false);

        CanonicalObservation obs = new CanonicalObservation(
                "tenant-1", "entity-1", ".1.3.6.1.2.1.1.3.0", "sensor-net-01",
                Instant.now(), 50.0, 1, 0, "100", "snmp"
        );

        IngestionException ex = assertThrows(IngestionException.class, () ->
                producer.publishBatchTransactionally(List.of(obs)));
        assertEquals(HttpStatus.SERVICE_UNAVAILABLE, ex.getStatus());
        assertEquals("ERR_KAFKA_TRANSACTIONS_UNAVAILABLE", ex.getCode());
        verify(kafkaTemplate, never()).executeInTransaction(any());
    }

    @Test
    void testMidBatchSendFailureAbortsTransactionAndThrows() {
        when(kafkaTemplate.isTransactional()).thenReturn(true);

        CanonicalObservation obs1 = new CanonicalObservation(
                "tenant-1", "entity-1", ".1.3.6.1.2.1.1.3.0", "sensor-net-01",
                Instant.now(), 50.0, 1, 0, "100", "snmp"
        );
        CanonicalObservation obs2 = new CanonicalObservation(
                "tenant-1", "entity-1", ".1.3.6.1.2.1.2.2.1.10.1", "sensor-net-01",
                Instant.now(), 100.0, 1, 0, "101", "snmp"
        );

        when(kafkaTemplate.executeInTransaction(any())).thenAnswer(invocation -> {
            KafkaOperations.OperationsCallback<String, String, Object> callback = invocation.getArgument(0);
            KafkaOperations<String, String> ops = mock(KafkaOperations.class);

            CompletableFuture<SendResult<String, String>> successFuture = CompletableFuture.completedFuture(mock(SendResult.class));
            CompletableFuture<SendResult<String, String>> failFuture = new CompletableFuture<>();
            failFuture.completeExceptionally(new RuntimeException("Broker network timeout on record 2"));

            when(ops.send(eq("telemetry.numeric.v1"), eq("sensor-net-01:100"), anyString())).thenReturn(successFuture);
            when(ops.send(eq("telemetry.numeric.v1"), eq("sensor-net-01:101"), anyString())).thenReturn(failFuture);

            return callback.doInOperations(ops);
        });

        IngestionException ex = assertThrows(IngestionException.class, () ->
                producer.publishBatchTransactionally(List.of(obs1, obs2)));
        assertEquals(HttpStatus.SERVICE_UNAVAILABLE, ex.getStatus());
        assertEquals("ERR_KAFKA_PUBLICATION_FAILED", ex.getCode());
        assertEquals(1, metrics.getKafkaFailures());
    }
}
