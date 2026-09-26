package com.platform.core.control;

/**
 * JSON-safe control-generation ceiling (BUILD-000 D016 / BUILD-004 Slice 1).
 *
 * <p>Applies to assignment/config/policy/generation counters carried on JSON control
 * contracts. Never apply to SNMP Counter64 observation values or transport sequences
 * (see {@code TransportSequenceLimits}).
 */
public final class ControlGenerationLimits {

    /** {@code 2^53 - 1} — maximum JSON-safe integer. */
    public static final long MAX_SAFE_CONTROL_GENERATION = 9_007_199_254_740_991L;

    public static final String ERR_CONTROL_GENERATION_EXHAUSTED = "ERR_CONTROL_GENERATION_EXHAUSTED";

    public static final String EXHAUSTED_MESSAGE =
            "Control generation cannot advance beyond MAX_SAFE_CONTROL_GENERATION";

    private ControlGenerationLimits() {}

    /** Non-negative control counter within the JSON-safe range (includes zero). */
    public static boolean isJsonSafe(long value) {
        return value >= 0L && value <= MAX_SAFE_CONTROL_GENERATION;
    }

    /** Positive control counter within the JSON-safe range (excludes zero). */
    public static boolean isPositiveJsonSafe(long value) {
        return value >= 1L && value <= MAX_SAFE_CONTROL_GENERATION;
    }

    /**
     * True when {@code current + 1} would remain within {@link #MAX_SAFE_CONTROL_GENERATION}.
     * {@code current} itself may be 0 (e.g. never-mutated desired generation).
     */
    public static boolean canIncrement(long current) {
        return current >= 0L && current < MAX_SAFE_CONTROL_GENERATION;
    }

    /**
     * Returns {@code current + 1} when safe; otherwise empty so callers can throw the
     * domain-appropriate exhaustion exception without wrapping.
     */
    public static long nextOrThrow(long current, RuntimeException exhausted) {
        if (!canIncrement(current)) {
            throw exhausted;
        }
        return current + 1L;
    }
}
