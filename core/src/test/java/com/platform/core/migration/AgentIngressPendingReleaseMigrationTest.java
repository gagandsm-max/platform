package com.platform.core.migration;

import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;

import java.util.List;
import java.util.Map;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;

@Tag("postgres")
class AgentIngressPendingReleaseMigrationTest {

    private static final UUID TENANT_A = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private static final UUID TENANT_B = UUID.fromString("018f0e0a-0000-7000-8000-000000000099");
    private static final String STREAM = "stream-00112233445566778899aabbccddeeff";
    private static final String CHECKSUM = "a".repeat(64);
    private static final String BODY_SHA = "b".repeat(64);

    private PostgresTestSupport pg;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        // External Compose mode retains its private database between test methods.
        // Each case needs an independent V020 schema, even after a prior V021 case.
        pg.jdbc().execute("DROP SCHEMA IF EXISTS platform CASCADE");
        pg.jdbc().execute("CREATE SCHEMA platform");
        pg.runner(catalogThroughV020()).applyPendingMigrations();
        seedTenant(TENANT_A);
    }

    @AfterEach
    void tearDown() {
        if (pg != null) pg.stop();
    }

    @Test
    void u1CleanV020HistoryMigratesWithoutChangingAcceptedOrSettledState() {
        seedAccepted(TENANT_A, "sensor-a");
        Map<String, Object> before = state(TENANT_A, "sensor-a");

        pg.migrate();

        assertEquals(before, state(TENANT_A, "sensor-a"));
        assertEquals(1, count("SELECT count(*) FROM platform.schema_history WHERE version='V021'"));
        assertEquals(2, count("""
                SELECT count(*) FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='sensor_agent_ingress_watermark'
                   AND column_name IN ('pending_reservation_id','pending_publication_attempted')
                """));
        assertEquals(1, count("""
                SELECT count(*) FROM pg_constraint
                 WHERE conrelid='platform.sensor_agent_ingress_watermark'::regclass
                   AND conname='chk_agent_ingress_pending_release_guard'
                """));
        assertEquals(false, pg.jdbc().queryForObject("""
                SELECT pending_publication_attempted FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id='sensor-a'
                """, Boolean.class, TENANT_A));
        pg.migrate();
        assertEquals(1, count("SELECT count(*) FROM platform.schema_history WHERE version='V021'"));
    }

    @Test
    void u2PendingV020RowBlocksAndRollsBackWithoutTouchingTheRow() {
        seedAccepted(TENANT_A, "sensor-a");
        seedPending(TENANT_A, "sensor-a");
        Map<String, Object> before = state(TENANT_A, "sensor-a");

        MigrationException failure = assertThrows(MigrationException.class, pg::migrate);
        assertEquals("AGENT_INGRESS_PENDING_UPGRADE_BLOCKED", failure.getCode());
        assertEquals("P0211", deepestSqlState(failure));
        assertV021Absent();
        assertEquals(before, state(TENANT_A, "sensor-a"));
    }

    @Test
    void u3PendingInAnotherTenantBlocksTheWholeSchema() {
        seedTenant(TENANT_B);
        seedAccepted(TENANT_A, "sensor-a");
        seedAccepted(TENANT_B, "sensor-b");
        seedPending(TENANT_B, "sensor-b");
        Map<String, Object> aBefore = state(TENANT_A, "sensor-a");
        Map<String, Object> bBefore = state(TENANT_B, "sensor-b");

        MigrationException failure = assertThrows(MigrationException.class, pg::migrate);
        assertEquals("AGENT_INGRESS_PENDING_UPGRADE_BLOCKED", failure.getCode());
        assertV021Absent();
        assertEquals(aBefore, state(TENANT_A, "sensor-a"));
        assertEquals(bBefore, state(TENANT_B, "sensor-b"));
    }

    @Test
    void u4UpgradeSucceedsAfterV020CompatiblePendingFinalization() {
        seedAccepted(TENANT_A, "sensor-a");
        seedPending(TENANT_A, "sensor-a");
        assertEquals("AGENT_INGRESS_PENDING_UPGRADE_BLOCKED",
                assertThrows(MigrationException.class, pg::migrate).getCode());

        // Models the old Core's successful publish-then-finalize transaction.
        // A real upgrade must use the old binary to do this; operators must not edit rows.
        assertEquals(1, pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET acked_through_sequence=3, settled_through_sequence=3,
                       last_first_sequence=3, last_last_sequence=3,
                       last_checksum=pending_checksum, last_batch_id=pending_batch_id,
                       last_request_sha256=pending_request_sha256,
                       pending_first_sequence=NULL, pending_last_sequence=NULL,
                       pending_checksum=NULL, pending_request_sha256=NULL,
                       pending_batch_id=NULL, pending_correlation_id=NULL,
                       pending_reserved_at=NULL,
                       pending_publisher_owner=NULL, pending_publisher_claimed_at=NULL,
                       pending_publisher_lease_until=NULL
                 WHERE tenant_id=? AND sensor_id='sensor-a'
                   AND settled_through_sequence=2 AND pending_first_sequence=3
                   AND pending_publisher_owner IS NULL
                """, TENANT_A));
        assertEquals(0, count("SELECT count(*) FROM platform.sensor_agent_ingress_watermark WHERE pending_first_sequence IS NOT NULL"));

        pg.migrate();
        assertEquals(1, count("SELECT count(*) FROM platform.schema_history WHERE version='V021'"));
        Map<String, Object> finalState = state(TENANT_A, "sensor-a");
        assertEquals(3L, finalState.get("acked_through_sequence"));
        assertEquals(3L, finalState.get("settled_through_sequence"));
        assertNull(finalState.get("pending_first_sequence"));
    }

    private void seedTenant(UUID tenant) {
        pg.jdbc().update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?, 'Migration Test Tenant', 'SINGLE', 'ACTIVE', 'UTC', now(), now())
                ON CONFLICT (tenant_id) DO NOTHING
                """, tenant);
    }

    private void seedAccepted(UUID tenant, String sensor) {
        pg.jdbc().update("""
                INSERT INTO platform.sensor_agent_ingress_watermark (
                    tenant_id, sensor_id, stream_id, acked_through_sequence,
                    settled_through_sequence, last_first_sequence, last_last_sequence,
                    last_checksum, last_batch_id, last_request_sha256)
                VALUES (?, ?, ?, 2, 2, 2, 2, ?, 'accepted-batch', ?)
                """, tenant, sensor, STREAM, CHECKSUM, BODY_SHA);
    }

    private void seedPending(UUID tenant, String sensor) {
        pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_first_sequence=3, pending_last_sequence=3,
                       pending_checksum=?, pending_request_sha256=?,
                       pending_batch_id='pending-batch', pending_correlation_id='pending-corr',
                       pending_reserved_at=clock_timestamp()
                 WHERE tenant_id=? AND sensor_id=?
                """, CHECKSUM, BODY_SHA, tenant, sensor);
    }

    private Map<String, Object> state(UUID tenant, String sensor) {
        return pg.jdbc().queryForMap("""
                SELECT acked_through_sequence, settled_through_sequence,
                       last_first_sequence, last_last_sequence, last_checksum, last_batch_id,
                       last_request_sha256, pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_request_sha256, pending_batch_id,
                       pending_correlation_id, pending_reserved_at, pending_publisher_owner,
                       pending_publisher_claimed_at, pending_publisher_lease_until
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenant, sensor);
    }

    private void assertV021Absent() {
        assertEquals(0, count("SELECT count(*) FROM platform.schema_history WHERE version='V021'"));
        assertEquals(0, count("""
                SELECT count(*) FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='sensor_agent_ingress_watermark'
                   AND column_name IN ('pending_reservation_id','pending_publication_attempted')
                """));
        assertEquals(0, count("""
                SELECT count(*) FROM pg_constraint
                 WHERE conrelid='platform.sensor_agent_ingress_watermark'::regclass
                   AND conname='chk_agent_ingress_pending_release_guard'
                """));
    }

    private int count(String sql) {
        return pg.jdbc().queryForObject(sql, Integer.class);
    }

    private static String deepestSqlState(Throwable error) {
        String state = null;
        for (Throwable current = error; current != null; current = current.getCause()) {
            if (current instanceof java.sql.SQLException sql) state = sql.getSQLState();
        }
        return state;
    }

    private static MigrationCatalog catalogThroughV020() {
        MigrationCatalog real = new MigrationCatalog();
        return new MigrationCatalog() {
            @Override
            public List<MigrationDefinition> load() {
                return real.load().stream().filter(definition ->
                        definition.version().compareTo("V020") <= 0).toList();
            }

            @Override
            public String loadSql(MigrationDefinition definition) {
                return real.loadSql(definition);
            }
        };
    }
}
