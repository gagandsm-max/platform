package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpStatus;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.kafka.support.SendResult;
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
    }

    public void publishSynchronously(List<CanonicalObservation> observations) {
        if (observations == null || observations.isEmpty()) {
            return;
        }

        for (CanonicalObservation obs : observations) {
            String key = obs.tenantId() + ":" + obs.entityId();
            try {
                String payload = objectMapper.writeValueAsString(obs);
                CompletableFuture<SendResult<String, String>> future = kafkaTemplate.send(TOPIC_TELEMETRY_NUMERIC, key, payload);
                future.get(timeoutSeconds, TimeUnit.SECONDS);
            } catch (Exception e) {
                log.error("Kafka publication failed for topic={} key={}: {}", TOPIC_TELEMETRY_NUMERIC, key, e.getMessage());
                metrics.recordKafkaFailure(e.getMessage());
                throw new IngestionException(HttpStatus.SERVICE_UNAVAILABLE, "ERR_KAFKA_PUBLICATION_FAILED",
                        "Failed to publish telemetry to Kafka cluster: " + e.getMessage(), e);
            }
        }
    }
}
