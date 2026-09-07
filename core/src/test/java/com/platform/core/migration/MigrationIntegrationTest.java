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
                                     'entity_attribute','entity_group_membership','entity_tag','schema_history')
                """,
                Integer.class
        );
        assertEquals(10, tables);

        List<Map<String, Object>> history = jdbc.queryForList(
                "SELECT version, checksum FROM platform.schema_history ORDER BY version"
        );
        assertEquals(1, history.size());
        assertEquals("V001", history.get(0).get("version"));
        String checksum = String.valueOf(history.get(0).get("checksum"));
        assertTrue(checksum.matches("[a-f0-9]{64}"));

        pg.migrate();
        assertEquals(1, jdbc.queryForObject("SELECT COUNT(*) FROM platform.schema_history", Integer.class));

        jdbc.update("UPDATE platform.schema_history SET checksum='deadbeef' WHERE version='V001'");
        MigrationException ex = assertThrows(MigrationException.class, pg::migrate);
        assertEquals("MIGRATION_CHECKSUM_MISMATCH", ex.getCode());
    }

    @Test
    void dayOneVarcharTenantBootstrapUpgradesAndRestartsCleanly() {
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
        jdbc.update("INSERT INTO platform.tenant(tenant_id,name) VALUES ('tenant-default-001','Default Enterprise')");

        pg.migrate();
        assertEquals("uuid", jdbc.queryForObject("""
                SELECT data_type FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='tenant' AND column_name='tenant_id'
                """, String.class));
        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.tenant
                 WHERE tenant_id='018f0e0a-0000-7000-8000-000000000001'::uuid
                """, Integer.class));

        pg.migrate();
        assertEquals(1, jdbc.queryForObject("SELECT COUNT(*) FROM platform.schema_history", Integer.class));
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
