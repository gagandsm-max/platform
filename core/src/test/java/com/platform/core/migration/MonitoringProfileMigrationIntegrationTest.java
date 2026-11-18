package com.platform.core.migration;

import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.postgresql.util.PSQLException;
import org.springframework.dao.DataAccessException;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.jdbc.core.JdbcTemplate;

import java.util.List;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

@Tag("postgres")
class MonitoringProfileMigrationIntegrationTest {

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
    void genuineV022UpgradeAddsEmptyTenantScopedProfileFoundationAndEnforcesInvariants() {
        pg.runner(catalogThroughVersion("V022")).applyPendingMigrations();
        JdbcTemplate jdbc = pg.jdbc();
        assertEquals(22, count("SELECT COUNT(*) FROM platform.schema_history"));

        pg.migrate();
        assertEquals(23, count("SELECT COUNT(*) FROM platform.schema_history"));
        assertEquals(1, count("SELECT COUNT(*) FROM platform.schema_history WHERE version='V023'"));
        assertEquals(0, count("SELECT COUNT(*) FROM platform.monitoring_profile"));
        assertEquals(0, count("SELECT COUNT(*) FROM platform.monitoring_profile_version"));
        assertEquals(0, count("SELECT COUNT(*) FROM platform.monitoring_profile_assignment"));

        UUID tenantA = UUID.randomUUID();
        UUID tenantB = UUID.randomUUID();
        UUID entityA = UUID.randomUUID();
        UUID entityAUnassigned = UUID.randomUUID();
        UUID entityB = UUID.randomUUID();
        UUID profileId = UUID.randomUUID();
        UUID actor = UUID.randomUUID();
        insertTenant(tenantA);
        insertTenant(tenantB);
        insertEntity(tenantA, entityA);
        insertEntity(tenantA, entityAUnassigned);
        insertEntity(tenantB, entityB);
        insertProfile(tenantA, profileId, "Operations");
        insertProfile(tenantB, profileId, "Operations");
        insertVersion(tenantA, profileId, 1, actor);
        insertVersion(tenantA, profileId, 3, actor);
        insertVersion(tenantB, profileId, 1, actor);
        insertVersion(tenantB, profileId, 2, actor);

        jdbc.update("""
                INSERT INTO platform.monitoring_profile_assignment
                    (tenant_id, entity_id, profile_id, profile_version_no, assignment_mode,
                     assigned_by, effective_from, audit_ref)
                VALUES (?::uuid, ?::uuid, ?::uuid, 3, 'EXPLICIT_PROFILE', ?::uuid, NOW(), ?::uuid)
                """, tenantA, entityA, profileId, actor, UUID.randomUUID());

        // Version 2 exists only in tenant B. Use an unassigned tenant-A entity to
        // prove the version FK fails, rather than accidentally hitting the assignment PK.
        assertForeignKeyViolation("fk_monitoring_profile_assignment_version_same_tenant", () -> jdbc.update("""
                INSERT INTO platform.monitoring_profile_assignment
                    (tenant_id, entity_id, profile_id, profile_version_no, assignment_mode,
                     assigned_by, effective_from)
                VALUES (?::uuid, ?::uuid, ?::uuid, 2, 'AUTO_FROM_CRITICALITY', ?::uuid, NOW())
                """, tenantA, entityAUnassigned, profileId, actor));
        // Entity B exists only in tenant B; assert the exact same-tenant entity FK.
        assertForeignKeyViolation("fk_monitoring_profile_assignment_entity_same_tenant", () -> jdbc.update("""
                INSERT INTO platform.monitoring_profile_assignment
                    (tenant_id, entity_id, profile_id, profile_version_no, assignment_mode,
                     assigned_by, effective_from)
                VALUES (?::uuid, ?::uuid, ?::uuid, 1, 'AUTO_FROM_CRITICALITY', ?::uuid, NOW())
                """, tenantA, entityB, profileId, actor));

        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.monitoring_profile_version SET definition='{"changed":true}'::jsonb
                 WHERE tenant_id=?::uuid AND profile_id=?::uuid AND version_no=3
                """, tenantA, profileId));
        // Tenant B's version 2 is unreferenced, so only the immutability trigger
        // (not an assignment FK) can reject this delete.
        DataAccessException deleteDenied = assertThrows(DataAccessException.class, () -> jdbc.update("""
                DELETE FROM platform.monitoring_profile_version
                 WHERE tenant_id=?::uuid AND profile_id=?::uuid AND version_no=2
                """, tenantB, profileId));
        assertImmutableTrigger(deleteDenied);
        // Truncate BOTH referencing and referenced tables to bypass PostgreSQL's
        // ordinary TRUNCATE foreign-key prohibition and exercise our trigger.
        DataAccessException truncateDenied = assertThrows(DataAccessException.class, () -> jdbc.execute("""
                TRUNCATE TABLE platform.monitoring_profile_assignment,
                               platform.monitoring_profile_version
                """));
        assertImmutableTrigger(truncateDenied);
        assertEquals(1, count("SELECT COUNT(*) FROM platform.monitoring_profile_assignment"));
        assertThrows(Exception.class, () -> insertVersion(tenantA, profileId, 2, actor));
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.monitoring_profile_version
                    (tenant_id, profile_id, version_no, definition, content_hash, created_by)
                VALUES (?::uuid, ?::uuid, 4,
                        (('{"payload":"' || repeat('a',65536) || '"}'))::jsonb,
                        decode(repeat('ab',32),'hex'), ?::uuid)
                """, tenantA, profileId, actor));

        jdbc.update("UPDATE platform.entity SET criticality='CRITICAL' WHERE tenant_id=?::uuid AND entity_id=?::uuid",
                tenantA, entityA);
        assertEquals("EXPLICIT_PROFILE", jdbc.queryForObject("""
                SELECT assignment_mode FROM platform.monitoring_profile_assignment
                 WHERE tenant_id=?::uuid AND entity_id=?::uuid
                """, String.class, tenantA, entityA));
        assertEquals(3, jdbc.queryForObject("""
                SELECT profile_version_no FROM platform.monitoring_profile_assignment
                 WHERE tenant_id=?::uuid AND entity_id=?::uuid
                """, Integer.class, tenantA, entityA));
        assertEquals(1, count("SELECT COUNT(*) FROM platform.schema_history WHERE version='V022'"));
        pg.migrate();
        assertEquals(23, count("SELECT COUNT(*) FROM platform.schema_history"));
    }

    private static void assertForeignKeyViolation(
            String expectedConstraint, org.junit.jupiter.api.function.Executable operation) {
        DataIntegrityViolationException error = assertThrows(DataIntegrityViolationException.class, operation);
        PSQLException sql = assertInstanceOf(PSQLException.class, error.getMostSpecificCause());
        assertEquals("23503", sql.getSQLState());
        assertEquals(expectedConstraint, sql.getServerErrorMessage().getConstraint());
    }

    private static void assertImmutableTrigger(DataAccessException error) {
        PSQLException sql = assertInstanceOf(PSQLException.class, error.getMostSpecificCause());
        assertEquals("P0001", sql.getSQLState());
        assertTrue(sql.getMessage().contains("monitoring profile versions are immutable"));
    }

    private void insertTenant(UUID tenantId) {
        pg.jdbc().update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?::uuid, 'Profile tenant', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                """, tenantId);
    }

    private void insertEntity(UUID tenantId, UUID entityId) {
        pg.jdbc().update("""
                INSERT INTO platform.entity
                    (entity_id, tenant_id, entity_type, display_name, lifecycle_state,
                     first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?::uuid, ?::uuid, 'HOST', 'Profile entity', 'ACTIVE', NOW(), NOW(), NOW(), NOW())
                """, entityId, tenantId);
    }

    private void insertProfile(UUID tenantId, UUID profileId, String name) {
        pg.jdbc().update("""
                INSERT INTO platform.monitoring_profile (tenant_id, profile_id, name, created_by)
                VALUES (?::uuid, ?::uuid, ?, ?::uuid)
                """, tenantId, profileId, name, UUID.randomUUID());
    }

    private void insertVersion(UUID tenantId, UUID profileId, int version, UUID actor) {
        pg.jdbc().update("""
                INSERT INTO platform.monitoring_profile_version
                    (tenant_id, profile_id, version_no, definition, content_hash, created_by, capacity_estimate)
                VALUES (?::uuid, ?::uuid, ?, '{"cadence":"bounded"}'::jsonb,
                        decode(repeat('ab',32),'hex'), ?::uuid, '{"estimate":1}'::jsonb)
                """, tenantId, profileId, version, actor);
    }

    private int count(String sql) {
        return pg.jdbc().queryForObject(sql, Integer.class);
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
}
