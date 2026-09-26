package com.platform.core.sensorcontrol.desiredstate;

/**
 * Centralized finite resource ceilings for Sensor control desired-config v1.
 * Operational bounds — never {@link Integer#MAX_VALUE}.
 */
public final class SensorControlDesiredLimits {

    public static final int SCHEMA_VERSION = 1;

    public static final int MAX_WORKERS = 256;
    public static final int MAX_QUEUE_DEPTH = 10_000;
    public static final int MAX_TRACKED_ASSIGNMENTS = 50_000;
    public static final int MAX_TRACKED_JOBS = 50_000;
    public static final int MAX_PAYLOAD_BYTES = 4 * 1024 * 1024;
    public static final int MAX_EXECUTION_TIMEOUT_MS = 3_600_000;
    public static final int MAX_SHUTDOWN_GRACE_MS = 600_000;
    public static final int MAX_RETRY_ATTEMPTS = 100;
    public static final int MAX_RETRY_BACKOFF_MS = 3_600_000;
    public static final int MAX_JITTER_BASIS_POINTS = 10_000;

    public static final int MAX_REQUEST_BYTES = 16 * 1024 * 1024;
    public static final int MAX_RESPONSE_BYTES = 16 * 1024 * 1024;
    public static final int MAX_UPLOAD_BATCH_RECORDS = 10_000;
    public static final int MAX_UPLOAD_BATCH_BYTES = 16 * 1024 * 1024;

    public static final int MAX_POOL_COUNT = 6;
    public static final int MAX_REJECTION_CODE_LEN = 128;
    public static final int MAX_REJECTION_MESSAGE_LEN = 512;
    public static final int MAX_DESIRED_BY_LEN = 256;
    public static final int MAX_CONFIG_JSON_BYTES = 65_536;

    private SensorControlDesiredLimits() {}
}
