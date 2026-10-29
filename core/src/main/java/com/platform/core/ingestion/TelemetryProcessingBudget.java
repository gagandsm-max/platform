package com.platform.core.ingestion;

import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.common.config.ConfigDef;

import java.util.Map;

/** Fail-closed budget for a complete poll, including synchronous offset commits. */
final class TelemetryProcessingBudget {
    static final long POLL_TIMEOUT_MS = 5_000;
    static final long COMMIT_TIMEOUT_MS = 5_000;
    // Serialization, parsing, logging and scheduling allowance; checked at runtime as well.
    static final long PROCESSING_OVERHEAD_MS = 1_000;
    final int attempts;
    final long backoffMs;
    final long recordProcessingMs;
    final int maxPollIntervalMs;
    final int maxPollRecords;

    TelemetryProcessingBudget(Map<String, Object> effective, int attempts, long backoffMs) {
        if (attempts < 1 || backoffMs < 0) {
            throw new IllegalArgumentException("Telemetry consumer requires positive attempts and nonnegative backoff");
        }
        this.attempts = attempts;
        this.backoffMs = backoffMs;
        maxPollIntervalMs = effectiveInt(effective, ConsumerConfig.MAX_POLL_INTERVAL_MS_CONFIG);
        maxPollRecords = effectiveInt(effective, ConsumerConfig.MAX_POLL_RECORDS_CONFIG);
        try {
            long processing = Math.addExact(PROCESSING_OVERHEAD_MS, Math.multiplyExact((long) attempts,
                    ClickHouseClient.INSERT_REQUEST_TIMEOUT_MS + PROCESSING_OVERHEAD_MS));
            // At most 63 nonzero exponential terms fit in a signed long; never loop attempts times.
            if (backoffMs != 0) {
                if (attempts > 64) {
                    throw new ArithmeticException("exponential backoff overflow");
                }
                long delay = backoffMs;
                for (int failed = 1; failed < attempts; failed++) {
                    processing = Math.addExact(processing, delay);
                    if (failed < attempts - 1) {
                        delay = Math.multiplyExact(delay, 2);
                    }
                }
            }
            recordProcessingMs = processing;
            long pollBudget = Math.addExact(POLL_TIMEOUT_MS,
                    Math.multiplyExact((long) maxPollRecords, Math.addExact(processing, COMMIT_TIMEOUT_MS)));
            if (pollBudget >= maxPollIntervalMs) {
                throw new IllegalArgumentException("ERR_KAFKA_POLL_BUDGET: required poll budget ms=" + pollBudget
                        + " must be less than effective max.poll.interval.ms=" + maxPollIntervalMs
                        + "; max.poll.records=" + maxPollRecords);
            }
        } catch (ArithmeticException overflow) {
            throw new IllegalArgumentException("ERR_KAFKA_POLL_BUDGET: processing budget overflow");
        }
    }

    private static int effectiveInt(Map<String, Object> effective, String key) {
        Object value = effective.getOrDefault(key, ConsumerConfig.configDef().configKeys().get(key).defaultValue);
        int parsed = (Integer) ConfigDef.parseType(key, value, ConfigDef.Type.INT);
        if (parsed < 1) {
            throw new IllegalArgumentException("ERR_KAFKA_POLL_BUDGET: " + key + " must be positive");
        }
        return parsed;
    }
}
