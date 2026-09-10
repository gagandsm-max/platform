package com.platform.core.agentcontrol;

import org.springframework.stereotype.Component;

import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Low-cardinality Agent→Sensor assignment metrics (reason-class counters + optional latency totals).
 * Never labels with tenant, agent id, sensor id, or assignment id.
 */
@Component
public class AssignmentMetrics {

    public static final String REASON_ASSIGNED = "ASSIGNED";
    public static final String REASON_REASSIGNED = "REASSIGNED";
    public static final String REASON_REVOKED = "REVOKED";
    public static final String REASON_CONFLICT = "CONFLICT";
    public static final String REASON_SENSOR_INELIGIBLE = "SENSOR_INELIGIBLE";
    public static final String REASON_SITE_INCOMPATIBLE = "SITE_INCOMPATIBLE";
    public static final String REASON_TENANT_MISMATCH = "TENANT_MISMATCH";
    public static final String REASON_AGENT_NOT_ACTIVE = "AGENT_NOT_ACTIVE";
    public static final String REASON_ASSIGNMENT_ERROR = "ASSIGNMENT_ERROR";

    private final ConcurrentHashMap<String, AtomicLong> reasonCounts = new ConcurrentHashMap<>();
    private final AtomicLong assignmentAttempts = new AtomicLong();
    private final AtomicLong assignmentSuccesses = new AtomicLong();
    private final AtomicLong assignmentFailures = new AtomicLong();
    private final AtomicLong assignmentTotalMs = new AtomicLong();

    public void recordSuccess(String reasonClass, long durationMs) {
        assignmentAttempts.incrementAndGet();
        assignmentSuccesses.incrementAndGet();
        assignmentTotalMs.addAndGet(Math.max(0L, durationMs));
        increment(sanitize(reasonClass, REASON_ASSIGNED));
    }

    public void recordFailure(String reasonClass, long durationMs) {
        assignmentAttempts.incrementAndGet();
        assignmentFailures.incrementAndGet();
        assignmentTotalMs.addAndGet(Math.max(0L, durationMs));
        increment(sanitize(reasonClass, REASON_ASSIGNMENT_ERROR));
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

    public long getAssignmentAttempts() {
        return assignmentAttempts.get();
    }

    public long getAssignmentSuccesses() {
        return assignmentSuccesses.get();
    }

    public long getAssignmentFailures() {
        return assignmentFailures.get();
    }

    public long getAssignmentTotalMs() {
        return assignmentTotalMs.get();
    }

    public long getReasonCount(String reasonClass) {
        AtomicLong counter = reasonCounts.get(sanitize(reasonClass, REASON_ASSIGNMENT_ERROR));
        return counter == null ? 0L : counter.get();
    }

    public Map<String, Long> snapshotReasonCounts() {
        Map<String, Long> out = new ConcurrentHashMap<>();
        reasonCounts.forEach((k, v) -> out.put(k, v.get()));
        return Map.copyOf(out);
    }
}
