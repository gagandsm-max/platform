package com.platform.core.sensorcontrol.scheduling;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;
import org.junit.jupiter.params.provider.ValueSource;

import java.math.BigInteger;
import java.time.Instant;

import static org.junit.jupiter.api.Assertions.*;

class PollGrantSlotWindowTest {

    private static final long GOLDEN_ANCHOR = 1_730_550_000_000L;
    private static final long GOLDEN_CADENCE = 60_000L;
    private static final long GOLDEN_PHASE = 14_250L;
    private static final Instant GOLDEN_START = Instant.parse("2026-11-02T10:00:00Z");
    private static final Instant GOLDEN_END = Instant.parse("2026-11-02T10:15:00Z");

    @Test
    void goldenNovember2026RangeMatchesIndependentReference() {
        ReferenceRange ref = independentRange(
                GOLDEN_ANCHOR, GOLDEN_CADENCE, GOLDEN_PHASE, GOLDEN_START, GOLDEN_END);
        assertFalse(ref.empty);
        assertEquals(1_051_060L, ref.first);
        assertEquals(1_051_074L, ref.last);
        assertEquals(15L, ref.last - ref.first + 1L);

        PollGrantSlotWindow.SlotIndexRange range = PollGrantSlotWindow.eligibleSlotRange(
                GOLDEN_ANCHOR, GOLDEN_CADENCE, GOLDEN_PHASE, GOLDEN_START, GOLDEN_END);
        assertFalse(range.isEmpty());
        assertEquals(ref.first, range.firstInclusive());
        assertEquals(ref.last, range.lastInclusive());
        assertEquals(15L, range.size());

        assertEquals(Instant.parse("2026-11-02T10:00:14.250Z"),
                Instant.ofEpochMilli(PollGrantSlotWindow.occurrenceEpochMillis(
                        GOLDEN_ANCHOR, GOLDEN_CADENCE, GOLDEN_PHASE, range.firstInclusive())));
        assertEquals(Instant.parse("2026-11-02T10:14:14.250Z"),
                Instant.ofEpochMilli(PollGrantSlotWindow.occurrenceEpochMillis(
                        GOLDEN_ANCHOR, GOLDEN_CADENCE, GOLDEN_PHASE, range.lastInclusive())));

        long k0 = PollGrantSlotWindow.occurrenceEpochMillis(
                GOLDEN_ANCHOR, GOLDEN_CADENCE, GOLDEN_PHASE, 0L);
        assertTrue(Instant.ofEpochMilli(k0).isBefore(GOLDEN_START));
        assertTrue(0L < range.firstInclusive());
    }

    @Test
    void goldenResultIsDeterministicAcrossRepeatedCalls() {
        PollGrantSlotWindow.SlotIndexRange a = PollGrantSlotWindow.eligibleSlotRange(
                GOLDEN_ANCHOR, GOLDEN_CADENCE, GOLDEN_PHASE, GOLDEN_START, GOLDEN_END);
        PollGrantSlotWindow.SlotIndexRange b = PollGrantSlotWindow.eligibleSlotRange(
                GOLDEN_ANCHOR, GOLDEN_CADENCE, GOLDEN_PHASE, GOLDEN_START, GOLDEN_END);
        assertEquals(a, b);
        assertEquals(
                PollGrantSlotWindow.occurrenceEpochMillis(
                        GOLDEN_ANCHOR, GOLDEN_CADENCE, GOLDEN_PHASE, a.firstInclusive()),
                PollGrantSlotWindow.occurrenceEpochMillis(
                        GOLDEN_ANCHOR, GOLDEN_CADENCE, GOLDEN_PHASE, b.firstInclusive()));
    }

    @Test
    void inclusiveEqualityAtBothWindowBoundaries() {
        long anchor = 0L;
        long cadence = 1_000L;
        long phase = 250L;
        Instant occurrence = Instant.ofEpochMilli(anchor + phase); // k=0 → 250ms
        PollGrantSlotWindow.SlotIndexRange atStart = PollGrantSlotWindow.eligibleSlotRange(
                anchor, cadence, phase, occurrence, occurrence.plusMillis(1));
        assertEquals(0L, atStart.firstInclusive());
        assertEquals(0L, atStart.lastInclusive());

        PollGrantSlotWindow.SlotIndexRange atEnd = PollGrantSlotWindow.eligibleSlotRange(
                anchor, cadence, phase, occurrence.minusMillis(1), occurrence);
        assertEquals(0L, atEnd.firstInclusive());
        assertEquals(0L, atEnd.lastInclusive());
    }

    @Test
    void justBeforeAndJustAfterNanoAndMicroBoundariesChangeMembership() {
        long anchor = 0L;
        long cadence = 10_000L;
        long phase = 0L;
        Instant occurrence = Instant.ofEpochMilli(0L);

        assertTrue(PollGrantSlotWindow.eligibleSlotRange(
                anchor, cadence, phase, occurrence, occurrence.plusNanos(1)).size() == 1L);
        assertTrue(PollGrantSlotWindow.eligibleSlotRange(
                anchor, cadence, phase,
                occurrence.plusNanos(1), occurrence.plusMillis(1)).isEmpty());

        assertTrue(PollGrantSlotWindow.eligibleSlotRange(
                anchor, cadence, phase, occurrence.minusNanos(1), occurrence).size() == 1L);
        assertTrue(PollGrantSlotWindow.eligibleSlotRange(
                anchor, cadence, phase,
                occurrence.minusMillis(1), occurrence.minusNanos(1)).isEmpty());

        Instant microAfter = occurrence.plusNanos(1_000L);
        assertTrue(PollGrantSlotWindow.eligibleSlotRange(
                anchor, cadence, phase, microAfter, microAfter.plusMillis(5)).isEmpty());
        Instant microBeforeEnd = occurrence.plusMillis(10_000L).minusNanos(1_000L);
        assertTrue(PollGrantSlotWindow.eligibleSlotRange(
                anchor, cadence, phase, occurrence.plusMillis(1), microBeforeEnd).isEmpty());
        assertEquals(1L, PollGrantSlotWindow.eligibleSlotRange(
                anchor, cadence, phase,
                occurrence.plusMillis(1), occurrence.plusMillis(10_000L)).size());
    }

    @Test
    void smallIntervalWithNoEligibleSlotsReturnsExplicitEmpty() {
        PollGrantSlotWindow.SlotIndexRange range = PollGrantSlotWindow.eligibleSlotRange(
                0L, 60_000L, 0L,
                Instant.ofEpochMilli(1L), Instant.ofEpochMilli(59_999L));
        assertTrue(range.isEmpty());
        assertEquals(0L, range.size());
        assertEquals(PollGrantSlotWindow.SlotIndexRange.empty(), range);
    }

    @Test
    void windowBeforeAnchorYieldsEmptyWhileLegitimateK0IsIncludedWhenCovered() {
        assertTrue(PollGrantSlotWindow.eligibleSlotRange(
                1_000L, 100L, 5L,
                Instant.ofEpochMilli(-1_000L), Instant.ofEpochMilli(0L)).isEmpty());

        PollGrantSlotWindow.SlotIndexRange withK0 = PollGrantSlotWindow.eligibleSlotRange(
                1_000L, 100L, 5L,
                Instant.ofEpochMilli(1_005L), Instant.ofEpochMilli(1_005L).plusNanos(1));
        assertEquals(0L, withK0.firstInclusive());
        assertEquals(0L, withK0.lastInclusive());
        assertEquals(1_005L, PollGrantSlotWindow.occurrenceEpochMillis(1_000L, 100L, 5L, 0L));
    }

    @Test
    void negativeEpochAnchorAndNegativeTimeOffsetsRemainExact() {
        ReferenceRange ref = independentRange(
                -1_000_000L, 1_000L, 250L,
                Instant.ofEpochMilli(-999_750L), Instant.ofEpochMilli(-997_750L));
        assertFalse(ref.empty);
        PollGrantSlotWindow.SlotIndexRange range = PollGrantSlotWindow.eligibleSlotRange(
                -1_000_000L, 1_000L, 250L,
                Instant.ofEpochMilli(-999_750L), Instant.ofEpochMilli(-997_750L));
        assertEquals(ref.first, range.firstInclusive());
        assertEquals(ref.last, range.lastInclusive());
        assertEquals(-999_750L, PollGrantSlotWindow.occurrenceEpochMillis(
                -1_000_000L, 1_000L, 250L, range.firstInclusive()));
        assertEquals(-997_750L, PollGrantSlotWindow.occurrenceEpochMillis(
                -1_000_000L, 1_000L, 250L, range.lastInclusive()));
    }

    @ParameterizedTest
    @CsvSource({"0,0", "-1,0", "-9223372036854775808,0", "10,-1", "10,10", "10,11", "1,1"})
    void rejectsInvalidCadenceOrPhase(long cadence, long phase) {
        Instant start = Instant.EPOCH;
        Instant end = Instant.EPOCH.plusSeconds(1);
        assertThrows(IllegalArgumentException.class,
                () -> PollGrantSlotWindow.eligibleSlotRange(0L, cadence, phase, start, end));
        assertThrows(IllegalArgumentException.class,
                () -> PollGrantSlotWindow.occurrenceEpochMillis(0L, cadence, phase, 0L));
    }

    @Test
    void rejectsNullWindowBoundsAndEndNotAfterStart() {
        Instant t = Instant.EPOCH;
        assertThrows(NullPointerException.class,
                () -> PollGrantSlotWindow.eligibleSlotRange(0L, 1L, 0L, null, t.plusMillis(1)));
        assertThrows(NullPointerException.class,
                () -> PollGrantSlotWindow.eligibleSlotRange(0L, 1L, 0L, t, null));
        assertThrows(IllegalArgumentException.class,
                () -> PollGrantSlotWindow.eligibleSlotRange(0L, 1L, 0L, t, t));
        assertThrows(IllegalArgumentException.class,
                () -> PollGrantSlotWindow.eligibleSlotRange(0L, 1L, 0L, t.plusMillis(1), t));
    }

    @Test
    void signedLongOverflowRejectsUnrepresentableButAcceptsRepresentableCandidates() {
        assertEquals(Long.MAX_VALUE,
                PollGrantSlotWindow.occurrenceEpochMillis(Long.MAX_VALUE - 2L, 3L, 2L, 0L));
        assertEquals(Long.MIN_VALUE,
                PollGrantSlotWindow.occurrenceEpochMillis(Long.MIN_VALUE, 1L, 0L, 0L));
        // JSON-safe k with wide BigInteger multiply still yields a representable long.
        assertEquals(Long.MAX_VALUE - 1L,
                PollGrantSlotWindow.occurrenceEpochMillis(Long.MIN_VALUE, Long.MAX_VALUE, 0L, 2L));
        assertEquals(-1L,
                PollGrantSlotWindow.occurrenceEpochMillis(Long.MIN_VALUE, Long.MAX_VALUE, 0L, 1L));

        assertThrows(ArithmeticException.class,
                () -> PollGrantSlotWindow.occurrenceEpochMillis(Long.MAX_VALUE, 2L, 1L, 0L));
        assertThrows(ArithmeticException.class,
                () -> PollGrantSlotWindow.occurrenceEpochMillis(Long.MAX_VALUE - 1L, 2L, 0L, 1L));

        // Window demands the unrepresentable T(0)=Long.MAX_VALUE+1 → empty (no representable slot).
        assertTrue(PollGrantSlotWindow.eligibleSlotRange(
                Long.MAX_VALUE, 2L, 1L,
                Instant.ofEpochMilli(Long.MAX_VALUE), Instant.ofEpochMilli(Long.MAX_VALUE).plusSeconds(1)
        ).isEmpty());

        // Representable MAX occurrence inside window remains eligible.
        PollGrantSlotWindow.SlotIndexRange maxSlot = PollGrantSlotWindow.eligibleSlotRange(
                Long.MAX_VALUE - 2L, 3L, 2L,
                Instant.ofEpochMilli(Long.MAX_VALUE), Instant.ofEpochMilli(Long.MAX_VALUE).plusNanos(1));
        assertEquals(0L, maxSlot.firstInclusive());
        assertEquals(0L, maxSlot.lastInclusive());
    }

    @Test
    void slotIndexAboveMaxJsonSafeIsRejectedAndInWindowOverMaxSafeIsNotClipped() {
        assertThrows(IllegalArgumentException.class,
                () -> PollGrantSlotWindow.occurrenceEpochMillis(
                        0L, 1L, 0L, PollGrantSlotWindow.MAX_JSON_SAFE_SLOT_INDEX + 1L));
        assertThrows(IllegalArgumentException.class,
                () -> PollGrantSlotWindow.occurrenceEpochMillis(0L, 1L, 0L, -1L));

        assertEquals(PollGrantSlotWindow.MAX_JSON_SAFE_SLOT_INDEX,
                PollGrantSlotWindow.occurrenceEpochMillis(
                        0L, 1L, 0L, PollGrantSlotWindow.MAX_JSON_SAFE_SLOT_INDEX));

        long first = 0L;
        long cadence = 1L;
        long phase = 0L;
        long overMax = PollGrantSlotWindow.MAX_JSON_SAFE_SLOT_INDEX + 1L;
        Instant start = Instant.ofEpochMilli(first + phase + overMax * cadence);
        Instant end = start.plusMillis(2);
        assertThrows(ArithmeticException.class,
                () -> PollGrantSlotWindow.eligibleSlotRange(first, cadence, phase, start, end));
    }

    @Test
    void impossibleFarFutureEmptyWithoutEnumeration() {
        // Window ends before first occurrence; must return empty without scanning k.
        assertTrue(PollGrantSlotWindow.eligibleSlotRange(
                Long.MAX_VALUE / 2L, Long.MAX_VALUE / 4L, 0L,
                Instant.EPOCH, Instant.EPOCH.plusSeconds(1)).isEmpty());
    }

    @Test
    void independentBoundedEnumerationOracleForSmallSchedules() {
        for (long anchor = -3; anchor <= 3; anchor++) {
            for (long cadence = 1; cadence <= 5; cadence++) {
                for (long phase = 0; phase < cadence; phase++) {
                    for (long startMs = -4; startMs <= 8; startMs++) {
                        for (long endMs = startMs + 1; endMs <= 10; endMs++) {
                            Instant start = Instant.ofEpochMilli(startMs);
                            Instant end = Instant.ofEpochMilli(endMs);
                            ReferenceRange ref = independentRange(
                                    anchor, cadence, phase, start, end);
                            PollGrantSlotWindow.SlotIndexRange range =
                                    PollGrantSlotWindow.eligibleSlotRange(
                                            anchor, cadence, phase, start, end);
                            if (ref.empty) {
                                assertTrue(range.isEmpty());
                            } else {
                                assertEquals(ref.first, range.firstInclusive());
                                assertEquals(ref.last, range.lastInclusive());
                            }
                        }
                    }
                }
            }
        }
    }

    @Test
    void subMillisecondWindowBoundsUseExactInstantMembershipNotTruncation() {
        // Truncating windowStart to epoch milli would wrongly include T=0.
        Instant start = Instant.EPOCH.plusNanos(1);
        Instant end = Instant.EPOCH.plusMillis(5);
        assertTrue(PollGrantSlotWindow.eligibleSlotRange(0L, 10L, 0L, start, end).isEmpty());

        // Truncating windowEnd would wrongly include the next milli-aligned occurrence.
        Instant endExclusiveNano = Instant.ofEpochMilli(10L).minusNanos(1);
        assertTrue(PollGrantSlotWindow.eligibleSlotRange(
                0L, 10L, 0L, Instant.EPOCH.plusMillis(1), endExclusiveNano).isEmpty());
        assertEquals(1L, PollGrantSlotWindow.eligibleSlotRange(
                0L, 10L, 0L, Instant.EPOCH.plusMillis(1), Instant.ofEpochMilli(10L)).size());
    }

    @ParameterizedTest
    @ValueSource(longs = {0L, 1L, 15L, 1_051_060L, PollGrantSlotWindow.MAX_JSON_SAFE_SLOT_INDEX})
    void occurrenceMatchesIndependentBigIntegerFormula(long slotIndex) {
        long anchor = -123_456_789L;
        long cadence = 60_000L;
        long phase = 14_250L;
        long expected = BigInteger.valueOf(anchor)
                .add(BigInteger.valueOf(phase))
                .add(BigInteger.valueOf(slotIndex).multiply(BigInteger.valueOf(cadence)))
                .longValueExact();
        assertEquals(expected,
                PollGrantSlotWindow.occurrenceEpochMillis(anchor, cadence, phase, slotIndex));
    }

    /**
     * Test-only reference: BigInteger ceil/floor range for milli-aligned Instant membership.
     * Does not call {@link PollGrantSlotWindow} production helpers.
     */
    private static ReferenceRange independentRange(long anchor, long cadence, long phase,
                                                   Instant windowStart, Instant windowEnd) {
        BigInteger first = BigInteger.valueOf(anchor).add(BigInteger.valueOf(phase));
        BigInteger cad = BigInteger.valueOf(cadence);
        BigInteger startFloor = BigInteger.valueOf(windowStart.getEpochSecond())
                .multiply(BigInteger.valueOf(1000L))
                .add(BigInteger.valueOf(windowStart.getNano() / 1_000_000));
        BigInteger milliMin = (windowStart.getNano() % 1_000_000 == 0)
                ? startFloor : startFloor.add(BigInteger.ONE);
        BigInteger milliMax = BigInteger.valueOf(windowEnd.getEpochSecond())
                .multiply(BigInteger.valueOf(1000L))
                .add(BigInteger.valueOf(windowEnd.getNano() / 1_000_000));
        if (milliMin.compareTo(milliMax) > 0) {
            return ReferenceRange.empty();
        }
        BigInteger searchMin = milliMin.max(BigInteger.valueOf(Long.MIN_VALUE));
        BigInteger searchMax = milliMax.min(BigInteger.valueOf(Long.MAX_VALUE));
        if (searchMin.compareTo(searchMax) > 0) {
            return ReferenceRange.empty();
        }
        BigInteger kMin = ceilDiv(searchMin.subtract(first), cad);
        BigInteger kMax = floorDiv(searchMax.subtract(first), cad);
        if (kMin.signum() < 0) {
            kMin = BigInteger.ZERO;
        }
        if (kMin.compareTo(kMax) > 0) {
            return ReferenceRange.empty();
        }
        if (kMin.compareTo(BigInteger.valueOf(PollGrantSlotWindow.MAX_JSON_SAFE_SLOT_INDEX)) > 0
                || kMax.compareTo(BigInteger.valueOf(PollGrantSlotWindow.MAX_JSON_SAFE_SLOT_INDEX)) > 0) {
            throw new ArithmeticException("reference range exceeds MAX_JSON_SAFE_SLOT_INDEX");
        }
        first.add(kMin.multiply(cad)).longValueExact();
        first.add(kMax.multiply(cad)).longValueExact();
        return new ReferenceRange(false, kMin.longValueExact(), kMax.longValueExact());
    }

    private static BigInteger floorDiv(BigInteger numerator, BigInteger positiveDivisor) {
        BigInteger[] qr = numerator.divideAndRemainder(positiveDivisor);
        if (qr[1].signum() < 0) {
            return qr[0].subtract(BigInteger.ONE);
        }
        return qr[0];
    }

    private static BigInteger ceilDiv(BigInteger numerator, BigInteger positiveDivisor) {
        BigInteger[] qr = numerator.divideAndRemainder(positiveDivisor);
        if (qr[1].signum() > 0) {
            return qr[0].add(BigInteger.ONE);
        }
        return qr[0];
    }

    private record ReferenceRange(boolean empty, long first, long last) {
        static ReferenceRange empty() {
            return new ReferenceRange(true, 0L, -1L);
        }
    }
}
