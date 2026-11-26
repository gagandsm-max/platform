package com.platform.core.support;

import org.junit.jupiter.api.Test;

import java.sql.SQLException;

import static org.junit.jupiter.api.Assertions.*;

/** Pure configuration/lifecycle controls: no connection, Docker, schema or migration effects. */
class PostgresTestSupportExternalConfigurationTest {
    @Test
    void absentOptInPreservesLoopbackDockerMode() {
        assertNull(PostgresTestSupport.externalTarget(null, null));
        PostgresTestSupport pg = new PostgresTestSupport(null, null);
        assertTrue(pg.jdbcUrl().matches("jdbc:postgresql://127\\.0\\.0\\.1:[0-9]+/platform"));
        assertTrue(pg.containerName().startsWith("platform-day02-pg-"));
        assertNull(pg.dataSource());
        assertNull(pg.ownedDatabaseName());
        assertNull(pg.ownedDatabaseOid());
        // Do not call default stop(): the default contract intentionally invokes Docker.
    }

    @Test
    void exactExternalSelectionAndRepeatedStopNeedNoDocker() {
        PostgresTestSupport pg = new PostgresTestSupport("external-compose",
                "jdbc:postgresql://platform-test-postgres:5432/platform");
        assertEquals(PostgresTestSupport.EXTERNAL_JDBC_URL, pg.jdbcUrl());
        assertNull(pg.ownedDatabaseName());
        assertNull(pg.ownedDatabaseOid());
        pg.stop();
        pg.stop();
        assertNull(pg.dataSource());
        assertNull(pg.ownedDatabaseName());
        assertNull(pg.ownedDatabaseOid());
    }

    @Test
    void ownedDatabaseNamesAndOwnershipTokensAreBounded() {
        String first = PostgresTestSupport.generateOwnedDatabaseName();
        String second = PostgresTestSupport.generateOwnedDatabaseName();
        assertNotEquals(first, second);
        assertEquals(first, PostgresTestSupport.validateOwnedDatabaseName(first));
        assertEquals("platform.test.fixture.owner=" + "a".repeat(32),
                PostgresTestSupport.ownershipComment("a".repeat(32)));
        String[] rejectedNames = {
                null, "", "platform", "postgres", "template0", "template1",
                "plt_t_", "plt_t_ABCDEF0123456789", "plt_t_" + "a".repeat(15),
                "plt_t_" + "a".repeat(17), "plt_t_0123456789abcdef;",
                "plt_t_0123456789abcde-", "drop_me", "platform;drop database platform"
        };
        for (String name : rejectedNames) {
            IllegalArgumentException failure = assertThrows(IllegalArgumentException.class,
                    () -> PostgresTestSupport.validateOwnedDatabaseName(name));
            assertEquals("Invalid isolated external test database name", failure.getMessage());
        }
        String[] rejectedTokens = {null, "", "a".repeat(31), "a".repeat(33), "A".repeat(32), "zzzz"};
        for (String token : rejectedTokens) {
            IllegalArgumentException failure = assertThrows(IllegalArgumentException.class,
                    () -> PostgresTestSupport.ownershipComment(token));
            assertEquals("Invalid isolated external test ownership token", failure.getMessage());
        }
    }

    @Test
    void ownershipIdentityRequiresNameTokenAndPositiveOid() {
        String name = PostgresTestSupport.generateOwnedDatabaseName();
        String token = "b".repeat(32);
        assertEquals(16384L, PostgresTestSupport.validateOwnedDatabaseOid(16384L));
        PostgresTestSupport.requireCompleteOwnershipIdentity(name, token, 16384L);

        long[] rejectedOids = {0L, -1L, Long.MIN_VALUE};
        for (long oid : rejectedOids) {
            IllegalArgumentException failure = assertThrows(IllegalArgumentException.class,
                    () -> PostgresTestSupport.validateOwnedDatabaseOid(oid));
            assertEquals("Invalid isolated external test database oid", failure.getMessage());
        }

        IllegalArgumentException missingOid = assertThrows(IllegalArgumentException.class,
                () -> PostgresTestSupport.requireCompleteOwnershipIdentity(name, token, null));
        assertEquals("Incomplete isolated external test ownership identity", missingOid.getMessage());

        // Incomplete or production-like identities never authorize drop helpers.
        assertThrows(IllegalArgumentException.class,
                () -> PostgresTestSupport.requireCompleteOwnershipIdentity("platform", token, 16384L));
        assertThrows(IllegalArgumentException.class,
                () -> PostgresTestSupport.requireCompleteOwnershipIdentity("postgres", token, 16384L));
        assertThrows(IllegalArgumentException.class,
                () -> PostgresTestSupport.requireCompleteOwnershipIdentity(name, "Z".repeat(32), 16384L));
        assertThrows(IllegalArgumentException.class,
                () -> PostgresTestSupport.requireCompleteOwnershipIdentity(name, token, 0L));
        assertThrows(IllegalArgumentException.class,
                () -> PostgresTestSupport.requireCompleteOwnershipIdentity("plt_t_", token, 16384L));
    }

    @Test
    void partialUnknownAndMalformedTargetsFailClosedWithoutEcho() {
        String[] modes = {null, "", "true", "false", "EXTERNAL", "external-compose"};
        String[] urls = {null, "", "jdbc:postgresql://localhost:5432/platform",
                "jdbc:postgresql://customer:5432/platform", PostgresTestSupport.EXTERNAL_JDBC_URL + "?user=secret",
                PostgresTestSupport.EXTERNAL_JDBC_URL + "/", "jdbc:postgresql://user:secret@platform-test-postgres:5432/platform",
                "jdbc:postgresql://platform-test-postgres:5433/platform", "jdbc:postgresql://platform-test-postgres:5432/other"};
        for (String mode : modes) {
            for (String url : urls) {
                if (mode == null && url == null) continue;
                IllegalArgumentException failure = assertThrows(IllegalArgumentException.class,
                        () -> new PostgresTestSupport(mode, url));
                assertEquals("Invalid isolated external test PostgreSQL configuration", failure.getMessage());
            }
        }
        assertThrows(IllegalArgumentException.class, () -> new PostgresTestSupport(null,
                PostgresTestSupport.EXTERNAL_JDBC_URL));
        assertThrows(IllegalArgumentException.class, () -> new PostgresTestSupport("false",
                PostgresTestSupport.EXTERNAL_JDBC_URL));
    }

    @Test
    void ambiguousCreateRetainsIntentAndStopRefusesNameDrop() {
        PostgresTestSupport pg = new PostgresTestSupport(PostgresTestSupport.EXTERNAL_MODE,
                PostgresTestSupport.EXTERNAL_JDBC_URL);
        String name = PostgresTestSupport.generateOwnedDatabaseName();
        String token = "c".repeat(32);
        pg.applyUnresolvedCreateForTest(name, token,
                PostgresTestSupport.OwnedDatabaseUnresolvedCreateException.Reason.AMBIGUOUS_CREATE_OUTCOME);
        assertTrue(pg.ownedCreateUnresolved());
        assertNull(pg.ownedDatabaseOid());
        assertFalse(pg.ownedDatabaseVerified());

        IllegalStateException first = assertThrows(IllegalStateException.class, pg::stop);
        assertEquals("External test PostgreSQL cleanup failed", first.getMessage());
        assertEquals(
                "Refusing to drop external test database with unresolved create outcome; identity retained until independently proven absent",
                first.getCause().getMessage());
        assertEquals(name, pg.ownedDatabaseName());
        assertEquals(token, pg.ownershipToken());
        assertNull(pg.ownedDatabaseOid());
        assertTrue(pg.ownedCreateUnresolved());

        // Second cleanup after first failure remains fail-closed and retains the same identity.
        IllegalStateException second = assertThrows(IllegalStateException.class, pg::stop);
        assertEquals("External test PostgreSQL cleanup failed", second.getMessage());
        assertEquals(name, pg.ownedDatabaseName());
        assertEquals(token, pg.ownershipToken());
        assertNull(pg.ownedDatabaseOid());
    }

    @Test
    void oidLookupFailureRetainsIntentWithoutInventedOwnership() {
        PostgresTestSupport pg = new PostgresTestSupport(PostgresTestSupport.EXTERNAL_MODE,
                PostgresTestSupport.EXTERNAL_JDBC_URL);
        String name = PostgresTestSupport.generateOwnedDatabaseName();
        String token = "d".repeat(32);
        pg.applyUnresolvedCreateForTest(name, token,
                PostgresTestSupport.OwnedDatabaseUnresolvedCreateException.Reason.OID_LOOKUP_FAILED);
        IllegalStateException failure = assertThrows(IllegalStateException.class, pg::stop);
        assertEquals("External test PostgreSQL cleanup failed", failure.getMessage());
        assertEquals(name, pg.ownedDatabaseName());
        assertNull(pg.ownedDatabaseOid());
        assertTrue(pg.ownedCreateUnresolved());
        // Missing OID is never complete ownership identity.
        assertThrows(IllegalArgumentException.class,
                () -> PostgresTestSupport.requireCompleteOwnershipIdentity(name, token, null));
    }

    @Test
    void missingOidRejectsStructuralIdentityWhileDistinctValidTokenRemainsStructurallyComplete() {
        String name = PostgresTestSupport.generateOwnedDatabaseName();
        String token = "e".repeat(32);
        String otherValidToken = "f".repeat(32);

        // Missing OID is a structural rejection — never invent drop authority.
        assertThrows(IllegalArgumentException.class,
                () -> PostgresTestSupport.requireCompleteOwnershipIdentity(name, token, null));

        // A different syntactically valid hex token is still structurally complete.
        // Marker authority is enforced only against the live database comment (IsolationIT).
        assertDoesNotThrow(
                () -> PostgresTestSupport.requireCompleteOwnershipIdentity(name, otherValidToken, 16384L));
        assertNotEquals(PostgresTestSupport.ownershipComment(token),
                PostgresTestSupport.ownershipComment(otherValidToken));
        assertNotEquals(token, otherValidToken);
    }

    @Test
    void incompleteCleanupIsObservableThroughSuppressedFailures() {
        IllegalStateException primary = new IllegalStateException("primary cleanup failure");
        IllegalStateException secondary = new IllegalStateException("secondary fixture cleanup failure");
        Throwable retained = PostgresTestSupport.retainPrimaryFailure(primary, secondary);
        assertSame(primary, retained);
        assertEquals(1, retained.getSuppressed().length);
        assertEquals("secondary fixture cleanup failure", retained.getSuppressed()[0].getMessage());

        IllegalStateException wrapped = new IllegalStateException("External test PostgreSQL cleanup failed", primary);
        assertEquals(1, wrapped.getCause().getSuppressed().length);
        assertEquals("secondary fixture cleanup failure", wrapped.getCause().getSuppressed()[0].getMessage());
    }

    @Test
    void createFaultInjectionIsFixtureInstanceScopedNotGlobal() {
        PostgresTestSupport first = new PostgresTestSupport(PostgresTestSupport.EXTERNAL_MODE,
                PostgresTestSupport.EXTERNAL_JDBC_URL);
        PostgresTestSupport second = new PostgresTestSupport(PostgresTestSupport.EXTERNAL_MODE,
                PostgresTestSupport.EXTERNAL_JDBC_URL);
        assertEquals(PostgresTestSupport.CreateFaultInjection.Mode.NONE, first.createFaultInjection().mode());
        assertNull(first.createFaultInjection().witnessedOid());

        first.createFaultInjection().install(PostgresTestSupport.CreateFaultInjection.Mode.FAIL_OID_LOOKUP);
        assertEquals(PostgresTestSupport.CreateFaultInjection.Mode.FAIL_OID_LOOKUP,
                first.createFaultInjection().mode());
        // Second fixture remains inert; no cross-test/global switch.
        assertEquals(PostgresTestSupport.CreateFaultInjection.Mode.NONE, second.createFaultInjection().mode());
        assertNull(second.createFaultInjection().witnessedOid());

        first.createFaultInjection().install(
                PostgresTestSupport.CreateFaultInjection.Mode.FAIL_UNCHECKED_AFTER_CREATE_AND_CLOSE);
        assertEquals(PostgresTestSupport.CreateFaultInjection.Mode.FAIL_UNCHECKED_AFTER_CREATE_AND_CLOSE,
                first.createFaultInjection().mode());
        assertEquals(PostgresTestSupport.CreateFaultInjection.Mode.NONE, second.createFaultInjection().mode());

        first.createFaultInjection().reset();
        assertEquals(PostgresTestSupport.CreateFaultInjection.Mode.NONE, first.createFaultInjection().mode());
        assertNull(first.createFaultInjection().witnessedOid());
    }

    @Test
    void jdbcCloseAfterCreatePreservesOwnershipPathNotGenericEscape() {
        String name = PostgresTestSupport.generateOwnedDatabaseName();
        String token = "a".repeat(32);
        SQLException closeError = new SQLException("statement close after create");
        RuntimeException uncheckedClose = new IllegalStateException("unchecked close after create");

        Exception unresolved = PostgresTestSupport.ownershipPreservingCloseFailure(
                true, null, name, token, closeError);
        assertTrue(unresolved instanceof PostgresTestSupport.OwnedDatabaseUnresolvedCreateException);
        assertEquals(PostgresTestSupport.OwnedDatabaseUnresolvedCreateException.Reason.AMBIGUOUS_CREATE_OUTCOME,
                ((PostgresTestSupport.OwnedDatabaseUnresolvedCreateException) unresolved).reason);
        assertSame(closeError, unresolved.getCause());

        Exception uncheckedMapped = PostgresTestSupport.ownershipPreservingCloseFailure(
                true, null, name, token, uncheckedClose);
        assertTrue(uncheckedMapped instanceof PostgresTestSupport.OwnedDatabaseUnresolvedCreateException);
        assertSame(uncheckedClose, uncheckedMapped.getCause());

        Exception oidBound = PostgresTestSupport.ownershipPreservingCloseFailure(
                true, 16384L, name, token, closeError);
        assertTrue(oidBound instanceof PostgresTestSupport.OwnedDatabaseCreateException);
        assertEquals(16384L, ((PostgresTestSupport.OwnedDatabaseCreateException) oidBound).oid);
        assertSame(closeError, oidBound.getCause());

        Exception preDdl = PostgresTestSupport.ownershipPreservingCloseFailure(
                false, null, name, token, closeError);
        assertSame(closeError, preDdl);

        Exception preDdlUnchecked = PostgresTestSupport.ownershipPreservingCloseFailure(
                false, null, name, token, uncheckedClose);
        assertSame(uncheckedClose, preDdlUnchecked);
    }

    @Test
    void retainPrimaryFailureKeepsAssertionErrorAndAttachesCleanup() {
        AssertionError primary = new AssertionError("body assertion failed");
        RuntimeException cleanupOne = new IllegalStateException("first fixture cleanup failure");
        RuntimeException cleanupTwo = new IllegalStateException("second fixture cleanup failure");

        Throwable afterFirst = PostgresTestSupport.retainPrimaryFailure(primary, cleanupOne);
        Throwable afterSecond = PostgresTestSupport.retainPrimaryFailure(afterFirst, cleanupTwo);

        assertSame(primary, afterSecond);
        assertEquals(2, afterSecond.getSuppressed().length);
        assertEquals("first fixture cleanup failure", afterSecond.getSuppressed()[0].getMessage());
        assertEquals("second fixture cleanup failure", afterSecond.getSuppressed()[1].getMessage());

        // Cleanup-only path still surfaces when there was no body failure.
        Throwable cleanupOnly = PostgresTestSupport.retainPrimaryFailure(null, cleanupOne);
        assertSame(cleanupOne, cleanupOnly);
        Throwable stillCleanup = PostgresTestSupport.retainPrimaryFailure(cleanupOnly, cleanupTwo);
        assertSame(cleanupOne, stillCleanup);
        assertEquals(1, stillCleanup.getSuppressed().length);
        assertSame(cleanupTwo, stillCleanup.getSuppressed()[0]);
    }
}
