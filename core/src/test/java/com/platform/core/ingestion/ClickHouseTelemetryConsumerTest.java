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
                    consumer.consume(record, ack));
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

            // Mirror IngestionKafkaConfiguration DefaultErrorHandler setup exactly, then force
            // immediate recovery via setBackOffFunction so handleOne invokes the recoverer without
            // multi-attempt seeks (production uses FixedBackOff(500L, 3L) on the handler itself).
            DefaultErrorHandler errorHandler = new DefaultErrorHandler((rec, exception) -> {
                throw new KafkaException(
                        "Exhausted retries for record at topic=" + rec.topic()
                                + " partition=" + rec.partition()
                                + " offset=" + rec.offset()
                                + "; halting partition progression",
                        exception
                );
            }, new FixedBackOff(500L, 3L));
            errorHandler.setAckAfterHandle(false);
            errorHandler.setBackOffFunction((rec, exception) -> new FixedBackOff(0L, 0L));

            ListenerExecutionFailedException listenerFailed = new ListenerExecutionFailedException(
                    "Listener method threw exception", ex);
            @SuppressWarnings("unchecked")
            Consumer<String, String> kafkaConsumer = mock(Consumer.class);
            MessageListenerContainer container = mock(MessageListenerContainer.class);
            when(container.getListenerId()).thenReturn("telemetry-test-listener");

            // Recoverer throws KafkaException; DefaultErrorHandler catches, logs the failure chain,
            // and returns without rethrowing. Capture that log path (same recoverer as production).
            boolean remaining = errorHandler.handleOne(listenerFailed, record, kafkaConsumer, container);
            assertFalse(remaining, "poison recovery must not treat the record as successfully handled");

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
        assertEquals(
                com.platform.core.logging.CoreSupportabilityEventCodes.TELEMETRY_CLICKHOUSE_UNAVAILABLE,
                metrics.getConsumerDegradedReason()
        );
    }

    @Test
    void retryExhaustionLogOmitsRawExceptionSentinels() throws Exception {
        CanonicalObservation obs = new CanonicalObservation(
                "tenant-1", "entity-1", ".1.3.6.1.2.1.1.3.0", "sensor-net-01",
                Instant.now(), 99.5, 1, 0, "100", "snmp"
        );
        String json = objectMapper.writeValueAsString(obs);
        ConsumerRecord<String, String> record = new ConsumerRecord<>(
                "telemetry.numeric.v1", 0, 100L, "sensor-net-01:100", json
        );
        Acknowledgment ack = mock(Acknowledgment.class);

        String bearer = "Bearer FAKESECRET_abc123";
        String password = "password=FAKEPASSWORD";
        String url = "https://user:FAKEPASS@clickhouse.invalid";
        doThrow(new RuntimeException(bearer + " " + password + " " + url))
                .when(clickHouseClient).insertNumericBatch(anyList());

        Logger logger = (Logger) LoggerFactory.getLogger(ClickHouseTelemetryConsumer.class);
        ListAppender<ILoggingEvent> appender = new ListAppender<>();
        appender.start();
        logger.addAppender(appender);
        Level previous = logger.getLevel();
        logger.setLevel(Level.TRACE);
        try {
            assertThrows(RuntimeException.class, () -> consumer.consume(record, ack));
            String logs = appender.list.stream()
                    .map(ILoggingEvent::getFormattedMessage)
                    .collect(Collectors.joining("\n"));
            assertFalse(logs.contains(bearer), logs);
            assertFalse(logs.contains(password), logs);
            assertFalse(logs.contains(url), logs);
            assertFalse(logs.contains("FAKESECRET_abc123"), logs);
            assertFalse(logs.contains("FAKEPASSWORD"), logs);
            assertTrue(logs.contains("failureClass=RuntimeException"), logs);
        } finally {
            logger.detachAppender(appender);
            logger.setLevel(previous);
            appender.stop();
        }
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
