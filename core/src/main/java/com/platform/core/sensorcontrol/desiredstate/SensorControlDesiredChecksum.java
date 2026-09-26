package com.platform.core.sensorcontrol.desiredstate;

import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.HexFormat;
import java.util.Map;
import java.util.Objects;

/**
 * Deterministic SHA-256 checksum over Sensor control desired-config v1 semantic fields.
 *
 * <p><b>Framing contract (must be mirrored by Go in Slice 3):</b>
 * <ol>
 *   <li>Digest domain prefix UTF-8 bytes of {@link #DOMAIN} followed by a single 0x00 byte.</li>
 *   <li>For each field below, write a length-prefixed UTF-8 string (4-byte big-endian length + bytes)
 *       for the field name, then again for the field value (decimal ASCII for integers, enum name for enums).</li>
 *   <li>Fixed field order (generation is intentionally excluded):
 *       {@code logLevel}, {@code maxWorkers}, {@code maxQueueDepth}, {@code maxTrackedAssignments},
 *       {@code maxTrackedJobs}, {@code maxPayloadBytes}, {@code defaultExecutionTimeoutMs},
 *       {@code shutdownGracePeriodMs}, {@code retryMaxAttempts}, {@code retryInitialBackoffMs},
 *       {@code retryMaxBackoffMs}, {@code retryJitterBasisPoints}.</li>
 *   <li>Then for each scheduler pool sorted by pool name ascending (Unicode code-point / UTF-8 byte order):
 *       field name {@code pool}, value = pool name; then {@code workers} and {@code queueDepth}.</li>
 *   <li>Then transport fields in order:
 *       {@code requestTimeoutMs}, {@code maxRequestBytes}, {@code maxResponseBytes},
 *       {@code uploadBatchRecords}, {@code uploadBatchBytes}.</li>
 * </ol>
 * Output is lowercase hexadecimal SHA-256 (64 chars).
 */
public final class SensorControlDesiredChecksum {

    public static final String DOMAIN = "platform.sensor.control.desired.v1";

    private SensorControlDesiredChecksum() {}

    public static String compute(SensorRuntimeDesiredConfig config) {
        Objects.requireNonNull(config, "config");
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            digest.update(DOMAIN.getBytes(StandardCharsets.UTF_8));
            digest.update((byte) 0);

            write(digest, "logLevel", config.logLevel().name());
            SensorSchedulerDesiredConfig scheduler = config.scheduler();
            write(digest, "maxWorkers", Integer.toString(scheduler.maxWorkers()));
            write(digest, "maxQueueDepth", Integer.toString(scheduler.maxQueueDepth()));
            write(digest, "maxTrackedAssignments", Integer.toString(scheduler.maxTrackedAssignments()));
            write(digest, "maxTrackedJobs", Integer.toString(scheduler.maxTrackedJobs()));
            write(digest, "maxPayloadBytes", Integer.toString(scheduler.maxPayloadBytes()));
            write(digest, "defaultExecutionTimeoutMs", Integer.toString(scheduler.defaultExecutionTimeoutMs()));
            write(digest, "shutdownGracePeriodMs", Integer.toString(scheduler.shutdownGracePeriodMs()));
            write(digest, "retryMaxAttempts", Integer.toString(scheduler.retryMaxAttempts()));
            write(digest, "retryInitialBackoffMs", Integer.toString(scheduler.retryInitialBackoffMs()));
            write(digest, "retryMaxBackoffMs", Integer.toString(scheduler.retryMaxBackoffMs()));
            write(digest, "retryJitterBasisPoints", Integer.toString(scheduler.retryJitterBasisPoints()));

            for (Map.Entry<String, SensorSchedulerPoolConfig> e : scheduler.poolsSortedByName().entrySet()) {
                write(digest, "pool", e.getKey());
                write(digest, "workers", Integer.toString(e.getValue().workers()));
                write(digest, "queueDepth", Integer.toString(e.getValue().queueDepth()));
            }

            SensorTransportDesiredConfig transport = config.transport();
            write(digest, "requestTimeoutMs", Integer.toString(transport.requestTimeoutMs()));
            write(digest, "maxRequestBytes", Integer.toString(transport.maxRequestBytes()));
            write(digest, "maxResponseBytes", Integer.toString(transport.maxResponseBytes()));
            write(digest, "uploadBatchRecords", Integer.toString(transport.uploadBatchRecords()));
            write(digest, "uploadBatchBytes", Integer.toString(transport.uploadBatchBytes()));

            return HexFormat.of().formatHex(digest.digest());
        } catch (NoSuchAlgorithmException ex) {
            throw new IllegalStateException("SHA-256 unavailable", ex);
        }
    }

    private static void write(MessageDigest digest, String name, String value) {
        writeLengthPrefixed(digest, name);
        writeLengthPrefixed(digest, value);
    }

    private static void writeLengthPrefixed(MessageDigest digest, String value) {
        byte[] utf8 = value.getBytes(StandardCharsets.UTF_8);
        ByteBuffer len = ByteBuffer.allocate(4);
        len.putInt(utf8.length);
        digest.update(len.array());
        digest.update(utf8);
    }
}
