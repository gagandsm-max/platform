package com.platform.core.domain.discovery.scheduling;

import java.time.DayOfWeek;
import java.time.LocalDate;
import java.time.LocalTime;
import java.time.ZoneId;
import java.util.Objects;
import java.util.Set;

/** Immutable calendar inputs for PRD-ONB-021 / REQ-ONB-020; no execution authority. */
public sealed interface DiscoveryRecurrence {
    ZoneId zone();

    /** Inclusive lower bound on the nominal date, before timezone resolution. */
    LocalDate effectiveFrom();

    LocalTime requestedTime();

    record Daily(ZoneId zone, LocalDate effectiveFrom, LocalTime requestedTime)
            implements DiscoveryRecurrence {
        public Daily {
            validate(zone, effectiveFrom, requestedTime);
        }
    }

    record Weekly(ZoneId zone, LocalDate effectiveFrom, LocalTime requestedTime,
                  Set<DayOfWeek> weekdays) implements DiscoveryRecurrence {
        public Weekly {
            validate(zone, effectiveFrom, requestedTime);
            weekdays = Set.copyOf(Objects.requireNonNull(weekdays, "weekdays"));
            if (weekdays.isEmpty()) {
                throw new IllegalArgumentException("weekdays must not be empty");
            }
        }
    }

    /** Missing days resolve to the last valid day of that month. */
    record Monthly(ZoneId zone, LocalDate effectiveFrom, LocalTime requestedTime,
                   int dayOfMonth) implements DiscoveryRecurrence {
        public Monthly {
            validate(zone, effectiveFrom, requestedTime);
            if (dayOfMonth < 1 || dayOfMonth > 31) {
                throw new IllegalArgumentException("dayOfMonth must be between 1 and 31");
            }
        }
    }

    private static void validate(ZoneId zone, LocalDate effectiveFrom, LocalTime requestedTime) {
        requireNamedZone(zone);
        Objects.requireNonNull(effectiveFrom, "effectiveFrom");
        Objects.requireNonNull(requestedTime, "requestedTime");
    }

    static void requireNamedZone(ZoneId zone) {
        Objects.requireNonNull(zone, "zone");
        // Membership, rather than isFixedOffset(), permits named fixed-rule zones such as UTC
        // and Etc/GMT+5 while rejecting synthesized IDs such as UTC+05:30 and bare offsets.
        if (!ZoneId.getAvailableZoneIds().contains(zone.getId())) {
            throw new IllegalArgumentException("zone must be a named JDK timezone database ID");
        }
    }
}
