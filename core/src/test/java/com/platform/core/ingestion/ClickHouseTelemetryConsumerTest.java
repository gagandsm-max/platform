package com.platform.core.ingestion;

import ch.qos.logback.classic.Level;
import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.classic.spi.IThrowableProxy;
import ch.qos.logback.classic.spi.StackTraceElementProxy;
import ch.qos.logback.core.read.ListAppender;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.apache.kafka.clients.consumer.Consumer;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.clients.consumer.ConsumerRecords;
import org.apache.kafka.common.TopicPartition;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
import org.springframework.kafka.KafkaException;
import org.springframework.kafka.listener.DefaultErrorHandler;
import org.springframework.kafka.listener.ListenerExecutionFailedException;
import org.springframework.kafka.listener.MessageListenerContainer;
import org.springframework.kafka.support.Acknowledgment;
import org.springframework.util.backoff.FixedBackOff;

import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
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

        verify(clickHouseClient, times(1)).insertNumericBatch(anyList(), anyString());
        verify(ack, times(1)).acknowledge();
        assertFalse(metrics.isConsumerDegraded());
    }

    @Test
    void testBatchConsumesOneClickHouseInsertAndOneAck() throws Exception {
        List<ConsumerRecord<String, String>> records = new ArrayList<>();
        for (int i = 0; i < 3; i++) {
            CanonicalObservation obs = new CanonicalObservation(
                    "tenant-1", "entity-" + i, "metric.cpu", "sensor-net-01",
                    Instant.parse("2026-09-19T07:00:00Z").plusSeconds(i),
                    10.0 + i, 1, 0, String.valueOf(100 + i), "snmp"
            );
            records.add(new ConsumerRecord<>(
                    "telemetry.numeric.v1", 0, 100L + i, "key-" + i,
                    objectMapper.writeValueAsString(obs)
            ));
        }
        Acknowledgment ack = mock(Acknowledgment.class);

        consumer.consumeBatch(records, ack);

        @SuppressWarnings("unchecked")
        org.mockito.ArgumentCaptor<List<CanonicalObservation>> batchCaptor =
                org.mockito.ArgumentCaptor.forClass(List.class);
        org.mockito.ArgumentCaptor<String> tokenCaptor =
                org.mockito.ArgumentCaptor.forClass(String.class);
        verify(clickHouseClient, times(1)).insertNumericBatch(batchCaptor.capture(), tokenCaptor.capture());
        assertEquals(3, batchCaptor.getValue().size());
        assertTrue(tokenCaptor.getValue().matches("[0-9a-f]{64}"));
        verify(ack, times(1)).acknowledge();
        assertFalse(metrics.isConsumerDegraded());
    }

    @Test
    void testDeduplicationTokenStableForSameKafkaCoordinates() {
        List<ConsumerRecord<String, String>> first = List.of(
                new ConsumerRecord<>("telemetry.numeric.v1", 0, 10L, "a", "payload-a"),
                new ConsumerRecord<>("telemetry.numeric.v1", 0, 11L, "b", "payload-b")
        );
        List<ConsumerRecord<String, String>> sameCoordinatesDifferentPayload = List.of(
                new ConsumerRecord<>("telemetry.numeric.v1", 0, 10L, "x", "different"),
                new ConsumerRecord<>("telemetry.numeric.v1", 0, 11L, "y", "different-again")
        );
        List<ConsumerRecord<String, String>> nextOffset = List.of(
                new ConsumerRecord<>("telemetry.numeric.v1", 0, 10L, "a", "payload-a"),
                new ConsumerRecord<>("telemetry.numeric.v1", 0, 12L, "b", "payload-b")
        );

        String token = ClickHouseTelemetryConsumer.deduplicationToken(first);
        assertEquals(token, ClickHouseTelemetryConsumer.deduplicationToken(sameCoordinatesDifferentPayload));
        assertNotEquals(token, ClickHouseTelemetryConsumer.deduplicationToken(nextOffset));
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

        List<Logger> capturedLoggers = new ArrayList<>();
        List<ListAppender<ILoggingEvent>> appenders = new ArrayList<>();
        List<Level> previousLevels = new ArrayList<>();
        for (String loggerName : List.of(
                ClickHouseTelemetryConsumer.class.getName(),
                DefaultErrorHandler.class.getName(),
                "org.springframework.kafka.listener.KafkaMessageListenerContainer",
                "org.springframework.kafka.listener.RecoveryCallback"
        )) {
            Logger logger = (Logger) LoggerFactory.getLogger(loggerName);
            ListAppender<ILoggingEvent> listAppender = new ListAppender<>();
            listAppender.start();
            logger.addAppender(listAppender);
            previousLevels.add(logger.getLevel());
            logger.setLevel(Level.TRACE);
            capturedLoggers.add(logger);
            appenders.add(listAppender);
        }

        try {
            IngestionException ex = assertThrows(IngestionException.class, () ->
                    consumer.consumeBatch(List.of(record), ack));
            assertEquals("ERR_KAFKA_POISON_EVENT", ex.getCode());

            verify(ack, never()).acknowledge();
            assertTrue(metrics.isConsumerDegraded());
            assertEquals(1, metrics.getConsumerPoisonEvents());

            assertFalse(ex.getMessage().contains(SUPER_SECRET_SENTINEL_DO_NOT_LOG),
                    "exception message must not echo sentinel");
            assertFalse(ex.getMessage().contains(poisonPayload),
                    "exception message must not echo raw poison payload");
            assertNull(ex.getCause(), "poison parse path must not attach parser/Jackson cause");
            assertNoSecretInThrowableChain(ex, poisonPayload);

            String consumerLogs = appenders.get(0).list.stream()
                    .map(ILoggingEvent::getFormattedMessage)
                    .collect(Collectors.joining("\n"));
            assertFalse(consumerLogs.contains(SUPER_SECRET_SENTINEL_DO_NOT_LOG),
                    "sentinel must not appear in poison-path logs");
            assertFalse(consumerLogs.contains(poisonPayload),
                    "full poison payload must not appear in logs");
            assertTrue(consumerLogs.contains("ERR_KAFKA_POISON_EVENT"),
                    "stable poison error code must be logged");
            assertTrue(consumerLogs.contains(topic), "topic must be logged");
            assertTrue(consumerLogs.contains(String.valueOf(partition)), "partition must be logged");
            assertTrue(consumerLogs.contains(String.valueOf(offset)), "offset must be logged");
            assertTrue(
                    consumerLogs.contains("JsonParseException") || consumerLogs.contains("JsonMappingException")
                            || consumerLogs.contains("MismatchedInputException")
                            || consumerLogs.contains("Exception"),
                    "parse failure class must be logged; got: " + consumerLogs
            );
            assertTrue(
                    consumerLogs.contains(String.valueOf(poisonPayload.getBytes(StandardCharsets.UTF_8).length)),
                    "payload byte length must be logged"
            );

            // Mirror the production batch DefaultErrorHandler path. A plain listener exception
            // means the complete batch is retried/redelivered; we never assert that preceding
            // records were durably processed.
            DefaultErrorHandler errorHandler = new DefaultErrorHandler((rec, exception) -> {
                throw new KafkaException(
                        "Exhausted retries for record at topic=" + rec.topic()
                                + " partition=" + rec.partition()
                                + " offset=" + rec.offset()
                                + "; halting partition progression",
                        exception
                );
            }, new FixedBackOff(0L, 0L));
            errorHandler.setAckAfterHandle(false);

            ListenerExecutionFailedException listenerFailed = new ListenerExecutionFailedException(
                    "Listener method threw exception", ex);
            @SuppressWarnings("unchecked")
            Consumer<String, String> kafkaConsumer = mock(Consumer.class);
            MessageListenerContainer container = mock(MessageListenerContainer.class);
            when(container.getListenerId()).thenReturn("telemetry-test-listener");
            ConsumerRecords<String, String> batch = new ConsumerRecords<>(
                    Map.of(new TopicPartition(topic, partition), List.of(record))
            );

            try {
                errorHandler.handleBatch(
                        listenerFailed,
                        batch,
                        kafkaConsumer,
                        container,
                        () -> { throw listenerFailed; }
                );
            } catch (RuntimeException expectedFailClosed) {
                // The production recoverer intentionally refuses to skip the poisoned record.
            }

            String errorHandlerLogs = loggingEventsIncludingThrowables(appenders.get(1));
            assertTrue(
                    errorHandlerLogs.contains("Failed to handle")
                            || errorHandlerLogs.contains("Exhausted retries"),
                    "DefaultErrorHandler recovery path must be exercised; got: " + errorHandlerLogs
            );
            assertFalse(errorHandlerLogs.contains(SUPER_SECRET_SENTINEL_DO_NOT_LOG),
                    "sentinel must not appear in DefaultErrorHandler recovery logs: " + errorHandlerLogs);
            assertFalse(errorHandlerLogs.contains(poisonPayload),
                    "raw poison payload must not appear in DefaultErrorHandler recovery logs: "
                            + errorHandlerLogs);

            String allFrameworkLogs = appenders.stream()
                    .skip(1)
                    .map(this::loggingEventsIncludingThrowables)
                    .collect(Collectors.joining("\n"));
            assertFalse(allFrameworkLogs.contains(SUPER_SECRET_SENTINEL_DO_NOT_LOG),
                    "sentinel must not appear in DefaultErrorHandler/container recovery logs: "
                            + allFrameworkLogs);
            assertFalse(allFrameworkLogs.contains(poisonPayload),
                    "raw poison payload must not appear in DefaultErrorHandler/container recovery logs: "
                            + allFrameworkLogs);

            String allLogsIncludingThrowables = appenders.stream()
                    .map(this::loggingEventsIncludingThrowables)
                    .collect(Collectors.joining("\n"));
            assertFalse(allLogsIncludingThrowables.contains(SUPER_SECRET_SENTINEL_DO_NOT_LOG),
                    "sentinel must not appear in any captured logger throwable chain");
            assertFalse(allLogsIncludingThrowables.contains(poisonPayload),
                    "raw poison payload must not appear in any captured logger throwable chain");
        } finally {
            for (int i = 0; i < capturedLoggers.size(); i++) {
                capturedLoggers.get(i).detachAppender(appenders.get(i));
                capturedLoggers.get(i).setLevel(previousLevels.get(i));
                appenders.get(i).stop();
            }
        }
    }

    private static void assertNoSecretInThrowableChain(Throwable throwable, String poisonPayload) {
        Throwable cursor = throwable;
        int depth = 0;
        while (cursor != null) {
            String message = cursor.getMessage();
            if (message != null) {
                assertFalse(message.contains(SUPER_SECRET_SENTINEL_DO_NOT_LOG),
                        "cause chain message must not contain sentinel at depth=" + depth
                                + " class=" + cursor.getClass().getName());
                assertFalse(message.contains(poisonPayload),
                        "cause chain message must not contain raw payload at depth=" + depth
                                + " class=" + cursor.getClass().getName());
            }
            cursor = cursor.getCause();
            depth++;
            assertTrue(depth < 32, "cause chain unexpectedly deep");
        }
    }

    private String loggingEventsIncludingThrowables(ListAppender<ILoggingEvent> appender) {
        StringBuilder sb = new StringBuilder();
        for (ILoggingEvent event : appender.list) {
            sb.append(event.getFormattedMessage()).append('\n');
            IThrowableProxy proxy = event.getThrowableProxy();
            while (proxy != null) {
                sb.append(proxy.getClassName()).append(": ").append(proxy.getMessage()).append('\n');
                StackTraceElementProxy[] frames = proxy.getStackTraceElementProxyArray();
                if (frames != null) {
                    for (StackTraceElementProxy frame : frames) {
                        sb.append(frame.getSTEAsString()).append('\n');
                    }
                }
                proxy = proxy.getCause();
            }
        }
        return sb.toString();
    }

    @Test
    void testPoisonRecordInBatchPreventsPartialPersistenceAndAck() throws Exception {
        CanonicalObservation valid = new CanonicalObservation(
                "tenant-1", "entity-1", "metric.cpu", "sensor-net-01",
                Instant.parse("2026-09-19T07:00:00Z"), 10.0, 1, 0, "100", "snmp"
        );
        ConsumerRecord<String, String> good = new ConsumerRecord<>(
                "telemetry.numeric.v1", 0, 200L, "good", objectMapper.writeValueAsString(valid)
        );
        ConsumerRecord<String, String> poison = new ConsumerRecord<>(
                "telemetry.numeric.v1", 0, 201L, "bad",
                "{\"secret\":\"" + SUPER_SECRET_SENTINEL_DO_NOT_LOG + "\",not-json"
        );
        Acknowledgment ack = mock(Acknowledgment.class);

        IngestionException ex = assertThrows(
                IngestionException.class,
                () -> consumer.consumeBatch(List.of(good, poison), ack)
        );

        assertEquals("ERR_KAFKA_POISON_EVENT", ex.getCode());
        verifyNoInteractions(clickHouseClient);
        verify(ack, never()).acknowledge();
        assertTrue(metrics.isConsumerDegraded());
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
                .when(clickHouseClient).insertNumericBatch(anyList(), anyString());

        assertDoesNotThrow(() -> consumer.consume(record, ack));

        verify(clickHouseClient, times(3)).insertNumericBatch(anyList(), anyString());
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

        doThrow(new RuntimeException("ClickHouse rejected row containing " + SUPER_SECRET_SENTINEL_DO_NOT_LOG))
                .when(clickHouseClient).insertNumericBatch(anyList(), anyString());

        RuntimeException ex = assertThrows(RuntimeException.class, () ->
                consumer.consume(record, ack));
        assertTrue(ex.getMessage().contains("ClickHouse persistence unavailable"));
        assertFalse(ex.getMessage().contains(SUPER_SECRET_SENTINEL_DO_NOT_LOG));
        assertNull(ex.getCause(), "downstream ClickHouse diagnostics must not cross the Kafka framework boundary");

        verify(clickHouseClient, times(3)).insertNumericBatch(anyList(), anyString());
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
