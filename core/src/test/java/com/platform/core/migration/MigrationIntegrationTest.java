package com.platform.core.migration;

import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;

import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

@Tag("postgres")
class MigrationIntegrationTest {

    private PostgresTestSupport pg;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void blankDatabaseMigratesIdempotentlyWithChecksumProtection() {
        pg.migrate();
        JdbcTemplate jdbc = pg.jdbc();

        Integer tables = jdbc.queryForObject(
                """
                SELECT COUNT(*) FROM information_schema.tables
                WHERE table_schema='platform'
                  AND table_name IN ('tenant','site','entity_group','tag','entity','entity_identifier',
                                     'entity_attribute','entity_group_membership','entity_tag','schema_history',
                                     'sensor_telemetry_watermark','sensor_agent_ingress_watermark',
                                     'discovery_request','discovery_target','discovery_evidence','discovery_candidate',
                                     'sensor','sensor_certificate_credential','sensor_allowed_entity',
                                     'agent','agent_certificate_credential','agent_sensor_assignment',
                                     'tenant_capability_pack','tenant_capability_pack_activation')
                """,
                Integer.class
        );
        assertEquals(24, tables);

        List<Map<String, Object>> history = jdbc.queryForList(
                "SELECT version, checksum FROM platform.schema_history ORDER BY version"
        );
        assertEquals(7, history.size());
        assertEquals("V001", history.get(0).get("version"));
        assertEquals("V002", history.get(1).get("version"));
        assertEquals("V003", history.get(2).get("version"));
        assertEquals("V004", history.get(3).get("version"));
        assertEquals("V005", history.get(4).get("version"));
        assertEquals("V006", history.get(5).get("version"));
        assertEquals("V007", history.get(6).get("version"));
        String checksum = String.valueOf(history.get(0).get("checksum"));
        assertTrue(checksum.matches("[a-f0-9]{64}"));

        pg.migrate();
        assertEquals(7, jdbc.queryForObject("SELECT COUNT(*) FROM platform.schema_history", Integer.class));

        jdbc.update("UPDATE platform.schema_history SET checksum='deadbeef' WHERE version='V001'");
        MigrationException ex = assertThrows(MigrationException.class, pg::migrate);
        assertEquals("MIGRATION_CHECKSUM_MISMATCH", ex.getCode());
    }

    @Test
    void dayOneVarcharTenantBootstrapUpgradesAndRestartsCleanly() {
        JdbcTemplate jdbc = pg.jdbc();

        // 1. Establish complete Day-01 bootstrap environment
        jdbc.execute("""
                CREATE TABLE platform.schema_migration (
                    migration_id SERIAL PRIMARY KEY,
                    version VARCHAR(64) NOT NULL UNIQUE,
                    name VARCHAR(255) NOT NULL,
                    applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    checksum VARCHAR(64) NOT NULL
                );
                CREATE TABLE platform.deployment_metadata (
                    deployment_id VARCHAR(64) PRIMARY KEY,
                    platform_version VARCHAR(32) NOT NULL,
                    cluster_mode VARCHAR(32) NOT NULL DEFAULT 'STANDALONE',
                    active_epoch BIGINT NOT NULL DEFAULT 1,
                    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
                );
                CREATE TABLE platform.tenant (
                    tenant_id VARCHAR(64) PRIMARY KEY,
                    name VARCHAR(255) NOT NULL,
                    mode VARCHAR(32) NOT NULL DEFAULT 'SINGLE',
                    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
                    timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
                    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
                );
                INSERT INTO platform.deployment_metadata (deployment_id, platform_version, cluster_mode, active_epoch)
                VALUES ('platform-dev-deployment-001', '0.1.0-dev', 'STANDALONE', 1);
                INSERT INTO platform.schema_migration (version, name, checksum)
                VALUES ('001_initial_bootstrap', 'Initial bootstrap migration', 'bootstrap-hash-v0.1.0');
                CREATE OR REPLACE VIEW platform.v_health_check AS
                SELECT
                    d.deployment_id,
                    d.platform_version,
                    d.cluster_mode,
                    d.active_epoch,
                    (SELECT COUNT(*) FROM platform.schema_migration) AS migrations_count,
                    CURRENT_TIMESTAMP AS checked_at
                FROM platform.deployment_metadata d;
                """);

        // 2. Insert default row with custom metadata/timestamps plus an existing UUID-format seed
        jdbc.update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES ('tenant-default-001', 'Acme Enterprise', 'SINGLE', 'ACTIVE', 'America/Chicago',
                        '2026-01-15 10:30:00+00'::timestamptz, '2026-02-20 15:45:00+00'::timestamptz)
                """);
        jdbc.update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES ('018f0e0a-0000-7000-8000-00000000beef', 'UUID Enterprise', 'MSP_CHILD', 'ACTIVE', 'UTC',
                        '2026-03-01 08:00:00+00'::timestamptz, '2026-03-01 09:00:00+00'::timestamptz)
                """);

        // 3. Execute Day-02 migration
        pg.migrate();

        // 4. Assert schema migration to normative UUID
        assertEquals("uuid", jdbc.queryForObject("""
                SELECT data_type FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='tenant' AND column_name='tenant_id'
                """, String.class));

        // 5. Assert row count preserved
        assertEquals(2, jdbc.queryForObject("SELECT COUNT(*) FROM platform.tenant", Integer.class));

        // 6. Assert mapped default row preserves all metadata and values
        Map<String, Object> defaultTenant = jdbc.queryForMap("""
                SELECT tenant_id::text AS id, name, mode, status, timezone, created_at, updated_at
                  FROM platform.tenant
                 WHERE tenant_id='018f0e0a-0000-7000-8000-000000000001'::uuid
                """);
        assertEquals("018f0e0a-0000-7000-8000-000000000001", defaultTenant.get("id"));
        assertEquals("Acme Enterprise", defaultTenant.get("name"));
        assertEquals("SINGLE", defaultTenant.get("mode"));
        assertEquals("ACTIVE", defaultTenant.get("status"));
        assertEquals("America/Chicago", defaultTenant.get("timezone"));
        assertNotNull(defaultTenant.get("created_at"));
        assertNotNull(defaultTenant.get("updated_at"));

        // 7. Assert second UUID tenant row preserved
        Map<String, Object> uuidTenant = jdbc.queryForMap("""
                SELECT tenant_id::text AS id, name, mode, status
                  FROM platform.tenant
                 WHERE tenant_id='018f0e0a-0000-7000-8000-00000000beef'::uuid
                """);
        assertEquals("018f0e0a-0000-7000-8000-00000000beef", uuidTenant.get("id"));
        assertEquals("UUID Enterprise", uuidTenant.get("name"));
        assertEquals("MSP_CHILD", uuidTenant.get("mode"));

        // 8. Assert unrelated Day-01 bootstrap artifacts remain intact
        assertEquals(1, jdbc.queryForObject("SELECT COUNT(*) FROM platform.schema_migration", Integer.class));
        assertEquals(1, jdbc.queryForObject("SELECT COUNT(*) FROM platform.deployment_metadata", Integer.class));
        assertEquals(1, jdbc.queryForObject("SELECT migrations_count FROM platform.v_health_check", Integer.class));

        // 9. Assert temporary staging table was dropped without leaving duplicates
        assertNull(jdbc.queryForObject("SELECT to_regclass('platform.tenant_legacy_day01')", String.class));

        // 10. Assert restart is idempotent and does not alter data
        pg.migrate();
        assertEquals(7, jdbc.queryForObject("SELECT COUNT(*) FROM platform.schema_history", Integer.class));
        assertEquals(2, jdbc.queryForObject("SELECT COUNT(*) FROM platform.tenant", Integer.class));

        // 11. V001→V007 upgrade must materialize Sensor/Agent Control and Capability Pack tables
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V004'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V005'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V006'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V007'", Integer.class));
        Integer sensorControlTables = jdbc.queryForObject(
                """
                SELECT COUNT(*) FROM information_schema.tables
                WHERE table_schema='platform'
                  AND table_name IN ('sensor','sensor_certificate_credential','sensor_allowed_entity')
                """,
                Integer.class
        );
        assertEquals(3, sensorControlTables);
        Integer agentControlTables = jdbc.queryForObject(
                """
                SELECT COUNT(*) FROM information_schema.tables
                WHERE table_schema='platform'
                  AND table_name IN ('agent','agent_certificate_credential','agent_sensor_assignment',
                                     'sensor_agent_ingress_watermark')
                """,
                Integer.class
        );
        assertEquals(4, agentControlTables);
        Integer capabilityPackTables = jdbc.queryForObject(
                """
                SELECT COUNT(*) FROM information_schema.tables
                WHERE table_schema='platform'
                  AND table_name IN ('tenant_capability_pack','tenant_capability_pack_activation')
                """,
                Integer.class
        );
        assertEquals(2, capabilityPackTables);
    }

    @Test
    void injectedFailureDuringLegacyTenantUpgradeRollsBackAndReleasesLock() {
        JdbcTemplate jdbc = pg.jdbc();
        jdbc.execute("""
                CREATE TABLE platform.tenant (
                    tenant_id VARCHAR(64) PRIMARY KEY,
                    name VARCHAR(255) NOT NULL,
                    mode VARCHAR(32) NOT NULL DEFAULT 'SINGLE',
                    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
                    timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
                    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """);
        jdbc.update("INSERT INTO platform.tenant(tenant_id, name) VALUES ('tenant-default-001', 'Preserved Enterprise')");

        // Failing migration that attempts table rename and then fails deliberately
        MigrationCatalog failingCatalog = catalog("V910", "failing upgrade", """
                ALTER TABLE platform.tenant RENAME TO tenant_legacy_day01;
                CREATE TABLE platform.tenant (tenant_id UUID PRIMARY KEY, name VARCHAR(255) NOT NULL);
                INSERT INTO platform.table_that_does_not_exist(id) VALUES (1);
                """);

        MigrationException ex = assertThrows(MigrationException.class,
                () -> pg.runner(failingCatalog).applyPendingMigrations());
        assertEquals("MIGRATION_SQL_FAILED", ex.getCode());

        // Assert schema_history does NOT record the failed version
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V910'", Integer.class));

        // Assert rollback restored platform.tenant under its original name and type
        assertEquals("character varying", jdbc.queryForObject("""
                SELECT data_type FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='tenant' AND column_name='tenant_id'
                """, String.class));

        // Assert original legacy data is completely intact
        assertEquals(1, jdbc.queryForObject("SELECT COUNT(*) FROM platform.tenant", Integer.class));
        assertEquals("Preserved Enterprise", jdbc.queryForObject(
                "SELECT name FROM platform.tenant WHERE tenant_id='tenant-default-001'", String.class));

        // Assert temporary table does not exist
        assertNull(jdbc.queryForObject("SELECT to_regclass('platform.tenant_legacy_day01')", String.class));

        // Assert advisory lock is released (can be acquired on fresh connection)
        Boolean lockAcquired = jdbc.queryForObject("SELECT pg_try_advisory_lock(1346982994, 1296650836)", Boolean.class);
        assertTrue(Boolean.TRUE.equals(lockAcquired), "Advisory lock must be released after failure");
        jdbc.execute("SELECT pg_advisory_unlock(1346982994, 1296650836)");

        // Assert running the real migration now succeeds
        pg.migrate();
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V001'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V002'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V003'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V004'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V005'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V006'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V007'", Integer.class));
        assertEquals("uuid", jdbc.queryForObject("""
                SELECT data_type FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='tenant' AND column_name='tenant_id'
                """, String.class));
        assertEquals("Preserved Enterprise", jdbc.queryForObject(
                "SELECT name FROM platform.tenant WHERE tenant_id='018f0e0a-0000-7000-8000-000000000001'::uuid", String.class));
    }

    @Test
    void unsupportedLegacyTenantDataFailsClosedWithoutDataLoss() {
        JdbcTemplate jdbc = pg.jdbc();
        jdbc.execute("""
                CREATE TABLE platform.tenant (
                    tenant_id VARCHAR(64) PRIMARY KEY,
                    name VARCHAR(255) NOT NULL,
                    mode VARCHAR(32) NOT NULL DEFAULT 'SINGLE',
                    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
                    timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
                    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """);
        jdbc.update("INSERT INTO platform.tenant(tenant_id, name) VALUES ('tenant-default-001', 'Default Enterprise')");
        jdbc.update("INSERT INTO platform.tenant(tenant_id, name) VALUES ('corrupted-legacy-tenant-007', 'Corrupted Enterprise')");

        MigrationException ex = assertThrows(MigrationException.class, pg::migrate);
        assertEquals("MIGRATION_SQL_FAILED", ex.getCode());

        // Assert schema_history does NOT record V001
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V001'", Integer.class));

        // Assert platform.tenant is still character varying
        assertEquals("character varying", jdbc.queryForObject("""
                SELECT data_type FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='tenant' AND column_name='tenant_id'
                """, String.class));

        // Assert both original records are completely untouched
        assertEquals(2, jdbc.queryForObject("SELECT COUNT(*) FROM platform.tenant", Integer.class));
        assertEquals("Default Enterprise", jdbc.queryForObject(
                "SELECT name FROM platform.tenant WHERE tenant_id='tenant-default-001'", String.class));
        assertEquals("Corrupted Enterprise", jdbc.queryForObject(
                "SELECT name FROM platform.tenant WHERE tenant_id='corrupted-legacy-tenant-007'", String.class));
    }

    @Test
    void sameConnectionLockAndConcurrentRunnersApplyMigrationOnce() throws Exception {
        String sql = """
                DO $body$
                BEGIN
                    IF NOT EXISTS (
                        SELECT 1 FROM pg_locks
                         WHERE locktype = 'advisory'
                           AND pid = pg_backend_pid()
                           AND classid = 1347179590
                           AND objid = 1296519764
                    ) THEN
                        RAISE EXCEPTION 'migration advisory lock not held by this backend';
                    END IF;
                END
                $body$;
                SELECT pg_sleep(0.5);
                CREATE TABLE platform.concurrent_migration_probe (id INTEGER PRIMARY KEY);
                """;
        MigrationCatalog catalog = catalog("V900", "concurrency proof", sql);
        MigrationRunner first = pg.runner(catalog);
        MigrationRunner second = pg.runner(catalog);
        ExecutorService pool = Executors.newFixedThreadPool(2);
        CountDownLatch ready = new CountDownLatch(2);
        CountDownLatch start = new CountDownLatch(1);
        try {
            Future<?> firstResult = pool.submit(() -> runTogether(first, ready, start));
            Future<?> secondResult = pool.submit(() -> runTogether(second, ready, start));
            assertTrue(ready.await(5, TimeUnit.SECONDS));
            start.countDown();
            firstResult.get(15, TimeUnit.SECONDS);
            secondResult.get(15, TimeUnit.SECONDS);
        } finally {
            pool.shutdownNow();
        }

        assertEquals(1, pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V900'", Integer.class));
        assertEquals(1, pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='platform' AND table_name='concurrent_migration_probe'",
                Integer.class));
    }

    @Test
    void concurrentRunnersAgainstBlankDatabaseSerializeCleanlyRepeatedly() throws Exception {
        MigrationCatalog realCatalog = new MigrationCatalog();
        ExecutorService pool = Executors.newFixedThreadPool(2);
        try {
            for (int iteration = 0; iteration < 10; iteration++) {
                pg.jdbc().execute("DROP SCHEMA IF EXISTS platform CASCADE; CREATE SCHEMA platform;");

                MigrationRunner runner1 = pg.runner(realCatalog);
                MigrationRunner runner2 = pg.runner(realCatalog);

                CountDownLatch ready = new CountDownLatch(2);
                CountDownLatch start = new CountDownLatch(1);

                Future<?> f1 = pool.submit(() -> runTogether(runner1, ready, start));
                Future<?> f2 = pool.submit(() -> runTogether(runner2, ready, start));

                assertTrue(ready.await(5, TimeUnit.SECONDS), "Threads must be ready");
                start.countDown();

                f1.get(20, TimeUnit.SECONDS);
                f2.get(20, TimeUnit.SECONDS);

                Integer historyCount = pg.jdbc().queryForObject(
                        "SELECT COUNT(*) FROM platform.schema_history WHERE version='V001'", Integer.class);
                assertEquals(1, historyCount, "Iteration " + iteration + " must have exactly 1 V001 history row");

                Integer tableCount = pg.jdbc().queryForObject(
                        """
                        SELECT COUNT(*) FROM information_schema.tables
                        WHERE table_schema='platform'
                          AND table_name IN ('tenant','site','entity_group','tag','entity','entity_identifier',
                                             'entity_attribute','entity_group_membership','entity_tag','schema_history')
                        """, Integer.class);
                assertEquals(10, tableCount, "Iteration " + iteration + " must have 10 platform tables");
            }
        } finally {
            pool.shutdownNow();
        }
    }

    @Test
    void failedTransactionalDdlRollsBackHistoryAndReleasesLock() {
        MigrationCatalog failing = catalog("V901", "rollback proof", """
                CREATE TABLE platform.rollback_probe (id INTEGER PRIMARY KEY);
                INSERT INTO platform.rollback_probe(id) VALUES (1);
                INSERT INTO platform.table_that_does_not_exist(id) VALUES (2);
                """);

        MigrationException failure = assertThrows(MigrationException.class,
                () -> pg.runner(failing).applyPendingMigrations());
        assertEquals("MIGRATION_SQL_FAILED", failure.getCode());
        assertNull(pg.jdbc().queryForObject("SELECT to_regclass('platform.rollback_probe')", String.class));
        assertEquals(0, pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V901'", Integer.class));

        MigrationCatalog corrected = catalog("V901", "rollback proof",
                "CREATE TABLE platform.rollback_probe (id INTEGER PRIMARY KEY);");
        pg.runner(corrected).applyPendingMigrations();
        assertEquals(1, pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V901'", Integer.class));
    }

    @Test
    void databaseConstraintsEnforceTenantIsolationAndIdentifierUniqueness() {
        pg.migrate();
        JdbcTemplate jdbc = pg.jdbc();

        jdbc.update("""
                INSERT INTO platform.tenant(tenant_id,name,mode,status,timezone,created_at,updated_at)
                VALUES
                ('018f0e0a-0000-7000-8000-00000000aaa1'::uuid,'A','SINGLE','ACTIVE','UTC',NOW(),NOW()),
                ('018f0e0a-0000-7000-8000-00000000bbb2'::uuid,'B','SINGLE','ACTIVE','UTC',NOW(),NOW())
                """);
        jdbc.update("""
                INSERT INTO platform.site(site_id,tenant_id,parent_site_id,name,description,timezone,created_at,updated_at)
                VALUES
                ('018f0e0a-0000-7000-8000-000000000001'::uuid,'018f0e0a-0000-7000-8000-00000000aaa1'::uuid,NULL,'SiteA',NULL,'UTC',NOW(),NOW()),
                ('018f0e0a-0000-7000-8000-000000000002'::uuid,'018f0e0a-0000-7000-8000-00000000bbb2'::uuid,NULL,'SiteB',NULL,'UTC',NOW(),NOW())
                """);
        jdbc.update("""
                INSERT INTO platform.entity(entity_id,tenant_id,entity_type,display_name,description,discovered_name,
                  lifecycle_state,site_id,criticality,owner_team_id,device_pack_id,device_pack_version,
                  first_seen_at,last_seen_at,created_by,created_at,updated_at)
                VALUES
                ('018f0e0a-0000-7000-8000-000000000011'::uuid,'018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                 'NETWORK_DEVICE','A1',NULL,NULL,'ACTIVE',NULL,NULL,NULL,NULL,NULL,NOW(),NOW(),'test',NOW(),NOW()),
                ('018f0e0a-0000-7000-8000-000000000013'::uuid,'018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                 'NETWORK_DEVICE','A2',NULL,NULL,'ACTIVE',NULL,NULL,NULL,NULL,NULL,NOW(),NOW(),'test',NOW(),NOW()),
                ('018f0e0a-0000-7000-8000-000000000012'::uuid,'018f0e0a-0000-7000-8000-00000000bbb2'::uuid,
                 'NETWORK_DEVICE','B1',NULL,NULL,'ACTIVE',NULL,NULL,NULL,NULL,NULL,NOW(),NOW(),'test',NOW(),NOW())
                """);
        jdbc.update("""
                INSERT INTO platform.entity_group(group_id,tenant_id,parent_group_id,name,description,created_at,updated_at)
                VALUES
                ('018f0e0a-0000-7000-8000-000000000031'::uuid,'018f0e0a-0000-7000-8000-00000000aaa1'::uuid,NULL,'GroupA',NULL,NOW(),NOW()),
                ('018f0e0a-0000-7000-8000-000000000032'::uuid,'018f0e0a-0000-7000-8000-00000000bbb2'::uuid,NULL,'GroupB',NULL,NOW(),NOW())
                """);
        jdbc.update("""
                INSERT INTO platform.tag(tag_id,tenant_id,tag_key,tag_value,created_at)
                VALUES
                ('018f0e0a-0000-7000-8000-000000000041'::uuid,'018f0e0a-0000-7000-8000-00000000aaa1'::uuid,'env','prod',NOW()),
                ('018f0e0a-0000-7000-8000-000000000042'::uuid,'018f0e0a-0000-7000-8000-00000000bbb2'::uuid,'env','prod',NOW()),
                ('018f0e0a-0000-7000-8000-000000000043'::uuid,'018f0e0a-0000-7000-8000-00000000aaa1'::uuid,'label',NULL,NOW())
                """);

        // same IP different tenants OK
        jdbc.update("""
                INSERT INTO platform.entity_identifier(identifier_id,tenant_id,entity_id,identifier_type,namespace,
                  identifier_value,confidence,first_seen_at,last_seen_at)
                VALUES
                ('018f0e0a-0000-7000-8000-000000000021'::uuid,'018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                 '018f0e0a-0000-7000-8000-000000000011'::uuid,'IP','default','10.0.0.1',1.0,NOW(),NOW()),
                ('018f0e0a-0000-7000-8000-000000000022'::uuid,'018f0e0a-0000-7000-8000-00000000bbb2'::uuid,
                 '018f0e0a-0000-7000-8000-000000000012'::uuid,'IP','default','10.0.0.1',1.0,NOW(),NOW())
                """);

        // same tenant duplicate fails
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.entity_identifier(identifier_id,tenant_id,entity_id,identifier_type,namespace,
                  identifier_value,confidence,first_seen_at,last_seen_at)
                VALUES
                ('018f0e0a-0000-7000-8000-000000000023'::uuid,'018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                 '018f0e0a-0000-7000-8000-000000000013'::uuid,'IP','default','10.0.0.1',1.0,NOW(),NOW())
                """));

        // PostgreSQL NULL uniqueness must also reject duplicate label-style tags.
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.tag(tag_id,tenant_id,tag_key,tag_value,created_at)
                VALUES ('018f0e0a-0000-7000-8000-000000000044'::uuid,
                        '018f0e0a-0000-7000-8000-00000000aaa1'::uuid,'label',NULL,NOW())
                """));

        // Every tenant-owned relationship is protected by a same-tenant composite FK.
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.site SET parent_site_id='018f0e0a-0000-7000-8000-000000000002'::uuid
                WHERE site_id='018f0e0a-0000-7000-8000-000000000001'::uuid
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.entity_group SET parent_group_id='018f0e0a-0000-7000-8000-000000000032'::uuid
                WHERE group_id='018f0e0a-0000-7000-8000-000000000031'::uuid
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.entity SET site_id='018f0e0a-0000-7000-8000-000000000002'::uuid
                WHERE entity_id='018f0e0a-0000-7000-8000-000000000011'::uuid
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.entity_identifier(identifier_id,tenant_id,entity_id,identifier_type,namespace,
                  identifier_value,confidence,first_seen_at,last_seen_at)
                VALUES ('018f0e0a-0000-7000-8000-000000000024'::uuid,
                        '018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                        '018f0e0a-0000-7000-8000-000000000012'::uuid,
                        'SERIAL','default','cross-tenant',1.0,NOW(),NOW())
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.entity_attribute(attribute_id,tenant_id,entity_id,schema_key,value_json,source,observed_at)
                VALUES ('018f0e0a-0000-7000-8000-000000000051'::uuid,
                        '018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                        '018f0e0a-0000-7000-8000-000000000012'::uuid,'metadata','{}'::jsonb,'test',NOW())
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.entity_group_membership(tenant_id,entity_id,group_id,created_at)
                VALUES ('018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                        '018f0e0a-0000-7000-8000-000000000011'::uuid,
                        '018f0e0a-0000-7000-8000-000000000032'::uuid,NOW())
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.entity_group_membership(tenant_id,entity_id,group_id,created_at)
                VALUES ('018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                        '018f0e0a-0000-7000-8000-000000000012'::uuid,
                        '018f0e0a-0000-7000-8000-000000000031'::uuid,NOW())
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.entity_tag(tenant_id,entity_id,tag_id,created_at)
                VALUES ('018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                        '018f0e0a-0000-7000-8000-000000000011'::uuid,
                        '018f0e0a-0000-7000-8000-000000000042'::uuid,NOW())
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.entity_tag(tenant_id,entity_id,tag_id,created_at)
                VALUES ('018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                        '018f0e0a-0000-7000-8000-000000000012'::uuid,
                        '018f0e0a-0000-7000-8000-000000000041'::uuid,NOW())
                """));
    }

    private static MigrationCatalog catalog(String version, String description, String sql) {
        MigrationDefinition definition = new MigrationDefinition(
                version, description, "test:" + version, MigrationCatalog.sha256(sql));
        return new MigrationCatalog() {
            @Override
            public List<MigrationDefinition> load() {
                return List.of(definition);
            }

            @Override
            public String loadSql(MigrationDefinition ignored) {
                return sql;
            }
        };
    }

    private static void runTogether(MigrationRunner runner, CountDownLatch ready, CountDownLatch start) {
        ready.countDown();
        try {
            if (!start.await(5, TimeUnit.SECONDS)) {
                throw new IllegalStateException("Concurrent migration start barrier timed out");
            }
            runner.applyPendingMigrations();
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new IllegalStateException("Concurrent migration interrupted", e);
        }
    }
}
