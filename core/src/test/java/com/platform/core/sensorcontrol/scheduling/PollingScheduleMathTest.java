package com.platform.core.sensorcontrol.scheduling;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

import static org.junit.jupiter.api.Assertions.*;

class PollingScheduleMathTest {
    @ParameterizedTest
    @CsvSource({
            "100,10,3,99,103", "100,10,3,102,103", "100,10,3,103,103",
            "100,10,3,104,113", "100,10,3,112,113", "100,10,3,113,113", "100,10,3,114,123",
            "-100,10,3,-98,-97", "-100,10,3,-97,-97", "-100,10,3,-96,-87",
            "0,10,0,-1,0", "0,10,0,0,0", "0,10,0,1,10", "0,10,9,9,9", "0,10,9,10,19"
    })
    void inclusiveBoundariesAndElapsedCycles(long anchor, long cadence, long phase, long notBefore, long expected) {
        assertEquals(expected, PollingScheduleMath.nextOccurrenceMillis(anchor, cadence, phase, notBefore));
    }

    @Test
    void neverSelectsNegativeCycleEvenWhenAnEarlierAlignedInstantExists() {
        assertEquals(103, PollingScheduleMath.nextOccurrenceMillis(100, 10, 3, Long.MIN_VALUE));
    }

    @Test
    void cadenceOneSkipsAcrossEntireSignedLongRangeWithoutIterationOrIntermediateOverflow() {
        assertEquals(Long.MAX_VALUE, PollingScheduleMath.nextOccurrenceMillis(Long.MIN_VALUE, 1, 0, Long.MAX_VALUE));
        assertEquals(Long.MIN_VALUE, PollingScheduleMath.nextOccurrenceMillis(Long.MIN_VALUE, 1, 0, Long.MIN_VALUE));
        assertEquals(Long.MAX_VALUE, PollingScheduleMath.nextOccurrenceMillis(Long.MAX_VALUE, 1, 0, Long.MIN_VALUE));
    }

    @Test
    void supportsWideDistanceAndMultiplicationWithRepresentableFinalInstant() {
        assertEquals(Long.MAX_VALUE - 1,
                PollingScheduleMath.nextOccurrenceMillis(Long.MIN_VALUE, 2, 0, Long.MAX_VALUE - 2));
        assertEquals(Long.MAX_VALUE - 1,
                PollingScheduleMath.nextOccurrenceMillis(Long.MIN_VALUE, Long.MAX_VALUE, 0, 0));
        assertEquals(Long.MAX_VALUE,
                PollingScheduleMath.nextOccurrenceMillis(Long.MIN_VALUE, Long.MAX_VALUE, 1, Long.MAX_VALUE));
    }

    @Test
    void phaseCanReachMaximumRepresentableFirstOccurrence() {
        assertEquals(Long.MAX_VALUE,
                PollingScheduleMath.nextOccurrenceMillis(Long.MAX_VALUE - 2, 3, 2, Long.MAX_VALUE));
    }

    @Test
    void rejectsFirstOccurrenceBeyondLongMaxRatherThanWrapping() {
        assertThrows(ArithmeticException.class,
                () -> PollingScheduleMath.nextOccurrenceMillis(Long.MAX_VALUE, 2, 1, Long.MIN_VALUE));
        assertThrows(ArithmeticException.class,
                () -> PollingScheduleMath.nextOccurrenceMillis(Long.MAX_VALUE - 1, Long.MAX_VALUE, 2, Long.MAX_VALUE));
    }

    @Test
    void rejectsNextOccurrenceBeyondLongMaxRatherThanReturningPastOrSaturating() {
        assertThrows(ArithmeticException.class,
                () -> PollingScheduleMath.nextOccurrenceMillis(Long.MIN_VALUE, 2, 0, Long.MAX_VALUE));
        assertThrows(ArithmeticException.class,
                () -> PollingScheduleMath.nextOccurrenceMillis(Long.MAX_VALUE - 1, 2, 0, Long.MAX_VALUE));
    }

    @ParameterizedTest
    @CsvSource({"0,0", "-1,0", "-9223372036854775808,0", "10,-1", "10,10", "10,11", "1,1"})
    void rejectsInvalidCadenceOrPhase(long cadence, long phase) {
        assertThrows(IllegalArgumentException.class,
                () -> PollingScheduleMath.nextOccurrenceMillis(0, cadence, phase, 0));
    }

    @Test
    void matchesIndependentBoundedEnumerationOracleForSmallSchedules() {
        // Test-only bounded enumeration; production skips elapsed ranges arithmetically.
        for (long anchor = -3; anchor <= 3; anchor++) {
            for (long cadence = 1; cadence <= 6; cadence++) {
                for (long phase = 0; phase < cadence; phase++) {
                    for (long notBefore = -5; notBefore <= 15; notBefore++) {
                        long expected = anchor + phase;
                        while (expected < notBefore) expected += cadence;
                        assertEquals(expected, PollingScheduleMath.nextOccurrenceMillis(anchor, cadence, phase, notBefore));
                    }
                }
            }
        }
    }
}
