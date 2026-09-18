package com.platform.core.domain.capabilitypack;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.CapabilityPackRepository;
import com.platform.core.persistence.TenantRepository;
import com.platform.core.security.authorization.AuthenticationState;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.http.PlatformAuthenticationToken;
import com.platform.core.security.identity.AuthenticatedExternalIdentity;
import com.platform.core.security.identity.AuthenticationAssurance;
import com.platform.core.security.identity.ExternalSubject;
import com.platform.core.security.identity.IdentityProviderType;
import com.platform.core.security.identity.PlatformSubject;
import com.platform.core.security.identity.ProviderInstanceId;
import com.platform.core.security.identity.ResolvedAuthenticatedIdentity;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.transaction.support.TransactionTemplate;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

@Tag("postgres")
class CapabilityPackCatalogIntegrationTest {

    private static final String BASE = "fixture.fictional.base";
    private static final String DEP = "fixture.fictional.dep";

    private PostgresTestSupport pg;
    private TransactionTemplate tx;
    private TenantService tenants;
    private CapabilityPackCatalogService catalog;
    private CapabilityPackRepository packRepo;
    private JdbcTemplate jdbc;
    private Tenant tenantA;
    private Tenant tenantB;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        NamedParameterJdbcTemplate named = pg.namedJdbc();
        jdbc = pg.jdbc();
        tx = pg.tx();
        TenantRepository tenantRepo = new TenantRepository(named);
        packRepo = new CapabilityPackRepository(named);
        tenants = new TenantService(tenantRepo);
        catalog = new CapabilityPackCatalogService(packRepo);
        tenantA = tenants.create("Pack Tenant A", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        tenantB = tenants.create("Pack Tenant B", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        authenticateAs("actor-a");
    }

    @AfterEach
    void tearDown() {
        SecurityContextHolder.clearContext();
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void registrationPersistsExactBytesAndEnforcesIdentityRules() {
        byte[] bytes = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "BaseProduct");
        CapabilityPackRegistrationResult created = catalog.register(tenantA.tenantId(), BASE, "1.0.0", bytes);
        assertTrue(created.created());
        assertArrayEquals(bytes, created.pack().manifestBytes());
        assertEquals(CapabilityPackTestSupport.sha256(bytes), created.pack().manifestSha256());

        CapabilityPackRegistrationResult again = catalog.register(tenantA.tenantId(), BASE, "1.0.0", bytes);
        assertFalse(again.created());
        assertEquals(created.pack().manifestSha256(), again.pack().manifestSha256());

        byte[] different = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "ChangedProduct");
        DomainException conflict = assertThrows(DomainException.class,
                () -> catalog.register(tenantA.tenantId(), BASE, "1.0.0", different));
        assertEquals(CapabilityPackCatalogService.ERR_IDENTITY_CONFLICT, conflict.getCode());

        byte[] v2 = CapabilityPackTestSupport.minimalManifestBytes(BASE, "2.0.0", "BaseProduct");
        assertTrue(catalog.register(tenantA.tenantId(), BASE, "2.0.0", v2).created());
        assertEquals(2, catalog.list(tenantA.tenantId(), 1, 50).total());

        DomainException mismatch = assertThrows(DomainException.class,
                () -> catalog.register(tenantA.tenantId(), "fixture.fictional.wrong", "1.0.0", bytes));
        assertEquals(CapabilityPackCatalogService.ERR_PATH_IDENTITY_MISMATCH, mismatch.getCode());
    }

    @Test
    void tenantIsolationAndCrossTenantActivationFkRejected() {
        byte[] bytes = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "BaseProduct");
        catalog.register(tenantA.tenantId(), BASE, "1.0.0", bytes);

        assertTrue(packRepo.findPack(tenantB.tenantId(), BASE, "1.0.0").isEmpty());
        DomainException missing = assertThrows(DomainException.class,
                () -> catalog.get(tenantB.tenantId(), BASE, "1.0.0"));
        assertEquals(CapabilityPackCatalogService.ERR_NOT_FOUND, missing.getCode());

        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.tenant_capability_pack_activation
                    (activation_id, tenant_id, pack_id, pack_version, generation, status,
                     activated_by, activated_at)
                VALUES
                    (?, ?, ?, ?, 1, 'ACTIVE', 'evil', NOW())
                """,
                UUID.randomUUID(), tenantB.tenantId(), BASE, "1.0.0"));
    }

    @Test
    void activationGenerationReplaceStaleAndIdempotent() {
        registerAndActivate(tenantA.tenantId(), BASE, "1.0.0", "BaseProduct");

        CapabilityPackActivation first = catalog.requireActive(tenantA.tenantId(), BASE);
        assertEquals(1L, first.generation());
        assertEquals("1.0.0", first.packVersion());

        CapabilityPackActivation idempotent = tx.execute(status ->
                catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 1L));
        assertEquals(1L, idempotent.generation());

        byte[] v2 = CapabilityPackTestSupport.minimalManifestBytes(BASE, "2.0.0", "BaseProduct");
        catalog.register(tenantA.tenantId(), BASE, "2.0.0", v2);
        CapabilityPackActivation replaced = tx.execute(status ->
                catalog.activate(tenantA.tenantId(), BASE, "2.0.0", 1L));
        assertEquals(2L, replaced.generation());
        assertEquals("2.0.0", replaced.packVersion());
        assertEquals("REVOKED", jdbc.queryForObject("""
                SELECT status FROM platform.tenant_capability_pack_activation
                 WHERE tenant_id=? AND pack_id=? AND generation=1
                """, String.class, tenantA.tenantId(), BASE));

        DomainException stale = assertThrows(DomainException.class, () ->
                tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 1L)));
        assertEquals(CapabilityPackCatalogService.ERR_GENERATION_CONFLICT, stale.getCode());
        assertEquals("2.0.0", catalog.requireActive(tenantA.tenantId(), BASE).packVersion());
    }

    @Test
    void activationRejectsMissingDependencyWrongVersionCycleAndAmbiguity() {
        byte[] base = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "BaseProduct");
        catalog.register(tenantA.tenantId(), BASE, "1.0.0", base);
        byte[] depMissing = CapabilityPackTestSupport.dependentManifestBytes(
                DEP, "1.0.0", "DepProduct", BASE, "1.0.0");
        catalog.register(tenantA.tenantId(), DEP, "1.0.0", depMissing);

        DomainException missingDep = assertThrows(DomainException.class, () ->
                tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), DEP, "1.0.0", 0L)));
        assertEquals(CapabilityPackCatalogService.ERR_SNAPSHOT_INVALID, missingDep.getCode());
        assertTrue(catalog.findActive(tenantA.tenantId(), DEP).isEmpty());

        tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 0L));
        tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), DEP, "1.0.0", 0L));

        byte[] base2 = CapabilityPackTestSupport.minimalManifestBytes(BASE, "2.0.0", "BaseProduct");
        catalog.register(tenantA.tenantId(), BASE, "2.0.0", base2);
        DomainException versionMismatch = assertThrows(DomainException.class, () ->
                tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), BASE, "2.0.0", 1L)));
        assertEquals(CapabilityPackCatalogService.ERR_SNAPSHOT_INVALID, versionMismatch.getCode());
        assertEquals("1.0.0", catalog.requireActive(tenantA.tenantId(), BASE).packVersion());

        // Self-dependency is rejected by snapshot validation (cycle family).
        Tenant cycleTenant = tenants.create("Cycle Tenant", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        String self = "fixture.fictional.selfdep";
        catalog.register(cycleTenant.tenantId(), self, "1.0.0",
                CapabilityPackTestSupport.dependentManifestBytes(self, "1.0.0", "Self", self, "1.0.0"));
        DomainException cycle = assertThrows(DomainException.class, () ->
                tx.executeWithoutResult(status -> catalog.activate(cycleTenant.tenantId(), self, "1.0.0", 0L)));
        assertEquals(CapabilityPackCatalogService.ERR_SNAPSHOT_INVALID, cycle.getCode());
        assertTrue(catalog.findActive(cycleTenant.tenantId(), self).isEmpty());

        // Equivalent cross-pack match definition.
        Tenant ambTenant = tenants.create("Amb Tenant", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        String p1 = "fixture.fictional.amb1";
        String p2 = "fixture.fictional.amb2";
        byte[] m1 = CapabilityPackTestSupport.minimalManifestBytes(p1, "1.0.0", "SameProduct");
        byte[] m2 = CapabilityPackTestSupport.minimalManifestBytes(p2, "1.0.0", "SameProduct");
        catalog.register(ambTenant.tenantId(), p1, "1.0.0", m1);
        catalog.register(ambTenant.tenantId(), p2, "1.0.0", m2);
        tx.executeWithoutResult(status -> catalog.activate(ambTenant.tenantId(), p1, "1.0.0", 0L));
        DomainException ambiguous = assertThrows(DomainException.class, () ->
                tx.executeWithoutResult(status -> catalog.activate(ambTenant.tenantId(), p2, "1.0.0", 0L)));
        assertEquals(CapabilityPackCatalogService.ERR_SNAPSHOT_INVALID, ambiguous.getCode());
    }

    @Test
    void deactivationRejectsRequiredDependencyThenSucceedsWhenSafe() {
        byte[] base = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "BaseProduct");
        byte[] dep = CapabilityPackTestSupport.dependentManifestBytes(DEP, "1.0.0", "DepProduct", BASE, "1.0.0");
        catalog.register(tenantA.tenantId(), BASE, "1.0.0", base);
        catalog.register(tenantA.tenantId(), DEP, "1.0.0", dep);
        tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 0L));
        tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), DEP, "1.0.0", 0L));

        DomainException inUse = assertThrows(DomainException.class, () ->
                tx.executeWithoutResult(status -> catalog.deactivate(tenantA.tenantId(), BASE, 1L)));
        assertEquals(CapabilityPackCatalogService.ERR_DEPENDENCY_IN_USE, inUse.getCode());
        assertEquals("1.0.0", catalog.requireActive(tenantA.tenantId(), BASE).packVersion());

        tx.executeWithoutResult(status -> catalog.deactivate(tenantA.tenantId(), DEP, 1L));
        assertTrue(catalog.findActive(tenantA.tenantId(), DEP).isEmpty());
        tx.executeWithoutResult(status -> catalog.deactivate(tenantA.tenantId(), BASE, 1L));
        assertTrue(catalog.findActive(tenantA.tenantId(), BASE).isEmpty());
    }

    @Test
    void concurrentActivationsSerializeForSameTenant() throws Exception {
        byte[] v1 = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "BaseProduct");
        byte[] v2 = CapabilityPackTestSupport.minimalManifestBytes(BASE, "2.0.0", "BaseProduct");
        catalog.register(tenantA.tenantId(), BASE, "1.0.0", v1);
        catalog.register(tenantA.tenantId(), BASE, "2.0.0", v2);

        int threads = 8;
        ExecutorService pool = Executors.newFixedThreadPool(threads);
        CountDownLatch ready = new CountDownLatch(threads);
        CountDownLatch start = new CountDownLatch(1);
        AtomicInteger successes = new AtomicInteger();
        AtomicInteger generationConflicts = new AtomicInteger();
        List<Future<?>> futures = new ArrayList<>();
        for (int i = 0; i < threads; i++) {
            final String version = (i % 2 == 0) ? "1.0.0" : "2.0.0";
            futures.add(pool.submit(() -> {
                authenticateAs("actor-" + Thread.currentThread().threadId());
                ready.countDown();
                start.await();
                try {
                    tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), BASE, version, 0L));
                    successes.incrementAndGet();
                } catch (DomainException ex) {
                    if (CapabilityPackCatalogService.ERR_GENERATION_CONFLICT.equals(ex.getCode())) {
                        generationConflicts.incrementAndGet();
                    } else {
                        throw ex;
                    }
                }
                return null;
            }));
        }
        assertTrue(ready.await(10, TimeUnit.SECONDS));
        start.countDown();
        for (Future<?> future : futures) {
            future.get(30, TimeUnit.SECONDS);
        }
        pool.shutdown();
        assertTrue(pool.awaitTermination(10, TimeUnit.SECONDS));

        assertEquals(1, successes.get());
        assertEquals(threads - 1, generationConflicts.get());
        CapabilityPackActivation active = catalog.requireActive(tenantA.tenantId(), BASE);
        assertEquals(1L, active.generation());
        Integer activeRows = jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.tenant_capability_pack_activation
                 WHERE tenant_id=? AND pack_id=? AND status='ACTIVE'
                """, Integer.class, tenantA.tenantId(), BASE);
        assertEquals(1, activeRows);
    }

    @Test
    void differentTenantsMutateIndependently() {
        byte[] bytes = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "BaseProduct");
        catalog.register(tenantA.tenantId(), BASE, "1.0.0", bytes);
        catalog.register(tenantB.tenantId(), BASE, "1.0.0", bytes);
        tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 0L));
        tx.executeWithoutResult(status -> catalog.activate(tenantB.tenantId(), BASE, "1.0.0", 0L));
        assertEquals(1L, catalog.requireActive(tenantA.tenantId(), BASE).generation());
        assertEquals(1L, catalog.requireActive(tenantB.tenantId(), BASE).generation());
    }

    @Test
    void reactivationAfterDeactivateUsesMonotonicHistoricalGeneration() {
        byte[] bytes = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "BaseProduct");
        catalog.register(tenantA.tenantId(), BASE, "1.0.0", bytes);

        CapabilityPackActivation gen1 = tx.execute(status ->
                catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 0L));
        assertEquals(1L, gen1.generation());

        tx.executeWithoutResult(status -> catalog.deactivate(tenantA.tenantId(), BASE, 1L));
        assertTrue(catalog.findActive(tenantA.tenantId(), BASE).isEmpty());

        CapabilityPackActivation gen2 = tx.execute(status ->
                catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 0L));
        assertEquals(2L, gen2.generation());
        assertEquals(CapabilityPackActivation.STATUS_ACTIVE, gen2.status());

        List<MapRow> history = jdbc.query("""
                SELECT generation, status FROM platform.tenant_capability_pack_activation
                 WHERE tenant_id=? AND pack_id=?
                 ORDER BY generation ASC
                """,
                (rs, rowNum) -> new MapRow(rs.getLong("generation"), rs.getString("status")),
                tenantA.tenantId(), BASE);
        assertEquals(2, history.size());
        assertEquals(1L, history.get(0).generation());
        assertEquals("REVOKED", history.get(0).status());
        assertEquals(2L, history.get(1).generation());
        assertEquals("ACTIVE", history.get(1).status());
    }

    @Test
    void multiLifecycleActivationsProduceMonotonicGenerations() {
        byte[] bytes = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "BaseProduct");
        catalog.register(tenantA.tenantId(), BASE, "1.0.0", bytes);

        tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 0L));
        tx.executeWithoutResult(status -> catalog.deactivate(tenantA.tenantId(), BASE, 1L));
        tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 0L));
        tx.executeWithoutResult(status -> catalog.deactivate(tenantA.tenantId(), BASE, 2L));
        CapabilityPackActivation third = tx.execute(status ->
                catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 0L));
        assertEquals(3L, third.generation());

        List<Long> generations = jdbc.queryForList("""
                SELECT generation FROM platform.tenant_capability_pack_activation
                 WHERE tenant_id=? AND pack_id=?
                 ORDER BY generation ASC
                """, Long.class, tenantA.tenantId(), BASE);
        assertEquals(List.of(1L, 2L, 3L), generations);

        Integer activeCount = jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.tenant_capability_pack_activation
                 WHERE tenant_id=? AND pack_id=? AND status='ACTIVE'
                """, Integer.class, tenantA.tenantId(), BASE);
        assertEquals(1, activeCount);
        assertEquals(3L, catalog.requireActive(tenantA.tenantId(), BASE).generation());
    }

    @Test
    void staleExpectedGenerationAfterHistoryDoesNotMutate() {
        byte[] bytes = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "BaseProduct");
        catalog.register(tenantA.tenantId(), BASE, "1.0.0", bytes);
        tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 0L));
        tx.executeWithoutResult(status -> catalog.deactivate(tenantA.tenantId(), BASE, 1L));
        CapabilityPackActivation gen2 = tx.execute(status ->
                catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 0L));
        assertEquals(2L, gen2.generation());

        Integer before = jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.tenant_capability_pack_activation
                 WHERE tenant_id=? AND pack_id=?
                """, Integer.class, tenantA.tenantId(), BASE);

        DomainException stale = assertThrows(DomainException.class, () ->
                tx.executeWithoutResult(status -> catalog.activate(tenantA.tenantId(), BASE, "1.0.0", 1L)));
        assertEquals(CapabilityPackCatalogService.ERR_GENERATION_CONFLICT, stale.getCode());

        Integer after = jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.tenant_capability_pack_activation
                 WHERE tenant_id=? AND pack_id=?
                """, Integer.class, tenantA.tenantId(), BASE);
        assertEquals(before, after);
        assertEquals(2L, catalog.requireActive(tenantA.tenantId(), BASE).generation());
        assertEquals("ACTIVE", jdbc.queryForObject("""
                SELECT status FROM platform.tenant_capability_pack_activation
                 WHERE tenant_id=? AND pack_id=? AND generation=2
                """, String.class, tenantA.tenantId(), BASE));
    }

    @Test
    void concurrentSameIdentitySameBytesRegistersIdempotently() throws Exception {
        byte[] bytes = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "BaseProduct");
        int threads = 8;
        ExecutorService pool = Executors.newFixedThreadPool(threads);
        CountDownLatch ready = new CountDownLatch(threads);
        CountDownLatch start = new CountDownLatch(1);
        AtomicInteger created = new AtomicInteger();
        AtomicInteger idempotent = new AtomicInteger();
        List<Future<?>> futures = new ArrayList<>();
        for (int i = 0; i < threads; i++) {
            final int idx = i;
            futures.add(pool.submit(() -> {
                authenticateAs("reg-actor-" + idx);
                ready.countDown();
                start.await();
                CapabilityPackRegistrationResult result =
                        catalog.register(tenantA.tenantId(), BASE, "1.0.0", bytes);
                if (result.created()) {
                    created.incrementAndGet();
                } else {
                    idempotent.incrementAndGet();
                }
                return null;
            }));
        }
        assertTrue(ready.await(10, TimeUnit.SECONDS));
        start.countDown();
        for (Future<?> future : futures) {
            future.get(30, TimeUnit.SECONDS);
        }
        pool.shutdown();
        assertTrue(pool.awaitTermination(10, TimeUnit.SECONDS));

        assertEquals(1, created.get());
        assertEquals(threads - 1, idempotent.get());
        Integer artifactRows = jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.tenant_capability_pack
                 WHERE tenant_id=? AND pack_id=? AND pack_version=?
                """, Integer.class, tenantA.tenantId(), BASE, "1.0.0");
        assertEquals(1, artifactRows);
        assertArrayEquals(bytes, catalog.get(tenantA.tenantId(), BASE, "1.0.0").manifestBytes());
    }

    @Test
    void concurrentSameIdentityDifferentBytesOneWinner() throws Exception {
        byte[] bytesA = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "WinnerProduct");
        byte[] bytesB = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "LoserProduct");
        int threads = 8;
        ExecutorService pool = Executors.newFixedThreadPool(threads);
        CountDownLatch ready = new CountDownLatch(threads);
        CountDownLatch start = new CountDownLatch(1);
        AtomicInteger created = new AtomicInteger();
        AtomicInteger conflicts = new AtomicInteger();
        AtomicInteger sameShaIdempotent = new AtomicInteger();
        List<Future<?>> futures = new ArrayList<>();
        for (int i = 0; i < threads; i++) {
            final int idx = i;
            final byte[] payload = (i % 2 == 0) ? bytesA : bytesB;
            futures.add(pool.submit(() -> {
                authenticateAs("conflict-actor-" + idx);
                ready.countDown();
                start.await();
                try {
                    CapabilityPackRegistrationResult result =
                            catalog.register(tenantA.tenantId(), BASE, "1.0.0", payload);
                    if (result.created()) {
                        created.incrementAndGet();
                    } else {
                        sameShaIdempotent.incrementAndGet();
                    }
                } catch (DomainException ex) {
                    if (CapabilityPackCatalogService.ERR_IDENTITY_CONFLICT.equals(ex.getCode())) {
                        conflicts.incrementAndGet();
                    } else {
                        throw ex;
                    }
                }
                return null;
            }));
        }
        assertTrue(ready.await(10, TimeUnit.SECONDS));
        start.countDown();
        for (Future<?> future : futures) {
            future.get(30, TimeUnit.SECONDS);
        }
        pool.shutdown();
        assertTrue(pool.awaitTermination(10, TimeUnit.SECONDS));

        assertEquals(1, created.get());
        assertTrue(conflicts.get() >= 1, "at least one different-bytes loser must conflict");
        assertEquals(threads, created.get() + conflicts.get() + sameShaIdempotent.get());
        byte[] stored = catalog.get(tenantA.tenantId(), BASE, "1.0.0").manifestBytes();
        assertTrue(
                java.util.Arrays.equals(stored, bytesA) || java.util.Arrays.equals(stored, bytesB),
                "stored bytes must equal the single winner payload");
        Integer artifactRows = jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.tenant_capability_pack
                 WHERE tenant_id=? AND pack_id=? AND pack_version=?
                """, Integer.class, tenantA.tenantId(), BASE, "1.0.0");
        assertEquals(1, artifactRows);
    }

    @Test
    void concurrentRegistrationDifferentTenantsIndependent() throws Exception {
        byte[] bytes = CapabilityPackTestSupport.minimalManifestBytes(BASE, "1.0.0", "BaseProduct");
        int threads = 4;
        ExecutorService pool = Executors.newFixedThreadPool(threads);
        CountDownLatch ready = new CountDownLatch(threads);
        CountDownLatch start = new CountDownLatch(1);
        AtomicInteger createdA = new AtomicInteger();
        AtomicInteger createdB = new AtomicInteger();
        List<Future<?>> futures = new ArrayList<>();
        for (int i = 0; i < threads; i++) {
            final int idx = i;
            final UUID tenantId = (i % 2 == 0) ? tenantA.tenantId() : tenantB.tenantId();
            final AtomicInteger counter = (i % 2 == 0) ? createdA : createdB;
            futures.add(pool.submit(() -> {
                authenticateAs("tenant-actor-" + idx);
                ready.countDown();
                start.await();
                CapabilityPackRegistrationResult result =
                        catalog.register(tenantId, BASE, "1.0.0", bytes);
                if (result.created()) {
                    counter.incrementAndGet();
                }
                return null;
            }));
        }
        assertTrue(ready.await(10, TimeUnit.SECONDS));
        start.countDown();
        for (Future<?> future : futures) {
            future.get(30, TimeUnit.SECONDS);
        }
        pool.shutdown();
        assertTrue(pool.awaitTermination(10, TimeUnit.SECONDS));

        assertEquals(1, createdA.get());
        assertEquals(1, createdB.get());
        assertTrue(packRepo.findPack(tenantA.tenantId(), BASE, "1.0.0").isPresent());
        assertTrue(packRepo.findPack(tenantB.tenantId(), BASE, "1.0.0").isPresent());
    }

    private void registerAndActivate(UUID tenantId, String packId, String version, String product) {
        byte[] bytes = CapabilityPackTestSupport.minimalManifestBytes(packId, version, product);
        catalog.register(tenantId, packId, version, bytes);
        tx.executeWithoutResult(status -> catalog.activate(tenantId, packId, version, 0L));
    }

    private static void authenticateAs(String subjectId) {
        Instant now = Instant.parse("2026-06-01T00:00:00Z");
        PrincipalContext principal = new PrincipalContext(
                subjectId,
                AuthenticationState.AUTHENTICATED,
                Set.of(),
                Set.of()
        );
        UUID subjectUuid = UUID.fromString("018f0e0a-0000-7000-8000-0000000000aa");
        SecurityContextHolder.getContext().setAuthentication(
                new PlatformAuthenticationToken(
                        ResolvedAuthenticatedIdentity.current(
                                new PlatformSubject(subjectUuid),
                                new AuthenticatedExternalIdentity(
                                        IdentityProviderType.OIDC,
                                        new ProviderInstanceId("platform-oidc"),
                                        new ExternalSubject(subjectId),
                                        now,
                                        AuthenticationAssurance.withoutVerifiedMfa(),
                                        Instant.parse("2099-01-01T00:00:00Z")
                                ),
                                now
                        ),
                        principal
                )
        );
    }

    private record MapRow(long generation, String status) {
    }
}
