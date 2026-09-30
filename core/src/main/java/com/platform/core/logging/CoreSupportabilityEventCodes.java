package com.platform.core.logging;

/**
 * Stable Core operational event codes for BUILD-005 Slice 2 health/logging.
 * Emitted as structured-log {@code event_code}, never as top-level {@code error_code}.
 */
public final class CoreSupportabilityEventCodes {

    public static final String KAFKA_HEALTH_PROBE_FAILED = "CORE_KAFKA_HEALTH_PROBE_FAILED";
    public static final String CLICKHOUSE_HEALTH_PROBE_FAILED = "CORE_CLICKHOUSE_HEALTH_PROBE_FAILED";
    public static final String TELEMETRY_CONSUMER_DEGRADED = "CORE_TELEMETRY_CONSUMER_DEGRADED";
    public static final String TELEMETRY_POISON_EVENT = "CORE_TELEMETRY_POISON_EVENT";
    public static final String TELEMETRY_CLICKHOUSE_UNAVAILABLE = "CORE_TELEMETRY_CLICKHOUSE_UNAVAILABLE";

    private CoreSupportabilityEventCodes() {
    }
}
