package com.platform.core.sensorcontrol;

import com.platform.core.domain.inventory.Site;
import com.platform.core.domain.inventory.SiteService;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.SiteRepository;
import com.platform.core.persistence.TenantRepository;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

@Tag("postgres")
class SensorEnrollmentAuthorityIntegrationTest {

    private PostgresTestSupport pg;
    private TenantService tenants;
    private SiteService sites;
    private SensorEnrollmentAuthorityService service;
    private SensorEnrollmentAuthorityRepository authorityRepo;
    private JdbcTemplate jdbc;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        NamedParameterJdbcTemplate named = pg.namedJdbc();
        jdbc = pg.jdbc();
        TenantRepository tenantRepo = new TenantRepository(named);
        SiteRepository siteRepo = new SiteRepository(named);
        tenants = new TenantService(tenantRepo);
        sites = new SiteService(siteRepo, tenants);
        SensorControlRepository sensors = new SensorControlRepository(named);
        authorityRepo = new SensorEnrollmentAuthorityRepository(named);
        service = new SensorEnrollmentAuthorityService(authorityRepo, sensors, new SecurityAuditEmitter());
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void issueConsumeReplayRevokeAndConcurrencyMatrix() throws Exception {
        Tenant tenant = tenants.create("Enrollment Auth Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Site site = sites.create(tenant.tenantId(), null, "HQ", "HQ", "UTC");
        Tenant other = tenants.create("Other Tenant", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Site otherSite = sites.create(other.tenantId(), null, "Remote", "Remote", "UTC");

        IssuedSensorEnrollmentAuthority issued = service.issue(
                tenant.tenantId(), "deploy-primary", site.siteId(), SensorRole.NETWORK, Duration.ofHours(1));
        assertTrue(issued.plaintextToken().length() >= 40);
        assertNotEquals(issued.plaintextToken(), issued.toString());

        Integer digestRows = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_enrollment_authority WHERE token_hash_sha256 = ?",
                Integer.class, SensorEnrollmentAuthorityService.sha256Hex(issued.plaintextToken()));
        assertEquals(1, digestRows);
        Integer plaintextHits = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_enrollment_authority WHERE token_hash_sha256 = ?",
                Integer.class, issued.plaintextToken());
        assertEquals(0, plaintextHits);

        ConsumedSensorEnrollmentAuthority consumed = service.consume(
                issued.plaintextToken(), SensorRole.NETWORK, site.siteId());
        assertEquals(issued.authorityId(), consumed.authorityId());
        assertEquals(tenant.tenantId(), consumed.tenantId());

        SensorControlException replay = assertThrows(SensorControlException.class,
                () -> service.consume(issued.plaintextToken(), SensorRole.NETWORK, site.siteId()));
        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID, replay.getCode());
        assertFalse(replay.getMessage().contains(issued.plaintextToken()));

        IssuedSensorEnrollmentAuthority scoped = service.issue(
                tenant.tenantId(), "deploy-primary", site.siteId(), SensorRole.NETWORK, Duration.ofMinutes(30));
        SensorControlException wrongRole = assertThrows(SensorControlException.class,
                () -> service.consume(scoped.plaintextToken(), SensorRole.INFRASTRUCTURE, site.siteId()));
        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID, wrongRole.getCode());
        // Scope mismatch must not consume — correct retry succeeds.
        ConsumedSensorEnrollmentAuthority retry = service.consume(
                scoped.plaintextToken(), SensorRole.NETWORK, site.siteId());
        assertEquals(scoped.authorityId(), retry.authorityId());

        IssuedSensorEnrollmentAuthority crossSite = service.issue(
                tenant.tenantId(), "deploy-primary", null, SensorRole.NETWORK, Duration.ofMinutes(10));
        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID,
                assertThrows(SensorControlException.class,
                        () -> service.consume(crossSite.plaintextToken(), SensorRole.NETWORK, otherSite.siteId()))
                        .getCode());

        IssuedSensorEnrollmentAuthority toRevoke = service.issue(
                tenant.tenantId(), "deploy-primary", null, null, Duration.ofMinutes(10));
        service.revoke(toRevoke.authorityId());
        service.revoke(toRevoke.authorityId()); // deterministic repeat
        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID,
                assertThrows(SensorControlException.class,
                        () -> service.consume(toRevoke.plaintextToken(), SensorRole.NETWORK, null)).getCode());

        IssuedSensorEnrollmentAuthority concurrent = service.issue(
                tenant.tenantId(), "deploy-race", null, SensorRole.NETWORK, Duration.ofMinutes(10));
        assertEquals(1, raceConsume(concurrent.plaintextToken(), SensorRole.NETWORK));

        // Constraint: invalid digest rejected
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_enrollment_authority
                    (authority_id, token_hash_sha256, tenant_id, deployment_id, site_id, role,
                     issued_at, expires_at, created_at, updated_at)
                VALUES
                    ('018f0e0a-0000-7000-8000-00000000dead'::uuid, 'NOT-A-HEX-DIGEST',
                     ?::uuid, 'd', NULL, NULL, NOW(), NOW() + interval '1 hour', NOW(), NOW())
                """, tenant.tenantId()));
    }

    @Test
    void expiredAuthorityFailsClosed() {
        Tenant tenant = tenants.create("Expiry Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        IssuedSensorEnrollmentAuthority issued = service.issue(
                tenant.tenantId(), "deploy-exp", null, SensorRole.NETWORK, Duration.ofHours(1));
        // Force expiry while preserving issued_at < expires_at integrity.
        jdbc.update("""
                UPDATE platform.sensor_enrollment_authority
                   SET issued_at = NOW() - interval '2 hours',
                       expires_at = NOW() - interval '1 second'
                 WHERE authority_id = ?
                """, issued.authorityId());
        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID,
                assertThrows(SensorControlException.class,
                        () -> service.consume(issued.plaintextToken(), SensorRole.NETWORK, null)).getCode());
    }

    @Test
    void tryInsertDuplicateReturnsFalseWithoutPoisoningTransaction() {
        Tenant tenant = tenants.create("Conflict Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Instant now = Instant.now();
        String digest = "a".repeat(64);
        SensorEnrollmentAuthority first = new SensorEnrollmentAuthority(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000c1"),
                digest, tenant.tenantId(), "deploy-conflict", null, SensorRole.NETWORK,
                now, now.plusSeconds(3600), null, null, now, now);
        assertTrue(authorityRepo.tryInsert(first));

        SensorEnrollmentAuthority duplicateDigest = new SensorEnrollmentAuthority(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000c2"),
                digest, tenant.tenantId(), "deploy-conflict", null, SensorRole.NETWORK,
                now, now.plusSeconds(3600), null, null, now, now);
        assertFalse(authorityRepo.tryInsert(duplicateDigest));

        // Same connection/transaction remains usable after conflict — subsequent insert succeeds.
        SensorEnrollmentAuthority next = new SensorEnrollmentAuthority(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000c3"),
                "b".repeat(64), tenant.tenantId(), "deploy-conflict", null, SensorRole.NETWORK,
                now, now.plusSeconds(3600), null, null, now, now);
        assertTrue(authorityRepo.tryInsert(next));
        assertEquals(2, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_enrollment_authority WHERE tenant_id = ?",
                Integer.class, tenant.tenantId()));
    }

    @Test
    void nullRequestedRoleFailsSafelyAgainstPersistedAuthority() {
        Tenant tenant = tenants.create("Null Role Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        IssuedSensorEnrollmentAuthority issued = service.issue(
                tenant.tenantId(), "deploy-null-role", null, SensorRole.NETWORK, Duration.ofMinutes(20));
        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> service.consume(issued.plaintextToken(), null, null));
        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID, ex.getCode());
        assertFalse(ex.getMessage().contains(issued.plaintextToken()));
        // Token must remain consumable after null-role rejection.
        ConsumedSensorEnrollmentAuthority consumed = service.consume(
                issued.plaintextToken(), SensorRole.NETWORK, null);
        assertEquals(issued.authorityId(), consumed.authorityId());
    }

    private int raceConsume(String token, SensorRole role) throws Exception {
        int threads = 8;
        ExecutorService pool = Executors.newFixedThreadPool(threads);
        CountDownLatch ready = new CountDownLatch(threads);
        CountDownLatch start = new CountDownLatch(1);
        AtomicInteger successes = new AtomicInteger();
        List<Future<?>> futures = new ArrayList<>();
        try {
            for (int i = 0; i < threads; i++) {
                futures.add(pool.submit(() -> {
                    ready.countDown();
                    start.await();
                    try {
                        service.consume(token, role, null);
                        successes.incrementAndGet();
                    } catch (SensorControlException ignored) {
                        // expected for losers
                    }
                    return null;
                }));
            }
            assertTrue(ready.await(5, TimeUnit.SECONDS));
            start.countDown();
            for (Future<?> f : futures) {
                f.get(20, TimeUnit.SECONDS);
            }
        } finally {
            pool.shutdownNow();
        }
        return successes.get();
    }
}
