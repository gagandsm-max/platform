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
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
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
    void authorizedEntityWithGoodFreshRowsReturnsOk() {
        MetricObservationQuery query = liveQuery();
        when(entities.get(TENANT, ENTITY)).thenReturn(entity());
        when(reader.read(query)).thenReturn(new MetricObservationPage(
                List.of(row(Instant.parse("2026-10-16T00:55:00Z"), 1)), false));

        var result = service.read(query, Instant.parse("2026-10-16T01:00:00Z"));
        assertEquals(ObservationReadState.OK, result.observationState());
        assertEquals(FreshnessClass.FRESH, result.freshness());
        assertEquals(1, result.observations().size());
        assertFalse(result.truncated());
        verify(entities).get(TENANT, ENTITY);
    }

    @Test
    void missingEntityIs404AndDoesNotCallClickHouse() {
        MetricObservationQuery query = liveQuery();
        when(entities.get(TENANT, ENTITY))
                .thenThrow(new DomainException("ENTITY_NOT_FOUND", "Entity not found"));
        DomainException ex = assertThrows(DomainException.class, () -> service.read(query, Instant.now()));
        assertEquals("ENTITY_NOT_FOUND", ex.getCode());
        verifyNoInteractions(reader);
    }

    @Test
    void emptyWindowIsNoDataNotDeviceDown() {
        MetricObservationQuery query = liveQuery();
        when(entities.get(TENANT, ENTITY)).thenReturn(entity());
        when(reader.read(query)).thenReturn(MetricObservationPage.empty());

        var result = service.read(query, Instant.parse("2026-10-16T01:00:00Z"));
        assertEquals(ObservationReadState.NO_DATA, result.observationState());
        assertEquals(FreshnessClass.MISSING, result.freshness());
        assertTrue(result.qualityNote().contains("NO_DATA"));
        assertTrue(result.qualityNote().contains("not DEVICE_DOWN"));
    }

    @Test
    void unknownQualityZeroNeverBecomesHealthy() {
        MetricObservationQuery query = liveQuery();
        var result = service.classify(
                query,
                new MetricObservationPage(List.of(row(Instant.parse("2026-10-16T00:55:00Z"), 0)), false),
                Instant.parse("2026-10-16T01:00:00Z")
        );
        assertEquals(ObservationReadState.UNKNOWN, result.observationState());
        assertEquals(FreshnessClass.FRESH, result.freshness());
        assertTrue(result.qualityNote().contains("not promoted to healthy"));
    }

    @Test
    void degradedQualityIsMonitoringDegradedNotDeviceDown() {
        MetricObservationQuery query = liveQuery();
        var result = service.classify(
                query,
                new MetricObservationPage(List.of(row(Instant.parse("2026-10-16T00:55:00Z"), 2)), false),
                Instant.parse("2026-10-16T01:00:00Z")
        );
        assertEquals(ObservationReadState.MONITORING_DEGRADED, result.observationState());
        assertTrue(result.qualityNote().contains("not DEVICE_DOWN"));
    }

    @Test
    void liveWindowStaleRowsAreStaleNotDeviceDown() {
        MetricObservationQuery query = liveQuery();
        when(entities.get(TENANT, ENTITY)).thenReturn(entity());
        when(reader.read(query)).thenReturn(new MetricObservationPage(
                List.of(row(Instant.parse("2026-10-16T00:00:00Z"), 1)), false));

        var result = service.read(query, Instant.parse("2026-10-16T01:00:00Z"));
        assertEquals(ObservationReadState.STALE, result.observationState());
        assertEquals(FreshnessClass.STALE, result.freshness());
        assertTrue(result.qualityNote().contains("not DEVICE_DOWN"));
    }

    @Test
    void historicalWindowDoesNotBecomeCurrentStaleOrBlanketFresh() {
        // Window ended hours before live horizon; evaluatedAt is much later.
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, "system.cpu.utilization", "NUMERIC",
                Instant.parse("2026-10-15T00:00:00Z"),
                Instant.parse("2026-10-15T01:00:00Z"),
                100
        );
        var result = service.classify(
                query,
                new MetricObservationPage(List.of(row(Instant.parse("2026-10-15T00:55:00Z"), 1)), false),
                Instant.parse("2026-10-16T12:00:00Z")
        );
        assertEquals(FreshnessClass.HISTORICAL, result.freshness());
        assertEquals(ObservationReadState.OK, result.observationState());
        assertFalse(result.freshness() == FreshnessClass.STALE);
        assertFalse(result.freshness() == FreshnessClass.FRESH);
        assertTrue(result.qualityNote().contains("HISTORICAL"));
    }

    @Test
    void historicalWindowWithUnknownQualityStaysUnknown() {
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, "system.cpu.utilization", "NUMERIC",
                Instant.parse("2026-10-15T00:00:00Z"),
                Instant.parse("2026-10-15T01:00:00Z"),
                100
        );
        var result = service.classify(
                query,
                new MetricObservationPage(List.of(row(Instant.parse("2026-10-15T00:55:00Z"), 0)), false),
                Instant.parse("2026-10-16T12:00:00Z")
        );
        assertEquals(FreshnessClass.HISTORICAL, result.freshness());
        assertEquals(ObservationReadState.UNKNOWN, result.observationState());
    }

    @Test
    void truncationFlagSurfacedInResult() {
        MetricObservationQuery query = liveQuery();
        var result = service.classify(
                query,
                new MetricObservationPage(List.of(row(Instant.parse("2026-10-16T00:55:00Z"), 1)), true),
                Instant.parse("2026-10-16T01:00:00Z")
        );
        assertTrue(result.truncated());
        assertTrue(result.qualityNote().contains("truncated"));
    }

    @Test
    void unsupportedQualityThreeCannotBecomeHealthy() {
        MetricObservationQuery query = liveQuery();
        DomainException ex = assertThrows(DomainException.class, () -> service.classify(
                query,
                new MetricObservationPage(List.of(row(Instant.parse("2026-10-16T00:55:00Z"), 3)), false),
                Instant.parse("2026-10-16T01:00:00Z")
        ));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void entityLookupFailureNeverInvokesReader() {
        MetricObservationQuery query = liveQuery();
        when(entities.get(TENANT, ENTITY))
                .thenThrow(new DomainException("ENTITY_NOT_FOUND", "Entity not found"));
        assertThrows(DomainException.class, () -> service.read(query, Instant.now()));
        verify(reader, never()).read(query);
    }

    private static MetricObservationQuery liveQuery() {
        return MetricObservationQuery.parse(
                TENANT, ENTITY, "system.cpu.utilization", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                100
        );
    }

    private static MetricObservationRow row(Instant ts, int quality) {
        return new MetricObservationRow(ts, 42.0, null, quality, 0, "sensor-1", "seq-1", "agent");
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
