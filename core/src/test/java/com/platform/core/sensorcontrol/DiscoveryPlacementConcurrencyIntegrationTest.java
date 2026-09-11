package com.platform.core.sensorcontrol;

import com.platform.core.config.PersistenceConfig;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.discovery.DiscoveryMethod;
import com.platform.core.domain.discovery.DiscoveryRequest;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.discovery.DiscoveryStatus;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.DiscoveryRepository;
import com.platform.core.persistence.TenantRepository;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

import javax.sql.DataSource;
import java.util.ArrayList;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;

@Tag("postgres")
class DiscoveryPlacementConcurrencyIntegrationTest {

    private PostgresTestSupport pg;
    private TenantService tenantService;
    private SensorControlRepository sensorRepo;
    private SensorControlService sensorControl;
    private DiscoveryRepository discoveryRepo;
    private DiscoveryService discoveryService;
    private DiscoveryPlacementService placement;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();

        NamedParameterJdbcTemplate named = pg.namedJdbc();
        TenantRepository tenantRepo = new TenantRepository(named);
        tenantService = new TenantService(tenantRepo);
        sensorRepo = new SensorControlRepository(named);
        SecurityAuditEmitter audit = new SecurityAuditEmitter();
        sensorControl = new SensorControlService(sensorRepo, audit);
        discoveryRepo = new DiscoveryRepository(named);
        discoveryService = new DiscoveryService(discoveryRepo, tenantService);
        placement = new DiscoveryPlacementService(
                sensorRepo, discoveryService, audit, new DiscoveryPlacementMetrics());
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    private DiscoveryRequest createPending(UUID tenantId, String name, String idem) {
        return discoveryService.createDiscovery(
                tenantId,
                name,
                List.of("10.0.0.10"),
                List.of(DiscoveryMethod.ICMP),
                null,
                null,
                null,
                161,
                1000,
                1,
                idem,
                null,
                "tester"
        );
    }

    @Test
    void singlePlaceStillWorks() {
        Tenant tenant = tenantService.create("Single Place Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "single", "idem-single");

        DiscoveryRequest assigned = placement.placePendingRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.ASSIGNED, assigned.status());
        assertEquals("sensor-a", assigned.assignedSensorId());
    }

    @Test
    void twoConcurrentPlacesExactlyOneWinner() throws Exception {
        Tenant tenant = tenantService.create("Two Concurrent Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "race-2", "idem-race-2");

        PlacementRaceResult race = racePlacement(tenant.tenantId(), created.requestId(), 2);
        assertEquals(1, race.successes.get());
        assertEquals(1, race.conflicts.get());
        assertEquals(0, race.otherFailures.get());
        assertEquals("sensor-a", race.winner.get().assignedSensorId());

        DiscoveryRequest finalState = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.ASSIGNED, finalState.status());
        assertEquals("sensor-a", finalState.assignedSensorId());
    }

    @Test
    void nConcurrentPlacesExactlyOneWinner() throws Exception {
        Tenant tenant = tenantService.create("N Concurrent Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "race-n", "idem-race-n");

        int n = 8;
        PlacementRaceResult race = racePlacement(tenant.tenantId(), created.requestId(), n);
        assertEquals(1, race.successes.get());
        assertEquals(n - 1, race.conflicts.get());
        assertEquals(0, race.otherFailures.get());

        DiscoveryRequest finalState = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.ASSIGNED, finalState.status());
        assertEquals("sensor-a", finalState.assignedSensorId());
    }

    @Test
    void loserCannotOverwriteWinnerAssignment() throws Exception {
        Tenant tenant = tenantService.create("No Overwrite Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        sensorControl.enrollSensor("sensor-b", tenant.tenantId(), null, SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "no-overwrite", "idem-no-overwrite");

        DiscoveryRequest winner = placement.placePendingRequest(tenant.tenantId(), created.requestId());
        assertEquals("sensor-a", winner.assignedSensorId());

        // Direct first-placement CAS must refuse overwrite of existing assignment.
        assertFalse(discoveryRepo.assignSensorFirstPlacement(
                tenant.tenantId(), created.requestId(), "sensor-b"));

        DomainException conflict = assertThrows(DomainException.class,
                () -> discoveryService.assignSensorFirstPlacementFromControlPlane(
                        tenant.tenantId(), created.requestId(), "sensor-b"));
        assertEquals("DISCOVERY_PLACEMENT_CONFLICT", conflict.getCode());

        DiscoveryRequest still = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals("sensor-a", still.assignedSensorId());
        assertEquals(DiscoveryStatus.ASSIGNED, still.status());
    }

    @Test
    void wrongTenantFailClosed() {
        Tenant tenantA = tenantService.create("Tenant A Place", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant tenantB = tenantService.create("Tenant B Place", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-b", tenantB.tenantId(), null, SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenantA.tenantId(), "cross-tenant", "idem-cross-tenant");

        DomainException ex = assertThrows(DomainException.class,
                () -> placement.placePendingRequest(tenantB.tenantId(), created.requestId()));
        assertEquals("RESOURCE_NOT_FOUND", ex.getCode());

        DiscoveryRequest stillPending = discoveryService.getRequest(tenantA.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.PENDING, stillPending.status());
        assertNull(stillPending.assignedSensorId());
    }

    @Test
    void ineligibleSensorFailClosed() {
        Tenant tenant = tenantService.create("Ineligible Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-infra", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "ineligible", "idem-ineligible");

        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> placement.placePendingRequest(tenant.tenantId(), created.requestId()));
        assertEquals(SensorControlException.ERR_NO_ELIGIBLE_SENSOR, ex.getCode());

        DiscoveryRequest stillPending = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.PENDING, stillPending.status());
        assertNull(stillPending.assignedSensorId());
    }

    @Test
    void staleSecondPlaceAfterAssignFails() {
        Tenant tenant = tenantService.create("Stale Place Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "stale", "idem-stale");

        DiscoveryRequest assigned = placement.placePendingRequest(tenant.tenantId(), created.requestId());
        assertEquals("sensor-a", assigned.assignedSensorId());

        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> placement.placePendingRequest(tenant.tenantId(), created.requestId()));
        assertEquals(SensorControlException.ERR_DISCOVERY_REQUEST_NOT_PLACEABLE, ex.getCode());

        DiscoveryRequest still = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals("sensor-a", still.assignedSensorId());
        assertEquals(DiscoveryStatus.ASSIGNED, still.status());
    }

    @Test
    void rollbackPreservesPriorPendingState() {
        Tenant tenant = tenantService.create("Rollback Place Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "rollback", "idem-rollback");

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class, FailingAfterAssignDiscoveryConfig.class);
            ctx.registerBean(TenantRepository.class, () -> new TenantRepository(pg.namedJdbc()));
            ctx.registerBean(SensorControlRepository.class, () -> sensorRepo);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.registerBean(DiscoveryPlacementMetrics.class, DiscoveryPlacementMetrics::new);
            ctx.register(TenantService.class, DiscoveryService.class, DiscoveryPlacementService.class);
            ctx.refresh();

            DiscoveryPlacementService txPlacement = ctx.getBean(DiscoveryPlacementService.class);
            assertThrows(IllegalStateException.class,
                    () -> txPlacement.placePendingRequest(tenant.tenantId(), created.requestId()));
        }

        DiscoveryRequest stillPending = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.PENDING, stillPending.status());
        assertNull(stillPending.assignedSensorId());
    }

    private PlacementRaceResult racePlacement(UUID tenantId, UUID requestId, int threads) throws Exception {
        CountDownLatch start = new CountDownLatch(1);
        AtomicInteger successes = new AtomicInteger();
        AtomicInteger conflicts = new AtomicInteger();
        AtomicInteger otherFailures = new AtomicInteger();
        AtomicReference<DiscoveryRequest> winner = new AtomicReference<>();
        ExecutorService pool = Executors.newFixedThreadPool(threads);
        try {
            List<Future<?>> futures = new ArrayList<>(threads);
            for (int i = 0; i < threads; i++) {
                futures.add(pool.submit(() -> {
                    start.await();
                    try {
                        DiscoveryRequest assigned = placement.placePendingRequest(tenantId, requestId);
                        successes.incrementAndGet();
                        winner.set(assigned);
                    } catch (SensorControlException e) {
                        if (SensorControlException.ERR_DISCOVERY_PLACEMENT_CONFLICT.equals(e.getCode())
                                || SensorControlException.ERR_DISCOVERY_REQUEST_NOT_PLACEABLE.equals(e.getCode())) {
                            // Losers may see either conflict (CAS) or not-placeable (locked re-read).
                            conflicts.incrementAndGet();
                        } else {
                            otherFailures.incrementAndGet();
                            throw e;
                        }
                    }
                    return null;
                }));
            }
            start.countDown();
            for (Future<?> future : futures) {
                future.get(30, TimeUnit.SECONDS);
            }
        } finally {
            pool.shutdownNow();
        }
        return new PlacementRaceResult(successes, conflicts, otherFailures, winner);
    }

    private record PlacementRaceResult(
            AtomicInteger successes,
            AtomicInteger conflicts,
            AtomicInteger otherFailures,
            AtomicReference<DiscoveryRequest> winner
    ) {}

    static class FailingAfterAssignDiscoveryRepository extends DiscoveryRepository {
        FailingAfterAssignDiscoveryRepository(NamedParameterJdbcTemplate jdbc) {
            super(jdbc);
        }

        @Override
        public boolean assignSensorFirstPlacement(UUID tenantId, UUID requestId, String sensorId) {
            boolean ok = super.assignSensorFirstPlacement(tenantId, requestId, sensorId);
            if (ok) {
                throw new IllegalStateException("forced failure after first placement for rollback test");
            }
            return false;
        }
    }

    @Configuration
    static class FailingAfterAssignDiscoveryConfig {
        @Bean
        @Primary
        DiscoveryRepository discoveryRepository(NamedParameterJdbcTemplate jdbc) {
            return new FailingAfterAssignDiscoveryRepository(jdbc);
        }
    }
}
