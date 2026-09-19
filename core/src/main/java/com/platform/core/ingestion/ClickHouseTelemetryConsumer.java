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

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.HexFormat;
import java.util.List;

@Component
public class ClickHouseTelemetryConsumer {
    private static final Logger log = LoggerFactory.getLogger(ClickHouseTelemetryConsumer.class);
    private static final String ERR_KAFKA_POISON_EVENT = "ERR_KAFKA_POISON_EVENT";

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
    public void consumeBatch(List<ConsumerRecord<String, String>> records, Acknowledgment acknowledgment) {
        consumeRecords(records, acknowledgment);
    }

    // Package-visible compatibility seam for focused unit tests and direct callers.
    void consume(ConsumerRecord<String, String> record, Acknowledgment acknowledgment) {
        consumeRecords(List.of(record), acknowledgment);
    }

    private void consumeRecords(List<ConsumerRecord<String, String>> records, Acknowledgment acknowledgment) {
        if (records == null || records.isEmpty()) {
            return;
        }

        List<CanonicalObservation> observations = new ArrayList<>(records.size());
        for (ConsumerRecord<String, String> record : records) {
            observations.add(parseObservation(record));
        }

        String deduplicationToken = deduplicationToken(records);
        ConsumerRecord<String, String> firstRecord = records.get(0);

        // Bounded retry and backoff against ClickHouse. The deterministic insert token makes
        // retry of the same Kafka poll idempotent within ClickHouse's configured dedup window.
        Exception lastException = null;
        for (int attempt = 1; attempt <= maxRetries; attempt++) {
            try {
                clickHouseClient.insertNumericBatch(observations, deduplicationToken);
                // Persistence succeeded: commit offsets for the complete poll batch.
                if (acknowledgment != null) {
                    acknowledgment.acknowledge();
                }
                metrics.recordConsumerHealthy();
                return;
            } catch (Exception e) {
                lastException = e;
                log.warn(
                        "ClickHouse batch insert attempt {}/{} failed batchSize={} firstTopic={} firstPartition={} firstOffset={} failureClass={}",
                        attempt,
                        maxRetries,
                        records.size(),
                        firstRecord.topic(),
                        firstRecord.partition(),
                        firstRecord.offset(),
                        e.getClass().getSimpleName()
                );
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

        String failureClass = lastException == null ? "Unknown" : lastException.getClass().getSimpleName();
        String failureReason = "ClickHouse unavailable after " + maxRetries + " attempts: " + failureClass;
        metrics.recordConsumerDegraded(failureReason);
        log.error(
                "ClickHouse retries exhausted batchSize={} firstTopic={} firstPartition={} firstOffset={}. Offsets will NOT progress. failureClass={}",
                records.size(),
                firstRecord.topic(),
                firstRecord.partition(),
                firstRecord.offset(),
                failureClass
        );
        throw new RuntimeException(
                "ClickHouse persistence unavailable; stopping offset progression for batch starting at offset "
                        + firstRecord.offset(),
                lastException
        );
    }

    private CanonicalObservation parseObservation(ConsumerRecord<String, String> record) {
        try {
            return objectMapper.readValue(record.value(), CanonicalObservation.class);
        } catch (Exception e) {
            String failureClass = e.getClass().getSimpleName();
            int payloadBytes = record.value() == null
                    ? 0
                    : record.value().getBytes(StandardCharsets.UTF_8).length;
            // Never log record.value(), payload fragments, or e.getMessage() (may echo attacker content).
            log.error(
                    "Poison/malformed telemetry message from Kafka topic={} partition={} offset={} failureClass={} payloadBytes={} code={}",
                    record.topic(),
                    record.partition(),
                    record.offset(),
                    failureClass,
                    payloadBytes,
                    ERR_KAFKA_POISON_EVENT
            );
            metrics.recordConsumerPoisonEvent(record.topic(), record.offset(), failureClass);
            throw new IngestionException(
                    org.springframework.http.HttpStatus.BAD_REQUEST,
                    ERR_KAFKA_POISON_EVENT,
                    "Malformed Kafka telemetry message at offset " + record.offset()
            );
        }
    }

    private static void updateInt(MessageDigest digest, int value) {
        digest.update((byte) (value >>> 24));
        digest.update((byte) (value >>> 16));
        digest.update((byte) (value >>> 8));
        digest.update((byte) value);
    }

    private static void updateLong(MessageDigest digest, long value) {
        for (int shift = 56; shift >= 0; shift -= 8) {
            digest.update((byte) (value >>> shift));
        }
    }

    static String deduplicationToken(List<ConsumerRecord<String, String>> records) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            for (ConsumerRecord<String, String> record : records) {
                digest.update(record.topic().getBytes(StandardCharsets.UTF_8));
                digest.update((byte) 0);
                updateInt(digest, record.partition());
                updateLong(digest, record.offset());
            }
            return HexFormat.of().formatHex(digest.digest());
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException("SHA-256 unavailable for ClickHouse deduplication token", e);
        }
    }

}