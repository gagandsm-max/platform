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

    public static final String KAFKA_PUBLICATION_FAILED = "CORE_KAFKA_PUBLICATION_FAILED";
    public static final String CLICKHOUSE_INSERT_FAILED = "CORE_CLICKHOUSE_INSERT_FAILED";
    public static final String TELEMETRY_RETRIES_EXHAUSTED = "CORE_TELEMETRY_RETRIES_EXHAUSTED";
    public static final String PUBLISHER_LEASE_RELEASE_FAILED = "CORE_PUBLISHER_LEASE_RELEASE_FAILED";
    public static final String PUBLISHER_FINALIZE_FAILED = "CORE_PUBLISHER_FINALIZE_FAILED";

    public static final String DATABASE_ACCESS_FAILED = "CORE_DATABASE_ACCESS_FAILED";
    public static final String UNHANDLED_API_FAILURE = "CORE_UNHANDLED_API_FAILURE";

    private CoreSupportabilityEventCodes() {
    }
}
