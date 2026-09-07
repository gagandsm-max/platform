package com.platform.core.migration;

import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;

import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class MigrationIntegrationTest {

    private PostgresTestSupport pg;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void blankDatabaseMigratesIdempotentlyWithChecksumProtection() {
        pg.migrate();
        JdbcTemplate jdbc = pg.jdbc();

        Integer tables = jdbc.queryForObject(
                """
                SELECT COUNT(*) FROM information_schema.tables
                WHERE table_schema='platform'
                  AND table_name IN ('tenant','site','entity_group','tag','entity','entity_identifier',
                                     'entity_attribute','entity_group_membership','entity_tag','schema_history')
                """,
                Integer.class
        );
        assertEquals(10, tables);

        List<Map<String, Object>> history = jdbc.queryForList(
                "SELECT version, checksum FROM platform.schema_history ORDER BY version"
        );
        assertEquals(1, history.size());
        assertEquals("V001", history.get(0).get("version"));
        String checksum = String.valueOf(history.get(0).get("checksum"));
        assertTrue(checksum.matches("[a-f0-9]{64}"));

        pg.migrate();
        assertEquals(1, jdbc.queryForObject("SELECT COUNT(*) FROM platform.schema_history", Integer.class));

        jdbc.update("UPDATE platform.schema_history SET checksum='deadbeef' WHERE version='V001'");
        MigrationException ex = assertThrows(MigrationException.class, pg::migrate);
        assertEquals("MIGRATION_CHECKSUM_MISMATCH", ex.getCode());
    }

    @Test
    void databaseConstraintsEnforceTenantIsolationAndIdentifierUniqueness() {
        pg.migrate();
        JdbcTemplate jdbc = pg.jdbc();

        jdbc.update("""
                INSERT INTO platform.tenant(tenant_id,name,mode,status,timezone,created_at,updated_at)
                VALUES
                ('018f0e0a-0000-7000-8000-00000000aaa1'::uuid,'A','SINGLE','ACTIVE','UTC',NOW(),NOW()),
                ('018f0e0a-0000-7000-8000-00000000bbb2'::uuid,'B','SINGLE','ACTIVE','UTC',NOW(),NOW())
                """);
        jdbc.update("""
                INSERT INTO platform.site(site_id,tenant_id,parent_site_id,name,description,timezone,created_at,updated_at)
                VALUES
                ('018f0e0a-0000-7000-8000-000000000001'::uuid,'018f0e0a-0000-7000-8000-00000000aaa1'::uuid,NULL,'SiteA',NULL,'UTC',NOW(),NOW()),
                ('018f0e0a-0000-7000-8000-000000000002'::uuid,'018f0e0a-0000-7000-8000-00000000bbb2'::uuid,NULL,'SiteB',NULL,'UTC',NOW(),NOW())
                """);
        jdbc.update("""
                INSERT INTO platform.entity(entity_id,tenant_id,entity_type,display_name,description,discovered_name,
                  lifecycle_state,site_id,criticality,owner_team_id,device_pack_id,device_pack_version,
                  first_seen_at,last_seen_at,created_by,created_at,updated_at)
                VALUES
                ('018f0e0a-0000-7000-8000-000000000011'::uuid,'018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                 'NETWORK_DEVICE','A1',NULL,NULL,'ACTIVE',NULL,NULL,NULL,NULL,NULL,NOW(),NOW(),'test',NOW(),NOW()),
                ('018f0e0a-0000-7000-8000-000000000012'::uuid,'018f0e0a-0000-7000-8000-00000000bbb2'::uuid,
                 'NETWORK_DEVICE','B1',NULL,NULL,'ACTIVE',NULL,NULL,NULL,NULL,NULL,NOW(),NOW(),'test',NOW(),NOW())
                """);

        // same IP different tenants OK
        jdbc.update("""
                INSERT INTO platform.entity_identifier(identifier_id,tenant_id,entity_id,identifier_type,namespace,
                  identifier_value,confidence,first_seen_at,last_seen_at)
                VALUES
                ('018f0e0a-0000-7000-8000-000000000021'::uuid,'018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                 '018f0e0a-0000-7000-8000-000000000011'::uuid,'IP','default','10.0.0.1',1.0,NOW(),NOW()),
                ('018f0e0a-0000-7000-8000-000000000022'::uuid,'018f0e0a-0000-7000-8000-00000000bbb2'::uuid,
                 '018f0e0a-0000-7000-8000-000000000012'::uuid,'IP','default','10.0.0.1',1.0,NOW(),NOW())
                """);

        // same tenant duplicate fails
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.entity_identifier(identifier_id,tenant_id,entity_id,identifier_type,namespace,
                  identifier_value,confidence,first_seen_at,last_seen_at)
                VALUES
                ('018f0e0a-0000-7000-8000-000000000023'::uuid,'018f0e0a-0000-7000-8000-00000000aaa1'::uuid,
                 '018f0e0a-0000-7000-8000-000000000011'::uuid,'IP','default','10.0.0.1',1.0,NOW(),NOW())
                """));

        // cross-tenant entity->site fails
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.entity SET site_id='018f0e0a-0000-7000-8000-000000000002'::uuid
                WHERE entity_id='018f0e0a-0000-7000-8000-000000000011'::uuid
                """));
    }
}
