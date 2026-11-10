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
        assertEquals(5, count("""
                SELECT count(*) FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='sensor_agent_ingress_watermark'
                   AND column_name IN ('pending_reservation_id','pending_publication_attempted',
                       'pending_authority_state','pending_authority_validated_at','pending_entity_ids')
                """));
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
        assertNull(pending.get("pending_entity_ids"));
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
    void u5LegacyExactRetryValidatesRevokedHistoryPublishesAndFinalizes() throws Exception {
        ObjectMapper mapper = new ObjectMapper();
        UUID entity = UUID.randomUUID();
        LegacyRequest legacy = legacyRequest(mapper);
        seedAccepted(TENANT_A, "sensor-a");
        seedPending(TENANT_A, "sensor-a");
        pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_checksum=?, pending_request_sha256=?
                 WHERE tenant_id=? AND sensor_id='sensor-a'
                """, legacy.checksum(), sha256(legacy.body()), TENANT_A);
        pg.migrate();
        assertEquals("LEGACY_UNKNOWN", pendingState(TENANT_A, "sensor-a").get("pending_authority_state"));
        SensorAuthorityResolver authority = mock(SensorAuthorityResolver.class);
        AgentControlRepository agentRepo = mock(AgentControlRepository.class);
        EntityIdentifierRepository identifiers = mock(EntityIdentifierRepository.class);
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
        when(identifiers.findByResolutionKey(TENANT_A, "platform.agent", "HOST_ID", "host-a"))
                .thenReturn(Optional.empty());
        AgentIngestionValidator validator = new AgentIngestionValidator(mapper, authority,
                agentRepo, identifiers, new IngestionMetrics());
        AgentIngestionService service = new AgentIngestionService(validator, store(), kafka,
                new IngestionMetrics());
        var ack = service.ingestAgentTelemetry(legacy.body(), legacy.request(),
                legacy.request().batchId(), certificate);
        assertEquals("ACCEPTED", ack.status());
        verify(kafka).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        assertEquals(3L, state(TENANT_A, "sensor-a").get("settled_through_sequence"));
        assertNull(state(TENANT_A, "sensor-a").get("pending_first_sequence"));
        assertEquals("NONE", pendingState(TENANT_A, "sensor-a").get("pending_authority_state"));
    }

    @Test
    void u6LegacyExactRetryWithoutHistoricalEntityProofRetainsPending() throws Exception {
        ObjectMapper mapper = new ObjectMapper();
        LegacyRequest legacy = legacyRequest(mapper);
        seedAccepted(TENANT_A, "sensor-a");
        seedPending(TENANT_A, "sensor-a");
        pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_checksum=?, pending_request_sha256=?
                 WHERE tenant_id=? AND sensor_id='sensor-a'
                """, legacy.checksum(), sha256(legacy.body()), TENANT_A);
        pg.migrate();
        Map<String, Object> before = state(TENANT_A, "sensor-a");
        SensorAuthorityResolver authority = mock(SensorAuthorityResolver.class);
        AgentControlRepository agentRepo = mock(AgentControlRepository.class);
        EntityIdentifierRepository identifiers = mock(EntityIdentifierRepository.class);
        KafkaTelemetryProducer kafka = mock(KafkaTelemetryProducer.class);
        X509Certificate certificate = mock(X509Certificate.class);
        UUID entity = UUID.randomUUID();
        when(authority.resolve(certificate)).thenReturn(
                new SensorScope("sensor-a", TENANT_A, null, Set.of(entity), true));
        Instant created = Instant.parse("2026-11-09T09:00:00Z");
        Instant revoked = Instant.parse("2026-11-09T11:00:00Z");
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
        AgentIngressRecord record = new AgentIngressRecord(3, "ingress-3", TENANT_A.toString(),
                "agent-a", "sensor-a", 1, 10, 10, "agent-batch", "agent-corr",
                "2026-11-09T10:00:00Z", List.of(mapper.readTree("""
                    {"sequence":10,"id":"delivery-10","checksum":"abc",
                     "payload":{"hostId":"host-a","collectedAt":"2026-11-09T10:00:00Z",
                     "observations":[{"metricKey":"cpu.util","value":42.5,
                                      "observedAt":"2026-11-09T10:00:00Z"}]}}
                    """)));
        String line = "3|agent-batch|10|10|1|" + TENANT_A + "|agent-a|sensor-a|10|delivery-10|abc";
        String checksum = sha256(line.getBytes(StandardCharsets.UTF_8));
        AgentTelemetryBatchRequest request = new AgentTelemetryBatchRequest(2, "sensor-a", STREAM,
                "pending-batch", 3, 3, 1, checksum, "pending-corr", List.of(record));
        return new LegacyRequest(request, mapper.writeValueAsBytes(request), checksum);
    }

    private record LegacyRequest(AgentTelemetryBatchRequest request, byte[] body, String checksum) { }

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
                       pending_authority_validated_at, pending_entity_ids,
                       pending_publication_attempted
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenant, sensor);
    }

    private void assertV021Absent() {
        assertEquals(0, count("SELECT count(*) FROM platform.schema_history WHERE version='V021'"));
        assertEquals(0, count("""
                SELECT count(*) FROM information_schema.columns
                 WHERE table_schema='platform' AND table_name='sensor_agent_ingress_watermark'
                   AND column_name IN ('pending_reservation_id','pending_publication_attempted',
                       'pending_authority_state','pending_authority_validated_at','pending_entity_ids')
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
