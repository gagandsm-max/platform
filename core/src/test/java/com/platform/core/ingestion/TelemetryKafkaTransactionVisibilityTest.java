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
                    throw new RuntimeException("intentional transient ClickHouse failure");
                }
                if (attempt == 2) {
                    failedAttemptBarrier.countDown();
                    assertTrue(releaseFailure.await(30, TimeUnit.SECONDS), "release failed insertion");
                    throw new RuntimeException("intentional exhausted ClickHouse failure");
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
        assertEquals(org.springframework.boot.actuate.health.Status.UP,
                context.getBean(TelemetryConsumerHealthIndicator.class).health().getStatus(),
                "intentionally disabled startup is not an abnormal failure");
        assertEquals("STOPPED", context.getBean(TelemetryConsumerHealthIndicator.class).health().getDetails().get("status"));

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
            // Attempt one has failed; attempt two cannot succeed or ACK at this barrier.
            assertEquals(offsetBeforeRedelivery, committedOffset(bootstrap, groupId));
            System.out.println("PR227_OFFSET failing=" + committedOffset(bootstrap, groupId)
                    + " recoveryPermitted=false");
            releaseFailure.countDown();
            awaitCondition(() -> !registeredListener().isRunning()
                    && !registeredListener().isInExpectedState(), 30, "exhaustion must stop abnormally");
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
            awaitCondition(() -> !metrics.isConsumerDegraded(), 30, "late success clears shared metrics");
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
