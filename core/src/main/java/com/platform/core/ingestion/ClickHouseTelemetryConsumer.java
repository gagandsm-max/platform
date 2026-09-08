package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Component;

import java.util.List;

@Component
public class ClickHouseTelemetryConsumer {
    private static final Logger log = LoggerFactory.getLogger(ClickHouseTelemetryConsumer.class);

    private final ClickHouseClient clickHouseClient;
    private final ObjectMapper objectMapper;
    private final IngestionMetrics metrics;

    public ClickHouseTelemetryConsumer(
            ClickHouseClient clickHouseClient,
            ObjectMapper objectMapper,
            IngestionMetrics metrics
    ) {
        this.clickHouseClient = clickHouseClient;
        this.objectMapper = objectMapper;
        this.metrics = metrics;
    }

    @KafkaListener(
            topics = "${platform.ingestion.kafka.topic:telemetry.numeric.v1}",
            groupId = "${spring.kafka.consumer.group-id:platform-core-group}",
            autoStartup = "${platform.telemetry.consumer.enabled:true}"
    )
    public void consume(ConsumerRecord<String, String> record) {
        try {
            CanonicalObservation obs = objectMapper.readValue(record.value(), CanonicalObservation.class);
            clickHouseClient.insertNumericBatch(List.of(obs));
        } catch (Exception e) {
            log.error("Failed to process telemetry message from Kafka topic={} partition={} offset={}: {}",
                    record.topic(), record.partition(), record.offset(), e.getMessage(), e);
            throw new RuntimeException("ClickHouse consumer error", e);
        }
    }
}
