package com.platform.core.ingestion;

import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.autoconfigure.condition.ConditionalOnClass;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.autoconfigure.kafka.KafkaProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.kafka.config.ConcurrentKafkaListenerContainerFactory;
import org.springframework.kafka.core.ConsumerFactory;
import org.springframework.kafka.core.DefaultKafkaConsumerFactory;
import org.springframework.kafka.core.DefaultKafkaProducerFactory;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.kafka.core.ProducerFactory;
import org.springframework.kafka.listener.ContainerProperties;
import org.springframework.kafka.listener.DefaultErrorHandler;
import org.springframework.util.backoff.FixedBackOff;

import java.util.HashMap;
import java.util.Map;

@Configuration
@ConditionalOnClass(KafkaTemplate.class)
public class IngestionKafkaConfiguration {

    static final String TELEMETRY_KAFKA_CONSUMER_FACTORY = "telemetryKafkaConsumerFactory";
    static final String TELEMETRY_KAFKA_LISTENER_CONTAINER_FACTORY = "telemetryKafkaListenerContainerFactory";
    private static final String READ_COMMITTED = "read_committed";

    @Value("${spring.kafka.producer.transaction-id-prefix:${SPRING_KAFKA_PRODUCER_TRANSACTION_ID_PREFIX:}}")
    private String transactionIdPrefix;

    @Bean
    @ConditionalOnMissingBean(ProducerFactory.class)
    public ProducerFactory<String, String> kafkaProducerFactory(KafkaProperties properties) {
        Map<String, Object> producerProperties = properties.buildProducerProperties(null);
        // BUILD-003 safety pins: hostile/external producer properties must not weaken fencing.
        producerProperties.put(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, true);
        producerProperties.put(ProducerConfig.ACKS_CONFIG, "all");
        producerProperties.put(ProducerConfig.TRANSACTION_TIMEOUT_CONFIG, 30_000);
        // Bound Kafka client blocking (including commitTransaction wait) below the 60s publisher lease.
        // This is not a hard wall-clock cancel of executeInTransaction; it limits max.block.ms wait.
        producerProperties.put(ProducerConfig.MAX_BLOCK_MS_CONFIG, 20_000);
        DefaultKafkaProducerFactory<String, String> factory = new DefaultKafkaProducerFactory<>(producerProperties);
        if (transactionIdPrefix != null && !transactionIdPrefix.isBlank()) {
            factory.setTransactionIdPrefix(transactionIdPrefix);
        }
        return factory;
    }

    @Bean
    @ConditionalOnMissingBean(KafkaTemplate.class)
    public KafkaTemplate<String, String> kafkaTemplate(ProducerFactory<String, String> producerFactory) {
        return new KafkaTemplate<>(producerFactory);
    }

    /**
     * Fail-closed numeric topic contract: producer hardcodes {@link KafkaTelemetryProducer#TOPIC_TELEMETRY_NUMERIC}.
     * Any configured consumer topic override that differs is rejected at startup so producer and consumer
     * cannot silently disagree.
     */
    @Bean(name = "telemetryNumericTopicContract")
    public String telemetryNumericTopicContract(
            @Value("${platform.ingestion.kafka.topic:telemetry.numeric.v1}") String configuredTopic
    ) {
        enforceTelemetryNumericTopicContract(configuredTopic);
        return configuredTopic;
    }

    /**
     * Telemetry listener consumer factory with fail-closed transactional visibility pins.
     * Hostile {@code isolation.level}/{@code enable.auto.commit} overrides cannot weaken these invariants.
     */
    @Bean(name = TELEMETRY_KAFKA_CONSUMER_FACTORY)
    @ConditionalOnMissingBean(name = TELEMETRY_KAFKA_CONSUMER_FACTORY)
    public ConsumerFactory<String, String> telemetryKafkaConsumerFactory(KafkaProperties properties) {
        return createPinnedTelemetryConsumerFactory(properties);
    }

    /**
     * Platform-owned telemetry listener container factory.
     * Not {@code @ConditionalOnMissingBean}: a replacing bean would bypass
     * {@link #enforceTransactionalVisibilityInvariants(ConsumerFactory)} at wiring time.
     * Spring Boot defaults ({@code spring.main.allow-bean-definition-overriding=false}) reject
     * a duplicate bean definition fail-closed. Residual replacement risk is limited to trusted
     * application classpath code that can already override security/Kafka beans under an
     * explicitly permissive bean-overriding policy — not plain YAML or env alone.
     */
    @Bean(name = TELEMETRY_KAFKA_LISTENER_CONTAINER_FACTORY)
    public ConcurrentKafkaListenerContainerFactory<String, String> telemetryKafkaListenerContainerFactory(
            @Qualifier(TELEMETRY_KAFKA_CONSUMER_FACTORY) ConsumerFactory<String, String> consumerFactory
    ) {
        // Re-validate at listener wiring so a replaced telemetryKafkaConsumerFactory bean cannot
        // silently expose aborted transactional records or auto-commit offsets.
        enforceTransactionalVisibilityInvariants(consumerFactory);

        ConcurrentKafkaListenerContainerFactory<String, String> factory = new ConcurrentKafkaListenerContainerFactory<>();
        factory.setConsumerFactory(consumerFactory);
        // Manual immediate acknowledgement ensures offsets ONLY commit after ClickHouse persistence succeeds
        factory.getContainerProperties().setAckMode(ContainerProperties.AckMode.MANUAL_IMMEDIATE);
        // Default error handler with fail-closed recoverer: halts offset progression without skipping failed records
        DefaultErrorHandler errorHandler = new DefaultErrorHandler((record, exception) -> {
            throw new org.springframework.kafka.KafkaException(
                    "Exhausted retries for record at topic=" + record.topic()
                            + " partition=" + record.partition()
                            + " offset=" + record.offset()
                            + "; halting partition progression",
                    exception
            );
        }, new FixedBackOff(500L, 3L));
        errorHandler.setAckAfterHandle(false);
        factory.setCommonErrorHandler(errorHandler);
        return factory;
    }

    static ConsumerFactory<String, String> createPinnedTelemetryConsumerFactory(KafkaProperties properties) {
        Map<String, Object> consumerProperties = new HashMap<>(properties.buildConsumerProperties(null));
        // Issue #222 safety pins: producer publishes transactionally; consumer must not see aborted records.
        consumerProperties.put(ConsumerConfig.ISOLATION_LEVEL_CONFIG, READ_COMMITTED);
        consumerProperties.put(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG, false);
        DefaultKafkaConsumerFactory<String, String> factory = new DefaultKafkaConsumerFactory<>(consumerProperties);
        enforceTransactionalVisibilityInvariants(factory);
        return factory;
    }

    static void enforceTelemetryNumericTopicContract(String configuredTopic) {
        String expected = KafkaTelemetryProducer.TOPIC_TELEMETRY_NUMERIC;
        if (configuredTopic == null || !expected.equals(configuredTopic.trim())) {
            throw new IllegalStateException(
                    "Telemetry numeric Kafka topic must be exactly '" + expected
                            + "' to match KafkaTelemetryProducer; configured platform.ingestion.kafka.topic="
                            + configuredTopic
            );
        }
    }

    static void enforceTransactionalVisibilityInvariants(ConsumerFactory<?, ?> consumerFactory) {
        if (!(consumerFactory instanceof DefaultKafkaConsumerFactory<?, ?> defaultFactory)) {
            throw new IllegalStateException(
                    "Telemetry Kafka consumer factory must be a DefaultKafkaConsumerFactory so "
                            + "isolation.level=read_committed and enable.auto.commit=false can be guaranteed"
            );
        }
        Map<String, Object> effective = defaultFactory.getConfigurationProperties();
        Object isolation = effective.get(ConsumerConfig.ISOLATION_LEVEL_CONFIG);
        if (!READ_COMMITTED.equals(String.valueOf(isolation))) {
            throw new IllegalStateException(
                    "Telemetry Kafka consumer requires isolation.level=read_committed; effective value="
                            + isolation
            );
        }
        Object autoCommit = effective.get(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG);
        if (!isExplicitlyFalse(autoCommit)) {
            throw new IllegalStateException(
                    "Telemetry Kafka consumer requires enable.auto.commit=false; effective value="
                            + autoCommit
            );
        }
    }

    private static boolean isExplicitlyFalse(Object value) {
        if (value instanceof Boolean bool) {
            return !bool;
        }
        return value != null && "false".equalsIgnoreCase(String.valueOf(value).trim());
    }
}
