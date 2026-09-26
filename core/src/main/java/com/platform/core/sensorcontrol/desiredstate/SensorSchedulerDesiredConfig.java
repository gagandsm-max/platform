package com.platform.core.sensorcontrol.desiredstate;

import java.util.Collections;
import java.util.EnumMap;
import java.util.Map;
import java.util.Objects;
import java.util.TreeMap;

public record SensorSchedulerDesiredConfig(
        int maxWorkers,
        int maxQueueDepth,
        int maxTrackedAssignments,
        int maxTrackedJobs,
        int maxPayloadBytes,
        int defaultExecutionTimeoutMs,
        int shutdownGracePeriodMs,
        int retryMaxAttempts,
        int retryInitialBackoffMs,
        int retryMaxBackoffMs,
        int retryJitterBasisPoints,
        Map<SensorSchedulerPoolName, SensorSchedulerPoolConfig> pools
) {
    public SensorSchedulerDesiredConfig {
        Objects.requireNonNull(pools, "pools");
        if (pools.isEmpty() || pools.size() > SensorControlDesiredLimits.MAX_POOL_COUNT) {
            throw new IllegalArgumentException("scheduler.pools count out of bounds");
        }
        // Defensive copy sorted by enum ordinal for stable iteration elsewhere.
        EnumMap<SensorSchedulerPoolName, SensorSchedulerPoolConfig> copy =
                new EnumMap<>(SensorSchedulerPoolName.class);
        copy.putAll(pools);
        pools = Collections.unmodifiableMap(copy);
    }

    /** Pools sorted by pool name for checksum framing. */
    public Map<String, SensorSchedulerPoolConfig> poolsSortedByName() {
        TreeMap<String, SensorSchedulerPoolConfig> sorted = new TreeMap<>();
        for (Map.Entry<SensorSchedulerPoolName, SensorSchedulerPoolConfig> e : pools.entrySet()) {
            sorted.put(e.getKey().name(), e.getValue());
        }
        return Collections.unmodifiableMap(sorted);
    }
}
