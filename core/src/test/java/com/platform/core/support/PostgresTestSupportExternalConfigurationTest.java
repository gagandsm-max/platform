package com.platform.core.support;

import org.junit.jupiter.api.Test;
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
        // Do not call default stop(): the default contract intentionally invokes Docker.
    }

    @Test
    void exactExternalSelectionAndRepeatedStopNeedNoDocker() {
        PostgresTestSupport pg = new PostgresTestSupport("external-compose",
                "jdbc:postgresql://platform-test-postgres:5432/platform");
        assertEquals(PostgresTestSupport.EXTERNAL_JDBC_URL, pg.jdbcUrl());
        pg.stop();
        pg.stop();
        assertNull(pg.dataSource());
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
}
