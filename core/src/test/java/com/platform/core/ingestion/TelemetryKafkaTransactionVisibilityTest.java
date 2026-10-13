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
import org.springframework.kafka.core.DefaultKafkaConsumerFactory;
import org.springframework.kafka.core.DefaultKafkaProducerFactory;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.kafka.listener.AcknowledgingMessageListener;
import org.springframework.kafka.listener.ConcurrentMessageListenerContainer;
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
        // Deterministic producer cleanup before broker teardown (success, failure, timeout).
        destroyProducerFactory();
        if (embeddedKafka != null) {
            embeddedKafka.destroy();
            embeddedKafka = null;
        }
    }

    @Test
    @Timeout(value = 120, unit = TimeUnit.SECONDS)
    void actualListenerContainerHonorsReadCommittedAckAndFailClosedOffsets() throws Exception {
        String bootstrap = embeddedKafka.getBrokersAsString();
        ObjectMapper objectMapper = new ObjectMapper().findAndRegisterModules();

        KafkaTemplate<String, String> kafkaTemplate = createTransactionalTemplate(bootstrap);
        ConcurrentKafkaListenerContainerFactory<String, String> listenerFactory =
                createApplicationListenerFactory(bootstrap);

        // 7 — actual application container factory pins.
        assertEquals(
                ContainerProperties.AckMode.MANUAL_IMMEDIATE,
                listenerFactory.getContainerProperties().getAckMode()
        );
        Map<String, Object> effectiveConsumer =
                ((DefaultKafkaConsumerFactory<String, String>) listenerFactory.getConsumerFactory())
                        .getConfigurationProperties();
        assertEquals("read_committed", effectiveConsumer.get(ConsumerConfig.ISOLATION_LEVEL_CONFIG));
        assertEquals(false, effectiveConsumer.get(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG));
        IngestionKafkaConfiguration.enforceTransactionalVisibilityInvariants(
                listenerFactory.getConsumerFactory()
        );

        String groupId = "issue222-listener-" + UUID.randomUUID();
        ClickHouseClient clickHouseClient = mock(ClickHouseClient.class);
        IngestionMetrics metrics = new IngestionMetrics();
        // Single CH attempt per delivery so Kafka-level redelivery is observable after throw.
        ClickHouseTelemetryConsumer telemetryConsumer =
                new ClickHouseTelemetryConsumer(clickHouseClient, objectMapper, metrics, 1, 1);

        List<String> deliveredSequenceIds = new CopyOnWriteArrayList<>();
        CountDownLatch committedInsertLatch = new CountDownLatch(1);
        CountDownLatch redeliverySuccessLatch = new CountDownLatch(1);
        AtomicInteger redeliveryInsertAttempts = new AtomicInteger();
        AtomicInteger poisonDeliveries = new AtomicInteger();

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
                if (attempt < 3) {
                    throw new RuntimeException(
                            "intentional ClickHouse failure for redelivery proof attempt=" + attempt
                    );
                }
                redeliverySuccessLatch.countDown();
                return null;
            }
            throw new RuntimeException("unexpected observation sequenceId=" + seq);
        }).when(clickHouseClient).insertNumericBatch(anyList());

        ConcurrentMessageListenerContainer<String, String> container =
                listenerFactory.createContainer(TOPIC);
        container.setBeanName("issue222-telemetry-listener");
        container.getContainerProperties().setGroupId(groupId);
        container.setupMessageListener((AcknowledgingMessageListener<String, String>) (record, acknowledgment) -> {
            if (poisonPayload.equals(record.value())) {
                poisonDeliveries.incrementAndGet();
            }
            telemetryConsumer.consume(record, acknowledgment);
        });

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

            // 6 — Kafka-level redelivery after ClickHouse failure; offset advances only on success.
            long offsetBeforeRedelivery = afterCommitted;
            kafkaTemplate.executeInTransaction(ops -> {
                sendSync(ops, TOPIC, "redeliver-key", redeliveryJson);
                return true;
            });

            assertTrue(
                    redeliverySuccessLatch.await(60, TimeUnit.SECONDS),
                    "ClickHouse must eventually succeed after Kafka redelivery; attempts="
                            + redeliveryInsertAttempts.get()
            );
            assertTrue(
                    redeliveryInsertAttempts.get() >= 3,
                    "failure must be retried via Kafka redelivery; attempts="
                            + redeliveryInsertAttempts.get()
            );
            Long afterRedelivery = awaitCommittedOffset(
                    bootstrap,
                    groupId,
                    offset -> offset != null && offset > offsetBeforeRedelivery,
                    30
            );
            assertNotNull(afterRedelivery, "offset must advance only after successful redelivery processing");
            assertTrue(afterRedelivery > offsetBeforeRedelivery);

            // 5 — poison/ClickHouse-path failure must not silently commit an unprocessed offset.
            long offsetBeforePoison = afterRedelivery;
            kafkaTemplate.executeInTransaction(ops -> {
                sendSync(ops, TOPIC, "poison-key", poisonPayload);
                return true;
            });

            awaitCondition(
                    () -> poisonDeliveries.get() >= 1 && metrics.getConsumerPoisonEvents() >= 1,
                    45,
                    "poison must be delivered to the listener without ACK"
            );
            // Allow error-handler seeks/retries; commit must remain unchanged.
            Thread.sleep(2_500);
            assertEquals(
                    offsetBeforePoison,
                    committedOffset(bootstrap, groupId),
                    "poison/fail-closed path must not advance the group offset"
            );
            // Still no aborted observation visibility after subsequent traffic.
            assertFalse(deliveredSequenceIds.contains(abortedObs.sequenceId()));
        } finally {
            stopContainerQuietly(container);
            // Ensure producer resources are released even if assertions fail before @AfterEach.
            destroyProducerFactory();
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

    private ConcurrentKafkaListenerContainerFactory<String, String> createApplicationListenerFactory(
            String bootstrap
    ) {
        KafkaProperties consumerProps = new KafkaProperties();
        consumerProps.setBootstrapServers(List.of(bootstrap));
        consumerProps.getConsumer().setAutoOffsetReset("earliest");
        consumerProps.getConsumer().setKeyDeserializer(StringDeserializer.class);
        consumerProps.getConsumer().setValueDeserializer(StringDeserializer.class);
        // Hostile overrides must remain pinned closed by the application factory.
        consumerProps.getConsumer().setEnableAutoCommit(true);
        consumerProps.getConsumer().getProperties().put(ConsumerConfig.ISOLATION_LEVEL_CONFIG, "read_uncommitted");

        IngestionKafkaConfiguration configuration = new IngestionKafkaConfiguration();
        var consumerFactory = configuration.telemetryKafkaConsumerFactory(consumerProps);
        return configuration.telemetryKafkaListenerContainerFactory(consumerFactory);
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

    private static void stopContainerQuietly(ConcurrentMessageListenerContainer<String, String> container) {
        if (container != null) {
            try {
                container.stop();
            } catch (Exception ignored) {
                // best-effort cleanup before broker destroy
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
