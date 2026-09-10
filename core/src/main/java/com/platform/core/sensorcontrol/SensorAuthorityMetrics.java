package com.platform.core.sensorcontrol;

import org.springframework.stereotype.Component;

import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Low-cardinality Sensor authority metrics (reason-class counters only).
 * Never labels with tenant, sensor id, or fingerprint.
 */
@Component
public class SensorAuthorityMetrics {

    public static final String REASON_RESOLVED = "RESOLVED";
    public static final String REASON_CLIENT_CERT_REQUIRED = "CLIENT_CERT_REQUIRED";
    public static final String REASON_UNKNOWN_FINGERPRINT = "UNKNOWN_FINGERPRINT";
    public static final String REASON_CREDENTIAL_EXPIRED = "CREDENTIAL_EXPIRED";
    public static final String REASON_CREDENTIAL_REVOKED = "CREDENTIAL_REVOKED";
    public static final String REASON_CREDENTIAL_SUPERSEDED = "CREDENTIAL_SUPERSEDED";
    public static final String REASON_SENSOR_DISABLED = "SENSOR_DISABLED";
    public static final String REASON_SENSOR_REVOKED = "SENSOR_REVOKED";
    public static final String REASON_TENANT_MISSING = "TENANT_MISSING";
    public static final String REASON_SITE_INVALID = "SITE_INVALID";
    public static final String REASON_DB_ERROR = "DB_ERROR";
    public static final String REASON_AUTHORITY_UNAVAILABLE = "AUTHORITY_UNAVAILABLE";

    private final ConcurrentHashMap<String, AtomicLong> reasonCounts = new ConcurrentHashMap<>();
    private final AtomicLong resolveAttempts = new AtomicLong();
    private final AtomicLong resolveSuccesses = new AtomicLong();
    private final AtomicLong resolveFailures = new AtomicLong();

    public void recordResolved() {
        resolveAttempts.incrementAndGet();
        resolveSuccesses.incrementAndGet();
        increment(REASON_RESOLVED);
    }

    public void recordRejection(String reasonClass) {
        resolveAttempts.incrementAndGet();
        resolveFailures.incrementAndGet();
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
