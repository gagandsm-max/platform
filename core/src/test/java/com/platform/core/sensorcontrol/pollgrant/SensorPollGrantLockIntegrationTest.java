package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.time.Instant;
import java.util.List;
import java.util.UUID;

import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.*;
import static org.junit.jupiter.api.Assertions.*;

@Tag("postgres")
class SensorPollGrantLockIntegrationTest {
    private static final Instant ISSUED = Instant.parse("2026-10-27T01:02:03.123456Z");
    private static final String LOCK_SQL = """
            SELECT grant_id FROM platform.sensor_poll_grant
             WHERE tenant_id = ? AND sensor_id = ? AND grant_id = ?
             FOR NO KEY UPDATE NOWAIT
            """;

    private PostgresTestSupport pg;
    private JdbcTemplate jdbc;
    private SensorPollGrantRepository repo;
    private final UUID tenant = UUID.randomUUID();
    private final UUID otherTenant = UUID.randomUUID();
    private final UUID entity = UUID.randomUUID();
    private final UUID endpoint = UUID.randomUUID();
    private final UUID grant = UUID.randomUUID();
    private final UUID otherGrant = UUID.randomUUID();
    private final String sensor = "lock-fixture-" + UUID.randomUUID();
    private final String otherSensor = "lock-fixture-" + UUID.randomUUID();
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
                        """, id, "grant-lock-fixture-" + id);
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
                    VALUES (?, ?, 'NETWORK_DEVICE', 'grant-lock-fixture', 'ACTIVE', NOW(), NOW(), 'fixture', NOW(), NOW())
                    """, entity, tenant);
            jdbc.update("""
                    INSERT INTO platform.entity_management_endpoint (endpoint_id, tenant_id, entity_id,
                        address_type, address, state, source, is_primary, created_by, created_at, updated_by, updated_at)
                    VALUES (?, ?, ?, 'IPV6', '2001:db8::1', 'ACTIVE', 'MANUAL', TRUE, 'fixture', NOW(), 'fixture', NOW())
                    """, endpoint, tenant, entity);
            for (UUID id : List.of(grant, otherGrant)) {
                // Fixture only: no operational plan, issuer or admission path is established.
                jdbc.update("""
                        INSERT INTO platform.sensor_poll_grant (grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                            grant_generation, assignment_generation, protocol, issued_address_type, issued_address,
                            profile_plan_id, profile_plan_version, issued_at, window_start, window_end, issued_by,
                            status, updated_at)
                        VALUES (?, ?, ?, ?, ?, 9007199254740991, 0, 'SNMP', 'IPV6', '2001:db8::1',
                            'test-only-unresolved-plan', 'fixture-v1', ?, ?, ?, 'fixture-only', 'ISSUED', ?)
                        """, id, tenant, sensor, entity, endpoint, Timestamp.from(ISSUED),
                        Timestamp.from(ISSUED.minusSeconds(1)), Timestamp.from(ISSUED.plusSeconds(60)),
                        Timestamp.from(ISSUED));
            }
        });
        fixtureOwned = true;
    }

    @AfterEach
    void stop() {
        try {
            if (fixtureOwned) {
                pg.tx().executeWithoutResult(tx -> {
                    for (UUID id : List.of(grant, otherGrant)) {
                        jdbc.update("DELETE FROM platform.sensor_poll_grant WHERE tenant_id = ? AND sensor_id = ? AND grant_id = ?",
                                tenant, sensor, id);
                    }
                    jdbc.update("DELETE FROM platform.entity_management_endpoint WHERE tenant_id = ? AND endpoint_id = ?",
                            tenant, endpoint);
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

    @Test
    void exactRowShareLockIsHeldUntilCallerCommit() throws Exception {
        assertHeldUntilCompletion(false);
    }

    @Test
    void exactRowShareLockIsHeldUntilCallerRollback() throws Exception {
        assertHeldUntilCompletion(true);
    }

    private void assertHeldUntilCompletion(boolean rollback) throws Exception {
        // Direct DataSource acquisition deliberately bypasses Spring's thread-bound connection.
        // Completed NOWAIT queries are the evidence; no workers, latches or sleeps are needed.
        try (Connection independent = pg.dataSource().getConnection()) {
            independent.setAutoCommit(false);
            try {
                pg.tx().executeWithoutResult(tx -> {
                    assertDifferentBackend(independent);
                    assertEquals(expectedIssued(grant), repo.findForShare(tenant, sensor, grant).orElseThrow());
                    assertLockConflict(independent, grant);
                    // Query/ResultSet closure and an ordinary lookup must not release the held lock.
                    assertEquals(expectedIssued(grant), repo.find(tenant, sensor, grant).orElseThrow());
                    assertLockConflict(independent, grant);
                    if (rollback) tx.setRollbackOnly();
                });
                assertLockAcquired(independent, grant);
                assertEquals(expectedIssued(grant), repo.find(tenant, sensor, grant).orElseThrow());
            } finally {
                independent.rollback();
            }
        }
    }

    @Test
    void wrongScopeAndMissingGrantDiscloseNothingAndDoNotLockOrMutateExistingRows() throws Exception {
        SensorPollGrant before = repo.find(tenant, sensor, grant).orElseThrow();
        try (Connection independent = pg.dataSource().getConnection()) {
            independent.setAutoCommit(false);
            try {
                pg.tx().executeWithoutResult(tx -> {
                    assertDifferentBackend(independent);
                    assertTrue(repo.findForShare(otherTenant, sensor, grant).isEmpty());
                    assertTrue(repo.findForShare(tenant, otherSensor, grant).isEmpty());
                    assertTrue(repo.findForShare(tenant, sensor, UUID.randomUUID()).isEmpty());
                    assertLockAcquired(independent, grant);
                    assertLockAcquired(independent, otherGrant);
                    assertEquals(before, repo.find(tenant, sensor, grant).orElseThrow());
                });
                assertEquals(before, repo.find(tenant, sensor, grant).orElseThrow());
                assertEquals(expectedIssued(otherGrant), repo.find(tenant, sensor, otherGrant).orElseThrow());
            } finally {
                independent.rollback();
            }
        }
    }

    @Test
    void differentGrantRowsRemainIndependentlyLockable() throws Exception {
        try (Connection independent = pg.dataSource().getConnection()) {
            independent.setAutoCommit(false);
            try {
                pg.tx().executeWithoutResult(tx -> {
                    assertDifferentBackend(independent);
                    assertEquals(expectedIssued(grant), repo.findForShare(tenant, sensor, grant).orElseThrow());
                    assertLockAcquired(independent, otherGrant);
                    assertLockConflict(independent, grant);
                });
            } finally {
                independent.rollback();
            }
        }
    }

    @Test
    void subsequentLockedLookupReturnsCompleteCommittedTerminalRow() throws Exception {
        pg.tx().executeWithoutResult(tx ->
                assertEquals(expectedIssued(grant), repo.findForShare(tenant, sensor, grant).orElseThrow()));

        Instant revoked = ISSUED.plusSeconds(10);
        Instant updated = ISSUED.plusSeconds(11);
        try (Connection independent = pg.dataSource().getConnection()) {
            independent.setAutoCommit(false);
            try {
                try (PreparedStatement statement = independent.prepareStatement("""
                        UPDATE platform.sensor_poll_grant SET status = 'REVOKED', revoked_at = ?,
                            revoked_by = 'security-actor', revocation_class = 'SECURITY',
                            revocation_reason = 'fixture-security-revoke', updated_at = ?
                         WHERE tenant_id = ? AND sensor_id = ? AND grant_id = ?
                        """)) {
                    statement.setQueryTimeout(5);
                    statement.setTimestamp(1, Timestamp.from(revoked));
                    statement.setTimestamp(2, Timestamp.from(updated));
                    statement.setObject(3, tenant);
                    statement.setString(4, sensor);
                    statement.setObject(5, grant);
                    assertEquals(1, statement.executeUpdate());
                }
                independent.commit();
                SensorPollGrant expected = new SensorPollGrant(grant, tenant, sensor, entity, endpoint,
                        9007199254740991L, 0, Protocol.SNMP, AddressType.IPV6, "2001:db8::1",
                        "test-only-unresolved-plan", "fixture-v1", ISSUED, ISSUED.minusSeconds(1),
                        ISSUED.plusSeconds(60), "fixture-only", Status.REVOKED, revoked, "security-actor",
                        RevocationClass.SECURITY, "fixture-security-revoke", updated);
                pg.tx().executeWithoutResult(tx -> {
                    assertDifferentBackend(independent);
                    assertEquals(expected, repo.findForShare(tenant, sensor, grant).orElseThrow());
                    assertEquals(expected, repo.find(tenant, sensor, grant).orElseThrow());
                    assertLockConflict(independent, grant);
                });
                assertLockAcquired(independent, grant);
            } finally {
                independent.rollback();
            }
        }
    }

    private SensorPollGrant expectedIssued(UUID id) {
        return new SensorPollGrant(id, tenant, sensor, entity, endpoint,
                9007199254740991L, 0, Protocol.SNMP, AddressType.IPV6, "2001:db8::1",
                "test-only-unresolved-plan", "fixture-v1", ISSUED, ISSUED.minusSeconds(1),
                ISSUED.plusSeconds(60), "fixture-only", Status.ISSUED, null, null, null, null, ISSUED);
    }

    private void assertDifferentBackend(Connection independent) {
        assertDoesNotThrow(() -> {
            try (PreparedStatement statement = independent.prepareStatement("SELECT pg_backend_pid()")) {
                statement.setQueryTimeout(5);
                try (ResultSet result = statement.executeQuery()) {
                    assertTrue(result.next());
                    assertNotEquals(jdbc.queryForObject("SELECT pg_backend_pid()", Integer.class),
                            Integer.valueOf(result.getInt(1)), "contender must be a different PostgreSQL session");
                }
            }
        });
    }

    private void assertLockConflict(Connection independent, UUID id) {
        try {
            SQLException failure = assertThrows(SQLException.class, () -> lockExactRow(independent, id));
            // FOR KEY SHARE would allow this non-key update lock, so it must fail this test.
            assertEquals("55P03", failure.getSQLState(), "FOR SHARE must conflict with FOR NO KEY UPDATE");
        } finally {
            assertDoesNotThrow(() -> { independent.rollback(); }); // clear the failed transaction before reusing it
        }
    }

    private void assertLockAcquired(Connection independent, UUID id) {
        try {
            assertDoesNotThrow(() -> lockExactRow(independent, id));
        } finally {
            assertDoesNotThrow(() -> { independent.rollback(); });
        }
    }

    private void lockExactRow(Connection independent, UUID id) throws SQLException {
        try (PreparedStatement statement = independent.prepareStatement(LOCK_SQL)) {
            statement.setQueryTimeout(5);
            statement.setObject(1, tenant);
            statement.setString(2, sensor);
            statement.setObject(3, id);
            try (ResultSet result = statement.executeQuery()) {
                assertTrue(result.next(), "exact fixture row must exist");
                assertEquals(id, result.getObject(1, UUID.class));
                assertFalse(result.next());
            }
        }
    }
}
