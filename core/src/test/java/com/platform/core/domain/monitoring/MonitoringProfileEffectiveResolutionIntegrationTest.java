package com.platform.core.domain.monitoring;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.AssignmentMode;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.ProfileRef;
import com.platform.core.persistence.MonitoringProfileRepository;
import com.platform.core.security.authorization.AuthenticationState;
import com.platform.core.security.authorization.PermissionGrant;
import com.platform.core.security.authorization.PlatformPermissions;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.authorization.ResourceScope;
import com.platform.core.security.authorization.ResourceType;
import com.platform.core.security.http.UnauthorizedAccessException;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.jdbc.datasource.AbstractDataSource;

import javax.sql.DataSource;
import java.sql.Connection;
import java.sql.SQLException;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

/** Real PostgreSQL coverage for BUILD-070 effective profile resolution. */
@Tag("postgres")
class MonitoringProfileEffectiveResolutionIntegrationTest {

    private final UUID fixtureNamespace = UUID.randomUUID();
    private final UUID TENANT = id("tenant");
    private final UUID OTHER_TENANT = id("other-tenant");
    private final UUID ENTITY = id("entity");
    private final UUID OTHER_ENTITY = id("other-entity");
    private final UUID ASSIGNED_PROFILE = id("assigned-profile");
    private final UUID SUGGESTED_PROFILE = id("suggested-profile");
    private final UUID ACTOR = id("actor");

    private PostgresTestSupport pg;
    private MonitoringProfileRepository profiles;
    private MonitoringProfileEffectiveResolutionService service;
    private CountingDataSource countedDataSource;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate(); // Includes V023, whose same-tenant foreign keys constrain this fixture.

        JdbcTemplate db = pg.jdbc();
        tenant(db, TENANT);
        tenant(db, OTHER_TENANT);
        entity(db, TENANT, ENTITY);
        entity(db, TENANT, OTHER_ENTITY);
        profile(db, TENANT, ASSIGNED_PROFILE, "Assigned profile");
        profile(db, TENANT, SUGGESTED_PROFILE, "Suggested profile");
        version(db, TENANT, ASSIGNED_PROFILE, 1);
        version(db, TENANT, ASSIGNED_PROFILE, 7);
        version(db, TENANT, SUGGESTED_PROFILE, 1);
        version(db, TENANT, SUGGESTED_PROFILE, 2);
        assignment(db, TENANT, ENTITY, ASSIGNED_PROFILE, 1, AssignmentMode.AUTO_FROM_CRITICALITY);

        countedDataSource = new CountingDataSource(pg.dataSource());
        profiles = new MonitoringProfileRepository(new NamedParameterJdbcTemplate(countedDataSource));
        service = new MonitoringProfileEffectiveResolutionService(profiles);
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void automaticAssignmentUsesCallerSuppliedExistingSuggestion() {
        var result = service.resolve(reader(TENANT, ENTITY), TENANT, ENTITY,
                ref(TENANT, SUGGESTED_PROFILE, 2)).orElseThrow();

        assertEquals(AssignmentMode.AUTO_FROM_CRITICALITY, result.assignmentMode());
        assertEquals(ref(TENANT, SUGGESTED_PROFILE, 2), result.suggestedProfile());
        assertEquals(ref(TENANT, SUGGESTED_PROFILE, 2), result.effectiveProfile());
        assertEquals(1, profiles.findAssignment(TENANT, ENTITY).orElseThrow().rowVersion());
    }

    @Test
    void explicitAssignmentRemainsPinnedAcrossSuggestionChanges() {
        JdbcTemplate db = pg.jdbc();
        assignment(db, TENANT, OTHER_ENTITY, ASSIGNED_PROFILE, 7, AssignmentMode.EXPLICIT_PROFILE);

        var first = service.resolve(reader(TENANT, OTHER_ENTITY), TENANT, OTHER_ENTITY,
                ref(TENANT, SUGGESTED_PROFILE, 1)).orElseThrow();
        var second = service.resolve(reader(TENANT, OTHER_ENTITY), TENANT, OTHER_ENTITY,
                ref(TENANT, SUGGESTED_PROFILE, 2)).orElseThrow();

        assertEquals(ref(TENANT, SUGGESTED_PROFILE, 1), first.suggestedProfile());
        assertEquals(ref(TENANT, ASSIGNED_PROFILE, 7), first.effectiveProfile());
        assertEquals(ref(TENANT, SUGGESTED_PROFILE, 2), second.suggestedProfile());
        assertEquals(ref(TENANT, ASSIGNED_PROFILE, 7), second.effectiveProfile());
    }

    @Test
    void wrongTenantAndOutOfScopeEntityAreDeniedBeforeRepositoryConnection() {
        PrincipalContext wrongTenant = reader(OTHER_TENANT, ENTITY);
        PrincipalContext wrongEntity = reader(TENANT, ENTITY);

        countedDataSource.reset();
        assertMasked(() -> service.resolve(wrongTenant, TENANT, ENTITY,
                ref(TENANT, SUGGESTED_PROFILE, 1)));
        assertEquals(0, countedDataSource.connectionRequests());

        countedDataSource.reset();
        assertMasked(() -> service.resolve(wrongEntity, TENANT, OTHER_ENTITY,
                ref(TENANT, SUGGESTED_PROFILE, 1)));
        assertEquals(0, countedDataSource.connectionRequests());
    }

    @Test
    void missingSuggestedVersionFailsClosedAgainstPostgres() {
        DomainException error = assertThrows(DomainException.class, () -> service.resolve(
                reader(TENANT, ENTITY), TENANT, ENTITY, ref(TENANT, SUGGESTED_PROFILE, 3)));

        assertEquals(MonitoringProfileSelection.ERR_VALIDATION, error.getCode());
        assertEquals("unknownVersion", error.getFieldErrors().get("suggestedProfile"));
    }

    @Test
    void invalidSuggestedVersionIsRejectedAfterAssignmentLookupWithoutVersionQuery() {
        countedDataSource.reset();
        DomainException error = assertThrows(DomainException.class, () -> service.resolve(
                reader(TENANT, ENTITY), TENANT, ENTITY, ref(TENANT, SUGGESTED_PROFILE, 0)));

        assertEquals(MonitoringProfileSelection.ERR_VALIDATION, error.getCode());
        assertEquals("invalidVersion", error.getFieldErrors().get("suggestedProfile"));
        assertEquals(1, countedDataSource.connectionRequests(),
                "Assignment lookup occurs, but invalid version is rejected before profile-version lookup");
    }

    private static void tenant(JdbcTemplate db, UUID tenantId) {
        db.update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?::uuid, 'Effective resolution fixture', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                """, tenantId);
    }

    private static void entity(JdbcTemplate db, UUID tenantId, UUID entityId) {
        db.update("""
                INSERT INTO platform.entity
                    (entity_id, tenant_id, entity_type, display_name, lifecycle_state,
                     first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?::uuid, ?::uuid, 'HOST', 'Effective resolution entity', 'ACTIVE', NOW(), NOW(), NOW(), NOW())
                """, entityId, tenantId);
    }

    private void profile(JdbcTemplate db, UUID tenantId, UUID profileId, String name) {
        db.update("""
                INSERT INTO platform.monitoring_profile (tenant_id, profile_id, name, created_by)
                VALUES (?::uuid, ?::uuid, ?, ?::uuid)
                """, tenantId, profileId, name, ACTOR);
    }

    private void version(JdbcTemplate db, UUID tenantId, UUID profileId, int versionNo) {
        db.update("""
                INSERT INTO platform.monitoring_profile_version
                    (tenant_id, profile_id, version_no, definition, content_hash, capacity_estimate, created_by)
                VALUES (?::uuid, ?::uuid, ?, '{"cadence":"bounded"}'::jsonb,
                        decode(repeat('ab',32),'hex'), '{"estimate":1}'::jsonb, ?::uuid)
                """, tenantId, profileId, versionNo, ACTOR);
    }

    private void assignment(JdbcTemplate db, UUID tenantId, UUID entityId, UUID profileId,
                                   int versionNo, AssignmentMode mode) {
        db.update("""
                INSERT INTO platform.monitoring_profile_assignment
                    (tenant_id, entity_id, profile_id, profile_version_no, assignment_mode,
                     assigned_by, effective_from)
                VALUES (?::uuid, ?::uuid, ?::uuid, ?, ?, ?::uuid, NOW())
                """, tenantId, entityId, profileId, versionNo, mode.name(), ACTOR);
    }

    private static PrincipalContext reader(UUID tenantId, UUID entityId) {
        return new PrincipalContext("effective-resolution-reader", AuthenticationState.AUTHENTICATED,
                List.of(tenantId), List.of(new PermissionGrant(tenantId, PlatformPermissions.ENTITY_READ,
                ResourceScope.named(ResourceType.ENTITY, List.of(entityId)))));
    }

    private static ProfileRef ref(UUID tenantId, UUID profileId, int versionNo) {
        return new ProfileRef(tenantId, profileId.toString(), versionNo);
    }

    private UUID id(String name) {
        return UUID.nameUUIDFromBytes(("BUILD-070-effective-resolution:" + fixtureNamespace + ":" + name)
                .getBytes(java.nio.charset.StandardCharsets.UTF_8));
    }

    private static void assertMasked(org.junit.jupiter.api.function.Executable action) {
        UnauthorizedAccessException error = assertThrows(UnauthorizedAccessException.class, action);
        assertEquals(404, error.httpStatus());
        assertEquals("RESOURCE_NOT_FOUND", error.code());
    }

    /** Counts repository-side connection requests without intercepting fixture setup SQL. */
    private static final class CountingDataSource extends AbstractDataSource {
        private final DataSource delegate;
        private final AtomicInteger connectionRequests = new AtomicInteger();

        private CountingDataSource(DataSource delegate) {
            this.delegate = delegate;
        }

        @Override
        public Connection getConnection() throws SQLException {
            connectionRequests.incrementAndGet();
            return delegate.getConnection();
        }

        @Override
        public Connection getConnection(String username, String password) throws SQLException {
            connectionRequests.incrementAndGet();
            return delegate.getConnection(username, password);
        }

        private int connectionRequests() {
            return connectionRequests.get();
        }

        private void reset() {
            connectionRequests.set(0);
        }
    }
}
