package com.platform.core.agentcontrol;

import org.springframework.stereotype.Component;

import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Low-cardinality Agent authority metrics (reason-class counters only).
 * Never labels with tenant, agent id, sensor id, or fingerprint.
 */
@Component
public class AgentAuthorityMetrics {

    public static final String REASON_RESOLVED = "RESOLVED";
    public static final String REASON_UNKNOWN_FINGERPRINT = "UNKNOWN_FINGERPRINT";
    public static final String REASON_CREDENTIAL_EXPIRED = "CREDENTIAL_EXPIRED";
    public static final String REASON_CREDENTIAL_NOT_YET_VALID = "CREDENTIAL_NOT_YET_VALID";
    public static final String REASON_CREDENTIAL_REVOKED = "CREDENTIAL_REVOKED";
    public static final String REASON_CREDENTIAL_SUPERSEDED = "CREDENTIAL_SUPERSEDED";
    public static final String REASON_AGENT_DISABLED = "AGENT_DISABLED";
    public static final String REASON_AGENT_REVOKED = "AGENT_REVOKED";
    public static final String REASON_ASSIGNMENT_MISSING = "ASSIGNMENT_MISSING";
    public static final String REASON_SENSOR_MISMATCH = "SENSOR_MISMATCH";
    public static final String REASON_TENANT_MISMATCH = "TENANT_MISMATCH";
    public static final String REASON_STALE_GENERATION = "STALE_GENERATION";
    public static final String REASON_DB_ERROR = "DB_ERROR";
    public static final String REASON_AUTHORITY_UNAVAILABLE = "AUTHORITY_UNAVAILABLE";

    private final ConcurrentHashMap<String, AtomicLong> reasonCounts = new ConcurrentHashMap<>();
    private final AtomicLong resolveAttempts = new AtomicLong();
    private final AtomicLong resolveSuccesses = new AtomicLong();
    private final AtomicLong resolveFailures = new AtomicLong();
    private final AtomicLong resolveTotalMs = new AtomicLong();

    public void recordResolved(long durationMs) {
        resolveAttempts.incrementAndGet();
        resolveSuccesses.incrementAndGet();
        resolveTotalMs.addAndGet(Math.max(0L, durationMs));
        increment(REASON_RESOLVED);
    }

    public void recordRejection(String reasonClass, long durationMs) {
        resolveAttempts.incrementAndGet();
        resolveFailures.incrementAndGet();
        resolveTotalMs.addAndGet(Math.max(0L, durationMs));
        increment(sanitize(reasonClass));
    }

    private void increment(String reasonClass) {
        reasonCounts.computeIfAbsent(reasonClass, ignored -> new AtomicLong()).incrementAndGet();
    }

    private static String sanitize(String reasonClass) {
        if (reasonClass == null || reasonClass.isBlank()) {
            return REASON_AUTHORITY_UNAVAILABLE;
        }
        return reasonClass.trim();
    }

    public long getResolveAttempts() {
        return resolveAttempts.get();
    }

    public long getResolveSuccesses() {
        return resolveSuccesses.get();
    }

    public long getResolveFailures() {
        return resolveFailures.get();
    }

    public long getResolveTotalMs() {
        return resolveTotalMs.get();
    }

    public long getReasonCount(String reasonClass) {
        AtomicLong counter = reasonCounts.get(sanitize(reasonClass));
        return counter == null ? 0L : counter.get();
    }

    public Map<String, Long> snapshotReasonCounts() {
        Map<String, Long> out = new ConcurrentHashMap<>();
        reasonCounts.forEach((k, v) -> out.put(k, v.get()));
        return Map.copyOf(out);
    }
}
