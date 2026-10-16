package com.platform.core.telemetry.read;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.inventory.Entity;
import com.platform.core.domain.inventory.EntityService;
import com.platform.core.domain.inventory.LifecycleState;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.List;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class EntityMetricObservationServiceTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");

    private EntityService entities;
    private ClickHouseMetricObservationReader reader;
    private EntityMetricObservationService service;

    @BeforeEach
    void setUp() {
        entities = mock(EntityService.class);
        reader = mock(ClickHouseMetricObservationReader.class);
        service = new EntityMetricObservationService(entities, reader, 300);
    }

    @Test
    void authorizedEntityWithRowsReturnsOk() {
        MetricObservationQuery query = baseQuery();
        when(entities.get(TENANT, ENTITY)).thenReturn(entity());
        when(reader.read(query)).thenReturn(List.of(row(Instant.parse("2026-10-16T00:55:00Z"), 1)));

        var result = service.read(query, Instant.parse("2026-10-16T01:00:00Z"));
        assertEquals(ObservationReadState.OK, result.observationState());
        assertEquals(FreshnessClass.FRESH, result.freshness());
        assertEquals(1, result.observations().size());
        verify(entities).get(TENANT, ENTITY);
    }

    @Test
    void missingEntityIs404NotDeviceDown() {
        MetricObservationQuery query = baseQuery();
        when(entities.get(TENANT, ENTITY))
                .thenThrow(new DomainException("ENTITY_NOT_FOUND", "Entity not found"));
        DomainException ex = assertThrows(DomainException.class, () -> service.read(query, Instant.now()));
        assertEquals("ENTITY_NOT_FOUND", ex.getCode());
    }

    @Test
    void noDataIsUnknownNotDeviceDown() {
        MetricObservationQuery query = baseQuery();
        when(entities.get(TENANT, ENTITY)).thenReturn(entity());
        when(reader.read(query)).thenReturn(List.of());

        var result = service.read(query, Instant.parse("2026-10-16T01:00:00Z"));
        assertEquals(ObservationReadState.UNKNOWN, result.observationState());
        assertEquals(FreshnessClass.MISSING, result.freshness());
        assertTrueNote(result.qualityNote());
    }

    @Test
    void staleRowsAreStaleNotDeviceDown() {
        MetricObservationQuery query = baseQuery();
        when(entities.get(TENANT, ENTITY)).thenReturn(entity());
        when(reader.read(query)).thenReturn(List.of(row(Instant.parse("2026-10-16T00:00:00Z"), 1)));

        var result = service.read(query, Instant.parse("2026-10-16T01:00:00Z"));
        assertEquals(ObservationReadState.STALE, result.observationState());
        assertEquals(FreshnessClass.STALE, result.freshness());
        assertTrueNote(result.qualityNote());
    }

    @Test
    void degradedQualityIsMonitoringDegradedNotDeviceDown() {
        MetricObservationQuery query = baseQuery();
        var result = service.classify(
                query,
                List.of(row(Instant.parse("2026-10-16T00:55:00Z"), 2)),
                Instant.parse("2026-10-16T01:00:00Z")
        );
        assertEquals(ObservationReadState.MONITORING_DEGRADED, result.observationState());
        assertTrueNote(result.qualityNote());
    }

    private static void assertTrueNote(String note) {
        org.junit.jupiter.api.Assertions.assertTrue(note.contains("not DEVICE_DOWN"));
    }

    private static MetricObservationQuery baseQuery() {
        return MetricObservationQuery.parse(
                TENANT, ENTITY, "cpu.util", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                100
        );
    }

    private static MetricObservationRow row(Instant ts, int quality) {
        return new MetricObservationRow(ts, 42.0, null, quality, 0, "sensor-1", "seq-1", "snmp");
    }

    private static Entity entity() {
        Instant now = Instant.parse("2026-10-16T00:00:00Z");
        return new Entity(
                ENTITY, TENANT, "DEVICE", "router-1", null, null,
                LifecycleState.ACTIVE, null, null, null, null, null,
                now, now, "system", now, now
        );
    }
}
