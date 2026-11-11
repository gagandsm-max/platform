package com.platform.core.migration;

import com.platform.core.support.PostgresTestSupport;
import com.platform.core.ingestion.agentingress.PostgresAgentIngressWatermarkStore;
import com.platform.core.ingestion.agentingress.AgentIngestionService;
import com.platform.core.ingestion.agentingress.AgentIngestionValidator;
import com.platform.core.ingestion.agentingress.dto.AgentIngressRecord;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryBatchRequest;
import com.platform.core.agentcontrol.AgentControlRepository;
import com.platform.core.agentcontrol.AgentSensorAssignment;
import com.platform.core.agentcontrol.AgentAssignmentStatus;
import com.platform.core.agentcontrol.EntityAgentBinding;
import com.platform.core.ingestion.IngestionMetrics;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.KafkaTelemetryProducer;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.TelemetryObservationBatch;
import com.platform.core.persistence.EntityIdentifierRepository;
import com.platform.core.domain.inventory.EntityIdentifier;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.replay.ReplayReservation;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;

import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.Optional;
import java.util.Set;
import java.time.Instant;
import java.security.MessageDigest;
import java.security.cert.X509Certificate;
import java.nio.charset.StandardCharsets;
import java.util.HexFormat;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.Timestamp;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;
import static org.mockito.ArgumentMatchers.*;

@Tag("postgres")
class AgentIngressPendingReleaseMigrationTest {

    private static final UUID TENANT_A = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private static final UUID TENANT_B = UUID.fromString("018f0e0a-0000-7000-8000-000000000099");
    private static final String STREAM = "stream-00112233445566778899aabbccddeeff";
    private static final String CHECKSUM = "a".repeat(64);
    private static final String BODY_SHA = "b".repeat(64);

    private PostgresTestSupport pg;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        // External Compose mode retains its private database between test methods.
        // Each case needs an independent V020 schema, even after a prior V021 case.
        pg.jdbc().execute("DROP SCHEMA IF EXISTS platform CASCADE");
        pg.jdbc().execute("CREATE SCHEMA platform");
        pg.runner(catalogThroughV020()).applyPendingMigrations();
        seedTenant(TENANT_A);
    }

    @AfterEach
    void tearDown() {
        if (pg != null) pg.stop();
    }

    @Test
    void u1CleanV020HistoryMigratesWithoutChangingAcceptedOrSettledState() {
        seedAccepted(TENANT_A, "sensor-a");
        Map<String, Object> before = state(TENANT_A, "sensor-a");

        pg.migrate();

        assertEquals(before, state(TENANT_A, "sensor-a"));
        assertEquals(1, count("SELECT count(*) FROM platform.schema_history WHERE version='V021'"));
        assertEquals(1, count("SELECT count(*) FROM information_schema.tables WHERE table_schema='platform' "
                + "AND table_name='sensor_agent_ingress_pending_canonical_binding'"));
        assertEquals(6, count("""
                SELECT count(*) FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='sensor_agent_ingress_watermark'
                   AND column_name IN ('pending_reservation_id','pending_publication_attempted',
                       'pending_authority_state','pending_authority_validated_at',
                       'pending_canonical_batch_sha256','pending_canonical_observation_sha256')
                """));
        assertEquals(1, count("SELECT count(*) FROM information_schema.tables "
                + "WHERE table_schema='platform' AND table_name='entity_identifier_binding_history'"));
        assertEquals(1, count("""
                SELECT count(*) FROM pg_constraint
                 WHERE conrelid='platform.sensor_agent_ingress_watermark'::regclass
                   AND conname='chk_agent_ingress_pending_release_guard'
                """));
        assertEquals(false, pg.jdbc().queryForObject("""
                SELECT pending_publication_attempted FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id='sensor-a'
                """, Boolean.class, TENANT_A));
        pg.migrate();
        assertEquals(1, count("SELECT count(*) FROM platform.schema_history WHERE version='V021'"));
    }

    @Test
    void u2DormantV020PendingMigratesWithoutChangingItsIdentityOrWatermarks() {
        seedAccepted(TENANT_A, "sensor-a");
        seedPending(TENANT_A, "sensor-a");
        Map<String, Object> before = state(TENANT_A, "sensor-a");

        pg.migrate();
        assertEquals(before, state(TENANT_A, "sensor-a"));
        Map<String, Object> pending = pendingState(TENANT_A, "sensor-a");
        assertEquals("LEGACY_UNKNOWN", pending.get("pending_authority_state"));
        assertNotNull(pending.get("pending_reservation_id"));
        assertNull(pending.get("pending_publication_attempted"));
        assertNull(pending.get("pending_authority_validated_at"));
        assertNull(pending.get("pending_canonical_batch_sha256"));
    }

    @Test
    void u3MultiTenantPendingMigratesIndependently() {
        seedTenant(TENANT_B);
        seedAccepted(TENANT_A, "sensor-a");
        seedAccepted(TENANT_B, "sensor-b");
        seedPending(TENANT_A, "sensor-a");
        seedPending(TENANT_B, "sensor-b");
        Map<String, Object> aBefore = state(TENANT_A, "sensor-a");
        Map<String, Object> bBefore = state(TENANT_B, "sensor-b");

        pg.migrate();
        assertEquals(aBefore, state(TENANT_A, "sensor-a"));
        assertEquals(bBefore, state(TENANT_B, "sensor-b"));
        assertNotEquals(pendingState(TENANT_A, "sensor-a").get("pending_reservation_id"),
                pendingState(TENANT_B, "sensor-b").get("pending_reservation_id"));
        assertEquals("LEGACY_UNKNOWN", pendingState(TENANT_B, "sensor-b").get("pending_authority_state"));
    }

    @Test
    void u4ActivePublisherAtCutoverBlocksAndRollsBack() {
        seedAccepted(TENANT_A, "sensor-a");
        seedPending(TENANT_A, "sensor-a");
        Map<String, Object> before = state(TENANT_A, "sensor-a");
        assertEquals(1, pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_publisher_owner=gen_random_uuid(), pending_publisher_epoch=1,
                       pending_publisher_claimed_at=clock_timestamp(),
                       pending_publisher_lease_until=clock_timestamp() + interval '60 seconds'
                 WHERE tenant_id=? AND sensor_id='sensor-a'
                """, TENANT_A));
        Map<String, Object> owned = state(TENANT_A, "sensor-a");
        MigrationException failure = assertThrows(MigrationException.class, pg::migrate);
        assertEquals("AGENT_INGRESS_PUBLISHER_UPGRADE_BLOCKED", failure.getCode());
        assertEquals("P0211", deepestSqlState(failure));
        assertV021Absent();
        assertEquals(owned, state(TENANT_A, "sensor-a"));
    }

    @Test
    void u5aPreHistoryIdentifierCannotProveHistoricalEntity() throws Exception {
        ObjectMapper mapper = new ObjectMapper().findAndRegisterModules();
        UUID entity = UUID.fromString("018f0e0a-0000-7000-8000-00000000ae01");
        LegacyRequest legacy = legacyRequest(mapper);
        seedAccepted(TENANT_A, "sensor-a");
        seedPending(TENANT_A, "sensor-a");
        pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_checksum=?, pending_request_sha256=?
                 WHERE tenant_id=? AND sensor_id='sensor-a'
                """, legacy.checksum(), sha256(legacy.body()), TENANT_A);
        seedCanonicalIdentity(entity, Instant.parse("2026-11-09T09:00:00Z"));
        pg.migrate();
        assertEquals("LEGACY_UNKNOWN", pendingState(TENANT_A, "sensor-a").get("pending_authority_state"));
        SensorAuthorityResolver authority = mock(SensorAuthorityResolver.class);
        AgentControlRepository agentRepo = mock(AgentControlRepository.class);
        EntityIdentifierRepository identifiers = new EntityIdentifierRepository(
                new NamedParameterJdbcTemplate(pg.dataSource()));
        KafkaTelemetryProducer kafka = mock(KafkaTelemetryProducer.class);
        X509Certificate certificate = mock(X509Certificate.class);
        SensorScope scope = new SensorScope("sensor-a", TENANT_A, null, Set.of(entity), true);
        when(authority.resolve(certificate)).thenReturn(scope);
        Instant created = Instant.parse("2026-11-09T09:00:00Z");
        Instant revoked = Instant.parse("2026-11-09T11:00:00Z");
        when(agentRepo.findAssignmentByGeneration(TENANT_A, "agent-a", 1))
                .thenReturn(Optional.of(new AgentSensorAssignment(UUID.randomUUID(), TENANT_A,
                        "agent-a", "sensor-a", 1, AgentAssignmentStatus.REVOKED,
                        created, created, revoked)));
        when(agentRepo.findActiveEntityBinding(TENANT_A, "agent-a"))
                .thenReturn(Optional.of(new EntityAgentBinding(TENANT_A, entity, "agent-a",
                        "a".repeat(64), "host-a", "linux", created, revoked, "ACTIVE", 1)));
        AgentIngestionValidator validator = new AgentIngestionValidator(mapper, authority,
                agentRepo, identifiers, new IngestionMetrics());
        AgentIngestionService service = new AgentIngestionService(validator, store(), kafka,
                new IngestionMetrics());
        IngestionException failure = assertThrows(IngestionException.class, () ->
                service.ingestAgentTelemetry(legacy.body(), legacy.request(),
                        legacy.request().batchId(), certificate));
        assertEquals("ERR_AGENT_LEGACY_PENDING_RECONCILIATION_REQUIRED", failure.getCode());
        verifyNoInteractions(kafka);
        assertEquals("LEGACY_UNKNOWN", pendingState(TENANT_A, "sensor-a").get("pending_authority_state"));
        assertEquals(0, count("SELECT count(*) FROM platform.sensor_agent_ingress_pending_canonical_binding"));
    }

    @Test
    void historyRecordsAThenBWithoutRewritingAcceptanceInterval() {
        pg.migrate();
        UUID a = UUID.fromString("018f0e0a-0000-7000-8000-00000000ae11");
        UUID b = UUID.fromString("018f0e0a-0000-7000-8000-00000000ae12");
        for (UUID entity : List.of(a, b)) {
            pg.jdbc().update("""
                    INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name,
                      lifecycle_state, first_seen_at, last_seen_at, created_at, updated_at)
                    VALUES (?, ?, 'HOST', 'history-test', 'ACTIVE', now(), now(), now(), now())
                    """, entity, TENANT_A);
        }
        UUID identifier = UUID.randomUUID();
        pg.jdbc().update("""
                INSERT INTO platform.entity_identifier (identifier_id, tenant_id, entity_id,
                  identifier_type, namespace, identifier_value, confidence, first_seen_at, last_seen_at)
                VALUES (?, ?, ?, 'HOST_ID', 'platform.agent', 'historical-a-b', 1.0, now(), now())
                """, identifier, TENANT_A, a);
        assertEquals("PENDING_COMMIT_VISIBILITY", historyState(identifier));
        EntityIdentifierRepository repo = new EntityIdentifierRepository(new NamedParameterJdbcTemplate(pg.dataSource()));
        Instant beforeActivation = dbNow();
        assertTrue(repo.findHistoricalBinding(TENANT_A, "platform.agent", "HOST_ID",
                "historical-a-b", beforeActivation, beforeActivation).isEmpty());
        assertTrue(activateHistory(identifier));
        Instant acceptedAt = pg.jdbc().queryForObject("SELECT clock_timestamp()", java.sql.Timestamp.class).toInstant();
        pg.jdbc().update("UPDATE platform.entity_identifier SET entity_id=? "
                + "WHERE tenant_id=? AND identifier_id=?", b, TENANT_A, identifier);
        assertEquals("PENDING_COMMIT_VISIBILITY", historyState(identifier));
        Instant gap = dbNow();
        assertTrue(repo.findHistoricalBinding(TENANT_A, "platform.agent", "HOST_ID",
                "historical-a-b", gap, gap).isEmpty());
        var historical = repo.findHistoricalBinding(TENANT_A, "platform.agent", "HOST_ID",
                "historical-a-b", acceptedAt, acceptedAt);
        assertTrue(historical.isPresent());
        assertEquals(a, historical.get().entityId());
        assertEquals(identifier, historical.get().identifierId());
        assertNotNull(historical.get().validTo());
        assertTrue(activateHistory(identifier));
        Instant afterActivation = dbNow();
        assertEquals(b, repo.findHistoricalBinding(TENANT_A, "platform.agent", "HOST_ID",
                "historical-a-b", afterActivation, afterActivation).orElseThrow().entityId());
        assertTrue(repo.findHistoricalBinding(TENANT_A, "platform.agent", "HOST_ID",
                "historical-a-b", gap, gap).isEmpty());
        assertEquals(b, repo.findByResolutionKey(TENANT_A, "platform.agent", "HOST_ID",
                "historical-a-b").orElseThrow().entityId());
        assertEquals(2, count("SELECT count(*) FROM platform.entity_identifier_binding_history "
                + "WHERE tenant_id='" + TENANT_A + "' AND identifier_value='historical-a-b'"));
        assertThrows(org.springframework.dao.DataAccessException.class, () -> pg.jdbc().update(
                "UPDATE platform.entity_identifier_binding_history SET entity_id=? "
                        + "WHERE tenant_id=? AND identifier_value='historical-a-b'", b, TENANT_A));
        assertThrows(org.springframework.dao.DataAccessException.class, () -> pg.jdbc().update("""
                INSERT INTO platform.entity_identifier_binding_history
                  (tenant_id, namespace, identifier_type, identifier_value, entity_identifier_id,
                   entity_id, confidence, change_started_at, activation_state, provenance)
                VALUES (?, 'platform.agent', 'HOST_ID', 'forged', ?, ?, 1.0, now(),
                        'PENDING_COMMIT_VISIBILITY', 'ENTITY_IDENTIFIER_WRITE_V021')
                """, TENANT_A, UUID.randomUUID(), b));
    }

    @Test
    void uncommittedAtoBDoesNotBecomeHistoricalAuthority() throws Exception {
        seedAccepted(TENANT_A, "sensor-a");
        seedPending(TENANT_A, "sensor-a");
        pg.migrate();
        UUID a = UUID.randomUUID();
        UUID b = UUID.randomUUID();
        seedCanonicalIdentity(a, dbNow().minusSeconds(60));
        pg.jdbc().update("""
                INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name,
                  lifecycle_state, first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?, ?, 'HOST', 'race-entity-b', 'ACTIVE', now(), now(), now(), now())
                """, b, TENANT_A);
        UUID identifier = UUID.fromString("018f0e0a-0000-7000-8000-00000000a101");
        assertTrue(activateHistory(identifier));
        CountDownLatch triggerDone = new CountDownLatch(1);
        CountDownLatch allowCommit = new CountDownLatch(1);
        AtomicReference<Instant> closedAt = new AtomicReference<>();
        AtomicReference<Throwable> writerFailure = new AtomicReference<>();
        Thread writer = new Thread(() -> {
            try (Connection connection = pg.dataSource().getConnection()) {
                connection.setAutoCommit(false);
                try (PreparedStatement update = connection.prepareStatement(
                        "UPDATE platform.entity_identifier SET entity_id=? WHERE tenant_id=? AND identifier_id=?")) {
                    update.setObject(1, b);
                    update.setObject(2, TENANT_A);
                    update.setObject(3, identifier);
                    assertEquals(1, update.executeUpdate());
                }
                try (PreparedStatement select = connection.prepareStatement("""
                        SELECT valid_to FROM platform.entity_identifier_binding_history
                         WHERE tenant_id=? AND identifier_value='host-a' AND entity_id=?
                        """)) {
                    select.setObject(1, TENANT_A);
                    select.setObject(2, a);
                    try (ResultSet row = select.executeQuery()) {
                        assertTrue(row.next());
                        closedAt.set(row.getTimestamp(1).toInstant());
                    }
                }
                try (PreparedStatement premature = connection.prepareStatement("""
                        SELECT platform.activate_entity_identifier_binding_history(
                            h.history_version, h.tenant_id, h.entity_identifier_id, h.entity_id,
                            h.namespace, h.identifier_type, h.identifier_value)
                          FROM platform.entity_identifier_binding_history h
                         WHERE h.tenant_id=? AND h.identifier_value='host-a'
                           AND h.entity_id=? AND h.activation_state='PENDING_COMMIT_VISIBILITY'
                        """)) {
                    premature.setObject(1, TENANT_A);
                    premature.setObject(2, b);
                    try (ResultSet row = premature.executeQuery()) {
                        assertTrue(row.next());
                        assertFalse(row.getBoolean(1));
                    }
                }
                System.out.println("HISTORY_T1_PRECOMMIT A_valid_to=" + closedAt.get()
                        + " B_state=PENDING_COMMIT_VISIBILITY B_valid_from=NULL premature_activation=DENIED");
                triggerDone.countDown();
                assertTrue(allowCommit.await(30, TimeUnit.SECONDS));
                connection.commit();
            } catch (Throwable ex) {
                writerFailure.set(ex);
                triggerDone.countDown();
            }
        }, "identifier-history-uncommitted-writer");
        writer.start();
        try {
            assertTrue(triggerDone.await(30, TimeUnit.SECONDS));
            assertNull(writerFailure.get());
            Instant t1 = dbNow();
            assertTrue(t1.isAfter(closedAt.get()));
            assertEquals(a, pg.jdbc().queryForObject("""
                    SELECT entity_id FROM platform.entity_identifier
                     WHERE tenant_id=? AND identifier_id=?
                    """, UUID.class, TENANT_A, identifier));
            ObjectMapper mapper = new ObjectMapper().findAndRegisterModules();
            LegacyRequest legacy = legacyRequest(mapper, t1.toString());
            allowCommit.countDown();
            writer.join(30000);
            assertFalse(writer.isAlive());
            assertNull(writerFailure.get());
            assertTrue(activateHistory(identifier));
            EntityIdentifierRepository repo = new EntityIdentifierRepository(
                    new NamedParameterJdbcTemplate(pg.dataSource()));
            assertTrue(repo.findHistoricalBinding(TENANT_A, "platform.agent", "HOST_ID",
                    "host-a", t1, t1).isEmpty());
            Instant activatedAt = pg.jdbc().queryForObject("""
                    SELECT activated_at FROM platform.entity_identifier_binding_history
                     WHERE tenant_id=? AND identifier_value='host-a' AND entity_id=?
                    """, Timestamp.class, TENANT_A, b).toInstant();
            assertEquals(activatedAt, pg.jdbc().queryForObject("""
                    SELECT valid_from FROM platform.entity_identifier_binding_history
                     WHERE tenant_id=? AND identifier_value='host-a' AND entity_id=?
                    """, Timestamp.class, TENANT_A, b).toInstant());
            assertTrue(activatedAt.isAfter(t1));
            pg.jdbc().update("""
                    UPDATE platform.sensor_agent_ingress_watermark
                       SET pending_checksum=?, pending_request_sha256=?
                     WHERE tenant_id=? AND sensor_id='sensor-a'
                    """, legacy.checksum(), sha256(legacy.body()), TENANT_A);
            SensorAuthorityResolver authority = mock(SensorAuthorityResolver.class);
            AgentControlRepository agentRepo = mock(AgentControlRepository.class);
            KafkaTelemetryProducer kafka = mock(KafkaTelemetryProducer.class);
            X509Certificate certificate = mock(X509Certificate.class);
            when(authority.resolve(certificate)).thenReturn(
                    new SensorScope("sensor-a", TENANT_A, null, Set.of(b), true));
            Instant revoked = dbNow();
            when(agentRepo.findAssignmentByGeneration(TENANT_A, "agent-a", 1))
                    .thenReturn(Optional.of(new AgentSensorAssignment(UUID.randomUUID(), TENANT_A,
                            "agent-a", "sensor-a", 1, AgentAssignmentStatus.REVOKED,
                            t1.minusSeconds(60), t1.minusSeconds(60), revoked)));
            when(agentRepo.findActiveEntityBinding(TENANT_A, "agent-a"))
                    .thenReturn(Optional.of(new EntityAgentBinding(TENANT_A, b, "agent-a",
                            "a".repeat(64), "host-a", "linux", revoked, revoked, "ACTIVE", 1)));
            AgentIngestionService service = new AgentIngestionService(
                    new AgentIngestionValidator(mapper, authority, agentRepo, repo,
                            new IngestionMetrics()), store(), kafka, new IngestionMetrics());
            IngestionException failure = assertThrows(IngestionException.class, () ->
                    service.ingestAgentTelemetry(legacy.body(), legacy.request(),
                            legacy.request().batchId(), certificate));
            assertEquals("ERR_AGENT_LEGACY_PENDING_RECONCILIATION_REQUIRED", failure.getCode());
            verifyNoInteractions(kafka);
            assertEquals("LEGACY_UNKNOWN", pendingState(TENANT_A, "sensor-a").get("pending_authority_state"));
            System.out.println("HISTORY_COMMIT_RACE t0=" + closedAt.get()
                    + " t1=" + t1 + " activation=" + activatedAt
                    + " committed_reader=A historical_t1=NONE disposition=U5B");
            assertEquals(b, repo.findByResolutionKey(TENANT_A, "platform.agent", "HOST_ID",
                    "host-a").orElseThrow().entityId());
        } finally {
            allowCommit.countDown();
            writer.join(30000);
        }
    }

    @Test
    void failedActivationAndRapidRepointNeverInventHistory() {
        pg.migrate();
        UUID a = UUID.randomUUID();
        UUID b = UUID.randomUUID();
        UUID c = UUID.randomUUID();
        for (UUID entity : List.of(a, b, c)) {
            pg.jdbc().update("""
                    INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name,
                      lifecycle_state, first_seen_at, last_seen_at, created_at, updated_at)
                    VALUES (?, ?, 'HOST', 'rapid-entity', 'ACTIVE', now(), now(), now(), now())
                    """, entity, TENANT_A);
        }
        UUID identifier = UUID.randomUUID();
        pg.jdbc().update("""
                INSERT INTO platform.entity_identifier (identifier_id, tenant_id, entity_id,
                  identifier_type, namespace, identifier_value, confidence, first_seen_at, last_seen_at)
                VALUES (?, ?, ?, 'HOST_ID', 'platform.agent', 'rapid-a-b-c', 1.0, now(), now())
                """, identifier, TENANT_A, a);
        assertTrue(activateHistory(identifier));
        pg.jdbc().update("UPDATE platform.entity_identifier SET entity_id=? WHERE identifier_id=?", b, identifier);
        long bVersion = pg.jdbc().queryForObject("""
                SELECT history_version FROM platform.entity_identifier_binding_history
                 WHERE entity_identifier_id=? AND entity_id=?
                """, Long.class, identifier, b);
        assertFalse(Boolean.TRUE.equals(pg.jdbc().queryForObject("""
                SELECT platform.activate_entity_identifier_binding_history(
                    ?, ?, ?, ?, 'platform.agent', 'HOST_ID', 'rapid-a-b-c')
                """, Boolean.class, bVersion, TENANT_A, identifier, a)));
        Instant bGap = dbNow();
        EntityIdentifierRepository repo = new EntityIdentifierRepository(pg.namedJdbc());
        assertTrue(repo.findHistoricalBinding(TENANT_A, "platform.agent", "HOST_ID",
                "rapid-a-b-c", bGap, bGap).isEmpty());
        pg.jdbc().update("UPDATE platform.entity_identifier SET entity_id=? WHERE identifier_id=?", c, identifier);
        assertEquals("SUPERSEDED", pg.jdbc().queryForObject("""
                SELECT activation_state FROM platform.entity_identifier_binding_history
                 WHERE history_version=?
                """, String.class, bVersion));
        assertFalse(Boolean.TRUE.equals(pg.jdbc().queryForObject("""
                SELECT platform.activate_entity_identifier_binding_history(
                    ?, ?, ?, ?, 'platform.agent', 'HOST_ID', 'rapid-a-b-c')
                """, Boolean.class, bVersion, TENANT_A, identifier, b)));
        assertTrue(repo.findHistoricalBinding(TENANT_A, "platform.agent", "HOST_ID",
                "rapid-a-b-c", bGap, bGap).isEmpty());
        assertTrue(activateHistory(identifier));
        Instant afterC = dbNow();
        assertEquals(c, repo.findHistoricalBinding(TENANT_A, "platform.agent", "HOST_ID",
                "rapid-a-b-c", afterC, afterC).orElseThrow().entityId());
        assertEquals(0, count("SELECT count(*) FROM platform.entity_identifier_binding_history "
                + "WHERE entity_identifier_id='" + identifier + "' AND entity_id='" + b
                + "' AND activation_state='ACTIVE'"));
    }

    @Test
    void ambiguousActiveHistoryIsRejectedEvenIfCurrentIdentifierMatches() {
        pg.migrate();
        UUID entity = UUID.randomUUID();
        UUID identifier = UUID.randomUUID();
        pg.jdbc().update("""
                INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name,
                  lifecycle_state, first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?, ?, 'HOST', 'ambiguous-entity', 'ACTIVE', now(), now(), now(), now())
                """, entity, TENANT_A);
        pg.jdbc().update("""
                INSERT INTO platform.entity_identifier (identifier_id, tenant_id, entity_id,
                  identifier_type, namespace, identifier_value, confidence, first_seen_at, last_seen_at)
                VALUES (?, ?, ?, 'HOST_ID', 'platform.agent', 'ambiguous-history', 1.0, now(), now())
                """, identifier, TENANT_A, entity);
        assertTrue(activateHistory(identifier));
        Instant sample = dbNow();
        // Controlled corruption fixture: bypass both DB guards only inside this private test schema.
        pg.jdbc().execute("ALTER TABLE platform.entity_identifier_binding_history DISABLE TRIGGER trg_guard_entity_identifier_binding_history");
        pg.jdbc().execute("DROP INDEX platform.uq_identifier_history_open_key");
        pg.jdbc().update("""
                INSERT INTO platform.entity_identifier_binding_history
                  (tenant_id, namespace, identifier_type, identifier_value, entity_identifier_id,
                   entity_id, confidence, change_started_at, valid_from, activation_state,
                   activated_at, provenance)
                SELECT tenant_id, namespace, identifier_type, identifier_value, entity_identifier_id,
                       entity_id, confidence, change_started_at, valid_from, activation_state,
                       activated_at, provenance
                  FROM platform.entity_identifier_binding_history WHERE entity_identifier_id=?
                """, identifier);
        EntityIdentifierRepository repo = new EntityIdentifierRepository(pg.namedJdbc());
        assertTrue(repo.findHistoricalBinding(TENANT_A, "platform.agent", "HOST_ID",
                "ambiguous-history", sample, sample).isEmpty());
    }

    @Test
    void repositoryActivatesOnlyAfterCommittedInsertAndSkipsRollback() {
        pg.migrate();
        UUID entity = UUID.randomUUID();
        pg.jdbc().update("""
                INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name,
                  lifecycle_state, first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?, ?, 'HOST', 'managed-history', 'ACTIVE', now(), now(), now(), now())
                """, entity, TENANT_A);
        EntityIdentifierRepository managed = new EntityIdentifierRepository(pg.namedJdbc(),
                new DataSourceTransactionManager(pg.dataSource()));
        Instant now = dbNow();
        UUID committedId = UUID.randomUUID();
        pg.tx().executeWithoutResult(status -> {
            managed.insert(new EntityIdentifier(committedId, TENANT_A, entity, "HOST_ID",
                    "platform.agent", "managed-commit", java.math.BigDecimal.ONE, now, now));
            assertEquals("PENDING_COMMIT_VISIBILITY", historyState(committedId));
        });
        assertEquals("ACTIVE", historyState(committedId));
        UUID rolledBackId = UUID.randomUUID();
        pg.tx().executeWithoutResult(status -> {
            managed.insert(new EntityIdentifier(rolledBackId, TENANT_A, entity, "HOST_ID",
                    "platform.agent", "managed-rollback", java.math.BigDecimal.ONE, now, now));
            assertEquals("PENDING_COMMIT_VISIBILITY", historyState(rolledBackId));
            status.setRollbackOnly();
        });
        assertEquals(0, count("SELECT count(*) FROM platform.entity_identifier_binding_history "
                + "WHERE entity_identifier_id='" + rolledBackId + "'"));
        UUID supersededId = UUID.randomUUID();
        pg.tx().executeWithoutResult(status -> {
            managed.insert(new EntityIdentifier(supersededId, TENANT_A, entity, "HOST_ID",
                    "platform.agent", "managed-superseded", java.math.BigDecimal.ONE, now, now));
            pg.jdbc().update("UPDATE platform.entity_identifier SET confidence=0.5 "
                    + "WHERE identifier_id=?", supersededId);
        });
        assertEquals(0, count("SELECT count(*) FROM platform.entity_identifier_binding_history "
                + "WHERE entity_identifier_id='" + supersededId + "' AND activation_state='ACTIVE'"));
        assertEquals("PENDING_COMMIT_VISIBILITY", historyState(supersededId));
    }

    private Instant dbNow() {
        return pg.jdbc().queryForObject("SELECT clock_timestamp()", Timestamp.class).toInstant();
    }

    private String historyState(UUID identifier) {
        return pg.jdbc().queryForObject("""
                SELECT activation_state FROM platform.entity_identifier_binding_history
                 WHERE tenant_id=? AND entity_identifier_id=? ORDER BY history_version DESC LIMIT 1
                """, String.class, TENANT_A, identifier);
    }

    private boolean activateHistory(UUID identifier) {
        return Boolean.TRUE.equals(pg.jdbc().queryForObject("""
                SELECT platform.activate_entity_identifier_binding_history(
                    h.history_version, h.tenant_id, h.entity_identifier_id, h.entity_id,
                    h.namespace, h.identifier_type, h.identifier_value)
                  FROM platform.entity_identifier_binding_history h
                 WHERE h.tenant_id=? AND h.entity_identifier_id=?
                   AND h.activation_state='PENDING_COMMIT_VISIBILITY'
                """, Boolean.class, TENANT_A, identifier));
    }

    @Test
    void u5aLegacyPendingPublishesOnlyWithActiveCommittedHistory() throws Exception {
        seedAccepted(TENANT_A, "sensor-a");
        seedPending(TENANT_A, "sensor-a");
        pg.migrate();
        UUID entity = UUID.fromString("018f0e0a-0000-7000-8000-00000000ae01");
        UUID identifier = UUID.fromString("018f0e0a-0000-7000-8000-00000000a101");
        seedCanonicalIdentity(entity, dbNow().minusSeconds(60));
        assertEquals("PENDING_COMMIT_VISIBILITY", historyState(identifier));
        assertTrue(activateHistory(identifier));
        assertEquals("ACTIVE", historyState(identifier));
        Instant acceptedAt = dbNow();
        ObjectMapper mapper = new ObjectMapper().findAndRegisterModules();
        LegacyRequest legacy = legacyRequest(mapper, acceptedAt.toString());
        pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_checksum=?, pending_request_sha256=?
                 WHERE tenant_id=? AND sensor_id='sensor-a'
                """, legacy.checksum(), sha256(legacy.body()), TENANT_A);
        SensorAuthorityResolver authority = mock(SensorAuthorityResolver.class);
        AgentControlRepository agentRepo = mock(AgentControlRepository.class);
        KafkaTelemetryProducer kafka = mock(KafkaTelemetryProducer.class);
        X509Certificate certificate = mock(X509Certificate.class);
        when(authority.resolve(certificate)).thenReturn(
                new SensorScope("sensor-a", TENANT_A, null, Set.of(entity), true));
        Instant created = acceptedAt.minusSeconds(60);
        Instant revoked = dbNow();
        when(agentRepo.findAssignmentByGeneration(TENANT_A, "agent-a", 1))
                .thenReturn(Optional.of(new AgentSensorAssignment(UUID.randomUUID(), TENANT_A,
                        "agent-a", "sensor-a", 1, AgentAssignmentStatus.REVOKED,
                        created, created, revoked)));
        when(agentRepo.findActiveEntityBinding(TENANT_A, "agent-a"))
                .thenReturn(Optional.of(new EntityAgentBinding(TENANT_A, entity, "agent-a",
                        "a".repeat(64), "host-a", "linux", created, revoked, "ACTIVE", 1)));
        EntityIdentifierRepository identifiers = new EntityIdentifierRepository(pg.namedJdbc());
        assertEquals(entity, identifiers.findHistoricalBinding(TENANT_A, "platform.agent", "HOST_ID",
                "host-a", acceptedAt, acceptedAt).orElseThrow().entityId());
        AgentIngestionService service = new AgentIngestionService(
                new AgentIngestionValidator(mapper, authority, agentRepo, identifiers,
                        new IngestionMetrics()), store(), kafka, new IngestionMetrics());
        var ack = service.ingestAgentTelemetry(legacy.body(), legacy.request(),
                legacy.request().batchId(), certificate);
        assertEquals("ACCEPTED", ack.status());
        assertEquals(3L, state(TENANT_A, "sensor-a").get("settled_through_sequence"));
        assertNull(state(TENANT_A, "sensor-a").get("pending_first_sequence"));
        assertEquals("NONE", pendingState(TENANT_A, "sensor-a").get("pending_authority_state"));
        verify(kafka, times(1)).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        System.out.println("HISTORY_U5A_ACTIVE_LEGACY accepted_at=" + acceptedAt
                + " history_state=ACTIVE entity=A publication=1 pending=CLEARED");
    }

    @Test
    void u5bLegacyExactRetryWithNewEntityBRequiresReconciliation() throws Exception {
        ObjectMapper mapper = new ObjectMapper().findAndRegisterModules();
        seedAccepted(TENANT_A, "sensor-a");
        seedPending(TENANT_A, "sensor-a");
        UUID originalA = UUID.fromString("018f0e0a-0000-7000-8000-00000000ae01");
        seedCanonicalIdentity(originalA, Instant.now().minusSeconds(60));
        Instant acceptedInstant = pg.jdbc().queryForObject("SELECT clock_timestamp()",
                java.sql.Timestamp.class).toInstant();
        LegacyRequest legacy = legacyRequest(mapper, acceptedInstant.toString());
        pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_checksum=?, pending_request_sha256=?
                 WHERE tenant_id=? AND sensor_id='sensor-a'
                """, legacy.checksum(), sha256(legacy.body()), TENANT_A);
        // A existed before V021, but that mutable row did not preserve its
        // historical entity value. Migration must never invent A history.
        pg.migrate();
        UUID entity = UUID.fromString("018f0e0a-0000-7000-8000-00000000ae02");
        pg.jdbc().update("""
                INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name,
                  lifecycle_state, first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?, ?, 'HOST', 'repointed-b', 'ACTIVE', now(), now(), now(), now())
                """, entity, TENANT_A);
        pg.jdbc().update("UPDATE platform.entity_identifier SET entity_id=? "
                + "WHERE tenant_id=? AND identifier_value='host-a'", entity, TENANT_A);
        assertEquals(entity, pg.jdbc().queryForObject("SELECT entity_id FROM platform.entity_identifier "
                + "WHERE tenant_id=? AND identifier_value='host-a'", UUID.class, TENANT_A));
        assertEquals(0, count("SELECT count(*) FROM platform.entity_identifier_binding_history "
                + "WHERE tenant_id='" + TENANT_A + "' AND identifier_value='host-a' "
                + "AND entity_id='" + originalA + "'"));
        Map<String, Object> before = state(TENANT_A, "sensor-a");
        SensorAuthorityResolver authority = mock(SensorAuthorityResolver.class);
        AgentControlRepository agentRepo = mock(AgentControlRepository.class);
        EntityIdentifierRepository identifiers = new EntityIdentifierRepository(
                new NamedParameterJdbcTemplate(pg.dataSource()));
        KafkaTelemetryProducer kafka = mock(KafkaTelemetryProducer.class);
        X509Certificate certificate = mock(X509Certificate.class);
        when(authority.resolve(certificate)).thenReturn(
                new SensorScope("sensor-a", TENANT_A, null, Set.of(entity), true));
        Instant created = acceptedInstant.minusSeconds(60);
        Instant revoked = pg.jdbc().queryForObject("SELECT clock_timestamp()",
                java.sql.Timestamp.class).toInstant();
        when(agentRepo.findAssignmentByGeneration(TENANT_A, "agent-a", 1))
                .thenReturn(Optional.of(new AgentSensorAssignment(UUID.randomUUID(), TENANT_A,
                        "agent-a", "sensor-a", 1, AgentAssignmentStatus.REVOKED,
                        created, created, revoked)));
        // The current binding was established after Sensor acceptance; it cannot prove the old entity.
        when(agentRepo.findActiveEntityBinding(TENANT_A, "agent-a"))
                .thenReturn(Optional.of(new EntityAgentBinding(TENANT_A, entity, "agent-a",
                        "a".repeat(64), "host-a", "linux", revoked, revoked, "ACTIVE", 1)));
        AgentIngestionValidator validator = new AgentIngestionValidator(mapper, authority,
                agentRepo, identifiers, new IngestionMetrics());
        AgentIngestionService service = new AgentIngestionService(validator, store(), kafka,
                new IngestionMetrics());
        IngestionException failure = assertThrows(IngestionException.class, () ->
                service.ingestAgentTelemetry(legacy.body(), legacy.request(),
                        legacy.request().batchId(), certificate));
        assertEquals("ERR_AGENT_LEGACY_PENDING_RECONCILIATION_REQUIRED", failure.getCode());
        assertEquals(before, state(TENANT_A, "sensor-a"));
        assertEquals("LEGACY_UNKNOWN", pendingState(TENANT_A, "sensor-a").get("pending_authority_state"));
        assertEquals(0, count("SELECT count(*) FROM platform.sensor_agent_ingress_pending_canonical_binding"));
        verifyNoInteractions(kafka);
    }

    @Test
    void legacyMissingExactBodyDigestRetainsPendingForReconciliation() {
        seedAccepted(TENANT_A, "sensor-a");
        seedPending(TENANT_A, "sensor-a");
        pg.jdbc().update("UPDATE platform.sensor_agent_ingress_watermark "
                + "SET pending_request_sha256=NULL WHERE tenant_id=? AND sensor_id='sensor-a'", TENANT_A);
        pg.migrate();
        Map<String, Object> before = state(TENANT_A, "sensor-a");
        ReplayReservation rejected = store().reserveBatchWithBody(TENANT_A, "sensor-a", STREAM,
                3, 3, CHECKSUM, "pending-batch", "pending-corr", BODY_SHA);
        assertEquals(ReplayReservation.Status.REJECTED, rejected.status());
        assertEquals("ERR_AGENT_LEGACY_PENDING_RECONCILIATION_REQUIRED", rejected.rejectionCode());
        assertEquals(before, state(TENANT_A, "sensor-a"));
        assertEquals("LEGACY_UNKNOWN", pendingState(TENANT_A, "sensor-a").get("pending_authority_state"));
    }

    @Test
    void u7AcceptedAndSettledHistoryRemainsUnchangedAfterMigration() {
        seedAccepted(TENANT_A, "sensor-a");
        Map<String, Object> before = state(TENANT_A, "sensor-a");
        pg.migrate();
        assertEquals(before, state(TENANT_A, "sensor-a"));
        assertEquals(2L, state(TENANT_A, "sensor-a").get("acked_through_sequence"));
        assertEquals(2L, state(TENANT_A, "sensor-a").get("settled_through_sequence"));
    }

    private static String sha256(byte[] bytes) throws Exception {
        return HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(bytes));
    }

    private static LegacyRequest legacyRequest(ObjectMapper mapper) throws Exception {
        return legacyRequest(mapper, "2026-11-09T10:00:00Z");
    }

    private static LegacyRequest legacyRequest(ObjectMapper mapper, String acceptedAt) throws Exception {
        AgentIngressRecord record = new AgentIngressRecord(3, "ingress-3", TENANT_A.toString(),
                "agent-a", "sensor-a", 1, 10, 10, "agent-batch", "agent-corr",
                acceptedAt,
                new com.platform.core.ingestion.agentingress.dto.SensorAcceptanceEvidenceV1(
                        mapper.valueToTree(1), acceptedAt, "QUALIFIED",
                        mapper.valueToTree(0), mapper.valueToTree(0), "TEST_CLOCK",
                        "migration-sensor-instance", mapper.valueToTree(3)),
                List.of(mapper.readTree("""
                    {"sequence":10,"id":"delivery-10","checksum":"abc",
                     "payload":{"hostId":"host-a","collectedAt":"%s",
                     "observations":[{"metricKey":"cpu.util","value":42.5,
                                      "observedAt":"%s"}]}}
                    """.formatted(acceptedAt, acceptedAt))));
        String line = "3|agent-batch|10|10|1|" + TENANT_A + "|agent-a|sensor-a|10|delivery-10|abc";
        String checksum = sha256(line.getBytes(StandardCharsets.UTF_8));
        AgentTelemetryBatchRequest request = new AgentTelemetryBatchRequest(2, "sensor-a", STREAM,
                "pending-batch", 3, 3, 1, checksum, "pending-corr", List.of(record));
        return new LegacyRequest(request, mapper.writeValueAsBytes(request), checksum);
    }

    private record LegacyRequest(AgentTelemetryBatchRequest request, byte[] body, String checksum) { }

    private void seedCanonicalIdentity(UUID entity, Instant firstSeen) {
        pg.jdbc().update("""
                INSERT INTO platform.agent (agent_id, tenant_id, status, created_at, updated_at)
                VALUES ('agent-a', ?, 'ACTIVE', ?, now())
                """, TENANT_A, java.sql.Timestamp.from(firstSeen));
        pg.jdbc().update("""
                INSERT INTO platform.entity
                  (entity_id, tenant_id, entity_type, display_name, lifecycle_state,
                   first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?, ?, 'HOST', 'historical-agent', 'ACTIVE', ?, now(), ?, now())
                """, entity, TENANT_A, java.sql.Timestamp.from(firstSeen), java.sql.Timestamp.from(firstSeen));
        pg.jdbc().update("""
                INSERT INTO platform.entity_identifier
                  (identifier_id, tenant_id, entity_id, identifier_type, namespace,
                   identifier_value, confidence, first_seen_at, last_seen_at)
                VALUES ('018f0e0a-0000-7000-8000-00000000a101', ?, ?,
                        'HOST_ID', 'platform.agent', 'host-a', 1.0, ?, now())
                """, TENANT_A, entity, java.sql.Timestamp.from(firstSeen));
    }

    private PostgresAgentIngressWatermarkStore store() {
        return new PostgresAgentIngressWatermarkStore(pg.jdbc(),
                new DataSourceTransactionManager(pg.dataSource()));
    }

    private void seedTenant(UUID tenant) {
        pg.jdbc().update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?, 'Migration Test Tenant', 'SINGLE', 'ACTIVE', 'UTC', now(), now())
                ON CONFLICT (tenant_id) DO NOTHING
                """, tenant);
    }

    private void seedAccepted(UUID tenant, String sensor) {
        pg.jdbc().update("""
                INSERT INTO platform.sensor_agent_ingress_watermark (
                    tenant_id, sensor_id, stream_id, acked_through_sequence,
                    settled_through_sequence, last_first_sequence, last_last_sequence,
                    last_checksum, last_batch_id, last_request_sha256)
                VALUES (?, ?, ?, 2, 2, 2, 2, ?, 'accepted-batch', ?)
                """, tenant, sensor, STREAM, CHECKSUM, BODY_SHA);
    }

    private void seedPending(UUID tenant, String sensor) {
        pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_first_sequence=3, pending_last_sequence=3,
                       pending_checksum=?, pending_request_sha256=?,
                       pending_batch_id='pending-batch', pending_correlation_id='pending-corr',
                       pending_reserved_at=clock_timestamp()
                 WHERE tenant_id=? AND sensor_id=?
                """, CHECKSUM, BODY_SHA, tenant, sensor);
    }

    private Map<String, Object> state(UUID tenant, String sensor) {
        return pg.jdbc().queryForMap("""
                SELECT acked_through_sequence, settled_through_sequence,
                       last_first_sequence, last_last_sequence, last_checksum, last_batch_id,
                       last_request_sha256, pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_request_sha256, pending_batch_id,
                       pending_correlation_id, pending_reserved_at, pending_publisher_owner,
                       pending_publisher_claimed_at, pending_publisher_lease_until
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenant, sensor);
    }

    private Map<String, Object> pendingState(UUID tenant, String sensor) {
        return pg.jdbc().queryForMap("""
                SELECT pending_reservation_id, pending_authority_state,
                       pending_authority_validated_at, pending_canonical_batch_sha256,
                       pending_publication_attempted
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenant, sensor);
    }

    private void assertV021Absent() {
        assertEquals(0, count("SELECT count(*) FROM platform.schema_history WHERE version='V021'"));
        assertEquals(0, count("SELECT count(*) FROM information_schema.tables WHERE table_schema='platform' "
                + "AND table_name='sensor_agent_ingress_pending_canonical_binding'"));
        assertEquals(0, count("""
                SELECT count(*) FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='sensor_agent_ingress_watermark'
                   AND column_name IN ('pending_reservation_id','pending_publication_attempted',
                       'pending_authority_state','pending_authority_validated_at','pending_canonical_batch_sha256')
                """));
        assertEquals(0, count("""
                SELECT count(*) FROM pg_constraint
                 WHERE conrelid='platform.sensor_agent_ingress_watermark'::regclass
                   AND conname='chk_agent_ingress_pending_release_guard'
                """));
    }

    private int count(String sql) {
        return pg.jdbc().queryForObject(sql, Integer.class);
    }

    private static String deepestSqlState(Throwable error) {
        String state = null;
        for (Throwable current = error; current != null; current = current.getCause()) {
            if (current instanceof java.sql.SQLException sql) state = sql.getSQLState();
        }
        return state;
    }

    private static MigrationCatalog catalogThroughV020() {
        MigrationCatalog real = new MigrationCatalog();
        return new MigrationCatalog() {
            @Override
            public List<MigrationDefinition> load() {
                return real.load().stream().filter(definition ->
                        definition.version().compareTo("V020") <= 0).toList();
            }

            @Override
            public String loadSql(MigrationDefinition definition) {
                return real.loadSql(definition);
            }
        };
    }
}
