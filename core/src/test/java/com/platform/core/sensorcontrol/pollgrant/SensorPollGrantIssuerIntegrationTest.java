package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.persistence.EntityManagementEndpointRepository;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.sensorcontrol.SensorControlException;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.sensorcontrol.SensorPrincipal;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.core.read.ListAppender;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.CyclicBarrier;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;
import java.util.concurrent.locks.LockSupport;

import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Protocol;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Status;
import static org.junit.jupiter.api.Assertions.*;

/**
 * PostgreSQL evidence for Issue #229 G1a issuance. Does not wire REST, scheduler or admission.
 */
@Tag("postgres")
class SensorPollGrantIssuerIntegrationTest {

    private static final Instant ISSUED = Instant.parse("2026-10-28T12:00:00.123456Z");
    private static final UUID FIXED_GRANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000401");

    private PostgresTestSupport pg;
    private SensorControlRepository sensors;
    private EntityManagementEndpointRepository endpoints;
    private SensorPollGrantRepository grants;
    private SecurityAuditEmitter audit;
    private SensorPollGrantIssuer issuer;
    private ListAppender<ILoggingEvent> auditLog;

    private final UUID tenant = UUID.randomUUID();
    private final UUID otherTenant = UUID.randomUUID();
    private final UUID entity = UUID.randomUUID();
    private final UUID otherEntity = UUID.randomUUID();
    private final UUID endpoint = UUID.randomUUID();
    private final UUID otherEndpoint = UUID.randomUUID();
    private final String sensor = "issuer-" + UUID.randomUUID();
    private final String allowlistSensor = "issuer-al-" + UUID.randomUUID();
    private final String otherTenantSensor = "issuer-ot-" + UUID.randomUUID();
    private boolean fixtureOwned;

    @BeforeEach
    void start() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        sensors = new SensorControlRepository(pg.namedJdbc());
        endpoints = new EntityManagementEndpointRepository(pg.namedJdbc());
        grants = new SensorPollGrantRepository(pg.namedJdbc());
        audit = new SecurityAuditEmitter();
        issuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(ISSUED, ZoneOffset.UTC),
                () -> FIXED_GRANT);

        Logger logger = (Logger) LoggerFactory.getLogger(SecurityAuditEmitter.class);
        auditLog = new ListAppender<>();
        auditLog.start();
        logger.addAppender(auditLog);

        pg.tx().executeWithoutResult(tx -> {
            for (UUID id : List.of(tenant, otherTenant)) {
                pg.jdbc().update("""
                        INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                        VALUES (?, ?, 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                        """, id, "issuer-fixture-" + id);
            }
            pg.jdbc().update("""
                    INSERT INTO platform.sensor (sensor_id, tenant_id, role, status,
                        allow_all_tenant_entities, created_at, updated_at)
                    VALUES (?, ?, 'NETWORK', 'ACTIVE', TRUE, NOW(), NOW())
                    """, sensor, tenant);
            pg.jdbc().update("""
                    INSERT INTO platform.sensor (sensor_id, tenant_id, role, status,
                        allow_all_tenant_entities, created_at, updated_at)
                    VALUES (?, ?, 'NETWORK', 'ACTIVE', FALSE, NOW(), NOW())
                    """, allowlistSensor, tenant);
            pg.jdbc().update("""
                    INSERT INTO platform.sensor (sensor_id, tenant_id, role, status,
                        allow_all_tenant_entities, created_at, updated_at)
                    VALUES (?, ?, 'NETWORK', 'ACTIVE', TRUE, NOW(), NOW())
                    """, otherTenantSensor, otherTenant);
            for (UUID id : List.of(entity, otherEntity)) {
                pg.jdbc().update("""
                        INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name,
                            lifecycle_state, first_seen_at, last_seen_at, created_by, created_at, updated_at)
                        VALUES (?, ?, 'NETWORK_DEVICE', 'issuer-fixture', 'ACTIVE', NOW(), NOW(), 'fixture', NOW(), NOW())
                        """, id, tenant);
            }
            pg.jdbc().update("""
                    INSERT INTO platform.sensor_allowed_entity (tenant_id, sensor_id, entity_id, created_at)
                    VALUES (?, ?, ?, NOW())
                    """, tenant, allowlistSensor, entity);
            pg.jdbc().update("""
                    INSERT INTO platform.entity_management_endpoint (endpoint_id, tenant_id, entity_id,
                        address_type, address, state, source, is_primary, created_by, created_at, updated_by, updated_at)
                    VALUES (?, ?, ?, 'IPV6', '2001:db8::aa', 'ACTIVE', 'MANUAL', TRUE, 'fixture', NOW(), 'fixture', NOW())
                    """, endpoint, tenant, entity);
            pg.jdbc().update("""
                    INSERT INTO platform.entity_management_endpoint (endpoint_id, tenant_id, entity_id,
                        address_type, address, state, source, is_primary, created_by, created_at, updated_by, updated_at)
                    VALUES (?, ?, ?, 'IPV4', '192.0.2.99', 'INACTIVE', 'MANUAL', FALSE, 'fixture', NOW(), 'fixture', NOW())
                    """, otherEndpoint, tenant, otherEntity);
        });
        fixtureOwned = true;
    }

    @AfterEach
    void stop() {
        try {
            Logger logger = (Logger) LoggerFactory.getLogger(SecurityAuditEmitter.class);
            if (auditLog != null) {
                logger.detachAppender(auditLog);
            }
            if (fixtureOwned) {
                pg.tx().executeWithoutResult(tx -> {
                    pg.jdbc().update("DELETE FROM platform.sensor_poll_grant WHERE tenant_id IN (?, ?)",
                            tenant, otherTenant);
                    pg.jdbc().update("DELETE FROM platform.sensor_allowed_entity WHERE tenant_id IN (?, ?)",
                            tenant, otherTenant);
                    pg.jdbc().update("DELETE FROM platform.entity_management_endpoint WHERE tenant_id IN (?, ?)",
                            tenant, otherTenant);
                    pg.jdbc().update("DELETE FROM platform.entity WHERE tenant_id IN (?, ?)",
                            tenant, otherTenant);
                    pg.jdbc().update("DELETE FROM platform.sensor WHERE tenant_id IN (?, ?)",
                            tenant, otherTenant);
                    for (UUID id : List.of(tenant, otherTenant)) {
                        pg.jdbc().update("DELETE FROM platform.tenant WHERE tenant_id = ?", id);
                    }
                });
            }
        } finally {
            if (pg != null) {
                pg.stop();
            }
        }
    }

    @Test
    void allowAllSuccessfulIssuanceReturnsExactDatabaseReadback() {
        SensorPollGrant row = pg.tx().execute(status -> issuer.issue(request(sensor, entity, endpoint, Protocol.SNMP)));
        assertNotNull(row);
        assertEquals(FIXED_GRANT, row.grantId());
        assertEquals(Status.ISSUED, row.status());
        assertEquals("2001:db8::aa", row.issuedAddress());
        assertEquals(ISSUED, row.issuedAt());
        assertEquals(ISSUED, row.updatedAt());
        SensorPollGrant reloaded = grants.find(tenant, sensor, FIXED_GRANT).orElseThrow();
        assertEquals(row, reloaded);
        assertTrue(auditLog.list.stream().anyMatch(event ->
                event.getFormattedMessage().contains("sensor.poll_grant.issue")
                        && event.getFormattedMessage().contains("SUCCESS")
                        && !event.getFormattedMessage().contains("2001:db8")
                        && !event.getFormattedMessage().contains("issued_by")
                        && !event.getFormattedMessage().contains("profile_plan")));
    }

    @Test
    void allowlistSuccessfulIssuance() {
        SensorPollGrantIssuer allowlistIssuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(ISSUED, ZoneOffset.UTC),
                UUID::randomUUID);
        SensorPollGrant row = pg.tx().execute(status ->
                allowlistIssuer.issue(request(allowlistSensor, entity, endpoint, Protocol.ICMP)));
        assertEquals(allowlistSensor, row.sensorId());
        assertEquals(Protocol.ICMP, row.protocol());
        assertEquals(Status.ISSUED, row.status());
        assertEquals(row, grants.find(tenant, allowlistSensor, row.grantId()).orElseThrow());
    }

    @Test
    void inactiveMissingAndWrongTenantSensorReject() {
        pg.tx().executeWithoutResult(tx ->
                pg.jdbc().update("UPDATE platform.sensor SET status = 'DISABLED' WHERE sensor_id = ?", sensor));
        SensorControlException inactive = assertThrows(SensorControlException.class,
                () -> pg.tx().execute(status -> issuer.issue(request(sensor, entity, endpoint, Protocol.SNMP))));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, inactive.getCode());

        SensorControlException missing = assertThrows(SensorControlException.class,
                () -> pg.tx().execute(status -> issuer.issue(
                        new SensorPollGrantIssuer.IssueRequest(
                                tenant, "missing-" + UUID.randomUUID(), entity, endpoint, Protocol.SNMP,
                                1L, 0L, "planned-profile", "v1",
                                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), "internal-actor"))));
        assertEquals(SensorControlException.ERR_SENSOR_NOT_FOUND, missing.getCode());

        SensorControlException wrongTenant = assertThrows(SensorControlException.class,
                () -> pg.tx().execute(status -> issuer.issue(
                        new SensorPollGrantIssuer.IssueRequest(
                                otherTenant, sensor, entity, endpoint, Protocol.SNMP,
                                1L, 0L, "planned-profile", "v1",
                                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), "internal-actor"))));
        assertEquals(SensorControlException.ERR_SENSOR_NOT_FOUND, wrongTenant.getCode());
        assertEquals(0, countGrants());
        assertTrue(auditLog.list.stream().noneMatch(event ->
                event.getFormattedMessage().contains("sensor.poll_grant.issue")));
    }

    @Test
    void entityOutsideScopeRejects() {
        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> pg.tx().execute(status -> issuer.issue(
                        request(allowlistSensor, otherEntity, otherEndpoint, Protocol.SNMP))));
        assertEquals(SensorControlException.ERR_POLL_GRANT_ENTITY_OUT_OF_SCOPE, failure.getCode());
        assertEquals(0, countGrants());
    }

    @Test
    void missingWrongEntityWrongTenantAndInactiveEndpointReject() {
        assertEquals(SensorControlException.ERR_POLL_GRANT_ENDPOINT_UNAVAILABLE,
                assertThrows(SensorControlException.class, () -> pg.tx().execute(status ->
                        issuer.issue(request(sensor, entity, UUID.randomUUID(), Protocol.SNMP)))).getCode());
        assertEquals(SensorControlException.ERR_POLL_GRANT_ENDPOINT_UNAVAILABLE,
                assertThrows(SensorControlException.class, () -> pg.tx().execute(status ->
                        issuer.issue(request(sensor, otherEntity, endpoint, Protocol.SNMP)))).getCode());
        assertEquals(SensorControlException.ERR_POLL_GRANT_ENDPOINT_UNAVAILABLE,
                assertThrows(SensorControlException.class, () -> pg.tx().execute(status ->
                        issuer.issue(new SensorPollGrantIssuer.IssueRequest(
                                otherTenant, otherTenantSensor, entity, endpoint, Protocol.SNMP,
                                1L, 0L, "planned-profile", "v1",
                                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), "internal-actor")))).getCode());
        assertEquals(SensorControlException.ERR_POLL_GRANT_ENDPOINT_UNAVAILABLE,
                assertThrows(SensorControlException.class, () -> pg.tx().execute(status ->
                        issuer.issue(request(sensor, otherEntity, otherEndpoint, Protocol.SNMP)))).getCode());
        assertEquals(0, countGrants());
    }

    @Test
    void typedInvalidActorRejectsWithoutMutation() {
        // List.of rejects null; keep the null actor case in a nullable-safe fixture.
        String[] actors = {null, "", " ", "\t", "\n", " \t\n "};
        for (String actor : actors) {
            SensorControlException failure = assertThrows(SensorControlException.class,
                    () -> pg.tx().execute(status -> issuer.issue(
                            new SensorPollGrantIssuer.IssueRequest(
                                    tenant, sensor, entity, endpoint, Protocol.SNMP,
                                    1L, 0L, "planned-profile", "v1",
                                    ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), actor))));
            assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID, failure.getCode());
        }
        assertEquals(0, countGrants());
        assertTrue(auditLog.list.stream().noneMatch(event ->
                event.getFormattedMessage().contains("sensor.poll_grant.issue")));
    }

    @Test
    void windowExpiryWhileWaitingForPrincipalLockRejects() throws Exception {
        clearGrants();
        AtomicReference<Instant> now = new AtomicReference<>(ISSUED);
        Clock mutable = new Clock() {
            @Override
            public Instant instant() {
                return now.get();
            }

            @Override
            public ZoneOffset getZone() {
                return ZoneOffset.UTC;
            }

            @Override
            public Clock withZone(java.time.ZoneId zone) {
                return this;
            }
        };
        SensorPollGrantIssuer gatedIssuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit, mutable, () -> FIXED_GRANT);

        CountDownLatch mutationHeld = new CountDownLatch(1);
        CountDownLatch releaseMutation = new CountDownLatch(1);
        CountDownLatch aboutToLock = new CountDownLatch(1);
        AtomicReference<Integer> holderPid = new AtomicReference<>();
        AtomicReference<Integer> contenderPid = new AtomicReference<>();
        AtomicReference<Throwable> issuerFailure = new AtomicReference<>();
        ExecutorService pool = Executors.newFixedThreadPool(2);
        Future<?> mutator = null;
        Future<?> issuance = null;
        try {
            mutator = pool.submit(() -> {
                pg.tx().executeWithoutResult(status -> {
                    sensors.lockPrincipalForUpdate(tenant, sensor);
                    holderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                    mutationHeld.countDown();
                    await(releaseMutation);
                });
                return null;
            });
            assertTrue(mutationHeld.await(30, TimeUnit.SECONDS));
            assertNotNull(holderPid.get());

            issuance = pool.submit(() -> {
                try {
                    pg.tx().execute(status -> {
                        contenderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                        aboutToLock.countDown();
                        return gatedIssuer.issue(new SensorPollGrantIssuer.IssueRequest(
                                tenant, sensor, entity, endpoint, Protocol.SNMP,
                                1L, 0L, "planned-profile", "v1",
                                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), "internal-actor"));
                    });
                } catch (Throwable t) {
                    issuerFailure.set(t);
                }
                return null;
            });

            assertTrue(aboutToLock.await(30, TimeUnit.SECONDS),
                    "contender must signal before invoking protected principal lock");
            assertNotNull(contenderPid.get());
            assertNotEquals(holderPid.get(), contenderPid.get(),
                    "holder and contender must be independent PostgreSQL backends");
            awaitDatabaseLockWait(contenderPid.get());
            now.set(ISSUED.plusSeconds(120));
            releaseMutation.countDown();
            mutator.get(30, TimeUnit.SECONDS);
            mutator = null;
            issuance.get(30, TimeUnit.SECONDS);
            issuance = null;
        } finally {
            releaseMutation.countDown();
            shutdownPool(pool, mutator, issuance);
        }

        Throwable failure = issuerFailure.get();
        assertNotNull(failure);
        SensorControlException typed = unwrap(failure);
        assertEquals(SensorControlException.ERR_POLL_GRANT_WINDOW_INVALID, typed.getCode());
        assertEquals(0, countGrants());
        assertTrue(auditLog.list.stream().noneMatch(event ->
                event.getFormattedMessage().contains("sensor.poll_grant.issue")));
    }

    @Test
    void principalShareLockRetainedThroughCommitAndRollback() throws Exception {
        clearGrants();
        assertPrincipalShareHeldUntilCompletion(true);
        assertEquals(1, countGrants());
        pg.tx().executeWithoutResult(tx ->
                pg.jdbc().update("DELETE FROM platform.sensor_poll_grant WHERE tenant_id = ?", tenant));
        assertPrincipalShareHeldUntilCompletion(false);
        assertEquals(0, countGrants());
    }

    @Test
    void endpointShareLockRetainedThroughCommitAndRollback() throws Exception {
        clearGrants();
        assertEndpointShareHeldUntilCompletion(true);
        assertEquals(1, countGrants());
        pg.tx().executeWithoutResult(tx ->
                pg.jdbc().update("DELETE FROM platform.sensor_poll_grant WHERE tenant_id = ?", tenant));
        assertEndpointShareHeldUntilCompletion(false);
        assertEquals(0, countGrants());
    }

    @Test
    void mutationFirstBlocksIssuanceUntilReleaseThenIssuanceCompletes() throws Exception {
        clearGrants();
        CountDownLatch mutationHeld = new CountDownLatch(1);
        CountDownLatch releaseMutation = new CountDownLatch(1);
        CountDownLatch aboutToLock = new CountDownLatch(1);
        AtomicBoolean issuanceCompleted = new AtomicBoolean(false);
        AtomicReference<SensorPollGrant> issued = new AtomicReference<>();
        AtomicReference<Integer> holderPid = new AtomicReference<>();
        AtomicReference<Integer> contenderPid = new AtomicReference<>();
        SensorPollGrantIssuer gatedIssuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(ISSUED, ZoneOffset.UTC),
                () -> FIXED_GRANT);
        ExecutorService pool = Executors.newFixedThreadPool(2);
        Future<?> mutator = null;
        Future<?> issuance = null;
        try {
            mutator = pool.submit(() -> {
                pg.tx().executeWithoutResult(status -> {
                    sensors.lockPrincipalForUpdate(tenant, sensor);
                    holderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                    mutationHeld.countDown();
                    await(releaseMutation);
                });
                return null;
            });
            assertTrue(mutationHeld.await(30, TimeUnit.SECONDS));
            assertNotNull(holderPid.get());

            issuance = pool.submit(() -> {
                SensorPollGrant row = pg.tx().execute(status -> {
                    contenderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                    aboutToLock.countDown();
                    return gatedIssuer.issue(request(sensor, entity, endpoint, Protocol.SNMP));
                });
                issued.set(row);
                issuanceCompleted.set(true);
                return null;
            });

            assertTrue(aboutToLock.await(30, TimeUnit.SECONDS),
                    "issuance must signal before invoking protected principal lock");
            assertNotNull(contenderPid.get());
            assertNotEquals(holderPid.get(), contenderPid.get(),
                    "holder and contender must be independent PostgreSQL backends");
            awaitDatabaseLockWait(contenderPid.get());
            assertFalse(issuanceCompleted.get(),
                    "issuance must not complete before mutation releases FOR UPDATE");
            releaseMutation.countDown();
            mutator.get(30, TimeUnit.SECONDS);
            mutator = null;
            issuance.get(30, TimeUnit.SECONDS);
            issuance = null;
        } finally {
            releaseMutation.countDown();
            shutdownPool(pool, mutator, issuance);
        }
        assertTrue(issuanceCompleted.get());
        assertEquals(FIXED_GRANT, issued.get().grantId());
        assertEquals(1, countGrants());
    }

    @Test
    void issuanceFirstBlocksPrincipalMutationWithNowaitOracleUntilRelease() throws Exception {
        clearGrants();
        assertIssuanceBlocksPrincipalMutationUntilCompletion(true);
        pg.tx().executeWithoutResult(tx ->
                pg.jdbc().update("DELETE FROM platform.sensor_poll_grant WHERE tenant_id = ?", tenant));
        assertIssuanceBlocksPrincipalMutationUntilCompletion(false);
    }

    @Test
    void simultaneousValidIssuancesAreNotSerializedByShareLocks() throws Exception {
        clearGrants();
        CyclicBarrier bothHoldingShare = new CyclicBarrier(2);
        AtomicInteger shareHolders = new AtomicInteger();
        SensorControlRepository shareAware = new SensorControlRepository(pg.namedJdbc()) {
            @Override
            public SensorPrincipal lockPrincipalForShare(UUID tenantId, String sensorId) {
                SensorPrincipal principal = super.lockPrincipalForShare(tenantId, sensorId);
                shareHolders.incrementAndGet();
                try {
                    bothHoldingShare.await(30, TimeUnit.SECONDS);
                } catch (Exception e) {
                    throw new IllegalStateException(e);
                }
                return principal;
            }
        };
        UUID grantA = UUID.fromString("018f0e0a-0000-7000-8000-000000000501");
        UUID grantB = UUID.fromString("018f0e0a-0000-7000-8000-000000000502");
        AtomicInteger next = new AtomicInteger();
        UUID[] ids = {grantA, grantB};
        SensorPollGrantIssuer parallelIssuer = new SensorPollGrantIssuer(
                shareAware, endpoints, grants, audit,
                Clock.fixed(ISSUED, ZoneOffset.UTC),
                () -> ids[next.getAndIncrement()]);

        ExecutorService pool = Executors.newFixedThreadPool(2);
        Future<SensorPollGrant> first = null;
        Future<SensorPollGrant> second = null;
        try {
            first = pool.submit(() -> pg.tx().execute(status ->
                    parallelIssuer.issue(request(sensor, entity, endpoint, Protocol.SNMP))));
            second = pool.submit(() -> pg.tx().execute(status ->
                    parallelIssuer.issue(new SensorPollGrantIssuer.IssueRequest(
                            tenant, sensor, entity, endpoint, Protocol.ICMP,
                            2L, 0L, "planned-profile", "v1",
                            ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), "internal-actor"))));
            SensorPollGrant a = first.get(30, TimeUnit.SECONDS);
            SensorPollGrant b = second.get(30, TimeUnit.SECONDS);
            first = null;
            second = null;
            assertNotEquals(a.grantId(), b.grantId());
            assertEquals(2, shareHolders.get());
            assertEquals(2, countGrants());
        } finally {
            shutdownPool(pool, first, second);
        }
    }

    @Test
    void nanosecondBeyondMicrosecondPersistsAsDatabaseAuthoritativeRow() {
        clearGrants();
        Instant nanoBeyondMicro = Instant.parse("2026-10-28T12:00:00.123456789Z");
        assertTrue(nanoBeyondMicro.getNano() % 1_000 != 0,
                "fixture must carry nanoseconds beyond microsecond precision");
        Instant windowStart = nanoBeyondMicro.minusSeconds(1);
        Instant windowEnd = nanoBeyondMicro.plusSeconds(60);
        UUID grantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000901");
        SensorPollGrantIssuer nanoIssuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(nanoBeyondMicro, ZoneOffset.UTC),
                () -> grantId);

        SensorPollGrant returned = pg.tx().execute(status -> nanoIssuer.issue(
                new SensorPollGrantIssuer.IssueRequest(
                        tenant, sensor, entity, endpoint, Protocol.SNMP,
                        1L, 0L, "planned-profile", "v1",
                        windowStart, windowEnd, "internal-actor")));
        assertNotNull(returned);
        SensorPollGrant reloaded = grants.find(tenant, sensor, grantId).orElseThrow();
        assertEquals(returned, reloaded);
        assertNotEquals(nanoBeyondMicro, returned.issuedAt(),
                "returned row, not the pre-insert Java candidate, is authoritative");
        assertEquals(0, returned.issuedAt().getNano() % 1_000,
                "issued_at must have database microsecond precision");
        assertEquals(0, returned.windowStart().getNano() % 1_000);
        assertEquals(0, returned.windowEnd().getNano() % 1_000);
        assertEquals(0, returned.updatedAt().getNano() % 1_000);
        Instant truncated = Instant.parse("2026-10-28T12:00:00.123456Z");
        Instant rounded = Instant.parse("2026-10-28T12:00:00.123457Z");
        assertTrue(returned.issuedAt().equals(truncated) || returned.issuedAt().equals(rounded),
                "persisted issued_at must be the microsecond projection of the sampled clock");
        assertEquals(1, countGrants());
    }

    /**
     * Real PostgreSQL 16 + JDBC evidence that the declared timestamptz envelope
     * bounds survive SensorPollGrantIssuer INSERT ... RETURNING and repository reload.
     * A runtime failure at an exact constant remains production-correction evidence.
     */
    @Test
    void exactPostgresqlTimestamptzBoundariesPersistThroughIssuerAndRepository() {
        clearGrants();
        UUID lowerGrant = UUID.fromString("018f0e0a-0000-7000-8000-000000000a01");
        UUID upperGrant = UUID.fromString("018f0e0a-0000-7000-8000-000000000a02");

        Instant min = SensorPollGrantIssuer.PG_TIMESTAMPTZ_MIN;
        Instant maxAccepted = SensorPollGrantIssuer.PG_TIMESTAMPTZ_MAX_ACCEPTED;
        Instant lowerIssuedAt = min.plusSeconds(1);
        Instant lowerWindowEnd = min.plusSeconds(60);
        Instant upperWindowStart = maxAccepted.minusSeconds(60);
        Instant upperIssuedAt = maxAccepted.minusSeconds(1);

        SensorPollGrantIssuer lowerIssuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(lowerIssuedAt, ZoneOffset.UTC),
                () -> lowerGrant);
        SensorPollGrant lowerReturned = pg.tx().execute(status -> lowerIssuer.issue(
                new SensorPollGrantIssuer.IssueRequest(
                        tenant, sensor, entity, endpoint, Protocol.SNMP,
                        1L, 0L, "planned-profile", "v1",
                        min, lowerWindowEnd, "internal-actor")));
        assertExactBoundaryPersisted(lowerReturned, lowerGrant, min, lowerIssuedAt, lowerWindowEnd);

        SensorPollGrantIssuer upperIssuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(upperIssuedAt, ZoneOffset.UTC),
                () -> upperGrant);
        SensorPollGrant upperReturned = pg.tx().execute(status -> upperIssuer.issue(
                new SensorPollGrantIssuer.IssueRequest(
                        tenant, sensor, entity, endpoint, Protocol.SNMP,
                        2L, 0L, "planned-profile", "v1",
                        upperWindowStart, maxAccepted, "internal-actor")));
        assertExactBoundaryPersisted(
                upperReturned, upperGrant, upperWindowStart, upperIssuedAt, maxAccepted);

        int deleted = pg.tx().execute(status -> pg.jdbc().update(
                "DELETE FROM platform.sensor_poll_grant WHERE grant_id IN (?, ?)",
                lowerGrant, upperGrant));
        assertEquals(2, deleted, "cleanup must delete exactly the two boundary grants");
        assertEquals(0, countGrantRows(lowerGrant));
        assertEquals(0, countGrantRows(upperGrant));
        assertEquals(0, countGrants());
    }

    private void assertExactBoundaryPersisted(
            SensorPollGrant returned,
            UUID grantId,
            Instant expectedWindowStart,
            Instant expectedIssuedAt,
            Instant expectedWindowEnd) {
        assertNotNull(returned);
        assertEquals(grantId, returned.grantId());
        assertEquals(Status.ISSUED, returned.status());
        SensorPollGrant reloaded = grants.find(tenant, sensor, grantId).orElseThrow();
        assertEquals(returned, reloaded,
                "INSERT ... RETURNING row must reload exactly through the repository");
        assertEquals(expectedWindowStart, returned.windowStart());
        assertEquals(expectedWindowEnd, returned.windowEnd());
        assertEquals(expectedIssuedAt, returned.issuedAt());
        assertEquals(returned.windowStart(), reloaded.windowStart());
        assertEquals(returned.windowEnd(), reloaded.windowEnd());
        assertEquals(returned.issuedAt(), reloaded.issuedAt());
        assertEquals(0, returned.windowStart().getNano() % 1_000,
                "window_start precision must be at most microseconds");
        assertEquals(0, returned.windowEnd().getNano() % 1_000,
                "window_end precision must be at most microseconds");
        assertEquals(0, returned.issuedAt().getNano() % 1_000,
                "issued_at precision must be at most microseconds");
        assertEquals(1, countGrantRows(grantId),
                "exactly one row must exist for boundary grant " + grantId);
        assertNoPostgresqlInfinity(grantId);
    }

    private int countGrantRows(UUID grantId) {
        Integer count = pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant WHERE grant_id = ?",
                Integer.class, grantId);
        return count == null ? 0 : count;
    }

    private void assertNoPostgresqlInfinity(UUID grantId) {
        Boolean anyInfinity = pg.jdbc().queryForObject("""
                SELECT window_start = '-infinity'::timestamptz
                    OR window_start = 'infinity'::timestamptz
                    OR window_end = '-infinity'::timestamptz
                    OR window_end = 'infinity'::timestamptz
                    OR issued_at = '-infinity'::timestamptz
                    OR issued_at = 'infinity'::timestamptz
                    OR window_start::text ILIKE '%infinity%'
                    OR window_end::text ILIKE '%infinity%'
                    OR issued_at::text ILIKE '%infinity%'
                  FROM platform.sensor_poll_grant
                 WHERE grant_id = ?
                """, Boolean.class, grantId);
        assertEquals(Boolean.FALSE, anyInfinity,
                "boundary timestamptz must not silently map to PostgreSQL infinity");
    }

    @Test
    void generatedUuidCollisionIncludingForeignTenantMapsToNondisclosingConflict() {
        clearGrants();
        UUID foreignGrant = FIXED_GRANT;
        pg.tx().executeWithoutResult(tx -> {
            UUID foreignEntity = UUID.randomUUID();
            UUID foreignEndpoint = UUID.randomUUID();
            pg.jdbc().update("""
                    INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name,
                        lifecycle_state, first_seen_at, last_seen_at, created_by, created_at, updated_at)
                    VALUES (?, ?, 'NETWORK_DEVICE', 'foreign-fixture', 'ACTIVE', NOW(), NOW(), 'fixture', NOW(), NOW())
                    """, foreignEntity, otherTenant);
            pg.jdbc().update("""
                    INSERT INTO platform.entity_management_endpoint (endpoint_id, tenant_id, entity_id,
                        address_type, address, state, source, is_primary, created_by, created_at, updated_by, updated_at)
                    VALUES (?, ?, ?, 'IPV4', '192.0.2.50', 'ACTIVE', 'MANUAL', TRUE, 'fixture', NOW(), 'fixture', NOW())
                    """, foreignEndpoint, otherTenant, foreignEntity);
            pg.jdbc().update("""
                    INSERT INTO platform.sensor_poll_grant (grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                        grant_generation, assignment_generation, protocol, issued_address_type, issued_address,
                        profile_plan_id, profile_plan_version, issued_at, window_start, window_end, issued_by,
                        status, updated_at)
                    VALUES (?, ?, ?, ?, ?, 1, 0, 'SNMP', 'IPV4', '192.0.2.50',
                        'foreign-plan', 'v1', ?, ?, ?, 'foreign-actor', 'ISSUED', ?)
                    """, foreignGrant, otherTenant, otherTenantSensor, foreignEntity, foreignEndpoint,
                    Timestamp.from(ISSUED), Timestamp.from(ISSUED.minusSeconds(1)),
                    Timestamp.from(ISSUED.plusSeconds(60)), Timestamp.from(ISSUED));
        });

        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> pg.tx().execute(status -> issuer.issue(request(sensor, entity, endpoint, Protocol.SNMP))));
        assertEquals(SensorControlException.ERR_POLL_GRANT_CONFLICT, failure.getCode());
        assertFalse(failure.getMessage().toLowerCase().contains("duplicate"));
        assertFalse(failure.getMessage().toLowerCase().contains("sql"));
        assertFalse(failure.getMessage().toLowerCase().contains(otherTenant.toString()));
        assertEquals(0, countGrants());
        assertTrue(auditLog.list.stream().noneMatch(event ->
                event.getFormattedMessage().contains("sensor.poll_grant.issue")));
    }

    @Test
    void endpointSnapshotUnchangedAfterLaterEndpointEdit() {
        SensorPollGrant row = pg.tx().execute(status -> issuer.issue(request(sensor, entity, endpoint, Protocol.SNMP)));
        pg.tx().executeWithoutResult(tx ->
                pg.jdbc().update("""
                        UPDATE platform.entity_management_endpoint
                           SET address = '2001:db8::ff', address_type = 'IPV6', updated_at = NOW()
                         WHERE endpoint_id = ?
                        """, endpoint));
        SensorPollGrant reloaded = grants.find(tenant, sensor, row.grantId()).orElseThrow();
        assertEquals("2001:db8::aa", reloaded.issuedAddress());
        assertEquals(row, reloaded);
        String live = pg.jdbc().queryForObject(
                "SELECT address FROM platform.entity_management_endpoint WHERE endpoint_id = ?",
                String.class, endpoint);
        assertEquals("2001:db8::ff", live);
    }

    @Test
    void rollbackLeavesNoGrantAndNoSuccessAudit() {
        auditLog.list.clear();
        assertThrows(SensorControlException.class, () -> pg.tx().execute(status -> {
            issuer.issue(request(sensor, entity, endpoint, Protocol.SNMP));
            status.setRollbackOnly();
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_CONFLICT, "forced-rollback");
        }));
        assertEquals(0, countGrants());
        assertTrue(auditLog.list.stream().noneMatch(event ->
                event.getFormattedMessage().contains("sensor.poll_grant.issue")));
    }

    @Test
    void ordinaryRollbackAfterInsertEmitsNoAudit() {
        auditLog.list.clear();
        pg.tx().execute(status -> {
            issuer.issue(request(sensor, entity, endpoint, Protocol.SNMP));
            status.setRollbackOnly();
            return null;
        });
        assertEquals(0, countGrants());
        assertTrue(auditLog.list.stream().noneMatch(event ->
                event.getFormattedMessage().contains("sensor.poll_grant.issue")));
    }

    @Test
    void noSeededGrantExistsBeforeIssuanceAndNoPublicWiringSurface() {
        assertEquals(0, countGrants());
        assertTrue(grants.find(tenant, sensor, FIXED_GRANT).isEmpty());
        assertEquals("sensor.poll_grant.issue", AuditActions.SENSOR_POLL_GRANT_ISSUE.value());
        assertFalse(SensorPollGrantIssuer.class.isAnnotationPresent(
                org.springframework.stereotype.Service.class));
        assertFalse(SensorPollGrantIssuer.class.isAnnotationPresent(
                org.springframework.web.bind.annotation.RestController.class));
        assertFalse(SensorPollGrantIssuer.class.isAnnotationPresent(
                org.springframework.scheduling.annotation.Scheduled.class));
        assertFalse(SensorPollGrantIssuer.class.isAnnotationPresent(
                org.springframework.context.annotation.Bean.class));
    }

    private void assertPrincipalShareHeldUntilCompletion(boolean commitHolder) throws Exception {
        CountDownLatch held = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        AtomicReference<Throwable> holderFailure = new AtomicReference<>();
        AtomicReference<Integer> holderPid = new AtomicReference<>();
        UUID grantId = commitHolder
                ? UUID.fromString("018f0e0a-0000-7000-8000-000000000601")
                : UUID.fromString("018f0e0a-0000-7000-8000-000000000602");
        SensorPollGrantIssuer localIssuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(ISSUED, ZoneOffset.UTC),
                () -> grantId);
        ExecutorService pool = Executors.newSingleThreadExecutor();
        Future<?> holder = null;
        try {
            holder = pool.submit(() -> {
                try {
                    pg.tx().executeWithoutResult(status -> {
                        localIssuer.issue(request(sensor, entity, endpoint, Protocol.SNMP));
                        holderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                        held.countDown();
                        await(release);
                        if (!commitHolder) {
                            status.setRollbackOnly();
                        }
                    });
                } catch (Throwable t) {
                    holderFailure.set(t);
                    held.countDown();
                    throw t instanceof RuntimeException re ? re : new IllegalStateException(t);
                }
                return null;
            });
            assertTrue(held.await(30, TimeUnit.SECONDS));
            assertNull(holderFailure.get());
            assertNotNull(holderPid.get());
            try (Connection contender = openContender()) {
                contender.setAutoCommit(false);
                assertDifferentBackend(contender, holderPid.get());
                // FOR NO KEY UPDATE distinguishes principal FOR SHARE from weaker FK KEY SHARE.
                assertForUpdateNowaitConflict(contender, """
                        SELECT sensor_id FROM platform.sensor
                         WHERE tenant_id = ? AND sensor_id = ?
                         FOR NO KEY UPDATE NOWAIT
                        """, ps -> {
                    ps.setObject(1, tenant);
                    ps.setString(2, sensor);
                }, "principal must remain SHARE-locked (55P03) while issuance holds");
                contender.rollback();
            }
            release.countDown();
            holder.get(30, TimeUnit.SECONDS);
            holder = null;
            try (Connection after = openContender()) {
                after.setAutoCommit(false);
                assertForUpdateNowaitAcquired(after, """
                        SELECT sensor_id FROM platform.sensor
                         WHERE tenant_id = ? AND sensor_id = ?
                         FOR NO KEY UPDATE NOWAIT
                        """, ps -> {
                    ps.setObject(1, tenant);
                    ps.setString(2, sensor);
                }, "principal FOR NO KEY UPDATE NOWAIT must succeed after holder "
                        + (commitHolder ? "commit" : "rollback"));
                after.rollback();
            }
        } finally {
            release.countDown();
            shutdownPool(pool, holder);
        }
    }

    private void assertEndpointShareHeldUntilCompletion(boolean commitHolder) throws Exception {
        CountDownLatch held = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        AtomicReference<Integer> holderPid = new AtomicReference<>();
        UUID grantId = commitHolder
                ? UUID.fromString("018f0e0a-0000-7000-8000-000000000701")
                : UUID.fromString("018f0e0a-0000-7000-8000-000000000702");
        SensorPollGrantIssuer localIssuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(ISSUED, ZoneOffset.UTC),
                () -> grantId);
        ExecutorService pool = Executors.newSingleThreadExecutor();
        Future<?> holder = null;
        try {
            holder = pool.submit(() -> {
                pg.tx().executeWithoutResult(status -> {
                    localIssuer.issue(request(sensor, entity, endpoint, Protocol.SNMP));
                    holderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                    held.countDown();
                    await(release);
                    if (!commitHolder) {
                        status.setRollbackOnly();
                    }
                });
                return null;
            });
            assertTrue(held.await(30, TimeUnit.SECONDS));
            assertNotNull(holderPid.get());
            try (Connection contender = openContender()) {
                contender.setAutoCommit(false);
                assertDifferentBackend(contender, holderPid.get());
                assertForUpdateNowaitConflict(contender, """
                        SELECT endpoint_id FROM platform.entity_management_endpoint
                         WHERE tenant_id = ? AND entity_id = ? AND endpoint_id = ?
                         FOR NO KEY UPDATE NOWAIT
                        """, ps -> {
                    ps.setObject(1, tenant);
                    ps.setObject(2, entity);
                    ps.setObject(3, endpoint);
                }, "endpoint must remain SHARE-locked (55P03) while issuance holds");
                contender.rollback();
            }
            release.countDown();
            holder.get(30, TimeUnit.SECONDS);
            holder = null;
            try (Connection after = openContender()) {
                after.setAutoCommit(false);
                assertForUpdateNowaitAcquired(after, """
                        SELECT endpoint_id FROM platform.entity_management_endpoint
                         WHERE tenant_id = ? AND entity_id = ? AND endpoint_id = ?
                         FOR NO KEY UPDATE NOWAIT
                        """, ps -> {
                    ps.setObject(1, tenant);
                    ps.setObject(2, entity);
                    ps.setObject(3, endpoint);
                }, "endpoint FOR NO KEY UPDATE NOWAIT must succeed after holder "
                        + (commitHolder ? "commit" : "rollback"));
                after.rollback();
            }
        } finally {
            release.countDown();
            shutdownPool(pool, holder);
        }
    }

    private void assertIssuanceBlocksPrincipalMutationUntilCompletion(boolean commitHolder) throws Exception {
        CountDownLatch held = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        CountDownLatch aboutToMutate = new CountDownLatch(1);
        AtomicBoolean mutationCompleted = new AtomicBoolean(false);
        AtomicReference<Integer> holderPid = new AtomicReference<>();
        AtomicReference<Integer> contenderPid = new AtomicReference<>();
        UUID grantId = commitHolder
                ? UUID.fromString("018f0e0a-0000-7000-8000-000000000801")
                : UUID.fromString("018f0e0a-0000-7000-8000-000000000802");
        SensorPollGrantIssuer localIssuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(ISSUED, ZoneOffset.UTC),
                () -> grantId);
        ExecutorService pool = Executors.newFixedThreadPool(2);
        Future<?> holder = null;
        Future<?> mutator = null;
        try {
            holder = pool.submit(() -> {
                pg.tx().executeWithoutResult(status -> {
                    localIssuer.issue(request(sensor, entity, endpoint, Protocol.SNMP));
                    holderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                    held.countDown();
                    await(release);
                    if (!commitHolder) {
                        status.setRollbackOnly();
                    }
                });
                return null;
            });
            assertTrue(held.await(30, TimeUnit.SECONDS));
            assertNotNull(holderPid.get());

            mutator = pool.submit(() -> {
                pg.tx().executeWithoutResult(status -> {
                    contenderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                    aboutToMutate.countDown();
                    sensors.lockPrincipalForUpdate(tenant, sensor);
                    mutationCompleted.set(true);
                });
                return null;
            });

            assertTrue(aboutToMutate.await(30, TimeUnit.SECONDS));
            assertNotNull(contenderPid.get());
            assertNotEquals(holderPid.get(), contenderPid.get(),
                    "issuance holder and mutation contender must be independent backends");
            awaitDatabaseLockWait(contenderPid.get());
            try (Connection probe = openContender()) {
                probe.setAutoCommit(false);
                assertDifferentBackend(probe, holderPid.get());
                assertForUpdateNowaitConflict(probe, """
                        SELECT sensor_id FROM platform.sensor
                         WHERE tenant_id = ? AND sensor_id = ?
                         FOR NO KEY UPDATE NOWAIT
                        """, ps -> {
                    ps.setObject(1, tenant);
                    ps.setString(2, sensor);
                }, "mutation must observe principal still SHARE-locked before release");
                probe.rollback();
            }
            assertFalse(mutationCompleted.get(),
                    "principal FOR UPDATE must not complete before issuance releases SHARE");
            release.countDown();
            holder.get(30, TimeUnit.SECONDS);
            holder = null;
            mutator.get(30, TimeUnit.SECONDS);
            mutator = null;
            assertTrue(mutationCompleted.get());
        } finally {
            release.countDown();
            shutdownPool(pool, holder, mutator);
        }
    }

    private SensorPollGrantIssuer.IssueRequest request(
            String sensorId, UUID entityId, UUID endpointId, Protocol protocol) {
        return new SensorPollGrantIssuer.IssueRequest(
                tenant, sensorId, entityId, endpointId, protocol,
                1L, 0L, "planned-profile", "v1",
                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), "internal-actor");
    }

    private int countGrants() {
        Integer count = pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant WHERE tenant_id = ?",
                Integer.class, tenant);
        return count == null ? 0 : count;
    }

    private void clearGrants() {
        pg.tx().executeWithoutResult(tx ->
                pg.jdbc().update("DELETE FROM platform.sensor_poll_grant WHERE tenant_id IN (?, ?)",
                        tenant, otherTenant));
    }

    private Connection openContender() throws SQLException {
        Connection connection = pg.dataSource().getConnection();
        try {
            connection.setNetworkTimeout(Runnable::run, 5_000);
        } catch (SQLException | AbstractMethodError | UnsupportedOperationException ignored) {
            // statement queryTimeout remains mandatory below
        }
        return connection;
    }

    /**
     * Poll PostgreSQL until the exact contender backend is observed waiting on a
     * database lock. Time delay between polls is not itself proof of waiting.
     */
    private void awaitDatabaseLockWait(int backendPid) throws SQLException {
        long deadlineNanos = System.nanoTime() + TimeUnit.SECONDS.toNanos(30);
        try (Connection control = openContender()) {
            Integer controlPid = null;
            try (PreparedStatement pidStatement = control.prepareStatement("SELECT pg_backend_pid()")) {
                pidStatement.setQueryTimeout(5);
                try (ResultSet rs = pidStatement.executeQuery()) {
                    assertTrue(rs.next());
                    controlPid = rs.getInt(1);
                }
            }
            assertNotEquals(backendPid, controlPid,
                    "lock-wait control connection must be a distinct PostgreSQL backend");
            while (System.nanoTime() < deadlineNanos) {
                if (isBackendWaitingOnLock(control, backendPid)) {
                    return;
                }
                LockSupport.parkNanos(TimeUnit.MILLISECONDS.toNanos(25));
            }
        }
        fail("PostgreSQL never reported backend pid=" + backendPid
                + " waiting on Lock / ungranted pg_locks within timeout");
    }

    private static boolean isBackendWaitingOnLock(Connection control, int backendPid) throws SQLException {
        try (PreparedStatement statement = control.prepareStatement("""
                SELECT EXISTS (
                         SELECT 1 FROM pg_stat_activity
                          WHERE pid = ? AND wait_event_type = 'Lock'
                       )
                    OR EXISTS (
                         SELECT 1 FROM pg_locks
                          WHERE pid = ? AND NOT granted
                       )
                """)) {
            statement.setQueryTimeout(5);
            statement.setInt(1, backendPid);
            statement.setInt(2, backendPid);
            try (ResultSet result = statement.executeQuery()) {
                assertTrue(result.next());
                return result.getBoolean(1);
            }
        }
    }

    private static void shutdownPool(ExecutorService pool, Future<?>... futures) throws InterruptedException {
        for (Future<?> future : futures) {
            if (future != null && !future.isDone()) {
                future.cancel(true);
            }
        }
        pool.shutdownNow();
        assertTrue(pool.awaitTermination(10, TimeUnit.SECONDS),
                "executor remained alive after shutdownNow/awaitTermination");
    }

    private static void assertDifferentBackend(Connection independent, int holderPid) {
        assertDoesNotThrow(() -> {
            try (PreparedStatement statement = independent.prepareStatement("SELECT pg_backend_pid()")) {
                statement.setQueryTimeout(5);
                try (ResultSet result = statement.executeQuery()) {
                    assertTrue(result.next());
                    assertNotEquals(holderPid, result.getInt(1),
                            "contender must be a different PostgreSQL backend/session");
                }
            }
        });
    }

    @FunctionalInterface
    private interface SqlBinder {
        void bind(PreparedStatement ps) throws SQLException;
    }

    private static void assertForUpdateNowaitConflict(
            Connection conn, String sql, SqlBinder binder, String message) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement(sql)) {
            ps.setQueryTimeout(5);
            binder.bind(ps);
            try {
                ps.executeQuery().close();
                fail(message + "; contender unexpectedly acquired lock");
            } catch (SQLException e) {
                assertTrue(sqlStateInChain(e, "55P03"),
                        message + "; expected SQLSTATE 55P03, got " + describeSqlStates(e));
            }
        }
    }

    private static void assertForUpdateNowaitAcquired(
            Connection conn, String sql, SqlBinder binder, String message) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement(sql)) {
            ps.setQueryTimeout(5);
            binder.bind(ps);
            try (ResultSet rs = ps.executeQuery()) {
                assertTrue(rs.next(), message + "; exact row missing");
            } catch (SQLException e) {
                fail(message + "; got " + describeSqlStates(e));
            }
        }
    }

    private static boolean sqlStateInChain(Throwable t, String sqlState) {
        for (Throwable cur = t; cur != null; cur = cur.getCause()) {
            if (cur instanceof SQLException sqlEx) {
                for (SQLException walk = sqlEx; walk != null; walk = walk.getNextException()) {
                    if (sqlState.equals(walk.getSQLState())) {
                        return true;
                    }
                }
            }
        }
        return false;
    }

    private static String describeSqlStates(Throwable t) {
        StringBuilder out = new StringBuilder();
        for (Throwable cur = t; cur != null; cur = cur.getCause()) {
            if (cur instanceof SQLException sqlEx) {
                for (SQLException walk = sqlEx; walk != null; walk = walk.getNextException()) {
                    if (!out.isEmpty()) {
                        out.append(',');
                    }
                    out.append(walk.getSQLState());
                }
            }
        }
        return out.isEmpty() ? t.toString() : out.toString();
    }

    private static SensorControlException unwrap(Throwable failure) {
        for (Throwable cur = failure; cur != null; cur = cur.getCause()) {
            if (cur instanceof SensorControlException typed) {
                return typed;
            }
        }
        fail("expected SensorControlException, got " + failure);
        throw new IllegalStateException(failure);
    }

    private static void await(CountDownLatch latch) {
        try {
            assertTrue(latch.await(30, TimeUnit.SECONDS));
        } catch (InterruptedException interrupted) {
            Thread.currentThread().interrupt();
            throw new IllegalStateException(interrupted);
        }
    }
}
