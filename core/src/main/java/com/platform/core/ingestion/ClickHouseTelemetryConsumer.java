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
    private final long recordProcessingMs;
    private final java.util.function.LongSupplier nanoTime;

    public ClickHouseTelemetryConsumer(
            ClickHouseClient clickHouseClient,
            ObjectMapper objectMapper,
            IngestionMetrics metrics
    ) {
        this(clickHouseClient, objectMapper, metrics, 5, 200);
    }

    public ClickHouseTelemetryConsumer(
            ClickHouseClient clickHouseClient,
            ObjectMapper objectMapper,
            IngestionMetrics metrics,
            int maxRetries,
            long backoffMs
    ) {
        this(clickHouseClient, objectMapper, metrics,
                new TelemetryProcessingBudget(java.util.Map.of(
                        org.apache.kafka.clients.consumer.ConsumerConfig.MAX_POLL_RECORDS_CONFIG, 1),
                        maxRetries, backoffMs), System::nanoTime);
    }

    @Autowired
    public ClickHouseTelemetryConsumer(ClickHouseClient clickHouseClient, ObjectMapper objectMapper,
                                      IngestionMetrics metrics, TelemetryProcessingBudget budget) {
        this(clickHouseClient, objectMapper, metrics, budget, System::nanoTime);
    }

    ClickHouseTelemetryConsumer(ClickHouseClient clickHouseClient, ObjectMapper objectMapper,
                               IngestionMetrics metrics, TelemetryProcessingBudget budget,
                               java.util.function.LongSupplier nanoTime) {
        this.clickHouseClient = clickHouseClient;
        this.objectMapper = objectMapper;
        this.metrics = metrics;
        this.maxRetries = budget.attempts;
        this.backoffMs = budget.backoffMs;
        this.recordProcessingMs = budget.recordProcessingMs;
        this.nanoTime = nanoTime;
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
        long started = nanoTime.getAsLong();
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
            metrics.recordConsumerFailure(record, TelemetryConsumerFailure.POISON);
            metrics.recordConsumerPoisonEvent(record.topic(), record.offset(), failureClass);
            var failure = new IngestionException(
                    org.springframework.http.HttpStatus.BAD_REQUEST,
                    ERR_KAFKA_POISON_EVENT,
                    "Malformed Kafka telemetry message at offset " + record.offset()
            );
            metrics.recordTerminalFailure(record, TelemetryConsumerFailure.POISON, failure);
            throw failure;
        }

        Exception lastException = null;
        boolean persisted = false;
        // long avoids counter wrap even when maxRetries is Integer.MAX_VALUE.
        for (long attempt = 1; attempt <= maxRetries; attempt++) {
            requireProcessingTime(record, started, ClickHouseClient.INSERT_REQUEST_TIMEOUT_MS);
            try {
                insert.run();
                persisted = true;
                break;
            } catch (Exception e) {
                lastException = e;
                var reason = TelemetryConsumerFailure.insertion(e);
                metrics.recordConsumerFailure(record, reason);
                if (!reason.retryableInsert) {
                    if (reason == TelemetryConsumerFailure.SHUTDOWN) Thread.currentThread().interrupt();
                    throw terminalFailure(record, reason, "Insertion stopped; record retained", e);
                }
                final long attemptNumber = attempt;
                final String failureClass = e.getClass().getSimpleName();
                SupportabilityMdc.withEventCode(CoreSupportabilityEventCodes.CLICKHOUSE_INSERT_FAILED, () ->
                        log.warn(
                                "ClickHouse insert attempt {}/{} failed for topic={} partition={} offset={} failureClass={} reasonCode={}",
                                attemptNumber,
                                maxRetries,
                                record.topic(),
                                record.partition(),
                                record.offset(),
                                failureClass,
                                reason.reasonCode,
                                sanitizedCause(e)
                        )
                );
                if (attempt < maxRetries) {
                    try {
                        long delay = retryBackoffMs(backoffMs, attempt);
                        requireProcessingTime(record, started, delay + ClickHouseClient.INSERT_REQUEST_TIMEOUT_MS);
                        Thread.sleep(delay);
                    } catch (InterruptedException ie) {
                        Thread.currentThread().interrupt();
                        throw terminalFailure(record, TelemetryConsumerFailure.SHUTDOWN, "Consumer retry interrupted", ie);
                    }
                }
            }
        }

        if (persisted) {
            // Commit exceptions are terminal, never another persistence attempt or a healthy result.
            requireProcessingTime(record, started, 0);
            try {
                if (acknowledgment == null) {
                    throw new IllegalStateException("Kafka acknowledgment unavailable");
                }
                acknowledgment.acknowledge();
            } catch (Exception commitFailure) {
                var reason = TelemetryConsumerFailure.acknowledgment(commitFailure);
                if (reason == TelemetryConsumerFailure.SHUTDOWN) Thread.currentThread().interrupt();
                throw terminalFailure(record, reason, "Telemetry ACK failed; persistence succeeded, commit uncertain", commitFailure);
            }
            metrics.recordConsumerHealthy(record);
            return;
        }
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
        throw terminalFailure(record, TelemetryConsumerFailure.insertion(lastException),
                "ClickHouse persistence unavailable; stopping offset progression", lastException);
    }

    private void requireProcessingTime(ConsumerRecord<String, String> record, long started, long requiredMs) {
        long elapsedMs = java.util.concurrent.TimeUnit.NANOSECONDS.toMillis(nanoTime.getAsLong() - started);
        if (elapsedMs >= recordProcessingMs || requiredMs > recordProcessingMs - elapsedMs) {
            throw terminalFailure(record, TelemetryConsumerFailure.POLL_BUDGET,
                    "Processing deadline exceeded; listener stopping, record retained", null);
        }
    }

    private RuntimeException terminalFailure(ConsumerRecord<String, String> record, TelemetryConsumerFailure reason,
                                             String message, Throwable cause) {
        // Classify first: sanitized causes intentionally no longer have their original Java types.
        metrics.recordConsumerFailure(record, reason);
        Throwable safeCause = sanitizedCause(cause);
        SupportabilityMdc.withEventCode(reason.reasonCode, () ->
                log.error("Telemetry consumer stopped topic={} partition={} offset={} reasonCode={} retryableInsert={} failureClass={}",
                        record.topic(), record.partition(), record.offset(), reason.reasonCode,
                        reason.retryableInsert, cause == null ? "unknown" : cause.getClass().getSimpleName(), safeCause));
        var failure = new RuntimeException(reason.reasonCode + ": " + message + " at offset " + record.offset(), safeCause);
        metrics.recordTerminalFailure(record, reason, failure);
        return failure;
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
        String details = failure instanceof TelemetryConsumerFailure.InsertFailure typed
                ? typed.reason.reasonCode + "; HTTP status=" + typed.httpStatus : "details redacted";
        var safe = new RuntimeException(failure.getClass().getName() + " [" + details + "]",
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

        for (String field : List.of("tenantId", "entityId", "metricId", "sensorId", "timestamp",
                "sequenceId", "sourceProtocol", "quality", "flags")) {
            if (!root.hasNonNull(field)) {
                throw new IllegalArgumentException("telemetry payload missing required field");
            }
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
