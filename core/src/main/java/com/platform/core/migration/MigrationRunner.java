package com.platform.core.migration;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.dao.DataAccessException;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.datasource.DataSourceUtils;
import org.springframework.stereotype.Component;
import org.springframework.transaction.support.TransactionTemplate;

import javax.sql.DataSource;
import java.sql.Connection;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

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
    private final JdbcTemplate jdbcTemplate;
    private final TransactionTemplate transactionTemplate;
    private final MigrationCatalog catalog;
    private final String applicationVersion;
    private final boolean enabled;

    public MigrationRunner(
            DataSource dataSource,
            JdbcTemplate jdbcTemplate,
            TransactionTemplate transactionTemplate,
            MigrationCatalog catalog,
            @Value("${platform.version:0.1.0-dev}") String applicationVersion,
            @Value("${platform.migrations.enabled:true}") boolean enabled
    ) {
        this.dataSource = dataSource;
        this.jdbcTemplate = jdbcTemplate;
        this.transactionTemplate = transactionTemplate;
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
        Connection connection = DataSourceUtils.getConnection(dataSource);
        boolean lockHeld = false;
        try {
            lockHeld = tryAdvisoryLock(connection);
            if (!lockHeld) {
                throw new MigrationException("MIGRATION_LOCK_FAILED", "Could not acquire Platform schema migration advisory lock");
            }
            ensureHistoryTable();
            List<MigrationDefinition> definitions = catalog.load();
            for (MigrationDefinition definition : definitions) {
                applyOne(definition);
            }
            MDC.put("event_code", "PLT-CORE-MIG-OK");
            log.info("Platform schema migrations up to date; applied catalog size={}", definitions.size());
            MDC.remove("event_code");
        } catch (MigrationException e) {
            MDC.put("event_code", "PLT-CORE-MIG-FAIL");
            MDC.put("error_code", e.getCode());
            log.error("Platform schema migration failed: {}", e.getMessage());
            MDC.remove("event_code");
            MDC.remove("error_code");
            throw e;
        } catch (DataAccessException e) {
            throw new MigrationException("DATABASE_UNAVAILABLE", "PostgreSQL unavailable during migration", e);
        } finally {
            if (lockHeld) {
                releaseAdvisoryLock(connection);
            }
            DataSourceUtils.releaseConnection(connection, dataSource);
        }
    }

    private void ensureHistoryTable() {
        jdbcTemplate.execute("""
                CREATE TABLE IF NOT EXISTS platform.schema_history (
                    version VARCHAR(64) PRIMARY KEY,
                    description VARCHAR(255) NOT NULL,
                    checksum VARCHAR(64) NOT NULL,
                    applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    application_version VARCHAR(64) NOT NULL
                )
                """);
    }

    private void applyOne(MigrationDefinition definition) {
        List<Map<String, Object>> existing = jdbcTemplate.queryForList(
                "SELECT version, checksum FROM platform.schema_history WHERE version = ?",
                definition.version()
        );
        if (!existing.isEmpty()) {
            String appliedChecksum = String.valueOf(existing.get(0).get("checksum"));
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
        transactionTemplate.executeWithoutResult(status -> {
            executeSqlScript(sql);
            jdbcTemplate.update(
                    """
                    INSERT INTO platform.schema_history(version, description, checksum, applied_at, application_version)
                    VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?)
                    """,
                    definition.version(),
                    definition.description(),
                    definition.checksum(),
                    applicationVersion
            );
        });
        log.info("Applied migration {} ({})", definition.version(), definition.description());
    }

    private void executeSqlScript(String sql) {
        List<String> statements = splitSqlStatements(sql);
        Connection connection = DataSourceUtils.getConnection(dataSource);
        try (Statement statement = connection.createStatement()) {
            for (String stmt : statements) {
                statement.execute(stmt);
            }
        } catch (SQLException e) {
            throw new MigrationException("MIGRATION_SQL_FAILED", "Failed executing migration SQL", e);
        } finally {
            DataSourceUtils.releaseConnection(connection, dataSource);
        }
    }

    /**
     * Split SQL on semicolons while ignoring those inside comments, quotes, and dollar-quotes.
     */
    static List<String> splitSqlStatements(String sql) {
        List<String> statements = new ArrayList<>();
        StringBuilder current = new StringBuilder();
        boolean inLineComment = false;
        boolean inBlockComment = false;
        boolean inSingleQuote = false;
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
            if (inBlockComment) {
                current.append(c);
                if (c == '*' && next == '/') {
                    current.append('/');
                    i++;
                    inBlockComment = false;
                }
                continue;
            }
            if (inSingleQuote) {
                current.append(c);
                if (c == '\'' && next == '\'') {
                    current.append('\'');
                    i++;
                } else if (c == '\'') {
                    inSingleQuote = false;
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
                inBlockComment = true;
                continue;
            }
            if (c == '\'') {
                current.append(c);
                inSingleQuote = true;
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
                int end = trimmed.indexOf("*/");
                if (end < 0) {
                    return trimmed;
                }
                remaining = trimmed.substring(end + 2);
                continue;
            }
            return trimmed;
        }
    }

    private boolean tryAdvisoryLock(Connection connection) {
        try {
            Boolean locked = jdbcTemplate.queryForObject(
                    "SELECT pg_try_advisory_lock(?, ?)",
                    Boolean.class,
                    LOCK_KEY_1,
                    LOCK_KEY_2
            );
            return Boolean.TRUE.equals(locked);
        } catch (DataAccessException e) {
            throw new MigrationException("DATABASE_UNAVAILABLE", "Failed to acquire migration lock", e);
        }
    }

    private void releaseAdvisoryLock(Connection connection) {
        try {
            jdbcTemplate.queryForObject(
                    "SELECT pg_advisory_unlock(?, ?)",
                    Boolean.class,
                    LOCK_KEY_1,
                    LOCK_KEY_2
            );
        } catch (DataAccessException e) {
            log.warn("Failed to release migration advisory lock: {}", e.getMessage());
        }
    }
}
