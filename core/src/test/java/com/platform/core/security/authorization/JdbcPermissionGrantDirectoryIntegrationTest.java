package com.platform.core.security.authorization;

import com.platform.core.security.http.SecurityDirectoryConfiguration;
import com.platform.core.security.http.SecurityDirectoryProperties;
import com.platform.core.security.identity.PlatformSubject;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.TransactionDefinition;
import org.springframework.transaction.support.TransactionTemplate;

import java.time.Duration;
import java.time.Instant;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Real PostgreSQL membership/grant integrity, snapshot, limits and fail-closed behavior.
 * SOURCE_DRAFT — focused verification authorized after Master SOURCE_ACCEPTED.
 */
@Tag("postgres")
class JdbcPermissionGrantDirectoryIntegrationTest {

    private PostgresTestSupport pg;
    private JdbcPermissionGrantDirectory directory;
    private UUID tenantA;
    private UUID tenantB;
    private UUID subjectId;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        directory = newDirectory(Duration.ofSeconds(2), 256, 1024, 1024, 8192);
        tenantA = insertTenant("Tenant A");
        tenantB = insertTenant("Tenant B");
        subjectId = insertSubject();
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void membershipAndTenantWideGrantLoad() {
        insertMembership(subjectId, tenantA);
        UUID grantId = insertTenantWideGrant(subjectId, tenantA, "tenant.read", "TENANT");

        PermissionGrantDirectory.PrincipalAuthorization authz =
                directory.load(new PlatformSubject(subjectId));
        assertEquals(Set.of(tenantA), authz.tenantScopes());
        assertEquals(1, authz.permissionGrants().size());
        PermissionGrant grant = authz.permissionGrants().iterator().next();
        assertEquals(tenantA, grant.tenantId());
        assertEquals(PlatformPermissions.TENANT_READ, grant.permission());
        assertEquals(ResourceScope.Kind.TENANT_WIDE, grant.resourceScope().kind());
        assertTrue(grant.resourceScope().resourceIds().isEmpty());
        Integer persisted = pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.permission_grant WHERE grant_id = ?",
                Integer.class, grantId);
        assertEquals(1, persisted);
    }

    @Test
    void namedResourceE1DoesNotAuthorizeE2AndEmptyNamedFailsClosed() {
        insertMembership(subjectId, tenantA);
        UUID entity1 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");
        UUID entity2 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e2");
        UUID grantId = insertNamedGrant(subjectId, tenantA, "entity.read", "ENTITY", Set.of(entity1));

        PermissionGrantDirectory.PrincipalAuthorization authz =
                directory.load(new PlatformSubject(subjectId));
        assertEquals(1, authz.permissionGrants().size());
        PermissionGrant grant = authz.permissionGrants().iterator().next();
        assertTrue(grant.resourceScope().allows(ResourceType.ENTITY, entity1));
        assertFalse(grant.resourceScope().allows(ResourceType.ENTITY, entity2));
        Integer namedRows = pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.permission_grant_resource WHERE grant_id = ?",
                Integer.class, grantId);
        assertEquals(1, namedRows);

        UUID emptyNamed = UUID.randomUUID();
        pg.jdbc().update("""
                INSERT INTO platform.permission_grant
                    (grant_id, subject_id, tenant_id, permission, resource_type, scope_kind)
                VALUES (?, ?, ?, 'entity.read', 'ENTITY', 'NAMED_RESOURCES')
                """, emptyNamed, subjectId, tenantA);
        SecurityDirectoryConfiguration.DirectoryFailureException corrupt = assertThrows(
                SecurityDirectoryConfiguration.DirectoryFailureException.class,
                () -> directory.load(new PlatformSubject(subjectId))
        );
        assertEquals("SECURITY_DIRECTORY_CORRUPT", corrupt.reasonCode());
    }

    @Test
    void grantWithoutMembershipAndIncompatiblePermissionFailClosed() {
        JdbcTemplate jdbc = pg.jdbc();
        jdbc.update("""
                INSERT INTO platform.subject_tenant_scope (subject_id, tenant_id, status)
                VALUES (?, ?, 'REVOKED')
                """, subjectId, tenantA);
        jdbc.update("""
                INSERT INTO platform.permission_grant
                    (grant_id, subject_id, tenant_id, permission, resource_type, scope_kind)
                VALUES (?, ?, ?, 'tenant.read', 'TENANT', 'TENANT_WIDE')
                """, UUID.randomUUID(), subjectId, tenantA);

        SecurityDirectoryConfiguration.DirectoryFailureException missingMembership = assertThrows(
                SecurityDirectoryConfiguration.DirectoryFailureException.class,
                () -> directory.load(new PlatformSubject(subjectId))
        );
        assertEquals("SECURITY_DIRECTORY_CORRUPT", missingMembership.reasonCode());

        jdbc.update("DELETE FROM platform.permission_grant WHERE subject_id=?", subjectId);
        jdbc.update("UPDATE platform.subject_tenant_scope SET status='ACTIVE' WHERE subject_id=?", subjectId);
        jdbc.update("""
                INSERT INTO platform.permission_grant
                    (grant_id, subject_id, tenant_id, permission, resource_type, scope_kind)
                VALUES (?, ?, ?, 'tenant.read', 'ENTITY', 'TENANT_WIDE')
                """, UUID.randomUUID(), subjectId, tenantA);
        SecurityDirectoryConfiguration.DirectoryFailureException incompatible = assertThrows(
                SecurityDirectoryConfiguration.DirectoryFailureException.class,
                () -> directory.load(new PlatformSubject(subjectId))
        );
        assertEquals("SECURITY_DIRECTORY_CORRUPT", incompatible.reasonCode());
    }

    @Test
    void foreignKeyRejectsGrantWithoutSubjectTenantMembershipRow() {
        UUID orphanTenant = insertTenant("Orphan Tenant");
        assertThrows(DataIntegrityViolationException.class, () ->
                pg.jdbc().update("""
                        INSERT INTO platform.permission_grant
                            (grant_id, subject_id, tenant_id, permission, resource_type, scope_kind)
                        VALUES (?, ?, ?, 'tenant.read', 'TENANT', 'TENANT_WIDE')
                        """, UUID.randomUUID(), subjectId, orphanTenant));
    }

    @Test
    void committedGrantRevocationObservedOnLaterLoad() {
        insertMembership(subjectId, tenantA);
        UUID grantId = insertTenantWideGrant(subjectId, tenantA, "tenant.read", "TENANT");
        assertEquals(1, directory.load(new PlatformSubject(subjectId)).permissionGrants().size());

        pg.jdbc().update(
                "UPDATE platform.permission_grant SET revoked_at = ? WHERE grant_id = ?",
                java.sql.Timestamp.from(Instant.now()), grantId
        );
        PermissionGrantDirectory.PrincipalAuthorization after =
                directory.load(new PlatformSubject(subjectId));
        assertEquals(Set.of(tenantA), after.tenantScopes());
        assertTrue(after.permissionGrants().isEmpty());
    }

    @Test
    void committedMembershipRevocationObservedOnLaterLoad() {
        insertMembership(subjectId, tenantA);
        insertTenantWideGrant(subjectId, tenantA, "tenant.read", "TENANT");
        assertEquals(Set.of(tenantA), directory.load(new PlatformSubject(subjectId)).tenantScopes());

        pg.jdbc().update("""
                UPDATE platform.subject_tenant_scope SET status = 'REVOKED'
                 WHERE subject_id = ? AND tenant_id = ?
                """, subjectId, tenantA);
        // Grant still present but membership inactive → whole load fails closed (corrupt),
        // not a partial principal with orphaned grants.
        SecurityDirectoryConfiguration.DirectoryFailureException ex = assertThrows(
                SecurityDirectoryConfiguration.DirectoryFailureException.class,
                () -> directory.load(new PlatformSubject(subjectId))
        );
        assertEquals("SECURITY_DIRECTORY_CORRUPT", ex.reasonCode());
    }

    @Test
    void newlyConstructedDirectoryInstanceSeesCommittedState() {
        insertMembership(subjectId, tenantA);
        UUID grantId = insertTenantWideGrant(subjectId, tenantA, "tenant.read", "TENANT");
        JdbcPermissionGrantDirectory fresh = newDirectory(Duration.ofSeconds(2), 256, 1024, 1024, 8192);
        assertEquals(1, fresh.load(new PlatformSubject(subjectId)).permissionGrants().size());

        pg.jdbc().update(
                "UPDATE platform.permission_grant SET revoked_at = NOW() WHERE grant_id = ?", grantId);
        JdbcPermissionGrantDirectory afterRevoke = newDirectory(Duration.ofSeconds(2), 256, 1024, 1024, 8192);
        assertTrue(afterRevoke.load(new PlatformSubject(subjectId)).permissionGrants().isEmpty());
    }

    @Test
    void atomicCommittedMembershipAndGrantChangeObservedConsistentlyOnLaterLoad() {
        insertMembership(subjectId, tenantA);
        insertMembership(subjectId, tenantB);
        UUID grantA = insertTenantWideGrant(subjectId, tenantA, "tenant.read", "TENANT");
        insertTenantWideGrant(subjectId, tenantB, "tenant.read", "TENANT");

        PermissionGrantDirectory.PrincipalAuthorization before =
                directory.load(new PlatformSubject(subjectId));
        assertEquals(Set.of(tenantA, tenantB), before.tenantScopes());
        assertEquals(2, before.permissionGrants().size());

        TransactionTemplate writeTx = new TransactionTemplate(new DataSourceTransactionManager(pg.dataSource()));
        writeTx.execute(status -> {
            pg.jdbc().update(
                    "UPDATE platform.permission_grant SET revoked_at = NOW() WHERE grant_id = ?", grantA);
            pg.jdbc().update("""
                    UPDATE platform.subject_tenant_scope SET status = 'REVOKED'
                     WHERE subject_id = ? AND tenant_id = ?
                    """, subjectId, tenantA);
            return null;
        });

        PermissionGrantDirectory.PrincipalAuthorization after =
                directory.load(new PlatformSubject(subjectId));
        assertEquals(Set.of(tenantB), after.tenantScopes());
        assertEquals(1, after.permissionGrants().size());
        assertEquals(tenantB, after.permissionGrants().iterator().next().tenantId());
    }

    /**
     * Deterministic interleave: pause after real membership SELECT completes and before grant SELECT.
     * Writer atomically revokes membership+grant and commits; RR in-flight must keep complete old state.
     * Bounded READ_COMMITTED control proves the same interleave can tear the snapshot.
     */
    @Test
    void repeatableReadSnapshotHoldsAcrossMembershipGrantInterleave_readCommittedControlTears()
            throws Exception {
        insertMembership(subjectId, tenantA);
        UUID grantId = insertTenantWideGrant(subjectId, tenantA, "tenant.read", "TENANT");

        CountDownLatch membershipDone = new CountDownLatch(1);
        CountDownLatch resumeGrantQuery = new CountDownLatch(1);
        CountDownLatch writerCommitted = new CountDownLatch(1);
        AtomicBoolean pauseArmed = new AtomicBoolean(true);

        PausingAfterMembershipJdbcTemplate pausingJdbc = new PausingAfterMembershipJdbcTemplate(
                pg.dataSource(), membershipDone, resumeGrantQuery, pauseArmed);
        DataSourceTransactionManager tm = new DataSourceTransactionManager(pg.dataSource());
        JdbcPermissionGrantDirectory rrDirectory = newDirectoryWithJdbc(
                pausingJdbc, tm, Duration.ofSeconds(2), 256, 1024, 1024, 8192);

        ExecutorService pool = Executors.newFixedThreadPool(2);
        Future<PermissionGrantDirectory.PrincipalAuthorization> inFlight = null;
        Future<?> writer = null;
        try {
            inFlight = pool.submit(() -> rrDirectory.load(new PlatformSubject(subjectId)));

            assertTrue(membershipDone.await(10, TimeUnit.SECONDS), "membership SELECT did not complete");

            writer = pool.submit(() -> {
                TransactionTemplate writeTx = new TransactionTemplate(tm);
                writeTx.execute(status -> {
                    pg.jdbc().update(
                            "UPDATE platform.permission_grant SET revoked_at = NOW() WHERE grant_id = ?",
                            grantId);
                    pg.jdbc().update("""
                            UPDATE platform.subject_tenant_scope SET status = 'REVOKED'
                             WHERE subject_id = ? AND tenant_id = ?
                            """, subjectId, tenantA);
                    return null;
                });
                writerCommitted.countDown();
                resumeGrantQuery.countDown();
                return null;
            });

            assertTrue(writerCommitted.await(10, TimeUnit.SECONDS), "writer did not commit");
            writer.get(10, TimeUnit.SECONDS);

            PermissionGrantDirectory.PrincipalAuthorization mid = inFlight.get(15, TimeUnit.SECONDS);
            assertEquals(Set.of(tenantA), mid.tenantScopes(), "RR in-flight must keep complete old membership");
            assertEquals(1, mid.permissionGrants().size(), "RR in-flight must keep complete old grant");
            assertEquals(tenantA, mid.permissionGrants().iterator().next().tenantId());

            PermissionGrantDirectory.PrincipalAuthorization later =
                    directory.load(new PlatformSubject(subjectId));
            assertTrue(later.tenantScopes().isEmpty(), "later load must see complete new membership state");
            assertTrue(later.permissionGrants().isEmpty(), "later load must see complete new grant state");

            // Bounded test-only READ_COMMITTED control: same interleave must be able to tear.
            pauseArmed.set(true);
            CountDownLatch rcMembershipDone = new CountDownLatch(1);
            CountDownLatch rcResume = new CountDownLatch(1);
            PausingAfterMembershipJdbcTemplate rcJdbc = new PausingAfterMembershipJdbcTemplate(
                    pg.dataSource(), rcMembershipDone, rcResume, pauseArmed);
            // Re-seed old state for the control run.
            pg.jdbc().update("""
                    UPDATE platform.subject_tenant_scope SET status = 'ACTIVE'
                     WHERE subject_id = ? AND tenant_id = ?
                    """, subjectId, tenantA);
            pg.jdbc().update(
                    "UPDATE platform.permission_grant SET revoked_at = NULL WHERE grant_id = ?", grantId);

            Future<PermissionGrantDirectory.PrincipalAuthorization> rcInFlight = pool.submit(() ->
                    loadWithIsolation(rcJdbc, tm, TransactionDefinition.ISOLATION_READ_COMMITTED,
                            new PlatformSubject(subjectId), Duration.ofSeconds(2), 256, 1024, 1024, 8192));
            assertTrue(rcMembershipDone.await(10, TimeUnit.SECONDS), "RC membership SELECT did not complete");
            TransactionTemplate writeTx2 = new TransactionTemplate(tm);
            writeTx2.execute(status -> {
                pg.jdbc().update(
                        "UPDATE platform.permission_grant SET revoked_at = NOW() WHERE grant_id = ?",
                        grantId);
                pg.jdbc().update("""
                        UPDATE platform.subject_tenant_scope SET status = 'REVOKED'
                         WHERE subject_id = ? AND tenant_id = ?
                        """, subjectId, tenantA);
                return null;
            });
            rcResume.countDown();
            PermissionGrantDirectory.PrincipalAuthorization torn = rcInFlight.get(15, TimeUnit.SECONDS);
            // Torn: membership list from before revoke, grants from after → not a committed snapshot.
            assertEquals(Set.of(tenantA), torn.tenantScopes());
            assertTrue(torn.permissionGrants().isEmpty(),
                    "READ_COMMITTED control must observe post-commit empty grants against pre-commit membership");
        } finally {
            resumeGrantQuery.countDown();
            if (inFlight != null && !inFlight.isDone()) {
                inFlight.cancel(true);
            }
            if (writer != null && !writer.isDone()) {
                writer.cancel(true);
            }
            pool.shutdownNow();
            assertTrue(pool.awaitTermination(10, TimeUnit.SECONDS), "worker pool did not terminate");
        }
    }

    @Test
    void membershipBoundAcceptsExactMaxAndRejectsOverflow() {
        UUID tenantC = insertTenant("Tenant C");
        insertMembership(subjectId, tenantA);
        insertMembership(subjectId, tenantB);
        // Exact bound=2 succeeds (unrelated limits high).
        JdbcPermissionGrantDirectory atBound = newDirectory(Duration.ofSeconds(2), 2, 1024, 1024, 8192);
        assertEquals(Set.of(tenantA, tenantB), atBound.load(new PlatformSubject(subjectId)).tenantScopes());

        insertMembership(subjectId, tenantC);
        JdbcPermissionGrantDirectory overflow = newDirectory(Duration.ofSeconds(2), 2, 1024, 1024, 8192);
        SecurityDirectoryConfiguration.DirectoryFailureException ex = assertThrows(
                SecurityDirectoryConfiguration.DirectoryFailureException.class,
                () -> overflow.load(new PlatformSubject(subjectId))
        );
        assertEquals("SECURITY_DIRECTORY_LIMIT", ex.reasonCode());
    }

    @Test
    void grantCountBoundAcceptsExactMaxAndRejectsOverflow() {
        insertMembership(subjectId, tenantA);
        insertTenantWideGrant(subjectId, tenantA, "tenant.read", "TENANT");
        insertTenantWideGrant(subjectId, tenantA, "tenant.update", "TENANT");
        JdbcPermissionGrantDirectory atBound = newDirectory(Duration.ofSeconds(2), 256, 2, 1024, 8192);
        assertEquals(2, atBound.load(new PlatformSubject(subjectId)).permissionGrants().size());

        insertMembership(subjectId, tenantB);
        insertTenantWideGrant(subjectId, tenantB, "tenant.read", "TENANT");
        JdbcPermissionGrantDirectory overflow = newDirectory(Duration.ofSeconds(2), 256, 2, 1024, 8192);
        SecurityDirectoryConfiguration.DirectoryFailureException ex = assertThrows(
                SecurityDirectoryConfiguration.DirectoryFailureException.class,
                () -> overflow.load(new PlatformSubject(subjectId))
        );
        assertEquals("SECURITY_DIRECTORY_LIMIT", ex.reasonCode());
    }

    @Test
    void namedResourceBoundAcceptsExactMaxAndRejectsOverflow() {
        insertMembership(subjectId, tenantA);
        UUID e1 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");
        UUID e2 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e2");
        UUID e3 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e3");
        insertNamedGrant(subjectId, tenantA, "entity.read", "ENTITY", Set.of(e1, e2));
        // Aggregate high enough that named-per-grant is the binding limit.
        JdbcPermissionGrantDirectory atBound = newDirectory(Duration.ofSeconds(2), 256, 1024, 2, 8192);
        assertEquals(1, atBound.load(new PlatformSubject(subjectId)).permissionGrants().size());
        assertEquals(2, atBound.load(new PlatformSubject(subjectId)).permissionGrants()
                .iterator().next().resourceScope().resourceIds().size());

        pg.jdbc().update("DELETE FROM platform.permission_grant_resource");
        pg.jdbc().update("DELETE FROM platform.permission_grant WHERE subject_id=?", subjectId);
        insertNamedGrant(subjectId, tenantA, "entity.read", "ENTITY", Set.of(e1, e2, e3));
        JdbcPermissionGrantDirectory overflow = newDirectory(Duration.ofSeconds(2), 256, 1024, 2, 8192);
        SecurityDirectoryConfiguration.DirectoryFailureException ex = assertThrows(
                SecurityDirectoryConfiguration.DirectoryFailureException.class,
                () -> overflow.load(new PlatformSubject(subjectId))
        );
        assertEquals("SECURITY_DIRECTORY_LIMIT", ex.reasonCode());
    }

    @Test
    void materializedAuthorityRowsBoundAcceptsExactMaxAndRejectsOverflow() {
        insertMembership(subjectId, tenantA);
        UUID e1 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");
        UUID e2 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e2");
        UUID e3 = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e3");
        // Two joined rows; aggregate max=2 accepts (named max kept high).
        insertNamedGrant(subjectId, tenantA, "entity.read", "ENTITY", Set.of(e1, e2));
        JdbcPermissionGrantDirectory atBound = newDirectory(Duration.ofSeconds(2), 256, 1024, 1024, 2);
        assertEquals(1, atBound.load(new PlatformSubject(subjectId)).permissionGrants().size());

        pg.jdbc().update("DELETE FROM platform.permission_grant_resource");
        pg.jdbc().update("DELETE FROM platform.permission_grant WHERE subject_id=?", subjectId);
        insertNamedGrant(subjectId, tenantA, "entity.read", "ENTITY", Set.of(e1, e2, e3));
        JdbcPermissionGrantDirectory overflow = newDirectory(Duration.ofSeconds(2), 256, 1024, 1024, 2);
        SecurityDirectoryConfiguration.DirectoryFailureException ex = assertThrows(
                SecurityDirectoryConfiguration.DirectoryFailureException.class,
                () -> overflow.load(new PlatformSubject(subjectId))
        );
        assertEquals("SECURITY_DIRECTORY_LIMIT", ex.reasonCode());
    }

    @Test
    void statementTimeoutFailsClosedAsTimeoutWithoutDetailLeak() throws Exception {
        insertMembership(subjectId, tenantA);
        insertTenantWideGrant(subjectId, tenantA, "tenant.read", "TENANT");
        JdbcPermissionGrantDirectory timed = newDirectory(Duration.ofMillis(1), 256, 1024, 1024, 8192);

        TransactionTemplate locker = new TransactionTemplate(new DataSourceTransactionManager(pg.dataSource()));
        CountDownLatch locked = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        ExecutorService pool = Executors.newSingleThreadExecutor();
        Future<?> holder = null;
        try {
            holder = pool.submit(() -> locker.execute(status -> {
                pg.jdbc().execute("LOCK TABLE platform.permission_grant IN ACCESS EXCLUSIVE MODE");
                locked.countDown();
                try {
                    if (!release.await(15, TimeUnit.SECONDS)) {
                        throw new IllegalStateException("lock holder release deadline exceeded");
                    }
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    throw new IllegalStateException("lock holder interrupted", ie);
                }
                return null;
            }));
            assertTrue(locked.await(10, TimeUnit.SECONDS), "table lock was not acquired");
            SecurityDirectoryConfiguration.DirectoryFailureException ex = assertThrows(
                    SecurityDirectoryConfiguration.DirectoryFailureException.class,
                    () -> timed.load(new PlatformSubject(subjectId))
            );
            assertEquals("SECURITY_DIRECTORY_TIMEOUT", ex.reasonCode());
            assertFalse(ex.getMessage().toLowerCase().contains("password"));
            assertFalse(ex.getMessage().toLowerCase().contains("jdbc:"));
            assertFalse(ex.getMessage().toLowerCase().contains("permission_grant"));
        } finally {
            release.countDown();
            if (holder != null) {
                holder.get(15, TimeUnit.SECONDS);
            }
            pool.shutdownNow();
            assertTrue(pool.awaitTermination(10, TimeUnit.SECONDS));
        }
    }

    @Test
    void tenantWideMustNotCarryNamedChildren() {
        insertMembership(subjectId, tenantA);
        UUID grantId = insertTenantWideGrant(subjectId, tenantA, "tenant.read", "TENANT");
        pg.jdbc().update("""
                INSERT INTO platform.permission_grant_resource (grant_id, resource_id)
                VALUES (?, ?)
                """, grantId, UUID.randomUUID());
        SecurityDirectoryConfiguration.DirectoryFailureException ex = assertThrows(
                SecurityDirectoryConfiguration.DirectoryFailureException.class,
                () -> directory.load(new PlatformSubject(subjectId))
        );
        assertEquals("SECURITY_DIRECTORY_CORRUPT", ex.reasonCode());
    }

    private JdbcPermissionGrantDirectory newDirectory(
            Duration timeout, int maxScopes, int maxGrants, int maxNamed, int maxAuthorityRows
    ) {
        return newDirectoryWithJdbc(
                pg.jdbc(),
                new DataSourceTransactionManager(pg.dataSource()),
                timeout, maxScopes, maxGrants, maxNamed, maxAuthorityRows
        );
    }

    private JdbcPermissionGrantDirectory newDirectoryWithJdbc(
            JdbcTemplate jdbc,
            PlatformTransactionManager tm,
            Duration timeout, int maxScopes, int maxGrants, int maxNamed, int maxAuthorityRows
    ) {
        SecurityDirectoryProperties props = new SecurityDirectoryProperties();
        props.setPersistenceEnabled(true);
        props.setQueryTimeout(timeout);
        props.setMaxTenantScopes(maxScopes);
        props.setMaxPermissionGrants(maxGrants);
        props.setMaxNamedResourcesPerGrant(maxNamed);
        props.setMaxMaterializedAuthorityRows(maxAuthorityRows);
        return new JdbcPermissionGrantDirectory(jdbc, tm, props);
    }

    /**
     * Test-only loader mirroring production SQL with forced isolation (READ_COMMITTED control).
     * Production {@link JdbcPermissionGrantDirectory} bytes remain unchanged.
     */
    private static PermissionGrantDirectory.PrincipalAuthorization loadWithIsolation(
            JdbcTemplate jdbcTemplate,
            PlatformTransactionManager transactionManager,
            int isolation,
            PlatformSubject subject,
            Duration queryTimeout,
            int maxTenantScopes,
            int maxPermissionGrants,
            int maxNamedResourcesPerGrant,
            int maxMaterializedAuthorityRows
    ) {
        TransactionTemplate tx = new TransactionTemplate(transactionManager);
        tx.setReadOnly(true);
        tx.setPropagationBehavior(TransactionDefinition.PROPAGATION_REQUIRES_NEW);
        tx.setIsolationLevel(isolation);
        PermissionGrantDirectory.PrincipalAuthorization loaded = tx.execute(status -> {
            long millis = Math.max(1L, queryTimeout.toMillis());
            jdbcTemplate.execute((java.sql.Connection connection) -> {
                try (var ps = connection.prepareStatement(
                        "SET LOCAL statement_timeout = '" + millis + "ms'")) {
                    ps.execute();
                }
                return null;
            });
            int membershipFetch = SecurityDirectoryProperties.overflowDetectFetchSize(maxTenantScopes);
            List<UUID> memberships = jdbcTemplate.query(
                    """
                    SELECT tenant_id
                      FROM platform.subject_tenant_scope
                     WHERE subject_id = ?
                       AND status = 'ACTIVE'
                     ORDER BY tenant_id
                     LIMIT ?
                    """,
                    (rs, rowNum) -> rs.getObject("tenant_id", UUID.class),
                    subject.subjectId(),
                    membershipFetch
            );
            if (memberships.size() > maxTenantScopes) {
                throw new SecurityDirectoryConfiguration.DirectoryFailureException("SECURITY_DIRECTORY_LIMIT");
            }
            Set<UUID> tenantScopes = Set.copyOf(memberships);
            int authorityFetch = SecurityDirectoryProperties.overflowDetectFetchSize(maxMaterializedAuthorityRows);
            List<Object[]> grantRows = jdbcTemplate.query(
                    """
                    SELECT g.grant_id, g.tenant_id, g.permission, g.resource_type, g.scope_kind, r.resource_id
                      FROM platform.permission_grant g
                      LEFT JOIN platform.permission_grant_resource r ON r.grant_id = g.grant_id
                     WHERE g.subject_id = ? AND g.revoked_at IS NULL
                     ORDER BY g.grant_id, r.resource_id
                     LIMIT ?
                    """,
                    (rs, rowNum) -> new Object[]{
                            rs.getObject("grant_id", UUID.class),
                            rs.getObject("tenant_id", UUID.class),
                            rs.getString("permission"),
                            rs.getString("resource_type"),
                            rs.getString("scope_kind"),
                            rs.getObject("resource_id", UUID.class)
                    },
                    subject.subjectId(),
                    authorityFetch
            );
            if (grantRows.size() > maxMaterializedAuthorityRows) {
                throw new SecurityDirectoryConfiguration.DirectoryFailureException("SECURITY_DIRECTORY_LIMIT");
            }
            Map<UUID, MutableGrant> byGrant = new HashMap<>();
            for (Object[] row : grantRows) {
                UUID grantId = (UUID) row[0];
                UUID tenantId = (UUID) row[1];
                String permission = (String) row[2];
                String resourceType = (String) row[3];
                String scopeKind = (String) row[4];
                UUID resourceId = (UUID) row[5];
                MutableGrant grant = byGrant.computeIfAbsent(grantId,
                        id -> new MutableGrant(tenantId, permission, resourceType, scopeKind));
                if (resourceId != null) {
                    grant.resourceIds.add(resourceId);
                    if (grant.resourceIds.size() > maxNamedResourcesPerGrant) {
                        throw new SecurityDirectoryConfiguration.DirectoryFailureException("SECURITY_DIRECTORY_LIMIT");
                    }
                }
            }
            if (byGrant.size() > maxPermissionGrants) {
                throw new SecurityDirectoryConfiguration.DirectoryFailureException("SECURITY_DIRECTORY_LIMIT");
            }
            Set<PermissionGrant> grants = new LinkedHashSet<>();
            for (MutableGrant mutable : byGrant.values()) {
                if (!tenantScopes.contains(mutable.tenantId)) {
                    throw new SecurityDirectoryConfiguration.DirectoryFailureException("SECURITY_DIRECTORY_CORRUPT");
                }
                ResourceType resourceType = ResourceType.valueOf(mutable.resourceType);
                ResourceScope.Kind kind = ResourceScope.Kind.valueOf(mutable.scopeKind);
                ResourceScope scope = kind == ResourceScope.Kind.TENANT_WIDE
                        ? ResourceScope.tenantWide(resourceType)
                        : ResourceScope.named(resourceType, mutable.resourceIds);
                grants.add(new PermissionGrant(mutable.tenantId, Permission.of(mutable.permission), scope));
            }
            return new PermissionGrantDirectory.PrincipalAuthorization(tenantScopes, grants);
        });
        return Objects.requireNonNull(loaded);
    }

    private static final class MutableGrant {
        private final UUID tenantId;
        private final String permission;
        private final String resourceType;
        private final String scopeKind;
        private final Set<UUID> resourceIds = new HashSet<>();

        private MutableGrant(UUID tenantId, String permission, String resourceType, String scopeKind) {
            this.tenantId = tenantId;
            this.permission = permission;
            this.resourceType = resourceType;
            this.scopeKind = scopeKind;
        }
    }

    /**
     * Delegates the real membership query, then pauses once before returning so a writer can commit.
     */
    private static final class PausingAfterMembershipJdbcTemplate extends JdbcTemplate {
        private final CountDownLatch membershipDone;
        private final CountDownLatch resume;
        private final AtomicBoolean armed;

        private PausingAfterMembershipJdbcTemplate(
                javax.sql.DataSource dataSource,
                CountDownLatch membershipDone,
                CountDownLatch resume,
                AtomicBoolean armed
        ) {
            super(dataSource);
            this.membershipDone = membershipDone;
            this.resume = resume;
            this.armed = armed;
        }

        @Override
        public <T> List<T> query(String sql, RowMapper<T> rowMapper, Object... args) {
            List<T> rows = super.query(sql, rowMapper, args);
            if (armed.get() && sql.contains("subject_tenant_scope")) {
                armed.set(false);
                membershipDone.countDown();
                try {
                    if (!resume.await(20, TimeUnit.SECONDS)) {
                        throw new IllegalStateException("resume after membership SELECT timed out");
                    }
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    throw new IllegalStateException("interrupted while paused after membership SELECT", ie);
                }
            }
            return rows;
        }
    }

    private UUID insertTenant(String name) {
        UUID id = UUID.randomUUID();
        pg.jdbc().update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?, ?, 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                """, id, name);
        return id;
    }

    private UUID insertSubject() {
        UUID id = UUID.randomUUID();
        pg.jdbc().update("""
                INSERT INTO platform.platform_subject (subject_id, status)
                VALUES (?, 'ACTIVE')
                """, id);
        return id;
    }

    private void insertMembership(UUID subject, UUID tenant) {
        pg.jdbc().update("""
                INSERT INTO platform.subject_tenant_scope (subject_id, tenant_id, status)
                VALUES (?, ?, 'ACTIVE')
                """, subject, tenant);
    }

    private UUID insertTenantWideGrant(UUID subject, UUID tenant, String permission, String resourceType) {
        UUID grantId = UUID.randomUUID();
        pg.jdbc().update("""
                INSERT INTO platform.permission_grant
                    (grant_id, subject_id, tenant_id, permission, resource_type, scope_kind)
                VALUES (?, ?, ?, ?, ?, 'TENANT_WIDE')
                """, grantId, subject, tenant, permission, resourceType);
        return grantId;
    }

    private UUID insertNamedGrant(
            UUID subject, UUID tenant, String permission, String resourceType, Set<UUID> resources
    ) {
        UUID grantId = UUID.randomUUID();
        pg.jdbc().update("""
                INSERT INTO platform.permission_grant
                    (grant_id, subject_id, tenant_id, permission, resource_type, scope_kind)
                VALUES (?, ?, ?, ?, ?, 'NAMED_RESOURCES')
                """, grantId, subject, tenant, permission, resourceType);
        for (UUID resourceId : resources) {
            pg.jdbc().update("""
                    INSERT INTO platform.permission_grant_resource (grant_id, resource_id)
                    VALUES (?, ?)
                    """, grantId, resourceId);
        }
        return grantId;
    }
}
