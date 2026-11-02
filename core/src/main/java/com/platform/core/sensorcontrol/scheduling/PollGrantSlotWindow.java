package com.platform.core.sensorcontrol.scheduling;

import java.math.BigInteger;
import java.time.Instant;
import java.util.Objects;

/**
 * Pure grant slot-window arithmetic. Calculation only: no Spring bean, authorization,
 * schedule ownership, grant issuance, Sensor binding or clock side effects.
 *
 * <p>{@code T(k) = anchorEpochMillis + phaseMillis + k * cadenceMillis} for nonnegative
 * slot index {@code k}. Eligibility is exact inclusive Instant membership of
 * {@link Instant#ofEpochMilli(long) Instant.ofEpochMilli(T(k))} in
 * {@code [windowStart, windowEnd]} — window bounds are never truncated to epoch millis.
 *
 * <p>Computing a slot does not authenticate tenant, Sensor, endpoint, plan or grant.
 */
public final class PollGrantSlotWindow {

    /** {@code 2^53 - 1} — JSON-safe nonnegative slot-index ceiling. */
    public static final long MAX_JSON_SAFE_SLOT_INDEX = 9_007_199_254_740_991L;

    private static final BigInteger MAX_SAFE = BigInteger.valueOf(MAX_JSON_SAFE_SLOT_INDEX);
    private static final BigInteger THOUSAND = BigInteger.valueOf(1000L);

    private PollGrantSlotWindow() { }

    /**
     * Inclusive nonnegative slot-index range whose occurrences fall inside
     * {@code [windowStart, windowEnd]}. Returns an explicit empty range when none exist.
     *
     * <p>Does not enumerate elapsed cycles. Does not silently clip a mathematically
     * in-window range that contains a slot index above {@link #MAX_JSON_SAFE_SLOT_INDEX}
     * or an occurrence outside signed 64-bit epoch millis.
     *
     * @throws NullPointerException if {@code windowStart} or {@code windowEnd} is null
     * @throws IllegalArgumentException for nonpositive cadence, phase outside
     *         {@code [0, cadence)}, or {@code windowEnd <= windowStart}
     * @throws ArithmeticException if an in-window slot index or occurrence cannot be
     *         represented under JSON-safe / signed-long constraints
     */
    public static SlotIndexRange eligibleSlotRange(long anchorEpochMillis, long cadenceMillis,
                                                   long phaseMillis,
                                                   Instant windowStart, Instant windowEnd) {
        validateCadenceAndPhase(cadenceMillis, phaseMillis);
        Objects.requireNonNull(windowStart, "windowStart");
        Objects.requireNonNull(windowEnd, "windowEnd");
        if (!windowEnd.isAfter(windowStart)) {
            throw new IllegalArgumentException("windowEnd must be after windowStart");
        }

        BigInteger first = BigInteger.valueOf(anchorEpochMillis).add(BigInteger.valueOf(phaseMillis));
        BigInteger cadence = BigInteger.valueOf(cadenceMillis);

        BigInteger milliMin = earliestEpochMilliAtOrAfter(windowStart);
        BigInteger milliMax = latestEpochMilliAtOrBefore(windowEnd);
        if (milliMin.compareTo(milliMax) > 0) {
            return SlotIndexRange.empty();
        }

        // Full mathematical eligible nonnegative k from milliMin/milliMax before any
        // signed-long bound intersection. Empty only when no mathematical eligible slot;
        // unrepresentable in-window endpoints fail closed (no silent clip).
        BigInteger kMin = ceilDiv(milliMin.subtract(first), cadence);
        BigInteger kMax = floorDiv(milliMax.subtract(first), cadence);
        if (kMin.signum() < 0) {
            kMin = BigInteger.ZERO;
        }
        if (kMin.compareTo(kMax) > 0) {
            return SlotIndexRange.empty();
        }

        if (kMin.compareTo(MAX_SAFE) > 0 || kMax.compareTo(MAX_SAFE) > 0) {
            throw new ArithmeticException(
                    "eligible slot index exceeds MAX_JSON_SAFE_SLOT_INDEX");
        }

        first.add(kMin.multiply(cadence)).longValueExact();
        first.add(kMax.multiply(cadence)).longValueExact();

        return new SlotIndexRange(kMin.longValueExact(), kMax.longValueExact());
    }

    /**
     * Epoch-millis occurrence {@code T(k)} for a nonnegative JSON-safe slot index.
     *
     * @throws IllegalArgumentException for nonpositive cadence, phase outside
     *         {@code [0, cadence)}, negative {@code slotIndex}, or {@code slotIndex}
     *         above {@link #MAX_JSON_SAFE_SLOT_INDEX}
     * @throws ArithmeticException if {@code T(k)} overflows signed 64-bit epoch millis
     */
    public static long occurrenceEpochMillis(long anchorEpochMillis, long cadenceMillis,
                                             long phaseMillis, long slotIndex) {
        validateCadenceAndPhase(cadenceMillis, phaseMillis);
        if (slotIndex < 0L || slotIndex > MAX_JSON_SAFE_SLOT_INDEX) {
            throw new IllegalArgumentException(
                    "slotIndex must be in [0, MAX_JSON_SAFE_SLOT_INDEX]");
        }
        return BigInteger.valueOf(anchorEpochMillis)
                .add(BigInteger.valueOf(phaseMillis))
                .add(BigInteger.valueOf(slotIndex).multiply(BigInteger.valueOf(cadenceMillis)))
                .longValueExact();
    }

    private static void validateCadenceAndPhase(long cadenceMillis, long phaseMillis) {
        if (cadenceMillis <= 0L) {
            throw new IllegalArgumentException("cadenceMillis must be positive");
        }
        if (phaseMillis < 0L || phaseMillis >= cadenceMillis) {
            throw new IllegalArgumentException("phaseMillis must be in [0, cadenceMillis)");
        }
    }

    /** Smallest epoch milli {@code M} with {@code Instant.ofEpochMilli(M) >= bound}. */
    private static BigInteger earliestEpochMilliAtOrAfter(Instant bound) {
        BigInteger floor = epochMilliFloor(bound);
        return isExactMilliBoundary(bound) ? floor : floor.add(BigInteger.ONE);
    }

    /** Largest epoch milli {@code M} with {@code Instant.ofEpochMilli(M) <= bound}. */
    private static BigInteger latestEpochMilliAtOrBefore(Instant bound) {
        return epochMilliFloor(bound);
    }

    private static BigInteger epochMilliFloor(Instant instant) {
        return BigInteger.valueOf(instant.getEpochSecond())
                .multiply(THOUSAND)
                .add(BigInteger.valueOf(instant.getNano() / 1_000_000));
    }

    private static boolean isExactMilliBoundary(Instant instant) {
        return instant.getNano() % 1_000_000 == 0;
    }

    /** Floor division toward {@code -∞} for positive divisor. */
    private static BigInteger floorDiv(BigInteger numerator, BigInteger positiveDivisor) {
        BigInteger[] qr = numerator.divideAndRemainder(positiveDivisor);
        if (qr[1].signum() < 0) {
            return qr[0].subtract(BigInteger.ONE);
        }
        return qr[0];
    }

    /** Ceiling division toward {@code +∞} for positive divisor. */
    private static BigInteger ceilDiv(BigInteger numerator, BigInteger positiveDivisor) {
        BigInteger[] qr = numerator.divideAndRemainder(positiveDivisor);
        if (qr[1].signum() > 0) {
            return qr[0].add(BigInteger.ONE);
        }
        return qr[0];
    }

    /**
     * Inclusive slot-index range. Empty ranges use {@code firstInclusive > lastInclusive}
     * and never confer eligibility.
     */
    public record SlotIndexRange(long firstInclusive, long lastInclusive) {
        public static SlotIndexRange empty() {
            return new SlotIndexRange(0L, -1L);
        }

        public boolean isEmpty() {
            return firstInclusive > lastInclusive;
        }

        /** Number of slots; empty ranges return 0. */
        public long size() {
            if (isEmpty()) {
                return 0L;
            }
            return Math.subtractExact(Math.addExact(lastInclusive, 1L), firstInclusive);
        }
    }
}
