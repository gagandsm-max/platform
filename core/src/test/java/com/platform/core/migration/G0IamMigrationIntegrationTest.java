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

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Focused G0 + IAM catalogue integration source for Issue #249 Cycle 16.
 * Covers the combined sixteen-migration catalogue and genuine V015→V016 upgrade
 * without mixing the PR251 gapped (V015-absent) branch fixture into Production assertions.
 * Execution of these tests is NOT authorized by the source-only dispatch.
 */
@Tag("postgres")
class G0IamMigrationIntegrationTest {

    private static final String[] COMBINED_PLATFORM_TABLES = {
            "tenant", "site", "entity_group", "tag", "entity", "entity_identifier",
            "entity_attribute", "entity_group_membership", "entity_tag", "schema_history",
            "sensor_telemetry_watermark", "sensor_agent_ingress_watermark",
            "discovery_request", "discovery_target", "discovery_evidence", "discovery_candidate",
            "sensor", "sensor_certificate_credential", "sensor_allowed_entity",
            "agent", "agent_certificate_credential", "agent_sensor_assignment",
            "tenant_capability_pack", "tenant_capability_pack_activation",
            "sensor_capability_pack_assignment", "sensor_capability_pack_state",
            "entity_management_endpoint", "sensor_enrollment_authority",
            "sensor_control_state", "sensor_poll_grant",
            "identity_provider_instance", "platform_subject", "identity_mapping",
            "subject_tenant_scope", "permission_grant", "permission_grant_resource"
    };

    private static final String[] IAM_TABLES = {
            "identity_provider_instance", "platform_subject", "identity_mapping",
            "subject_tenant_scope", "permission_grant", "permission_grant_resource"
    };

    private static final String[] IAM_CONSTRAINTS = {
            "chk_identity_provider_type",
            "chk_identity_provider_status",
            "chk_identity_provider_instance_id_format",
            "chk_identity_provider_oidc_trust",
            "chk_platform_subject_status",
            "chk_identity_mapping_provider_type",
            "chk_identity_mapping_external_subject",
            "chk_subject_tenant_scope_status",
            "fk_permission_grant_membership",
            "chk_permission_grant_scope_kind",
            "chk_permission_grant_resource_type",
            "chk_permission_grant_permission_format"
    };

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
    void cleanSixteenMigrationInstall_materializesG0AndIamSchema() {
        pg.migrate();
        JdbcTemplate jdbc = pg.jdbc();

        List<Map<String, Object>> history = jdbc.queryForList(
                "SELECT version FROM platform.schema_history ORDER BY version");
        assertEquals(16, history.size());
        assertEquals("V015", history.get(14).get("version"));
        assertEquals("V016", history.get(15).get("version"));

        assertEquals(36, countNamedPlatformTables(jdbc, COMBINED_PLATFORM_TABLES));
        assertEquals(6, countNamedPlatformTables(jdbc, IAM_TABLES));
        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM information_schema.tables
                 WHERE table_schema='platform' AND table_name='sensor_poll_grant'
                """, Integer.class));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant", Integer.class));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.identity_provider_instance", Integer.class));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.permission_grant", Integer.class));
        assertEquals(12, countNamedConstraints(jdbc, IAM_CONSTRAINTS));

        pg.migrate();
        assertEquals(16, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history", Integer.class));
        assertEquals(36, countNamedPlatformTables(jdbc, COMBINED_PLATFORM_TABLES));
    }

    @Test
    void genuineV015DatabaseWithSeededG0State_upgradesToV016PreservingPollGrantInvariants() {
        JdbcTemplate jdbc = pg.jdbc();

        pg.runner(catalogThroughVersion("V015")).applyPendingMigrations();
        assertEquals(15, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V015'", Integer.class));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V016'", Integer.class));
        assertEquals(0, countNamedPlatformTables(jdbc, IAM_TABLES));

        UUID tenantA = UUID.fromString("018f0e0a-0000-7000-8000-0000000000d1");
        UUID publisherOwner = UUID.fromString("018f0e0a-0000-7000-8000-0000000000d2");
        String sensorId = "sensor-g0-iam-upgrade";
        String pendingChecksum = "ccddeeff00112233445566778899aabbccddeeff00112233445566778899aabb";
        String streamId = "stream-cccccccccccccccccccccccccccccccc";

        jdbc.update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?::uuid, 'G0/IAM Upgrade Tenant', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                """, tenantA);
        jdbc.update("""
                INSERT INTO platform.sensor (
                    sensor_id, tenant_id, site_id, role, status, allow_all_tenant_entities,
                    created_at, updated_at)
                VALUES (?, ?::uuid, NULL, 'NETWORK', 'ACTIVE', FALSE, NOW(), NOW())
                """, sensorId, tenantA);
        jdbc.update("""
                INSERT INTO platform.sensor_telemetry_watermark (
                    tenant_id, sensor_id, acked_through_sequence, last_first_sequence, last_last_sequence,
                    last_checksum, last_batch_id, updated_at, stream_id,
                    pending_first_sequence, pending_last_sequence, pending_checksum, pending_batch_id,
                    pending_reserved_at, pending_publisher_owner, pending_publisher_epoch,
                    pending_publisher_claimed_at, pending_publisher_lease_until,
                    pending_authorization_digest, pending_authorization_version)
                VALUES (?::uuid, ?, 3, 1, 3, 'acked', 'acked-batch',
                        '2026-10-21 12:00:00+00'::timestamptz, ?,
                        4, 8, ?, 'pending-batch',
                        '2026-10-21 12:01:00+00'::timestamptz, ?::uuid, 2,
                        '2026-10-21 12:01:00+00'::timestamptz, '2026-10-21 12:11:00+00'::timestamptz,
                        NULL, NULL)
                """, tenantA, sensorId, streamId, pendingChecksum, publisherOwner);

        Map<String, Object> watermarkBefore = jdbc.queryForMap("""
                SELECT * FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id = ?::uuid AND sensor_id = ?
                """, tenantA, sensorId);
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant", Integer.class));

        pg.migrate();

        assertEquals(16, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V016'", Integer.class));
        assertEquals(6, countNamedPlatformTables(jdbc, IAM_TABLES));
        assertEquals(12, countNamedConstraints(jdbc, IAM_CONSTRAINTS));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_poll_grant", Integer.class),
                "V016 must not seed operational poll grants");
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.permission_grant", Integer.class),
                "V016 must not seed permission grants");

        Map<String, Object> watermarkAfter = jdbc.queryForMap("""
                SELECT * FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id = ?::uuid AND sensor_id = ?
                """, tenantA, sensorId);
        assertEquals(watermarkBefore.get("pending_checksum"), watermarkAfter.get("pending_checksum"));
        assertEquals(watermarkBefore.get("pending_publisher_owner"), watermarkAfter.get("pending_publisher_owner"));
        assertEquals(watermarkBefore.get("pending_publisher_epoch"), watermarkAfter.get("pending_publisher_epoch"));
        assertNull(watermarkAfter.get("pending_authorization_digest"));
        assertNull(watermarkAfter.get("pending_authorization_version"));

        String v016Checksum = jdbc.queryForObject(
                "SELECT checksum FROM platform.schema_history WHERE version='V016'", String.class);
        assertNotNull(v016Checksum);
        assertTrue(v016Checksum.matches("[a-f0-9]{64}"));
    }

    @Test
    void combinedCatalogueRepeatAndV016ChecksumMismatch_remainIdempotentAndFailClosed() {
        pg.migrate();
        JdbcTemplate jdbc = pg.jdbc();
        assertEquals(16, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history", Integer.class));

        pg.migrate();
        assertEquals(16, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V015'", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V016'", Integer.class));

        jdbc.update("UPDATE platform.schema_history SET checksum='deadbeef' WHERE version='V016'");
        MigrationException ex = assertThrows(MigrationException.class, pg::migrate);
        assertEquals("MIGRATION_CHECKSUM_MISMATCH", ex.getCode());
        assertEquals(16, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history", Integer.class),
                "Checksum mismatch must not invent additional history rows");
    }

    @Test
    void controlledFailureAfterV015_leavesTruthfulHistory_thenForwardRecoveryAppliesV016() {
        JdbcTemplate jdbc = pg.jdbc();

        pg.runner(catalogThroughVersion("V015")).applyPendingMigrations();
        assertEquals(15, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history", Integer.class));
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V016'", Integer.class));

        MigrationCatalog failingV016 = catalog("V016", "deliberate IAM failure probe", """
                CREATE TABLE platform.iam_failure_probe (id INTEGER PRIMARY KEY);
                INSERT INTO platform.iam_failure_probe(id) VALUES (1);
                INSERT INTO platform.table_that_does_not_exist(id) VALUES (2);
                """);

        MigrationException failure = assertThrows(MigrationException.class,
                () -> pg.runner(appendAfter(catalogThroughVersion("V015"), failingV016)).applyPendingMigrations());
        assertEquals("MIGRATION_SQL_FAILED", failure.getCode());

        assertEquals(15, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history", Integer.class),
                "Failed V016 must not record a false-success history row");
        assertEquals(0, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V016'", Integer.class));
        assertNull(jdbc.queryForObject("SELECT to_regclass('platform.iam_failure_probe')", String.class));
        assertEquals(0, countNamedPlatformTables(jdbc, IAM_TABLES));
        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM information_schema.tables
                 WHERE table_schema='platform' AND table_name='sensor_poll_grant'
                """, Integer.class));

        Boolean lockAcquired = jdbc.queryForObject(
                "SELECT pg_try_advisory_lock(1346982994, 1296650836)", Boolean.class);
        assertTrue(Boolean.TRUE.equals(lockAcquired), "Advisory lock must be released after failure");
        jdbc.execute("SELECT pg_advisory_unlock(1346982994, 1296650836)");

        // Documented forward recovery: re-run the real combined catalogue; do not claim success from the failed probe.
        pg.migrate();
        assertEquals(16, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history", Integer.class));
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V016'", Integer.class));
        assertEquals(6, countNamedPlatformTables(jdbc, IAM_TABLES));
        assertNull(jdbc.queryForObject("SELECT to_regclass('platform.iam_failure_probe')", String.class));
    }

    private static int countNamedPlatformTables(JdbcTemplate jdbc, String[] names) {
        StringBuilder inList = new StringBuilder();
        for (int i = 0; i < names.length; i++) {
            if (i > 0) {
                inList.append(',');
            }
            inList.append('\'').append(names[i]).append('\'');
        }
        Integer count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='platform' AND table_name IN ("
                        + inList + ")",
                Integer.class);
        return count == null ? 0 : count;
    }

    private static int countNamedConstraints(JdbcTemplate jdbc, String[] names) {
        StringBuilder inList = new StringBuilder();
        for (int i = 0; i < names.length; i++) {
            if (i > 0) {
                inList.append(',');
            }
            inList.append('\'').append(names[i]).append('\'');
        }
        Integer count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM pg_constraint WHERE conname IN (" + inList + ")",
                Integer.class);
        return count == null ? 0 : count;
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

    /**
     * Compose an already-applied prefix catalogue with a single replacement tip version.
     * Used only to inject a controlled failing V016 body while keeping V001–V015 real.
     */
    private static MigrationCatalog appendAfter(MigrationCatalog prefix, MigrationCatalog tip) {
        return new MigrationCatalog() {
            @Override
            public List<MigrationDefinition> load() {
                List<MigrationDefinition> combined = new java.util.ArrayList<>(prefix.load());
                combined.addAll(tip.load());
                return List.copyOf(combined);
            }

            @Override
            public String loadSql(MigrationDefinition definition) {
                for (MigrationDefinition tipDefinition : tip.load()) {
                    if (tipDefinition.version().equals(definition.version())) {
                        return tip.loadSql(definition);
                    }
                }
                return prefix.loadSql(definition);
            }
        };
    }
}
