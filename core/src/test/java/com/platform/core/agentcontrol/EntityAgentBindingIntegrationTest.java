package com.platform.core.agentcontrol;

import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.dao.DataIntegrityViolationException;

import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;

/** Real PostgreSQL constraints/query tests; certificate fixtures are not enrollment runtime proof. */
@Tag("postgres")
class EntityAgentBindingIntegrationTest {
    private PostgresTestSupport pg;
    private AgentControlRepository repository;
    private final UUID tenantA = UUID.randomUUID();
    private final UUID tenantB = UUID.randomUUID();
    private final UUID entityA = UUID.randomUUID();
    private final UUID entityB = UUID.randomUUID();

    @BeforeEach
    void start() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        repository = new AgentControlRepository(pg.namedJdbc());
        seed(tenantA, entityA, "agent-a", "a".repeat(64));
        seed(tenantB, entityB, "agent-b", "b".repeat(64));
    }

    @AfterEach
    void stop() { if (pg != null) pg.stop(); }

    private void seed(UUID tenant, UUID entity, String agent, String fingerprint) {
        var jdbc = pg.jdbc();
        jdbc.update("""
                INSERT INTO platform.tenant VALUES (?, 'Binding test', 'SINGLE', 'ACTIVE', 'UTC', now(), now())
                """, tenant);
        jdbc.update("""
                INSERT INTO platform.entity (tenant_id, entity_id, entity_type, display_name, lifecycle_state,
                    first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?, ?, 'HOST', ?, 'ACTIVE', now(), now(), now(), now())
                """, tenant, entity, agent);
        jdbc.update("""
                INSERT INTO platform.agent (tenant_id, agent_id, status, created_at, updated_at)
                VALUES (?, ?, 'ACTIVE', now(), now())
                """, tenant, agent);
        jdbc.update("""
                INSERT INTO platform.agent_certificate_credential
                    (credential_id, tenant_id, agent_id, fingerprint_sha256, serial_number, issuer_dn,
                     not_before, not_after, status, activated_at, created_at, updated_at)
                VALUES (?, ?, ?, ?, '01', 'CN=test', now()-interval '1 hour', now()+interval '1 day',
                        'ACTIVE', now(), now(), now())
                """, UUID.randomUUID(), tenant, agent, fingerprint);
    }

    private void bind(UUID tenant, UUID entity, String agent, String fingerprint, String state, long generation) {
        pg.jdbc().update("""
                INSERT INTO platform.entity_agent_binding VALUES (?, ?, ?, ?, ?, 'linux', now(), now(), ?, ?)
                """, tenant, entity, agent, fingerprint, agent, state, generation);
    }

    @Test
    void noInferredBackfillAndOnlyExplicitActiveBindingAuthorizes() {
        assertTrue(repository.findActiveEntityBinding(tenantA, "agent-a").isEmpty());
        bind(tenantA, entityA, "agent-a", "a".repeat(64), "PENDING_APPROVAL", 1);
        assertTrue(repository.findActiveEntityBinding(tenantA, "agent-a").isEmpty());
        pg.jdbc().update("UPDATE platform.entity_agent_binding SET binding_state='ACTIVE' WHERE agent_id='agent-a'");
        assertEquals(entityA, repository.findActiveEntityBinding(tenantA, "agent-a").orElseThrow().entityId());
        assertTrue(repository.findActiveEntityBinding(tenantB, "agent-a").isEmpty());
        for (String state : new String[]{"UNMANAGED", "REVOKED", "SECURITY_HOLD"}) {
            pg.jdbc().update("UPDATE platform.entity_agent_binding SET binding_state=? WHERE agent_id='agent-a'", state);
            assertTrue(repository.findActiveEntityBinding(tenantA, "agent-a").isEmpty());
        }
        pg.migrate();
        assertEquals(1, pg.jdbc().queryForObject("SELECT count(*) FROM platform.entity_agent_binding", Integer.class));
    }

    @Test
    void hostContradictionQueryIsTenantScopedAndIgnoresUnknownEvidence() {
        var identifiers = new com.platform.core.persistence.EntityIdentifierRepository(pg.namedJdbc());
        var now = java.time.Instant.now();
        identifiers.insert(new com.platform.core.domain.inventory.EntityIdentifier(UUID.randomUUID(), tenantA,
                entityA, "HOST_ID", "platform.agent", "host-a", java.math.BigDecimal.ONE, now, now));
        identifiers.insert(new com.platform.core.domain.inventory.EntityIdentifier(UUID.randomUUID(), tenantB,
                entityB, "HOST_ID", "platform.agent", "host-b", java.math.BigDecimal.ONE, now, now));
        assertFalse(identifiers.hasConflictingHostEvidence(tenantA, entityA, java.util.List.of("host-a", "host-b", "new")));
        assertTrue(identifiers.hasConflictingHostEvidence(tenantA, UUID.randomUUID(), java.util.List.of("host-a", "new")));
        assertFalse(identifiers.hasConflictingHostEvidence(tenantA, entityA, java.util.List.of()));
    }

    @Test
    void tenantAgentEntityAndCertificateCannotBeMixed() {
        assertThrows(DataIntegrityViolationException.class,
                () -> bind(tenantA, entityB, "agent-a", "a".repeat(64), "ACTIVE", 1));
        assertThrows(DataIntegrityViolationException.class,
                () -> bind(tenantA, entityA, "agent-b", "b".repeat(64), "ACTIVE", 1));
        assertThrows(DataIntegrityViolationException.class,
                () -> bind(tenantA, entityA, "agent-a", "b".repeat(64), "ACTIVE", 1));
        assertEquals(0, pg.jdbc().queryForObject("SELECT count(*) FROM platform.entity_agent_binding", Integer.class));
        bind(tenantA, entityA, "agent-a", "a".repeat(64), "ACTIVE", 1);
        assertThrows(DataIntegrityViolationException.class,
                () -> bind(tenantA, entityA, "agent-a", "a".repeat(64), "ACTIVE", 1));
    }

    @Test
    void invalidStateAndGenerationFailClosedWhileCredentialExpiryDoesNotRebindHistory() {
        for (long generation : new long[]{0, -1, 9007199254740992L}) {
            assertThrows(DataIntegrityViolationException.class,
                    () -> bind(tenantA, entityA, "agent-a", "a".repeat(64), "ACTIVE", generation));
        }
        assertThrows(DataIntegrityViolationException.class,
                () -> bind(tenantA, entityA, "agent-a", "a".repeat(64), "DOWN", 1));
        bind(tenantA, entityA, "agent-a", "a".repeat(64), "ACTIVE", 1);
        pg.jdbc().update("UPDATE platform.agent_certificate_credential SET status='SUPERSEDED' WHERE agent_id='agent-a'");
        assertEquals(entityA, repository.findActiveEntityBinding(tenantA, "agent-a").orElseThrow().entityId());
        pg.jdbc().update("UPDATE platform.entity_agent_binding SET binding_state='REVOKED' WHERE agent_id='agent-a'");
        assertTrue(repository.findActiveEntityBinding(tenantA, "agent-a").isEmpty());
    }
}
