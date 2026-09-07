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
import java.sql.SQLException;
import java.sql.Statement;
import java.time.Duration;
import java.util.UUID;

/**
 * Ephemeral PostgreSQL helper for Day-02 Core integration tests.
 * Requires Docker. Does not use host PostgreSQL packages.
 */
public final class PostgresTestSupport {

    public static final String IMAGE = "postgres:16-alpine";

    private final String containerName;
    private final int hostPort;
    private DataSource dataSource;

    public PostgresTestSupport() {
        this.containerName = "platform-day02-pg-" + UUID.randomUUID().toString().substring(0, 8);
        this.hostPort = allocatePort();
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
        requireDockerAvailable();
        exec("docker", "run", "-d", "--rm",
                "--name", containerName,
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
        config.setPoolName("day02-pg-" + containerName.substring(containerName.length() - 8));
        this.dataSource = new HikariDataSource(config);
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
        if (dataSource instanceof HikariDataSource hikari) {
            hikari.close();
        }
        try {
            exec("docker", "rm", "-f", containerName);
        } catch (Exception ignored) {
            // best-effort cleanup
        }
    }

    public String jdbcUrl() {
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
}
