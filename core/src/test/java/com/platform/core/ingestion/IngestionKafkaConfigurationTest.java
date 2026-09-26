package com.platform.core.ingestion;

import org.apache.kafka.clients.producer.ProducerConfig;
import org.junit.jupiter.api.Test;
import org.springframework.boot.autoconfigure.kafka.KafkaProperties;
import org.springframework.kafka.core.DefaultKafkaProducerFactory;
import org.springframework.kafka.core.ProducerFactory;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;

class IngestionKafkaConfigurationTest {

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
}
