package com.platform.core.sensorcontrol;

import com.platform.core.domain.discovery.DiscoveryRequest;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.discovery.DiscoveryStatus;
import com.platform.core.security.http.SecurityAuditEmitter;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class DiscoveryPlacementServiceTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private static final UUID OTHER_TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000002");
    private static final UUID REQUEST = UUID.fromString("018f0e0a-0000-7000-8000-000000000321");
    private static final UUID SITE_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID SITE_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a2");
    private static final Instant NOW = Instant.parse("2026-09-10T06:00:00Z");

    private SensorControlRepository sensorRepository;
    private DiscoveryService discoveryService;
    private SecurityAuditEmitter auditEmitter;
    private DiscoveryPlacementMetrics metrics;
    private DiscoveryPlacementService placement;

    @BeforeEach
    void setUp() {
        sensorRepository = mock(SensorControlRepository.class);
        discoveryService = mock(DiscoveryService.class);
        auditEmitter = mock(SecurityAuditEmitter.class);
        metrics = new DiscoveryPlacementMetrics();
        placement = new DiscoveryPlacementService(sensorRepository, discoveryService, auditEmitter, metrics);
    }

    private DiscoveryRequest pending(UUID siteId) {
        return new DiscoveryRequest(
                REQUEST, TENANT, "scan", DiscoveryStatus.PENDING, "corr", "idem",
                siteId, null, null, List.of(), null, 161, 1000, 1, "user",
                NOW, null, null, Map.of(), null
        );
    }

    private SensorPrincipal sensor(String id, SensorRole role, UUID siteId) {
        return new SensorPrincipal(id, TENANT, siteId, role, SensorLifecycleStatus.ACTIVE, true, NOW, NOW);
    }

    @Test
    void placesLexicographicallyFirstDiscoveryCapableSensor() {
        when(discoveryService.getRequest(TENANT, REQUEST)).thenReturn(pending(null));
        when(sensorRepository.listActivePrincipalsByTenant(TENANT)).thenReturn(List.of(
                sensor("sensor-z", SensorRole.NETWORK, null),
                sensor("sensor-a", SensorRole.INFRASTRUCTURE, null),
                sensor("sensor-b", SensorRole.NETWORK, null),
                sensor("sensor-c", SensorRole.COMBINED, null)
        ));
        DiscoveryRequest assigned = pending(null);
        when(discoveryService.assignSensorFromControlPlane(TENANT, REQUEST, "sensor-b"))
                .thenReturn(assigned);

        // sensor-a is INFRASTRUCTURE (no DISCOVERY); among NETWORK/COMBINED, sensor-b < sensor-c < sensor-z
        placement.placePendingRequest(TENANT, REQUEST);
        verify(discoveryService).assignSensorFromControlPlane(TENANT, REQUEST, "sensor-b");
        assertEquals(1, metrics.getPlacementSuccesses());
        assertEquals(1, metrics.getReasonCount(DiscoveryPlacementMetrics.REASON_ASSIGNED));
    }

    @Test
    void infrastructureOnlyIsNotEligibleForDiscovery() {
        when(discoveryService.getRequest(TENANT, REQUEST)).thenReturn(pending(null));
        when(sensorRepository.listActivePrincipalsByTenant(TENANT)).thenReturn(List.of(
                sensor("sensor-infra", SensorRole.INFRASTRUCTURE, null)
        ));
        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> placement.placePendingRequest(TENANT, REQUEST));
        assertEquals(SensorControlException.ERR_NO_ELIGIBLE_SENSOR, ex.getCode());
        verify(discoveryService, never()).assignSensorFromControlPlane(any(), any(), any());
        assertEquals(1, metrics.getReasonCount(DiscoveryPlacementMetrics.REASON_NO_ELIGIBLE_SENSOR));
    }

    @Test
    void siteScopedRequestRequiresMatchingSensorSite() {
        when(discoveryService.getRequest(TENANT, REQUEST)).thenReturn(pending(SITE_A));
        when(sensorRepository.listActivePrincipalsByTenant(TENANT)).thenReturn(List.of(
                sensor("sensor-wide", SensorRole.NETWORK, null),
                sensor("sensor-b", SensorRole.NETWORK, SITE_B),
                sensor("sensor-a", SensorRole.NETWORK, SITE_A)
        ));
        when(discoveryService.assignSensorFromControlPlane(TENANT, REQUEST, "sensor-a"))
                .thenReturn(pending(SITE_A));

        placement.placePendingRequest(TENANT, REQUEST);
        verify(discoveryService).assignSensorFromControlPlane(TENANT, REQUEST, "sensor-a");
    }

    @Test
    void noEligibleLeavesPendingWithoutInventingSensor() {
        when(discoveryService.getRequest(TENANT, REQUEST)).thenReturn(pending(SITE_A));
        when(sensorRepository.listActivePrincipalsByTenant(TENANT)).thenReturn(List.of(
                sensor("sensor-wide", SensorRole.NETWORK, null)
        ));
        assertThrows(SensorControlException.class, () -> placement.placePendingRequest(TENANT, REQUEST));
        verify(discoveryService, never()).assignSensorFromControlPlane(any(), any(), any());
    }

    @Test
    void listActiveIsTenantScopedSoCrossTenantSensorsAreInvisible() {
        when(discoveryService.getRequest(TENANT, REQUEST)).thenReturn(pending(null));
        // Repository is queried with request tenant only — other-tenant sensors never appear.
        when(sensorRepository.listActivePrincipalsByTenant(TENANT)).thenReturn(List.of());
        when(sensorRepository.listActivePrincipalsByTenant(OTHER_TENANT)).thenReturn(List.of(
                new SensorPrincipal("other-sensor", OTHER_TENANT, null, SensorRole.NETWORK,
                        SensorLifecycleStatus.ACTIVE, true, NOW, NOW)
        ));
        assertThrows(SensorControlException.class, () -> placement.placePendingRequest(TENANT, REQUEST));
        verify(sensorRepository).listActivePrincipalsByTenant(TENANT);
        verify(sensorRepository, never()).listActivePrincipalsByTenant(OTHER_TENANT);
        verify(discoveryService, never()).assignSensorFromControlPlane(any(), any(), any());
    }

    @Test
    void siteCompatibilityPolicyDocumented() {
        assertTrue(DiscoveryPlacementService.siteCompatible(null, null));
        assertTrue(DiscoveryPlacementService.siteCompatible(null, SITE_A));
        assertFalse(DiscoveryPlacementService.siteCompatible(SITE_A, null));
        assertTrue(DiscoveryPlacementService.siteCompatible(SITE_A, SITE_A));
        assertFalse(DiscoveryPlacementService.siteCompatible(SITE_A, SITE_B));
    }

    @Test
    void nonPendingRequestRejected() {
        DiscoveryRequest assigned = new DiscoveryRequest(
                REQUEST, TENANT, "scan", DiscoveryStatus.ASSIGNED, "corr", "idem",
                null, null, "sensor-a", List.of(), null, 161, 1000, 1, "user",
                NOW, NOW, null, Map.of(), null
        );
        when(discoveryService.getRequest(TENANT, REQUEST)).thenReturn(assigned);
        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> placement.placePendingRequest(TENANT, REQUEST));
        assertEquals(SensorControlException.ERR_DISCOVERY_REQUEST_NOT_PLACEABLE, ex.getCode());
        verify(discoveryService, never()).assignSensorFromControlPlane(eq(TENANT), eq(REQUEST), any());
        assertEquals(1, metrics.getReasonCount(DiscoveryPlacementMetrics.REASON_NOT_PLACEABLE));
        assertEquals(1, metrics.getPlacementFailures());
    }
}
