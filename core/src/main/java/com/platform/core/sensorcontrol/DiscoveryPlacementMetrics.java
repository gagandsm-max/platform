package com.platform.core.sensorcontrol;

import org.springframework.stereotype.Component;

import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Low-cardinality discovery placement metrics (reason-class counters + simple latency totals).
 * Never labels with tenant, sensor id, or fingerprint.
 */
@Component
public class DiscoveryPlacementMetrics {

    public static final String REASON_ASSIGNED = "ASSIGNED";
    public static final String REASON_NOT_PLACEABLE = "NOT_PLACEABLE";
    public static final String REASON_NO_ELIGIBLE_SENSOR = "NO_ELIGIBLE_SENSOR";
    public static final String REASON_PLACEMENT_ERROR = "PLACEMENT_ERROR";

    private final ConcurrentHashMap<String, AtomicLong> reasonCounts = new ConcurrentHashMap<>();
    private final AtomicLong placementAttempts = new AtomicLong();
    private final AtomicLong placementSuccesses = new AtomicLong();
    private final AtomicLong placementFailures = new AtomicLong();
    private final AtomicLong placementTotalMs = new AtomicLong();

    public void recordSuccess(String reasonClass, long durationMs) {
        placementAttempts.incrementAndGet();
        placementSuccesses.incrementAndGet();
        placementTotalMs.addAndGet(Math.max(0L, durationMs));
        increment(sanitize(reasonClass, REASON_ASSIGNED));
    }

    public void recordFailure(String reasonClass, long durationMs) {
        placementAttempts.incrementAndGet();
        placementFailures.incrementAndGet();
        placementTotalMs.addAndGet(Math.max(0L, durationMs));
        increment(sanitize(reasonClass, REASON_PLACEMENT_ERROR));
    }

    private void increment(String reasonClass) {
        reasonCounts.computeIfAbsent(reasonClass, ignored -> new AtomicLong()).incrementAndGet();
    }

    private static String sanitize(String reasonClass, String fallback) {
        if (reasonClass == null || reasonClass.isBlank()) {
            return fallback;
        }
        return reasonClass.trim();
    }

    public long getPlacementAttempts() {
        return placementAttempts.get();
    }

    public long getPlacementSuccesses() {
        return placementSuccesses.get();
    }

    public long getPlacementFailures() {
        return placementFailures.get();
    }

    public long getPlacementTotalMs() {
        return placementTotalMs.get();
    }

    public long getReasonCount(String reasonClass) {
        AtomicLong counter = reasonCounts.get(sanitize(reasonClass, REASON_PLACEMENT_ERROR));
        return counter == null ? 0L : counter.get();
    }

    public Map<String, Long> snapshotReasonCounts() {
        Map<String, Long> out = new ConcurrentHashMap<>();
        reasonCounts.forEach((k, v) -> out.put(k, v.get()));
        return Map.copyOf(out);
    }
}
