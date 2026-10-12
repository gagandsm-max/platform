package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.apache.kafka.clients.consumer.Consumer;
import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.clients.consumer.ConsumerRecords;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.common.serialization.StringDeserializer;
import org.apache.kafka.common.serialization.StringSerializer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.springframework.boot.autoconfigure.kafka.KafkaProperties;
import org.springframework.kafka.core.DefaultKafkaConsumerFactory;
import org.springframework.kafka.core.DefaultKafkaProducerFactory;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.kafka.core.ProducerFactory;
import org.springframework.kafka.support.Acknowledgment;
import org.springframework.kafka.test.EmbeddedKafkaBroker;
import org.springframework.kafka.test.EmbeddedKafkaKraftBroker;

import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.junit.jupiter.api.Assertions.fail;
import static org.mockito.ArgumentMatchers.anyList;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;

/**
 * Isolated embedded KRaft broker evidence that aborted transactional records are
 * invisible to the Platform-pinned {@code read_committed} telemetry consumer path.
 * Does not touch the live Compose Kafka broker, volumes or identity.
 */
class TelemetryKafkaTransactionVisibilityTest {

    private static final String TOPIC = KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC;
    private static final String ABORTED_MARKER = "ISSUE222-ABORTED-" + UUID.randomUUID();
    private static final String COMMITTED_MARKER = "ISSUE222-COMMITTED-" + UUID.randomUUID();

    private EmbeddedKafkaBroker embeddedKafka;

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
        if (embeddedKafka != null) {
            embeddedKafka.destroy();
        }
    }

    @Test
    @Timeout(value = 90, unit = TimeUnit.SECONDS)
    void readCommittedConsumerSeesCommittedRecordOnlyAndFeedsClickHousePath() throws Exception {
        String bootstrap = embeddedKafka.getBrokersAsString();

        // D — transactional producer settings remain correct (Platform pins).
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

        ProducerFactory<String, String> producerFactory = configuration.kafkaProducerFactory(producerProps);
        Map<String, Object> effectiveProducer =
                ((DefaultKafkaProducerFactory<String, String>) producerFactory).getConfigurationProperties();
        assertEquals(true, effectiveProducer.get(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG));
        assertEquals("all", effectiveProducer.get(ProducerConfig.ACKS_CONFIG));
        assertEquals(30_000, effectiveProducer.get(ProducerConfig.TRANSACTION_TIMEOUT_CONFIG));
        assertTrue(producerFactory.transactionCapable());

        KafkaTemplate<String, String> kafkaTemplate = new KafkaTemplate<>(producerFactory);
        assertTrue(kafkaTemplate.isTransactional());

        // A — publish inside a transaction, then abort.
        try {
            kafkaTemplate.executeInTransaction(ops -> {
                sendSync(ops, TOPIC, "abort-key", ABORTED_MARKER);
                throw new IllegalStateException("intentional abort for issue222 visibility proof");
            });
            fail("aborted transaction must throw");
        } catch (Exception expected) {
            assertTrue(containsMessage(expected, "intentional abort"), "expected intentional abort; got: " + expected);
        }

        // B — publish a distinct record in a committed transaction.
        kafkaTemplate.executeInTransaction(ops -> {
            sendSync(ops, TOPIC, "commit-key", COMMITTED_MARKER);
            return true;
        });

        // C/E — actual configured read_committed consumer; auto-commit remains disabled.
        KafkaProperties consumerProps = new KafkaProperties();
        consumerProps.setBootstrapServers(List.of(bootstrap));
        consumerProps.getConsumer().setGroupId("issue222-visibility-" + UUID.randomUUID());
        consumerProps.getConsumer().setAutoOffsetReset("earliest");
        consumerProps.getConsumer().setKeyDeserializer(StringDeserializer.class);
        consumerProps.getConsumer().setValueDeserializer(StringDeserializer.class);
        consumerProps.getConsumer().setEnableAutoCommit(true); // hostile; must be pinned false
        consumerProps.getConsumer().getProperties().put(ConsumerConfig.ISOLATION_LEVEL_CONFIG, "read_uncommitted");

        var consumerFactory = IngestionKafkaConfiguration.createPinnedTelemetryConsumerFactory(consumerProps);
        Map<String, Object> effectiveConsumer =
                ((DefaultKafkaConsumerFactory<String, String>) consumerFactory).getConfigurationProperties();
        assertEquals("read_committed", effectiveConsumer.get(ConsumerConfig.ISOLATION_LEVEL_CONFIG));
        assertEquals(false, effectiveConsumer.get(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG));

        List<ConsumerRecord<String, String>> markerVisible = pollUntil(
                consumerFactory,
                "issue222-markers-" + UUID.randomUUID(),
                records -> records.stream().anyMatch(r -> COMMITTED_MARKER.equals(r.value())),
                30
        );

        assertEquals(
                1,
                markerVisible.stream().filter(r -> COMMITTED_MARKER.equals(r.value()) || ABORTED_MARKER.equals(r.value())).count(),
                "read_committed must see only the committed marker; got=" + values(markerVisible)
        );
        assertTrue(markerVisible.stream().anyMatch(r -> COMMITTED_MARKER.equals(r.value())));
        assertFalse(
                markerVisible.stream().anyMatch(r -> ABORTED_MARKER.equals(r.value())),
                "aborted transactional record must not be visible"
        );

        // F — ClickHouse-facing persistence cannot observe the aborted record through the intended path.
        ClickHouseClient clickHouseClient = mock(ClickHouseClient.class);
        ObjectMapper objectMapper = new ObjectMapper().findAndRegisterModules();
        IngestionMetrics metrics = new IngestionMetrics();
        ClickHouseTelemetryConsumer clickHouseConsumer =
                new ClickHouseTelemetryConsumer(clickHouseClient, objectMapper, metrics, 3, 10);

        CanonicalObservation committedObs = new CanonicalObservation(
                "tenant-1", "entity-1", ".1.3.6.1.2.1.1.3.0", "sensor-net-01",
                Instant.now(), 42.0, 1, 0, "seq-committed", "snmp"
        );
        String committedJson = objectMapper.writeValueAsString(committedObs);

        kafkaTemplate.executeInTransaction(ops -> {
            sendSync(ops, TOPIC, "obs-key", committedJson);
            return true;
        });

        List<ConsumerRecord<String, String>> observationVisible = pollUntil(
                consumerFactory,
                "issue222-ch-" + UUID.randomUUID(),
                records -> records.stream().anyMatch(r -> committedJson.equals(r.value())),
                30
        );

        ConsumerRecord<String, String> observationRecord = observationVisible.stream()
                .filter(r -> committedJson.equals(r.value()))
                .findFirst()
                .orElse(null);
        assertNotNull(observationRecord, "committed observation must be visible to read_committed consumer");
        assertFalse(observationVisible.stream().anyMatch(r -> ABORTED_MARKER.equals(r.value())));

        Acknowledgment ack = mock(Acknowledgment.class);
        clickHouseConsumer.consume(observationRecord, ack);
        verify(clickHouseClient, times(1)).insertNumericBatch(anyList());
        verify(clickHouseClient, never()).insertCounterBatch(anyList());
        verify(ack, times(1)).acknowledge();
    }

    private List<ConsumerRecord<String, String>> pollUntil(
            org.springframework.kafka.core.ConsumerFactory<String, String> consumerFactory,
            String groupId,
            java.util.function.Predicate<List<ConsumerRecord<String, String>>> done,
            int timeoutSeconds
    ) {
        List<ConsumerRecord<String, String>> visible = new ArrayList<>();
        try (Consumer<String, String> consumer = consumerFactory.createConsumer(groupId, null)) {
            embeddedKafka.consumeFromAnEmbeddedTopic(consumer, TOPIC);
            long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(timeoutSeconds);
            while (System.nanoTime() < deadline && !done.test(visible)) {
                ConsumerRecords<String, String> polled = consumer.poll(Duration.ofMillis(500));
                polled.forEach(visible::add);
            }
        }
        return visible;
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

    private static List<String> values(List<ConsumerRecord<String, String>> records) {
        return records.stream().map(ConsumerRecord::value).toList();
    }
}
