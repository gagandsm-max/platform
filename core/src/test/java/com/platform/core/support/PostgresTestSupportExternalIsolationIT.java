package com.platform.core.support;

import com.zaxxer.hikari.HikariDataSource;
import org.junit.jupiter.api.Test;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.Statement;
import java.util.Properties;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Live external-compose isolation proof: each helper start owns a fresh database on the
 * fixed private endpoint. Requires the exact PLATFORM_TEST_PG_* server; does not start Docker.
 *
 * <p>Named {@code *IT} (not {@code *Test}) so Maven Surefire default includes and the canonical
 * Day-02 {@code -Dgroups=postgres} Docker suite do not discover this class. Invoke explicitly,
 * for example {@code mvn -Dtest=PostgresTestSupportExternalIsolationIT test}. No silent skip /
 * assumption path: when targeted, absence of the private endpoint fails the acceptance.
 *
 * <p>Destructive fixture cleanup requires positive name/OID/marker proof inside this exclusive
 * private cluster. Missing OID is never permission to terminate sessions or drop. Hostname and
 * test credentials alone do not prove cluster ownership for residue beyond helper identity.
 */
class PostgresTestSupportExternalIsolationIT {

    @Test
    void twoStartsOwnIsolatedDatabasesAndRepeatedStopIsSafe() throws Exception {
        PostgresTestSupport first = new PostgresTestSupport(
                PostgresTestSupport.EXTERNAL_MODE, PostgresTestSupport.EXTERNAL_JDBC_URL);
        PostgresTestSupport second = new PostgresTestSupport(
                PostgresTestSupport.EXTERNAL_MODE, PostgresTestSupport.EXTERNAL_JDBC_URL);
        String firstDb = null;
        String secondDb = null;
        Throwable failure = null;
        try {
            first.start();
            second.start();
            firstDb = first.ownedDatabaseName();
            secondDb = second.ownedDatabaseName();
            assertNotNull(firstDb);
            assertNotNull(secondDb);
            assertNotEquals(firstDb, secondDb);
            assertNotNull(first.ownedDatabaseOid());
            assertNotNull(second.ownedDatabaseOid());
            assertNotEquals(first.ownedDatabaseOid(), second.ownedDatabaseOid());
            assertTrue(first.jdbcUrl().endsWith("/" + firstDb));
            assertTrue(second.jdbcUrl().endsWith("/" + secondDb));
            assertNotEquals(PostgresTestSupport.EXTERNAL_JDBC_URL, first.jdbcUrl());
            assertNotEquals(PostgresTestSupport.EXTERNAL_JDBC_URL, second.jdbcUrl());

            first.jdbc().execute("CREATE TABLE platform.isolation_marker (id INTEGER PRIMARY KEY)");
            first.jdbc().update("INSERT INTO platform.isolation_marker(id) VALUES (1)");
            assertEquals(1, first.jdbc().queryForObject(
                    "SELECT COUNT(*) FROM platform.isolation_marker", Integer.class));
            assertEquals(0, second.jdbc().queryForObject("""
                    SELECT COUNT(*) FROM information_schema.tables
                     WHERE table_schema='platform' AND table_name='isolation_marker'
                    """, Integer.class));

            first.migrate();
            assertEquals(1, first.jdbc().queryForObject(
                    "SELECT COUNT(*) FROM platform.schema_history WHERE version='V001'", Integer.class));
            assertEquals(0, second.jdbc().queryForObject("""
                    SELECT COUNT(*) FROM information_schema.tables
                     WHERE table_schema='platform' AND table_name='schema_history'
                    """, Integer.class));
            assertTrue(databaseExists(firstDb));
            assertTrue(databaseExists(secondDb));
        } catch (Throwable testFailure) {
            failure = testFailure;
        } finally {
            // Attempt each fixture cleanup independently; first stop failure must not skip second.
            failure = stopIndependent(failure, first);
            failure = stopIndependent(failure, first);
            failure = stopIndependent(failure, second);
            failure = stopIndependent(failure, second);
        }
        rethrowFixtureFailure(failure);
        assertNull(first.dataSource());
        assertNull(second.dataSource());
        assertNull(first.ownedDatabaseName());
        assertNull(second.ownedDatabaseName());
        assertNull(first.ownedDatabaseOid());
        assertNull(second.ownedDatabaseOid());
        assertFalse(databaseExists(firstDb));
        assertFalse(databaseExists(secondDb));
    }

    @Test
    void doubleStartFailsClosedAndStopAfterPoolCloseStillDropsOwnedDatabase() throws Exception {
        PostgresTestSupport pg = new PostgresTestSupport(
                PostgresTestSupport.EXTERNAL_MODE, PostgresTestSupport.EXTERNAL_JDBC_URL);
        Throwable failure = null;
        try {
            pg.start();
            String owned = pg.ownedDatabaseName();
            Long ownedOid = pg.ownedDatabaseOid();
            assertNotNull(owned);
            assertNotNull(ownedOid);
            assertTrue(databaseExists(owned));

            IllegalStateException alreadyStarted = assertThrows(IllegalStateException.class, pg::start);
            assertEquals("External test PostgreSQL helper already started", alreadyStarted.getMessage());
            assertEquals(owned, pg.ownedDatabaseName());
            assertEquals(ownedOid, pg.ownedDatabaseOid());
            assertNotNull(pg.dataSource());

            if (pg.dataSource() instanceof HikariDataSource hikari) {
                hikari.close();
            }
            pg.stop();
            pg.stop();
            assertNull(pg.dataSource());
            assertNull(pg.ownedDatabaseName());
            assertNull(pg.ownedDatabaseOid());
            assertFalse(databaseExists(owned));
        } catch (Throwable testFailure) {
            failure = testFailure;
        } finally {
            failure = stopIndependent(failure, pg);
            if (pg.ownedDatabaseName() != null) {
                IllegalStateException hold = new IllegalStateException(
                        "Fixture cleanup HOLD: owned database identity retained without verified drop");
                failure = PostgresTestSupport.retainPrimaryFailure(failure, hold);
            }
        }
        rethrowFixtureFailure(failure);
    }

    @Test
    void cleanupPropagatesWhenOwnershipProofIsMissingAndRetainsIdentity() throws Exception {
        PostgresTestSupport pg = new PostgresTestSupport(
                PostgresTestSupport.EXTERNAL_MODE, PostgresTestSupport.EXTERNAL_JDBC_URL);
        Throwable failure = null;
        String owned = null;
        Long ownedOid = null;
        String priorMarker = null;
        try {
            pg.start();
            owned = pg.ownedDatabaseName();
            ownedOid = pg.ownedDatabaseOid();
            String token = pg.ownershipToken();
            assertNotNull(owned);
            assertNotNull(ownedOid);
            assertNotNull(token);
            priorMarker = PostgresTestSupport.ownershipComment(token);

            clearOwnershipComment(owned, ownedOid);
            IllegalStateException stopFailure = assertThrows(IllegalStateException.class, pg::stop);
            assertEquals("External test PostgreSQL cleanup failed", stopFailure.getMessage());
            assertTrue(stopFailure.getCause() instanceof IllegalStateException);
            assertEquals("Refusing to drop unowned external test database", stopFailure.getCause().getMessage());
            // Retain identity after failed cleanup; never clear then name-only delete.
            assertEquals(owned, pg.ownedDatabaseName());
            assertEquals(ownedOid, pg.ownedDatabaseOid());
            assertNull(pg.dataSource());
            assertTrue(databaseExists(owned));

            // Second cleanup after first failure still refuses without marker proof.
            IllegalStateException second = assertThrows(IllegalStateException.class, pg::stop);
            assertEquals("External test PostgreSQL cleanup failed", second.getMessage());
            assertEquals(owned, pg.ownedDatabaseName());
        } catch (Throwable testFailure) {
            failure = testFailure;
        } finally {
            failure = restoreOwnershipMarkerIndependent(failure, owned, ownedOid, priorMarker);
            failure = stopIndependent(failure, pg);
            if (pg.ownedDatabaseName() != null) {
                failure = PostgresTestSupport.retainPrimaryFailure(failure, new IllegalStateException(
                        "Fixture cleanup HOLD: owned database identity retained without verified drop"));
            }
        }
        rethrowFixtureFailure(failure);
        assertNull(pg.ownedDatabaseName());
        assertFalse(databaseExists(owned));
    }

    @Test
    void mismatchedValidOwnershipMarkerRefusesDropWithoutDestroyingDatabase() throws Exception {
        PostgresTestSupport pg = new PostgresTestSupport(
                PostgresTestSupport.EXTERNAL_MODE, PostgresTestSupport.EXTERNAL_JDBC_URL);
        Throwable failure = null;
        String owned = null;
        Long ownedOid = null;
        String priorMarker = null;
        try {
            pg.start();
            owned = pg.ownedDatabaseName();
            ownedOid = pg.ownedDatabaseOid();
            String token = pg.ownershipToken();
            assertNotNull(owned);
            assertNotNull(ownedOid);
            assertNotNull(token);
            priorMarker = PostgresTestSupport.ownershipComment(token);

            // Different syntactically valid hex token — structural identity would accept it,
            // but live marker authority must refuse drop.
            String alienToken = token.charAt(0) == 'a' ? "b".repeat(32) : "a".repeat(32);
            assertNotEquals(token, alienToken);
            PostgresTestSupport.requireCompleteOwnershipIdentity(owned, alienToken, ownedOid);
            String alienMarker = PostgresTestSupport.ownershipComment(alienToken);
            assertNotEquals(priorMarker, alienMarker);
            replaceOwnershipComment(owned, ownedOid, alienMarker);

            IllegalStateException stopFailure = assertThrows(IllegalStateException.class, pg::stop);
            assertEquals("External test PostgreSQL cleanup failed", stopFailure.getMessage());
            assertEquals("Refusing to drop unowned external test database",
                    stopFailure.getCause().getMessage());
            assertEquals(owned, pg.ownedDatabaseName());
            assertEquals(ownedOid, pg.ownedDatabaseOid());
            assertEquals(token, pg.ownershipToken());
            assertTrue(databaseExists(owned));
            assertEquals(alienMarker, readOwnershipComment(owned, ownedOid));
        } catch (Throwable testFailure) {
            failure = testFailure;
        } finally {
            failure = restoreOwnershipMarkerIndependent(failure, owned, ownedOid, priorMarker);
            failure = stopIndependent(failure, pg);
            if (pg.ownedDatabaseName() != null) {
                failure = PostgresTestSupport.retainPrimaryFailure(failure, new IllegalStateException(
                        "Fixture cleanup HOLD: mismatched-marker fixture retained without verified drop"));
            }
        }
        rethrowFixtureFailure(failure);
        assertNull(pg.ownedDatabaseName());
        assertFalse(databaseExists(owned));
    }

    @Test
    void ambiguousCreateFaultRetainsIntentAndWitnessedOidRollbackOnly() throws Exception {
        PostgresTestSupport pg = new PostgresTestSupport(
                PostgresTestSupport.EXTERNAL_MODE, PostgresTestSupport.EXTERNAL_JDBC_URL);
        pg.createFaultInjection().install(
                PostgresTestSupport.CreateFaultInjection.Mode.LOSE_CREATE_RESPONSE);
        Throwable failure = null;
        try {
            PostgresTestSupport.OwnedDatabaseUnresolvedCreateException unresolved =
                    assertThrows(PostgresTestSupport.OwnedDatabaseUnresolvedCreateException.class, pg::start);
            assertEquals(PostgresTestSupport.OwnedDatabaseUnresolvedCreateException.Reason.AMBIGUOUS_CREATE_OUTCOME,
                    unresolved.reason);
            assertNotNull(pg.ownedDatabaseName());
            assertEquals(unresolved.databaseName, pg.ownedDatabaseName());
            assertNull(pg.ownedDatabaseOid());
            assertTrue(pg.ownedCreateUnresolved());
            IllegalStateException stopFailure = assertThrows(IllegalStateException.class, pg::stop);
            assertEquals("External test PostgreSQL cleanup failed", stopFailure.getMessage());
            assertEquals(pg.ownedDatabaseName(), unresolved.databaseName);
        } catch (Throwable testFailure) {
            failure = testFailure;
        } finally {
            failure = rollbackWitnessedIndependent(failure, pg);
        }
        rethrowFixtureFailure(failure);
        assertNull(pg.ownedDatabaseName());
    }

    @Test
    void oidLookupFaultRetainsIntentAndDoesNotNameDrop() throws Exception {
        PostgresTestSupport pg = new PostgresTestSupport(
                PostgresTestSupport.EXTERNAL_MODE, PostgresTestSupport.EXTERNAL_JDBC_URL);
        pg.createFaultInjection().install(
                PostgresTestSupport.CreateFaultInjection.Mode.FAIL_OID_LOOKUP);
        Throwable failure = null;
        try {
            PostgresTestSupport.OwnedDatabaseUnresolvedCreateException unresolved =
                    assertThrows(PostgresTestSupport.OwnedDatabaseUnresolvedCreateException.class, pg::start);
            assertEquals(PostgresTestSupport.OwnedDatabaseUnresolvedCreateException.Reason.OID_LOOKUP_FAILED,
                    unresolved.reason);
            assertNull(pg.ownedDatabaseOid());
            assertTrue(pg.ownedCreateUnresolved());
            assertThrows(IllegalStateException.class, pg::stop);
            assertNotNull(pg.ownedDatabaseName());
        } catch (Throwable testFailure) {
            failure = testFailure;
        } finally {
            failure = rollbackWitnessedIndependent(failure, pg);
        }
        rethrowFixtureFailure(failure);
        assertNull(pg.ownedDatabaseName());
    }

    @Test
    void statementCloseAfterCreateFaultRetainsUnresolvedIntent() throws Exception {
        PostgresTestSupport pg = new PostgresTestSupport(
                PostgresTestSupport.EXTERNAL_MODE, PostgresTestSupport.EXTERNAL_JDBC_URL);
        pg.createFaultInjection().install(
                PostgresTestSupport.CreateFaultInjection.Mode.FAIL_STATEMENT_CLOSE_AFTER_CREATE);
        Throwable failure = null;
        try {
            PostgresTestSupport.OwnedDatabaseUnresolvedCreateException unresolved =
                    assertThrows(PostgresTestSupport.OwnedDatabaseUnresolvedCreateException.class, pg::start);
            assertEquals(PostgresTestSupport.OwnedDatabaseUnresolvedCreateException.Reason.AMBIGUOUS_CREATE_OUTCOME,
                    unresolved.reason);
            assertTrue(pg.ownedCreateUnresolved());
            assertNull(pg.ownedDatabaseOid());
            assertThrows(IllegalStateException.class, pg::stop);
            assertNotNull(pg.ownedDatabaseName());
        } catch (Throwable testFailure) {
            failure = testFailure;
        } finally {
            failure = rollbackWitnessedIndependent(failure, pg);
        }
        rethrowFixtureFailure(failure);
        assertNull(pg.ownedDatabaseName());
    }

    @Test
    void uncheckedCreateAndCloseFaultRetainsUnresolvedIntentWithSuppressedClose() throws Exception {
        PostgresTestSupport pg = new PostgresTestSupport(
                PostgresTestSupport.EXTERNAL_MODE, PostgresTestSupport.EXTERNAL_JDBC_URL);
        pg.createFaultInjection().install(
                PostgresTestSupport.CreateFaultInjection.Mode.FAIL_UNCHECKED_AFTER_CREATE_AND_CLOSE);
        Throwable failure = null;
        try {
            PostgresTestSupport.OwnedDatabaseUnresolvedCreateException unresolved =
                    assertThrows(PostgresTestSupport.OwnedDatabaseUnresolvedCreateException.class, pg::start);
            assertEquals(PostgresTestSupport.OwnedDatabaseUnresolvedCreateException.Reason.AMBIGUOUS_CREATE_OUTCOME,
                    unresolved.reason);
            assertTrue(unresolved.getCause() instanceof RuntimeException);
            assertEquals("Injected unchecked failure after CREATE submission",
                    unresolved.getCause().getMessage());
            // Statement-close failure after CREATE attempt must be preserved as suppressed,
            // not replace the original CREATE-path error or clear retained intent.
            assertTrue(unresolved.getSuppressed().length >= 1);
            boolean sawClose = false;
            for (Throwable suppressed : unresolved.getSuppressed()) {
                Throwable probe = suppressed;
                while (probe != null) {
                    if (probe.getMessage() != null
                            && probe.getMessage().contains("unchecked statement close failure after CREATE")) {
                        sawClose = true;
                        break;
                    }
                    probe = probe.getCause();
                }
                if (sawClose) {
                    break;
                }
            }
            assertTrue(sawClose, "expected suppressed unchecked statement-close failure after CREATE");
            assertTrue(pg.ownedCreateUnresolved());
            assertNull(pg.ownedDatabaseOid());
            assertNotNull(pg.ownedDatabaseName());
            assertEquals(unresolved.databaseName, pg.ownedDatabaseName());
            IllegalStateException stopFailure = assertThrows(IllegalStateException.class, pg::stop);
            assertEquals("External test PostgreSQL cleanup failed", stopFailure.getMessage());
            assertNotNull(pg.ownedDatabaseName());
        } catch (Throwable testFailure) {
            failure = testFailure;
        } finally {
            failure = rollbackWitnessedIndependent(failure, pg);
        }
        rethrowFixtureFailure(failure);
        assertNull(pg.ownedDatabaseName());
    }

    @Test
    void connectionCloseAfterCreateFaultPreservesOidBoundOwnershipPath() throws Exception {
        PostgresTestSupport pg = new PostgresTestSupport(
                PostgresTestSupport.EXTERNAL_MODE, PostgresTestSupport.EXTERNAL_JDBC_URL);
        pg.createFaultInjection().install(
                PostgresTestSupport.CreateFaultInjection.Mode.FAIL_CONNECTION_CLOSE_AFTER_CREATE);
        Throwable failure = null;
        try {
            // Marker verified then connection close fails: OID-bound create failure, not generic clear.
            PostgresTestSupport.OwnedDatabaseCreateException createFailure =
                    assertThrows(PostgresTestSupport.OwnedDatabaseCreateException.class, pg::start);
            assertTrue(createFailure.oid > 0L);
            // Successful OID-bound rollback clears identity; failed rollback retains it.
            if (pg.ownedDatabaseName() != null) {
                assertEquals(createFailure.oid, pg.ownedDatabaseOid());
                assertThrows(IllegalStateException.class, pg::stop);
            }
        } catch (Throwable testFailure) {
            failure = testFailure;
        } finally {
            failure = closePoolIndependent(failure, pg);
            pg.createFaultInjection().reset();
            String name = pg.ownedDatabaseName();
            Long oid = pg.ownedDatabaseOid();
            if (name != null && oid != null) {
                try {
                    PostgresTestSupport.oidBoundCreateRollback(name, oid);
                    pg.clearOwnedIdentityAfterIndependentAbsenceProofForTest(oid);
                } catch (Throwable cleanupFailure) {
                    failure = PostgresTestSupport.retainPrimaryFailure(failure, cleanupFailure);
                }
            } else if (name != null) {
                failure = PostgresTestSupport.retainPrimaryFailure(failure, new IllegalStateException(
                        "Fixture cleanup HOLD: connection-close create failure without OID binding"));
            }
        }
        rethrowFixtureFailure(failure);
        assertNull(pg.ownedDatabaseName());
    }

    @Test
    void replacedOidUnderSameNameRefusesDropAndDoesNotDestroyReplacement() throws Exception {
        PostgresTestSupport pg = new PostgresTestSupport(
                PostgresTestSupport.EXTERNAL_MODE, PostgresTestSupport.EXTERNAL_JDBC_URL);
        Throwable failure = null;
        String owned = null;
        Long ownedOid = null;
        Long replacementOid = null;
        try {
            pg.start();
            owned = pg.ownedDatabaseName();
            ownedOid = pg.ownedDatabaseOid();
            String token = pg.ownershipToken();
            assertNotNull(owned);
            assertNotNull(ownedOid);
            assertNotNull(token);

            // Remove the original with positive ownership proof, then create a same-name replacement
            // that this helper must not destroy.
            PostgresTestSupport.oidBoundCreateRollback(owned, ownedOid);
            replacementOid = createUnmarkedDatabase(owned);
            assertNotEquals(ownedOid, replacementOid);

            IllegalStateException stopFailure = assertThrows(IllegalStateException.class, pg::stop);
            assertEquals("External test PostgreSQL cleanup failed", stopFailure.getMessage());
            assertEquals(owned, pg.ownedDatabaseName());
            assertEquals(ownedOid, pg.ownedDatabaseOid());
            assertTrue(databaseExists(owned));
            assertEquals(replacementOid, readDatabaseOid(owned));
        } catch (Throwable testFailure) {
            failure = testFailure;
        } finally {
            failure = closePoolIndependent(failure, pg);
            // Predicates and authorized replacement cleanup share one catch so probe failures
            // cannot escape finally (masking the primary error / skipping identity clear).
            try {
                if (replacementOid != null && owned != null && oidNameBound(owned, replacementOid)) {
                    PostgresTestSupport.oidBoundCreateRollback(owned, replacementOid);
                } else if (owned != null && databaseExists(owned)) {
                    throw new IllegalStateException(
                            "Fixture cleanup HOLD: replacement database lacks OID/name binding proof");
                }
            } catch (Throwable cleanupFailure) {
                failure = PostgresTestSupport.retainPrimaryFailure(failure, cleanupFailure);
            }
            // Release retained helper identity only after original OID + name are both absent.
            if (pg.ownedDatabaseName() != null && ownedOid != null) {
                try {
                    pg.clearOwnedIdentityAfterIndependentAbsenceProofForTest(ownedOid);
                } catch (Throwable clearFailure) {
                    failure = PostgresTestSupport.retainPrimaryFailure(failure, clearFailure);
                }
            } else if (pg.ownedDatabaseName() != null) {
                failure = PostgresTestSupport.retainPrimaryFailure(failure, new IllegalStateException(
                        "Fixture cleanup HOLD: retained identity without OID-bound absence proof"));
            }
        }
        rethrowFixtureFailure(failure);
    }

    private static Throwable stopIndependent(Throwable prior, PostgresTestSupport helper) {
        try {
            helper.stop();
            return prior;
        } catch (Throwable cleanupFailure) {
            return PostgresTestSupport.retainPrimaryFailure(prior, cleanupFailure);
        }
    }

    private static Throwable closePoolIndependent(Throwable prior, PostgresTestSupport helper) {
        try {
            if (helper.dataSource() instanceof HikariDataSource hikari) {
                hikari.close();
            }
            return prior;
        } catch (Throwable cleanupFailure) {
            return PostgresTestSupport.retainPrimaryFailure(prior, cleanupFailure);
        }
    }

    private static Throwable rollbackWitnessedIndependent(Throwable prior, PostgresTestSupport pg) {
        Throwable failure = closePoolIndependent(prior, pg);
        Long witnessed = pg.createFaultInjection().witnessedOid();
        String name = pg.ownedDatabaseName();
        pg.createFaultInjection().reset();
        if (name != null && witnessed != null) {
            try {
                PostgresTestSupport.oidBoundCreateRollback(name, witnessed);
                pg.clearOwnedIdentityAfterIndependentAbsenceProofForTest(witnessed);
            } catch (Throwable cleanupFailure) {
                failure = PostgresTestSupport.retainPrimaryFailure(failure, cleanupFailure);
            }
        } else if (name != null) {
            failure = PostgresTestSupport.retainPrimaryFailure(failure, new IllegalStateException(
                    "Fixture cleanup HOLD: unresolved create without witnessed OID binding"));
        }
        return failure;
    }

    private static Throwable restoreOwnershipMarkerIndependent(Throwable prior, String dbName, Long oid,
                                                               String priorMarker) {
        if (dbName == null || oid == null || priorMarker == null) {
            return prior;
        }
        try {
            restoreOwnershipMarkerOrHold(dbName, oid, priorMarker);
            return prior;
        } catch (Throwable cleanupFailure) {
            return PostgresTestSupport.retainPrimaryFailure(prior, cleanupFailure);
        }
    }

    private static void rethrowFixtureFailure(Throwable failure) throws Exception {
        if (failure == null) {
            return;
        }
        if (failure instanceof Error error) {
            throw error;
        }
        if (failure instanceof Exception exception) {
            throw exception;
        }
        throw new IllegalStateException("External test PostgreSQL fixture failed", failure);
    }

    private static boolean databaseExists(String dbName) throws Exception {
        Properties props = bootstrapProperties();
        try (Connection connection = DriverManager.getConnection(PostgresTestSupport.EXTERNAL_JDBC_URL, props);
             PreparedStatement statement = connection.prepareStatement(
                     "SELECT 1 FROM pg_database WHERE datname = ?")) {
            statement.setQueryTimeout(5);
            statement.setString(1, dbName);
            try (ResultSet rs = statement.executeQuery()) {
                return rs.next();
            }
        }
    }

    private static boolean oidNameBound(String dbName, long oid) throws Exception {
        try (Connection connection = DriverManager.getConnection(
                PostgresTestSupport.EXTERNAL_JDBC_URL, bootstrapProperties());
             PreparedStatement binding = connection.prepareStatement(
                     "SELECT datname FROM pg_catalog.pg_database WHERE oid = ?")) {
            binding.setQueryTimeout(5);
            binding.setLong(1, PostgresTestSupport.validateOwnedDatabaseOid(oid));
            try (ResultSet rs = binding.executeQuery()) {
                return rs.next() && PostgresTestSupport.validateOwnedDatabaseName(dbName).equals(rs.getString(1));
            }
        }
    }

    private static long readDatabaseOid(String dbName) throws Exception {
        try (Connection connection = DriverManager.getConnection(
                PostgresTestSupport.EXTERNAL_JDBC_URL, bootstrapProperties());
             PreparedStatement probe = connection.prepareStatement(
                     "SELECT oid FROM pg_catalog.pg_database WHERE datname = ?")) {
            probe.setQueryTimeout(5);
            probe.setString(1, PostgresTestSupport.validateOwnedDatabaseName(dbName));
            try (ResultSet rs = probe.executeQuery()) {
                if (!rs.next()) {
                    throw new IllegalStateException("Fixture database missing for OID read");
                }
                return PostgresTestSupport.validateOwnedDatabaseOid(rs.getLong(1));
            }
        }
    }

    private static long createUnmarkedDatabase(String dbName) throws Exception {
        String safe = PostgresTestSupport.validateOwnedDatabaseName(dbName);
        try (Connection connection = DriverManager.getConnection(
                PostgresTestSupport.EXTERNAL_JDBC_URL, bootstrapProperties())) {
            connection.setAutoCommit(true);
            try (Statement statement = connection.createStatement()) {
                statement.setQueryTimeout(15);
                statement.execute("CREATE DATABASE " + safe + " TEMPLATE template0");
            }
            try (PreparedStatement probe = connection.prepareStatement(
                    "SELECT oid FROM pg_catalog.pg_database WHERE datname = ?")) {
                probe.setQueryTimeout(5);
                probe.setString(1, safe);
                try (ResultSet rs = probe.executeQuery()) {
                    if (!rs.next()) {
                        throw new IllegalStateException("Replacement fixture database missing after create");
                    }
                    return PostgresTestSupport.validateOwnedDatabaseOid(rs.getLong(1));
                }
            }
        }
    }

    private static void clearOwnershipComment(String dbName, long oid) throws Exception {
        replaceOwnershipComment(dbName, oid, null);
    }

    private static void replaceOwnershipComment(String dbName, long oid, String markerOrNull) throws Exception {
        if (!oidNameBound(dbName, oid)) {
            throw new IllegalStateException("Fixture cleanup HOLD: refusing marker mutation without OID/name binding");
        }
        String safe = PostgresTestSupport.validateOwnedDatabaseName(dbName);
        String ddl = markerOrNull == null
                ? "COMMENT ON DATABASE " + safe + " IS NULL"
                : "COMMENT ON DATABASE " + safe + " IS '" + markerOrNull + "'";
        try (Connection connection = DriverManager.getConnection(
                PostgresTestSupport.EXTERNAL_JDBC_URL, bootstrapProperties());
             Statement statement = connection.createStatement()) {
            statement.setQueryTimeout(15);
            statement.execute(ddl);
        }
    }

    private static String readOwnershipComment(String dbName, long oid) throws Exception {
        if (!oidNameBound(dbName, oid)) {
            throw new IllegalStateException("Fixture cleanup HOLD: refusing marker read without OID/name binding");
        }
        try (Connection connection = DriverManager.getConnection(
                PostgresTestSupport.EXTERNAL_JDBC_URL, bootstrapProperties());
             PreparedStatement probe = connection.prepareStatement(
                     """
                     SELECT pg_catalog.shobj_description(d.oid, 'pg_database')
                       FROM pg_catalog.pg_database d
                      WHERE d.datname = ? AND d.oid = ?
                     """)) {
            probe.setQueryTimeout(5);
            probe.setString(1, PostgresTestSupport.validateOwnedDatabaseName(dbName));
            probe.setLong(2, PostgresTestSupport.validateOwnedDatabaseOid(oid));
            try (ResultSet rs = probe.executeQuery()) {
                if (!rs.next()) {
                    throw new IllegalStateException("Fixture database missing for marker read");
                }
                return rs.getString(1);
            }
        }
    }

    private static void restoreOwnershipMarkerOrHold(String dbName, Long oid, String priorMarker)
            throws Exception {
        if (dbName == null || oid == null || priorMarker == null) {
            throw new IllegalStateException("Fixture cleanup HOLD: missing name/OID/marker proof");
        }
        if (!oidNameBound(dbName, oid)) {
            throw new IllegalStateException(
                    "Fixture cleanup HOLD: original OID/name binding lost or replaced");
        }
        replaceOwnershipComment(dbName, oid, priorMarker);
    }

    private static Properties bootstrapProperties() {
        Properties props = new Properties();
        props.setProperty("user", "platform_app");
        props.setProperty("password", "platform_test");
        props.setProperty("connectTimeout", "2");
        props.setProperty("socketTimeout", "5");
        return props;
    }
}
