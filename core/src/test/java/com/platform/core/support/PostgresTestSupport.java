package com.platform.core.support;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import com.platform.core.migration.MigrationCatalog;
import com.platform.core.migration.MigrationRunner;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;
import org.springframework.transaction.support.TransactionTemplate;

import javax.sql.DataSource;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.time.Duration;
import java.util.UUID;
import java.util.Properties;
import java.util.regex.Pattern;

/**
 * Ephemeral PostgreSQL helper for Day-02 Core integration tests.
 * Default mode requires Docker. Explicit external-compose mode uses JDBC only against
 * the fixed private Compose endpoint and creates one owned database per {@link #start()}.
 * Never uses host PostgreSQL packages.
 * Set both PLATFORM_TEST_PG_MODE=external-compose and PLATFORM_TEST_PG_JDBC_URL
 * to the exact EXTERNAL_JDBC_URL below; partial/unknown configuration fails closed.
 *
 * <p>Ephemeral storage policy: PostgreSQL data is mounted on tmpfs so the
 * image-declared VOLUME does not create anonymous Docker volumes. Teardown
 * uses {@code docker rm -fv} so any unexpected volume is still removed.
 *
 * <p>Residual race: {@code CREATE DATABASE} is not atomic with the client's receipt of
 * success. A transport/timeout failure after the server commits can leave a database while
 * this helper only holds create intent (name/token) without a verified OID or ownership
 * marker. A later name/prefix match or fresh OID lookup must not authorize destruction —
 * the name may already refer to a replacement database. Fixed JDBC hostname, URL shape, or
 * test credentials do not prove exclusive cluster ownership. Only the outer runtime owner of
 * a private, exclusively owned disposable test cluster may reconcile such residue by
 * destroying that cluster boundary.
 */
public final class PostgresTestSupport {

    public static final String IMAGE = "postgres:16-alpine";
    public static final String LABEL_LIFECYCLE = "com.platform.lifecycle=ephemeral";
    public static final String LABEL_OWNER = "com.platform.owner=test";
    public static final String PG_DATA_TMPFS = "/var/lib/postgresql/data:rw,exec,size=512m";

    private final String containerName;
    private final String testRunId;
    // Explicit test-only Compose contract; no arbitrary host, query options or credentials.
    static final String EXTERNAL_MODE = "external-compose";
    static final String EXTERNAL_JDBC_URL = "jdbc:postgresql://platform-test-postgres:5432/platform";
    private static final String EXTERNAL_JDBC_URL_PREFIX = "jdbc:postgresql://platform-test-postgres:5432/";
    private static final Pattern OWNED_DATABASE_NAME = Pattern.compile("^plt_t_[0-9a-f]{16}$");
    private static final String OWNERSHIP_COMMENT_PREFIX = "platform.test.fixture.owner=";
    private final String externalJdbcUrl;
    private final int hostPort;
    private final CreateFaultInjection createFaultInjection = new CreateFaultInjection();
    private DataSource dataSource;
    private String ownedDatabaseName;
    private String ownershipToken;
    private Long ownedDatabaseOid;
    /** True only after create + ownership marker verification succeeded. */
    private boolean ownedDatabaseVerified;
    /**
     * True when CREATE outcome is ambiguous or OID was never bound after create intent.
     * Destructive cleanup is refused until identity is independently proven absent.
     */
    private boolean ownedCreateUnresolved;

    public PostgresTestSupport() {
        this(System.getenv("PLATFORM_TEST_PG_MODE"), System.getenv("PLATFORM_TEST_PG_JDBC_URL"));
    }

    // Package-private seam for no-I/O configuration controls; not a production service API.
    PostgresTestSupport(String mode, String jdbcTarget) {
        this.externalJdbcUrl = externalTarget(mode, jdbcTarget);
        this.testRunId = UUID.randomUUID().toString().substring(0, 8);
        this.containerName = "platform-day02-pg-" + testRunId;
        this.hostPort = externalJdbcUrl == null ? allocatePort() : -1;
    }

    static String externalTarget(String mode, String jdbcTarget) {
        if (mode == null && jdbcTarget == null) {
            return null;
        }
        if (!EXTERNAL_MODE.equals(mode) || !EXTERNAL_JDBC_URL.equals(jdbcTarget)) {
            // Do not echo an invalid URL: it may contain credentials or hostile query text.
            throw new IllegalArgumentException("Invalid isolated external test PostgreSQL configuration");
        }
        return jdbcTarget;
    }

    static String validateOwnedDatabaseName(String name) {
        if (name == null || !OWNED_DATABASE_NAME.matcher(name).matches()) {
            throw new IllegalArgumentException("Invalid isolated external test database name");
        }
        return name;
    }

    static String generateOwnedDatabaseName() {
        String hex = UUID.randomUUID().toString().replace("-", "");
        return validateOwnedDatabaseName("plt_t_" + hex.substring(0, 16));
    }

    static String ownershipComment(String token) {
        if (token == null || !token.matches("[0-9a-f]{32}")) {
            throw new IllegalArgumentException("Invalid isolated external test ownership token");
        }
        return OWNERSHIP_COMMENT_PREFIX + token;
    }

    static long validateOwnedDatabaseOid(long oid) {
        if (oid <= 0L) {
            throw new IllegalArgumentException("Invalid isolated external test database oid");
        }
        return oid;
    }

    /**
     * Structural completeness only: name syntax, ownership-token syntax and positive OID.
     * Does <em>not</em> prove the token matches the live database ownership marker — that
     * authority is enforced only by {@link #assertOwnership} against the private cluster.
     * A different syntactically valid hex token remains structurally complete here.
     */
    static void requireCompleteOwnershipIdentity(String name, String token, Long oid) {
        validateOwnedDatabaseName(name);
        ownershipComment(token);
        if (oid == null) {
            throw new IllegalArgumentException("Incomplete isolated external test ownership identity");
        }
        validateOwnedDatabaseOid(oid);
    }

    /**
     * Preserve a primary body failure (including {@link AssertionError}) and attach an independent
     * cleanup failure. Never replaces the primary with a later cleanup exception.
     */
    static Throwable retainPrimaryFailure(Throwable primary, Throwable cleanupFailure) {
        if (cleanupFailure == null) {
            return primary;
        }
        if (primary == null) {
            return cleanupFailure;
        }
        primary.addSuppressed(cleanupFailure);
        return primary;
    }

    /**
     * Map JDBC close failures after CREATE onto the ownership-preservation path.
     * Pre-DDL close failures remain ordinary SQLExceptions. Close after create without a bound OID
     * becomes unresolved create intent. Close after an OID is bound becomes OID-retaining create failure.
     */
    static Exception ownershipPreservingCloseFailure(boolean createCompletedOrAmbiguous, Long boundOid,
                                                     String databaseName, String ownershipToken,
                                                     Exception closeError) {
        if (closeError == null) {
            throw new IllegalArgumentException("Close error required");
        }
        if (!createCompletedOrAmbiguous) {
            return closeError;
        }
        if (boundOid != null) {
            return new OwnedDatabaseCreateException(databaseName, boundOid, closeError);
        }
        return new OwnedDatabaseUnresolvedCreateException(
                databaseName, ownershipToken,
                OwnedDatabaseUnresolvedCreateException.Reason.AMBIGUOUS_CREATE_OUTCOME,
                closeError);
    }

    public String containerName() {
        return containerName;
    }

    public String testRunId() {
        return testRunId;
    }

    String ownedDatabaseName() {
        return ownedDatabaseName;
    }

    Long ownedDatabaseOid() {
        return ownedDatabaseOid;
    }

    String ownershipToken() {
        return ownershipToken;
    }

    boolean ownedDatabaseVerified() {
        return ownedDatabaseVerified;
    }

    boolean ownedCreateUnresolved() {
        return ownedCreateUnresolved;
    }

    /**
     * Fixture-instance fault-injection seam. Never shared across helpers or tests.
     */
    CreateFaultInjection createFaultInjection() {
        return createFaultInjection;
    }

    /**
     * Install unresolved create intent without JDBC for deterministic configuration tests.
     * Does not invent OID ownership and does not authorize destructive cleanup.
     */
    void applyUnresolvedCreateForTest(String name, String token,
                                      OwnedDatabaseUnresolvedCreateException.Reason reason) {
        if (externalJdbcUrl == null) {
            throw new IllegalStateException("Unresolved create fixture requires external-compose mode");
        }
        if (reason == null) {
            throw new IllegalArgumentException("Unresolved create reason required");
        }
        this.ownedDatabaseName = validateOwnedDatabaseName(name);
        ownershipComment(token);
        this.ownershipToken = token;
        this.ownedDatabaseOid = null;
        this.ownedDatabaseVerified = false;
        this.ownedCreateUnresolved = true;
        this.dataSource = null;
    }

    /**
     * Test-only clear after an independent OID-bound absence proof. Never use after name-only probes.
     * Verifies name and OID are both absent on the private endpoint before releasing retained identity.
     */
    void clearOwnedIdentityAfterIndependentAbsenceProofForTest(long provenAbsentOid) throws SQLException {
        String name = ownedDatabaseName;
        if (name == null) {
            throw new IllegalStateException(
                    "Independent absence proof requires retained owned database name");
        }
        long oid = validateOwnedDatabaseOid(provenAbsentOid);
        try (Connection connection = DriverManager.getConnection(EXTERNAL_JDBC_URL, externalProperties())) {
            assertDatabaseAbsent(connection, name, oid);
        }
        clearOwnedIdentity();
    }

    /**
     * OID-bound create-time rollback for exclusive-fixture tests that independently witnessed OID
     * at create time. Not authorized by name/prefix alone.
     */
    static void oidBoundCreateRollback(String dbName, long oid) throws SQLException {
        rollbackOwnedDatabase(dbName, oid);
    }

    private static int allocatePort() {
        try (java.net.ServerSocket socket = new java.net.ServerSocket(0)) {
            socket.setReuseAddress(true);
            return socket.getLocalPort();
        } catch (Exception e) {
            return 56000 + (int) (Math.random() * 2000);
        }
    }

    public void start() throws Exception {
        if (externalJdbcUrl != null) {
            startExternal();
            return;
        }
        requireDockerAvailable();
        exec("docker", "run", "-d", "--rm",
                "--name", containerName,
                "--tmpfs", PG_DATA_TMPFS,
                "--label", LABEL_LIFECYCLE,
                "--label", LABEL_OWNER,
                "--label", "com.platform.test-run=" + testRunId,
                "-e", "POSTGRES_USER=platform_app",
                "-e", "POSTGRES_PASSWORD=platform_test",
                "-e", "POSTGRES_DB=platform",
                "-p", hostPort + ":5432",
                IMAGE);
        waitReady();
        exec("docker", "exec", containerName, "psql", "-U", "platform_app", "-d", "platform",
                "-c", "CREATE SCHEMA IF NOT EXISTS platform;");
        HikariConfig config = new HikariConfig();
        config.setJdbcUrl(jdbcUrl());
        config.setUsername("platform_app");
        config.setPassword("platform_test");
        config.setMaximumPoolSize(4);
        config.setMinimumIdle(0);
        config.setPoolName("day02-pg-" + testRunId);
        this.dataSource = new HikariDataSource(config);
    }

    private void startExternal() throws Exception {
        if (dataSource != null || ownedDatabaseName != null || ownedDatabaseOid != null
                || ownedCreateUnresolved || ownershipToken != null) {
            throw new IllegalStateException("External test PostgreSQL helper already started");
        }
        waitExternalReady();
        // Record create intent before DDL so failed cleanup can retain a concrete identity.
        String dbName = generateOwnedDatabaseName();
        String token = UUID.randomUUID().toString().replace("-", "");
        this.ownedDatabaseName = dbName;
        this.ownershipToken = token;
        this.ownedDatabaseVerified = false;
        this.ownedCreateUnresolved = false;
        Long oid = null;
        try {
            oid = createOwnedDatabase(dbName, token);
            this.ownedDatabaseOid = oid;
            this.ownedDatabaseVerified = true;
            this.ownedCreateUnresolved = false;
            try (Connection connection = DriverManager.getConnection(ownedJdbcUrl(dbName), externalProperties());
                 Statement statement = connection.createStatement()) {
                statement.setQueryTimeout(5);
                statement.execute("CREATE SCHEMA IF NOT EXISTS platform;");
            }
            HikariConfig config = new HikariConfig();
            config.setJdbcUrl(ownedJdbcUrl(dbName));
            config.setUsername("platform_app");
            config.setPassword("platform_test");
            config.addDataSourceProperty("connectTimeout", "2");
            config.addDataSourceProperty("socketTimeout", "5");
            config.setConnectionTimeout(5000);
            config.setValidationTimeout(2000);
            config.setMaximumPoolSize(4);
            config.setMinimumIdle(0);
            config.setPoolName("day02-pg-" + testRunId);
            this.dataSource = new HikariDataSource(config);
        } catch (OwnedDatabaseUnresolvedCreateException unresolved) {
            // Preserve create intent on ambiguity / OID-bind failure. Do not invent ownership
            // from a fresh name→OID read and do not name-drop.
            this.dataSource = null;
            this.ownedDatabaseName = unresolved.databaseName;
            this.ownershipToken = unresolved.ownershipToken;
            this.ownedDatabaseOid = null;
            this.ownedDatabaseVerified = false;
            this.ownedCreateUnresolved = true;
            throw unresolved;
        } catch (OwnedDatabaseCreateException createFailure) {
            this.dataSource = null;
            this.ownedDatabaseOid = createFailure.oid;
            this.ownedDatabaseVerified = false;
            this.ownedCreateUnresolved = false;
            try {
                // Marker never verified: OID-bound create rollback only (not comment/name-only drop).
                rollbackOwnedDatabase(dbName, createFailure.oid);
                clearOwnedIdentity();
            } catch (Exception cleanup) {
                createFailure.addSuppressed(cleanup);
                // Retain name/token/oid after failed cleanup so a later stop() can retry.
            }
            throw createFailure;
        } catch (Exception failure) {
            this.dataSource = null;
            if (oid != null && ownedDatabaseVerified) {
                this.ownedDatabaseOid = oid;
                try {
                    dropOwnedDatabase(dbName, token, oid);
                    clearOwnedIdentity();
                } catch (Exception cleanup) {
                    failure.addSuppressed(cleanup);
                    // Failed/ambiguous drop retains identity until independently proven absent.
                }
            } else {
                // Pre-DDL failure only: CREATE was never known to complete. Unresolved create
                // outcomes are handled above and must not reach this clear path.
                clearOwnedIdentity();
            }
            throw failure;
        }
    }

    private void clearOwnedIdentity() {
        ownedDatabaseName = null;
        ownershipToken = null;
        ownedDatabaseOid = null;
        ownedDatabaseVerified = false;
        ownedCreateUnresolved = false;
    }

    private void waitExternalReady() throws Exception {
        long started = System.nanoTime();
        while (System.nanoTime() - started < Duration.ofSeconds(60).toNanos()) {
            try (Connection connection = DriverManager.getConnection(EXTERNAL_JDBC_URL, externalProperties());
                 Statement statement = connection.createStatement()) {
                statement.setQueryTimeout(5);
                statement.execute("SELECT 1");
                return;
            } catch (SQLException ignored) {
                // Bounded per-call connection/socket timeouts; no URL/SQL details in errors.
                Thread.sleep(500);
            }
        }
        throw new IllegalStateException("Isolated external test PostgreSQL did not become ready");
    }

    private long createOwnedDatabase(String dbName, String token) throws SQLException,
            OwnedDatabaseCreateException, OwnedDatabaseUnresolvedCreateException {
        String safeName = validateOwnedDatabaseName(dbName);
        String comment = ownershipComment(token);
        Connection connection = DriverManager.getConnection(EXTERNAL_JDBC_URL, externalProperties());
        boolean createCompletedOrAmbiguous = false;
        Long boundOid = null;
        Throwable outcome = null;
        Long successOid = null;
        try {
            connection.setAutoCommit(true);
            outcome = executeCreateDdl(connection, safeName, token);
            if (outcome == null) {
                // CREATE returned success to the client — continue OID/marker binding.
                createCompletedOrAmbiguous = true;
                try {
                    boundOid = createFaultInjection.readOidOrInject(connection, safeName);
                } catch (Exception oidFailure) {
                    // Known create completion path could not bind OID. Retain intent without inventing
                    // ownership via a later name lookup retry inside this helper.
                    outcome = new OwnedDatabaseUnresolvedCreateException(
                            safeName, token,
                            OwnedDatabaseUnresolvedCreateException.Reason.OID_LOOKUP_FAILED,
                            oidFailure);
                }
                if (outcome == null) {
                    try {
                        createFaultInjection.beforeMarkerVerify();
                        try (Statement statement = connection.createStatement()) {
                            statement.setQueryTimeout(15);
                            statement.execute("COMMENT ON DATABASE " + safeName + " IS '" + comment + "'");
                        }
                        assertOwnership(connection, safeName, comment, boundOid);
                        successOid = boundOid;
                    } catch (Exception commentOrVerifyFailure) {
                        try {
                            // Create-time rollback is OID-bound (not name-only after ambiguity).
                            rollbackCreatedDatabase(connection, safeName, boundOid);
                        } catch (Exception rollbackFailure) {
                            commentOrVerifyFailure.addSuppressed(rollbackFailure);
                            outcome = new OwnedDatabaseCreateException(
                                    safeName, boundOid, commentOrVerifyFailure);
                        }
                        if (outcome == null) {
                            if (commentOrVerifyFailure instanceof SQLException sqlException) {
                                outcome = sqlException;
                            } else if (commentOrVerifyFailure instanceof RuntimeException runtimeException) {
                                outcome = runtimeException;
                            } else {
                                outcome = new SQLException(
                                        "External test PostgreSQL ownership marker failed",
                                        commentOrVerifyFailure);
                            }
                        }
                    }
                }
            } else if (outcome instanceof OwnedDatabaseUnresolvedCreateException) {
                // CREATE was attempted; retain ambiguous/unresolved intent for connection-close mapping.
                createCompletedOrAmbiguous = true;
            }
            // else: genuine pre-DDL failure from executeCreateDdl — leave createCompletedOrAmbiguous false
            // so a later connection-close maps as an ordinary SQLException, not unresolved create.
        } catch (SQLException | RuntimeException unexpected) {
            if (createCompletedOrAmbiguous && boundOid == null) {
                outcome = new OwnedDatabaseUnresolvedCreateException(
                        safeName, token,
                        OwnedDatabaseUnresolvedCreateException.Reason.AMBIGUOUS_CREATE_OUTCOME,
                        unexpected);
            } else if (createCompletedOrAmbiguous) {
                outcome = new OwnedDatabaseCreateException(safeName, boundOid, unexpected);
            } else {
                outcome = unexpected;
            }
            successOid = null;
        } finally {
            outcome = closeCreateConnection(
                    connection, createCompletedOrAmbiguous, boundOid, safeName, token, outcome);
            if (outcome != null) {
                successOid = null;
            }
        }
        if (outcome != null) {
            return abortCreate(outcome);
        }
        if (successOid == null) {
            throw new IllegalStateException("External test PostgreSQL owned database create did not complete");
        }
        return successOid;
    }

    private Throwable executeCreateDdl(Connection connection, String safeName, String token)
            throws SQLException {
        Statement createStatement = connection.createStatement();
        Throwable createOutcome = null;
        // True once CREATE execution is attempted. Server may have committed even when the client
        // later observes SQLException or unchecked RuntimeException. Pre-DDL failures leave this false.
        boolean createAttemptedOrAmbiguous = false;
        try {
            createStatement.setQueryTimeout(15);
            // Name is regex-validated; PostgreSQL DDL identifiers cannot be bound as parameters.
            createAttemptedOrAmbiguous = true;
            try {
                createStatement.execute("CREATE DATABASE " + safeName + " TEMPLATE template0");
                // Model lost client response after server-accepted CREATE without binding OID
                // into helper ownership.
                createFaultInjection.simulateLostCreateResponseIfInstalled(connection, safeName);
                createFaultInjection.throwUncheckedAfterCreateSubmissionIfInstalled(connection, safeName);
            } catch (SQLException | RuntimeException createError) {
                // Client lost certainty after submission attempt: retain unresolved intent.
                // Preserve the original error; never name-drop from this failure alone.
                createOutcome = new OwnedDatabaseUnresolvedCreateException(
                        safeName, token,
                        OwnedDatabaseUnresolvedCreateException.Reason.AMBIGUOUS_CREATE_OUTCOME,
                        createError);
            }
        } catch (SQLException | RuntimeException preDdlOrEscaped) {
            if (createAttemptedOrAmbiguous) {
                // Should be rare (outer envelope); still treat as unresolved once CREATE was attempted.
                if (createOutcome == null) {
                    createOutcome = new OwnedDatabaseUnresolvedCreateException(
                            safeName, token,
                            OwnedDatabaseUnresolvedCreateException.Reason.AMBIGUOUS_CREATE_OUTCOME,
                            preDdlOrEscaped);
                } else {
                    createOutcome.addSuppressed(preDdlOrEscaped);
                }
            } else {
                createOutcome = preDdlOrEscaped;
            }
        } finally {
            Exception closeError = null;
            try {
                createFaultInjection.beforeStatementCloseAfterCreate(
                        connection, safeName, createAttemptedOrAmbiguous);
            } catch (Exception injected) {
                closeError = injected;
            }
            try {
                createStatement.close();
            } catch (Exception realClose) {
                if (closeError == null) {
                    closeError = realClose;
                } else {
                    closeError.addSuppressed(realClose);
                }
            }
            if (closeError != null) {
                Exception mapped = ownershipPreservingCloseFailure(
                        createAttemptedOrAmbiguous, null, safeName, token, closeError);
                if (createOutcome != null) {
                    createOutcome.addSuppressed(mapped);
                } else {
                    createOutcome = mapped;
                }
            }
        }
        return createOutcome;
    }

    private static long abortCreate(Throwable outcome) throws SQLException,
            OwnedDatabaseCreateException, OwnedDatabaseUnresolvedCreateException {
        if (outcome instanceof OwnedDatabaseUnresolvedCreateException unresolved) {
            throw unresolved;
        }
        if (outcome instanceof OwnedDatabaseCreateException createFailure) {
            throw createFailure;
        }
        if (outcome instanceof SQLException sqlException) {
            throw sqlException;
        }
        if (outcome instanceof RuntimeException runtimeException) {
            throw runtimeException;
        }
        if (outcome instanceof Error error) {
            throw error;
        }
        throw new SQLException("External test PostgreSQL owned database create failed", outcome);
    }

    private Throwable closeCreateConnection(Connection connection, boolean createCompletedOrAmbiguous,
                                            Long boundOid, String safeName, String token,
                                            Throwable primaryOutcome) {
        if (connection == null) {
            return primaryOutcome;
        }
        Exception closeError = null;
        try {
            createFaultInjection.beforeConnectionCloseAfterCreate();
        } catch (Exception injected) {
            closeError = injected;
        }
        try {
            connection.close();
        } catch (Exception realClose) {
            if (closeError == null) {
                closeError = realClose;
            } else {
                closeError.addSuppressed(realClose);
            }
        }
        if (closeError == null) {
            return primaryOutcome;
        }
        Exception mapped = ownershipPreservingCloseFailure(
                createCompletedOrAmbiguous, boundOid, safeName, token, closeError);
        if (primaryOutcome != null) {
            primaryOutcome.addSuppressed(mapped);
            return primaryOutcome;
        }
        return mapped;
    }

    private static long readDatabaseOid(Connection connection, String dbName) throws SQLException {
        try (PreparedStatement probe = connection.prepareStatement(
                """
                SELECT d.oid
                  FROM pg_catalog.pg_database d
                 WHERE d.datname = ?
                """)) {
            probe.setQueryTimeout(5);
            probe.setString(1, dbName);
            try (ResultSet rs = probe.executeQuery()) {
                if (!rs.next()) {
                    throw new IllegalStateException("External test PostgreSQL owned database missing after create");
                }
                return validateOwnedDatabaseOid(rs.getLong(1));
            }
        }
    }

    private static void dropOwnedDatabase(String dbName, String token, long oid) throws SQLException {
        requireCompleteOwnershipIdentity(dbName, token, oid);
        String safeName = validateOwnedDatabaseName(dbName);
        String expectedComment = ownershipComment(token);
        long expectedOid = validateOwnedDatabaseOid(oid);
        try (Connection connection = DriverManager.getConnection(EXTERNAL_JDBC_URL, externalProperties())) {
            connection.setAutoCommit(true);
            assertOwnership(connection, safeName, expectedComment, expectedOid);
            terminateBackends(connection, safeName);
            try (Statement statement = connection.createStatement()) {
                statement.setQueryTimeout(15);
                statement.execute("DROP DATABASE " + safeName);
            }
            assertDatabaseAbsent(connection, safeName, expectedOid);
        }
    }

    private static void rollbackOwnedDatabase(String dbName, long oid) throws SQLException {
        try (Connection connection = DriverManager.getConnection(EXTERNAL_JDBC_URL, externalProperties())) {
            connection.setAutoCommit(true);
            rollbackCreatedDatabase(connection, dbName, oid);
        }
    }

    /**
     * OID-bound create-time rollback only. Does not authorize name-only deletion and does not
     * require the ownership comment (comment/verify may be what failed).
     */
    private static void rollbackCreatedDatabase(Connection connection, String dbName, long oid)
            throws SQLException {
        String safeName = validateOwnedDatabaseName(dbName);
        long expectedOid = validateOwnedDatabaseOid(oid);
        assertOidNameBinding(connection, safeName, expectedOid);
        terminateBackends(connection, safeName);
        try (Statement statement = connection.createStatement()) {
            statement.setQueryTimeout(15);
            statement.execute("DROP DATABASE " + safeName);
        }
        assertDatabaseAbsent(connection, safeName, expectedOid);
    }

    private static void terminateBackends(Connection connection, String dbName) throws SQLException {
        try (PreparedStatement terminate = connection.prepareStatement(
                """
                SELECT pg_terminate_backend(pid)
                  FROM pg_stat_activity
                 WHERE datname = ?
                   AND pid <> pg_backend_pid()
                """)) {
            terminate.setQueryTimeout(10);
            terminate.setString(1, dbName);
            terminate.executeQuery().close();
        }
    }

    private static void assertOwnership(Connection connection, String dbName, String expectedComment, long expectedOid)
            throws SQLException {
        assertOidNameBinding(connection, dbName, expectedOid);
        try (PreparedStatement probe = connection.prepareStatement(
                """
                SELECT d.oid, pg_catalog.shobj_description(d.oid, 'pg_database')
                  FROM pg_catalog.pg_database d
                 WHERE d.datname = ?
                """)) {
            probe.setQueryTimeout(5);
            probe.setString(1, dbName);
            try (ResultSet rs = probe.executeQuery()) {
                if (!rs.next()) {
                    throw new IllegalStateException("External test PostgreSQL owned database missing");
                }
                long oid = rs.getLong(1);
                String comment = rs.getString(2);
                if (oid != expectedOid) {
                    throw new IllegalStateException("Refusing to drop external test database with OID mismatch");
                }
                if (!expectedComment.equals(comment)) {
                    throw new IllegalStateException("Refusing to drop unowned external test database");
                }
            }
        }
    }

    private static void assertOidNameBinding(Connection connection, String dbName, long expectedOid)
            throws SQLException {
        try (PreparedStatement probe = connection.prepareStatement(
                """
                SELECT d.datname
                  FROM pg_catalog.pg_database d
                 WHERE d.oid = ?
                """)) {
            probe.setQueryTimeout(5);
            probe.setLong(1, expectedOid);
            try (ResultSet rs = probe.executeQuery()) {
                if (!rs.next()) {
                    throw new IllegalStateException("External test PostgreSQL owned database OID missing");
                }
                String actualName = rs.getString(1);
                if (!dbName.equals(actualName)) {
                    throw new IllegalStateException("Refusing to drop external test database with OID mismatch");
                }
            }
        }
    }

    private static void assertDatabaseAbsent(Connection connection, String dbName, long oid) throws SQLException {
        try (PreparedStatement byName = connection.prepareStatement(
                "SELECT 1 FROM pg_catalog.pg_database WHERE datname = ?")) {
            byName.setQueryTimeout(5);
            byName.setString(1, dbName);
            try (ResultSet rs = byName.executeQuery()) {
                if (rs.next()) {
                    throw new IllegalStateException("External test PostgreSQL owned database still present after drop");
                }
            }
        }
        try (PreparedStatement byOid = connection.prepareStatement(
                "SELECT 1 FROM pg_catalog.pg_database WHERE oid = ?")) {
            byOid.setQueryTimeout(5);
            byOid.setLong(1, oid);
            try (ResultSet rs = byOid.executeQuery()) {
                if (rs.next()) {
                    throw new IllegalStateException("External test PostgreSQL owned database still present after drop");
                }
            }
        }
    }

    private static String ownedJdbcUrl(String dbName) {
        return EXTERNAL_JDBC_URL_PREFIX + validateOwnedDatabaseName(dbName);
    }

    private static Properties externalProperties() {
        Properties props = new Properties();
        props.setProperty("user", "platform_app");
        props.setProperty("password", "platform_test");
        props.setProperty("connectTimeout", "2");
        props.setProperty("socketTimeout", "5");
        return props;
    }

    public static void requireDockerAvailable() {
        try {
            Process p = new ProcessBuilder("docker", "info").redirectErrorStream(true).start();
            boolean finished = p.waitFor(5, java.util.concurrent.TimeUnit.SECONDS);
            if (!finished || p.exitValue() != 0) {
                throw new IllegalStateException("Docker engine required for PostgreSQL integration tests");
            }
        } catch (Exception e) {
            throw new IllegalStateException(
                    "Docker engine required for PostgreSQL integration tests: " + e.getMessage(), e);
        }
    }

    public void stop() {
        if (externalJdbcUrl != null) {
            stopExternal();
            return;
        }
        if (dataSource instanceof HikariDataSource hikari) {
            hikari.close();
        }
        dataSource = null;
        try {
            // -v removes anonymous volumes if any were created despite tmpfs.
            exec("docker", "rm", "-fv", containerName);
        } catch (Exception ignored) {
            // best-effort cleanup
        }
    }

    private void stopExternal() {
        Exception failure = null;
        if (dataSource instanceof HikariDataSource hikari) {
            try {
                hikari.close();
            } catch (Exception closeFailure) {
                failure = closeFailure;
            }
        }
        dataSource = null;
        String dbName = ownedDatabaseName;
        String token = ownershipToken;
        Long oid = ownedDatabaseOid;
        boolean verified = ownedDatabaseVerified;
        boolean unresolved = ownedCreateUnresolved;
        if (dbName == null && token == null && oid == null) {
            if (failure != null) {
                throw new IllegalStateException("External test PostgreSQL cleanup failed", failure);
            }
            return;
        }
        try {
            if (verified) {
                requireCompleteOwnershipIdentity(dbName, token, oid);
                dropOwnedDatabase(dbName, token, oid);
            } else if (unresolved || oid == null) {
                // Ambiguous/incomplete create: never terminate/drop by name or missing OID.
                throw new IllegalStateException(
                        "Refusing to drop external test database with unresolved create outcome; identity retained until independently proven absent");
            } else if (dbName != null) {
                // Incomplete create with known OID: OID-bound rollback only. Never prefix/name-only.
                rollbackOwnedDatabase(dbName, oid);
            } else {
                throw new IllegalStateException(
                        "Refusing to drop external test database without complete ownership identity");
            }
            clearOwnedIdentity();
        } catch (Exception dropFailure) {
            // Retain identity after failed cleanup; never fall back to prefix/name-only deletion.
            if (failure == null) {
                failure = dropFailure;
            } else {
                failure.addSuppressed(dropFailure);
            }
        }
        if (failure != null) {
            throw new IllegalStateException("External test PostgreSQL cleanup failed", failure);
        }
    }

    public String jdbcUrl() {
        if (externalJdbcUrl != null) {
            if (ownedDatabaseName != null) {
                return ownedJdbcUrl(ownedDatabaseName);
            }
            return externalJdbcUrl;
        }
        return "jdbc:postgresql://127.0.0.1:" + hostPort + "/platform";
    }

    public DataSource dataSource() {
        return dataSource;
    }

    public JdbcTemplate jdbc() {
        return new JdbcTemplate(dataSource);
    }

    public NamedParameterJdbcTemplate namedJdbc() {
        return new NamedParameterJdbcTemplate(dataSource);
    }

    public TransactionTemplate tx() {
        return new TransactionTemplate(new DataSourceTransactionManager(dataSource));
    }

    public void migrate() {
        MigrationRunner runner = new MigrationRunner(
                dataSource,
                new MigrationCatalog(),
                "0.1.0-dev-test",
                true
        );
        runner.applyPendingMigrations();
    }

    public MigrationRunner runner(MigrationCatalog catalog) {
        return new MigrationRunner(dataSource, catalog, "0.1.0-dev-test", true);
    }

    private void waitReady() throws Exception {
        long deadline = System.currentTimeMillis() + Duration.ofSeconds(60).toMillis();
        SQLException last = null;
        while (System.currentTimeMillis() < deadline) {
            try (Connection c = DriverManager.getConnection(jdbcUrl(), "platform_app", "platform_test");
                 Statement s = c.createStatement()) {
                s.execute("SELECT 1");
                return;
            } catch (SQLException e) {
                last = e;
                Thread.sleep(500);
            }
        }
        throw new IllegalStateException("PostgreSQL container did not become ready", last);
    }

    private static void exec(String... command) throws Exception {
        Process p = new ProcessBuilder(command).redirectErrorStream(true).start();
        int code = p.waitFor();
        if (code != 0) {
            String out = new String(p.getInputStream().readAllBytes());
            throw new IllegalStateException("Command failed (" + code + "): " + String.join(" ", command) + "\n" + out);
        }
    }

    /**
     * Deterministic fault-injection seam scoped to one fixture instance. Default is inert.
     * A witnessed OID is for exclusive-fixture test cleanup only and is never treated as
     * helper ownership after an unresolved create. No cross-test or global switches.
     */
    static final class CreateFaultInjection {
        enum Mode {
            NONE,
            LOSE_CREATE_RESPONSE,
            FAIL_OID_LOOKUP,
            FAIL_MARKER_VERIFY,
            FAIL_STATEMENT_CLOSE_AFTER_CREATE,
            FAIL_CONNECTION_CLOSE_AFTER_CREATE,
            /** Unchecked RuntimeException after CREATE submission, plus unchecked statement-close. */
            FAIL_UNCHECKED_AFTER_CREATE_AND_CLOSE
        }

        private Mode mode = Mode.NONE;
        private Long witnessedOid;

        void reset() {
            mode = Mode.NONE;
            witnessedOid = null;
        }

        void install(Mode next) {
            mode = next == null ? Mode.NONE : next;
            witnessedOid = null;
        }

        Mode mode() {
            return mode;
        }

        Long witnessedOid() {
            return witnessedOid;
        }

        private void simulateLostCreateResponseIfInstalled(Connection connection, String safeName)
                throws SQLException {
            if (mode != Mode.LOSE_CREATE_RESPONSE) {
                return;
            }
            witnessedOid = readDatabaseOid(connection, safeName);
            throw new SQLException("Injected lost CREATE response");
        }

        private void throwUncheckedAfterCreateSubmissionIfInstalled(Connection connection, String safeName) {
            if (mode != Mode.FAIL_UNCHECKED_AFTER_CREATE_AND_CLOSE) {
                return;
            }
            try {
                witnessedOid = readDatabaseOid(connection, safeName);
            } catch (SQLException oidReadFailure) {
                throw new IllegalStateException(
                        "Injected unchecked failure after CREATE submission (OID witness failed)",
                        oidReadFailure);
            }
            throw new IllegalStateException("Injected unchecked failure after CREATE submission");
        }

        private long readOidOrInject(Connection connection, String safeName) throws Exception {
            if (mode == Mode.FAIL_OID_LOOKUP) {
                witnessedOid = readDatabaseOid(connection, safeName);
                throw new SQLException("Injected OID lookup failure");
            }
            return readDatabaseOid(connection, safeName);
        }

        private void beforeMarkerVerify() throws SQLException {
            if (mode == Mode.FAIL_MARKER_VERIFY) {
                throw new SQLException("Injected ownership marker verification failure");
            }
        }

        private void beforeStatementCloseAfterCreate(Connection connection, String safeName,
                                                     boolean createAttemptedOrAmbiguous)
                throws SQLException {
            if (mode == Mode.FAIL_STATEMENT_CLOSE_AFTER_CREATE) {
                // Witness OID for exclusive-fixture cleanup only; never bind as helper ownership.
                witnessedOid = readDatabaseOid(connection, safeName);
                throw new SQLException("Injected statement close failure after CREATE");
            }
            if (mode == Mode.FAIL_UNCHECKED_AFTER_CREATE_AND_CLOSE && createAttemptedOrAmbiguous) {
                if (witnessedOid == null) {
                    witnessedOid = readDatabaseOid(connection, safeName);
                }
                throw new IllegalStateException("Injected unchecked statement close failure after CREATE");
            }
        }

        private void beforeConnectionCloseAfterCreate() throws SQLException {
            if (mode == Mode.FAIL_CONNECTION_CLOSE_AFTER_CREATE) {
                throw new SQLException("Injected connection close failure after CREATE");
            }
        }
    }

    /**
     * CREATE completed with uncertain client-side finality, or OID could not be bound.
     * Callers retain name/token create intent and must not name-drop.
     */
    static final class OwnedDatabaseUnresolvedCreateException extends Exception {
        enum Reason {
            AMBIGUOUS_CREATE_OUTCOME,
            OID_LOOKUP_FAILED
        }

        final String databaseName;
        final String ownershipToken;
        final Reason reason;

        OwnedDatabaseUnresolvedCreateException(String databaseName, String ownershipToken,
                                               Reason reason, Throwable cause) {
            super("External test PostgreSQL owned database create outcome is unresolved ("
                    + reason + ")", cause);
            this.databaseName = databaseName;
            this.ownershipToken = ownershipToken;
            this.reason = reason;
        }
    }

    /**
     * Create produced an OID-bound database but ownership marker/rollback could not finish.
     * Callers retain identity for a later ownership-verified cleanup retry.
     */
    static final class OwnedDatabaseCreateException extends Exception {
        final String databaseName;
        final long oid;

        OwnedDatabaseCreateException(String databaseName, long oid, Throwable cause) {
            super("External test PostgreSQL owned database create could not be finalized", cause);
            this.databaseName = databaseName;
            this.oid = oid;
        }
    }
}
