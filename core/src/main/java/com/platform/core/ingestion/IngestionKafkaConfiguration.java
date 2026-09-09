package com.platform.core.ingestion;

import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.producer.ProducerConfig;
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

import java.util.Map;

@Configuration
@ConditionalOnClass(KafkaTemplate.class)
public class IngestionKafkaConfiguration {

    @Value("${spring.kafka.producer.transaction-id-prefix:${SPRING_KAFKA_PRODUCER_TRANSACTION_ID_PREFIX:}}")
    private String transactionIdPrefix;

    @Bean
    @ConditionalOnMissingBean(ProducerFactory.class)
    public ProducerFactory<String, String> kafkaProducerFactory(KafkaProperties properties) {
        Map<String, Object> producerProperties = properties.buildProducerProperties(null);
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

    @Bean
    @ConditionalOnMissingBean(name = "telemetryKafkaListenerContainerFactory")
    public ConcurrentKafkaListenerContainerFactory<String, String> telemetryKafkaListenerContainerFactory(
            ConsumerFactory<String, String> consumerFactory
    ) {
        ConcurrentKafkaListenerContainerFactory<String, String> factory = new ConcurrentKafkaListenerContainerFactory<>();
        factory.setConsumerFactory(consumerFactory);
        // Manual immediate acknowledgement ensures offsets ONLY commit after ClickHouse persistence succeeds
        factory.getContainerProperties().setAckMode(ContainerProperties.AckMode.MANUAL_IMMEDIATE);
        // Default error handler with no skip: stops container progression on unhandled failure
        DefaultErrorHandler errorHandler = new DefaultErrorHandler(new FixedBackOff(500L, 3L));
        factory.setCommonErrorHandler(errorHandler);
        return factory;
    }
}
