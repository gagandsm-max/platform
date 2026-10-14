package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalCounterObservation;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.logging.CoreSupportabilityEventCodes;
import com.platform.core.logging.SupportabilityMdc;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.kafka.support.Acknowledgment;
import org.springframework.stereotype.Component;

import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Arrays;
import java.util.Collections;
import java.util.IdentityHashMap;
import java.util.Set;

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
        if (maxRetries < 1 || backoffMs < 0) {
            throw new IllegalArgumentException("Telemetry consumer requires positive attempts and nonnegative backoff");
        }
        this.clickHouseClient = clickHouseClient;
        this.objectMapper = objectMapper;
        this.metrics = metrics;
        this.maxRetries = maxRetries;
        this.backoffMs = backoffMs;
    }

    @KafkaListener(
            id = "telemetryNumericListener",
            idIsGroup = false,
            topics = "${platform.ingestion.kafka.topic:telemetry.numeric.v1}",
            groupId = "${spring.kafka.consumer.group-id:platform-core-group}",
            autoStartup = "${platform.telemetry.consumer.enabled:false}",
            containerFactory = "telemetryKafkaListenerContainerFactory"
    )
    public void consume(ConsumerRecord<String, String> record, Acknowledgment acknowledgment) {
        final Runnable insert;
        try {
            insert = resolveInsert(record.value());
        } catch (Exception e) {
            String failureClass = e.getClass().getSimpleName();
            int payloadBytes = record.value() == null
                    ? 0
                    : record.value().getBytes(StandardCharsets.UTF_8).length;
            SupportabilityMdc.withEventCode(CoreSupportabilityEventCodes.TELEMETRY_POISON_EVENT, () ->
                    log.error(
                            "Poison/malformed telemetry message from Kafka topic={} partition={} offset={} failureClass={} payloadBytes={} code={}. Listener stopping; record retained. Correct the cause before explicit operator restart.",
                            record.topic(),
                            record.partition(),
                            record.offset(),
                            failureClass,
                            payloadBytes,
                            ERR_KAFKA_POISON_EVENT
                    )
            );
            metrics.recordConsumerPoisonEvent(record.topic(), record.offset(), failureClass);
            throw new IngestionException(
                    org.springframework.http.HttpStatus.BAD_REQUEST,
                    ERR_KAFKA_POISON_EVENT,
                    "Malformed Kafka telemetry message at offset " + record.offset()
            );
        }

        Exception lastException = null;
        // long avoids counter wrap even when maxRetries is Integer.MAX_VALUE.
        for (long attempt = 1; attempt <= maxRetries; attempt++) {
            try {
                insert.run();
                if (acknowledgment != null) {
                    acknowledgment.acknowledge();
                }
                metrics.recordConsumerHealthy();
                return;
            } catch (Exception e) {
                lastException = e;
                final long attemptNumber = attempt;
                final String failureClass = e.getClass().getSimpleName();
                SupportabilityMdc.withEventCode(CoreSupportabilityEventCodes.CLICKHOUSE_INSERT_FAILED, () ->
                        log.warn(
                                "ClickHouse insert attempt {}/{} failed for topic={} partition={} offset={} failureClass={}",
                                attemptNumber,
                                maxRetries,
                                record.topic(),
                                record.partition(),
                                record.offset(),
                                failureClass,
                                sanitizedCause(e)
                        )
                );
                if (attempt < maxRetries) {
                    try {
                        Thread.sleep(retryBackoffMs(backoffMs, attempt));
                    } catch (InterruptedException ie) {
                        Thread.currentThread().interrupt();
                        metrics.recordConsumerDegraded(CoreSupportabilityEventCodes.TELEMETRY_CLICKHOUSE_UNAVAILABLE);
                        throw new RuntimeException("Consumer retry interrupted", sanitizedCause(ie));
                    }
                }
            }
        }

        metrics.recordConsumerDegraded(CoreSupportabilityEventCodes.TELEMETRY_CLICKHOUSE_UNAVAILABLE);
        final String exhaustedFailureClass = lastException == null
                ? "unknown"
                : lastException.getClass().getSimpleName();
        SupportabilityMdc.withEventCode(CoreSupportabilityEventCodes.TELEMETRY_RETRIES_EXHAUSTED, () ->
                log.error(
                        "ClickHouse retries exhausted for topic={} partition={} offset={} attempts={} failureClass={}. Listener stopping; record retained and offset will NOT progress. Restore persistence before explicit operator restart.",
                        record.topic(),
                        record.partition(),
                        record.offset(),
                        maxRetries,
                        exhaustedFailureClass
                )
        );
        throw new RuntimeException(
                "ClickHouse persistence unavailable; stopping offset progression at offset " + record.offset(),
                sanitizedCause(lastException)
        );
    }

    // Saturate before shifting/multiplying; Java masks large shift distances and signed
    // overflow would otherwise produce negative sleeps or unexpectedly restart backoff.
    static long retryBackoffMs(long baseMs, long failedAttempt) {
        if (baseMs == 0) {
            return 0;
        }
        long exponent = failedAttempt - 1;
        if (exponent >= 63 || baseMs > (Long.MAX_VALUE >> exponent)) {
            return Long.MAX_VALUE;
        }
        return baseMs << exponent;
    }

    // Never attach the raw persistence cause: ClickHouse/Jackson messages and suppressed
    // exceptions may contain credentials or payloads. Retain bounded class/stack evidence.
    private static Throwable sanitizedCause(Throwable failure) {
        Set<Throwable> seen = Collections.newSetFromMap(new IdentityHashMap<>());
        return sanitizedCause(failure, seen, 0);
    }

    private static Throwable sanitizedCause(Throwable failure, Set<Throwable> seen, int depth) {
        if (failure == null || depth >= 8 || !seen.add(failure)) {
            return null;
        }
        var safe = new RuntimeException(failure.getClass().getName() + " [details redacted]",
                sanitizedCause(failure.getCause(), seen, depth + 1));
        StackTraceElement[] frames = failure.getStackTrace();
        safe.setStackTrace(Arrays.copyOf(frames, Math.min(frames.length, 32)));
        return safe;
    }

    private Runnable resolveInsert(String payload) throws Exception {
        JsonNode root = objectMapper.readTree(payload);
        if (root == null || !root.isObject()) {
            throw new IllegalArgumentException("telemetry payload is not a JSON object");
        }

        String kind = root.path("observationKind").asText("").trim();
        if (CanonicalCounterObservation.KIND.equals(kind) || root.hasNonNull("counterValue")) {
            CanonicalCounterObservation counter = objectMapper.treeToValue(root, CanonicalCounterObservation.class);
            if (counter.counterValue() == null || counter.counterValue().isBlank()) {
                throw new IllegalArgumentException("Counter64 payload missing counterValue");
            }
            // Counter64 has one authoritative exact value field. Any ordinary numeric
            // value alias makes the event ambiguous and must fail closed.
            if (root.has("value")) {
                throw new IllegalArgumentException("Counter64 payload must not also contain numeric value");
            }
            return () -> {
                try {
                    clickHouseClient.insertCounterBatch(List.of(counter));
                } catch (Exception e) {
                    throw new RuntimeException(e);
                }
            };
        }

        if (!root.has("value") || !root.get("value").isNumber()) {
            throw new IllegalArgumentException("numeric telemetry payload missing numeric value");
        }
        CanonicalObservation obs = objectMapper.treeToValue(root, CanonicalObservation.class);
        return () -> {
            try {
                clickHouseClient.insertNumericBatch(List.of(obs));
            } catch (Exception e) {
                throw new RuntimeException(e);
            }
        };
    }
}
