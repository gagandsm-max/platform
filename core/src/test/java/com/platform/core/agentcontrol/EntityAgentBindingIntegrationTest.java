package com.platform.core.agentcontrol;

import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.Timestamp;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.Callable;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.*;

/** Real PostgreSQL constraints/query tests; certificate fixtures are not enrollment runtime proof. */
@Tag("postgres")
class EntityAgentBindingIntegrationTest {
    private PostgresTestSupport pg;
    private AgentControlRepository repository;
    private AgentControlService agentControl;
    private final UUID tenantA = UUID.randomUUID();
    private final UUID tenantB = UUID.randomUUID();
    private final UUID entityA = UUID.randomUUID();
    private final UUID entityB = UUID.randomUUID();
    private final UUID entityA2 = UUID.randomUUID();

    @BeforeEach
    void start() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        repository = new AgentControlRepository(pg.namedJdbc(),
                new DataSourceTransactionManager(pg.dataSource()));
        agentControl = new AgentControlService(repository, new SensorControlRepository(pg.namedJdbc()),
                new SecurityAuditEmitter(), new AssignmentMetrics());
        seed(tenantA, entityA, "agent-a", "a".repeat(64));
        seed(tenantB, entityB, "agent-b", "b".repeat(64));
        pg.jdbc().update("""
                INSERT INTO platform.entity (tenant_id, entity_id, entity_type, display_name, lifecycle_state,
                    first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?, ?, 'HOST', 'agent-a-alt', 'ACTIVE', now(), now(), now(), now())
                """, tenantA, entityA2);
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
                INSERT INTO platform.entity_agent_binding
                    (tenant_id, entity_id, agent_id, certificate_identity, system_name, os_identity,
                     first_enrolled_at, last_heartbeat_at, binding_state, reinstall_generation)
                VALUES (?, ?, ?, ?, ?, CAST(? AS jsonb), now(), now(), ?, ?)
                """, tenant, entity, agent, fingerprint, agent, "{\"family\":\"linux\"}", state, generation);
    }

    @Test
    void physicalContractMatchesPlatform17KeysTypesAndUniqueness() {
        Map<String, Object> pk = pg.jdbc().queryForMap("""
                SELECT string_agg(a.attname, ',' ORDER BY array_position(i.indkey, a.attnum)) AS cols
                  FROM pg_index i
                  JOIN pg_class c ON c.oid = i.indrelid
                  JOIN pg_namespace n ON n.oid = c.relnamespace
                  JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY (i.indkey)
                 WHERE n.nspname = 'platform' AND c.relname = 'entity_agent_binding' AND i.indisprimary
                """);
        assertEquals("tenant_id,entity_id", pk.get("cols"));

        List<String> uniqueCols = pg.jdbc().queryForList("""
                SELECT a.attname
                  FROM pg_constraint x
                  JOIN pg_class c ON c.oid = x.conrelid
                  JOIN pg_namespace n ON n.oid = c.relnamespace
                  JOIN unnest(x.conkey) WITH ORDINALITY AS cols(attnum, ord) ON true
                  JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = cols.attnum
                 WHERE n.nspname = 'platform' AND c.relname = 'entity_agent_binding'
                   AND x.contype = 'u'
                 ORDER BY x.conname, cols.ord
                """, String.class);
        assertEquals(List.of("agent_id", "certificate_identity"), uniqueCols);

        Map<String, Object> types = pg.jdbc().queryForMap("""
                SELECT
                    MAX(CASE WHEN column_name='certificate_identity'
                             THEN data_type || ':' || character_maximum_length END) AS certificate_identity,
                    MAX(CASE WHEN column_name='os_identity' THEN udt_name END) AS os_identity,
                    MAX(CASE WHEN column_name='binding_state'
                             THEN data_type || ':' || character_maximum_length END) AS binding_state,
                    MAX(CASE WHEN column_name='reinstall_generation' THEN data_type END) AS reinstall_generation
                  FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='entity_agent_binding'
                """);
        assertEquals("character varying:256", types.get("certificate_identity"));
        assertEquals("jsonb", types.get("os_identity"));
        assertEquals("character varying:20", types.get("binding_state"));
        assertEquals("integer", types.get("reinstall_generation"));
    }

    @Test
    void noInferredBackfillAndOnlyExplicitActiveBindingAuthorizes() {
        assertTrue(repository.findActiveEntityBinding(tenantA, "agent-a").isEmpty());
        bind(tenantA, entityA, "agent-a", "a".repeat(64), "PENDING_APPROVAL", 1);
        assertTrue(repository.findActiveEntityBinding(tenantA, "agent-a").isEmpty());
        pg.jdbc().update("UPDATE platform.entity_agent_binding SET binding_state='ACTIVE' WHERE agent_id='agent-a'");
        EntityAgentBinding active = repository.findActiveEntityBinding(tenantA, "agent-a").orElseThrow();
        assertEquals(entityA, active.entityId());
        assertEquals("{\"family\":\"linux\"}", active.osIdentity().replace(" ", ""));
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
    void duplicateEntityAgentOrCertificateBindingsAreRejected() {
        bind(tenantA, entityA, "agent-a", "a".repeat(64), "ACTIVE", 1);

        // One agent cannot silently bind to a second endpoint entity.
        assertThrows(DataIntegrityViolationException.class,
                () -> bind(tenantA, entityA2, "agent-a", "a".repeat(64), "ACTIVE", 1));

        // One entity cannot accept a second agent row (PK).
        String agentC = "agent-c";
        String fpC = "c".repeat(64);
        pg.jdbc().update("""
                INSERT INTO platform.agent (tenant_id, agent_id, status, created_at, updated_at)
                VALUES (?, ?, 'ACTIVE', now(), now())
                """, tenantA, agentC);
        pg.jdbc().update("""
                INSERT INTO platform.agent_certificate_credential
                    (credential_id, tenant_id, agent_id, fingerprint_sha256, serial_number, issuer_dn,
                     not_before, not_after, status, activated_at, created_at, updated_at)
                VALUES (?, ?, ?, ?, '01', 'CN=test', now()-interval '1 hour', now()+interval '1 day',
                        'ACTIVE', now(), now(), now())
                """, UUID.randomUUID(), tenantA, agentC, fpC);
        assertThrows(DataIntegrityViolationException.class,
                () -> bind(tenantA, entityA, agentC, fpC, "ACTIVE", 1));

        // Certificate identity remains deployment-unique on the binding table.
        assertThrows(DataIntegrityViolationException.class, () ->
                pg.jdbc().update("""
                        INSERT INTO platform.entity_agent_binding
                            (tenant_id, entity_id, agent_id, certificate_identity, system_name, os_identity,
                             first_enrolled_at, last_heartbeat_at, binding_state, reinstall_generation)
                        VALUES (?, ?, ?, ?, 'host', CAST(? AS jsonb), now(), now(), 'ACTIVE', 1)
                        """, tenantA, entityA2, agentC, "a".repeat(64), "{\"family\":\"linux\"}"));

        assertEquals(1, pg.jdbc().queryForObject("SELECT count(*) FROM platform.entity_agent_binding", Integer.class));
    }

    @Test
    void invalidStateAndGenerationFailClosedWhileCredentialExpiryDoesNotRebindHistory() {
        for (long generation : new long[]{0, -1}) {
            assertThrows(DataIntegrityViolationException.class,
                    () -> bind(tenantA, entityA, "agent-a", "a".repeat(64), "ACTIVE", generation));
        }
        assertThrows(DataIntegrityViolationException.class,
                () -> bind(tenantA, entityA, "agent-a", "a".repeat(64), "DOWN", 1));
        assertThrows(DataIntegrityViolationException.class,
                () -> pg.jdbc().update("""
                        INSERT INTO platform.entity_agent_binding
                            (tenant_id, entity_id, agent_id, certificate_identity, system_name, os_identity,
                             first_enrolled_at, last_heartbeat_at, binding_state, reinstall_generation)
                        VALUES (?, ?, ?, ?, ?, 'linux', now(), now(), 'ACTIVE', 1)
                        """, tenantA, entityA, "agent-a", "a".repeat(64), "agent-a"));
        bind(tenantA, entityA, "agent-a", "a".repeat(64), "ACTIVE", 1);
        pg.jdbc().update("UPDATE platform.agent_certificate_credential SET status='SUPERSEDED' WHERE agent_id='agent-a'");
        assertEquals(entityA, repository.findActiveEntityBinding(tenantA, "agent-a").orElseThrow().entityId());
        pg.jdbc().update("UPDATE platform.entity_agent_binding SET binding_state='REVOKED' WHERE agent_id='agent-a'");
        assertTrue(repository.findActiveEntityBinding(tenantA, "agent-a").isEmpty());
    }

    @Test
    void establishedBindingHistoryCoversAcceptanceAndRejectsGapsOverlapsAndForeignAgents() {
        Instant before = dbClock();
        agentControl.establishEntityBinding(tenantA, "agent-a", entityA, "a".repeat(64),
                "host-a", "{\"family\":\"linux\"}", true);

        // Committed open interval from PostgreSQL — never invent a projected future probe.
        Map<String, Object> open = pg.jdbc().queryForMap("""
                SELECT valid_from, valid_to
                  FROM platform.entity_agent_binding_history
                 WHERE tenant_id=? AND agent_id='agent-a'
                   AND activation_state='ACTIVE' AND valid_to IS NULL
                """, tenantA);
        Instant validFrom = ((Timestamp) open.get("valid_from")).toInstant();
        assertNull(open.get("valid_to"));
        Instant inside = validFrom; // demonstrably in [valid_from, +∞) while open

        var proved = repository.findHistoricalAgentEntityBinding(tenantA, "agent-a", inside, inside).orElseThrow();
        assertEquals(entityA, proved.entityId());
        assertEquals("ENTITY_AGENT_BINDING_WRITE_V022", proved.provenance());
        assertEquals(validFrom, proved.validFrom());
        assertNull(proved.validTo());
        assertTrue(repository.findHistoricalAgentEntityBinding(tenantA, "agent-a", before, before).isEmpty());
        assertTrue(repository.findHistoricalAgentEntityBinding(tenantA, "agent-b", inside, inside).isEmpty());

        // Revocation boundary must be strictly after inside so half-open [valid_from, valid_to) retains it.
        Instant revokeReady = awaitDbClockStrictlyAfter(inside);
        assertTrue(revokeReady.isAfter(inside));
        pg.jdbc().update("UPDATE platform.entity_agent_binding SET binding_state='REVOKED' WHERE agent_id='agent-a'");

        Map<String, Object> closed = pg.jdbc().queryForMap("""
                SELECT valid_from, valid_to
                  FROM platform.entity_agent_binding_history
                 WHERE tenant_id=? AND agent_id='agent-a'
                   AND activation_state='ACTIVE' AND valid_to IS NOT NULL
                """, tenantA);
        Instant closedFrom = ((Timestamp) closed.get("valid_from")).toInstant();
        Instant validTo = ((Timestamp) closed.get("valid_to")).toInstant();
        assertEquals(validFrom, closedFrom);
        assertTrue(validTo.isAfter(inside));

        // Half-open [valid_from, valid_to): valid_to is excluded; inside remains covered.
        assertTrue(repository.findHistoricalAgentEntityBinding(tenantA, "agent-a", validTo, validTo).isEmpty());
        assertEquals(entityA, repository.findHistoricalAgentEntityBinding(tenantA, "agent-a", inside, inside)
                .orElseThrow().entityId());

        // Ambiguous overlapping ACTIVE intervals for the same Agent fail closed (LIMIT 2).
        pg.jdbc().execute("ALTER TABLE platform.entity_agent_binding_history "
                + "DISABLE TRIGGER trg_guard_entity_agent_binding_history");
        Instant forgedFrom = inside.minusSeconds(30);
        Instant forgedTo = inside.plusSeconds(30);
        pg.jdbc().update("""
                INSERT INTO platform.entity_agent_binding_history
                    (tenant_id, agent_id, entity_id, certificate_identity, binding_state, reinstall_generation,
                     change_started_at, valid_from, valid_to, activation_state, activated_at, provenance)
                VALUES (?, 'agent-a', ?, ?, 'ACTIVE', 1, ?, ?, ?, 'ACTIVE', ?,
                        'ENTITY_AGENT_BINDING_WRITE_V022')
                """, tenantA, entityA2, "a".repeat(64), Timestamp.from(forgedFrom),
                Timestamp.from(forgedFrom), Timestamp.from(forgedTo), Timestamp.from(forgedFrom));
        assertTrue(repository.findHistoricalAgentEntityBinding(tenantA, "agent-a", inside, inside).isEmpty());
    }

    @Test
    void fixtureStyleDirectSqlActivatesOnlyAfterCommitAndRejectsPrematureRollbackMissingAndSpoof()
            throws Exception {
        // Committed legitimate fixture write leaves PENDING until a separate activation txn.
        bind(tenantA, entityA, "agent-a", "a".repeat(64), "ACTIVE", 1);
        assertEquals("PENDING_COMMIT_VISIBILITY", agentHistoryState("agent-a"));
        assertTrue(repository.findHistoricalAgentEntityBinding(tenantA, "agent-a",
                dbClock(), dbClock()).isEmpty());
        assertTrue(activateAgentHistory("agent-a", entityA, "a".repeat(64), "ACTIVE", 1));
        assertEquals("ACTIVE", agentHistoryState("agent-a"));
        Instant inside = ((Timestamp) pg.jdbc().queryForObject("""
                SELECT valid_from FROM platform.entity_agent_binding_history
                 WHERE tenant_id=? AND agent_id='agent-a' AND activation_state='ACTIVE'
                   AND valid_to IS NULL
                """, Timestamp.class, tenantA)).toInstant();
        assertEquals(entityA, repository.findHistoricalAgentEntityBinding(tenantA, "agent-a",
                inside, inside).orElseThrow().entityId());

        // Duplicate activation after ACTIVE is a no-op (no pending row) / safe.
        assertEquals(0, pg.jdbc().queryForObject("""
                SELECT count(*) FROM platform.entity_agent_binding_history
                 WHERE tenant_id=? AND agent_id='agent-a'
                   AND activation_state='PENDING_COMMIT_VISIBILITY'
                """, Integer.class, tenantA).intValue());

        // Premature activation inside the creating transaction is denied by xmin visibility.
        CountDownLatch inserted = new CountDownLatch(1);
        CountDownLatch allowCommit = new CountDownLatch(1);
        AtomicReference<Boolean> premature = new AtomicReference<>();
        AtomicReference<Throwable> writerFailure = new AtomicReference<>();
        Thread writer = new Thread(() -> {
            try (Connection connection = pg.dataSource().getConnection()) {
                connection.setAutoCommit(false);
                try (PreparedStatement update = connection.prepareStatement(
                        "UPDATE platform.entity_agent_binding SET entity_id=? "
                                + "WHERE tenant_id=? AND agent_id='agent-a'")) {
                    update.setObject(1, entityA2);
                    update.setObject(2, tenantA);
                    assertEquals(1, update.executeUpdate());
                }
                try (PreparedStatement activate = connection.prepareStatement("""
                        SELECT platform.activate_entity_agent_binding_history(
                            h.history_version, h.tenant_id, h.agent_id, h.entity_id,
                            h.certificate_identity, h.binding_state, h.reinstall_generation)
                          FROM platform.entity_agent_binding_history h
                         WHERE h.tenant_id=? AND h.agent_id='agent-a'
                           AND h.entity_id=? AND h.activation_state='PENDING_COMMIT_VISIBILITY'
                        """)) {
                    activate.setObject(1, tenantA);
                    activate.setObject(2, entityA2);
                    try (ResultSet row = activate.executeQuery()) {
                        assertTrue(row.next());
                        premature.set(row.getBoolean(1));
                    }
                }
                inserted.countDown();
                assertTrue(allowCommit.await(30, TimeUnit.SECONDS));
                connection.rollback();
            } catch (Throwable ex) {
                writerFailure.set(ex);
                inserted.countDown();
            }
        }, "agent-binding-history-uncommitted-writer");
        writer.start();
        assertTrue(inserted.await(30, TimeUnit.SECONDS));
        assertNull(writerFailure.get());
        assertEquals(Boolean.FALSE, premature.get());
        allowCommit.countDown();
        writer.join(30000);
        assertNull(writerFailure.get());
        // Rolled-back change never leaves committed pending history for entityA2.
        assertEquals(0, pg.jdbc().queryForObject("""
                SELECT count(*) FROM platform.entity_agent_binding_history
                 WHERE tenant_id=? AND agent_id='agent-a' AND entity_id=?
                """, Integer.class, tenantA, entityA2).intValue());
        assertEquals(entityA, pg.jdbc().queryForObject(
                "SELECT entity_id FROM platform.entity_agent_binding WHERE agent_id='agent-a'",
                UUID.class));

        // Intentionally missing history stays missing for an Agent with no binding writes.
        assertTrue(repository.findHistoricalAgentEntityBinding(tenantA, "agent-missing",
                inside, inside).isEmpty());

        // Spoofed Agent / foreign entity parameters cannot activate another Agent's pending row.
        pg.jdbc().update("UPDATE platform.entity_agent_binding SET binding_state='REVOKED' "
                + "WHERE agent_id='agent-a'");
        assertEquals("PENDING_COMMIT_VISIBILITY", agentHistoryState("agent-a"));
        assertFalse(Boolean.TRUE.equals(pg.jdbc().queryForObject("""
                SELECT platform.activate_entity_agent_binding_history(
                    h.history_version, h.tenant_id, 'agent-spoofed', h.entity_id,
                    h.certificate_identity, h.binding_state, h.reinstall_generation)
                  FROM platform.entity_agent_binding_history h
                 WHERE h.tenant_id=? AND h.agent_id='agent-a'
                   AND h.activation_state='PENDING_COMMIT_VISIBILITY'
                """, Boolean.class, tenantA)));
        assertEquals("PENDING_COMMIT_VISIBILITY", agentHistoryState("agent-a"));
        assertFalse(Boolean.TRUE.equals(pg.jdbc().queryForObject("""
                SELECT platform.activate_entity_agent_binding_history(
                    h.history_version, h.tenant_id, h.agent_id, ?,
                    h.certificate_identity, h.binding_state, h.reinstall_generation)
                  FROM platform.entity_agent_binding_history h
                 WHERE h.tenant_id=? AND h.agent_id='agent-a'
                   AND h.activation_state='PENDING_COMMIT_VISIBILITY'
                """, Boolean.class, entityB, tenantA)));
        assertEquals("PENDING_COMMIT_VISIBILITY", agentHistoryState("agent-a"));
    }

    private String agentHistoryState(String agentId) {
        return pg.jdbc().queryForObject("""
                SELECT activation_state FROM platform.entity_agent_binding_history
                 WHERE tenant_id=? AND agent_id=? ORDER BY history_version DESC LIMIT 1
                """, String.class, tenantA, agentId);
    }

    private boolean activateAgentHistory(String agentId, UUID entityId, String certificate,
                                         String state, long generation) {
        return Boolean.TRUE.equals(pg.jdbc().queryForObject("""
                SELECT platform.activate_entity_agent_binding_history(
                    h.history_version, h.tenant_id, h.agent_id, h.entity_id,
                    h.certificate_identity, h.binding_state, h.reinstall_generation)
                  FROM platform.entity_agent_binding_history h
                 WHERE h.tenant_id=? AND h.agent_id=? AND h.entity_id=?
                   AND h.certificate_identity=? AND h.binding_state=?
                   AND h.reinstall_generation=?
                   AND h.activation_state='PENDING_COMMIT_VISIBILITY'
                """, Boolean.class, tenantA, agentId, entityId, certificate, state, generation));
    }

    private Instant dbClock() {
        return pg.jdbc().queryForObject("SELECT clock_timestamp()", Timestamp.class).toInstant();
    }

    /** Advance only the exact remaining PG-clock delta past threshold — no estimated sleeps. */
    private Instant awaitDbClockStrictlyAfter(Instant threshold) {
        Instant now = pg.jdbc().queryForObject("""
                SELECT clock_timestamp()
                  FROM (
                    SELECT pg_sleep(GREATEST(0::double precision,
                        EXTRACT(EPOCH FROM (CAST(? AS timestamptz) - clock_timestamp())) + 1e-6))
                  ) waited
                """, Timestamp.class, Timestamp.from(threshold)).toInstant();
        assertTrue(now.isAfter(threshold),
                "PostgreSQL clock must be strictly after selected historical probe before revoke");
        return now;
    }

    @Test
    void concurrentDuplicateEntityAgentOrCertificateBindingsMapToStableConflict() throws Exception {
        CountDownLatch start = new CountDownLatch(1);
        ExecutorService pool = Executors.newFixedThreadPool(2);
        AtomicInteger successes = new AtomicInteger();
        AtomicInteger conflicts = new AtomicInteger();
        Callable<Void> race = () -> {
            start.await(30, TimeUnit.SECONDS);
            try {
                agentControl.establishEntityBinding(tenantA, "agent-a", entityA, "a".repeat(64),
                        "host-a", "{\"family\":\"linux\"}", true);
                successes.incrementAndGet();
            } catch (AgentControlException ex) {
                assertEquals(AgentControlException.ERR_AGENT_BINDING_IDENTITY_CONFLICT, ex.getCode());
                String message = String.valueOf(ex.getMessage()).toLowerCase();
                assertFalse(message.contains("sql") || message.contains("duplicate key")
                        || message.contains("violat"));
                conflicts.incrementAndGet();
            }
            return null;
        };
        List<Future<Void>> futures = List.of(pool.submit(race), pool.submit(race));
        start.countDown();
        for (Future<Void> future : futures) {
            future.get(30, TimeUnit.SECONDS);
        }
        pool.shutdownNow();
        assertEquals(1, successes.get());
        assertEquals(1, conflicts.get());
        assertEquals(1, pg.jdbc().queryForObject("SELECT count(*) FROM platform.entity_agent_binding", Integer.class));

        String agentC = "agent-c";
        String fpC = "c".repeat(64);
        pg.jdbc().update("""
                INSERT INTO platform.agent (tenant_id, agent_id, status, created_at, updated_at)
                VALUES (?, ?, 'ACTIVE', now(), now())
                """, tenantA, agentC);
        pg.jdbc().update("""
                INSERT INTO platform.agent_certificate_credential
                    (credential_id, tenant_id, agent_id, fingerprint_sha256, serial_number, issuer_dn,
                     not_before, not_after, status, activated_at, created_at, updated_at)
                VALUES (?, ?, ?, ?, '01', 'CN=test', now()-interval '1 hour', now()+interval '1 day',
                        'ACTIVE', now(), now(), now())
                """, UUID.randomUUID(), tenantA, agentC, fpC);
        AgentControlException entityConflict = assertThrows(AgentControlException.class, () ->
                agentControl.establishEntityBinding(tenantA, agentC, entityA, fpC,
                        "host-c", "{\"family\":\"linux\"}", true));
        assertEquals(AgentControlException.ERR_AGENT_BINDING_IDENTITY_CONFLICT, entityConflict.getCode());
        AgentControlException agentConflict = assertThrows(AgentControlException.class, () ->
                agentControl.establishEntityBinding(tenantA, "agent-a", entityA2, "a".repeat(64),
                        "host-a2", "{\"family\":\"linux\"}", true));
        assertEquals(AgentControlException.ERR_AGENT_BINDING_IDENTITY_CONFLICT, agentConflict.getCode());
    }
}
