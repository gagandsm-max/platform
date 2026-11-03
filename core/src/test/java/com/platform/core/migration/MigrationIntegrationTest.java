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
                                     'tenant_capability_pack','tenant_capability_pack_activation',
                                     'sensor_capability_pack_assignment','sensor_capability_pack_state',
                                     'entity_management_endpoint','sensor_enrollment_authority',
                                     'sensor_control_state','sensor_poll_grant','sensor_poll_grant_audit_intent',
                                     'identity_provider_instance','platform_subject','identity_mapping',
                                     'subject_tenant_scope','permission_grant','permission_grant_resource')
                """,
                Integer.class
        );
        assertEquals(37, tables);

        List<Map<String, Object>> history = jdbc.queryForList(
                "SELECT version, checksum FROM platform.schema_history ORDER BY version"
        );
        assertEquals(17, history.size());
        assertEquals("V001", history.get(0).get("version"));
        assertEquals("V002", history.get(1).get("version"));
        assertEquals("V003", history.get(2).get("version"));
        assertEquals("V004", history.get(3).get("version"));
        assertEquals("V005", history.get(4).get("version"));
        assertEquals("V006", history.get(5).get("version"));
        assertEquals("V007", history.get(6).get("version"));
        assertEquals("V008", history.get(7).get("version"));
        assertEquals("V009", history.get(8).get("version"));
        assertEquals("V010", history.get(9).get("version"));
        assertEquals("V011", history.get(10).get("version"));
        assertEquals("V012", history.get(11).get("version"));
        assertEquals("V013", history.get(12).get("version"));
        assertEquals("V014", history.get(13).get("version"));
        assertEquals("V015", history.get(14).get("version"));
        assertEquals("V016", history.get(15).get("version"));
        assertEquals("V017", history.get(16).get("version"));
        String checksum = String.valueOf(history.get(0).get("checksum"));
        assertTrue(checksum.matches("[a-f0-9]{64}"));

        pg.migrate();
        assertEquals(17, jdbc.queryForObject("SELECT COUNT(*) FROM platform.schema_history", Integer.class));

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
        assertEquals(17, jdbc.queryForObject("SELECT COUNT(*) FROM platform.schema_history", Integer.class));
        assertEquals(2, jdbc.queryForObject("SELECT COUNT(*) FROM platform.tenant", Integer.class));

        // 11. V001→V017 combined upgrade must materialize Sensor/Agent Control/Capability Pack,
        // replay-reservation, publisher-lease, entity management-endpoint, enrollment-authority,
        // BUILD-004 control-generation safety, Sensor control desired-state, inert poll-grant ledger,
        // IAM identity/permission-grant directory tables, and inert poll-grant audit intent.
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V004'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V005'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V006'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V007'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V008'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V009'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V010'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V011'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V012'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V013'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V014'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V015'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V016'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V017'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='platform' AND table_name='sensor_control_state'",
                Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='platform' AND table_name='sensor_poll_grant'",
                Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='platform' AND table_name='sensor_poll_grant_audit_intent'",
                Integer.class));
        Integer iamTables = jdbc.queryForObject(
                """
                SELECT COUNT(*) FROM information_schema.tables
                WHERE table_schema='platform'
                  AND table_name IN (
                    'identity_provider_instance','platform_subject','identity_mapping',
                    'subject_tenant_scope','permission_grant','permission_grant_resource')
                """,
                Integer.class
        );
        assertEquals(6, iamTables);
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
                  AND table_name IN ('tenant_capability_pack','tenant_capability_pack_activation',
                                     'sensor_capability_pack_assignment','sensor_capability_pack_state')
                """,
                Integer.class
        );
        assertEquals(4, capabilityPackTables);
        Integer managementEndpointTables = jdbc.queryForObject(
                """
                SELECT COUNT(*) FROM information_schema.tables
                WHERE table_schema='platform'
                  AND table_name IN ('entity_management_endpoint')
                """,
                Integer.class
        );
        assertEquals(1, managementEndpointTables);
        Integer enrollmentAuthorityTables = jdbc.queryForObject(
                """
                SELECT COUNT(*) FROM information_schema.tables
                WHERE table_schema='platform'
                  AND table_name IN ('sensor_enrollment_authority')
                """,
                Integer.class
        );
        assertEquals(1, enrollmentAuthorityTables);
    }

    @Test
    void v013ControlGenerationMaxSafeConstraintsRejectDirectWritesAboveCeiling() {
        pg.migrate();
        JdbcTemplate jdbc = pg.jdbc();

        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V013'", Integer.class));
        Integer maxSafeConstraints = jdbc.queryForObject("""
                SELECT COUNT(*) FROM pg_constraint
                 WHERE conname IN (
                     'chk_discovery_assignment_version_max_safe',
                     'chk_agent_sensor_assignment_generation_max_safe',
                     'chk_tcpa_generation_max_safe',
                     'chk_scps_desired_generation_max_safe',
                     'chk_scps_applied_generation_max_safe'
                 )
                """, Integer.class);
        assertEquals(5, maxSafeConstraints);

        UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c1");
        jdbc.update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?::uuid, 'MaxSafe Tenant', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                """, tenantId);

        long overMax = 9007199254740992L;

        UUID requestId = UUID.fromString("018f0e0a-0000-7000-8000-0000000000d1");
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.discovery_request (
                    request_id, tenant_id, name, status, correlation_id, idempotency_key,
                    assigned_sensor_id, assignment_version, requested_methods, snmp_port,
                    timeout_ms, max_retries, created_by, created_at)
                VALUES (?::uuid, ?::uuid, 'x', 'PENDING', 'c', 'i', NULL, ?,
                        '["ICMP"]'::jsonb, 161, 2000, 1, 't', NOW())
                """, requestId, tenantId, overMax));

        jdbc.update("""
                INSERT INTO platform.agent (agent_id, tenant_id, site_id, status, created_at, updated_at)
                VALUES ('agent-max', ?::uuid, NULL, 'ACTIVE', NOW(), NOW())
                """, tenantId);
        jdbc.update("""
                INSERT INTO platform.sensor (
                    sensor_id, tenant_id, site_id, role, status, allow_all_tenant_entities,
                    created_at, updated_at)
                VALUES ('sensor-max', ?::uuid, NULL, 'INFRASTRUCTURE', 'ACTIVE', FALSE, NOW(), NOW())
                """, tenantId);
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.agent_sensor_assignment (
                    assignment_id, tenant_id, agent_id, sensor_id, generation, status,
                    created_at, updated_at, revoked_at)
                VALUES (?::uuid, ?::uuid, 'agent-max', 'sensor-max', ?, 'ACTIVE',
                        NOW(), NOW(), NULL)
                """, UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1"), tenantId, overMax));

        String sha = "a".repeat(64);
        jdbc.update("""
                INSERT INTO platform.tenant_capability_pack (
                    tenant_id, pack_id, pack_version, schema_version, pack_class, name,
                    publisher_type, publisher_name, certification, manifest_sha256, manifest_bytes,
                    created_by, created_at)
                VALUES (?::uuid, 'pack.max', '1.0.0', '1.0.0', 'DEVICE_NETWORK', 'Max Pack',
                        'PLATFORM', 'Platform', 'NONE', ?, E'\\\\x7b7d', 't', NOW())
                """, tenantId, sha);
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.tenant_capability_pack_activation (
                    activation_id, tenant_id, pack_id, pack_version, generation, status,
                    activated_by, activated_at, revoked_by, revoked_at, correlation_id)
                VALUES (?::uuid, ?::uuid, 'pack.max', '1.0.0', ?, 'ACTIVE',
                        't', NOW(), NULL, NULL, NULL)
                """, UUID.fromString("018f0e0a-0000-7000-8000-0000000000b1"), tenantId, overMax));

        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_capability_pack_state (
                    tenant_id, sensor_id, desired_generation, desired_checksum,
                    applied_generation, applied_checksum, applied_at, updated_by, updated_at)
                VALUES (?::uuid, 'sensor-max', ?, NULL, 0, NULL, NULL, 't', NOW())
                """, tenantId, overMax));

        jdbc.update("""
                INSERT INTO platform.sensor_capability_pack_state (
                    tenant_id, sensor_id, desired_generation, desired_checksum,
                    applied_generation, applied_checksum, applied_at, updated_by, updated_at)
                VALUES (?::uuid, 'sensor-max', 1, NULL, 0, NULL, NULL, 't', NOW())
                """, tenantId);
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_capability_pack_state
                   SET applied_generation = ?, desired_generation = ?
                 WHERE tenant_id = ?::uuid AND sensor_id = 'sensor-max'
                """, overMax, overMax, tenantId));
    }

    @Test
    void v012DirtyControlRowCausesV013ValidationFailureThenSucceedsAfterRepair() {
        JdbcTemplate jdbc = pg.jdbc();

        pg.runner(catalogThroughVersion("V012")).applyPendingMigrations();
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V012'", Integer.class));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V013'", Integer.class));
        assertEquals(0, countBuild004MaxSafeConstraints(jdbc));

        UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-0000000000d2");
        UUID requestId = UUID.fromString("018f0e0a-0000-7000-8000-0000000000d3");
        long dirty = 9007199254740992L;
        jdbc.update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?::uuid, 'Dirty MaxSafe Tenant', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                """, tenantId);
        jdbc.update("""
                INSERT INTO platform.discovery_request (
                    request_id, tenant_id, name, status, correlation_id, idempotency_key,
                    assigned_sensor_id, assignment_version, requested_methods, snmp_port,
                    timeout_ms, max_retries, created_by, created_at)
                VALUES (?::uuid, ?::uuid, 'dirty-fence', 'PENDING', 'corr-dirty', 'idem-dirty', NULL, ?,
                        '["ICMP"]'::jsonb, 161, 2000, 1, 'tester', NOW())
                """, requestId, tenantId, dirty);
        assertEquals(dirty, jdbc.queryForObject("""
                SELECT assignment_version FROM platform.discovery_request WHERE request_id = ?::uuid
                """, Long.class, requestId));

        MigrationException failed = assertThrows(MigrationException.class, pg::migrate);
        assertEquals("MIGRATION_SQL_FAILED", failed.getCode());

        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V013'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V012'", Integer.class));
        assertEquals(dirty, jdbc.queryForObject("""
                SELECT assignment_version FROM platform.discovery_request WHERE request_id = ?::uuid
                """, Long.class, requestId));
        assertEquals(0, countBuild004MaxSafeConstraints(jdbc),
                "Failed V013 must roll back partially added MAX_SAFE constraints");

        jdbc.update("""
                UPDATE platform.discovery_request
                   SET assignment_version = 0
                 WHERE request_id = ?::uuid
                """, requestId);

        pg.migrate();
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V013'", Integer.class));
        assertEquals(5, countBuild004MaxSafeConstraints(jdbc));
        assertEquals(0L, jdbc.queryForObject("""
                SELECT assignment_version FROM platform.discovery_request WHERE request_id = ?::uuid
                """, Long.class, requestId));
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
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V008'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V009'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V010'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V011'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V012'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V013'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V014'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V015'", Integer.class));
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

    @Test
    void v015InertPollGrantSchemaPreservesLegacyPendingAndEnforcesAuthority() {
        JdbcTemplate jdbc = pg.jdbc();

        pg.runner(catalogThroughVersion("V014")).applyPendingMigrations();
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V014'", Integer.class));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V015'", Integer.class));
        assertEquals(0, jdbc.queryForObject("""
                SELECT COUNT(*) FROM information_schema.tables
                 WHERE table_schema='platform' AND table_name='sensor_poll_grant'
                """, Integer.class));

        UUID tenantA = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
        UUID tenantB = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b1");
        UUID publisherOwner = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c1");
        String claimedSensorId = "sensor-claimed-legacy";
        String orphanSensorId = "sensor-orphan-legacy-no-principal";
        String claimedChecksum = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899";
        String orphanChecksum = "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff";
        String claimedStream = "stream-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
        String orphanStream = "stream-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";

        jdbc.update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?::uuid, 'Grant Tenant A', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW()),
                       (?::uuid, 'Grant Tenant B', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                """, tenantA, tenantB);
        jdbc.update("""
                INSERT INTO platform.sensor (
                    sensor_id, tenant_id, site_id, role, status, allow_all_tenant_entities,
                    created_at, updated_at)
                VALUES (?, ?::uuid, NULL, 'NETWORK', 'ACTIVE', FALSE, NOW(), NOW())
                """, claimedSensorId, tenantA);

        // Claimed PENDING with full publisher lease under a real Sensor principal.
        jdbc.update("""
                INSERT INTO platform.sensor_telemetry_watermark (
                    tenant_id, sensor_id, acked_through_sequence, last_first_sequence, last_last_sequence,
                    last_checksum, last_batch_id, updated_at, stream_id,
                    pending_first_sequence, pending_last_sequence, pending_checksum, pending_batch_id,
                    pending_reserved_at, pending_publisher_owner, pending_publisher_epoch,
                    pending_publisher_claimed_at, pending_publisher_lease_until)
                VALUES (?::uuid, ?, 10, 1, 10, 'legacy-acked-checksum', 'acked-batch',
                        '2026-10-01 12:00:00+00'::timestamptz, ?,
                        11, 20, ?, 'pending-batch-claimed',
                        '2026-10-01 12:05:00+00'::timestamptz, ?::uuid, 7,
                        '2026-10-01 12:05:00+00'::timestamptz, '2026-10-01 12:10:00+00'::timestamptz)
                """, tenantA, claimedSensorId, claimedStream, claimedChecksum, publisherOwner);

        // Valid legacy watermark WITHOUT a Sensor principal (sensor_id width/FK not retrofitted).
        jdbc.update("""
                INSERT INTO platform.sensor_telemetry_watermark (
                    tenant_id, sensor_id, acked_through_sequence, last_first_sequence, last_last_sequence,
                    last_checksum, last_batch_id, updated_at, stream_id,
                    pending_first_sequence, pending_last_sequence, pending_checksum, pending_batch_id,
                    pending_reserved_at, pending_publisher_owner, pending_publisher_epoch,
                    pending_publisher_claimed_at, pending_publisher_lease_until)
                VALUES (?::uuid, ?, 0, 0, 0, 'orphan-acked-checksum', NULL,
                        '2026-10-01 11:00:00+00'::timestamptz, ?,
                        1, 5, ?, 'pending-batch-orphan',
                        '2026-10-01 11:01:00+00'::timestamptz, NULL, 0,
                        NULL, NULL)
                """, tenantA, orphanSensorId, orphanStream, orphanChecksum);

        Map<String, Object> claimedBefore = jdbc.queryForMap("""
                SELECT * FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id = ?::uuid AND sensor_id = ?
                """, tenantA, claimedSensorId);
        Map<String, Object> orphanBefore = jdbc.queryForMap("""
                SELECT * FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id = ?::uuid AND sensor_id = ?
                """, tenantA, orphanSensorId);

        // Pre-V016 checkpoint: apply through V015 only (fifteen history rows) before IAM V016.
        pg.runner(catalogThroughVersion("V015")).applyPendingMigrations();

        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V015'", Integer.class));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V016'", Integer.class));
        assertEquals(15, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history", Integer.class));
        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM information_schema.tables
                 WHERE table_schema='platform' AND table_name='sensor_poll_grant'
                """, Integer.class));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant", Integer.class),
                "V015 must seed zero operational grants");

        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='sensor_telemetry_watermark'
                   AND column_name='pending_authorization_digest'
                """, Integer.class));
        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='sensor_telemetry_watermark'
                   AND column_name='pending_authorization_version'
                """, Integer.class));
        assertEquals("character varying", jdbc.queryForObject("""
                SELECT data_type FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='sensor_telemetry_watermark'
                   AND column_name='sensor_id'
                """, String.class));
        assertEquals(255, jdbc.queryForObject("""
                SELECT character_maximum_length FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='sensor_telemetry_watermark'
                   AND column_name='sensor_id'
                """, Integer.class));

        Map<String, Object> claimedAfter = jdbc.queryForMap("""
                SELECT * FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id = ?::uuid AND sensor_id = ?
                """, tenantA, claimedSensorId);
        Map<String, Object> orphanAfter = jdbc.queryForMap("""
                SELECT * FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id = ?::uuid AND sensor_id = ?
                """, tenantA, orphanSensorId);

        assertLegacyWatermarkConserved(claimedBefore, claimedAfter);
        assertLegacyWatermarkConserved(orphanBefore, orphanAfter);
        assertNull(claimedAfter.get("pending_authorization_digest"));
        assertNull(claimedAfter.get("pending_authorization_version"));
        assertNull(orphanAfter.get("pending_authorization_digest"));
        assertNull(orphanAfter.get("pending_authorization_version"));

        // Authorization pair: NULL/NULL remains valid; inconsistent/malformed rejected.
        jdbc.update("""
                UPDATE platform.sensor_telemetry_watermark
                   SET pending_authorization_digest = NULL,
                       pending_authorization_version = NULL
                 WHERE tenant_id = ?::uuid AND sensor_id = ?
                """, tenantA, claimedSensorId);
        Exception authDigestOnly = assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_telemetry_watermark
                   SET pending_authorization_digest = ?,
                       pending_authorization_version = NULL
                 WHERE tenant_id = ?::uuid AND sensor_id = ?
                """, claimedChecksum, tenantA, claimedSensorId));
        assertConstraintViolation(authDigestOnly, "chk_sensor_telemetry_pending_authorization_pair");
        Exception authVersionOnly = assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_telemetry_watermark
                   SET pending_authorization_digest = NULL,
                       pending_authorization_version = 1
                 WHERE tenant_id = ?::uuid AND sensor_id = ?
                """, tenantA, claimedSensorId));
        assertConstraintViolation(authVersionOnly, "chk_sensor_telemetry_pending_authorization_pair");
        Exception authMalformedDigest = assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_telemetry_watermark
                   SET pending_authorization_digest = 'not-a-hex-digest',
                       pending_authorization_version = 1
                 WHERE tenant_id = ?::uuid AND sensor_id = ?
                """, tenantA, claimedSensorId));
        assertConstraintViolation(authMalformedDigest, "chk_sensor_telemetry_pending_authorization_pair");
        jdbc.update("""
                UPDATE platform.sensor_telemetry_watermark
                   SET pending_authorization_digest = ?,
                       pending_authorization_version = 1
                 WHERE tenant_id = ?::uuid AND sensor_id = ?
                """, claimedChecksum, tenantA, claimedSensorId);
        jdbc.update("""
                UPDATE platform.sensor_telemetry_watermark
                   SET pending_authorization_digest = NULL,
                       pending_authorization_version = NULL
                 WHERE tenant_id = ?::uuid AND sensor_id = ?
                """, tenantA, claimedSensorId);

        UUID entityA1 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");
        UUID entityA2 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e2");
        UUID entityB1 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e3");
        UUID endpointA1 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000f1");
        UUID endpointA2 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000f2");
        UUID endpointB1 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000f3");
        UUID grantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000101");

        jdbc.update("""
                INSERT INTO platform.entity (
                    entity_id, tenant_id, entity_type, display_name, description, discovered_name,
                    lifecycle_state, site_id, criticality, owner_team_id, device_pack_id, device_pack_version,
                    first_seen_at, last_seen_at, created_by, created_at, updated_at)
                VALUES
                (?::uuid, ?::uuid, 'NETWORK_DEVICE', 'A1', NULL, NULL, 'ACTIVE', NULL, NULL, NULL, NULL, NULL,
                 NOW(), NOW(), 'test', NOW(), NOW()),
                (?::uuid, ?::uuid, 'NETWORK_DEVICE', 'A2', NULL, NULL, 'ACTIVE', NULL, NULL, NULL, NULL, NULL,
                 NOW(), NOW(), 'test', NOW(), NOW()),
                (?::uuid, ?::uuid, 'NETWORK_DEVICE', 'B1', NULL, NULL, 'ACTIVE', NULL, NULL, NULL, NULL, NULL,
                 NOW(), NOW(), 'test', NOW(), NOW())
                """, entityA1, tenantA, entityA2, tenantA, entityB1, tenantB);
        jdbc.update("""
                INSERT INTO platform.sensor (
                    sensor_id, tenant_id, site_id, role, status, allow_all_tenant_entities,
                    created_at, updated_at)
                VALUES ('sensor-b', ?::uuid, NULL, 'NETWORK', 'ACTIVE', FALSE, NOW(), NOW())
                """, tenantB);
        jdbc.update("""
                INSERT INTO platform.entity_management_endpoint (
                    endpoint_id, tenant_id, entity_id, address_type, address, state, source,
                    is_primary, created_by, created_at, updated_by, updated_at)
                VALUES
                (?::uuid, ?::uuid, ?::uuid, 'IPV4', '10.0.0.10', 'ACTIVE', 'MANUAL', TRUE,
                 'test', NOW(), 'test', NOW()),
                (?::uuid, ?::uuid, ?::uuid, 'IPV4', '10.0.0.11', 'ACTIVE', 'MANUAL', TRUE,
                 'test', NOW(), 'test', NOW()),
                (?::uuid, ?::uuid, ?::uuid, 'IPV4', '10.0.1.10', 'ACTIVE', 'MANUAL', TRUE,
                 'test', NOW(), 'test', NOW())
                """, endpointA1, tenantA, entityA1,
                endpointA2, tenantA, entityA2,
                endpointB1, tenantB, entityB1);

        jdbc.update("""
                INSERT INTO platform.sensor_poll_grant (
                    grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                    grant_generation, assignment_generation, protocol,
                    issued_address_type, issued_address,
                    profile_plan_id, profile_plan_version,
                    issued_at, window_start, window_end, issued_by,
                    status, revoked_at, revoked_by, revocation_class, revocation_reason, updated_at)
                VALUES (
                    ?::uuid, ?::uuid, ?, ?::uuid, ?::uuid,
                    1, 3, 'ICMP',
                    'IPV4', '10.0.0.10',
                    'profile.plan.icmp.default', '1.0.0',
                    '2026-10-17 10:00:00+00'::timestamptz,
                    '2026-10-17 09:00:00+00'::timestamptz,
                    '2026-10-17 11:00:00+00'::timestamptz,
                    'core-issuer-test',
                    'ISSUED', NULL, NULL, NULL, NULL, NOW())
                """, grantId, tenantA, claimedSensorId, entityA1, endpointA1);
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant", Integer.class));

        Exception crossTenantSensor = assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_poll_grant (
                    grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                    grant_generation, assignment_generation, protocol,
                    issued_address_type, issued_address,
                    profile_plan_id, profile_plan_version,
                    issued_at, window_start, window_end, issued_by,
                    status, updated_at)
                VALUES (
                    '018f0e0a-0000-7000-8000-000000000102'::uuid, ?::uuid, 'sensor-b', ?::uuid, ?::uuid,
                    1, 1, 'ICMP', 'IPV4', '10.0.0.10', 'p', '1',
                    '2026-10-17 10:00:00+00'::timestamptz,
                    '2026-10-17 09:00:00+00'::timestamptz,
                    '2026-10-17 11:00:00+00'::timestamptz,
                    't', 'ISSUED', NOW())
                """, tenantA, entityA1, endpointA1));
        assertConstraintViolation(crossTenantSensor, "fk_spg_sensor_same_tenant");

        Exception crossTenantEntity = assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_poll_grant (
                    grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                    grant_generation, assignment_generation, protocol,
                    issued_address_type, issued_address,
                    profile_plan_id, profile_plan_version,
                    issued_at, window_start, window_end, issued_by,
                    status, updated_at)
                VALUES (
                    '018f0e0a-0000-7000-8000-000000000103'::uuid, ?::uuid, ?, ?::uuid, ?::uuid,
                    1, 1, 'ICMP', 'IPV4', '10.0.0.10', 'p', '1',
                    '2026-10-17 10:00:00+00'::timestamptz,
                    '2026-10-17 09:00:00+00'::timestamptz,
                    '2026-10-17 11:00:00+00'::timestamptz,
                    't', 'ISSUED', NOW())
                """, tenantA, claimedSensorId, entityB1, endpointA1));
        assertConstraintViolation(crossTenantEntity, "fk_spg_entity_same_tenant");

        Exception wrongEntityEndpoint = assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_poll_grant (
                    grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                    grant_generation, assignment_generation, protocol,
                    issued_address_type, issued_address,
                    profile_plan_id, profile_plan_version,
                    issued_at, window_start, window_end, issued_by,
                    status, updated_at)
                VALUES (
                    '018f0e0a-0000-7000-8000-000000000104'::uuid, ?::uuid, ?, ?::uuid, ?::uuid,
                    1, 1, 'ICMP', 'IPV4', '10.0.0.10', 'p', '1',
                    '2026-10-17 10:00:00+00'::timestamptz,
                    '2026-10-17 09:00:00+00'::timestamptz,
                    '2026-10-17 11:00:00+00'::timestamptz,
                    't', 'ISSUED', NOW())
                """, tenantA, claimedSensorId, entityA1, endpointA2));
        assertConstraintViolation(wrongEntityEndpoint, "fk_spg_endpoint_same_tenant_entity");

        Exception crossTenantEndpoint = assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_poll_grant (
                    grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                    grant_generation, assignment_generation, protocol,
                    issued_address_type, issued_address,
                    profile_plan_id, profile_plan_version,
                    issued_at, window_start, window_end, issued_by,
                    status, updated_at)
                VALUES (
                    '018f0e0a-0000-7000-8000-000000000105'::uuid, ?::uuid, ?, ?::uuid, ?::uuid,
                    1, 1, 'ICMP', 'IPV4', '10.0.0.10', 'p', '1',
                    '2026-10-17 10:00:00+00'::timestamptz,
                    '2026-10-17 09:00:00+00'::timestamptz,
                    '2026-10-17 11:00:00+00'::timestamptz,
                    't', 'ISSUED', NOW())
                """, tenantA, claimedSensorId, entityA1, endpointB1));
        assertConstraintViolation(crossTenantEndpoint, "fk_spg_endpoint_same_tenant_entity");

        Exception immutableIssued = assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_poll_grant
                   SET issued_address = '10.9.9.9'
                 WHERE grant_id = ?::uuid
                """, grantId));
        assertConstraintViolation(immutableIssued, "trg_spg_protect_issued");

        Exception immutableWindow = assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_poll_grant
                   SET window_end = '2026-10-17 12:00:00+00'::timestamptz
                 WHERE grant_id = ?::uuid
                """, grantId));
        assertConstraintViolation(immutableWindow, "trg_spg_protect_issued");

        jdbc.update("""
                UPDATE platform.sensor_poll_grant
                   SET status = 'REVOKED',
                       revoked_at = '2026-10-17 10:30:00+00'::timestamptz,
                       revoked_by = 'security-officer',
                       revocation_class = 'SECURITY',
                       revocation_reason = 'credential-compromised',
                       updated_at = NOW()
                 WHERE grant_id = ?::uuid
                """, grantId);
        Map<String, Object> revoked = jdbc.queryForMap("""
                SELECT status, issued_address, revocation_class, revoked_by,
                       revoked_at, revocation_reason, updated_at
                  FROM platform.sensor_poll_grant WHERE grant_id = ?::uuid
                """, grantId);
        assertEquals("REVOKED", revoked.get("status"));
        assertEquals("10.0.0.10", revoked.get("issued_address"));
        assertEquals("SECURITY", revoked.get("revocation_class"));
        assertEquals("security-officer", revoked.get("revoked_by"));
        assertEquals("credential-compromised", revoked.get("revocation_reason"));
        assertNotNull(revoked.get("revoked_at"));

        // REVOKED -> ISSUED with all revocation fields cleared must be rejected.
        Exception revokedToIssued = assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_poll_grant
                   SET status = 'ISSUED',
                       revoked_at = NULL,
                       revoked_by = NULL,
                       revocation_class = NULL,
                       revocation_reason = NULL,
                       updated_at = NOW()
                 WHERE grant_id = ?::uuid
                """, grantId));
        assertConstraintViolation(revokedToIssued, "terminal status cannot reactivate");
        Map<String, Object> revokedAfterRejectedReissue = jdbc.queryForMap("""
                SELECT status, issued_address, revocation_class, revoked_by,
                       revoked_at, revocation_reason, updated_at
                  FROM platform.sensor_poll_grant WHERE grant_id = ?::uuid
                """, grantId);
        assertEquals(revoked.get("status"), revokedAfterRejectedReissue.get("status"));
        assertEquals(revoked.get("issued_address"), revokedAfterRejectedReissue.get("issued_address"));
        assertEquals(revoked.get("revocation_class"), revokedAfterRejectedReissue.get("revocation_class"));
        assertEquals(revoked.get("revoked_by"), revokedAfterRejectedReissue.get("revoked_by"));
        assertEquals(revoked.get("revocation_reason"), revokedAfterRejectedReissue.get("revocation_reason"));
        assertEquals(String.valueOf(revoked.get("revoked_at")),
                String.valueOf(revokedAfterRejectedReissue.get("revoked_at")));
        assertEquals(String.valueOf(revoked.get("updated_at")),
                String.valueOf(revokedAfterRejectedReissue.get("updated_at")));

        UUID supersededGrantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000106");
        jdbc.update("""
                INSERT INTO platform.sensor_poll_grant (
                    grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                    grant_generation, assignment_generation, protocol,
                    issued_address_type, issued_address,
                    profile_plan_id, profile_plan_version,
                    issued_at, window_start, window_end, issued_by,
                    status, revoked_at, revoked_by, revocation_class, revocation_reason, updated_at)
                VALUES (
                    ?::uuid, ?::uuid, ?, ?::uuid, ?::uuid,
                    2, 3, 'SNMP',
                    'IPV4', '10.0.0.10',
                    'profile.plan.snmp.default', '1.0.0',
                    '2026-10-17 10:00:00+00'::timestamptz,
                    '2026-10-17 09:00:00+00'::timestamptz,
                    '2026-10-17 11:00:00+00'::timestamptz,
                    'core-issuer-test',
                    'ISSUED', NULL, NULL, NULL, NULL, NOW())
                """, supersededGrantId, tenantA, claimedSensorId, entityA1, endpointA1);

        // ISSUED -> SUPERSEDED positive control.
        jdbc.update("""
                UPDATE platform.sensor_poll_grant
                   SET status = 'SUPERSEDED',
                       revoked_at = '2026-10-17 10:45:00+00'::timestamptz,
                       revoked_by = 'grant-rotator',
                       revocation_class = 'SUPERSEDED',
                       revocation_reason = 'replaced-by-newer-generation',
                       updated_at = NOW()
                 WHERE grant_id = ?::uuid
                """, supersededGrantId);
        Map<String, Object> superseded = jdbc.queryForMap("""
                SELECT status, issued_address, revocation_class, revoked_by,
                       revoked_at, revocation_reason, updated_at
                  FROM platform.sensor_poll_grant WHERE grant_id = ?::uuid
                """, supersededGrantId);
        assertEquals("SUPERSEDED", superseded.get("status"));
        assertEquals("10.0.0.10", superseded.get("issued_address"));
        assertEquals("SUPERSEDED", superseded.get("revocation_class"));
        assertEquals("grant-rotator", superseded.get("revoked_by"));
        assertEquals("replaced-by-newer-generation", superseded.get("revocation_reason"));
        assertNotNull(superseded.get("revoked_at"));

        // SUPERSEDED -> ISSUED with all revocation fields cleared must be rejected.
        Exception supersededToIssued = assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_poll_grant
                   SET status = 'ISSUED',
                       revoked_at = NULL,
                       revoked_by = NULL,
                       revocation_class = NULL,
                       revocation_reason = NULL,
                       updated_at = NOW()
                 WHERE grant_id = ?::uuid
                """, supersededGrantId));
        assertConstraintViolation(supersededToIssued, "terminal status cannot reactivate");
        Map<String, Object> supersededAfterRejectedReissue = jdbc.queryForMap("""
                SELECT status, issued_address, revocation_class, revoked_by,
                       revoked_at, revocation_reason, updated_at
                  FROM platform.sensor_poll_grant WHERE grant_id = ?::uuid
                """, supersededGrantId);
        assertEquals(superseded.get("status"), supersededAfterRejectedReissue.get("status"));
        assertEquals(superseded.get("issued_address"), supersededAfterRejectedReissue.get("issued_address"));
        assertEquals(superseded.get("revocation_class"), supersededAfterRejectedReissue.get("revocation_class"));
        assertEquals(superseded.get("revoked_by"), supersededAfterRejectedReissue.get("revoked_by"));
        assertEquals(superseded.get("revocation_reason"), supersededAfterRejectedReissue.get("revocation_reason"));
        assertEquals(String.valueOf(superseded.get("revoked_at")),
                String.valueOf(supersededAfterRejectedReissue.get("revoked_at")));
        assertEquals(String.valueOf(superseded.get("updated_at")),
                String.valueOf(supersededAfterRejectedReissue.get("updated_at")));

        jdbc.update("""
                UPDATE platform.entity_management_endpoint
                   SET address = '10.0.0.99', updated_at = NOW(), updated_by = 'editor'
                 WHERE endpoint_id = ?::uuid
                """, endpointA1);
        assertEquals("10.0.0.10", jdbc.queryForObject("""
                SELECT issued_address FROM platform.sensor_poll_grant WHERE grant_id = ?::uuid
                """, String.class, grantId));
        assertEquals("10.0.0.99", jdbc.queryForObject("""
                SELECT address FROM platform.entity_management_endpoint WHERE endpoint_id = ?::uuid
                """, String.class, endpointA1));

        // Full catalogue migrate advances the V015 checkpoint through IAM V016 and audit-intent V017.
        pg.migrate();
        assertEquals(17, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V016'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V017'", Integer.class));
        assertEquals(2, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant", Integer.class));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant_audit_intent", Integer.class),
                "V017 must not backfill audit intent for pre-migration grants");
    }

    @Test
    void v017AuditIntentCapturesSameTransactionProvenanceAndSurvivesGrantDeletion() {
        JdbcTemplate jdbc = pg.jdbc();
        pg.migrate();

        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V017'", Integer.class));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant_audit_intent", Integer.class));

        UUID tenantA = UUID.fromString("018f0e0a-0000-7000-8000-000000000171");
        UUID entityA = UUID.fromString("018f0e0a-0000-7000-8000-000000000172");
        UUID endpointA = UUID.fromString("018f0e0a-0000-7000-8000-000000000173");
        UUID grantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000174");
        UUID blockedGrantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000175");
        String sensorId = "sensor-v017-audit-intent";

        jdbc.update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?::uuid, 'V017 Audit Tenant', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                """, tenantA);
        jdbc.update("""
                INSERT INTO platform.sensor (
                    sensor_id, tenant_id, site_id, role, status, allow_all_tenant_entities,
                    created_at, updated_at)
                VALUES (?, ?::uuid, NULL, 'NETWORK', 'ACTIVE', TRUE, NOW(), NOW())
                """, sensorId, tenantA);
        jdbc.update("""
                INSERT INTO platform.entity (
                    entity_id, tenant_id, entity_type, display_name, lifecycle_state,
                    first_seen_at, last_seen_at, created_by, created_at, updated_at)
                VALUES (?::uuid, ?::uuid, 'NETWORK_DEVICE', 'V017', 'ACTIVE',
                        NOW(), NOW(), 'test', NOW(), NOW())
                """, entityA, tenantA);
        jdbc.update("""
                INSERT INTO platform.entity_management_endpoint (
                    endpoint_id, tenant_id, entity_id, address_type, address, state, source,
                    is_primary, created_by, created_at, updated_by, updated_at)
                VALUES (?::uuid, ?::uuid, ?::uuid, 'IPV4', '10.0.0.17', 'ACTIVE', 'MANUAL', TRUE,
                        'test', NOW(), 'test', NOW())
                """, endpointA, tenantA, entityA);

        jdbc.update("""
                INSERT INTO platform.sensor_poll_grant (
                    grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                    grant_generation, assignment_generation, protocol,
                    issued_address_type, issued_address,
                    profile_plan_id, profile_plan_version,
                    issued_at, window_start, window_end, issued_by,
                    status, revoked_at, revoked_by, revocation_class, revocation_reason, updated_at)
                VALUES (
                    ?::uuid, ?::uuid, ?, ?::uuid, ?::uuid,
                    1, 0, 'ICMP',
                    'IPV4', '10.0.0.17',
                    'profile.plan.icmp.default', '1.0.0',
                    '2026-11-03 10:00:00+00'::timestamptz,
                    '2026-11-03 09:00:00+00'::timestamptz,
                    '2026-11-03 11:00:00+00'::timestamptz,
                    'core-issuer-test',
                    'ISSUED', NULL, NULL, NULL, NULL, NOW())
                """, grantId, tenantA, sensorId, entityA, endpointA);

        Map<String, Object> intent = jdbc.queryForMap("""
                SELECT grant_id, tenant_id, recorded_at
                  FROM platform.sensor_poll_grant_audit_intent
                 WHERE grant_id = ?::uuid
                """, grantId);
        assertEquals(grantId, intent.get("grant_id"));
        assertEquals(tenantA, intent.get("tenant_id"));
        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.sensor_poll_grant_audit_intent
                 WHERE grant_id = ?::uuid
                   AND recorded_at = '2026-11-03 10:00:00+00'::timestamptz
                """, Integer.class, grantId),
                "recorded_at must copy NEW.issued_at from the validated grant row");
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant_audit_intent WHERE grant_id = ?::uuid",
                Integer.class, grantId));

        Exception denyUpdate = assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_poll_grant_audit_intent
                   SET tenant_id = '018f0e0a-0000-7000-8000-00000000ffff'::uuid
                 WHERE grant_id = ?::uuid
                """, grantId));
        assertConstraintViolation(denyUpdate, "trg_spgai_deny_mutation");

        Exception denyDelete = assertThrows(Exception.class, () -> jdbc.update("""
                DELETE FROM platform.sensor_poll_grant_audit_intent WHERE grant_id = ?::uuid
                """, grantId));
        assertConstraintViolation(denyDelete, "trg_spgai_deny_mutation");

        // Pre-seeded intent PK conflict must fail the grant INSERT in the same statement/transaction.
        jdbc.update("""
                INSERT INTO platform.sensor_poll_grant_audit_intent (grant_id, tenant_id, recorded_at)
                VALUES (?::uuid, ?::uuid, '2026-11-03 12:00:00+00'::timestamptz)
                """, blockedGrantId, tenantA);
        Exception intentConflict = assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_poll_grant (
                    grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                    grant_generation, assignment_generation, protocol,
                    issued_address_type, issued_address,
                    profile_plan_id, profile_plan_version,
                    issued_at, window_start, window_end, issued_by,
                    status, updated_at)
                VALUES (
                    ?::uuid, ?::uuid, ?, ?::uuid, ?::uuid,
                    2, 0, 'ICMP', 'IPV4', '10.0.0.17', 'p', '1',
                    '2026-11-03 10:00:00+00'::timestamptz,
                    '2026-11-03 09:00:00+00'::timestamptz,
                    '2026-11-03 11:00:00+00'::timestamptz,
                    't', 'ISSUED', NOW())
                """, blockedGrantId, tenantA, sensorId, entityA, endpointA));
        assertTrue(flattenThrowableMessage(intentConflict).toLowerCase().contains("duplicate")
                        || flattenThrowableMessage(intentConflict).contains("sensor_poll_grant_audit_intent"),
                () -> "Expected audit-intent PK conflict, got: " + flattenThrowableMessage(intentConflict));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant WHERE grant_id = ?::uuid",
                Integer.class, blockedGrantId));

        jdbc.update("DELETE FROM platform.sensor_poll_grant WHERE grant_id = ?::uuid", grantId);
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant WHERE grant_id = ?::uuid",
                Integer.class, grantId));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant_audit_intent WHERE grant_id = ?::uuid",
                Integer.class, grantId),
                "Audit intent must survive parent grant deletion");

        jdbc.update("DELETE FROM platform.entity_management_endpoint WHERE endpoint_id = ?::uuid", endpointA);
        jdbc.update("DELETE FROM platform.entity WHERE entity_id = ?::uuid", entityA);
        jdbc.update("DELETE FROM platform.sensor WHERE sensor_id = ?", sensorId);
        jdbc.update("DELETE FROM platform.tenant WHERE tenant_id = ?::uuid", tenantA);
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant_audit_intent WHERE grant_id = ?::uuid",
                Integer.class, grantId),
                "Audit intent must survive test-owned tenant deletion");
        assertEquals(tenantA, jdbc.queryForObject(
                "SELECT tenant_id FROM platform.sensor_poll_grant_audit_intent WHERE grant_id = ?::uuid",
                UUID.class, grantId));
    }

    private static void assertLegacyWatermarkConserved(Map<String, Object> before, Map<String, Object> after) {
        assertEquals(before.get("acked_through_sequence"), after.get("acked_through_sequence"));
        assertEquals(before.get("last_first_sequence"), after.get("last_first_sequence"));
        assertEquals(before.get("last_last_sequence"), after.get("last_last_sequence"));
        assertEquals(before.get("last_checksum"), after.get("last_checksum"));
        assertEquals(before.get("last_batch_id"), after.get("last_batch_id"));
        assertEquals(String.valueOf(before.get("updated_at")), String.valueOf(after.get("updated_at")));
        assertEquals(before.get("stream_id"), after.get("stream_id"));
        assertEquals(before.get("pending_first_sequence"), after.get("pending_first_sequence"));
        assertEquals(before.get("pending_last_sequence"), after.get("pending_last_sequence"));
        assertEquals(before.get("pending_checksum"), after.get("pending_checksum"));
        assertEquals(before.get("pending_batch_id"), after.get("pending_batch_id"));
        assertEquals(String.valueOf(before.get("pending_reserved_at")),
                String.valueOf(after.get("pending_reserved_at")));
        assertEquals(before.get("pending_publisher_owner") == null ? null
                        : String.valueOf(before.get("pending_publisher_owner")),
                after.get("pending_publisher_owner") == null ? null
                        : String.valueOf(after.get("pending_publisher_owner")));
        assertEquals(before.get("pending_publisher_epoch"), after.get("pending_publisher_epoch"));
        assertEquals(String.valueOf(before.get("pending_publisher_claimed_at")),
                String.valueOf(after.get("pending_publisher_claimed_at")));
        assertEquals(String.valueOf(before.get("pending_publisher_lease_until")),
                String.valueOf(after.get("pending_publisher_lease_until")));
    }

    private static void assertConstraintViolation(Throwable error, String constraintName) {
        String message = flattenThrowableMessage(error);
        assertTrue(message.contains(constraintName),
                () -> "Expected constraint '" + constraintName + "' in failure, got: " + message);
    }

    private static String flattenThrowableMessage(Throwable error) {
        StringBuilder sb = new StringBuilder();
        Throwable cursor = error;
        while (cursor != null) {
            if (cursor.getMessage() != null) {
                sb.append(cursor.getMessage()).append('\n');
            }
            cursor = cursor.getCause();
        }
        return sb.toString();
    }

    private static MigrationCatalog catalogThroughVersion(String maxInclusiveVersion) {
        MigrationCatalog real = new MigrationCatalog();
        return new MigrationCatalog() {
            @Override
            public List<MigrationDefinition> load() {
                return real.load().stream()
                        .filter(definition -> definition.version().compareTo(maxInclusiveVersion) <= 0)
                        .toList();
            }

            @Override
            public String loadSql(MigrationDefinition definition) {
                return real.loadSql(definition);
            }
        };
    }

    private static int countBuild004MaxSafeConstraints(JdbcTemplate jdbc) {
        Integer count = jdbc.queryForObject("""
                SELECT COUNT(*) FROM pg_constraint
                 WHERE conname IN (
                     'chk_discovery_assignment_version_max_safe',
                     'chk_agent_sensor_assignment_generation_max_safe',
                     'chk_tcpa_generation_max_safe',
                     'chk_scps_desired_generation_max_safe',
                     'chk_scps_applied_generation_max_safe'
                 )
                """, Integer.class);
        return count == null ? 0 : count;
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
