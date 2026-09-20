package com.platform.core.domain;

import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;

@Tag("postgres")
class DatabaseIndexMigrationIntegrationTest {

    private PostgresTestSupport pg;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void identifierResolutionUsesUniqueConstraintWithoutRedundantLookupIndex() {
        Integer uniqueConstraintIndex = pg.jdbc().queryForObject(
                """
                SELECT COUNT(*)
                  FROM pg_indexes
                 WHERE schemaname = 'platform'
                   AND tablename = 'entity_identifier'
                   AND indexname = 'uq_identifier_resolution'
                """,
                Integer.class
        );
        Integer redundantIndex = pg.jdbc().queryForObject(
                """
                SELECT COUNT(*)
                  FROM pg_indexes
                 WHERE schemaname = 'platform'
                   AND tablename = 'entity_identifier'
                   AND indexname = 'idx_identifier_lookup'
                """,
                Integer.class
        );
        Integer migrationApplied = pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version = 'V009'",
                Integer.class
        );

        assertEquals(1, uniqueConstraintIndex);
        assertEquals(0, redundantIndex);
        assertEquals(1, migrationApplied);
    }
}
