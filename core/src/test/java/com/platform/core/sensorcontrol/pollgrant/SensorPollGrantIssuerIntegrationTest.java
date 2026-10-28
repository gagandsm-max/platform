package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.persistence.EntityManagementEndpointRepository;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.sensorcontrol.SensorControlException;
import com.platform.core.sensorcontrol.SensorControlRepository;
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
import java.sql.SQLException;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

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
                    pg.jdbc().update("DELETE FROM platform.sensor_allowed_entity WHERE tenant_id = ?", tenant);
                    pg.jdbc().update("DELETE FROM platform.entity_management_endpoint WHERE tenant_id = ?", tenant);
                    pg.jdbc().update("DELETE FROM platform.entity WHERE tenant_id = ?", tenant);
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
    void allowAllSuccessfulIssuancePersistsExactSnapshot() {
        SensorPollGrant row = pg.tx().execute(status -> issuer.issue(request(sensor, entity, endpoint, Protocol.SNMP)));
        assertNotNull(row);
        assertEquals(FIXED_GRANT, row.grantId());
        assertEquals(Status.ISSUED, row.status());
        assertEquals("2001:db8::aa", row.issuedAddress());
        assertEquals(ISSUED, row.issuedAt());
        assertEquals(row, grants.find(tenant, sensor, FIXED_GRANT).orElseThrow());
        assertTrue(auditLog.list.stream().anyMatch(event ->
                event.getFormattedMessage().contains("sensor.poll_grant.issue")
                        && event.getFormattedMessage().contains("SUCCESS")
                        && !event.getFormattedMessage().contains("2001:db8")));
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
    void heldPrincipalLockSerializesConcurrentScopeReplacement() throws Exception {
        CountDownLatch locked = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        AtomicBoolean replaceFinished = new AtomicBoolean(false);
        ExecutorService pool = Executors.newFixedThreadPool(2);
        try {
            Future<?> holder = pool.submit(() -> {
                pg.tx().executeWithoutResult(status -> {
                    issuer.issue(request(sensor, entity, endpoint, Protocol.SNMP));
                    locked.countDown();
                    await(release);
                });
                return null;
            });
            Future<?> replacer = pool.submit(() -> {
                assertTrue(locked.await(30, TimeUnit.SECONDS));
                pg.tx().executeWithoutResult(status -> {
                    sensors.lockPrincipalForUpdate(tenant, sensor);
                    sensors.replaceAllowedEntities(tenant, sensor, java.util.Set.of(), Instant.now());
                    replaceFinished.set(true);
                });
                return null;
            });
            assertTrue(locked.await(30, TimeUnit.SECONDS));
            Thread.sleep(500);
            assertFalse(replaceFinished.get(),
                    "scope replacement must wait while issuance holds principal FOR UPDATE");
            release.countDown();
            holder.get(30, TimeUnit.SECONDS);
            replacer.get(30, TimeUnit.SECONDS);
        } finally {
            pool.shutdownNow();
        }
        assertTrue(replaceFinished.get());
        assertEquals(1, countGrants());
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
    void heldPrincipalLockBlocksConcurrentDisable() throws Exception {
        CountDownLatch locked = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        AtomicBoolean disableFinished = new AtomicBoolean(false);
        AtomicReference<Throwable> disableFailure = new AtomicReference<>();
        ExecutorService pool = Executors.newFixedThreadPool(2);
        try {
            Future<?> holder = pool.submit(() -> {
                pg.tx().executeWithoutResult(status -> {
                    issuer.issue(request(sensor, entity, endpoint, Protocol.SNMP));
                    locked.countDown();
                    await(release);
                });
                return null;
            });
            Future<?> disabler = pool.submit(() -> {
                assertTrue(locked.await(30, TimeUnit.SECONDS));
                try {
                    pg.tx().executeWithoutResult(status ->
                            sensors.lockPrincipalForUpdate(tenant, sensor));
                    disableFinished.set(true);
                } catch (Throwable t) {
                    disableFailure.set(t);
                }
                return null;
            });
            assertTrue(locked.await(30, TimeUnit.SECONDS));
            Thread.sleep(500);
            assertFalse(disableFinished.get(), "disable/lock must wait while issuance holds principal FOR UPDATE");
            release.countDown();
            holder.get(30, TimeUnit.SECONDS);
            disabler.get(30, TimeUnit.SECONDS);
        } finally {
            pool.shutdownNow();
        }
        assertNull(disableFailure.get());
        assertTrue(disableFinished.get());
        assertEquals(1, countGrants());
    }

    @Test
    void heldEndpointShareBlocksConcurrentEndpointMutation() throws Exception {
        CountDownLatch locked = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        AtomicBoolean mutationFinished = new AtomicBoolean(false);
        ExecutorService pool = Executors.newFixedThreadPool(2);
        try {
            Future<?> holder = pool.submit(() -> {
                pg.tx().executeWithoutResult(status -> {
                    issuer.issue(request(sensor, entity, endpoint, Protocol.SNMP));
                    locked.countDown();
                    await(release);
                });
                return null;
            });
            Future<?> mutator = pool.submit(() -> {
                assertTrue(locked.await(30, TimeUnit.SECONDS));
                try (Connection independent = pg.dataSource().getConnection()) {
                    independent.setAutoCommit(false);
                    try (PreparedStatement statement = independent.prepareStatement("""
                            UPDATE platform.entity_management_endpoint
                               SET address = '2001:db8::mutated'
                             WHERE endpoint_id = ?
                            """)) {
                        statement.setQueryTimeout(2);
                        statement.setObject(1, endpoint);
                        try {
                            statement.executeUpdate();
                            mutationFinished.set(true);
                            independent.commit();
                        } catch (SQLException expected) {
                            independent.rollback();
                        }
                    }
                }
                return null;
            });
            assertTrue(locked.await(30, TimeUnit.SECONDS));
            Thread.sleep(500);
            assertFalse(mutationFinished.get(),
                    "endpoint mutation must not complete while issuance holds FOR SHARE");
            release.countDown();
            holder.get(30, TimeUnit.SECONDS);
            mutator.get(30, TimeUnit.SECONDS);
        } finally {
            pool.shutdownNow();
        }
        SensorPollGrant row = grants.find(tenant, sensor, FIXED_GRANT).orElseThrow();
        assertEquals("2001:db8::aa", row.issuedAddress());
    }

    @Test
    void noSeededGrantExistsBeforeIssuanceAndNoPublicWiringSurface() {
        assertEquals(0, countGrants());
        assertTrue(grants.find(tenant, sensor, FIXED_GRANT).isEmpty());
        assertEquals("sensor.poll_grant.issue", AuditActions.SENSOR_POLL_GRANT_ISSUE.value());
        // Source-only proof: issuer is constructed in-test; no Spring controller/route is invoked.
        assertFalse(SensorPollGrantIssuer.class.isAnnotationPresent(
                org.springframework.stereotype.Service.class));
        assertFalse(SensorPollGrantIssuer.class.isAnnotationPresent(
                org.springframework.web.bind.annotation.RestController.class));
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

    private static void await(CountDownLatch latch) {
        try {
            assertTrue(latch.await(30, TimeUnit.SECONDS));
        } catch (InterruptedException interrupted) {
            Thread.currentThread().interrupt();
            throw new IllegalStateException(interrupted);
        }
    }
}
