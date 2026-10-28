package com.platform.core.domain.discovery.scheduling;

import java.time.DateTimeException;
import java.time.Instant;
import java.time.LocalDate;
import java.time.LocalDateTime;
import java.time.YearMonth;
import java.time.ZoneId;
import java.time.ZoneOffset;
import java.util.Objects;

/** Pure calendar calculation using the JDK timezone rules available at calculation time. */
public final class DiscoveryOccurrenceCalculator {
    private DiscoveryOccurrenceCalculator() {
    }

    /**
     * Returns the first resolved occurrence strictly after the cursor, without historical replay.
     * Effective dates apply to nominal dates, including dates removed by timezone transitions.
     *
     * @throws DateTimeException if the cursor or next occurrence exceeds the representable calendar
     *                          or instant range
     * @throws NullPointerException if either argument is null
     */
    public static Instant nextAfter(DiscoveryRecurrence recurrence, Instant afterExclusive) {
        Objects.requireNonNull(recurrence, "recurrence");
        Objects.requireNonNull(afterExclusive, "afterExclusive");

        // All Java zone offsets lie within +/-18 hours. Even gap-boundary resolution lies
        // within those bounds of the requested local datetime interpreted as UTC. Starting
        // one UTC date earlier therefore includes every potentially future nominal date,
        // including one hidden by a skipped date or an overlap across local midnight.
        LocalDate cursorDate = LocalDateTime.ofInstant(afterExclusive, ZoneOffset.UTC).toLocalDate();
        LocalDate start = cursorDate.equals(LocalDate.MIN) ? cursorDate : cursorDate.minusDays(1);
        if (start.isBefore(recurrence.effectiveFrom())) {
            start = recurrence.effectiveFrom();
        }
        LocalDate date = onOrAfter(recurrence, start);
        // Four daily candidates span beyond the +/-18-hour uncertainty; weekly/monthly
        // candidates advance at least as far. Work is independent of schedule age.
        for (int attempt = 0; attempt < 4; attempt++) {
            Instant resolved = resolve(date.atTime(recurrence.requestedTime()), recurrence.zone());
            if (resolved.isAfter(afterExclusive)) {
                return resolved;
            }
            date = onOrAfter(recurrence, date.plusDays(1));
        }
        throw new DateTimeException("No strictly future representable Discovery occurrence");
    }

    /**
     * Initially resolves Run Once using current timezone rules: gaps go to the first valid
     * instant after the gap; overlaps choose the earlier instant. The future caller must retain
     * the local input and zone, persist this result, and execute that saved instant. Re-resolution
     * after a timezone-database update is not promised to produce the same result.
     *
     * @throws DateTimeException if the result cannot be represented
     * @throws IllegalArgumentException if the zone is not a named JDK timezone database ID
     * @throws NullPointerException if either argument is null
     */
    public static Instant resolveRunOnce(LocalDateTime requestedLocal, ZoneId zone) {
        Objects.requireNonNull(requestedLocal, "requestedLocal");
        DiscoveryRecurrence.requireNamedZone(zone);
        return resolve(requestedLocal, zone);
    }

    private static LocalDate onOrAfter(DiscoveryRecurrence recurrence, LocalDate start) {
        return switch (recurrence) {
            case DiscoveryRecurrence.Daily daily -> start;
            case DiscoveryRecurrence.Weekly weekly -> {
                int days = weekly.weekdays().stream()
                        .mapToInt(day -> Math.floorMod(day.getValue() - start.getDayOfWeek().getValue(), 7))
                        .min().orElseThrow();
                yield start.plusDays(days);
            }
            case DiscoveryRecurrence.Monthly monthly -> {
                YearMonth month = YearMonth.from(start);
                LocalDate candidate = month.atDay(Math.min(monthly.dayOfMonth(), month.lengthOfMonth()));
                if (candidate.isBefore(start)) {
                    month = month.plusMonths(1);
                    candidate = month.atDay(Math.min(monthly.dayOfMonth(), month.lengthOfMonth()));
                }
                yield candidate;
            }
        };
    }

    private static Instant resolve(LocalDateTime requestedLocal, ZoneId zone) {
        var rules = zone.getRules();
        var offsets = rules.getValidOffsets(requestedLocal);
        if (offsets.isEmpty()) {
            // atZone() shifts by the gap duration, which would incorrectly turn 02:30 into
            // 03:30. The transition instant is the required first valid boundary (03:00).
            return rules.getTransition(requestedLocal).getInstant();
        }
        Instant earliest = requestedLocal.toInstant(offsets.get(0));
        for (int i = 1; i < offsets.size(); i++) {
            Instant alternative = requestedLocal.toInstant(offsets.get(i));
            if (alternative.isBefore(earliest)) {
                earliest = alternative;
            }
        }
        return earliest;
    }
}
