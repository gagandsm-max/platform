package com.platform.core.telemetry.read;

import com.platform.core.domain.DomainException;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class MetricObservationQueryTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");

    @Test
    void acceptsAllowlistedNumericSelector() {
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, "cpu.util", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                50
        );
        assertEquals(AllowedMetricSelector.CPU_UTIL, query.selector());
        assertEquals(50, query.limit());
    }

    @Test
    void rejectsUnknownMetricId() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, "free.form.metric", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        ));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void rejectsKindMismatchForCounterSelector() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, "if.hc_in_octets", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        ));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void rejectsInvertedWindow() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, "cpu.util", "NUMERIC",
                Instant.parse("2026-10-16T02:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        ));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void rejectsWindowBeyondMax() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, "cpu.util", "NUMERIC",
                Instant.parse("2026-10-01T00:00:00Z"),
                Instant.parse("2026-10-03T00:00:01Z"),
                null
        ));
        assertEquals("QUERY_LIMIT_EXCEEDED", ex.getCode());
    }

    @Test
    void rejectsLimitAboveCap() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, "cpu.util", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                MetricObservationQuery.MAX_LIMIT + 1
        ));
        assertEquals("QUERY_LIMIT_EXCEEDED", ex.getCode());
    }
}
