package com.platform.core.ingestion;

import ch.qos.logback.classic.Level;
import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.core.read.ListAppender;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
import org.springframework.kafka.support.Acknowledgment;

import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.stream.Collectors;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.anyList;
import static org.mockito.Mockito.*;

class ClickHouseTelemetryConsumerTest {

    private static final String SUPER_SECRET_SENTINEL_DO_NOT_LOG = "SUPER_SECRET_SENTINEL_DO_NOT_LOG";

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
        String poisonPayload = "{\"secret\":\"" + SUPER_SECRET_SENTINEL_DO_NOT_LOG + "\",not-json";
        String topic = "telemetry.numeric.v1";
        int partition = 0;
        long offset = 101L;
        ConsumerRecord<String, String> record = new ConsumerRecord<>(
                topic, partition, offset, "key-bad", poisonPayload
        );
        Acknowledgment ack = mock(Acknowledgment.class);

        Logger logger = (Logger) LoggerFactory.getLogger(ClickHouseTelemetryConsumer.class);
        ListAppender<ILoggingEvent> listAppender = new ListAppender<>();
        listAppender.start();
        logger.addAppender(listAppender);
        Level previousLevel = logger.getLevel();
        logger.setLevel(Level.ERROR);
        try {
            IngestionException ex = assertThrows(IngestionException.class, () ->
                    consumer.consume(record, ack));
            assertEquals("ERR_KAFKA_POISON_EVENT", ex.getCode());

            verify(ack, never()).acknowledge();
            assertTrue(metrics.isConsumerDegraded());
            assertEquals(1, metrics.getConsumerPoisonEvents());

            String logs = listAppender.list.stream()
                    .map(ILoggingEvent::getFormattedMessage)
                    .collect(Collectors.joining("\n"));
            assertFalse(logs.contains(SUPER_SECRET_SENTINEL_DO_NOT_LOG),
                    "sentinel must not appear in poison-path logs");
            assertFalse(logs.contains(poisonPayload),
                    "full poison payload must not appear in logs");
            assertTrue(logs.contains("ERR_KAFKA_POISON_EVENT"),
                    "stable poison error code must be logged");
            assertTrue(logs.contains(topic), "topic must be logged");
            assertTrue(logs.contains(String.valueOf(partition)), "partition must be logged");
            assertTrue(logs.contains(String.valueOf(offset)), "offset must be logged");
            assertTrue(
                    logs.contains("JsonParseException") || logs.contains("JsonMappingException")
                            || logs.contains("MismatchedInputException")
                            || logs.contains("Exception"),
                    "parse failure class must be logged; got: " + logs
            );
            assertTrue(
                    logs.contains(String.valueOf(poisonPayload.getBytes(StandardCharsets.UTF_8).length)),
                    "payload byte length must be logged"
            );
            assertFalse(ex.getMessage().contains(SUPER_SECRET_SENTINEL_DO_NOT_LOG),
                    "exception message must not echo sentinel");
        } finally {
            logger.detachAppender(listAppender);
            logger.setLevel(previousLevel);
            listAppender.stop();
        }
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

    @Test
    void testSpringContextBeanInstantiation() {
        org.springframework.context.annotation.AnnotationConfigApplicationContext context =
                new org.springframework.context.annotation.AnnotationConfigApplicationContext();
        context.getDefaultListableBeanFactory().registerSingleton("clickHouseClient", clickHouseClient);
        context.getDefaultListableBeanFactory().registerSingleton("objectMapper", objectMapper);
        context.getDefaultListableBeanFactory().registerSingleton("ingestionMetrics", metrics);
        context.register(ClickHouseTelemetryConsumer.class);
        assertDoesNotThrow(context::refresh);
        assertNotNull(context.getBean(ClickHouseTelemetryConsumer.class));
        context.close();
    }
}
