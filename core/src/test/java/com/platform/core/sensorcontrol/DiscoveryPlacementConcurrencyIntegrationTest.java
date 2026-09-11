package com.platform.core.sensorcontrol;

import com.platform.core.config.PersistenceConfig;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.discovery.DiscoveryMethod;
import com.platform.core.domain.discovery.DiscoveryRequest;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.discovery.DiscoveryStatus;
import com.platform.core.domain.inventory.Site;
import com.platform.core.domain.inventory.SiteService;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.DiscoveryRepository;
import com.platform.core.persistence.SiteRepository;
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
import java.util.Map;
import java.util.Set;
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
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * PostgreSQL proof that first-placement is single-winner under concurrency.
 * Uses a Spring TX-proxied {@link DiscoveryPlacementService} so production
 * {@code @Transactional} + {@code FOR UPDATE} boundaries are exercised (not only autocommit CAS).
 */
@Tag("postgres")
class DiscoveryPlacementConcurrencyIntegrationTest {

    private PostgresTestSupport pg;
    private AnnotationConfigApplicationContext ctx;
    private TenantService tenantService;
    private SiteService siteService;
    private SensorControlRepository sensorRepo;
    private SensorControlService sensorControl;
    private DiscoveryRepository discoveryRepo;
    private DiscoveryService discoveryService;
    private DiscoveryPlacementService placement;
    private NamedParameterJdbcTemplate named;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        named = pg.namedJdbc();

        ctx = new AnnotationConfigApplicationContext();
        ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
        ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
        ctx.register(PersistenceConfig.class);
        ctx.registerBean(TenantRepository.class, () -> new TenantRepository(named));
        ctx.registerBean(SiteRepository.class, () -> new SiteRepository(named));
        ctx.registerBean(SensorControlRepository.class, () -> new SensorControlRepository(named));
        ctx.registerBean(DiscoveryRepository.class, () -> new DiscoveryRepository(named));
        ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
        ctx.registerBean(DiscoveryPlacementMetrics.class, DiscoveryPlacementMetrics::new);
        ctx.register(
                TenantService.class,
                SiteService.class,
                SensorControlService.class,
                DiscoveryService.class,
                DiscoveryPlacementService.class
        );
        ctx.refresh();

        tenantService = ctx.getBean(TenantService.class);
        siteService = ctx.getBean(SiteService.class);
        sensorRepo = ctx.getBean(SensorControlRepository.class);
        sensorControl = ctx.getBean(SensorControlService.class);
        discoveryRepo = ctx.getBean(DiscoveryRepository.class);
        discoveryService = ctx.getBean(DiscoveryService.class);
        placement = ctx.getBean(DiscoveryPlacementService.class);
    }

    @AfterEach
    void tearDown() {
        if (ctx != null) {
            ctx.close();
        }
        if (pg != null) {
            pg.stop();
        }
    }

    private DiscoveryRequest createPending(UUID tenantId, String name, String idem) {
        return createPending(tenantId, name, idem, null);
    }

    private DiscoveryRequest createPending(UUID tenantId, String name, String idem, UUID siteId) {
        return discoveryService.createDiscovery(
                tenantId,
                name,
                List.of("10.0.0.10"),
                List.of(DiscoveryMethod.ICMP),
                null,
                siteId,
                null,
                161,
                1000,
                1,
                idem,
                null,
                "tester"
        );
    }

    private long assignmentVersion(UUID tenantId, UUID requestId) {
        Long version = named.queryForObject(
                """
                        SELECT assignment_version
                          FROM platform.discovery_request
                         WHERE tenant_id = :tenantId AND request_id = :requestId
                        """,
                Map.of("tenantId", tenantId, "requestId", requestId),
                Long.class
        );
        return version == null ? -1L : version;
    }

    @Test
    void singlePlaceStillWorks() {
        Tenant tenant = tenantService.create("Single Place Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "single", "idem-single");
        assertEquals(0L, assignmentVersion(tenant.tenantId(), created.requestId()));

        DiscoveryRequest assigned = placement.placePendingRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.ASSIGNED, assigned.status());
        assertEquals("sensor-a", assigned.assignedSensorId());
        assertEquals(1L, assignmentVersion(tenant.tenantId(), created.requestId()));
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
        assertEquals(1L, assignmentVersion(tenant.tenantId(), created.requestId()));
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
        assertEquals(1L, assignmentVersion(tenant.tenantId(), created.requestId()));
    }

    @Test
    void concurrentDualWriterDifferentSensorsOneAuthoritativeAssignment() throws Exception {
        Tenant tenant = tenantService.create("Dual Writer Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        sensorControl.enrollSensor("sensor-b", tenant.tenantId(), null, SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "dual-writer", "idem-dual-writer");

        CountDownLatch start = new CountDownLatch(1);
        AtomicInteger successes = new AtomicInteger();
        AtomicInteger conflicts = new AtomicInteger();
        AtomicReference<String> winnerSensor = new AtomicReference<>();
        ExecutorService pool = Executors.newFixedThreadPool(2);
        try {
            List<Future<?>> futures = List.of(
                    pool.submit(() -> {
                        raceFirstPlacement(start, tenant.tenantId(), created.requestId(),
                                "sensor-a", successes, conflicts, winnerSensor);
                        return null;
                    }),
                    pool.submit(() -> {
                        raceFirstPlacement(start, tenant.tenantId(), created.requestId(),
                                "sensor-b", successes, conflicts, winnerSensor);
                        return null;
                    })
            );
            start.countDown();
            for (Future<?> future : futures) {
                future.get(30, TimeUnit.SECONDS);
            }
        } finally {
            pool.shutdownNow();
        }

        assertEquals(1, successes.get());
        assertEquals(1, conflicts.get());
        DiscoveryRequest finalState = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.ASSIGNED, finalState.status());
        assertTrue(Set.of("sensor-a", "sensor-b").contains(finalState.assignedSensorId()));
        assertEquals(winnerSensor.get(), finalState.assignedSensorId());
        assertEquals(1L, assignmentVersion(tenant.tenantId(), created.requestId()));
    }

    @Test
    void loserCannotOverwriteWinnerAssignment() {
        Tenant tenant = tenantService.create("No Overwrite Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        sensorControl.enrollSensor("sensor-b", tenant.tenantId(), null, SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "no-overwrite", "idem-no-overwrite");

        DiscoveryRequest winner = placement.placePendingRequest(tenant.tenantId(), created.requestId());
        assertEquals("sensor-a", winner.assignedSensorId());
        assertEquals(1L, assignmentVersion(tenant.tenantId(), created.requestId()));

        assertFalse(discoveryRepo.assignSensorFirstPlacement(
                tenant.tenantId(), created.requestId(), "sensor-b"));

        DomainException conflict = assertThrows(DomainException.class,
                () -> discoveryService.assignSensorFirstPlacementFromControlPlane(
                        tenant.tenantId(), created.requestId(), "sensor-b"));
        assertEquals("DISCOVERY_PLACEMENT_CONFLICT", conflict.getCode());

        DiscoveryRequest still = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals("sensor-a", still.assignedSensorId());
        assertEquals(DiscoveryStatus.ASSIGNED, still.status());
        assertEquals(1L, assignmentVersion(tenant.tenantId(), created.requestId()));
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
        assertEquals(0L, assignmentVersion(tenantA.tenantId(), created.requestId()));
    }

    @Test
    void roleIneligibleSensorFailClosed() {
        Tenant tenant = tenantService.create("Ineligible Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-infra", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "ineligible", "idem-ineligible");

        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> placement.placePendingRequest(tenant.tenantId(), created.requestId()));
        assertEquals(SensorControlException.ERR_NO_ELIGIBLE_SENSOR, ex.getCode());

        DiscoveryRequest stillPending = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.PENDING, stillPending.status());
        assertNull(stillPending.assignedSensorId());
        assertEquals(0L, assignmentVersion(tenant.tenantId(), created.requestId()));
    }

    @Test
    void inactiveSensorFailClosed() {
        Tenant tenant = tenantService.create("Inactive Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        sensorControl.disableSensor("sensor-a");
        DiscoveryRequest created = createPending(tenant.tenantId(), "inactive", "idem-inactive");

        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> placement.placePendingRequest(tenant.tenantId(), created.requestId()));
        assertEquals(SensorControlException.ERR_NO_ELIGIBLE_SENSOR, ex.getCode());

        DiscoveryRequest stillPending = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.PENDING, stillPending.status());
        assertNull(stillPending.assignedSensorId());
        assertEquals(0L, assignmentVersion(tenant.tenantId(), created.requestId()));
    }

    @Test
    void siteIncompatibleSensorFailClosed() {
        Tenant tenant = tenantService.create("Site Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Site siteA = siteService.create(tenant.tenantId(), null, "Site A", "A", "UTC");
        Site siteB = siteService.create(tenant.tenantId(), null, "Site B", "B", "UTC");
        sensorControl.enrollSensor("sensor-b", tenant.tenantId(), siteB.siteId(), SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "site-miss", "idem-site-miss", siteA.siteId());

        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> placement.placePendingRequest(tenant.tenantId(), created.requestId()));
        assertEquals(SensorControlException.ERR_NO_ELIGIBLE_SENSOR, ex.getCode());

        DiscoveryRequest stillPending = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.PENDING, stillPending.status());
        assertNull(stillPending.assignedSensorId());
        assertEquals(0L, assignmentVersion(tenant.tenantId(), created.requestId()));
    }

    @Test
    void staleSecondPlaceAfterAssignFailsWithoutBumpingGeneration() {
        Tenant tenant = tenantService.create("Stale Place Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        sensorControl.enrollSensor("sensor-b", tenant.tenantId(), null, SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "stale", "idem-stale");

        DiscoveryRequest assigned = placement.placePendingRequest(tenant.tenantId(), created.requestId());
        assertEquals("sensor-a", assigned.assignedSensorId());
        assertEquals(1L, assignmentVersion(tenant.tenantId(), created.requestId()));

        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> placement.placePendingRequest(tenant.tenantId(), created.requestId()));
        assertEquals(SensorControlException.ERR_DISCOVERY_REQUEST_NOT_PLACEABLE, ex.getCode());

        assertFalse(discoveryRepo.assignSensorFirstPlacement(
                tenant.tenantId(), created.requestId(), "sensor-b"));

        DiscoveryRequest still = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals("sensor-a", still.assignedSensorId());
        assertEquals(DiscoveryStatus.ASSIGNED, still.status());
        assertEquals(1L, assignmentVersion(tenant.tenantId(), created.requestId()));
    }

    @Test
    void rollbackPreservesPriorPendingState() {
        Tenant tenant = tenantService.create("Rollback Place Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        DiscoveryRequest created = createPending(tenant.tenantId(), "rollback", "idem-rollback");

        try (AnnotationConfigApplicationContext failCtx = new AnnotationConfigApplicationContext()) {
            failCtx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            failCtx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            failCtx.register(PersistenceConfig.class, FailingAfterAssignDiscoveryConfig.class);
            failCtx.registerBean(TenantRepository.class, () -> new TenantRepository(named));
            failCtx.registerBean(SensorControlRepository.class, () -> new SensorControlRepository(named));
            failCtx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            failCtx.registerBean(DiscoveryPlacementMetrics.class, DiscoveryPlacementMetrics::new);
            failCtx.register(TenantService.class, DiscoveryService.class, DiscoveryPlacementService.class);
            failCtx.refresh();

            DiscoveryPlacementService txPlacement = failCtx.getBean(DiscoveryPlacementService.class);
            assertThrows(IllegalStateException.class,
                    () -> txPlacement.placePendingRequest(tenant.tenantId(), created.requestId()));
        }

        DiscoveryRequest stillPending = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.PENDING, stillPending.status());
        assertNull(stillPending.assignedSensorId());
        assertEquals(0L, assignmentVersion(tenant.tenantId(), created.requestId()));
    }

    private void raceFirstPlacement(
            CountDownLatch start,
            UUID tenantId,
            UUID requestId,
            String sensorId,
            AtomicInteger successes,
            AtomicInteger conflicts,
            AtomicReference<String> winnerSensor
    ) throws InterruptedException {
        start.await();
        try {
            discoveryService.assignSensorFirstPlacementFromControlPlane(tenantId, requestId, sensorId);
            successes.incrementAndGet();
            winnerSensor.set(sensorId);
        } catch (DomainException e) {
            if ("DISCOVERY_PLACEMENT_CONFLICT".equals(e.getCode())) {
                conflicts.incrementAndGet();
            } else {
                throw e;
            }
        }
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
                        // Under TX + FOR UPDATE, losers may observe CAS conflict or post-commit not-placeable.
                        if (SensorControlException.ERR_DISCOVERY_PLACEMENT_CONFLICT.equals(e.getCode())
                                || SensorControlException.ERR_DISCOVERY_REQUEST_NOT_PLACEABLE.equals(e.getCode())) {
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
