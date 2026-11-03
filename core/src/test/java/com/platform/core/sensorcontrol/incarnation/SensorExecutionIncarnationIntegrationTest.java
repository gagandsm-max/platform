package com.platform.core.sensorcontrol.incarnation;

import com.platform.core.control.ControlGenerationLimits;
import com.platform.core.sensorcontrol.SensorControlException;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.jdbc.core.JdbcTemplate;

import java.util.List;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

import static com.platform.core.sensorcontrol.incarnation.SensorExecutionIncarnation.Status;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Issue #288 G1b-C3: inert Core Sensor execution incarnation persistence.
 * No enrollment binding, grant delivery or live polling authorization.
 */
@Tag("postgres")
class SensorExecutionIncarnationIntegrationTest {

    private PostgresTestSupport pg;
    private JdbcTemplate jdbc;
    private SensorExecutionIncarnationRepository repo;
    private final UUID tenant = UUID.fromString("018f0e0a-0000-7000-8000-000000000288");
    private final UUID otherTenant = UUID.fromString("018f0e0a-0000-7000-8000-000000000289");
    private final String sensor = "sensor-incarnation-c3";
    private final String otherSensor = "sensor-incarnation-c3-other";
    private final String foreignSensor = "sensor-incarnation-c3-foreign-tenant";
    private boolean fixtureOwned;

    @BeforeEach
    void start() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        jdbc = pg.jdbc();
        repo = new SensorExecutionIncarnationRepository(pg.namedJdbc());
        pg.tx().executeWithoutResult(tx -> {
            for (UUID id : List.of(tenant, otherTenant)) {
                jdbc.update("""
                        INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                        VALUES (?, ?, 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                        """, id, "incarnation-fixture-" + id);
            }
            for (String id : List.of(sensor, otherSensor)) {
                jdbc.update("""
                        INSERT INTO platform.sensor (sensor_id, tenant_id, role, status,
                            allow_all_tenant_entities, created_at, updated_at)
                        VALUES (?, ?, 'NETWORK', 'ACTIVE', FALSE, NOW(), NOW())
                        """, id, tenant);
            }
            // sensor_id is globally unique (V004 PK); cross-tenant isolation uses a distinct id.
            jdbc.update("""
                    INSERT INTO platform.sensor (sensor_id, tenant_id, role, status,
                        allow_all_tenant_entities, created_at, updated_at)
                    VALUES (?, ?, 'NETWORK', 'ACTIVE', FALSE, NOW(), NOW())
                    """, foreignSensor, otherTenant);
        });
        fixtureOwned = true;
    }

    @AfterEach
    void stop() {
        try {
            if (fixtureOwned) {
                pg.tx().executeWithoutResult(tx -> {
                    for (UUID id : List.of(tenant, otherTenant)) {
                        jdbc.update("DELETE FROM platform.sensor_certificate_credential WHERE tenant_id = ?", id);
                        jdbc.update("DELETE FROM platform.sensor WHERE tenant_id = ?", id);
                        jdbc.update("DELETE FROM platform.tenant WHERE tenant_id = ?", id);
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
    void issueInitialMintsOpaqueCoreIdentityAndGenerationOne() {
        SensorExecutionIncarnation issued = pg.tx().execute(tx -> repo.issueInitial(tenant, sensor));
        assertNotNull(issued);
        assertNotNull(issued.incarnationId());
        assertEquals(tenant, issued.tenantId());
        assertEquals(sensor, issued.sensorId());
        assertEquals(1L, issued.incarnationGeneration());
        assertEquals(Status.ACTIVE, issued.status());
        assertNotNull(issued.activatedAt());
        assertNull(issued.supersededAt());
        assertEquals(issued, repo.findActive(tenant, sensor).orElseThrow());
        assertEquals(Optional.empty(), repo.findActive(otherTenant, foreignSensor));
        assertEquals(Optional.empty(), repo.findActive(tenant, otherSensor));
    }

    @Test
    void legacySensorRemainsAbsentUntilExplicitIssue_andRepeatIssueFailsClosed() {
        assertTrue(repo.findActive(tenant, sensor).isEmpty());
        assertEquals(0, jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = ? AND sensor_id = ?
                """, Integer.class, tenant, sensor));

        SensorExecutionIncarnation first = pg.tx().execute(tx -> repo.issueInitial(tenant, sensor));
        SensorControlException duplicate = assertThrows(SensorControlException.class,
                () -> pg.tx().executeWithoutResult(tx -> repo.issueInitial(tenant, sensor)));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, duplicate.getCode());
        assertEquals(first, repo.findActive(tenant, sensor).orElseThrow());
        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = ? AND sensor_id = ?
                """, Integer.class, tenant, sensor));
    }

    @Test
    void rolloverIsMonotonicPreservesHistoryAndKeepsOneActive() {
        SensorExecutionIncarnation first = pg.tx().execute(tx -> repo.issueInitial(tenant, sensor));
        SensorExecutionIncarnation second = pg.tx().execute(tx -> repo.rollover(tenant, sensor));

        assertNotEquals(first.incarnationId(), second.incarnationId());
        assertEquals(2L, second.incarnationGeneration());
        assertEquals(Status.ACTIVE, second.status());
        assertEquals(second, repo.findActive(tenant, sensor).orElseThrow());

        SensorExecutionIncarnation prior = repo.find(tenant, sensor, first.incarnationId()).orElseThrow();
        assertEquals(Status.SUPERSEDED, prior.status());
        assertNotNull(prior.supersededAt());
        assertTrue(!prior.supersededAt().isBefore(prior.activatedAt()));
        assertEquals(2, jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = ? AND sensor_id = ?
                """, Integer.class, tenant, sensor));
        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = ? AND sensor_id = ? AND status = 'ACTIVE'
                """, Integer.class, tenant, sensor));
    }

    @Test
    void credentialRotationDoesNotChangeIncarnation() {
        SensorExecutionIncarnation issued = pg.tx().execute(tx -> repo.issueInitial(tenant, sensor));
        UUID credentialA = UUID.fromString("018f0e0a-0000-7000-8000-0000000002a1");
        UUID credentialB = UUID.fromString("018f0e0a-0000-7000-8000-0000000002a2");
        String fingerprintA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
        String fingerprintB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";

        jdbc.update("""
                INSERT INTO platform.sensor_certificate_credential (
                    credential_id, tenant_id, sensor_id, fingerprint_sha256, serial_number,
                    issuer_dn, not_before, not_after, status, activated_at, created_at, updated_at)
                VALUES (?, ?, ?, ?, 'serial-a', 'CN=fixture-ca', NOW() - INTERVAL '1 hour',
                        NOW() + INTERVAL '30 days', 'ACTIVE', NOW(), NOW(), NOW())
                """, credentialA, tenant, sensor, fingerprintA);

        // Simulated credential rotation under the same principal: supersede old, activate new.
        jdbc.update("""
                UPDATE platform.sensor_certificate_credential
                   SET status = 'SUPERSEDED', revoked_at = NOW(), updated_at = NOW()
                 WHERE credential_id = ?
                """, credentialA);
        jdbc.update("""
                INSERT INTO platform.sensor_certificate_credential (
                    credential_id, tenant_id, sensor_id, fingerprint_sha256, serial_number,
                    issuer_dn, not_before, not_after, status, activated_at, created_at, updated_at)
                VALUES (?, ?, ?, ?, 'serial-b', 'CN=fixture-ca', NOW() - INTERVAL '1 hour',
                        NOW() + INTERVAL '30 days', 'ACTIVE', NOW(), NOW(), NOW())
                """, credentialB, tenant, sensor, fingerprintB);

        SensorExecutionIncarnation afterRotation = repo.findActive(tenant, sensor).orElseThrow();
        assertEquals(issued.incarnationId(), afterRotation.incarnationId());
        assertEquals(issued.incarnationGeneration(), afterRotation.incarnationGeneration());
        assertEquals(Status.ACTIVE, afterRotation.status());
        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = ? AND sensor_id = ?
                """, Integer.class, tenant, sensor));
        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.sensor_certificate_credential
                 WHERE tenant_id = ? AND sensor_id = ? AND status = 'ACTIVE'
                """, Integer.class, tenant, sensor));
    }

    @Test
    void tenantAndSensorIsolationRejectsCrossScopeReadsAndMutations() {
        SensorExecutionIncarnation home = pg.tx().execute(tx -> repo.issueInitial(tenant, sensor));
        SensorExecutionIncarnation foreign = pg.tx().execute(tx -> repo.issueInitial(otherTenant, foreignSensor));

        assertTrue(repo.find(otherTenant, foreignSensor, home.incarnationId()).isEmpty());
        assertTrue(repo.find(tenant, otherSensor, home.incarnationId()).isEmpty());
        assertTrue(repo.find(tenant, sensor, foreign.incarnationId()).isEmpty());
        assertTrue(repo.find(otherTenant, sensor, home.incarnationId()).isEmpty());
        assertEquals(home, repo.findActive(tenant, sensor).orElseThrow());
        assertEquals(foreign, repo.findActive(otherTenant, foreignSensor).orElseThrow());
        assertTrue(repo.findActive(tenant, otherSensor).isEmpty());
        assertTrue(repo.findActive(otherTenant, sensor).isEmpty());
    }

    @Test
    void missingPrincipalAndMissingTransactionFailClosed() {
        SensorControlException missing = assertThrows(SensorControlException.class,
                () -> pg.tx().executeWithoutResult(tx -> repo.issueInitial(tenant, "missing-sensor")));
        assertEquals(SensorControlException.ERR_SENSOR_NOT_FOUND, missing.getCode());

        SensorControlException noTx = assertThrows(SensorControlException.class,
                () -> repo.issueInitial(tenant, sensor));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, noTx.getCode());
        assertTrue(noTx.getMessage().contains("Active PostgreSQL transaction required"));
        assertTrue(repo.findActive(tenant, sensor).isEmpty());
    }

    @Test
    void callerRollbackRestoresAbsenceAndAllowsRetry() {
        pg.tx().executeWithoutResult(tx -> {
            repo.issueInitial(tenant, sensor);
            assertTrue(repo.findActive(tenant, sensor).isPresent());
            tx.setRollbackOnly();
        });
        assertTrue(repo.findActive(tenant, sensor).isEmpty());
        SensorExecutionIncarnation retried = pg.tx().execute(tx -> repo.issueInitial(tenant, sensor));
        assertEquals(1L, retried.incarnationGeneration());
        assertEquals(Status.ACTIVE, retried.status());
    }

    @Test
    void concurrentInitialIssuanceCommitsExactlyOneWinner() throws Exception {
        ExecutorService workers = Executors.newFixedThreadPool(2);
        CountDownLatch ready = new CountDownLatch(2);
        CountDownLatch start = new CountDownLatch(1);
        AtomicReference<Throwable> unexpected = new AtomicReference<>();
        try {
            Future<Optional<SensorExecutionIncarnation>> a = workers.submit(() -> attemptIssue(ready, start, unexpected));
            Future<Optional<SensorExecutionIncarnation>> b = workers.submit(() -> attemptIssue(ready, start, unexpected));
            assertTrue(ready.await(5, TimeUnit.SECONDS));
            start.countDown();
            Optional<SensorExecutionIncarnation> first = a.get(15, TimeUnit.SECONDS);
            Optional<SensorExecutionIncarnation> second = b.get(15, TimeUnit.SECONDS);
            assertNull(unexpected.get());
            assertEquals(1, List.of(first, second).stream().filter(Optional::isPresent).count());
            assertEquals(1, List.of(first, second).stream().filter(Optional::isEmpty).count());
            SensorExecutionIncarnation winner = first.or(() -> second).orElseThrow();
            assertEquals(winner, repo.findActive(tenant, sensor).orElseThrow());
            assertEquals(1, jdbc.queryForObject("""
                    SELECT COUNT(*) FROM platform.sensor_execution_incarnation
                     WHERE tenant_id = ? AND sensor_id = ?
                    """, Integer.class, tenant, sensor));
        } finally {
            start.countDown();
            workers.shutdownNow();
            assertTrue(workers.awaitTermination(10, TimeUnit.SECONDS));
        }
    }

    @Test
    void concurrentRolloversSerializeUnderPrincipalLockWithoutDuplicateGenerations() throws Exception {
        SensorExecutionIncarnation initial = pg.tx().execute(tx -> repo.issueInitial(tenant, sensor));
        ExecutorService workers = Executors.newFixedThreadPool(2);
        CountDownLatch ready = new CountDownLatch(2);
        CountDownLatch start = new CountDownLatch(1);
        AtomicReference<Throwable> unexpected = new AtomicReference<>();
        try {
            Future<SensorExecutionIncarnation> a = workers.submit(() -> attemptRollover(ready, start, unexpected));
            Future<SensorExecutionIncarnation> b = workers.submit(() -> attemptRollover(ready, start, unexpected));
            assertTrue(ready.await(5, TimeUnit.SECONDS));
            start.countDown();
            SensorExecutionIncarnation first = a.get(15, TimeUnit.SECONDS);
            SensorExecutionIncarnation second = b.get(15, TimeUnit.SECONDS);
            assertNull(unexpected.get());
            assertNotEquals(first.incarnationId(), second.incarnationId());
            assertEquals(Status.SUPERSEDED,
                    repo.find(tenant, sensor, initial.incarnationId()).orElseThrow().status());
            // Principal FOR UPDATE linearizes both rollovers: generations 1→2→3, one ACTIVE.
            assertEquals(3L, repo.findActive(tenant, sensor).orElseThrow().incarnationGeneration());
            assertEquals(3, jdbc.queryForObject("""
                    SELECT COUNT(*) FROM platform.sensor_execution_incarnation
                     WHERE tenant_id = ? AND sensor_id = ?
                    """, Integer.class, tenant, sensor));
            assertEquals(3, jdbc.queryForObject("""
                    SELECT COUNT(DISTINCT incarnation_generation) FROM platform.sensor_execution_incarnation
                     WHERE tenant_id = ? AND sensor_id = ?
                    """, Integer.class, tenant, sensor));
            assertEquals(1, jdbc.queryForObject("""
                    SELECT COUNT(*) FROM platform.sensor_execution_incarnation
                     WHERE tenant_id = ? AND sensor_id = ? AND status = 'ACTIVE'
                    """, Integer.class, tenant, sensor));
        } finally {
            start.countDown();
            workers.shutdownNow();
            assertTrue(workers.awaitTermination(10, TimeUnit.SECONDS));
        }
    }

    @Test
    void databaseRejectsSecondActiveAndGenerationOverflowAndReactivation() {
        SensorExecutionIncarnation issued = pg.tx().execute(tx -> repo.issueInitial(tenant, sensor));

        assertThrows(DataIntegrityViolationException.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_execution_incarnation (
                    incarnation_id, tenant_id, sensor_id, incarnation_generation, status,
                    activated_at, superseded_at, created_at, updated_at)
                VALUES (?, ?, ?, 2, 'ACTIVE', NOW(), NULL, NOW(), NOW())
                """, UUID.randomUUID(), tenant, sensor));

        assertThrows(DataIntegrityViolationException.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_execution_incarnation (
                    incarnation_id, tenant_id, sensor_id, incarnation_generation, status,
                    activated_at, superseded_at, created_at, updated_at)
                VALUES (?, ?, ?, ?, 'SUPERSEDED', NOW(), NOW(), NOW(), NOW())
                """, UUID.randomUUID(), tenant, sensor, ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION + 1L));

        jdbc.update("""
                UPDATE platform.sensor_execution_incarnation
                   SET status = 'SUPERSEDED', superseded_at = NOW(), updated_at = NOW()
                 WHERE incarnation_id = ?
                """, issued.incarnationId());
        DataIntegrityViolationException reactivation = assertThrows(DataIntegrityViolationException.class,
                () -> jdbc.update("""
                        UPDATE platform.sensor_execution_incarnation
                           SET status = 'ACTIVE', superseded_at = NULL, updated_at = NOW()
                         WHERE incarnation_id = ?
                        """, issued.incarnationId()));
        assertTrue(reactivation.getMostSpecificCause().getMessage().contains("trg_sei_protect"));
    }

    @Test
    void sensorDeleteCascadesIncarnationHistoryWithoutTouchingAuditIntentProtections() {
        SensorExecutionIncarnation first = pg.tx().execute(tx -> repo.issueInitial(tenant, sensor));
        pg.tx().executeWithoutResult(tx -> repo.rollover(tenant, sensor));
        assertEquals(2, jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = ? AND sensor_id = ?
                """, Integer.class, tenant, sensor));

        jdbc.update("DELETE FROM platform.sensor WHERE tenant_id = ? AND sensor_id = ?", tenant, sensor);
        assertEquals(0, jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = ? AND sensor_id = ?
                """, Integer.class, tenant, sensor));
        assertTrue(repo.find(tenant, sensor, first.incarnationId()).isEmpty());

        // V017 append-only protection remains intact after V018.
        Exception denyTruncate = assertThrows(Exception.class,
                () -> jdbc.execute("TRUNCATE platform.sensor_poll_grant_audit_intent"));
        assertTrue(flatten(denyTruncate).contains("trg_spgai_deny_mutation"));
    }

    @Test
    void v017ToV018UpgradeLeavesLegacySensorsWithoutFabricatedIncarnation() {
        // Fresh support started at V018 catalogue; prove no seed and explicit issue still works.
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V018'", Integer.class));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_execution_incarnation", Integer.class),
                "V018 must not fabricate incarnation rows");
        assertTrue(repo.findActive(tenant, sensor).isEmpty());
        SensorExecutionIncarnation issued = pg.tx().execute(tx -> repo.issueInitial(tenant, sensor));
        assertEquals(1L, issued.incarnationGeneration());
    }

    @Test
    void maxSafeExhaustionFailsClosedWithoutPartialRollover() {
        UUID incarnationId = UUID.fromString("018f0e0a-0000-7000-8000-0000000002ff");
        jdbc.update("""
                INSERT INTO platform.sensor_execution_incarnation (
                    incarnation_id, tenant_id, sensor_id, incarnation_generation, status,
                    activated_at, superseded_at, created_at, updated_at)
                VALUES (?, ?, ?, ?, 'ACTIVE', NOW(), NULL, NOW(), NOW())
                """, incarnationId, tenant, sensor, ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION);

        SensorControlException exhausted = assertThrows(SensorControlException.class,
                () -> pg.tx().executeWithoutResult(tx -> repo.rollover(tenant, sensor)));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, exhausted.getCode());
        assertTrue(exhausted.getMessage().contains(ControlGenerationLimits.EXHAUSTED_MESSAGE)
                || exhausted.getMessage().contains("MAX_SAFE"));

        SensorExecutionIncarnation stillActive = repo.findActive(tenant, sensor).orElseThrow();
        assertEquals(incarnationId, stillActive.incarnationId());
        assertEquals(ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION, stillActive.incarnationGeneration());
        assertEquals(Status.ACTIVE, stillActive.status());
        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = ? AND sensor_id = ?
                """, Integer.class, tenant, sensor));
    }

    private Optional<SensorExecutionIncarnation> attemptIssue(
            CountDownLatch ready, CountDownLatch start, AtomicReference<Throwable> unexpected) {
        return pg.tx().execute(tx -> {
            ready.countDown();
            await(start);
            try {
                return Optional.of(repo.issueInitial(tenant, sensor));
            } catch (SensorControlException expected) {
                assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, expected.getCode());
                return Optional.empty();
            } catch (RuntimeException unexpectedFailure) {
                unexpected.compareAndSet(null, unexpectedFailure);
                throw unexpectedFailure;
            }
        });
    }

    private SensorExecutionIncarnation attemptRollover(
            CountDownLatch ready, CountDownLatch start, AtomicReference<Throwable> unexpected) {
        return pg.tx().execute(tx -> {
            ready.countDown();
            await(start);
            try {
                return repo.rollover(tenant, sensor);
            } catch (RuntimeException unexpectedFailure) {
                unexpected.compareAndSet(null, unexpectedFailure);
                throw unexpectedFailure;
            }
        });
    }

    private static void await(CountDownLatch start) {
        try {
            assertTrue(start.await(5, TimeUnit.SECONDS));
        } catch (InterruptedException interrupted) {
            Thread.currentThread().interrupt();
            throw new IllegalStateException(interrupted);
        }
    }

    private static String flatten(Throwable failure) {
        StringBuilder out = new StringBuilder();
        for (Throwable cursor = failure; cursor != null; cursor = cursor.getCause()) {
            if (cursor.getMessage() != null) {
                out.append(cursor.getMessage()).append('\n');
            }
        }
        return out.toString();
    }
}
