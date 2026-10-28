package com.platform.core.domain.discovery.scheduling;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.parallel.ResourceLock;

import java.time.DateTimeException;
import java.time.DayOfWeek;
import java.time.Instant;
import java.time.LocalDate;
import java.time.LocalDateTime;
import java.time.LocalTime;
import java.time.ZoneId;
import java.time.ZoneOffset;
import java.util.EnumSet;
import java.util.HashSet;
import java.util.Set;
import java.util.TimeZone;

import static com.platform.core.domain.discovery.scheduling.DiscoveryOccurrenceCalculator.nextAfter;
import static com.platform.core.domain.discovery.scheduling.DiscoveryOccurrenceCalculator.resolveRunOnce;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class DiscoveryOccurrenceCalculatorTest {
    private static final ZoneId UTC = ZoneId.of("UTC");
    private static final LocalDate START = LocalDate.of(2024, 1, 1);
    private static final LocalTime NINE = LocalTime.of(9, 0);

    @Test
    void dailyUsesStrictCursorAndInclusiveEffectiveDate() {
        var recurrence = new DiscoveryRecurrence.Daily(UTC, START, NINE);
        Instant first = Instant.parse("2024-01-01T09:00:00Z");
        assertEquals(first, nextAfter(recurrence, first.minusNanos(1)));
        assertEquals(first, nextAfter(recurrence, Instant.parse("2023-12-01T00:00:00Z")));
        assertEquals(Instant.parse("2024-01-02T09:00:00Z"), nextAfter(recurrence, first));
    }

    @Test
    void weeklyUsesOnlySelectedWeekdaysAndInclusiveStart() {
        var recurrence = new DiscoveryRecurrence.Weekly(UTC, START, NINE,
                Set.of(DayOfWeek.MONDAY, DayOfWeek.WEDNESDAY));
        Instant monday = Instant.parse("2024-01-01T09:00:00Z");
        Instant wednesday = Instant.parse("2024-01-03T09:00:00Z");
        assertEquals(monday, nextAfter(recurrence, monday.minusNanos(1)));
        assertEquals(wednesday, nextAfter(recurrence, monday));
        assertEquals(wednesday, nextAfter(recurrence, wednesday.minusNanos(1)));
        assertEquals(Instant.parse("2024-01-08T09:00:00Z"), nextAfter(recurrence, wednesday));
        var thursdayStart = new DiscoveryRecurrence.Weekly(UTC, START.plusDays(3), NINE,
                recurrence.weekdays());
        assertEquals(Instant.parse("2024-01-08T09:00:00Z"), nextAfter(thursdayStart, monday));
    }

    @Test
    void monthlyClampsFebruaryAndRestoresRequestedDayInMarch() {
        for (int year : new int[]{2023, 2024, 1900, 2000}) {
            var recurrence = new DiscoveryRecurrence.Monthly(UTC, LocalDate.of(year, 1, 1), NINE, 31);
            Instant january = LocalDate.of(year, 1, 31).atTime(NINE).toInstant(ZoneOffset.UTC);
            int februaryDay = LocalDate.of(year, 2, 1).lengthOfMonth();
            Instant february = LocalDate.of(year, 2, februaryDay).atTime(NINE).toInstant(ZoneOffset.UTC);
            Instant march = LocalDate.of(year, 3, 31).atTime(NINE).toInstant(ZoneOffset.UTC);
            assertEquals(january, nextAfter(recurrence, january.minusNanos(1)));
            assertEquals(february, nextAfter(recurrence, january));
            assertEquals(february, nextAfter(recurrence, february.minusNanos(1)));
            assertEquals(march, nextAfter(recurrence, february));
        }
    }

    @Test
    void monthlyEffectiveDateFiltersNominalDate() {
        var inclusive = new DiscoveryRecurrence.Monthly(UTC, LocalDate.of(2024, 2, 29), NINE, 31);
        assertEquals(Instant.parse("2024-02-29T09:00:00Z"),
                nextAfter(inclusive, Instant.parse("2024-01-01T00:00:00Z")));
        var pastThisMonth = new DiscoveryRecurrence.Monthly(UTC, LocalDate.of(2024, 2, 20), NINE, 15);
        assertEquals(Instant.parse("2024-03-15T09:00:00Z"),
                nextAfter(pastThisMonth, Instant.parse("2024-01-01T00:00:00Z")));
    }

    @Test
    void springGapResolvesToBoundaryRatherThanShiftingRequestedMinutes() {
        ZoneId zone = ZoneId.of("America/New_York");
        var recurrence = new DiscoveryRecurrence.Daily(zone, LocalDate.of(2024, 3, 10),
                LocalTime.of(2, 30));
        Instant boundary = Instant.parse("2024-03-10T07:00:00Z");
        assertEquals(boundary, nextAfter(recurrence, boundary.minusNanos(1)));
        assertEquals(Instant.parse("2024-03-11T06:30:00Z"), nextAfter(recurrence, boundary));
        assertEquals(boundary, resolveRunOnce(LocalDateTime.of(2024, 3, 10, 2, 30), zone));
    }

    @Test
    void overlapNeverSelectsLaterCopyForAnyRecurrence() {
        ZoneId zone = ZoneId.of("America/New_York");
        LocalDate date = LocalDate.of(2024, 11, 3);
        LocalTime time = LocalTime.of(1, 30);
        Instant earlier = Instant.parse("2024-11-03T05:30:00Z");
        Instant between = Instant.parse("2024-11-03T06:00:00Z");
        var daily = new DiscoveryRecurrence.Daily(zone, date, time);
        var weekly = new DiscoveryRecurrence.Weekly(zone, date, time, Set.of(DayOfWeek.SUNDAY));
        var monthly = new DiscoveryRecurrence.Monthly(zone, date, time, 3);
        for (DiscoveryRecurrence recurrence : new DiscoveryRecurrence[]{daily, weekly, monthly}) {
            assertEquals(earlier, nextAfter(recurrence, earlier.minusNanos(1)));
        }
        assertEquals(Instant.parse("2024-11-04T06:30:00Z"), nextAfter(daily, between));
        assertEquals(Instant.parse("2024-11-04T06:30:00Z"), nextAfter(daily, earlier));
        assertEquals(Instant.parse("2024-11-10T06:30:00Z"), nextAfter(weekly, between));
        assertEquals(Instant.parse("2024-12-03T06:30:00Z"), nextAfter(monthly, between));
        assertEquals(earlier, resolveRunOnce(date.atTime(time), zone));
    }

    @Test
    void lordHoweHandlesHalfHourGapAndOverlap() {
        ZoneId zone = ZoneId.of("Australia/Lord_Howe");
        var gap = new DiscoveryRecurrence.Daily(zone, LocalDate.of(2024, 10, 6), LocalTime.of(2, 15));
        Instant boundary = Instant.parse("2024-10-05T15:30:00Z");
        assertEquals(boundary, nextAfter(gap, boundary.minusNanos(1)));
        assertEquals(boundary, resolveRunOnce(LocalDateTime.of(2024, 10, 6, 2, 15), zone));
        var overlap = new DiscoveryRecurrence.Daily(zone, LocalDate.of(2024, 4, 7), LocalTime.of(1, 45));
        assertEquals(Instant.parse("2024-04-06T14:45:00Z"),
                nextAfter(overlap, Instant.parse("2024-04-06T14:44:59Z")));
        assertEquals(Instant.parse("2024-04-07T15:15:00Z"),
                nextAfter(overlap, Instant.parse("2024-04-06T15:00:00Z")));
    }

    @Test
    void skippedApiaDateRemainsEligibleByNominalEffectiveDate() {
        ZoneId zone = ZoneId.of("Pacific/Apia");
        LocalDate skipped = LocalDate.of(2011, 12, 30);
        Instant boundary = Instant.parse("2011-12-30T10:00:00Z");
        var daily = new DiscoveryRecurrence.Daily(zone, skipped, NINE);
        var weekly = new DiscoveryRecurrence.Weekly(zone, skipped, NINE, Set.of(DayOfWeek.FRIDAY));
        var monthly = new DiscoveryRecurrence.Monthly(zone, skipped, NINE, 30);
        for (DiscoveryRecurrence recurrence : new DiscoveryRecurrence[]{daily, weekly, monthly}) {
            assertEquals(boundary, nextAfter(recurrence, boundary.minusNanos(1)));
        }
        assertEquals(boundary, resolveRunOnce(skipped.atTime(NINE), zone));
        // Dec 30 00:00 (gap) and Dec 31 00:00 resolve to the same instant. Neither repeats.
        var midnight = new DiscoveryRecurrence.Daily(zone, skipped, LocalTime.MIDNIGHT);
        assertEquals(boundary, nextAfter(midnight, boundary.minusNanos(1)));
        Instant next = nextAfter(midnight, boundary);
        assertEquals(Instant.parse("2011-12-31T10:00:00Z"), next);
        assertTrue(next.isAfter(boundary));
    }

    @Test
    void fractionalHourZonesAndNamedFixedRuleZonesAreAccepted() {
        assertEquals(Instant.parse("2024-01-01T03:30:00Z"), nextAfter(
                new DiscoveryRecurrence.Daily(ZoneId.of("Asia/Kolkata"), START, NINE),
                Instant.parse("2024-01-01T00:00:00Z")));
        assertEquals(Instant.parse("2024-01-01T03:15:00Z"), nextAfter(
                new DiscoveryRecurrence.Daily(ZoneId.of("Asia/Kathmandu"), START, NINE),
                Instant.parse("2024-01-01T00:00:00Z")));
        assertEquals(Instant.parse("2024-01-01T14:00:00Z"), nextAfter(
                new DiscoveryRecurrence.Daily(ZoneId.of("Etc/GMT+5"), START, NINE),
                Instant.parse("2024-01-01T00:00:00Z")));
    }

    @Test
    @ResourceLock("java.util.TimeZone.default")
    void defaultTimezoneCannotChangeRecurringOrRunOnceCalculations() {
        TimeZone original = TimeZone.getDefault();
        try {
            for (String defaultId : new String[]{"Pacific/Honolulu", "Asia/Tokyo", "UTC"}) {
                TimeZone.setDefault(TimeZone.getTimeZone(defaultId));
                ZoneId zone = ZoneId.of("Asia/Kathmandu");
                var recurrence = new DiscoveryRecurrence.Daily(zone, START, NINE);
                Instant expected = Instant.parse("2024-01-01T03:15:00Z");
                assertEquals(expected, nextAfter(recurrence, expected.minusNanos(1)));
                assertEquals(expected, resolveRunOnce(START.atTime(NINE), zone));
                assertEquals(resolveRunOnce(START.atTime(NINE), zone),
                        resolveRunOnce(START.atTime(NINE), zone));
            }
        } finally {
            TimeZone.setDefault(original);
        }
    }

    @Test
    void jumpsAcrossMillionsOfYearsWithoutReplayingHistory() {
        LocalDate ancient = LocalDate.of(-900_000_000, 1, 1);
        LocalDate future = LocalDate.of(900_000_000, 1, 1);
        Instant cursor = future.atTime(NINE).toInstant(ZoneOffset.UTC);
        assertEquals(future.plusDays(1).atTime(NINE).toInstant(ZoneOffset.UTC),
                nextAfter(new DiscoveryRecurrence.Daily(UTC, ancient, NINE), cursor));
        assertEquals(future.plusMonths(1).atTime(NINE).toInstant(ZoneOffset.UTC),
                nextAfter(new DiscoveryRecurrence.Monthly(UTC, ancient, NINE, 1), cursor));
        var weekly = new DiscoveryRecurrence.Weekly(UTC, ancient, NINE, Set.of(future.getDayOfWeek()));
        assertEquals(future.plusDays(7).atTime(NINE).toInstant(ZoneOffset.UTC), nextAfter(weekly, cursor));
    }

    @Test
    void weeklyWeekdaysAreCopiedAndImmutable() {
        var input = EnumSet.of(DayOfWeek.MONDAY, DayOfWeek.WEDNESDAY);
        var recurrence = new DiscoveryRecurrence.Weekly(UTC, START, NINE, input);
        input.clear();
        input.add(DayOfWeek.FRIDAY);
        assertEquals(Set.of(DayOfWeek.MONDAY, DayOfWeek.WEDNESDAY), recurrence.weekdays());
        assertThrows(UnsupportedOperationException.class, () -> recurrence.weekdays().clear());
    }

    @Test
    void rejectsMissingFieldsEmptyOrNullWeekdaysAndInvalidMonthlyDays() {
        assertThrows(NullPointerException.class, () -> new DiscoveryRecurrence.Daily(null, START, NINE));
        assertThrows(NullPointerException.class, () -> new DiscoveryRecurrence.Daily(UTC, null, NINE));
        assertThrows(NullPointerException.class, () -> new DiscoveryRecurrence.Daily(UTC, START, null));
        assertThrows(NullPointerException.class, () -> new DiscoveryRecurrence.Weekly(UTC, START, NINE, null));
        assertThrows(IllegalArgumentException.class,
                () -> new DiscoveryRecurrence.Weekly(UTC, START, NINE, Set.of()));
        Set<DayOfWeek> nullDay = new HashSet<>();
        nullDay.add(null);
        assertThrows(NullPointerException.class,
                () -> new DiscoveryRecurrence.Weekly(UTC, START, NINE, nullDay));
        for (int day : new int[]{0, -1, 32, Integer.MAX_VALUE}) {
            assertThrows(IllegalArgumentException.class,
                    () -> new DiscoveryRecurrence.Monthly(UTC, START, NINE, day));
        }
        var daily = new DiscoveryRecurrence.Daily(UTC, START, NINE);
        assertThrows(NullPointerException.class, () -> nextAfter(null, Instant.EPOCH));
        assertThrows(NullPointerException.class, () -> nextAfter(daily, null));
        assertThrows(NullPointerException.class, () -> resolveRunOnce(null, UTC));
        assertThrows(NullPointerException.class, () -> resolveRunOnce(START.atTime(NINE), null));
    }

    @Test
    void rejectsBareAndSynthesizedOffsetsAcrossAllEntryPoints() {
        for (ZoneId zone : new ZoneId[]{ZoneOffset.UTC, ZoneOffset.of("+05:30"),
                ZoneId.of("UTC+05:30"), ZoneId.of("GMT-04:00")}) {
            assertThrows(IllegalArgumentException.class, () -> new DiscoveryRecurrence.Daily(zone, START, NINE));
            assertThrows(IllegalArgumentException.class,
                    () -> new DiscoveryRecurrence.Weekly(zone, START, NINE, Set.of(DayOfWeek.MONDAY)));
            assertThrows(IllegalArgumentException.class,
                    () -> new DiscoveryRecurrence.Monthly(zone, START, NINE, 1));
            assertThrows(IllegalArgumentException.class, () -> resolveRunOnce(START.atTime(NINE), zone));
        }
    }

    @Test
    void rangeBoundariesReturnRepresentableFutureOrThrowRatherThanWrap() {
        var daily = new DiscoveryRecurrence.Daily(UTC, LocalDate.MIN, LocalTime.MIDNIGHT);
        Instant first = LocalDate.MIN.atStartOfDay().toInstant(ZoneOffset.UTC);
        assertThrows(DateTimeException.class, () -> nextAfter(daily, first.minusNanos(1)));
        assertEquals(first.plusSeconds(86_400), nextAfter(daily, first));
        Instant last = LocalDate.MAX.atTime(LocalTime.MAX).toInstant(ZoneOffset.UTC);
        var lastDaily = new DiscoveryRecurrence.Daily(UTC, LocalDate.MAX, LocalTime.MAX);
        assertEquals(last, nextAfter(lastDaily, last.minusNanos(1)));
        assertThrows(DateTimeException.class, () -> nextAfter(lastDaily, last));
        assertThrows(DateTimeException.class, () -> nextAfter(daily, Instant.MIN));
        assertThrows(DateTimeException.class, () -> nextAfter(daily, Instant.MAX));
        var weekly = new DiscoveryRecurrence.Weekly(UTC, LocalDate.MAX, NINE,
                Set.of(LocalDate.MAX.getDayOfWeek().plus(1)));
        assertThrows(DateTimeException.class, () -> nextAfter(weekly, last.minusSeconds(86_400)));
        var monthly = new DiscoveryRecurrence.Monthly(UTC, LocalDate.MAX, NINE, 1);
        assertThrows(DateTimeException.class, () -> nextAfter(monthly, last.minusSeconds(86_400)));
        assertEquals(last, resolveRunOnce(LocalDate.MAX.atTime(LocalTime.MAX), UTC));
    }
}
