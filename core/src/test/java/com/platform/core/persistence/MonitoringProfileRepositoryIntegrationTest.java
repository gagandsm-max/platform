package com.platform.core.persistence;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.AssignmentMode;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

import java.util.List;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

/** Real PostgreSQL coverage for the dormant, read-only BUILD-070 persistence adapter. */
@Tag("postgres")
class MonitoringProfileRepositoryIntegrationTest {

    private PostgresTestSupport pg;
    private MonitoringProfileRepository repository;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        repository = new MonitoringProfileRepository(new NamedParameterJdbcTemplate(pg.dataSource()));
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void tenantScopedReadsDoNotCrossMatchingProfileIdsOrExposeOtherTenantAssignments() throws Exception {
        JdbcTemplate db = pg.jdbc();
        UUID tenantA = UUID.randomUUID();
        UUID tenantB = UUID.randomUUID();
        UUID profileId = UUID.randomUUID();
        UUID entityA = UUID.randomUUID();
        UUID entityB = UUID.randomUUID();
        UUID actor = UUID.randomUUID();
        tenant(db, tenantA);
        tenant(db, tenantB);
        entity(db, tenantA, entityA);
        entity(db, tenantB, entityB);
        profile(db, tenantA, profileId, "Alpha", actor);
        profile(db, tenantB, profileId, "Beta", actor);
        version(db, tenantA, profileId, 1, "{\"tenant\":\"A\"}", actor);
        version(db, tenantB, profileId, 1, "{\"tenant\":\"B\"}", actor);
        version(db, tenantB, profileId, 2, "{\"tenant\":\"B2\"}", actor);
        assign(db, tenantA, entityA, profileId, 1, "EXPLICIT_PROFILE", actor, 7);
        assign(db, tenantB, entityB, profileId, 2, "AUTO_FROM_CRITICALITY", actor, 1);

        assertEquals("Alpha", repository.findProfile(tenantA, profileId).orElseThrow().name());
        assertEquals("Beta", repository.findProfile(tenantB, profileId).orElseThrow().name());
        assertEquals("A", new ObjectMapper().readTree(
                repository.findVersion(tenantA, profileId, 1).orElseThrow().definitionJson())
                .get("tenant").asText());
        assertTrue(repository.findVersion(tenantA, profileId, 2).isEmpty(),
                "Tenant B version 2 cannot be accessed via tenant A");
        assertEquals(1, repository.latestVersion(tenantA, profileId).orElseThrow().versionNo());
        assertEquals(2, repository.latestVersion(tenantB, profileId).orElseThrow().versionNo());
        assertFalse(repository.findVersion(tenantA, profileId, 1).orElseThrow().contentHashHex().isBlank());
        assertEquals(64, repository.findVersion(tenantA, profileId, 1).orElseThrow().contentHashHex().length());
        assertEquals(AssignmentMode.EXPLICIT_PROFILE,
                repository.findAssignment(tenantA, entityA).orElseThrow().mode());
        assertEquals(7, repository.findAssignment(tenantA, entityA).orElseThrow().rowVersion());
        assertTrue(repository.findAssignment(tenantA, entityB).isEmpty());
        assertTrue(repository.findAssignment(tenantB, entityA).isEmpty());
        assertEquals(AssignmentMode.AUTO_FROM_CRITICALITY,
                repository.findAssignment(tenantB, entityB).orElseThrow().mode());

        db.update("UPDATE platform.entity SET criticality='CRITICAL' WHERE tenant_id=?::uuid AND entity_id=?::uuid",
                tenantA, entityA);
        assertEquals(AssignmentMode.EXPLICIT_PROFILE,
                repository.findAssignment(tenantA, entityA).orElseThrow().mode(),
                "Criticality metadata must not replace the explicit profile");
    }

    @Test
    void listIsBoundedDeterministicAndTenantScoped() {
        JdbcTemplate db = pg.jdbc();
        UUID tenantA = UUID.randomUUID();
        UUID tenantB = UUID.randomUUID();
        UUID actor = UUID.randomUUID();
        tenant(db, tenantA);
        tenant(db, tenantB);
        profile(db, tenantA, UUID.randomUUID(), "Zeta", actor);
        profile(db, tenantA, UUID.randomUUID(), "Alpha", actor);
        profile(db, tenantB, UUID.randomUUID(), "Secret", actor);

        List<MonitoringProfileRepository.Profile> page1 = repository.listProfiles(tenantA, 1, 0);
        List<MonitoringProfileRepository.Profile> page2 = repository.listProfiles(tenantA, 1, 1);
        assertEquals("Alpha", page1.get(0).name());
        assertEquals("Zeta", page2.get(0).name());
        assertTrue(repository.listProfiles(tenantA, 2, 2).isEmpty());
        assertEquals("Secret", repository.listProfiles(tenantB, 10, 0).get(0).name());
        assertThrows(IllegalArgumentException.class, () -> repository.listProfiles(tenantA, 0, 0));
        assertThrows(IllegalArgumentException.class, () -> repository.listProfiles(tenantA, 201, 0));
        assertThrows(IllegalArgumentException.class, () -> repository.listProfiles(tenantA, 1, -1));
        assertThrows(NullPointerException.class, () -> repository.findProfile(null, UUID.randomUUID()));
        assertThrows(IllegalArgumentException.class, () -> repository.findVersion(tenantA, UUID.randomUUID(), 0));
    }

    private static void tenant(JdbcTemplate db, UUID tenantId) {
        db.update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?::uuid, 'Read model tenant', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                """, tenantId);
    }

    private static void entity(JdbcTemplate db, UUID tenantId, UUID entityId) {
        db.update("""
                INSERT INTO platform.entity
                    (entity_id, tenant_id, entity_type, display_name, lifecycle_state,
                     first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?::uuid, ?::uuid, 'HOST', 'Read model host', 'ACTIVE', NOW(), NOW(), NOW(), NOW())
                """, entityId, tenantId);
    }

    private static void profile(JdbcTemplate db, UUID tenantId, UUID profileId, String name, UUID actor) {
        db.update("""
                INSERT INTO platform.monitoring_profile (tenant_id, profile_id, name, created_by)
                VALUES (?::uuid, ?::uuid, ?, ?::uuid)
                """, tenantId, profileId, name, actor);
    }

    private static void version(JdbcTemplate db, UUID tenantId, UUID profileId,
                                int versionNo, String definition, UUID actor) {
        db.update("""
                INSERT INTO platform.monitoring_profile_version
                    (tenant_id, profile_id, version_no, definition, content_hash, capacity_estimate, created_by)
                VALUES (?::uuid, ?::uuid, ?, ?::jsonb, decode(repeat('ab',32),'hex'),
                        '{"estimate":1}'::jsonb, ?::uuid)
                """, tenantId, profileId, versionNo, definition, actor);
    }

    private static void assign(JdbcTemplate db, UUID tenantId, UUID entityId, UUID profileId,
                               int versionNo, String mode, UUID actor, long rowVersion) {
        db.update("""
                INSERT INTO platform.monitoring_profile_assignment
                    (tenant_id, entity_id, profile_id, profile_version_no, assignment_mode,
                     assigned_by, effective_from, row_version)
                VALUES (?::uuid, ?::uuid, ?::uuid, ?, ?, ?::uuid, NOW(), ?)
                """, tenantId, entityId, profileId, versionNo, mode, actor, rowVersion);
    }
}
