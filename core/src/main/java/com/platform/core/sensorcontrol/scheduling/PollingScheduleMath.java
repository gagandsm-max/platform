package com.platform.core.sensorcontrol.scheduling;

import java.math.BigInteger;

/** Pure epoch-millisecond arithmetic; no clock, timezone or runtime scheduling side effects. */
public final class PollingScheduleMath {
    private PollingScheduleMath() { }

    /**
     * Earliest representable anchor + phase + k * cadence at/after notBefore, k >= 0.
     * Equality is inclusive. Elapsed occurrences are skipped by division, never enumerated.
     *
     * @throws IllegalArgumentException for nonpositive cadence or phase outside [0, cadence)
     * @throws ArithmeticException if no occurrence at/after notBefore fits a signed long
     */
    public static long nextOccurrenceMillis(long anchorEpochMillis, long cadenceMillis,
                                            long phaseMillis, long notBeforeEpochMillis) {
        if (cadenceMillis <= 0) {
            throw new IllegalArgumentException("cadenceMillis must be positive");
        }
        if (phaseMillis < 0 || phaseMillis >= cadenceMillis) {
            throw new IllegalArgumentException("phaseMillis must be in [0, cadenceMillis)");
        }
        BigInteger first = BigInteger.valueOf(anchorEpochMillis).add(BigInteger.valueOf(phaseMillis));
        BigInteger notBefore = BigInteger.valueOf(notBeforeEpochMillis);
        if (first.compareTo(notBefore) >= 0) {
            return first.longValueExact();
        }
        BigInteger cadence = BigInteger.valueOf(cadenceMillis);
        BigInteger elapsed = notBefore.subtract(first);
        BigInteger steps = elapsed.add(cadence).subtract(BigInteger.ONE).divide(cadence);
        return first.add(steps.multiply(cadence)).longValueExact();
    }
}
