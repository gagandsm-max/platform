package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.kafka.support.Acknowledgment;
import org.springframework.stereotype.Component;

import java.util.List;

@Component
public class ClickHouseTelemetryConsumer {
    private static final Logger log = LoggerFactory.getLogger(ClickHouseTelemetryConsumer.class);

    private final ClickHouseClient clickHouseClient;
    private final ObjectMapper objectMapper;
    private final IngestionMetrics metrics;
    private final int maxRetries;
    private final long backoffMs;

    public ClickHouseTelemetryConsumer(
            ClickHouseClient clickHouseClient,
            ObjectMapper objectMapper,
            IngestionMetrics metrics
    ) {
        this(clickHouseClient, objectMapper, metrics, 5, 200);
    }

    @Autowired
    public ClickHouseTelemetryConsumer(
            ClickHouseClient clickHouseClient,
            ObjectMapper objectMapper,
            IngestionMetrics metrics,
            @Value("${platform.telemetry.consumer.max-retries:5}") int maxRetries,
            @Value("${platform.telemetry.consumer.backoff-ms:200}") long backoffMs
    ) {
        this.clickHouseClient = clickHouseClient;
        this.objectMapper = objectMapper;
        this.metrics = metrics;
        this.maxRetries = maxRetries;
        this.backoffMs = backoffMs;
    }

    @KafkaListener(
            topics = "${platform.ingestion.kafka.topic:telemetry.numeric.v1}",
            groupId = "${spring.kafka.consumer.group-id:platform-core-group}",
            autoStartup = "${platform.telemetry.consumer.enabled:false}",
            containerFactory = "telemetryKafkaListenerContainerFactory"
    )
    public void consume(ConsumerRecord<String, String> record, Acknowledgment acknowledgment) {
        CanonicalObservation obs;
        try {
            obs = objectMapper.readValue(record.value(), CanonicalObservation.class);
        } catch (Exception e) {
            // Never log record.value() — payloads may contain sensitive operational content.
            String safeCorrelation = extractSafeCorrelationHint(record.value());
            log.error("Poison/malformed telemetry message from Kafka topic={} partition={} offset={} errorClass={}{}: {}",
                    record.topic(),
                    record.partition(),
                    record.offset(),
                    e.getClass().getSimpleName(),
                    safeCorrelation.isEmpty() ? "" : " correlationHint=" + safeCorrelation,
                    e.getMessage());
            metrics.recordConsumerPoisonEvent(record.topic(), record.offset(), e.getMessage());
            // Malformed/poison event must not silently disappear.
            // Do NOT acknowledge; throw exception so offset does not progress past poison event.
            throw new IngestionException(
                    org.springframework.http.HttpStatus.BAD_REQUEST,
                    "ERR_KAFKA_POISON_EVENT",
                    "Malformed Kafka telemetry message at offset " + record.offset() + ": " + e.getMessage(),
                    e
            );
        }

        // Bounded retry and backoff against ClickHouse
        Exception lastException = null;
        for (int attempt = 1; attempt <= maxRetries; attempt++) {
            try {
                clickHouseClient.insertNumericBatch(List.of(obs));
                // Insertion succeeded: commit offset and update health
                if (acknowledgment != null) {
                    acknowledgment.acknowledge();
                }
                metrics.recordConsumerHealthy();
                return;
            } catch (Exception e) {
                lastException = e;
                log.warn("ClickHouse insert attempt {}/{} failed for topic={} partition={} offset={}: {}",
                        attempt, maxRetries, record.topic(), record.partition(), record.offset(), e.getMessage());
                if (attempt < maxRetries) {
                    try {
                        Thread.sleep(backoffMs * (1L << (attempt - 1)));
                    } catch (InterruptedException ie) {
                        Thread.currentThread().interrupt();
                        throw new RuntimeException("Consumer retry interrupted", ie);
                    }
                }
            }
        }

        // Retries exhausted: do NOT acknowledge offset. Surface degraded state.
        String failureReason = "ClickHouse unavailable after " + maxRetries + " attempts: " + lastException.getMessage();
        metrics.recordConsumerDegraded(failureReason);
        log.error("ClickHouse retries exhausted for topic={} partition={} offset={}. Offset will NOT progress.",
                record.topic(), record.partition(), record.offset(), lastException);
        throw new RuntimeException("ClickHouse persistence unavailable; stopping offset progression at offset " + record.offset(), lastException);
    }

    /**
     * Best-effort extraction of a bounded correlation hint without logging the full payload.
     * Returns empty when the payload is not safely parseable as a small JSON object field.
     */
    String extractSafeCorrelationHint(String payload) {
        if (payload == null || payload.isBlank() || payload.length() > 4096) {
            return "";
        }
        try {
            var node = objectMapper.readTree(payload);
            if (node != null && node.hasNonNull("correlationId")) {
                String corr = node.path("correlationId").asText("").trim();
                if (!corr.isEmpty() && corr.length() <= 128) {
                    return corr;
                }
            }
        } catch (Exception ignored) {
            // Not parseable — no correlation hint.
        }
        return "";
    }
}
