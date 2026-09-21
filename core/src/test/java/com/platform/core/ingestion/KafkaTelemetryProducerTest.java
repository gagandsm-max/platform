package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalCounterObservation;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryObservationBatch;
import java.math.BigInteger;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.kafka.core.KafkaOperations;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.kafka.support.SendResult;

import java.time.Instant;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.atomic.AtomicInteger;

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

        assertDoesNotThrow(() -> producer.publishBatchTransactionally(TelemetryObservationBatch.ofNumeric(List.of(obs1, obs2))));
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
                producer.publishBatchTransactionally(TelemetryObservationBatch.ofNumeric(List.of(obs))));
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

            when(ops.send(eq("telemetry.numeric.v1"), eq("100"), anyString())).thenReturn(successFuture);
            when(ops.send(eq("telemetry.numeric.v1"), eq("101"), anyString())).thenReturn(failFuture);

            return callback.doInOperations(ops);
        });

        IngestionException ex = assertThrows(IngestionException.class, () ->
                producer.publishBatchTransactionally(TelemetryObservationBatch.ofNumeric(List.of(obs1, obs2))));
        assertEquals(HttpStatus.SERVICE_UNAVAILABLE, ex.getStatus());
        assertEquals("ERR_KAFKA_PUBLICATION_FAILED", ex.getCode());
        assertEquals("Failed to atomically publish telemetry batch to Kafka cluster", ex.getMessage());
        assertEquals(1, metrics.getKafkaFailures());
    }

    @Test
    void testSpringContextBeanInstantiation() {
        org.springframework.context.annotation.AnnotationConfigApplicationContext context =
                new org.springframework.context.annotation.AnnotationConfigApplicationContext();
        context.getDefaultListableBeanFactory().registerSingleton("kafkaTemplate", kafkaTemplate);
        context.getDefaultListableBeanFactory().registerSingleton("objectMapper", objectMapper);
        context.getDefaultListableBeanFactory().registerSingleton("ingestionMetrics", metrics);
        context.register(KafkaTelemetryProducer.class);
        assertDoesNotThrow(context::refresh);
        assertNotNull(context.getBean(KafkaTelemetryProducer.class));
        context.close();
    }

    @Test
    void mixedNumericAndCounterBatchPublishesInsideOneTransaction() throws Exception {
        when(kafkaTemplate.isTransactional()).thenReturn(true);
        AtomicInteger sends = new AtomicInteger();
        when(kafkaTemplate.executeInTransaction(any())).thenAnswer(invocation -> {
            @SuppressWarnings("unchecked")
            org.springframework.kafka.core.KafkaOperations<String, String> ops =
                    mock(org.springframework.kafka.core.KafkaOperations.class);
            when(ops.send(anyString(), anyString(), anyString())).thenAnswer(send -> {
                sends.incrementAndGet();
                String payload = send.getArgument(2);
                assertFalse(payload.contains("1.8446744073709552E19"));
                if (payload.contains("COUNTER64") || payload.contains("counterValue")) {
                    assertTrue(payload.contains("\"counterValue\":\"18446744073709551615\""));
                    assertEquals("seq-c", send.getArgument(1));
                }
                return CompletableFuture.completedFuture(null);
            });
            return invocation.getArgument(0, org.springframework.kafka.core.KafkaOperations.OperationsCallback.class)
                    .doInOperations(ops);
        });

        CanonicalObservation gauge = new CanonicalObservation(
                "t", "e", "cpu", "s", Instant.now(), 1.25, 1, 0, "seq-g", "snmp");
        CanonicalCounterObservation counter = new CanonicalCounterObservation(
                "t", "e", "ifHCInOctets", "s", Instant.now(),
                new BigInteger("18446744073709551615"), 1, 0, "seq-c", "snmp");
        producer.publishBatchTransactionally(new TelemetryObservationBatch(List.of(gauge), List.of(counter)));
        assertEquals(2, sends.get());
        verify(kafkaTemplate, times(1)).executeInTransaction(any());
    }
}
