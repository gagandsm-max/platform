package com.platform.core.ingestion;

/**
 * JSON-safe transport sequence ceiling (BUILD-000 D001 / BUILD-003 enforcement).
 * Never apply to SNMP Counter64 observation values.
 */
public final class TransportSequenceLimits {
    public static final long MAX_SAFE_SEQUENCE = 9007199254740991L;
    public static final long MIN_WIRE_SEQUENCE = 1L;

    private TransportSequenceLimits() {}

    public static boolean isValidWireSequence(long sequence) {
        return sequence >= MIN_WIRE_SEQUENCE && sequence <= MAX_SAFE_SEQUENCE;
    }

    public static boolean isValidWatermark(long sequence) {
        return sequence >= 0 && sequence <= MAX_SAFE_SEQUENCE;
    }
}
