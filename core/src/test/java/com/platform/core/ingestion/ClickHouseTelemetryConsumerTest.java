package com.platform.core.ingestion;

import ch.qos.logback.classic.Level;
import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.classic.spi.IThrowableProxy;
import ch.qos.logback.classic.spi.StackTraceElementProxy;
import ch.qos.logback.core.read.ListAppender;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalCounterObservation;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.apache.kafka.clients.consumer.Consumer;
import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
import org.springframework.boot.autoconfigure.kafka.KafkaProperties;
import org.springframework.kafka.KafkaException;
import org.springframework.kafka.config.ConcurrentKafkaListenerContainerFactory;
import org.springframework.kafka.core.ConsumerFactory;
import org.springframework.kafka.core.DefaultKafkaConsumerFactory;
import org.springframework.kafka.listener.ContainerProperties;
import org.springframework.kafka.listener.DefaultErrorHandler;
import org.springframework.kafka.listener.ListenerExecutionFailedException;
import org.springframework.kafka.listener.MessageListenerContainer;
import org.springframework.kafka.support.Acknowledgment;

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

        verify(clickHouseClient, times(1)).insertNumericBatch(anyList());
        verify(ack, times(1)).acknowledge();
        assertFalse(metrics.isConsumerDegraded());
    }

    @Test
    void testCounter64ConsumeSuccessAcknowledgesOffset() throws Exception {
        CanonicalCounterObservation counter = new CanonicalCounterObservation(
                "tenant-1", "entity-1", ".1.3.6.1.2.1.2.2.1.16.1", "sensor-net-01",
                Instant.now(), new java.math.BigInteger("18446744073709551615"), 1, 0, "100", "snmp"
        );
        String json = objectMapper.writeValueAsString(counter);
        ConsumerRecord<String, String> record = new ConsumerRecord<>(
                "telemetry.numeric.v1", 0, 200L, "sensor-net-01:100", json
        );
        Acknowledgment ack = mock(Acknowledgment.class);

        consumer.consume(record, ack);

        verify(clickHouseClient, times(1)).insertCounterBatch(anyList());
        verify(clickHouseClient, never()).insertNumericBatch(anyList());
        verify(ack, times(1)).acknowledge();
        assertFalse(metrics.isConsumerDegraded());
    }

    @Test
    void telemetryListenerFactoryPreservesManualAckAndReadCommittedPins() {
        IngestionKafkaConfiguration configuration = new IngestionKafkaConfiguration();
        ConsumerFactory<String, String> consumerFactory =
                configuration.telemetryKafkaConsumerFactory(new KafkaProperties());
        ConcurrentKafkaListenerContainerFactory<String, String> listenerFactory =
                configuration.telemetryKafkaListenerContainerFactory(consumerFactory);

        assertSame(consumerFactory, listenerFactory.getConsumerFactory());
        assertEquals(
                ContainerProperties.AckMode.MANUAL_IMMEDIATE,
                listenerFactory.getContainerProperties().getAckMode()
        );
        Map<String, Object> effective = ((DefaultKafkaConsumerFactory<String, String>) consumerFactory)
                .getConfigurationProperties();
        assertEquals("read_committed", effective.get(ConsumerConfig.ISOLATION_LEVEL_CONFIG));
        assertEquals(false, effective.get(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG));
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

            // Exercise the supported stopping handler without asynchronous unit-test threads.
            var errorHandler = new org.springframework.kafka.listener.CommonContainerStoppingErrorHandler(Runnable::run);
            ListenerExecutionFailedException listenerFailed = new ListenerExecutionFailedException(
                    "Listener method threw exception", ex);
            Consumer<String, String> kafkaConsumer = mock(Consumer.class);
            MessageListenerContainer container = mock(MessageListenerContainer.class);
            assertThrows(KafkaException.class, () -> errorHandler.handleRemaining(
                    listenerFailed, List.of(record), kafkaConsumer, container));
            verify(container).stopAbnormally(any(Runnable.class));
            verifyNoInteractions(kafkaConsumer);

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
        doThrow(new java.net.ConnectException("unavailable"))
                .doThrow(new java.net.http.HttpTimeoutException("timeout"))
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

        doThrow(new java.net.ConnectException("connection refused"))
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
            var failure = assertThrows(RuntimeException.class, () -> consumer.consume(record, ack));
            String logs = loggingEventsIncludingThrowables(appender);
            assertNoSecretInThrowableChain(failure, json);
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
    void retryConfigurationAcceptsExistingPositiveRangesAndZeroDelay() {
        for (int attempts : new int[] {1, 6}) {
            for (long delay : new long[] {0, 1001}) {
                assertDoesNotThrow(() -> new ClickHouseTelemetryConsumer(
                        clickHouseClient, objectMapper, metrics, attempts, delay));
            }
        }
        // An unused backoff is safe even at Long.MAX_VALUE; unsafe used delays fail closed.
        assertDoesNotThrow(() -> new ClickHouseTelemetryConsumer(
                clickHouseClient, objectMapper, metrics, 1, Long.MAX_VALUE));
        assertThrows(IllegalArgumentException.class, () -> new ClickHouseTelemetryConsumer(
                clickHouseClient, objectMapper, metrics, 2, Long.MAX_VALUE));
        assertThrows(IllegalArgumentException.class, () -> new ClickHouseTelemetryConsumer(
                clickHouseClient, objectMapper, metrics, Integer.MAX_VALUE, 0));
        for (int attempts : new int[] {0, -1}) {
            assertThrows(IllegalArgumentException.class, () -> new ClickHouseTelemetryConsumer(
                    clickHouseClient, objectMapper, metrics, attempts, 0));
        }
        assertThrows(IllegalArgumentException.class, () -> new ClickHouseTelemetryConsumer(
                clickHouseClient, objectMapper, metrics, 1, -1));
    }

    @Test
    void latePersistenceReturnNeverAcknowledges() throws Exception {
        var clock = new java.util.concurrent.atomic.AtomicLong();
        var budget = new TelemetryProcessingBudget(Map.of(ConsumerConfig.MAX_POLL_RECORDS_CONFIG, 1), 1, 0);
        var guarded = new ClickHouseTelemetryConsumer(clickHouseClient, objectMapper, metrics, budget, clock::get);
        doAnswer(invocation -> {
            clock.set(java.util.concurrent.TimeUnit.MILLISECONDS.toNanos(budget.recordProcessingMs));
            return null;
        }).when(clickHouseClient).insertNumericBatch(anyList());
        var ack = mock(Acknowledgment.class);
        assertThrows(RuntimeException.class, () -> guarded.consume(validRecord(), ack));
        verify(ack, never()).acknowledge();
        assertTrue(metrics.isConsumerDegraded());
        assertEquals("ERR_KAFKA_POLL_BUDGET", metrics.getConsumerDegradedReason());
    }

    @Test
    void commitFailureNeverRetriesPersistenceOrReportsHealthy() throws Exception {
        var ack = mock(Acknowledgment.class);
        doThrow(new org.apache.kafka.clients.consumer.CommitFailedException(SUPER_SECRET_SENTINEL_DO_NOT_LOG))
                .when(ack).acknowledge();
        var failure = assertThrows(RuntimeException.class, () -> consumer.consume(validRecord(), ack));
        verify(clickHouseClient, times(1)).insertNumericBatch(anyList());
        verify(ack, times(1)).acknowledge();
        assertTrue(metrics.isConsumerDegraded());
        assertNoSecretInThrowableChain(failure, "unused");
        assertEquals("ERR_KAFKA_GROUP_MEMBERSHIP_LOST", metrics.getConsumerDegradedReason());
    }

    @Test
    void unknownInsertionFailureStopsWithoutRetry() throws Exception {
        doThrow(new IllegalStateException(SUPER_SECRET_SENTINEL_DO_NOT_LOG))
                .when(clickHouseClient).insertNumericBatch(anyList());
        var ack = mock(Acknowledgment.class);
        assertThrows(RuntimeException.class, () -> consumer.consume(validRecord(), ack));
        verify(clickHouseClient, times(1)).insertNumericBatch(anyList());
        verifyNoInteractions(ack);
        assertEquals("ERR_CLICKHOUSE_PERMANENT", metrics.getConsumerDegradedReason());
    }

    @Test
    void successfulSiblingPartitionDoesNotClearBlockedPartition() throws Exception {
        doThrow(new java.net.ConnectException("unavailable"))
                .when(clickHouseClient).insertNumericBatch(anyList());
        var blocked = validRecord();
        assertThrows(RuntimeException.class, () -> consumer.consume(blocked, mock(Acknowledgment.class)));
        doNothing().when(clickHouseClient).insertNumericBatch(anyList());
        var sibling = new ConsumerRecord<>(blocked.topic(), 1, blocked.offset(), blocked.key(), blocked.value());
        consumer.consume(sibling, mock(Acknowledgment.class));
        assertTrue(metrics.isConsumerDegraded(), "sibling success cannot prove blocked record recovery");
        consumer.consume(blocked, mock(Acknowledgment.class));
        assertFalse(metrics.isConsumerDegraded(), "exact retained record can recover after persistence and ACK");
    }

    @Test
    void missingRequiredNumericAndCounterFieldsArePoisonBeforeInsertion() throws Exception {
        var numeric = objectMapper.readTree(validRecord().value());
        var counter = numeric.deepCopy();
        ((com.fasterxml.jackson.databind.node.ObjectNode) counter).remove("value");
        ((com.fasterxml.jackson.databind.node.ObjectNode) counter).put("observationKind", "COUNTER64");
        ((com.fasterxml.jackson.databind.node.ObjectNode) counter).put("counterValue", "18446744073709551615");
        for (var source : List.of(numeric, counter)) {
            for (String field : List.of("tenantId", "entityId", "metricId", "sensorId", "timestamp",
                    "sequenceId", "sourceProtocol", "quality", "flags",
                    source.has("value") ? "value" : "counterValue")) {
                var missing = (com.fasterxml.jackson.databind.node.ObjectNode) source.deepCopy();
                missing.remove(field);
                var record = new ConsumerRecord<String, String>("telemetry.numeric.v1", 0, 100L, "key", missing.toString());
                var ack = mock(Acknowledgment.class);
                var failure = assertThrows(IngestionException.class, () -> consumer.consume(record, ack), field);
                assertEquals("ERR_KAFKA_POISON_EVENT", failure.getCode());
                verifyNoInteractions(ack);
            }
        }
        verifyNoInteractions(clickHouseClient);
    }

    @Test
    void timeoutRemainsRecoverableWithoutExposingDetails() throws Exception {
        doThrow(new java.net.http.HttpTimeoutException(SUPER_SECRET_SENTINEL_DO_NOT_LOG))
                .when(clickHouseClient).insertNumericBatch(anyList());
        var ack = mock(Acknowledgment.class);
        var failure = assertThrows(RuntimeException.class, () -> consumer.consume(validRecord(), ack));
        assertNoSecretInThrowableChain(failure, "unused");
        verify(clickHouseClient, times(3)).insertNumericBatch(anyList());
        verifyNoInteractions(ack);
        assertEquals(TelemetryConsumerFailure.DEPENDENCY.reasonCode, metrics.getConsumerDegradedReason());
    }

    @Test
    void permanentHttpFailuresNeverRetryAndServiceFailureCanRecover() throws Exception {
        for (var reason : List.of(TelemetryConsumerFailure.AUTHORIZATION, TelemetryConsumerFailure.SCHEMA,
                TelemetryConsumerFailure.CONFIGURATION, TelemetryConsumerFailure.PERMANENT, TelemetryConsumerFailure.UNKNOWN)) {
            var client = mock(ClickHouseClient.class);
            var localMetrics = new IngestionMetrics();
            var local = new ClickHouseTelemetryConsumer(client, objectMapper, localMetrics, 3, 0);
            doThrow(new TelemetryConsumerFailure.InsertFailure(reason, 500)).when(client).insertNumericBatch(anyList());
            var ack = mock(Acknowledgment.class);
            var failure = assertThrows(RuntimeException.class, () -> local.consume(validRecord(), ack));
            verify(client, times(1)).insertNumericBatch(anyList());
            verifyNoInteractions(ack);
            assertEquals(reason.reasonCode, localMetrics.getConsumerDegradedReason());
            assertNoSecretInThrowableChain(failure, "unused");
        }
        doThrow(new TelemetryConsumerFailure.InsertFailure(TelemetryConsumerFailure.DEPENDENCY, 503))
                .doNothing().when(clickHouseClient).insertNumericBatch(anyList());
        var ack = mock(Acknowledgment.class);
        consumer.consume(validRecord(), ack);
        verify(clickHouseClient, times(2)).insertNumericBatch(anyList());
        verify(ack).acknowledge();
    }

    @Test
    void ambiguousCommitAfterPersistenceNeverReinserts() throws Exception {
        var ack = mock(Acknowledgment.class);
        doThrow(new org.apache.kafka.common.errors.TimeoutException(SUPER_SECRET_SENTINEL_DO_NOT_LOG))
                .when(ack).acknowledge();
        var failure = assertThrows(RuntimeException.class, () -> consumer.consume(validRecord(), ack));
        verify(clickHouseClient, times(1)).insertNumericBatch(anyList());
        verify(ack, times(1)).acknowledge();
        assertEquals(TelemetryConsumerFailure.ACK_UNCERTAIN.reasonCode, metrics.getConsumerDegradedReason());
        assertNoSecretInThrowableChain(failure, "unused");
    }

    @Test
    void interruptedInsertionIsShutdownWithoutRetry() throws Exception {
        doThrow(new InterruptedException(SUPER_SECRET_SENTINEL_DO_NOT_LOG))
                .when(clickHouseClient).insertNumericBatch(anyList());
        var ack = mock(Acknowledgment.class);
        try {
            var failure = assertThrows(RuntimeException.class, () -> consumer.consume(validRecord(), ack));
            assertTrue(Thread.currentThread().isInterrupted());
            assertEquals(TelemetryConsumerFailure.SHUTDOWN.reasonCode, metrics.getConsumerDegradedReason());
            assertNoSecretInThrowableChain(failure, "unused");
            verify(clickHouseClient, times(1)).insertNumericBatch(anyList());
            verifyNoInteractions(ack);
        } finally {
            Thread.interrupted();
        }
    }

    @Test
    void missingAcknowledgmentCannotDeclareRecovery() throws Exception {
        assertThrows(RuntimeException.class, () -> consumer.consume(validRecord(), null));
        verify(clickHouseClient, times(1)).insertNumericBatch(anyList());
        assertEquals(TelemetryConsumerFailure.ACK_UNCERTAIN.reasonCode, metrics.getConsumerDegradedReason());
    }

    private ConsumerRecord<String, String> validRecord() throws Exception {
        return new ConsumerRecord<>("telemetry.numeric.v1", 0, 100L, "key",
                objectMapper.writeValueAsString(new CanonicalObservation(
                        "tenant-1", "entity-1", "oid", "sensor", Instant.now(), 1, 1, 0, "1", "snmp")));
    }

    @Test
    void exhaustionPreservesSanitizedCausalEvidence() throws Exception {
        var source = new java.net.ConnectException(SUPER_SECRET_SENTINEL_DO_NOT_LOG);
        source.addSuppressed(new IllegalStateException(SUPER_SECRET_SENTINEL_DO_NOT_LOG));
        doThrow(source).when(clickHouseClient).insertNumericBatch(anyList());
        var record = new ConsumerRecord<String, String>("telemetry.numeric.v1", 0, 100L, "key",
                objectMapper.writeValueAsString(new CanonicalObservation(
                        "tenant-1", "entity-1", "oid", "sensor", Instant.now(), 1, 1, 0, "1", "snmp")));
        var failure = assertThrows(RuntimeException.class, () -> consumer.consume(record, mock(Acknowledgment.class)));
        assertNotNull(failure.getCause(), "retain sanitized cause rather than discard diagnosis");
        assertTrue(failure.getCause().getCause().getMessage().contains("java.net.ConnectException"));
        assertArrayEquals(java.util.Arrays.copyOf(source.getStackTrace(), Math.min(source.getStackTrace().length, 32)),
                failure.getCause().getCause().getStackTrace());
        assertNoSecretInThrowableChain(failure, record.value());
        assertEquals(0, failure.getCause().getCause().getSuppressed().length);
    }

    @Test
    void retryArithmeticSaturatesWithoutWrappingAndAcceptsZeroDelay() {
        assertEquals(0, ClickHouseTelemetryConsumer.retryBackoffMs(0, Integer.MAX_VALUE));
        assertEquals(1001, ClickHouseTelemetryConsumer.retryBackoffMs(1001, 1));
        assertEquals(8008, ClickHouseTelemetryConsumer.retryBackoffMs(1001, 4));
        assertEquals(1L << 62, ClickHouseTelemetryConsumer.retryBackoffMs(1, 63));
        for (long attempt : new long[] {2, 63, 64, 65, Integer.MAX_VALUE}) {
            assertEquals(Long.MAX_VALUE, ClickHouseTelemetryConsumer.retryBackoffMs(Long.MAX_VALUE, attempt));
        }
        assertEquals(Long.MAX_VALUE, ClickHouseTelemetryConsumer.retryBackoffMs(1, 64));
        assertEquals(Long.MAX_VALUE, ClickHouseTelemetryConsumer.retryBackoffMs(1, 65));
    }

    @Test
    void zeroDelayCanExhaustMoreThanFiveAttemptsWithoutAcknowledging() throws Exception {
        var extended = new ClickHouseTelemetryConsumer(clickHouseClient, objectMapper, metrics, 7, 0);
        doThrow(new java.net.ConnectException("not available")).when(clickHouseClient).insertNumericBatch(anyList());
        var ack = mock(Acknowledgment.class);
        var record = new ConsumerRecord<String, String>("telemetry.numeric.v1", 0, 100L, "key",
                objectMapper.writeValueAsString(new CanonicalObservation(
                        "tenant-1", "entity-1", "oid", "sensor", Instant.now(), 1, 1, 0, "1", "snmp")));
        assertThrows(RuntimeException.class, () -> extended.consume(record, ack));
        verify(clickHouseClient, times(7)).insertNumericBatch(anyList());
        verifyNoInteractions(ack);
        assertTrue(metrics.isConsumerDegraded());
    }

    @Test
    void interruptedLargeBackoffRetainsSafeCauseAndDoesNotAcknowledge() throws Exception {
        var extended = new ClickHouseTelemetryConsumer(clickHouseClient, objectMapper, metrics, 2, 1001);
        doThrow(new java.net.ConnectException(SUPER_SECRET_SENTINEL_DO_NOT_LOG)).when(clickHouseClient).insertNumericBatch(anyList());
        var ack = mock(Acknowledgment.class);
        var record = new ConsumerRecord<String, String>("telemetry.numeric.v1", 0, 100L, "key",
                objectMapper.writeValueAsString(new CanonicalObservation(
                        "tenant-1", "entity-1", "oid", "sensor", Instant.now(), 1, 1, 0, "1", "snmp")));
        Thread.currentThread().interrupt();
        try {
            var failure = assertThrows(RuntimeException.class, () -> extended.consume(record, ack));
            assertTrue(Thread.currentThread().isInterrupted());
            assertTrue(failure.getCause().getMessage().contains("InterruptedException"));
            assertNoSecretInThrowableChain(failure, record.value());
            verify(clickHouseClient, times(1)).insertNumericBatch(anyList());
            verifyNoInteractions(ack);
            assertTrue(metrics.isConsumerDegraded());
        } finally {
            Thread.interrupted();
        }
    }

    @Test
    void testSpringContextBeanInstantiation() {
        org.springframework.context.annotation.AnnotationConfigApplicationContext context =
                new org.springframework.context.annotation.AnnotationConfigApplicationContext();
        context.getDefaultListableBeanFactory().registerSingleton("clickHouseClient", clickHouseClient);
        context.getDefaultListableBeanFactory().registerSingleton("objectMapper", objectMapper);
        context.getDefaultListableBeanFactory().registerSingleton("ingestionMetrics", metrics);
        context.registerBean(KafkaProperties.class, KafkaProperties::new);
        context.register(IngestionKafkaConfiguration.class, ClickHouseTelemetryConsumer.class);
        assertDoesNotThrow(context::refresh);
        assertNotNull(context.getBean(ClickHouseTelemetryConsumer.class));
        context.close();
    }
}
