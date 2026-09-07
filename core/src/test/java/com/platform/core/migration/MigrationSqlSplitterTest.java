package com.platform.core.migration;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class MigrationSqlSplitterTest {

    @Test
    void preservesSemicolonsInQuotesDollarBodiesAndComments() {
        String sql = """
                -- leading ; comment
                SELECT 'semi;colon', 'it''s;still one string';
                SELECT "quoted;identifier", "escaped"";identifier";
                DO $body$
                BEGIN
                    PERFORM 'inside;body';
                END
                $body$;
                /* outer ; comment /* nested ; comment */ still comment */
                SELECT E'backslash\\';still quoted';
                SELECT 5 /* inline ; comment */;
                """;

        List<String> statements = MigrationRunner.splitSqlStatements(sql);

        assertEquals(5, statements.size());
        assertTrue(statements.get(0).contains("semi;colon"));
        assertTrue(statements.get(1).contains("quoted;identifier"));
        assertTrue(statements.get(2).contains("inside;body"));
        assertTrue(statements.get(3).contains("backslash\\';still quoted"));
        assertTrue(statements.get(4).startsWith("SELECT 5"));
    }

    @Test
    void supportsUntaggedDollarQuotesAndFunctionTags() {
        String sql = """
                CREATE OR REPLACE FUNCTION platform.test_fn() RETURNS void AS $$
                BEGIN
                    INSERT INTO platform.tenant(name) VALUES ('val;1');
                END;
                $$ LANGUAGE plpgsql;
                CREATE OR REPLACE FUNCTION platform.test_fn2() RETURNS void AS $func$
                BEGIN
                    PERFORM 'nested;call';
                END;
                $func$ LANGUAGE plpgsql;
                """;

        List<String> statements = MigrationRunner.splitSqlStatements(sql);

        assertEquals(2, statements.size());
        assertTrue(statements.get(0).contains("val;1"));
        assertTrue(statements.get(0).endsWith("LANGUAGE plpgsql"));
        assertTrue(statements.get(1).contains("nested;call"));
        assertTrue(statements.get(1).endsWith("LANGUAGE plpgsql"));
    }

    @Test
    void handlesEmptyAndCommentOnlyInput() {
        assertTrue(MigrationRunner.splitSqlStatements("").isEmpty());
        assertTrue(MigrationRunner.splitSqlStatements("   \n\t  ").isEmpty());
        assertTrue(MigrationRunner.splitSqlStatements("-- comment only\n").isEmpty());
        assertTrue(MigrationRunner.splitSqlStatements("/* block only */").isEmpty());
        assertTrue(MigrationRunner.splitSqlStatements("; ; ;").isEmpty());
    }
}
