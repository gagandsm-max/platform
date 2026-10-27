package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.*;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.jdbc.core.JdbcTemplate;

import java.sql.Timestamp;
import java.time.Instant;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.*;

import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.*;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrantRepository.*;
import static org.junit.jupiter.api.Assertions.*;

@Tag("postgres")
class SensorPollGrantRepositoryIntegrationTest {
    private static final Instant ISSUED = Instant.parse("2026-10-27T01:02:03.123456Z");
    private PostgresTestSupport pg;
    private JdbcTemplate jdbc;
    private SensorPollGrantRepository repo;
    private final UUID tenant = UUID.randomUUID();
    private final UUID otherTenant = UUID.randomUUID();
    private final UUID entity = UUID.randomUUID();
    private final UUID endpoint = UUID.randomUUID();
    private final UUID grant = UUID.randomUUID();
    private final String sensor = "fixture-" + UUID.randomUUID();
    private final String otherSensor = "fixture-" + UUID.randomUUID();
    private boolean fixtureOwned;

    @BeforeEach
    void start() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        jdbc = pg.jdbc();
        repo = new SensorPollGrantRepository(pg.namedJdbc());
        pg.tx().executeWithoutResult(tx -> {
            for (UUID id : List.of(tenant, otherTenant)) {
                jdbc.update("""
                        INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                        VALUES (?, ?, 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                        """, id, "grant-fixture-" + id);
            }
            for (String id : List.of(sensor, otherSensor)) {
                jdbc.update("""
                        INSERT INTO platform.sensor (sensor_id, tenant_id, role, status,
                            allow_all_tenant_entities, created_at, updated_at)
                        VALUES (?, ?, 'NETWORK', 'ACTIVE', FALSE, NOW(), NOW())
                        """, id, tenant);
            }
            jdbc.update("""
                    INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name,
                        lifecycle_state, first_seen_at, last_seen_at, created_by, created_at, updated_at)
                    VALUES (?, ?, 'NETWORK_DEVICE', 'grant-fixture', 'ACTIVE', NOW(), NOW(), 'fixture', NOW(), NOW())
                    """, entity, tenant);
            jdbc.update("""
                    INSERT INTO platform.entity_management_endpoint (endpoint_id, tenant_id, entity_id,
                        address_type, address, state, source, is_primary, created_by, created_at, updated_by, updated_at)
                    VALUES (?, ?, ?, 'IPV6', '2001:db8::1', 'ACTIVE', 'MANUAL', TRUE, 'fixture', NOW(), 'fixture', NOW())
                    """, endpoint, tenant, entity);
            // Test fixture only: this does not establish an operational plan or issuer.
            jdbc.update("""
                    INSERT INTO platform.sensor_poll_grant (grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                        grant_generation, assignment_generation, protocol, issued_address_type, issued_address,
                        profile_plan_id, profile_plan_version, issued_at, window_start, window_end, issued_by,
                        status, updated_at)
                    VALUES (?, ?, ?, ?, ?, 9007199254740991, 0, 'SNMP', 'IPV6', '2001:db8::1',
                        'test-only-unresolved-plan', 'fixture-v1', ?, ?, ?, 'fixture-only', 'ISSUED', ?)
                    """, grant, tenant, sensor, entity, endpoint, Timestamp.from(ISSUED),
                    Timestamp.from(ISSUED.minusSeconds(1)), Timestamp.from(ISSUED.plusSeconds(60)), Timestamp.from(ISSUED));
        });
        fixtureOwned = true;
    }

    @AfterEach
    void stop() {
        try {
            if (fixtureOwned) {
                pg.tx().executeWithoutResult(tx -> {
                    jdbc.update("DELETE FROM platform.sensor_poll_grant WHERE tenant_id = ? AND sensor_id = ? AND grant_id = ?",
                            tenant, sensor, grant);
                    jdbc.update("DELETE FROM platform.entity_management_endpoint WHERE tenant_id = ? AND endpoint_id = ?", tenant, endpoint);
                    jdbc.update("DELETE FROM platform.entity WHERE tenant_id = ? AND entity_id = ?", tenant, entity);
                    for (String id : List.of(sensor, otherSensor)) {
                        jdbc.update("DELETE FROM platform.sensor WHERE tenant_id = ? AND sensor_id = ?", tenant, id);
                    }
                    for (UUID id : List.of(tenant, otherTenant)) {
                        jdbc.update("DELETE FROM platform.tenant WHERE tenant_id = ?", id);
                    }
                });
            }
        } finally {
            if (pg != null) pg.stop();
        }
    }

    private SensorPollGrant row() { return repo.find(tenant, sensor, grant).orElseThrow(); }

    private Termination terminal(Status status) {
        return new Termination(status, ISSUED.plusSeconds(10),
                status == Status.REVOKED ? "revoke-actor" : "supersede-actor",
                status == Status.REVOKED ? RevocationClass.ROUTINE : RevocationClass.SUPERSEDED,
                status == Status.REVOKED ? "routine reason" : "superseded reason");
    }

    private void assertTuple(Termination expected, SensorPollGrant actual) {
        assertEquals(expected.status(), actual.status());
        assertEquals(expected.revokedAt(), actual.revokedAt());
        assertEquals(expected.revokedBy(), actual.revokedBy());
        assertEquals(expected.revocationClass(), actual.revocationClass());
        assertEquals(expected.revocationReason(), actual.revocationReason());
    }

    @Test
    void mapsEveryIssuedColumnAndNullTerminalFields() {
        assertEquals(new SensorPollGrant(grant, tenant, sensor, entity, endpoint,
                9007199254740991L, 0, Protocol.SNMP, AddressType.IPV6, "2001:db8::1",
                "test-only-unresolved-plan", "fixture-v1", ISSUED, ISSUED.minusSeconds(1), ISSUED.plusSeconds(60),
                "fixture-only", Status.ISSUED, null, null, null, null, ISSUED), row());
    }

    @Test
    void wrongTenantSensorAndMissingGrantHaveSameEmptyResultsAndNoMutation() {
        SensorPollGrant before = row();
        assertTrue(repo.find(otherTenant, sensor, grant).isEmpty());
        assertTrue(repo.find(tenant, otherSensor, grant).isEmpty());
        assertTrue(repo.find(tenant, sensor, UUID.randomUUID()).isEmpty());
        assertEquals(TransitionOutcome.NO_PERMITTED_TRANSITION, repo.terminate(otherTenant, sensor, grant, terminal(Status.REVOKED)));
        assertEquals(TransitionOutcome.NO_PERMITTED_TRANSITION, repo.terminate(tenant, otherSensor, grant, terminal(Status.REVOKED)));
        assertEquals(TransitionOutcome.NO_PERMITTED_TRANSITION, repo.terminate(tenant, sensor, UUID.randomUUID(), terminal(Status.REVOKED)));
        assertEquals(before, row());
    }

    @Test
    void missingAuthorityInputsFailClosed() {
        assertThrows(NullPointerException.class, () -> repo.find(null, sensor, grant));
        assertThrows(NullPointerException.class, () -> repo.find(tenant, null, grant));
        assertThrows(NullPointerException.class, () -> repo.find(tenant, sensor, null));
        assertThrows(NullPointerException.class, () -> repo.terminate(null, sensor, grant, terminal(Status.REVOKED)));
        assertThrows(NullPointerException.class, () -> repo.terminate(tenant, null, grant, terminal(Status.REVOKED)));
        assertThrows(NullPointerException.class, () -> repo.terminate(tenant, sensor, null, terminal(Status.REVOKED)));
        assertThrows(NullPointerException.class, () -> repo.terminate(tenant, sensor, grant, null));
        assertEquals(Status.ISSUED, row().status());
    }

    @Test
    void databaseRejectsIncoherentTerminalTimeWithoutChangingRow() {
        SensorPollGrant before = row();
        assertThrows(DataIntegrityViolationException.class, () -> repo.terminate(tenant, sensor, grant,
                new Termination(Status.REVOKED, ISSUED.minusSeconds(1), null, RevocationClass.SECURITY, null)));
        assertEquals(before, row());
    }

    @Test
    void revokedTupleAndUpdatedAtSurviveRepeatedAndConflictingRequests() {
        Termination first = terminal(Status.REVOKED);
        assertEquals(TransitionOutcome.TRANSITIONED, repo.terminate(tenant, sensor, grant, first));
        SensorPollGrant after = row();
        assertTuple(first, after);
        assertTrue(after.updatedAt().isAfter(ISSUED));
        assertEquals(TransitionOutcome.NO_PERMITTED_TRANSITION, repo.terminate(tenant, sensor, grant, first));
        assertEquals(TransitionOutcome.NO_PERMITTED_TRANSITION, repo.terminate(tenant, sensor, grant, terminal(Status.SUPERSEDED)));
        assertEquals(TransitionOutcome.NO_PERMITTED_TRANSITION, repo.terminate(tenant, sensor, grant,
                new Termination(Status.REVOKED, ISSUED.plusSeconds(20), "security-actor", RevocationClass.SECURITY, "escalation")));
        assertEquals(after, row());
    }

    @Test
    void supersededWithNullableActorReasonCannotBeRewritten() {
        Termination first = new Termination(Status.SUPERSEDED, ISSUED.plusSeconds(5), null, RevocationClass.SUPERSEDED, null);
        assertEquals(TransitionOutcome.TRANSITIONED, repo.terminate(tenant, sensor, grant, first));
        SensorPollGrant after = row();
        assertTuple(first, after);
        assertEquals(TransitionOutcome.NO_PERMITTED_TRANSITION, repo.terminate(tenant, sensor, grant, first));
        assertEquals(TransitionOutcome.NO_PERMITTED_TRANSITION, repo.terminate(tenant, sensor, grant, terminal(Status.REVOKED)));
        assertEquals(after, row());
    }

    @Test
    void concurrentConflictingTransitionsCommitOneCompleteWinner() throws Exception {
        ExecutorService workers = Executors.newFixedThreadPool(2);
        CountDownLatch ready = new CountDownLatch(2);
        CountDownLatch start = new CountDownLatch(1);
        List<Termination> requests = List.of(terminal(Status.REVOKED), terminal(Status.SUPERSEDED));
        try {
            List<Future<TransitionOutcome>> results = requests.stream().map(request -> workers.submit(() -> {
                return pg.tx().execute(tx -> {
                    ready.countDown();
                    try {
                        assertTrue(start.await(5, TimeUnit.SECONDS));
                    } catch (InterruptedException interrupted) {
                        Thread.currentThread().interrupt();
                        throw new IllegalStateException(interrupted);
                    }
                    return repo.terminate(tenant, sensor, grant, request);
                });
            })).toList();
            assertTrue(ready.await(5, TimeUnit.SECONDS));
            start.countDown();
            TransitionOutcome a = results.get(0).get(10, TimeUnit.SECONDS);
            TransitionOutcome b = results.get(1).get(10, TimeUnit.SECONDS);
            assertEquals(1, List.of(a, b).stream().filter(r -> r == TransitionOutcome.TRANSITIONED).count());
            assertEquals(1, List.of(a, b).stream().filter(r -> r == TransitionOutcome.NO_PERMITTED_TRANSITION).count());
            assertTuple(requests.get(a == TransitionOutcome.TRANSITIONED ? 0 : 1), row());
        } finally {
            start.countDown();
            workers.shutdownNow();
            assertTrue(workers.awaitTermination(10, TimeUnit.SECONDS));
        }
    }

    @Test
    void callerRollbackRestoresEntireRowAndAllowsAnotherTransition() {
        SensorPollGrant before = row();
        pg.tx().executeWithoutResult(tx -> {
            assertEquals(TransitionOutcome.TRANSITIONED, repo.terminate(tenant, sensor, grant, terminal(Status.REVOKED)));
            assertTuple(terminal(Status.REVOKED), row());
            tx.setRollbackOnly();
        });
        assertEquals(before, row());
        assertEquals(TransitionOutcome.TRANSITIONED, repo.terminate(tenant, sensor, grant, terminal(Status.SUPERSEDED)));
        assertTuple(terminal(Status.SUPERSEDED), row());
    }

    @Test
    void v015StillRejectsIssuanceRewritesAndTerminalReactivation() {
        SensorPollGrant before = row();
        List<String> rewrites = List.of(
                "grant_id = gen_random_uuid()", "tenant_id = '" + otherTenant + "'::uuid",
                "sensor_id = '" + otherSensor + "'", "entity_id = gen_random_uuid()", "endpoint_id = gen_random_uuid()",
                "grant_generation = 2", "assignment_generation = 1", "protocol = 'ICMP'",
                "issued_address_type = 'FQDN'", "issued_address = 'example.invalid'",
                "profile_plan_id = 'another-plan'", "profile_plan_version = 'another-version'",
                "issued_at = issued_at + interval '1 microsecond'", "window_start = window_start - interval '1 second'",
                "window_end = window_end + interval '1 second'", "issued_by = 'another-issuer'");
        for (String rewrite : rewrites) {
            DataIntegrityViolationException failure = assertThrows(DataIntegrityViolationException.class,
                    () -> jdbc.update("UPDATE platform.sensor_poll_grant SET " + rewrite +
                            " WHERE tenant_id = ? AND sensor_id = ? AND grant_id = ?", tenant, sensor, grant));
            assertTrue(failure.getMostSpecificCause().getMessage().contains("trg_spg_protect_issued"));
            assertEquals(before, row());
        }
        assertReactivationDenied(Status.REVOKED);
    }

    @Test
    void v015SupersededCannotReactivate() {
        assertReactivationDenied(Status.SUPERSEDED);
    }

    private void assertReactivationDenied(Status status) {
        repo.terminate(tenant, sensor, grant, terminal(status));
        SensorPollGrant before = row();
        assertThrows(DataIntegrityViolationException.class, () -> jdbc.update("""
                UPDATE platform.sensor_poll_grant SET status = 'ISSUED', revoked_at = NULL,
                    revoked_by = NULL, revocation_class = NULL, revocation_reason = NULL
                 WHERE tenant_id = ? AND sensor_id = ? AND grant_id = ?
                """, tenant, sensor, grant));
        assertEquals(before, row());
    }
}
