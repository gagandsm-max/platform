package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalCounterObservation;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryObservationBatch;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpStatus;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Component;

import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

@Component
public class KafkaTelemetryProducer {
    private static final Logger log = LoggerFactory.getLogger(KafkaTelemetryProducer.class);
    public static final String TOPIC_TELEMETRY_NUMERIC = "telemetry.numeric.v1";

    private final KafkaTemplate<String, String> kafkaTemplate;
    private final ObjectMapper objectMapper;
    private final IngestionMetrics metrics;
    private final long timeoutSeconds;
    private final String topic;

    @Autowired
    public KafkaTelemetryProducer(
            KafkaTemplate<String, String> kafkaTemplate,
            ObjectMapper objectMapper,
            IngestionMetrics metrics,
            @Value("${platform.kafka.publish-timeout-seconds:5}") long timeoutSeconds
    ) {
        this.kafkaTemplate = kafkaTemplate;
        this.objectMapper = objectMapper;
        this.metrics = metrics;
        this.timeoutSeconds = timeoutSeconds;
        this.topic = TOPIC_TELEMETRY_NUMERIC;
    }

    public boolean isTransactionCapable() {
        return kafkaTemplate != null && kafkaTemplate.isTransactional();
    }

    /**
     * Publishes all numeric and Counter64 observations of a batch in one atomic Kafka transaction.
     * Mixed batches remain inside the same transactional boundary (PR #81).
     * Agent numeric-only callers should wrap with {@link TelemetryObservationBatch#ofNumeric(List)}.
     */
    public void publishBatchTransactionally(TelemetryObservationBatch batch) {
        if (batch == null || batch.isEmpty()) {
            return;
        }

        if (!isTransactionCapable()) {
            metrics.recordKafkaFailure("Kafka producer is not transaction-capable");
            throw new IngestionException(
                    HttpStatus.SERVICE_UNAVAILABLE,
                    "ERR_KAFKA_TRANSACTIONS_UNAVAILABLE",
                    "Kafka producer is not configured for transactions; atomic batch publication is required"
            );
        }

        long startTime = System.currentTimeMillis();
        try {
            kafkaTemplate.executeInTransaction(operations -> {
                try {
                    for (CanonicalObservation obs : batch.numericObservations()) {
                        sendOne(operations, obs.sequenceId(), objectMapper.writeValueAsString(obs));
                    }
                    for (CanonicalCounterObservation obs : batch.counterObservations()) {
                        sendOne(operations, obs.sequenceId(), objectMapper.writeValueAsString(obs));
                    }
                } catch (com.fasterxml.jackson.core.JsonProcessingException e) {
                    throw new RuntimeException("Failed to serialize telemetry observation for Kafka", e);
                }
                return true;
            });
            metrics.recordKafkaPublish(batch.size(), System.currentTimeMillis() - startTime);
        } catch (Exception e) {
            long duration = System.currentTimeMillis() - startTime;
            log.error("Kafka atomic batch publication failed after {}ms for topic={} count={}: {}",
                    duration, topic, batch.size(), e.getMessage());
            metrics.recordKafkaFailure(e.getMessage());
            throw new IngestionException(
                    HttpStatus.SERVICE_UNAVAILABLE,
                    "ERR_KAFKA_PUBLICATION_FAILED",
                    "Failed to atomically publish telemetry batch to Kafka cluster",
                    e
            );
        }
    }

    private void sendOne(
            org.springframework.kafka.core.KafkaOperations<String, String> operations,
            String key,
            String payload
    ) {
        try {
            CompletableFuture<?> future = operations.send(topic, key, payload);
            future.get(timeoutSeconds, TimeUnit.SECONDS);
        } catch (Exception e) {
            throw new RuntimeException(
                    "Failed to send observation to Kafka topic=" + topic + " key=" + key + ": " + e.getMessage(),
                    e
            );
        }
    }
}
