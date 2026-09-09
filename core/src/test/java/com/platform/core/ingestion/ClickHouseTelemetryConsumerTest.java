package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.kafka.support.Acknowledgment;

import java.time.Instant;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.anyList;
import static org.mockito.Mockito.*;

class ClickHouseTelemetryConsumerTest {

    private ClickHouseClient clickHouseClient;
    private ObjectMapper objectMapper;
    private IngestionMetrics metrics;
    private ClickHouseTelemetryConsumer consumer;

    @BeforeEach
    void setUp() {
        clickHouseClient = mock(ClickHouseClient.class);
        objectMapper = new ObjectMapper().findAndRegisterModules();
        metrics = new IngestionMetrics();
        consumer = new ClickHouseTelemetryConsumer(clickHouseClient, objectMapper, metrics, 3, 10);
    }

    @Test
    void testConsumeSuccessAcknowledgesOffset() throws Exception {
        CanonicalObservation obs = new CanonicalObservation(
                "tenant-1", "entity-1", ".1.3.6.1.2.1.1.3.0", "sensor-net-01",
                Instant.now(), 99.5, 1, 0, "100", "snmp"
        );
        String json = objectMapper.writeValueAsString(obs);
        ConsumerRecord<String, String> record = new ConsumerRecord<>(
                "telemetry.numeric.v1", 0, 100L, "sensor-net-01:100", json
        );
        Acknowledgment ack = mock(Acknowledgment.class);

        consumer.consume(record, ack);

        verify(clickHouseClient, times(1)).insertNumericBatch(anyList());
        verify(ack, times(1)).acknowledge();
        assertFalse(metrics.isConsumerDegraded());
    }

    @Test
    void testMalformedPoisonMessageDoesNotAcknowledgeAndThrows() {
        ConsumerRecord<String, String> record = new ConsumerRecord<>(
                "telemetry.numeric.v1", 0, 101L, "key-bad", "not-json"
        );
        Acknowledgment ack = mock(Acknowledgment.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                consumer.consume(record, ack));
        assertEquals("ERR_KAFKA_POISON_EVENT", ex.getCode());

        verify(ack, never()).acknowledge();
        assertTrue(metrics.isConsumerDegraded());
        assertEquals(1, metrics.getConsumerPoisonEvents());
    }

    @Test
    void testTransientFailureRetriesAndSucceedsWithAck() throws Exception {
        CanonicalObservation obs = new CanonicalObservation(
                "tenant-1", "entity-1", ".1.3.6.1.2.1.1.3.0", "sensor-net-01",
                Instant.now(), 99.5, 1, 0, "100", "snmp"
        );
        String json = objectMapper.writeValueAsString(obs);
        ConsumerRecord<String, String> record = new ConsumerRecord<>(
                "telemetry.numeric.v1", 0, 100L, "sensor-net-01:100", json
        );
        Acknowledgment ack = mock(Acknowledgment.class);

        // Fail first 2 attempts, succeed on 3rd
        doThrow(new RuntimeException("ClickHouse 503"))
                .doThrow(new RuntimeException("ClickHouse timeout"))
                .doNothing()
                .when(clickHouseClient).insertNumericBatch(anyList());

        assertDoesNotThrow(() -> consumer.consume(record, ack));

        verify(clickHouseClient, times(3)).insertNumericBatch(anyList());
        verify(ack, times(1)).acknowledge();
        assertFalse(metrics.isConsumerDegraded());
    }

    @Test
    void testRetryExhaustionDoesNotAcknowledgeAndMarksDegraded() throws Exception {
        CanonicalObservation obs = new CanonicalObservation(
                "tenant-1", "entity-1", ".1.3.6.1.2.1.1.3.0", "sensor-net-01",
                Instant.now(), 99.5, 1, 0, "100", "snmp"
        );
        String json = objectMapper.writeValueAsString(obs);
        ConsumerRecord<String, String> record = new ConsumerRecord<>(
                "telemetry.numeric.v1", 0, 100L, "sensor-net-01:100", json
        );
        Acknowledgment ack = mock(Acknowledgment.class);

        doThrow(new RuntimeException("ClickHouse connection refused"))
                .when(clickHouseClient).insertNumericBatch(anyList());

        RuntimeException ex = assertThrows(RuntimeException.class, () ->
                consumer.consume(record, ack));
        assertTrue(ex.getMessage().contains("ClickHouse persistence unavailable"));

        verify(clickHouseClient, times(3)).insertNumericBatch(anyList());
        verify(ack, never()).acknowledge();
        assertTrue(metrics.isConsumerDegraded());
    }
}
