package com.platform.core.persistence;

import com.platform.core.domain.monitoring.MonitoringProfileSelection.AssignmentMode;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

/** Real PostgreSQL coverage for the inert BUILD-070 assignment mutation primitives. */
@Tag("postgres")
class MonitoringProfileAssignmentMutationIntegrationTest {

    private PostgresTestSupport pg;
    private MonitoringProfileAssignmentMutationRepository mutations;
    private MonitoringProfileRepository reads;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        NamedParameterJdbcTemplate named = new NamedParameterJdbcTemplate(pg.dataSource());
        mutations = new MonitoringProfileAssignmentMutationRepository(named);
        reads = new MonitoringProfileRepository(named);
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void insertIsNoClobberAndPreservesActorAuditAndVersion() {
        Fixture f = fixture();
        assertEquals(1, insert(f, f.profileId, 1, AssignmentMode.EXPLICIT_PROFILE));
        MonitoringProfileRepository.Assignment saved = reads.findAssignment(f.tenantId, f.entityId).orElseThrow();
        assertEquals(1, saved.rowVersion());
        assertEquals(f.actorId, saved.assignedBy());
        assertEquals(f.auditRef, saved.auditRef());
        assertEquals(f.profileId, saved.profileId());
        assertEquals(0, mutations.insertIfAbsent(f.tenantId, f.entityId, f.otherProfileId, 1,
                AssignmentMode.AUTO_FROM_CRITICALITY, f.otherActorId, f.otherAuditRef));
        assertEquals(saved, reads.findAssignment(f.tenantId, f.entityId).orElseThrow());
    }

    @Test
    void matchingCasIncrementsAndStaleCasDoesNotMutate() {
        Fixture f = fixture();
        insert(f, f.profileId, 1, AssignmentMode.AUTO_FROM_CRITICALITY);
        assertEquals(0, update(f, f.otherProfileId, 1, AssignmentMode.AUTO_FROM_CRITICALITY, 9));
        assertEquals(1, update(f, f.otherProfileId, 1, AssignmentMode.AUTO_FROM_CRITICALITY, 1));
        MonitoringProfileRepository.Assignment saved = reads.findAssignment(f.tenantId, f.entityId).orElseThrow();
        assertEquals(2, saved.rowVersion());
        assertEquals(f.otherProfileId, saved.profileId());
        assertEquals(f.otherActorId, saved.assignedBy());
        assertEquals(f.otherAuditRef, saved.auditRef());
    }

    @Test
    void explicitAssignmentCannotBeReplacedByAutomaticSuggestion() {
        Fixture f = fixture();
        insert(f, f.profileId, 1, AssignmentMode.EXPLICIT_PROFILE);
        assertEquals(0, update(f, f.otherProfileId, 1, AssignmentMode.AUTO_FROM_CRITICALITY, 1));
        MonitoringProfileRepository.Assignment saved = reads.findAssignment(f.tenantId, f.entityId).orElseThrow();
        assertEquals(AssignmentMode.EXPLICIT_PROFILE, saved.mode());
        assertEquals(f.profileId, saved.profileId());
        assertEquals(1, saved.rowVersion());
        assertEquals(1, update(f, f.otherProfileId, 1, AssignmentMode.EXPLICIT_PROFILE, 1));
        assertEquals(AssignmentMode.EXPLICIT_PROFILE,
                reads.findAssignment(f.tenantId, f.entityId).orElseThrow().mode());
    }

    @Test
    void tenantEntityAndProfileVersionForeignKeysFailClosed() {
        Fixture f = fixture();
        UUID otherTenant = UUID.randomUUID();
        tenant(otherTenant);
        entity(otherTenant, UUID.randomUUID());
        assertThrows(RuntimeException.class, () -> mutations.insertIfAbsent(otherTenant, f.entityId,
                f.profileId, 1, AssignmentMode.EXPLICIT_PROFILE, f.actorId, f.auditRef));
        assertThrows(RuntimeException.class, () -> mutations.insertIfAbsent(f.tenantId, UUID.randomUUID(),
                f.profileId, 1, AssignmentMode.EXPLICIT_PROFILE, f.actorId, f.auditRef));
        assertThrows(RuntimeException.class, () -> insert(f, f.profileId, 2, AssignmentMode.EXPLICIT_PROFILE));
        assertEquals(0, mutations.updateIfVersion(f.tenantId, UUID.randomUUID(), f.profileId, 1,
                AssignmentMode.EXPLICIT_PROFILE, f.actorId, f.auditRef, 1));
        assertEquals(0, pg.jdbc().queryForObject("SELECT COUNT(*) FROM platform.monitoring_profile_assignment "
                + "WHERE tenant_id=?::uuid", Integer.class, f.tenantId));
    }

    @Test
    void nullIdsAndInvalidProfileVersionAreRejectedBeforeMutation() {
        Fixture f = fixture();
        assertThrows(NullPointerException.class, () -> mutations.insertIfAbsent(null, f.entityId,
                f.profileId, 1, AssignmentMode.EXPLICIT_PROFILE, f.actorId, f.auditRef));
        assertThrows(NullPointerException.class, () -> mutations.insertIfAbsent(f.tenantId, null,
                f.profileId, 1, AssignmentMode.EXPLICIT_PROFILE, f.actorId, f.auditRef));
        assertThrows(NullPointerException.class, () -> mutations.insertIfAbsent(f.tenantId, f.entityId,
                null, 1, AssignmentMode.EXPLICIT_PROFILE, f.actorId, f.auditRef));
        assertThrows(NullPointerException.class, () -> mutations.insertIfAbsent(f.tenantId, f.entityId,
                f.profileId, 1, AssignmentMode.EXPLICIT_PROFILE, null, f.auditRef));
        assertThrows(NullPointerException.class, () -> mutations.insertIfAbsent(f.tenantId, f.entityId,
                f.profileId, 1, AssignmentMode.EXPLICIT_PROFILE, f.actorId, null));
        assertThrows(IllegalArgumentException.class, () -> mutations.insertIfAbsent(f.tenantId, f.entityId,
                f.profileId, 0, AssignmentMode.EXPLICIT_PROFILE, f.actorId, f.auditRef));
        assertEquals(0, pg.jdbc().queryForObject("SELECT COUNT(*) FROM platform.monitoring_profile_assignment "
                + "WHERE tenant_id=?::uuid", Integer.class, f.tenantId));
    }

    @Test
    void concurrentWritersAtSameExpectedVersionHaveExactlyOneWinner() throws Exception {
        Fixture f = fixture();
        insert(f, f.profileId, 1, AssignmentMode.AUTO_FROM_CRITICALITY);
        CountDownLatch ready = new CountDownLatch(2);
        CountDownLatch start = new CountDownLatch(1);
        ExecutorService pool = Executors.newFixedThreadPool(2);
        try {
            Future<Integer> first = pool.submit(() -> raceUpdate(f, f.profileId, ready, start));
            Future<Integer> second = pool.submit(() -> raceUpdate(f, f.otherProfileId, ready, start));
            ready.await();
            start.countDown();
            assertEquals(1, first.get() + second.get());
            assertEquals(2, reads.findAssignment(f.tenantId, f.entityId).orElseThrow().rowVersion());
        } finally {
            pool.shutdownNow();
        }
    }

    @Test
    void rowVersionOverflowIsRejectedAndReferencedProfileVersionStaysImmutable() throws Exception {
        Fixture f = fixture();
        insert(f, f.profileId, 1, AssignmentMode.AUTO_FROM_CRITICALITY);
        version(f.tenantId, f.profileId, 2, "{\"definition\":\"v2\"}");
        pg.jdbc().update("UPDATE platform.monitoring_profile_assignment SET row_version=? "
                + "WHERE tenant_id=?::uuid AND entity_id=?::uuid", Long.MAX_VALUE, f.tenantId, f.entityId);
        assertEquals(0, update(f, f.otherProfileId, 1, AssignmentMode.AUTO_FROM_CRITICALITY, Long.MAX_VALUE));
        assertEquals(Long.MAX_VALUE,
                reads.findAssignment(f.tenantId, f.entityId).orElseThrow().rowVersion());

        pg.jdbc().update("UPDATE platform.monitoring_profile_assignment SET row_version=1 "
                + "WHERE tenant_id=?::uuid AND entity_id=?::uuid", f.tenantId, f.entityId);
        assertEquals(1, update(f, f.profileId, 2, AssignmentMode.AUTO_FROM_CRITICALITY, 1));
        var json = new com.fasterxml.jackson.databind.ObjectMapper();
        assertEquals(json.readTree("{\"definition\":\"v1\"}"),
                json.readTree(reads.findVersion(f.tenantId, f.profileId, 1)
                        .orElseThrow().definitionJson()));
        assertEquals(json.readTree("{\"definition\":\"v2\"}"),
                json.readTree(reads.findVersion(f.tenantId, f.profileId, 2)
                        .orElseThrow().definitionJson()));
    }

    private int raceUpdate(Fixture f, UUID profileId, CountDownLatch ready, CountDownLatch start) throws Exception {
        ready.countDown();
        start.await();
        return mutations.updateIfVersion(f.tenantId, f.entityId, profileId, 1,
                AssignmentMode.AUTO_FROM_CRITICALITY, f.otherActorId, f.otherAuditRef, 1);
    }

    private int insert(Fixture f, UUID profileId, int profileVersion, AssignmentMode mode) {
        return mutations.insertIfAbsent(f.tenantId, f.entityId, profileId, profileVersion,
                mode, f.actorId, f.auditRef);
    }

    private int update(Fixture f, UUID profileId, int profileVersion, AssignmentMode mode, long expected) {
        return mutations.updateIfVersion(f.tenantId, f.entityId, profileId, profileVersion,
                mode, f.otherActorId, f.otherAuditRef, expected);
    }

    private Fixture fixture() {
        UUID tenantId = UUID.randomUUID();
        UUID entityId = UUID.randomUUID();
        UUID profileId = UUID.randomUUID();
        UUID otherProfileId = UUID.randomUUID();
        UUID actorId = UUID.randomUUID();
        UUID otherActorId = UUID.randomUUID();
        UUID auditRef = UUID.randomUUID();
        UUID otherAuditRef = UUID.randomUUID();
        tenant(tenantId);
        entity(tenantId, entityId);
        profile(tenantId, profileId, "Primary");
        profile(tenantId, otherProfileId, "Secondary");
        version(tenantId, profileId, 1, "{\"definition\":\"v1\"}");
        version(tenantId, otherProfileId, 1, "{\"definition\":\"other\"}");
        return new Fixture(tenantId, entityId, profileId, otherProfileId,
                actorId, otherActorId, auditRef, otherAuditRef);
    }

    private void tenant(UUID tenantId) {
        pg.jdbc().update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?::uuid, 'Assignment mutation fixture', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                """, tenantId);
    }

    private void entity(UUID tenantId, UUID entityId) {
        pg.jdbc().update("""
                INSERT INTO platform.entity
                    (entity_id, tenant_id, entity_type, display_name, lifecycle_state,
                     first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?::uuid, ?::uuid, 'HOST', 'Mutation fixture host', 'ACTIVE', NOW(), NOW(), NOW(), NOW())
                """, entityId, tenantId);
    }

    private void profile(UUID tenantId, UUID profileId, String name) {
        pg.jdbc().update("""
                INSERT INTO platform.monitoring_profile (tenant_id, profile_id, name, created_by)
                VALUES (?::uuid, ?::uuid, ?, ?::uuid)
                """, tenantId, profileId, name, UUID.randomUUID());
    }

    private void version(UUID tenantId, UUID profileId, int version, String definition) {
        pg.jdbc().update("""
                INSERT INTO platform.monitoring_profile_version
                    (tenant_id, profile_id, version_no, definition, content_hash, created_by)
                VALUES (?::uuid, ?::uuid, ?, ?::jsonb, decode(repeat('ab',32),'hex'), ?::uuid)
                """, tenantId, profileId, version, definition, UUID.randomUUID());
    }

    private record Fixture(UUID tenantId, UUID entityId, UUID profileId, UUID otherProfileId,
                          UUID actorId, UUID otherActorId, UUID auditRef, UUID otherAuditRef) { }
}
