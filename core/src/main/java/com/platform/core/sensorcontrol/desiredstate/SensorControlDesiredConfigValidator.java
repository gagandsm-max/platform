package com.platform.core.sensorcontrol.desiredstate;

import com.platform.core.domain.DomainException;

import java.util.EnumMap;
import java.util.Map;

/**
 * Validates typed desired-config v1 against centralized finite resource ceilings.
 */
public final class SensorControlDesiredConfigValidator {

    private SensorControlDesiredConfigValidator() {}

    public static void validate(SensorRuntimeDesiredConfig config) {
        if (config == null) {
            throw new DomainException("VALIDATION_FAILED", "configuration is required");
        }
        validateScheduler(config.scheduler());
        validateTransport(config.transport());
    }

    private static void validateScheduler(SensorSchedulerDesiredConfig s) {
        requirePositive("scheduler.maxWorkers", s.maxWorkers(), SensorControlDesiredLimits.MAX_WORKERS);
        requirePositive("scheduler.maxQueueDepth", s.maxQueueDepth(), SensorControlDesiredLimits.MAX_QUEUE_DEPTH);
        requirePositive("scheduler.maxTrackedAssignments", s.maxTrackedAssignments(),
                SensorControlDesiredLimits.MAX_TRACKED_ASSIGNMENTS);
        requirePositive("scheduler.maxTrackedJobs", s.maxTrackedJobs(), SensorControlDesiredLimits.MAX_TRACKED_JOBS);
        requirePositive("scheduler.maxPayloadBytes", s.maxPayloadBytes(), SensorControlDesiredLimits.MAX_PAYLOAD_BYTES);
        requirePositive("scheduler.defaultExecutionTimeoutMs", s.defaultExecutionTimeoutMs(),
                SensorControlDesiredLimits.MAX_EXECUTION_TIMEOUT_MS);
        requirePositive("scheduler.shutdownGracePeriodMs", s.shutdownGracePeriodMs(),
                SensorControlDesiredLimits.MAX_SHUTDOWN_GRACE_MS);
        requirePositive("scheduler.retryMaxAttempts", s.retryMaxAttempts(),
                SensorControlDesiredLimits.MAX_RETRY_ATTEMPTS);
        requirePositive("scheduler.retryInitialBackoffMs", s.retryInitialBackoffMs(),
                SensorControlDesiredLimits.MAX_RETRY_BACKOFF_MS);
        requirePositive("scheduler.retryMaxBackoffMs", s.retryMaxBackoffMs(),
                SensorControlDesiredLimits.MAX_RETRY_BACKOFF_MS);
        if (s.retryJitterBasisPoints() < 0
                || s.retryJitterBasisPoints() > SensorControlDesiredLimits.MAX_JITTER_BASIS_POINTS) {
            throw new DomainException("VALIDATION_FAILED",
                    "scheduler.retryJitterBasisPoints must be 0..10000");
        }
        if (s.retryMaxBackoffMs() < s.retryInitialBackoffMs()) {
            throw new DomainException("VALIDATION_FAILED",
                    "scheduler.retryMaxBackoffMs must be >= retryInitialBackoffMs");
        }
        Map<SensorSchedulerPoolName, SensorSchedulerPoolConfig> pools = s.pools();
        if (pools == null || pools.isEmpty()) {
            throw new DomainException("VALIDATION_FAILED", "scheduler.pools is required");
        }
        if (pools.size() > SensorControlDesiredLimits.MAX_POOL_COUNT) {
            throw new DomainException("VALIDATION_FAILED", "scheduler.pools exceeds max pool count");
        }
        int sumWorkers = 0;
        int sumQueues = 0;
        for (Map.Entry<SensorSchedulerPoolName, SensorSchedulerPoolConfig> e : pools.entrySet()) {
            if (e.getKey() == null || e.getValue() == null) {
                throw new DomainException("VALIDATION_FAILED", "scheduler.pools entry is invalid");
            }
            sumWorkers += e.getValue().workers();
            sumQueues += e.getValue().queueDepth();
        }
        if (sumWorkers > s.maxWorkers()) {
            throw new DomainException("VALIDATION_FAILED",
                    "sum of pool.workers exceeds scheduler.maxWorkers");
        }
        if (sumQueues > s.maxQueueDepth()) {
            throw new DomainException("VALIDATION_FAILED",
                    "sum of pool.queueDepth exceeds scheduler.maxQueueDepth");
        }
        if (s.maxTrackedJobs() < s.maxQueueDepth() + s.maxWorkers()) {
            throw new DomainException("VALIDATION_FAILED",
                    "scheduler.maxTrackedJobs must be >= maxQueueDepth + maxWorkers");
        }
    }

    private static void validateTransport(SensorTransportDesiredConfig t) {
        requirePositive("transport.requestTimeoutMs", t.requestTimeoutMs(),
                SensorControlDesiredLimits.MAX_EXECUTION_TIMEOUT_MS);
        requirePositive("transport.maxRequestBytes", t.maxRequestBytes(),
                SensorControlDesiredLimits.MAX_REQUEST_BYTES);
        requirePositive("transport.maxResponseBytes", t.maxResponseBytes(),
                SensorControlDesiredLimits.MAX_RESPONSE_BYTES);
        requirePositive("transport.uploadBatchRecords", t.uploadBatchRecords(),
                SensorControlDesiredLimits.MAX_UPLOAD_BATCH_RECORDS);
        requirePositive("transport.uploadBatchBytes", t.uploadBatchBytes(),
                SensorControlDesiredLimits.MAX_UPLOAD_BATCH_BYTES);
        if (t.uploadBatchBytes() > t.maxRequestBytes()) {
            throw new DomainException("VALIDATION_FAILED",
                    "transport.uploadBatchBytes cannot exceed maxRequestBytes");
        }
    }

    private static void requirePositive(String field, int value, int max) {
        if (value <= 0 || value > max) {
            throw new DomainException("VALIDATION_FAILED", field + " out of bounds");
        }
    }

    /** Build pools map rejecting unknown names (caller supplies already-parsed enum keys). */
    public static Map<SensorSchedulerPoolName, SensorSchedulerPoolConfig> copyPools(
            Map<SensorSchedulerPoolName, SensorSchedulerPoolConfig> pools
    ) {
        EnumMap<SensorSchedulerPoolName, SensorSchedulerPoolConfig> copy =
                new EnumMap<>(SensorSchedulerPoolName.class);
        copy.putAll(pools);
        return copy;
    }
}
