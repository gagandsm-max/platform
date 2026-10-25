package com.platform.core.telemetry.read;

import com.platform.core.domain.DomainException;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class MetricObservationQueryTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");

    @Test
    void acceptsRealAgentCpuUtilizationKey() {
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, "system.cpu.utilization", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                50
        );
        assertEquals("system.cpu.utilization", query.selector().metricId());
        assertEquals(MetricValueKind.NUMERIC, query.selector().valueKind());
        assertEquals(50, query.limit());
    }

    @Test
    void acceptsRealAgentMemoryUtilizationKey() {
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, "system.memory.utilization", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        );
        assertEquals("system.memory.utilization", query.selector().metricId());
    }

    @Test
    void acceptsFullArcSnmpIfOperStatusWithBoundedInstance() {
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, ".1.3.6.1.2.1.2.2.1.8.1", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                10
        );
        assertEquals(".1.3.6.1.2.1.2.2.1.8.1", query.selector().metricId());
        assertEquals(MetricValueKind.NUMERIC, query.selector().valueKind());
    }

    @Test
    void acceptsFullArcSnmpIfHcInOctetsCounter64() {
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, ".1.3.6.1.2.1.31.1.1.1.6.1", "COUNTER64",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                10
        );
        assertEquals(".1.3.6.1.2.1.31.1.1.1.6.1", query.selector().metricId());
        assertEquals(MetricValueKind.COUNTER64, query.selector().valueKind());
    }

    @Test
    void acceptsFullArcSnmpIfHcOutOctetsCounter64() {
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, ".1.3.6.1.2.1.31.1.1.1.10.42", "COUNTER64",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                10
        );
        assertEquals(".1.3.6.1.2.1.31.1.1.1.10.42", query.selector().metricId());
    }

    @Test
    void rejectsLegacyHumanAliasCpuUtil() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, "cpu.util", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        ));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void rejectsOidPrefixLookalikeIfOperStatus() {
        // ...1.81.1 must not match family ...1.8
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, ".1.3.6.1.2.1.2.2.1.81.1", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        ));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void rejectsOidFamilyWithoutInstanceArc() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, ".1.3.6.1.2.1.2.2.1.8", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        ));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void rejectsOidWithExtraTrailingArc() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, ".1.3.6.1.2.1.31.1.1.1.6.1.9", "COUNTER64",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        ));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void rejectsOidInstanceAboveBound() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, ".1.3.6.1.2.1.2.2.1.8.2147483648", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        ));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void rejectsOidInstanceZeroAndLeadingZero() {
        assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, ".1.3.6.1.2.1.2.2.1.8.0", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        ));
        assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, ".1.3.6.1.2.1.2.2.1.8.01", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        ));
    }

    @Test
    void rejectsKindMismatchForCounterOid() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, ".1.3.6.1.2.1.31.1.1.1.6.1", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        ));
        assertEquals("VALIDATION_FAILED", ex.getCode());
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
    void rejectsInvertedWindow() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, "system.cpu.utilization", "NUMERIC",
                Instant.parse("2026-10-16T02:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                null
        ));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void rejectsWindowBeyondMax() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, "system.cpu.utilization", "NUMERIC",
                Instant.parse("2026-10-01T00:00:00Z"),
                Instant.parse("2026-10-03T00:00:01Z"),
                null
        ));
        assertEquals("QUERY_LIMIT_EXCEEDED", ex.getCode());
    }

    @Test
    void rejectsLimitAboveCap() {
        DomainException ex = assertThrows(DomainException.class, () -> MetricObservationQuery.parse(
                TENANT, ENTITY, "system.cpu.utilization", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                MetricObservationQuery.MAX_LIMIT + 1
        ));
        assertEquals("QUERY_LIMIT_EXCEEDED", ex.getCode());
    }

    @Test
    void maxSnmpInstanceAccepted() {
        String oid = ".1.3.6.1.2.1.2.2.1.8." + AllowedMetricSelector.MAX_SNMP_INSTANCE;
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, oid, "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                1
        );
        assertTrue(query.selector().metricId().endsWith("." + AllowedMetricSelector.MAX_SNMP_INSTANCE));
    }
}
