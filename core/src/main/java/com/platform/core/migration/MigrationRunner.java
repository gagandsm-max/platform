package com.platform.core.migration;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.stereotype.Component;

import javax.sql.DataSource;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.List;

/**
 * Small Platform-owned SQL migration runner (Spring JDBC only).
 * Uses PostgreSQL advisory lock {@code platform.core.schema.migrate}.
 */
@Component
@Order(Ordered.HIGHEST_PRECEDENCE)
public class MigrationRunner implements ApplicationRunner {

    private static final Logger log = LoggerFactory.getLogger(MigrationRunner.class);

    /** Stable lock derived from Platform-specific name (two int keys). */
    static final int LOCK_KEY_1 = 0x504C5446; // 'PLTF'
    static final int LOCK_KEY_2 = 0x4D475254; // 'MGRT'

    private final DataSource dataSource;
    private final MigrationCatalog catalog;
    private final String applicationVersion;
    private final boolean enabled;

    public MigrationRunner(
            DataSource dataSource,
            MigrationCatalog catalog,
            @Value("${platform.version:0.1.0-dev}") String applicationVersion,
            @Value("${platform.migrations.enabled:true}") boolean enabled
    ) {
        this.dataSource = dataSource;
        this.catalog = catalog;
        this.applicationVersion = applicationVersion;
        this.enabled = enabled;
    }

    @Override
    public void run(ApplicationArguments args) {
        if (!enabled) {
            log.info("Platform schema migrations disabled by configuration");
            return;
        }
        applyPendingMigrations();
    }

    public void applyPendingMigrations() {
        try (Connection connection = dataSource.getConnection()) {
            boolean lockHeld = false;
            try {
                acquireAdvisoryLock(connection);
                lockHeld = true;
                ensureHistoryTable(connection);
                List<MigrationDefinition> definitions = catalog.load();
                for (MigrationDefinition definition : definitions) {
                    applyOne(connection, definition);
                }
                MDC.put("event_code", "PLT-CORE-MIG-OK");
                log.info("Platform schema migrations up to date; applied catalog size={}", definitions.size());
            } finally {
                MDC.remove("event_code");
                if (lockHeld) {
                    releaseAdvisoryLock(connection);
                }
            }
        } catch (MigrationException e) {
            MDC.put("event_code", "PLT-CORE-MIG-FAIL");
            MDC.put("error_code", e.getCode());
            try {
                log.error("Platform schema migration failed: {}", e.getMessage());
            } finally {
                MDC.remove("event_code");
                MDC.remove("error_code");
            }
            throw e;
        } catch (SQLException e) {
            throw new MigrationException("DATABASE_UNAVAILABLE", "PostgreSQL unavailable during migration", e);
        }
    }

    private void ensureHistoryTable(Connection connection) throws SQLException {
        try (Statement statement = connection.createStatement()) {
            statement.execute("""
                CREATE TABLE IF NOT EXISTS platform.schema_history (
                    version VARCHAR(64) PRIMARY KEY,
                    description VARCHAR(255) NOT NULL,
                    checksum VARCHAR(64) NOT NULL,
                    applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    application_version VARCHAR(64) NOT NULL
                )
                """);
        }
    }

    private void applyOne(Connection connection, MigrationDefinition definition) throws SQLException {
        String appliedChecksum = appliedChecksum(connection, definition.version());
        if (appliedChecksum != null) {
            if (!appliedChecksum.equalsIgnoreCase(definition.checksum())) {
                throw new MigrationException(
                        "MIGRATION_CHECKSUM_MISMATCH",
                        "Checksum mismatch for migration " + definition.version()
                                + ": expected=" + definition.checksum() + " applied=" + appliedChecksum
                );
            }
            return;
        }

        String sql = catalog.loadSql(definition);
        boolean originalAutoCommit = connection.getAutoCommit();
        connection.setAutoCommit(false);
        try {
            executeSqlScript(connection, sql);
            try (PreparedStatement statement = connection.prepareStatement("""
                    INSERT INTO platform.schema_history(version, description, checksum, applied_at, application_version)
                    VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?)
                    """)) {
                statement.setString(1, definition.version());
                statement.setString(2, definition.description());
                statement.setString(3, definition.checksum());
                statement.setString(4, applicationVersion);
                statement.executeUpdate();
            }
            connection.commit();
        } catch (SQLException | RuntimeException e) {
            rollback(connection, e);
            throw e;
        } finally {
            connection.setAutoCommit(originalAutoCommit);
        }
        log.info("Applied migration {} ({})", definition.version(), definition.description());
    }

    private static String appliedChecksum(Connection connection, String version) throws SQLException {
        try (PreparedStatement statement = connection.prepareStatement(
                "SELECT checksum FROM platform.schema_history WHERE version = ?")) {
            statement.setString(1, version);
            try (ResultSet result = statement.executeQuery()) {
                return result.next() ? result.getString("checksum") : null;
            }
        }
    }

    private void executeSqlScript(Connection connection, String sql) {
        List<String> statements = splitSqlStatements(sql);
        try (Statement statement = connection.createStatement()) {
            for (String stmt : statements) {
                statement.execute(stmt);
            }
        } catch (SQLException e) {
            if ("P0211".equals(e.getSQLState())) {
                throw new MigrationException("AGENT_INGRESS_PUBLISHER_UPGRADE_BLOCKED",
                        "Drain active Agent-ingress publisher ownership and leases before retrying V021", e);
            }
            throw new MigrationException("MIGRATION_SQL_FAILED", "Failed executing migration SQL", e);
        }
    }

    private static void rollback(Connection connection, Exception original) {
        try {
            connection.rollback();
        } catch (SQLException rollbackFailure) {
            original.addSuppressed(rollbackFailure);
        }
    }

    /**
     * Split SQL on semicolons while ignoring those inside comments, quotes, and dollar-quotes.
     */
    static List<String> splitSqlStatements(String sql) {
        List<String> statements = new ArrayList<>();
        StringBuilder current = new StringBuilder();
        boolean inLineComment = false;
        int blockCommentDepth = 0;
        boolean inSingleQuote = false;
        boolean backslashEscapes = false;
        boolean inDoubleQuote = false;
        String dollarTag = null;
        for (int i = 0; i < sql.length(); i++) {
            char c = sql.charAt(i);
            char next = i + 1 < sql.length() ? sql.charAt(i + 1) : '\0';
            if (dollarTag != null) {
                if (c == '$' && sql.startsWith(dollarTag, i)) {
                    current.append(dollarTag);
                    i += dollarTag.length() - 1;
                    dollarTag = null;
                } else {
                    current.append(c);
                }
                continue;
            }
            if (inLineComment) {
                current.append(c);
                if (c == '\n') {
                    inLineComment = false;
                }
                continue;
            }
            if (blockCommentDepth > 0) {
                current.append(c);
                if (c == '/' && next == '*') {
                    current.append('*');
                    i++;
                    blockCommentDepth++;
                } else if (c == '*' && next == '/') {
                    current.append('/');
                    i++;
                    blockCommentDepth--;
                }
                continue;
            }
            if (inSingleQuote) {
                current.append(c);
                if (c == '\'' && next == '\'') {
                    current.append('\'');
                    i++;
                } else if (c == '\'' && (!backslashEscapes || !isBackslashEscaped(sql, i))) {
                    inSingleQuote = false;
                    backslashEscapes = false;
                }
                continue;
            }
            if (inDoubleQuote) {
                current.append(c);
                if (c == '"' && next == '"') {
                    current.append('"');
                    i++;
                } else if (c == '"') {
                    inDoubleQuote = false;
                }
                continue;
            }
            if (c == '-' && next == '-') {
                current.append(c);
                inLineComment = true;
                continue;
            }
            if (c == '/' && next == '*') {
                current.append(c);
                blockCommentDepth = 1;
                continue;
            }
            if (c == '\'') {
                current.append(c);
                inSingleQuote = true;
                backslashEscapes = isEscapeStringPrefix(sql, i);
                continue;
            }
            if (c == '"') {
                current.append(c);
                inDoubleQuote = true;
                continue;
            }
            if (c == '$') {
                int end = sql.indexOf('$', i + 1);
                if (end > i) {
                    String tag = sql.substring(i, end + 1);
                    if (tag.matches("\\$[A-Za-z0-9_]*\\$")) {
                        dollarTag = tag;
                        current.append(tag);
                        i = end;
                        continue;
                    }
                }
            }
            if (c == ';') {
                String trimmed = stripLeadingComments(current.toString()).trim();
                if (!trimmed.isEmpty()) {
                    statements.add(trimmed);
                }
                current.setLength(0);
                continue;
            }
            current.append(c);
        }
        String trailing = stripLeadingComments(current.toString()).trim();
        if (!trailing.isEmpty()) {
            statements.add(trailing);
        }
        return statements;
    }

    private static boolean isBackslashEscaped(String sql, int position) {
        int count = 0;
        for (int i = position - 1; i >= 0 && sql.charAt(i) == '\\'; i--) {
            count++;
        }
        return (count & 1) == 1;
    }

    private static boolean isEscapeStringPrefix(String sql, int quotePosition) {
        if (quotePosition == 0 || (sql.charAt(quotePosition - 1) != 'E' && sql.charAt(quotePosition - 1) != 'e')) {
            return false;
        }
        return quotePosition == 1 || !Character.isJavaIdentifierPart(sql.charAt(quotePosition - 2));
    }

    private static String stripLeadingComments(String sqlFragment) {
        String remaining = sqlFragment;
        while (true) {
            String trimmed = remaining.stripLeading();
            if (trimmed.startsWith("--")) {
                int nl = trimmed.indexOf('\n');
                if (nl < 0) {
                    return "";
                }
                remaining = trimmed.substring(nl + 1);
                continue;
            }
            if (trimmed.startsWith("/*")) {
                int end = endOfLeadingBlockComment(trimmed);
                if (end < 0) {
                    return trimmed;
                }
                remaining = trimmed.substring(end);
                continue;
            }
            return trimmed;
        }
    }

    private static int endOfLeadingBlockComment(String value) {
        int depth = 0;
        for (int i = 0; i + 1 < value.length(); i++) {
            char c = value.charAt(i);
            char next = value.charAt(i + 1);
            if (c == '/' && next == '*') {
                depth++;
                i++;
            } else if (c == '*' && next == '/') {
                depth--;
                i++;
                if (depth == 0) {
                    return i + 1;
                }
            }
        }
        return -1;
    }

    private void acquireAdvisoryLock(Connection connection) {
        try (PreparedStatement statement = connection.prepareStatement(
                "SELECT pg_advisory_lock(?, ?)")) {
            statement.setInt(1, LOCK_KEY_1);
            statement.setInt(2, LOCK_KEY_2);
            statement.execute();
        } catch (SQLException e) {
            throw new MigrationException("DATABASE_UNAVAILABLE", "Failed to acquire migration lock", e);
        }
    }

    private void releaseAdvisoryLock(Connection connection) {
        try (PreparedStatement statement = connection.prepareStatement(
                "SELECT pg_advisory_unlock(?, ?)")) {
            statement.setInt(1, LOCK_KEY_1);
            statement.setInt(2, LOCK_KEY_2);
            try (ResultSet result = statement.executeQuery()) {
                if (!result.next() || !result.getBoolean(1)) {
                    log.warn("Platform schema migration advisory lock was not held at release");
                }
            }
        } catch (SQLException e) {
            log.warn("Failed to release Platform schema migration advisory lock");
        }
    }
}
