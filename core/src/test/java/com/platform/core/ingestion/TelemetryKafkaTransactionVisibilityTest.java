package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.apache.kafka.clients.consumer.Consumer;
import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.consumer.OffsetAndMetadata;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.common.TopicPartition;
import org.apache.kafka.common.serialization.StringDeserializer;
import org.apache.kafka.common.serialization.StringSerializer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.springframework.boot.autoconfigure.kafka.KafkaProperties;
import org.springframework.kafka.config.ConcurrentKafkaListenerContainerFactory;
import org.springframework.kafka.core.DefaultKafkaProducerFactory;
import org.springframework.kafka.core.DefaultKafkaConsumerFactory;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;
import org.springframework.context.annotation.Configuration;
import org.springframework.core.env.MapPropertySource;
import org.springframework.kafka.annotation.EnableKafka;
import org.springframework.kafka.config.KafkaListenerEndpointRegistry;
import org.springframework.kafka.listener.MessageListenerContainer;
import org.springframework.kafka.listener.ContainerProperties;
import org.springframework.kafka.test.EmbeddedKafkaBroker;
import org.springframework.kafka.test.EmbeddedKafkaKraftBroker;

import java.time.Instant;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.junit.jupiter.api.Assertions.fail;
import static org.mockito.ArgumentMatchers.anyList;
import static org.mockito.Mockito.doAnswer;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;

/**
 * Isolated embedded KRaft broker evidence that aborted transactional records are
 * invisible to the Platform-pinned {@code read_committed} telemetry listener path,
 * that offsets commit only after ClickHouse persistence, and that poison/ClickHouse
 * failure does not silently advance the group offset.
 * Does not touch the live Compose Kafka broker, volumes or identity.
 */
class TelemetryKafkaTransactionVisibilityTest {

    private static final String TOPIC = KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC;
    private static final TopicPartition PARTITION = new TopicPartition(TOPIC, 0);

    private EmbeddedKafkaBroker embeddedKafka;
    private DefaultKafkaProducerFactory<String, String> producerFactory;
    private AnnotationConfigApplicationContext context;

    @Configuration
    @EnableKafka
    static class ListenerInfrastructure { }


    @BeforeEach
    void startBroker() {
        EmbeddedKafkaKraftBroker broker = new EmbeddedKafkaKraftBroker(1, 1, TOPIC);
        broker.brokerProperty("transaction.state.log.replication.factor", "1");
        broker.brokerProperty("transaction.state.log.min.isr", "1");
        broker.afterPropertiesSet();
        this.embeddedKafka = broker;
    }

    @AfterEach
    void stopBroker() {
        if (context != null) { context.close(); context = null; }
        // Deterministic producer cleanup before broker teardown (success, failure, timeout).
        destroyProducerFactory();
        if (embeddedKafka != null) {
            embeddedKafka.destroy();
            embeddedKafka = null;
        }
    }

    @Test
    @Timeout(value = 120, unit = TimeUnit.SECONDS)
    void lateHttp200ExceptionRetainsBrokerOffsetUntilExplicitReplaySucceeds() throws Exception {
        var reject = new java.util.concurrent.atomic.AtomicBoolean(false);
        var payloads = new CopyOnWriteArrayList<String>();
        var server = com.sun.net.httpserver.HttpServer.create(new java.net.InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", exchange -> {
            payloads.add(new String(exchange.getRequestBody().readAllBytes(), java.nio.charset.StandardCharsets.UTF_8));
            if (reject.get()) {
                exchange.sendResponseHeaders(200, 0);
                exchange.getResponseBody().flush();
                exchange.getResponseBody().write("Code: 60. DB::Exception: PRIVATE_SENTINEL".getBytes());
            } else {
                exchange.sendResponseHeaders(200, -1);
            }
            exchange.close();
        });
        server.start();
        try {
            var bootstrap = embeddedKafka.getBrokersAsString();
            var mapper = new ObjectMapper().findAndRegisterModules();
            var metrics = new IngestionMetrics();
            var client = new ClickHouseClient("127.0.0.1", server.getAddress().getPort(), "user", "secret", mapper, metrics);
            var group = "k11-late-" + UUID.randomUUID();
            var template = createTransactionalTemplate(bootstrap);
            context = createListenerContext(bootstrap, group, client, mapper, metrics);
            var listener = registeredListener();
            listener.start();
            String baseline = mapper.writeValueAsString(observation("baseline"));
            template.executeInTransaction(ops -> {
                sendSync(ops, TOPIC, "same-key", baseline);
                return null;
            });
            Long before = awaitCommittedOffset(bootstrap, group, n -> n != null && n == 1, 30);
            assertNotNull(before);
            assertEquals(1, metrics.getClickHouseInserts());
            reject.set(true);
            String retained = mapper.writeValueAsString(observation("retained"));
            template.executeInTransaction(ops -> { sendSync(ops, TOPIC, "same-key", retained); return null; });
            awaitCondition(() -> !listener.isRunning(), 30, "late exception must stop registered listener");
            awaitCondition(() -> metrics.getConsumerTerminalStop() != null
                    && metrics.getConsumerTerminalStop().stopCompleted(), 30, "observe unknown-failure stop completion");
            assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
            assertEquals(before, committedOffset(bootstrap, group), "late HTTP error cannot commit Kafka offset");
            assertEquals(1, metrics.getClickHouseInserts(), "only the initial genuine success counts");
            assertEquals(1, metrics.getClickHouseFailures());
            assertEquals("ERR_CLICKHOUSE_UNKNOWN", metrics.getConsumerDegradedReason());
            assertEquals(2, payloads.size(), "UNKNOWN must not trigger insertion retries");
            reject.set(false);
            listener.start(); // Explicit operator recovery only; K2 is not implemented.
            Long after = awaitCommittedOffset(bootstrap, group, n -> n != null && n > before, 30);
            assertNotNull(after);
            assertEquals(3L, after); // Second data record is offset 2, following the first transaction marker.
            assertEquals(2, metrics.getClickHouseInserts());
            assertEquals(payloads.get(1), payloads.get(2), "replay must preserve exact insertion identity/body");
            System.out.printf("K11_OFFSET before=%d late_error=%d explicit_replay=%d inserts=2 failures=1%n",
                    before, before, after);
        } finally {
            if (context != null) { context.close(); context = null; }
            server.stop(0);
        }
    }

    @Test
    @Timeout(value = 180, unit = TimeUnit.SECONDS)
    void actualListenerContainerHonorsReadCommittedAckAndFailClosedOffsets() throws Exception {
        String bootstrap = embeddedKafka.getBrokersAsString();
        ObjectMapper objectMapper = new ObjectMapper().findAndRegisterModules();

        KafkaTemplate<String, String> kafkaTemplate = createTransactionalTemplate(bootstrap);
        String groupId = "issue222-listener-" + UUID.randomUUID();
        ClickHouseClient clickHouseClient = mock(ClickHouseClient.class);
        IngestionMetrics metrics = new IngestionMetrics();
        List<String> deliveredSequenceIds = new CopyOnWriteArrayList<>();
        CountDownLatch committedInsertLatch = new CountDownLatch(1);
        CountDownLatch failedAttemptBarrier = new CountDownLatch(1);
        CountDownLatch releaseFailure = new CountDownLatch(1);
        CountDownLatch recoveryBarrier = new CountDownLatch(1);
        CountDownLatch allowRecovery = new CountDownLatch(1);
        CountDownLatch redeliverySuccessLatch = new CountDownLatch(1);
        AtomicInteger redeliveryInsertAttempts = new AtomicInteger();

        CanonicalObservation abortedObs = observation("seq-aborted-" + UUID.randomUUID());
        CanonicalObservation committedObs = observation("seq-committed-" + UUID.randomUUID());
        CanonicalObservation redeliveryObs = observation("seq-redeliver-" + UUID.randomUUID());
        String abortedJson = objectMapper.writeValueAsString(abortedObs);
        String committedJson = objectMapper.writeValueAsString(committedObs);
        String redeliveryJson = objectMapper.writeValueAsString(redeliveryObs);
        String poisonPayload = "{\"not\":\"json\"";

        doAnswer(invocation -> {
            @SuppressWarnings("unchecked")
            List<CanonicalObservation> batch = invocation.getArgument(0);
            String seq = batch.getFirst().sequenceId();
            deliveredSequenceIds.add(seq);
            if (committedObs.sequenceId().equals(seq)) {
                committedInsertLatch.countDown();
                return null;
            }
            if (redeliveryObs.sequenceId().equals(seq)) {
                int attempt = redeliveryInsertAttempts.incrementAndGet();
                if (attempt == 1) {
                    throw new java.net.ConnectException("intentional transient ClickHouse failure");
                }
                if (attempt == 2) {
                    failedAttemptBarrier.countDown();
                    assertTrue(releaseFailure.await(30, TimeUnit.SECONDS), "release failed insertion");
                    throw new java.net.ConnectException("intentional exhausted ClickHouse failure");
                }
                recoveryBarrier.countDown();
                assertTrue(allowRecovery.await(30, TimeUnit.SECONDS), "allow recovered insertion");
                redeliverySuccessLatch.countDown();
                return null;
            }
            throw new RuntimeException("unexpected observation sequenceId=" + seq);
        }).when(clickHouseClient).insertNumericBatch(anyList());

        context = createListenerContext(bootstrap, groupId, clickHouseClient, objectMapper, metrics);
        var listenerFactory = context.getBean(
                IngestionKafkaConfiguration.TELEMETRY_KAFKA_LISTENER_CONTAINER_FACTORY,
                ConcurrentKafkaListenerContainerFactory.class);
        assertEquals(ContainerProperties.AckMode.MANUAL_IMMEDIATE,
                listenerFactory.getContainerProperties().getAckMode());
        Map<String, Object> effectiveConsumer = listenerFactory.getConsumerFactory().getConfigurationProperties();
        assertEquals("read_committed", effectiveConsumer.get(ConsumerConfig.ISOLATION_LEVEL_CONFIG));
        assertEquals(false, effectiveConsumer.get(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG));
        MessageListenerContainer container = registeredListener();
        assertEquals(groupId, container.getContainerProperties().getGroupId());
        assertEquals(List.of(TOPIC), List.of(container.getContainerProperties().getTopics()));
        assertFalse(container.isRunning(), "annotation autoStartup=false is resolved");
        assertEquals(org.springframework.boot.actuate.health.Status.UNKNOWN,
                context.getBean(TelemetryConsumerHealthIndicator.class).health().getStatus(),
                "intentionally disabled startup is not an abnormal failure");
        assertEquals("DISABLED", context.getBean(TelemetryConsumerHealthIndicator.class).health().getDetails().get("status"));

        try {
            // 1/2 — abort then commit; aborted must be invisible to the real listener.
            try {
                kafkaTemplate.executeInTransaction(ops -> {
                    sendSync(ops, TOPIC, "abort-key", abortedJson);
                    throw new IllegalStateException("intentional abort for issue222 visibility proof");
                });
                fail("aborted transaction must throw");
            } catch (Exception expected) {
                assertTrue(
                        containsMessage(expected, "intentional abort"),
                        "expected intentional abort; got: " + expected
                );
            }

            kafkaTemplate.executeInTransaction(ops -> {
                sendSync(ops, TOPIC, "commit-key", committedJson);
                return true;
            });

            container.start();
            assertTrue(
                    committedInsertLatch.await(45, TimeUnit.SECONDS),
                    "committed numeric record must be consumed by the real listener; delivered="
                            + deliveredSequenceIds
            );
            assertFalse(
                    deliveredSequenceIds.contains(abortedObs.sequenceId()),
                    "aborted transactional record must not reach ClickHouse; delivered="
                            + deliveredSequenceIds
            );
            assertTrue(deliveredSequenceIds.contains(committedObs.sequenceId()));

            // 3/4 — ClickHouse insertion occurred; group offset advances only after success.
            verify(clickHouseClient, times(1)).insertNumericBatch(anyList());
            Long afterCommitted = awaitCommittedOffset(
                    bootstrap,
                    groupId,
                    offset -> offset != null && offset > 0,
                    30
            );
            assertNotNull(afterCommitted, "group offset must advance after successful ClickHouse + ACK");
            assertTrue(afterCommitted > 0);
            System.out.println("PR227_OFFSET persisted=" + afterCommitted);

            // 6 — Kafka-level redelivery after ClickHouse failure; offset advances only on success.
            long offsetBeforeRedelivery = afterCommitted;
            kafkaTemplate.executeInTransaction(ops -> {
                sendSync(ops, TOPIC, "redeliver-key", redeliveryJson);
                return true;
            });

            assertTrue(failedAttemptBarrier.await(30, TimeUnit.SECONDS));
            assertTrue(metrics.isConsumerDegraded(), "dependency failure remains visible during bounded retry");
            // Attempt one has failed; attempt two cannot succeed or ACK at this barrier.
            assertEquals(offsetBeforeRedelivery, committedOffset(bootstrap, groupId));
            System.out.println("PR227_OFFSET failing=" + committedOffset(bootstrap, groupId)
                    + " recoveryPermitted=false");
            releaseFailure.countDown();
            awaitCondition(() -> !registeredListener().isRunning()
                    && !registeredListener().isInExpectedState(), 30, "exhaustion must stop abnormally");
            awaitCondition(() -> metrics.getConsumerTerminalStop() != null
                    && metrics.getConsumerTerminalStop().stopCompleted(), 30, "observe dependency stop completion");
            assertEquals("RECOVERABLE_DEPENDENCY_STOP", metrics.getConsumerTerminalStop().status());
            assertEquals(TelemetryConsumerFailure.DEPENDENCY, metrics.getConsumerTerminalStop().failure().reason());
            assertEquals("RECOVERABLE_DEPENDENCY_STOP", context.getBean(TelemetryConsumerHealthIndicator.class)
                    .health().getDetails().get("terminalStopStatus"));
            assertTrue(metrics.isConsumerDegraded());
            assertEquals(2, redeliveryInsertAttempts.get(), "bounded local attempts, no Kafka retry loop");
            assertEquals(offsetBeforeRedelivery, committedOffset(bootstrap, groupId));
            Thread.sleep(1_500);
            assertEquals(2, redeliveryInsertAttempts.get(), "exhaustion stays stopped until operator restart");
            System.out.println("PR227_EXHAUSTION attempts=" + redeliveryInsertAttempts.get()
                    + " running=" + registeredListener().isRunning()
                    + " expectedState=" + registeredListener().isInExpectedState()
                    + " offset=" + committedOffset(bootstrap, groupId));
            context.close();
            context = null;
            assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status(), "shutdown vetoes recovery candidate");
            assertEquals(offsetBeforeRedelivery, committedOffset(bootstrap, groupId),
                    "shutdown must not commit the failed record");
            System.out.println("PR227_OFFSET shutdown=" + committedOffset(bootstrap, groupId));

            // A fresh annotated consumer, same group, must replay the uncommitted record.
            context = createListenerContext(bootstrap, groupId, clickHouseClient, objectMapper, metrics);
            container = registeredListener();
            container.start();
            assertTrue(recoveryBarrier.await(30, TimeUnit.SECONDS));
            assertEquals(3, redeliveryInsertAttempts.get());
            assertEquals(offsetBeforeRedelivery, committedOffset(bootstrap, groupId),
                    "restart/redelivery must not ACK before persistence returns");
            System.out.println("PR227_OFFSET restartedBeforePersistence=" + committedOffset(bootstrap, groupId));
            allowRecovery.countDown();
            assertTrue(redeliverySuccessLatch.await(30, TimeUnit.SECONDS));
            Long afterRedelivery = awaitCommittedOffset(
                    bootstrap,
                    groupId,
                    offset -> offset != null && offset > offsetBeforeRedelivery,
                    30
            );
            assertNotNull(afterRedelivery, "offset must advance only after successful redelivery processing");
            assertTrue(afterRedelivery > offsetBeforeRedelivery);
            awaitCondition(() -> !metrics.isConsumerDegraded(), 30, "successful persistence clears degradation after ACK");
            assertEquals(org.springframework.boot.actuate.health.Status.UP,
                    context.getBean(TelemetryConsumerHealthIndicator.class).health().getStatus());
            System.out.println("PR227_OFFSET recovered=" + afterRedelivery);

            // 5 — poison/ClickHouse-path failure must not silently commit an unprocessed offset.
            long offsetBeforePoison = afterRedelivery;
            kafkaTemplate.executeInTransaction(ops -> {
                sendSync(ops, TOPIC, "poison-key", poisonPayload);
                return true;
            });

            awaitCondition(() -> metrics.getConsumerPoisonEvents() == 1
                    && !registeredListener().isRunning() && !registeredListener().isInExpectedState(),
                    30, "poison must stop the annotated listener abnormally");
            awaitCondition(() -> metrics.getConsumerTerminalStop() != null
                    && metrics.getConsumerTerminalStop().stopCompleted(), 30, "observe poison stop completion");
            assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
            assertEquals(TelemetryConsumerFailure.POISON, metrics.getConsumerTerminalStop().failure().reason());
            assertTrue(metrics.isConsumerDegraded());
            assertEquals(com.platform.core.logging.CoreSupportabilityEventCodes.TELEMETRY_POISON_EVENT,
                    metrics.getConsumerDegradedReason());
            assertEquals(offsetBeforePoison, committedOffset(bootstrap, groupId));
            Thread.sleep(1_500);
            assertEquals(1, metrics.getConsumerPoisonEvents(), "no automatic restart or poison error storm");
            // Later valid traffic must remain blocked behind the retained poison record.
            kafkaTemplate.executeInTransaction(ops -> {
                sendSync(ops, TOPIC, "blocked-key", committedJson);
                return true;
            });
            // Explicit operator restart retries the same poison; it never skips to later data.
            context.close();
            context = null;
            context = createListenerContext(bootstrap, groupId, clickHouseClient, objectMapper, metrics);
            registeredListener().start();
            awaitCondition(() -> metrics.getConsumerPoisonEvents() == 2
                    && !registeredListener().isRunning() && !registeredListener().isInExpectedState(), 30, "poison replay on explicit restart");
            assertEquals(offsetBeforePoison, committedOffset(bootstrap, groupId));
            System.out.println("PR227_POISON events=" + metrics.getConsumerPoisonEvents()
                    + " offset=" + committedOffset(bootstrap, groupId) + " laterRecordBlocked=true");
            verify(clickHouseClient, times(4)).insertNumericBatch(anyList());
            // Still no aborted observation visibility after subsequent traffic.
            assertFalse(deliveredSequenceIds.contains(abortedObs.sequenceId()));
        } finally {
            releaseFailure.countDown();
            allowRecovery.countDown();
            if (context != null) { context.close(); context = null; }
            // Ensure producer resources are released even if assertions fail before @AfterEach.
            destroyProducerFactory();
        }
    }

    @Test
    @Timeout(value = 120, unit = TimeUnit.SECONDS)
    void incompleteStopThenOperatorRestartCannotRetainOldTerminalAuthority() throws Exception {
        assertRestartGenerationIsolation(false);
    }

    @Test
    @Timeout(value = 120, unit = TimeUnit.SECONDS)
    void registeredTerminalStopRetainsUnprovenInsertAcrossFinalReasonChange() throws Exception {
        var mapper = new ObjectMapper().findAndRegisterModules();
        var metrics = new IngestionMetrics();
        var client = mock(ClickHouseClient.class);
        org.mockito.Mockito.doThrow(new java.io.EOFException("PRIVATE_RESPONSE_SENTINEL"))
                .doThrow(new TelemetryConsumerFailure.InsertFailure(TelemetryConsumerFailure.SCHEMA, 422))
                .when(client).insertNumericBatch(anyList());
        String bootstrap = embeddedKafka.getBrokersAsString();
        String group = "k2b-completion-" + UUID.randomUUID();
        context = createListenerContext(bootstrap, group, client, mapper, metrics);
        registeredListener().start();
        String valid = mapper.writeValueAsString(observation("retained-completion"));
        createTransactionalTemplate(bootstrap).executeInTransaction(ops -> {
            sendPartition(ops, 0, "failed", valid);
            sendPartition(ops, 0, "following", valid);
            return true;
        });
        awaitCondition(() -> metrics.getConsumerTerminalStop() != null
                && metrics.getConsumerTerminalStop().stopCompleted(), 30, "observe actual terminal stop completion");
        assertFalse(registeredListener().isRunning());
        assertEquals(null, committedOffset(bootstrap, group), "unproven INSERT and following record cannot advance offset");
        verify(client, times(2)).insertNumericBatch(anyList());
        assertEquals(TelemetryConsumerFailure.SCHEMA, metrics.getConsumerTerminalStop().failure().reason());
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
        var details = context.getBean(TelemetryConsumerHealthIndicator.class).health().getDetails();
        assertEquals(Boolean.TRUE, details.get("terminalPersistenceUncertain"));
        assertEquals(Boolean.FALSE, details.get("terminalInsertCompleted"));
        assertFalse(details.toString().contains("PRIVATE_RESPONSE_SENTINEL"));
    }

    @org.junit.jupiter.params.ParameterizedTest
    @org.junit.jupiter.params.provider.CsvSource({"1,false", "1,true", "2,false", "2,true"})
    @Timeout(value = 120, unit = TimeUnit.SECONDS)
    void retiredStartsBeforeCurrentStartsCannotSuppressRegisteredFailStop(int concurrency, boolean oldAuth) throws Exception {
        embeddedKafka.destroy();
        var broker = new EmbeddedKafkaKraftBroker(1, concurrency, TOPIC);
        broker.brokerProperty("transaction.state.log.replication.factor", "1");
        broker.brokerProperty("transaction.state.log.min.isr", "1");
        broker.afterPropertiesSet();
        embeddedKafka = broker;
        String bootstrap = broker.getBrokersAsString();
        var mapper = new ObjectMapper().findAndRegisterModules();
        var metrics = new IngestionMetrics();
        var client = mock(ClickHouseClient.class);
        var group = "k2a-startup-" + UUID.randomUUID();
        context = createListenerContext(bootstrap, group, client, mapper, metrics);
        var container = (org.springframework.kafka.listener.ConcurrentMessageListenerContainer<?, ?>) registeredListener();
        container.setConcurrency(concurrency);
        var queuedStops = new java.util.concurrent.LinkedBlockingQueue<Runnable>();
        container.setCommonErrorHandler(IngestionKafkaConfiguration.terminalStoppingErrorHandler(metrics, queuedStops::add));
        container.start();
        awaitCondition(() -> container.getAssignedPartitions() != null
                && container.getAssignedPartitions().size() == concurrency, 30, "old generation assigned");
        var retiredChildren = List.copyOf(container.getContainers());
        var retiredPublisher = container.getApplicationEventPublisher();
        container.stop();
        var injected = new java.util.concurrent.atomic.AtomicBoolean();
        var factory = context.getBean(IngestionKafkaConfiguration.TELEMETRY_KAFKA_CONSUMER_FACTORY,
                org.springframework.kafka.core.ConsumerFactory.class);
        factory.addListener(new org.springframework.kafka.core.ConsumerFactory.Listener<String, String>() {
            @Override
            public void consumerAdded(String id, Consumer<String, String> consumer) {
                if (!injected.compareAndSet(false, true)) return;
                // Spring creates the real consumer before its child publishes Starting,
                // while the parent has not yet reconciled the new child collection.
                for (var retired : retiredChildren) {
                    retiredPublisher.publishEvent(new org.springframework.kafka.event.ConsumerStartingEvent(retired, container));
                    if (oldAuth) retiredPublisher.publishEvent(new org.springframework.kafka.event.ConsumerStoppedEvent(
                            retired, container, org.springframework.kafka.event.ConsumerStoppedEvent.Reason.AUTH));
                }
            }
        });
        container.start();
        assertTrue(injected.get(), "retired publication injected during actual child construction");
        awaitCondition(() -> container.getAssignedPartitions() != null
                && container.getAssignedPartitions().size() == concurrency, 30, "current generation assigned");
        Long before = committedOffset(bootstrap, group);
        var template = createTransactionalTemplate(bootstrap);
        String followingValid = mapper.writeValueAsString(observation("following-valid"));
        template.executeInTransaction(ops -> {
            sendPartition(ops, 0, "failed", "{invalid");
            sendPartition(ops, 0, "following-valid", followingValid);
            return true;
        });
        var stop = queuedStops.poll(30, TimeUnit.SECONDS);
        assertNotNull(stop, "actual error handler submitted its physical stopping action");
        stop.run();
        assertFalse(container.isRunning(), "current failed consumer must physically stop, not discard a null token");
        assertNotNull(metrics.getConsumerTerminalStop(), "current start/thread must retain terminal attribution");
        awaitCondition(() -> metrics.getConsumerTerminalStop().stopCompleted(), 30, "all current children completed");
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status(), "poison is never recovery authority");
        assertTrue(metrics.isConsumerDegraded());
        assertEquals(before, committedOffset(bootstrap, group), "failed then valid cannot commit past failure");
        verify(client, org.mockito.Mockito.never()).insertNumericBatch(anyList());
    }

    @Test
    @Timeout(value = 120, unit = TimeUnit.SECONDS)
    void queuedOldStopCannotStopNewRegisteredListenerGeneration() throws Exception {
        assertRestartGenerationIsolation(true);
    }

    private void assertRestartGenerationIsolation(boolean exerciseQueuedAction) throws Exception {
        embeddedKafka.destroy();
        var broker = new EmbeddedKafkaKraftBroker(1, 2, TOPIC);
        broker.brokerProperty("transaction.state.log.replication.factor", "1");
        broker.brokerProperty("transaction.state.log.min.isr", "1");
        broker.afterPropertiesSet();
        embeddedKafka = broker;
        String bootstrap = broker.getBrokersAsString();
        var mapper = new ObjectMapper().findAndRegisterModules();
        var metrics = new IngestionMetrics();
        var group = "k2a-generation-" + UUID.randomUUID();
        context = createListenerContext(bootstrap, group, mock(ClickHouseClient.class), mapper, metrics);
        var container = (org.springframework.kafka.listener.ConcurrentMessageListenerContainer<?, ?>) registeredListener();
        container.setConcurrency(2);
        var queuedStops = new java.util.concurrent.LinkedBlockingQueue<Runnable>();
        container.setCommonErrorHandler(IngestionKafkaConfiguration.terminalStoppingErrorHandler(metrics, queuedStops::add));
        var dropOneCompletion = new java.util.concurrent.atomic.AtomicBoolean(true);
        var observedCompletions = new AtomicInteger();
        container.start();
        awaitCondition(() -> container.getContainers().size() == 2
                && container.getAssignedPartitions() != null && container.getAssignedPartitions().size() == 2,
                30, "first registered generation owns both partitions");
        var oldChildren = List.copyOf(container.getContainers());
        for (var child : oldChildren) {
            var inheritedPublisher = child.getApplicationEventPublisher();
            child.setApplicationEventPublisher(event -> {
                if (event instanceof org.springframework.kafka.event.ConsumerStoppedEvent) {
                    observedCompletions.incrementAndGet();
                    // Drop at the real child boundary before the stamped provenance publisher.
                    if (dropOneCompletion.compareAndSet(true, false)) return;
                }
                inheritedPublisher.publishEvent(event);
            });
        }
        createTransactionalTemplate(bootstrap).executeInTransaction(ops -> {
            sendPartition(ops, 0, "poison", "{invalid");
            return true;
        });
        var oldStop = queuedStops.poll(30, TimeUnit.SECONDS);
        assertNotNull(oldStop, "hold an actual typed handler stop before executor delivery");
        long oldEpoch = metrics.getConsumerTerminalStop().failure().stopEpoch();
        var retainedOffset = committedOffset(bootstrap, group);
        container.stop(); // Explicit operator stop; both consumers close, one completion is unobserved.
        awaitCondition(() -> observedCompletions.get() == 2, 30, "actual old consumers closed");
        assertFalse(metrics.getConsumerTerminalStop().stopCompleted(), "missing completion is not proof");
        assertTrue(metrics.isConsumerDegraded(), "operator stop must retain blocked poison evidence");
        container.start(); // The real parent constructs fresh child-container objects.
        assertTrue(container.getContainers().stream().noneMatch(oldChildren::contains));
        var newStop = queuedStops.poll(30, TimeUnit.SECONDS);
        assertNotNull(newStop, "new generation replays retained poison through the registered listener");
        assertTrue(metrics.isConsumerDegraded(), "restart alone is not persistence plus ACK");
        assertEquals(retainedOffset, committedOffset(bootstrap, group));
        if (exerciseQueuedAction) {
            oldStop.run();
            assertTrue(container.isRunning(), "queued previous-generation stop must not stop the new generation");
        } else {
            var current = metrics.getConsumerTerminalStop();
            assertNotNull(current);
            assertTrue(current.failure().stopEpoch() > oldEpoch,
                    "old incomplete terminal authority must not masquerade as the new generation");
            assertFalse(current.stopCompleted());
            assertEquals(TelemetryConsumerFailure.POISON, current.failure().reason());
        }
    }

    @Test
    @Timeout(value = 180, unit = TimeUnit.SECONDS)
    void stoppedListenerHealthSurvivesConcurrentSuccessfulPartition() throws Exception {
        embeddedKafka.destroy();
        var broker = new EmbeddedKafkaKraftBroker(1, 2, TOPIC);
        broker.brokerProperty("transaction.state.log.replication.factor", "1");
        broker.brokerProperty("transaction.state.log.min.isr", "1");
        broker.afterPropertiesSet();
        embeddedKafka = broker;
        String bootstrap = broker.getBrokersAsString();
        var mapper = new ObjectMapper().findAndRegisterModules();
        var client = mock(ClickHouseClient.class);
        var metrics = new IngestionMetrics();
        var insertionEntered = new CountDownLatch(1);
        var releaseInsertion = new CountDownLatch(1);
        var insertionReturned = new CountDownLatch(1);
        doAnswer(invocation -> {
            insertionEntered.countDown();
            assertTrue(releaseInsertion.await(30, TimeUnit.SECONDS));
            insertionReturned.countDown();
            return null;
        }).when(client).insertNumericBatch(anyList());
        context = createListenerContext(bootstrap, "health-race-" + UUID.randomUUID(), client, mapper, metrics);
        var container = (org.springframework.kafka.listener.ConcurrentMessageListenerContainer<?, ?>) registeredListener();
        container.setConcurrency(2);
        var childrenStopped = new CountDownLatch(2);
        context.addApplicationListener(event -> {
            if (event instanceof org.springframework.kafka.event.ConsumerStoppedEvent stopped
                    && stopped.getContainer(MessageListenerContainer.class) == container) {
                childrenStopped.countDown();
            }
        });
        var template = createTransactionalTemplate(bootstrap);
        String healthyJson = mapper.writeValueAsString(observation("healthy"));
        try {
            container.start();
            // Total assignment count can briefly be two on the first child while the
            // second joins. Require distinct one-partition ownership before blocking one.
            awaitCondition(() -> container.getContainers().size() == 2
                    && container.getContainers().stream().allMatch(child ->
                            child.getAssignedPartitions() != null && child.getAssignedPartitions().size() == 1)
                    && container.getContainers().stream().flatMap(child -> child.getAssignedPartitions().stream())
                            .collect(java.util.stream.Collectors.toSet())
                            .equals(Set.of(PARTITION, new TopicPartition(TOPIC, 1))),
                    30, "both children must own distinct partitions before the health race");
            template.executeInTransaction(ops -> {
                sendPartition(ops, 0, "healthy", healthyJson);
                return true;
            });
            assertTrue(insertionEntered.await(30, TimeUnit.SECONDS));
            template.executeInTransaction(ops -> {
                sendPartition(ops, 1, "poison", "{invalid");
                return true;
            });
            awaitCondition(() -> metrics.getConsumerPoisonEvents() == 1 && !container.isRunning(),
                    30, "poison stops parent while other partition insertion is in flight");
            releaseInsertion.countDown();
            assertTrue(insertionReturned.await(30, TimeUnit.SECONDS));
            assertTrue(childrenStopped.await(30, TimeUnit.SECONDS), "observe both callbacks finish before health assertion");
            awaitCondition(() -> metrics.getConsumerTerminalStop() != null
                    && metrics.getConsumerTerminalStop().stopCompleted(), 30, "observe all sibling completions");
            assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
            assertTrue(metrics.isConsumerDegraded(), "late sibling success must retain poison degradation");
            assertEquals(com.platform.core.logging.CoreSupportabilityEventCodes.TELEMETRY_POISON_EVENT,
                    metrics.getConsumerDegradedReason());
            System.out.println("PR227_HEALTH_RACE running=" + container.isRunning()
                    + " expectedState=" + container.isInExpectedState() + " metricsDegraded=" + metrics.isConsumerDegraded()
                    + " health=" + context.getBean(TelemetryConsumerHealthIndicator.class).health().getStatus());
            assertEquals(org.springframework.boot.actuate.health.Status.DOWN,
                    context.getBean(TelemetryConsumerHealthIndicator.class).health().getStatus());
        } finally {
            releaseInsertion.countDown();
        }
    }

    @Test
    @Timeout(value = 90, unit = TimeUnit.SECONDS)
    @SuppressWarnings("unchecked")
    void registeredListenerCommitFailureAfterPersistenceDoesNotReinsert() throws Exception {
        String bootstrap = embeddedKafka.getBrokersAsString();
        String group = "ack-failure-" + UUID.randomUUID();
        var mapper = new ObjectMapper().findAndRegisterModules();
        var client = mock(ClickHouseClient.class);
        var metrics = new IngestionMetrics();
        var persisted = new java.util.concurrent.atomic.AtomicBoolean();
        var inserts = new AtomicInteger();
        var commitFailures = new AtomicInteger();
        doAnswer(invocation -> {
            inserts.incrementAndGet();
            persisted.set(true);
            return null;
        }).when(client).insertNumericBatch(anyList());
        context = createListenerContext(bootstrap, group, client, mapper, metrics);
        var factory = (DefaultKafkaConsumerFactory<String, String>) context.getBean(
                IngestionKafkaConfiguration.TELEMETRY_KAFKA_CONSUMER_FACTORY);
        // Keep the real broker consumer; inject only at the synchronous commit boundary.
        factory.addPostProcessor(delegate -> (Consumer<String, String>) java.lang.reflect.Proxy.newProxyInstance(
                Consumer.class.getClassLoader(), new Class<?>[] {Consumer.class}, (proxy, method, args) -> {
                    if (method.getName().equals("commitSync") && persisted.get()) {
                        commitFailures.incrementAndGet();
                        throw new org.apache.kafka.clients.consumer.CommitFailedException("fixture membership loss");
                    }
                    try {
                        return method.invoke(delegate, args);
                    } catch (java.lang.reflect.InvocationTargetException invocation) {
                        throw invocation.getCause();
                    }
                }));
        var container = registeredListener();
        container.start();
        awaitCondition(() -> container.getAssignedPartitions() != null
                && container.getAssignedPartitions().contains(PARTITION), 30, "real partition assigned");
        Long before = committedOffset(bootstrap, group);
        String payload = mapper.writeValueAsString(observation("ack-failure"));
        createTransactionalTemplate(bootstrap).executeInTransaction(ops -> {
            sendSync(ops, TOPIC, "ack-failure", payload);
            return true;
        });
        awaitCondition(() -> commitFailures.get() > 0 && !container.isRunning() && !container.isInExpectedState(),
                30, "commit failure stops registered listener");
        awaitCondition(() -> metrics.getConsumerTerminalStop() != null
                && metrics.getConsumerTerminalStop().stopCompleted(), 30, "observe commit-failure stop completion");
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
        assertEquals(1, inserts.get(), "ACK failure cannot trigger another ClickHouse insertion");
        assertEquals(before, committedOffset(bootstrap, group), "failed commit cannot advance broker offset");
        assertEquals(TelemetryConsumerFailure.MEMBERSHIP_LOST.reasonCode, metrics.getConsumerDegradedReason());
        assertEquals(org.springframework.boot.actuate.health.Status.DOWN,
                context.getBean(TelemetryConsumerHealthIndicator.class).health().getStatus());
        System.out.println("PR227_ACK_FAILURE inserts=" + inserts.get() + " offset=" + committedOffset(bootstrap, group)
                + " reason=" + metrics.getConsumerDegradedReason());
    }

    @Test
    void smallPollIntervalWithLongBackoffRejectedBeforeListenerStarts() {
        var failure = org.junit.jupiter.api.Assertions.assertThrows(Exception.class, () ->
                createListenerContext(embeddedKafka.getBrokersAsString(), "unsafe-poll-budget", mock(ClickHouseClient.class),
                        new ObjectMapper(), new IngestionMetrics(),
                        Map.of(ConsumerConfig.MAX_POLL_INTERVAL_MS_CONFIG, "500"), "2", "2000"));
        assertTrue(containsMessage(failure, "ERR_KAFKA_POLL_BUDGET"));
    }

    @Test
    @Timeout(value = 120, unit = TimeUnit.SECONDS)
    void membershipLossDuringLatePersistenceCannotProduceAck() throws Exception {
        String bootstrap = embeddedKafka.getBrokersAsString();
        String group = "poll-overrun-" + UUID.randomUUID();
        var mapper = new ObjectMapper().findAndRegisterModules();
        var client = mock(ClickHouseClient.class);
        var metrics = new IngestionMetrics();
        var entered = new CountDownLatch(1);
        var release = new CountDownLatch(1);
        doAnswer(invocation -> {
            entered.countDown();
            assertTrue(release.await(60, TimeUnit.SECONDS));
            return null; // A late success must not acknowledge after losing membership.
        }).when(client).insertNumericBatch(anyList());
        context = createListenerContext(bootstrap, group, client, mapper, metrics,
                Map.of(ConsumerConfig.MAX_POLL_INTERVAL_MS_CONFIG, "25000"), "1", "0");
        var template = createTransactionalTemplate(bootstrap);
        String json = mapper.writeValueAsString(observation("late-success"));
        try (var admin = org.apache.kafka.clients.admin.AdminClient.create(Map.of("bootstrap.servers", bootstrap))) {
            registeredListener().start();
            template.executeInTransaction(ops -> {
                sendSync(ops, TOPIC, "slow", json);
                return true;
            });
            assertTrue(entered.await(30, TimeUnit.SECONDS));
            awaitCondition(() -> admin.describeConsumerGroups(List.of(group)).all().get()
                    .get(group).members().isEmpty(), 40, "poll timeout must evict blocked listener");
            assertEquals(null, committedOffset(bootstrap, group));
            release.countDown();
            awaitCondition(() -> !registeredListener().isRunning() && !registeredListener().isInExpectedState(),
                    20, "late return must fail-stop instead of ACK");
            assertEquals(null, committedOffset(bootstrap, group));
            assertTrue(metrics.isConsumerDegraded());
            assertEquals(org.springframework.boot.actuate.health.Status.DOWN,
                    context.getBean(TelemetryConsumerHealthIndicator.class).health().getStatus());
            System.out.println("PR227_POLL_OVERRUN members=0 offset=null latePersistence=success health=DOWN");
            context.close();
            context = null;
            // Only explicit restart can replay and acknowledge the retained record.
            context = createListenerContext(bootstrap, group, client, mapper, metrics);
            registeredListener().start();
            assertNotNull(awaitCommittedOffset(bootstrap, group, offset -> offset != null && offset > 0, 30));
            System.out.println("PR227_POLL_OVERRUN restartedOffset=" + committedOffset(bootstrap, group));
        } finally {
            release.countDown();
        }
    }

    private KafkaTemplate<String, String> createTransactionalTemplate(String bootstrap) throws Exception {
        KafkaProperties producerProps = new KafkaProperties();
        producerProps.setBootstrapServers(List.of(bootstrap));
        producerProps.getProducer().setKeySerializer(StringSerializer.class);
        producerProps.getProducer().setValueSerializer(StringSerializer.class);
        producerProps.getProducer().setAcks("all");
        producerProps.getProducer().getProperties().put(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");

        IngestionKafkaConfiguration configuration = new IngestionKafkaConfiguration();
        var transactionIdField = IngestionKafkaConfiguration.class.getDeclaredField("transactionIdPrefix");
        transactionIdField.setAccessible(true);
        transactionIdField.set(configuration, "tx-issue222-visibility-");

        destroyProducerFactory();
        producerFactory = (DefaultKafkaProducerFactory<String, String>) configuration.kafkaProducerFactory(producerProps);
        Map<String, Object> effectiveProducer = producerFactory.getConfigurationProperties();
        assertEquals(true, effectiveProducer.get(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG));
        assertEquals("all", effectiveProducer.get(ProducerConfig.ACKS_CONFIG));
        assertEquals(30_000, effectiveProducer.get(ProducerConfig.TRANSACTION_TIMEOUT_CONFIG));
        assertTrue(producerFactory.transactionCapable());

        KafkaTemplate<String, String> kafkaTemplate = new KafkaTemplate<>(producerFactory);
        assertTrue(kafkaTemplate.isTransactional());
        return kafkaTemplate;
    }

    private AnnotationConfigApplicationContext createListenerContext(
            String bootstrap, String groupId, ClickHouseClient client, ObjectMapper mapper, IngestionMetrics metrics
    ) {
        return createListenerContext(bootstrap, groupId, client, mapper, metrics, Map.of(), "2", "100");
    }

    private AnnotationConfigApplicationContext createListenerContext(
            String bootstrap, String groupId, ClickHouseClient client, ObjectMapper mapper, IngestionMetrics metrics,
            Map<String, String> consumerProperties, String attempts, String backoff
    ) {
        KafkaProperties props = new KafkaProperties();
        props.getConsumer().getProperties().putAll(consumerProperties);
        props.setBootstrapServers(List.of(bootstrap));
        props.getConsumer().setAutoOffsetReset("earliest");
        props.getConsumer().setKeyDeserializer(StringDeserializer.class);
        props.getConsumer().setValueDeserializer(StringDeserializer.class);
        props.getConsumer().setEnableAutoCommit(true); // hostile overrides are pinned
        props.getConsumer().getProperties().put(ConsumerConfig.ISOLATION_LEVEL_CONFIG, "read_uncommitted");
        AnnotationConfigApplicationContext application = new AnnotationConfigApplicationContext();
        application.getEnvironment().getPropertySources().addFirst(new MapPropertySource("listener-test", Map.of(
                "spring.kafka.consumer.group-id", groupId,
                "platform.ingestion.kafka.topic", TOPIC,
                "platform.telemetry.consumer.enabled", "false",
                "platform.telemetry.consumer.max-retries", attempts,
                "platform.telemetry.consumer.backoff-ms", backoff)));
        application.getDefaultListableBeanFactory().registerSingleton("kafkaProperties", props);
        application.getDefaultListableBeanFactory().registerSingleton("clickHouseClient", client);
        application.getDefaultListableBeanFactory().registerSingleton("objectMapper", mapper);
        application.getDefaultListableBeanFactory().registerSingleton("ingestionMetrics", metrics);
        application.register(ListenerInfrastructure.class, IngestionKafkaConfiguration.class,
                ClickHouseTelemetryConsumer.class, TelemetryConsumerHealthIndicator.class);
        try {
            application.refresh();
            assertEquals(Set.of("telemetryNumericListener"),
                    application.getBean(KafkaListenerEndpointRegistry.class).getListenerContainerIds());
            return application;
        } catch (Throwable failure) {
            application.close();
            throw failure;
        }
    }

    private MessageListenerContainer registeredListener() {
        MessageListenerContainer listener = context.getBean(KafkaListenerEndpointRegistry.class)
                .getListenerContainer("telemetryNumericListener");
        assertNotNull(listener, "Spring must discover the production @KafkaListener");
        return listener;
    }

    private void destroyProducerFactory() {
        if (producerFactory != null) {
            try {
                producerFactory.destroy();
            } finally {
                producerFactory = null;
            }
        }
    }

    private Long awaitCommittedOffset(
            String bootstrap,
            String groupId,
            java.util.function.Predicate<Long> done,
            int timeoutSeconds
    ) throws InterruptedException {
        long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(timeoutSeconds);
        Long latest = committedOffset(bootstrap, groupId);
        while (System.nanoTime() < deadline && !done.test(latest)) {
            Thread.sleep(200);
            latest = committedOffset(bootstrap, groupId);
        }
        return done.test(latest) ? latest : null;
    }

    private Long committedOffset(String bootstrap, String groupId) {
        Map<String, Object> props = new HashMap<>();
        props.put(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, bootstrap);
        props.put(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class);
        props.put(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class);
        props.put(ConsumerConfig.GROUP_ID_CONFIG, groupId);
        try (Consumer<String, String> consumer =
                     new org.apache.kafka.clients.consumer.KafkaConsumer<>(props)) {
            Map<TopicPartition, OffsetAndMetadata> committed = consumer.committed(Set.of(PARTITION));
            OffsetAndMetadata meta = committed.get(PARTITION);
            return meta == null ? null : meta.offset();
        }
    }

    private static void awaitCondition(
            java.util.concurrent.Callable<Boolean> condition,
            int timeoutSeconds,
            String message
    ) throws Exception {
        long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(timeoutSeconds);
        while (System.nanoTime() < deadline) {
            if (Boolean.TRUE.equals(condition.call())) {
                return;
            }
            Thread.sleep(200);
        }
        fail(message);
    }

    private static CanonicalObservation observation(String sequenceId) {
        return new CanonicalObservation(
                "tenant-1",
                "entity-1",
                ".1.3.6.1.2.1.1.3.0",
                "sensor-net-01",
                Instant.now(),
                42.0,
                1,
                0,
                sequenceId,
                "snmp"
        );
    }

    private static void sendSync(
            org.springframework.kafka.core.KafkaOperations<String, String> ops,
            String topic,
            String key,
            String value
    ) {
        try {
            ops.send(topic, key, value).get(20, TimeUnit.SECONDS);
        } catch (Exception e) {
            throw new IllegalStateException("failed to send key=" + key, e);
        }
    }

    private static void sendPartition(org.springframework.kafka.core.KafkaOperations<String, String> ops,
                                      int partition, String key, String value) {
        try {
            ops.send(TOPIC, partition, key, value).get(20, TimeUnit.SECONDS);
        } catch (Exception e) {
            throw new IllegalStateException("test send failed", e);
        }
    }

    private static boolean containsMessage(Throwable throwable, String needle) {
        Throwable cursor = throwable;
        while (cursor != null) {
            if (cursor.getMessage() != null && cursor.getMessage().contains(needle)) {
                return true;
            }
            cursor = cursor.getCause();
        }
        return false;
    }
}
