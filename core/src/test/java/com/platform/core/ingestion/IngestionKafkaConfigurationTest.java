package com.platform.core.ingestion;

import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.common.serialization.StringDeserializer;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.BeanCreationException;
import org.springframework.beans.factory.BeanDefinitionStoreException;
import org.springframework.boot.autoconfigure.kafka.KafkaProperties;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.kafka.config.ConcurrentKafkaListenerContainerFactory;
import org.springframework.kafka.core.ConsumerFactory;
import org.springframework.kafka.core.DefaultKafkaConsumerFactory;
import org.springframework.kafka.core.DefaultKafkaProducerFactory;
import org.springframework.kafka.core.ProducerFactory;
import org.springframework.kafka.listener.ContainerProperties;
import org.springframework.mock.env.MockEnvironment;

import java.util.HashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class IngestionKafkaConfigurationTest {

    @Test
    void pinnedDefaultHandlerResetsFiniteRetriesWhenRecovererThrows() {
        var deliveries = new java.util.ArrayList<Integer>();
        var recoveries = new java.util.concurrent.atomic.AtomicInteger();
        var handler = new org.springframework.kafka.listener.DefaultErrorHandler((record, failure) -> {
            recoveries.incrementAndGet();
            throw new IllegalStateException("recovery failed");
        }, new org.springframework.util.backoff.FixedBackOff(0, 1));
        handler.setRetryListeners((record, failure, attempt) -> deliveries.add(attempt));
        var record = new org.apache.kafka.clients.consumer.ConsumerRecord<>("telemetry.numeric.v1", 0, 0L, "key", "{}");
        var failure = new IllegalStateException("permanent failure");
        var consumer = org.mockito.Mockito.mock(org.apache.kafka.clients.consumer.Consumer.class);
        var container = org.mockito.Mockito.mock(org.springframework.kafka.listener.MessageListenerContainer.class);
        for (int attempt = 0; attempt < 4; attempt++) {
            assertFalse(handler.handleOne(failure, record, consumer, container));
        }
        assertEquals(java.util.List.of(1, 2, 1, 2), deliveries);
        assertEquals(2, recoveries.get(), "finite backoff restarts after each failed recovery");
    }

    @Test
    void pinsIdempotenceAcksAndTransactionTimeoutAgainstHostileOverrides() {
        KafkaProperties properties = new KafkaProperties();
        properties.getProducer().getProperties().put(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "false");
        properties.getProducer().getProperties().put(ProducerConfig.ACKS_CONFIG, "1");
        properties.getProducer().getProperties().put(ProducerConfig.TRANSACTION_TIMEOUT_CONFIG, "120000");
        properties.getProducer().getProperties().put(ProducerConfig.MAX_BLOCK_MS_CONFIG, "120000");

        IngestionKafkaConfiguration configuration = new IngestionKafkaConfiguration();
        ProducerFactory<String, String> factory = configuration.kafkaProducerFactory(properties);
        assertInstanceOf(DefaultKafkaProducerFactory.class, factory);

        Map<String, Object> effective = ((DefaultKafkaProducerFactory<String, String>) factory)
                .getConfigurationProperties();
        assertEquals(true, effective.get(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG));
        assertEquals("all", effective.get(ProducerConfig.ACKS_CONFIG));
        assertEquals(30_000, effective.get(ProducerConfig.TRANSACTION_TIMEOUT_CONFIG));
        assertEquals(20_000, effective.get(ProducerConfig.MAX_BLOCK_MS_CONFIG));
    }

    @Test
    void defaultTelemetryConsumerFactoryIsReadCommittedWithAutoCommitDisabled() {
        KafkaProperties properties = new KafkaProperties();
        ConsumerFactory<String, String> factory =
                IngestionKafkaConfiguration.createPinnedTelemetryConsumerFactory(properties);
        Map<String, Object> effective = ((DefaultKafkaConsumerFactory<String, String>) factory)
                .getConfigurationProperties();

        assertEquals("read_committed", effective.get(ConsumerConfig.ISOLATION_LEVEL_CONFIG));
        assertEquals(false, effective.get(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG));
    }

    @Test
    void hostileReadUncommittedAndAutoCommitOverridesArePinnedClosed() {
        KafkaProperties properties = new KafkaProperties();
        properties.getConsumer().setEnableAutoCommit(true);
        properties.getConsumer().getProperties().put(ConsumerConfig.ISOLATION_LEVEL_CONFIG, "read_uncommitted");
        properties.getConsumer().getProperties().put(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG, "true");

        ConsumerFactory<String, String> factory =
                IngestionKafkaConfiguration.createPinnedTelemetryConsumerFactory(properties);
        Map<String, Object> effective = ((DefaultKafkaConsumerFactory<String, String>) factory)
                .getConfigurationProperties();

        assertEquals("read_committed", effective.get(ConsumerConfig.ISOLATION_LEVEL_CONFIG));
        assertEquals(false, effective.get(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG));
        assertFalse(Boolean.parseBoolean(String.valueOf(effective.get(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG))));
    }

    @Test
    void listenerContainerUsesPinnedConsumerFactoryAndManualImmediateAck() {
        KafkaProperties properties = new KafkaProperties();
        IngestionKafkaConfiguration configuration = new IngestionKafkaConfiguration();
        ConsumerFactory<String, String> consumerFactory =
                configuration.telemetryKafkaConsumerFactory(properties);

        ConcurrentKafkaListenerContainerFactory<String, String> listenerFactory =
                configuration.telemetryKafkaListenerContainerFactory(consumerFactory, new IngestionMetrics(), event -> { });

        assertSame(consumerFactory, listenerFactory.getConsumerFactory());
        assertEquals(
                ContainerProperties.AckMode.MANUAL_IMMEDIATE,
                listenerFactory.getContainerProperties().getAckMode()
        );
        IngestionKafkaConfiguration.enforceTransactionalVisibilityInvariants(
                listenerFactory.getConsumerFactory()
        );
    }

    @Test
    void enforceRejectsReadUncommittedConsumerFactory() {
        Map<String, Object> unsafe = new HashMap<>();
        unsafe.put(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, "localhost:9092");
        unsafe.put(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class);
        unsafe.put(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class);
        unsafe.put(ConsumerConfig.ISOLATION_LEVEL_CONFIG, "read_uncommitted");
        unsafe.put(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG, false);
        ConsumerFactory<String, String> unsafeFactory = new DefaultKafkaConsumerFactory<>(unsafe);

        IllegalStateException ex = assertThrows(
                IllegalStateException.class,
                () -> IngestionKafkaConfiguration.enforceTransactionalVisibilityInvariants(unsafeFactory)
        );
        assertTrue(ex.getMessage().contains("read_committed"));
    }

    @Test
    void enforceRejectsAutoCommitEnabledConsumerFactory() {
        Map<String, Object> unsafe = new HashMap<>();
        unsafe.put(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, "localhost:9092");
        unsafe.put(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class);
        unsafe.put(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class);
        unsafe.put(ConsumerConfig.ISOLATION_LEVEL_CONFIG, "read_committed");
        unsafe.put(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG, true);
        ConsumerFactory<String, String> unsafeFactory = new DefaultKafkaConsumerFactory<>(unsafe);

        IllegalStateException ex = assertThrows(
                IllegalStateException.class,
                () -> IngestionKafkaConfiguration.enforceTransactionalVisibilityInvariants(unsafeFactory)
        );
        assertTrue(ex.getMessage().contains("enable.auto.commit=false"));
    }

    @Test
    void enforceRejectsUnsupportedTelemetryTopicOverride() {
        IllegalStateException ex = assertThrows(
                IllegalStateException.class,
                () -> IngestionKafkaConfiguration.enforceTelemetryNumericTopicContract("other.topic.v1")
        );
        assertTrue(ex.getMessage().contains(KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC));
        assertTrue(ex.getMessage().contains("other.topic.v1"));
    }

    @Test
    void enforceRejectsLeadingWhitespaceTopic() {
        IllegalStateException ex = assertThrows(
                IllegalStateException.class,
                () -> IngestionKafkaConfiguration.enforceTelemetryNumericTopicContract(
                        " " + KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC
                )
        );
        assertTrue(ex.getMessage().contains(KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC));
    }

    @Test
    void enforceRejectsTrailingWhitespaceTopic() {
        IllegalStateException ex = assertThrows(
                IllegalStateException.class,
                () -> IngestionKafkaConfiguration.enforceTelemetryNumericTopicContract(
                        KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC + " "
                )
        );
        assertTrue(ex.getMessage().contains(KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC));
    }

    @Test
    void enforceRejectsBlankAndNullTopic() {
        assertThrows(
                IllegalStateException.class,
                () -> IngestionKafkaConfiguration.enforceTelemetryNumericTopicContract("")
        );
        assertThrows(
                IllegalStateException.class,
                () -> IngestionKafkaConfiguration.enforceTelemetryNumericTopicContract("   ")
        );
        assertThrows(
                IllegalStateException.class,
                () -> IngestionKafkaConfiguration.enforceTelemetryNumericTopicContract(null)
        );
    }

    @Test
    void enforceAcceptsCanonicalTelemetryNumericTopic() {
        IngestionKafkaConfiguration.enforceTelemetryNumericTopicContract(
                KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC
        );
    }

    @Test
    void unsupportedTopicPropertyFailsContextStartup() {
        assertTopicPropertyFailsContextStartup("hostile.redirect.v1");
    }

    @Test
    void leadingWhitespaceTopicPropertyFailsContextStartup() {
        assertTopicPropertyFailsContextStartup(" " + KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC);
    }

    @Test
    void trailingWhitespaceTopicPropertyFailsContextStartup() {
        assertTopicPropertyFailsContextStartup(KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC + " ");
    }

    @Test
    void blankTopicPropertyFailsContextStartup() {
        assertTopicPropertyFailsContextStartup("   ");
    }

    @Test
    void canonicalTopicPropertyAllowsContextStartup() {
        try (AnnotationConfigApplicationContext context = new AnnotationConfigApplicationContext()) {
            MockEnvironment env = new MockEnvironment();
            env.setProperty("platform.ingestion.kafka.topic", KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC);
            context.setEnvironment(env);
            context.registerBean(KafkaProperties.class, KafkaProperties::new);
            context.registerBean(IngestionMetrics.class, IngestionMetrics::new);
            context.register(IngestionKafkaConfiguration.class);
            context.refresh();

            assertEquals(
                    KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC,
                    context.getBean("telemetryNumericTopicContract", String.class)
            );
        }
    }

    private static void assertTopicPropertyFailsContextStartup(String configuredTopic) {
        try (AnnotationConfigApplicationContext context = new AnnotationConfigApplicationContext()) {
            MockEnvironment env = new MockEnvironment();
            env.setProperty("platform.ingestion.kafka.topic", configuredTopic);
            context.setEnvironment(env);
            context.registerBean(KafkaProperties.class, KafkaProperties::new);
            context.registerBean(IngestionMetrics.class, IngestionMetrics::new);
            context.register(IngestionKafkaConfiguration.class);

            BeanCreationException ex = assertThrows(BeanCreationException.class, context::refresh);
            Throwable root = rootCause(ex);
            assertInstanceOf(IllegalStateException.class, root);
            assertTrue(
                    root.getMessage().contains(KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC),
                    "startup must fail closed on topic mismatch; got: " + root.getMessage()
            );
        }
    }

    @Test
    void replacedUnsafeTelemetryConsumerFactoryFailsListenerStartup() {
        try (AnnotationConfigApplicationContext context = new AnnotationConfigApplicationContext()) {
            context.registerBean(KafkaProperties.class, KafkaProperties::new);
            context.register(UnsafeTelemetryConsumerFactoryOverride.class);
            context.registerBean(IngestionMetrics.class, IngestionMetrics::new);
            context.register(IngestionKafkaConfiguration.class);

            BeanCreationException ex = assertThrows(BeanCreationException.class, context::refresh);
            Throwable root = rootCause(ex);
            assertInstanceOf(IllegalStateException.class, root);
            assertTrue(
                    root.getMessage().contains("read_committed")
                            || root.getMessage().contains("enable.auto.commit=false"),
                    "startup must fail closed on unsafe transactional visibility; got: " + root.getMessage()
            );
        }
    }

    @Test
    void duplicateListenerFactoryDefinitionFailsClosedWhenOverridingDisabled() {
        try (AnnotationConfigApplicationContext context = new AnnotationConfigApplicationContext()) {
            context.setAllowBeanDefinitionOverriding(false);
            context.registerBean(KafkaProperties.class, KafkaProperties::new);
            context.register(HostileListenerFactoryOverride.class);
            context.registerBean(IngestionMetrics.class, IngestionMetrics::new);
            context.register(IngestionKafkaConfiguration.class);

            assertThrows(BeanDefinitionStoreException.class, context::refresh);
        }
    }

    @Test
    void safeTelemetryConsumerBeansWireTogether() {
        try (AnnotationConfigApplicationContext context = new AnnotationConfigApplicationContext()) {
            context.registerBean(KafkaProperties.class, KafkaProperties::new);
            context.registerBean(IngestionMetrics.class, IngestionMetrics::new);
            context.register(IngestionKafkaConfiguration.class);
            context.refresh();

            assertEquals(
                    KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC,
                    context.getBean("telemetryNumericTopicContract", String.class)
            );

            ConsumerFactory<?, ?> consumerFactory = context.getBean(
                    IngestionKafkaConfiguration.TELEMETRY_KAFKA_CONSUMER_FACTORY,
                    ConsumerFactory.class
            );
            ConcurrentKafkaListenerContainerFactory<?, ?> listenerFactory = context.getBean(
                    IngestionKafkaConfiguration.TELEMETRY_KAFKA_LISTENER_CONTAINER_FACTORY,
                    ConcurrentKafkaListenerContainerFactory.class
            );

            assertSame(consumerFactory, listenerFactory.getConsumerFactory());
            assertEquals(
                    ContainerProperties.AckMode.MANUAL_IMMEDIATE,
                    listenerFactory.getContainerProperties().getAckMode()
            );
            IngestionKafkaConfiguration.enforceTransactionalVisibilityInvariants(consumerFactory);
        }
    }

    @Test
    void unsafeRetryPropertiesFailContextStartupEvenWhenListenerDisabled() {
        try (var context = new AnnotationConfigApplicationContext()) {
            var env = new MockEnvironment();
            env.setProperty("platform.telemetry.consumer.enabled", "false");
            env.setProperty("platform.telemetry.consumer.max-retries", "2147483647");
            env.setProperty("platform.telemetry.consumer.backoff-ms", "9223372036854775807");
            context.setEnvironment(env);
            context.registerBean(KafkaProperties.class, KafkaProperties::new);
            context.registerBean(IngestionMetrics.class, IngestionMetrics::new);
            context.register(IngestionKafkaConfiguration.class);
            var failure = assertThrows(BeanCreationException.class, context::refresh);
            assertTrue(rootCause(failure).getMessage().contains("ERR_KAFKA_POLL_BUDGET"));
        }
    }

    @Test
    void explicitSafeBatchSizeIsPreservedAndCommitWaitIsBounded() {
        var properties = new KafkaProperties();
        properties.getConsumer().setMaxPollRecords(4);
        var config = new IngestionKafkaConfiguration();
        var consumerFactory = config.telemetryKafkaConsumerFactory(properties);
        assertEquals(4, consumerFactory.getConfigurationProperties().get(ConsumerConfig.MAX_POLL_RECORDS_CONFIG));
        assertDoesNotThrow(() -> config.telemetryProcessingBudget(consumerFactory, 5, 200));
        var container = config.telemetryKafkaListenerContainerFactory(consumerFactory, new IngestionMetrics(), event -> { }).getContainerProperties();
        assertTrue(container.isSyncCommits());
        assertEquals(java.time.Duration.ofMillis(TelemetryProcessingBudget.COMMIT_TIMEOUT_MS), container.getSyncCommitTimeout());
        assertEquals(0, container.getCommitRetries());
        assertEquals(TelemetryProcessingBudget.POLL_TIMEOUT_MS, container.getPollTimeout());
    }

    private static Throwable rootCause(Throwable throwable) {
        Throwable cursor = throwable;
        while (cursor.getCause() != null && cursor.getCause() != cursor) {
            cursor = cursor.getCause();
        }
        return cursor;
    }

    @Configuration
    static class UnsafeTelemetryConsumerFactoryOverride {
        @Bean(name = IngestionKafkaConfiguration.TELEMETRY_KAFKA_CONSUMER_FACTORY)
        ConsumerFactory<String, String> telemetryKafkaConsumerFactory() {
            Map<String, Object> unsafe = new HashMap<>();
            unsafe.put(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, "localhost:9092");
            unsafe.put(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class);
            unsafe.put(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class);
            unsafe.put(ConsumerConfig.ISOLATION_LEVEL_CONFIG, "read_uncommitted");
            unsafe.put(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG, false);
            return new DefaultKafkaConsumerFactory<>(unsafe);
        }
    }

    @Configuration
    static class HostileListenerFactoryOverride {
        @Bean(name = IngestionKafkaConfiguration.TELEMETRY_KAFKA_LISTENER_CONTAINER_FACTORY)
        ConcurrentKafkaListenerContainerFactory<String, String> telemetryKafkaListenerContainerFactory() {
            return new ConcurrentKafkaListenerContainerFactory<>();
        }
    }
}
