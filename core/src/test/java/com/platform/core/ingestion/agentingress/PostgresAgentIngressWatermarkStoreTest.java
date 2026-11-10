package com.platform.core.ingestion.agentingress;

import com.platform.core.ingestion.replay.PublisherClaimResult;
import com.platform.core.ingestion.replay.ReplayReservation;
import com.platform.core.ingestion.agentingress.dto.AgentIngressRecord;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryBatchRequest;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.IngestionMetrics;
import com.platform.core.ingestion.KafkaTelemetryProducer;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryObservationBatch;
import com.platform.core.agentcontrol.EntityAgentBinding;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;

import java.util.Map;
import java.util.List;
import java.util.UUID;
import java.time.Instant;
import java.security.cert.X509Certificate;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.util.Set;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;
import static org.mockito.ArgumentMatchers.*;
import org.springframework.http.HttpStatus;

@Tag("postgres")
class PostgresAgentIngressWatermarkStoreTest {

    private PostgresTestSupport pg;
    private PostgresAgentIngressWatermarkStore store;
    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private final String streamId = "stream-00112233445566778899aabbccddeeff";

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        store = new PostgresAgentIngressWatermarkStore(pg.jdbc(), new DataSourceTransactionManager(pg.dataSource()));
        pg.jdbc().update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?, 'Test Tenant', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                ON CONFLICT (tenant_id) DO NOTHING
                """, tenantId);
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    private void claimAndFinalize(
            String sensorId, long first, long last, String checksum, String batchId, String correlationId
    ) {
        PublisherClaimResult claim = claimExact(store,
                tenantId, sensorId, streamId, first, last, checksum, batchId, correlationId);
        assertEquals(PublisherClaimResult.Status.CLAIMED, claim.status());
        store.finalizeReservedBatch(
                tenantId, sensorId, streamId, first, last, checksum, batchId, correlationId,
                claim.ownerToken(), claim.publisherEpoch());
    }

    private PublisherClaimResult claimExact(PostgresAgentIngressWatermarkStore subject,
            UUID tenant, String sensor, String stream, long first, long last,
            String checksum, String batchId, String correlationId) {
        List<Map<String, Object>> rows = pg.jdbc().queryForList("""
                SELECT pending_first_sequence, pending_last_sequence, pending_checksum,
                       pending_batch_id, pending_correlation_id, pending_request_sha256,
                       pending_reservation_id, pending_authority_state
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenant, sensor);
        if (rows.isEmpty() || rows.get(0).get("pending_first_sequence") == null) {
            return subject.claimPublisherLeaseExact(tenant, sensor, stream, first, last,
                    checksum, batchId, correlationId, "d".repeat(64), UUID.randomUUID());
        }
        Map<String, Object> row = rows.get(0);
        String digest = (String) row.get("pending_request_sha256");
        if (digest == null) {
            digest = "d".repeat(64);
            pg.jdbc().update("""
                    UPDATE platform.sensor_agent_ingress_watermark
                       SET pending_request_sha256=? WHERE tenant_id=? AND sensor_id=?
                    """, digest, tenant, sensor);
        }
        UUID reservationId = (UUID) row.get("pending_reservation_id");
        if ("UNVALIDATED".equals(row.get("pending_authority_state"))) {
            subject.checkpointAuthority(tenant, sensor, stream,
                    ((Number) row.get("pending_first_sequence")).longValue(),
                    ((Number) row.get("pending_last_sequence")).longValue(),
                    (String) row.get("pending_checksum"), (String) row.get("pending_batch_id"),
                    (String) row.get("pending_correlation_id"), digest, reservationId,
                    java.util.Collections.nCopies((int) (((Number) row.get("pending_last_sequence")).longValue()
                            - ((Number) row.get("pending_first_sequence")).longValue() + 1), UUID.randomUUID()));
        }
        return subject.claimPublisherLeaseExact(tenant, sensor, stream, first, last,
                checksum, batchId, correlationId, digest, reservationId);
    }

    private AgentIngressRecord terminalRecord(String sensorId, long sequence) {
        return new AgentIngressRecord(sequence, "ingress-record-" + sequence,
                tenantId.toString(), terminalAgentId(sensorId), sensorId, 1,
                sequence, sequence, "agent-batch-" + sequence,
                "agent-correlation-" + sequence, java.time.Instant.now().toString(),
                List.of(new com.fasterxml.jackson.databind.ObjectMapper().createObjectNode()));
    }

    private static String terminalAgentId(String sensorId) {
        return "agent-" + sensorId;
    }

    @Test
    void authorityCheckpointAndPublisherClaimAreExactRequestScoped() {
        String sensor = "sensor-checkpoint-exact";
        String checksum = "a".repeat(64);
        String digest = "d".repeat(64);
        ReplayReservation reserved = store.reserveBatchWithBody(tenantId, sensor, streamId,
                1, 1, checksum, "batch", "corr", digest);
        UUID id = reserved.pendingReservationId();
        UUID entity = UUID.randomUUID();
        assertEquals("UNVALIDATED", reserved.pendingAuthorityState());
        assertEquals(PublisherClaimResult.Status.REJECTED,
                store.claimPublisherLeaseExact(tenantId, sensor, streamId, 1, 1,
                        checksum, "batch", "corr", digest, id).status());
        assertFalse(store.checkpointAuthority(tenantId, sensor, streamId, 1, 1,
                checksum, "batch", "corr", "e".repeat(64), id, List.of(entity)));
        assertFalse(store.checkpointAuthority(tenantId, sensor, streamId, 1, 1,
                "b".repeat(64), "batch", "corr", digest, id, List.of(entity)));
        assertFalse(store.checkpointAuthority(UUID.randomUUID(), sensor, streamId, 1, 1,
                checksum, "batch", "corr", digest, id, List.of(entity)));
        assertFalse(store.checkpointAuthority(tenantId, "other-sensor", streamId, 1, 1,
                checksum, "batch", "corr", digest, id, List.of(entity)));
        assertFalse(store.checkpointAuthority(tenantId, sensor,
                "stream-ffffffffffffffffffffffffffffffff", 1, 1,
                checksum, "batch", "corr", digest, id, List.of(entity)));
        assertFalse(store.checkpointAuthority(tenantId, sensor, streamId, 1, 1,
                checksum, "other-batch", "corr", digest, id, List.of(entity)));
        assertFalse(store.checkpointAuthority(tenantId, sensor, streamId, 1, 1,
                checksum, "batch", "other-corr", digest, id, List.of(entity)));
        assertFalse(store.checkpointAuthority(tenantId, sensor, streamId, 1, 2,
                checksum, "batch", "corr", digest, id, List.of(entity, entity)));
        assertFalse(store.checkpointAuthority(tenantId, sensor, streamId, 1, 1,
                checksum, "batch", "corr", digest, UUID.randomUUID(), List.of(entity)));
        assertEquals("UNVALIDATED", pg.jdbc().queryForObject("""
                SELECT pending_authority_state FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, String.class, tenantId, sensor));
        assertTrue(store.checkpointAuthority(tenantId, sensor, streamId, 1, 1,
                checksum, "batch", "corr", digest, id, List.of(entity)));
        assertThrows(IllegalStateException.class, () -> store.terminallyRejectIdentityConflict(
                tenantId, sensor, streamId, 1, checksum, "batch", "corr",
                terminalRecord(sensor, 1), entity, 1L));
        assertEquals(ReplayReservation.Status.REJECTED,
                store.reserveBatchWithBody(tenantId, sensor, streamId, 1, 1,
                        checksum, "batch", "corr", "e".repeat(64)).status());
        assertEquals(PublisherClaimResult.Status.REJECTED,
                store.claimPublisherLeaseExact(tenantId, sensor, streamId, 1, 1,
                        checksum, "batch", "corr", "e".repeat(64), id).status());
        assertEquals(PublisherClaimResult.Status.REJECTED,
                store.claimPublisherLeaseExact(tenantId, sensor, streamId, 1, 1,
                        checksum, "batch", "corr", digest, UUID.randomUUID()).status());
        assertEquals(PublisherClaimResult.Status.CLAIMED,
                store.claimPublisherLeaseExact(tenantId, sensor, streamId, 1, 1,
                        checksum, "batch", "corr", digest, id).status());
    }

    @Test
    void legacyUnknownCannotClaimBeforeHistoricalValidation() {
        String sensor = "sensor-legacy-unknown-claim";
        String checksum = "a".repeat(64);
        String digest = "d".repeat(64);
        ReplayReservation pending = store.reserveBatchWithBody(tenantId, sensor, streamId,
                1, 1, checksum, "batch", "corr", digest);
        pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_authority_state='LEGACY_UNKNOWN', pending_publication_attempted=NULL
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensor);
        assertEquals(PublisherClaimResult.Status.REJECTED,
                store.claimPublisherLeaseExact(tenantId, sensor, streamId, 1, 1,
                        checksum, "batch", "corr", digest, pending.pendingReservationId()).status());
        assertEquals("LEGACY_UNKNOWN", pg.jdbc().queryForObject("""
                SELECT pending_authority_state FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, String.class, tenantId, sensor));
    }

    @Test
    void deterministicRejectionReleasesOnlyExactUnpublishedGeneration() {
        String sensorId = "sensor-pending-release";
        String checksum = "a".repeat(64);
        String digest = "d".repeat(64);
        ReplayReservation stale = store.reserveBatchWithBody(tenantId, sensorId, streamId,
                1, 1, checksum, "stale-batch", "stale-corr", digest);
        assertEquals(ReplayReservation.Status.RESERVED, stale.status());
        assertNotNull(stale.pendingReservationId());
        assertTrue(store.releaseUnpublishedPending(tenantId, sensorId, streamId,
                1, 1, checksum, "stale-batch", "stale-corr", digest, stale.pendingReservationId()));
        Map<String, Object> cleared = pg.jdbc().queryForMap("""
                SELECT acked_through_sequence, settled_through_sequence, pending_first_sequence,
                       pending_request_sha256, pending_reservation_id, pending_publisher_owner,
                       pending_publisher_lease_until, pending_publication_attempted
                  FROM platform.sensor_agent_ingress_watermark WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(0L, ((Number) cleared.get("acked_through_sequence")).longValue());
        assertEquals(0L, ((Number) cleared.get("settled_through_sequence")).longValue());
        assertNull(cleared.get("pending_first_sequence"));
        assertNull(cleared.get("pending_request_sha256"));
        assertNull(cleared.get("pending_reservation_id"));
        assertNull(cleared.get("pending_publisher_owner"));
        assertNull(cleared.get("pending_publisher_lease_until"));
        assertEquals(false, cleared.get("pending_publication_attempted"));

        ReplayReservation corrected = store.reserveBatchWithBody(tenantId, sensorId, streamId,
                1, 1, "b".repeat(64), "corrected-batch", "corrected-corr", "e".repeat(64));
        assertEquals(ReplayReservation.Status.RESERVED, corrected.status());
        assertNotEquals(stale.pendingReservationId(), corrected.pendingReservationId());
        assertFalse(store.releaseUnpublishedPending(tenantId, sensorId, streamId,
                1, 1, checksum, "stale-batch", "stale-corr", digest, stale.pendingReservationId()));
        assertEquals(corrected.pendingReservationId(), pg.jdbc().queryForObject("""
                SELECT pending_reservation_id FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, UUID.class, tenantId, sensorId));
    }

    @Test
    void pendingReleaseCannotErasePublisherOwnedAttemptedOrFinalizedBatch() {
        String sensorId = "sensor-pending-owned";
        String checksum = "a".repeat(64);
        String digest = "d".repeat(64);
        ReplayReservation pending = store.reserveBatchWithBody(tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr", digest);
        PublisherClaimResult claim = claimExact(store, tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr");
        assertEquals(PublisherClaimResult.Status.CLAIMED, claim.status());
        assertFalse(store.releaseUnpublishedPending(tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr", digest, pending.pendingReservationId()));
        store.releasePublisherLease(tenantId, sensorId, streamId, 1, 1, checksum, "batch", "corr",
                claim.ownerToken(), claim.publisherEpoch());
        assertFalse(store.releaseUnpublishedPending(tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr", digest, pending.pendingReservationId()),
                "released lease does not prove Kafka was never attempted");
        PublisherClaimResult retry = claimExact(store, tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr");
        store.finalizeReservedBatch(tenantId, sensorId, streamId, 1, 1, checksum, "batch", "corr",
                retry.ownerToken(), retry.publisherEpoch());
        assertFalse(store.releaseUnpublishedPending(tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr", digest, pending.pendingReservationId()));
        assertEquals(1L, pg.jdbc().queryForObject("""
                SELECT settled_through_sequence FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, Long.class, tenantId, sensorId));
    }

    @Test
    void pendingReleaseCannotEraseTerminalDisposition() {
        String sensorId = "sensor-pending-terminal";
        UUID entityId = UUID.randomUUID();
        seedTerminalAuthority(sensorId, entityId);
        String checksum = "a".repeat(64);
        String digest = "d".repeat(64);
        ReplayReservation pending = store.reserveBatchWithBody(tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr", digest);
        ReplayReservation terminal = store.terminallyRejectIdentityConflict(tenantId, sensorId, streamId,
                1, checksum, "batch", "corr", terminalRecord(sensorId, 1), entityId, 1L);
        assertEquals(ReplayReservation.Status.TERMINALLY_REJECTED, terminal.status());
        assertFalse(store.releaseUnpublishedPending(tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr", digest, pending.pendingReservationId()));
        assertEquals(1L, pg.jdbc().queryForObject("""
                SELECT settled_through_sequence FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, Long.class, tenantId, sensorId));
    }

    @Test
    void concurrentPublisherClaimAndPendingReleaseSerializeWithoutErasingClaim() throws Exception {
        String sensorId = "sensor-pending-race";
        String checksum = "a".repeat(64);
        String digest = "d".repeat(64);
        ReplayReservation pending = store.reserveBatchWithBody(tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr", digest);
        CountDownLatch start = new CountDownLatch(1);
        ExecutorService pool = Executors.newFixedThreadPool(2);
        try {
            var release = pool.submit(() -> {
                start.await();
                return store.releaseUnpublishedPending(tenantId, sensorId, streamId,
                        1, 1, checksum, "batch", "corr", digest, pending.pendingReservationId());
            });
            var claim = pool.submit(() -> {
                start.await();
                return claimExact(store, tenantId, sensorId, streamId,
                        1, 1, checksum, "batch", "corr");
            });
            start.countDown();
            boolean released = release.get(10, TimeUnit.SECONDS);
            PublisherClaimResult claimed = claim.get(10, TimeUnit.SECONDS);
            if (released) {
                assertEquals(PublisherClaimResult.Status.REJECTED, claimed.status());
                assertNull(pg.jdbc().queryForObject("""
                        SELECT pending_first_sequence FROM platform.sensor_agent_ingress_watermark
                         WHERE tenant_id=? AND sensor_id=?
                        """, Long.class, tenantId, sensorId));
            } else {
                assertEquals(PublisherClaimResult.Status.CLAIMED, claimed.status());
                assertEquals(claimed.ownerToken(), pg.jdbc().queryForObject("""
                        SELECT pending_publisher_owner FROM platform.sensor_agent_ingress_watermark
                         WHERE tenant_id=? AND sensor_id=?
                        """, UUID.class, tenantId, sensorId));
                assertEquals(pending.pendingReservationId(), pg.jdbc().queryForObject("""
                        SELECT pending_reservation_id FROM platform.sensor_agent_ingress_watermark
                         WHERE tenant_id=? AND sensor_id=?
                        """, UUID.class, tenantId, sensorId));
            }
        } finally {
            pool.shutdownNow();
        }
    }

    @Test
    void publisherClaimQueuedFirstBlocksReleaseUntilClaimCommits() throws Exception {
        String sensorId = "sensor-pending-claim-first";
        String checksum = "a".repeat(64);
        String digest = "d".repeat(64);
        ReplayReservation pending = store.reserveBatchWithBody(tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr", digest);
        ExecutorService pool = Executors.newFixedThreadPool(2);
        try (Connection gate = holdSensorAdvisoryLock(sensorId)) {
            var claim = pool.submit(() -> claimExact(store, tenantId, sensorId, streamId,
                    1, 1, checksum, "batch", "corr"));
            awaitAdvisoryWaiters(1);
            var release = pool.submit(() -> store.releaseUnpublishedPending(tenantId, sensorId, streamId,
                    1, 1, checksum, "batch", "corr", digest, pending.pendingReservationId()));
            awaitAdvisoryWaiters(2);
            assertFalse(claim.isDone());
            assertFalse(release.isDone());
            gate.commit();
            PublisherClaimResult claimed = claim.get(10, TimeUnit.SECONDS);
            assertEquals(PublisherClaimResult.Status.CLAIMED, claimed.status());
            assertFalse(release.get(10, TimeUnit.SECONDS));
            assertEquals(claimed.ownerToken(), pg.jdbc().queryForObject("""
                    SELECT pending_publisher_owner FROM platform.sensor_agent_ingress_watermark
                     WHERE tenant_id=? AND sensor_id=?
                    """, UUID.class, tenantId, sensorId));
            assertEquals(true, pg.jdbc().queryForObject("""
                    SELECT pending_publication_attempted FROM platform.sensor_agent_ingress_watermark
                     WHERE tenant_id=? AND sensor_id=?
                    """, Boolean.class, tenantId, sensorId));
        } finally {
            pool.shutdownNow();
        }
    }

    @Test
    void pendingReleaseQueuedFirstPreventsLaterPublisherClaim() throws Exception {
        String sensorId = "sensor-pending-release-first";
        String checksum = "a".repeat(64);
        String digest = "d".repeat(64);
        ReplayReservation pending = store.reserveBatchWithBody(tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr", digest);
        ExecutorService pool = Executors.newFixedThreadPool(2);
        try (Connection gate = holdSensorAdvisoryLock(sensorId)) {
            var release = pool.submit(() -> store.releaseUnpublishedPending(tenantId, sensorId, streamId,
                    1, 1, checksum, "batch", "corr", digest, pending.pendingReservationId()));
            awaitAdvisoryWaiters(1);
            var claim = pool.submit(() -> claimExact(store, tenantId, sensorId, streamId,
                    1, 1, checksum, "batch", "corr"));
            awaitAdvisoryWaiters(2);
            assertFalse(release.isDone());
            assertFalse(claim.isDone());
            gate.commit();
            assertTrue(release.get(10, TimeUnit.SECONDS));
            assertEquals(PublisherClaimResult.Status.REJECTED, claim.get(10, TimeUnit.SECONDS).status());
            assertNull(pg.jdbc().queryForObject("""
                    SELECT pending_first_sequence FROM platform.sensor_agent_ingress_watermark
                     WHERE tenant_id=? AND sensor_id=?
                    """, Long.class, tenantId, sensorId));
            assertNull(pg.jdbc().queryForObject("""
                    SELECT pending_publisher_owner FROM platform.sensor_agent_ingress_watermark
                     WHERE tenant_id=? AND sensor_id=?
                    """, UUID.class, tenantId, sensorId));
        } finally {
            pool.shutdownNow();
        }
    }

    private Connection holdSensorAdvisoryLock(String sensorId) throws Exception {
        Connection connection = pg.dataSource().getConnection();
        connection.setAutoCommit(false);
        try (PreparedStatement statement = connection.prepareStatement(
                "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))")) {
            statement.setString(1, "sensor_agent_ingress_watermark:" + tenantId + ":" + sensorId);
            statement.execute();
        }
        return connection;
    }

    private void awaitAdvisoryWaiters(int expected) throws Exception {
        long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(10);
        while (System.nanoTime() < deadline) {
            Integer blocked = pg.jdbc().queryForObject("""
                    SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND NOT granted
                    """, Integer.class);
            if (blocked != null && blocked == expected) return;
            Thread.sleep(10);
        }
        fail("Expected " + expected + " PostgreSQL advisory-lock waiters");
    }

    @Test
    void pendingReleaseDatabaseFailureRollsBackAndExactRetryCanRecover() {
        String sensorId = "sensor-pending-fault";
        String checksum = "a".repeat(64);
        String digest = "d".repeat(64);
        ReplayReservation pending = store.reserveBatchWithBody(tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr", digest);
        pg.jdbc().execute("""
                CREATE FUNCTION platform.test_pending_release_failure() RETURNS trigger
                LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected release failure'; END; $$
                """);
        pg.jdbc().execute("""
                CREATE TRIGGER test_pending_release_failure BEFORE UPDATE
                ON platform.sensor_agent_ingress_watermark FOR EACH ROW
                WHEN (OLD.pending_first_sequence IS NOT NULL AND NEW.pending_first_sequence IS NULL)
                EXECUTE FUNCTION platform.test_pending_release_failure()
                """);
        try {
            assertThrows(RuntimeException.class, () -> store.releaseUnpublishedPending(
                    tenantId, sensorId, streamId, 1, 1, checksum, "batch", "corr", digest,
                    pending.pendingReservationId()));
            assertEquals(pending.pendingReservationId(), pg.jdbc().queryForObject("""
                    SELECT pending_reservation_id FROM platform.sensor_agent_ingress_watermark
                     WHERE tenant_id=? AND sensor_id=?
                    """, UUID.class, tenantId, sensorId));
            assertEquals(ReplayReservation.Status.RETRY_PENDING, store.reserveBatchWithBody(
                    tenantId, sensorId, streamId, 1, 1, checksum, "batch", "corr", digest).status());
        } finally {
            pg.jdbc().execute("DROP TRIGGER test_pending_release_failure ON platform.sensor_agent_ingress_watermark");
            pg.jdbc().execute("DROP FUNCTION platform.test_pending_release_failure()");
        }
        assertTrue(store.releaseUnpublishedPending(tenantId, sensorId, streamId,
                1, 1, checksum, "batch", "corr", digest, pending.pendingReservationId()));
    }

    private static String terminalFingerprint(String sensorId) {
        try {
            return java.util.HexFormat.of().formatHex(java.security.MessageDigest.getInstance("SHA-256")
                    .digest(sensorId.getBytes(java.nio.charset.StandardCharsets.UTF_8)));
        } catch (java.security.NoSuchAlgorithmException e) {
            throw new IllegalStateException(e);
        }
    }

    private void seedTerminalAuthority(String sensorId, UUID entityId) {
        pg.jdbc().update("""
                INSERT INTO platform.sensor (sensor_id, tenant_id, role, status, created_at, updated_at)
                VALUES (?, ?, 'INFRASTRUCTURE', 'ACTIVE', now(), now())
                """, sensorId, tenantId);
        pg.jdbc().update("""
                INSERT INTO platform.entity (tenant_id, entity_id, entity_type, display_name,
                    lifecycle_state, first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?, ?, 'HOST', 'terminal host', 'ACTIVE', now(), now(), now(), now())
                """, tenantId, entityId);
        pg.jdbc().update("""
                INSERT INTO platform.agent (tenant_id, agent_id, status, created_at, updated_at)
                VALUES (?, ?, 'ACTIVE', now(), now())
                """, tenantId, terminalAgentId(sensorId));
        pg.jdbc().update("""
                INSERT INTO platform.agent_certificate_credential
                    (credential_id, tenant_id, agent_id, fingerprint_sha256, serial_number, issuer_dn,
                     not_before, not_after, status, activated_at, created_at, updated_at)
                VALUES (?, ?, ?, ?, '01', 'CN=test', now()-interval '1 hour',
                        now()+interval '1 day', 'ACTIVE', now(), now(), now())
                """, UUID.randomUUID(), tenantId, terminalAgentId(sensorId), terminalFingerprint(sensorId));
        pg.jdbc().update("""
                INSERT INTO platform.entity_agent_binding
                    (tenant_id, entity_id, agent_id, certificate_identity, system_name, os_identity,
                     first_enrolled_at, last_heartbeat_at, binding_state, reinstall_generation)
                VALUES (?, ?, ?, ?, 'terminal-host', 'linux', now(), now(), 'ACTIVE', 1)
                """, tenantId, entityId, terminalAgentId(sensorId), terminalFingerprint(sensorId));
        pg.jdbc().update("""
                INSERT INTO platform.agent_sensor_assignment
                    (assignment_id, tenant_id, agent_id, sensor_id, generation, status,
                     created_at, updated_at)
                VALUES (?, ?, ?, ?, 1, 'ACTIVE', now(), now())
                """, UUID.randomUUID(), tenantId, terminalAgentId(sensorId), sensorId);
    }

    @Test
    void terminalSettlementIsDurableReplayableAndDoesNotAdvanceAcceptedPrefix() {
        String sensorId = "sensor-terminal-sequence";
        UUID entityId = UUID.randomUUID();
        seedTerminalAuthority(sensorId, entityId);
        String checksum = "a".repeat(64);
        AgentIngressRecord record = terminalRecord(sensorId, 1);

        assertEquals(ReplayReservation.Status.RESERVED, store.reserveBatch(tenantId, sensorId,
                streamId, 1, 1, checksum, "batch-terminal", "corr-terminal").status());
        ReplayReservation terminal = store.terminallyRejectIdentityConflict(tenantId, sensorId,
                streamId, 1, checksum, "batch-terminal", "corr-terminal", record, entityId, 1L);
        assertEquals(ReplayReservation.Status.TERMINALLY_REJECTED, terminal.status());
        assertEquals(0, terminal.ackedThroughSequence());
        assertEquals(1, terminal.settledThroughSequence());
        assertTrue(store.terminalReplayMatchesRecord(tenantId, sensorId,
                terminal.terminalDispositionId(), record));
        assertFalse(store.terminalReplayMatchesRecord(UUID.randomUUID(), sensorId,
                terminal.terminalDispositionId(), record));

        PostgresAgentIngressWatermarkStore restarted = new PostgresAgentIngressWatermarkStore(
                pg.jdbc(), new DataSourceTransactionManager(pg.dataSource()));
        ReplayReservation retry = restarted.reserveBatch(tenantId, sensorId, streamId,
                1, 1, checksum, "batch-terminal", "corr-terminal");
        assertEquals(terminal.terminalDispositionId(), retry.terminalDispositionId());
        ReplayReservation changedTerminalChecksum = restarted.reserveBatch(tenantId, sensorId,
                streamId, 1, 1, "b".repeat(64), "batch-terminal", "corr-terminal");
        assertEquals(ReplayReservation.Status.REJECTED, changedTerminalChecksum.status());
        assertEquals("ERR_REPLAY_CONFLICT", changedTerminalChecksum.rejectionCode());

        assertEquals(ReplayReservation.Status.RESERVED, restarted.reserveBatch(tenantId, sensorId,
                streamId, 2, 2, "c".repeat(64), "batch-next", "corr-next").status());
        PublisherClaimResult claim = claimExact(restarted, tenantId, sensorId, streamId,
                2, 2, "c".repeat(64), "batch-next", "corr-next");
        assertEquals(PublisherClaimResult.Status.CLAIMED, claim.status());
        restarted.finalizeReservedBatch(tenantId, sensorId, streamId, 2, 2,
                "c".repeat(64), "batch-next", "corr-next", claim.ownerToken(), claim.publisherEpoch());
        Map<String, Object> watermarks = pg.jdbc().queryForMap("""
                SELECT acked_through_sequence, settled_through_sequence
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(0L, ((Number) watermarks.get("acked_through_sequence")).longValue());
        assertEquals(2L, ((Number) watermarks.get("settled_through_sequence")).longValue());
        assertEquals(1, pg.jdbc().queryForObject("""
                SELECT count(*) FROM platform.sensor_agent_ingress_terminal_disposition
                 WHERE tenant_id=? AND sensor_id=?
                """, Integer.class, tenantId, sensorId));
    }

    @Test
    void serviceLostTerminalAndAcceptedResponsesUseDurablePostgresDisposition() {
        String sensorId = "sensor-terminal-service";
        UUID entityId = UUID.randomUUID();
        seedTerminalAuthority(sensorId, entityId);
        AgentIngressRecord rejectedRecord = terminalRecord(sensorId, 1);
        AgentIngressRecord acceptedRecord = terminalRecord(sensorId, 2);
        String rejectedChecksum = "d".repeat(64);
        String acceptedChecksum = "e".repeat(64);
        AgentTelemetryBatchRequest rejected = new AgentTelemetryBatchRequest(2, sensorId,
                streamId, "batch-rejected", 1, 1, 1, rejectedChecksum, "corr-rejected",
                List.of(rejectedRecord));
        AgentTelemetryBatchRequest accepted = new AgentTelemetryBatchRequest(2, sensorId,
                streamId, "batch-accepted", 2, 2, 1, acceptedChecksum, "corr-accepted",
                List.of(acceptedRecord));
        SensorScope scope = new SensorScope(sensorId, tenantId, null, Set.of(entityId), true);
        X509Certificate certificate = mock(X509Certificate.class);
        AgentIngestionValidator validator = mock(AgentIngestionValidator.class);
        KafkaTelemetryProducer kafka = mock(KafkaTelemetryProducer.class);
        when(validator.resolveAuthority(certificate)).thenReturn(scope);
        when(validator.validateAndSnapshot(eq(rejected), eq(scope)))
                .thenThrow(new IngestionException(HttpStatus.CONFLICT,
                        "ERR_AGENT_IDENTITY_CONFLICT", "conflicting host"));
        when(validator.validateCurrentTerminalAuthority(rejectedRecord, scope))
                .thenReturn(new EntityAgentBinding(tenantId, entityId, terminalAgentId(sensorId),
                        terminalFingerprint(sensorId), "terminal-host", "linux", Instant.now(), Instant.now(),
                        "ACTIVE", 1));
        when(validator.validateAndSnapshot(eq(accepted), eq(scope)))
                .thenReturn(new AgentIngestionValidator.ValidatedBatch(
                        List.of(new CanonicalObservation(tenantId.toString(), entityId.toString(),
                                "cpu.util", sensorId, Instant.now(), 1.0, 1, 0, "seq-2", "AGENT_HOST")),
                        List.of(entityId)));
        AgentIngestionService service = new AgentIngestionService(validator, store, kafka,
                new IngestionMetrics());

        var first = service.ingestAgentTelemetry(new byte[]{1}, rejected, rejected.batchId(), certificate);
        var lostTerminalResponseRetry = service.ingestAgentTelemetry(
                new byte[]{1}, rejected, rejected.batchId(), certificate);
        assertEquals("TERMINALLY_REJECTED", first.status());
        assertEquals(first.dispositionId(), lostTerminalResponseRetry.dispositionId());
        IngestionException alteredTerminalBody = assertThrows(IngestionException.class, () ->
                service.ingestAgentTelemetry(new byte[]{9}, rejected, rejected.batchId(), certificate));
        assertEquals("ERR_REPLAY_CONFLICT", alteredTerminalBody.getCode());
        verify(validator, times(1)).validateAndSnapshot(eq(rejected), eq(scope));
        verify(kafka, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));

        var acceptedResponse = service.ingestAgentTelemetry(new byte[]{2}, accepted,
                accepted.batchId(), certificate);
        var lostAcceptedResponseRetry = service.ingestAgentTelemetry(new byte[]{2}, accepted,
                accepted.batchId(), certificate);
        assertEquals("ACCEPTED", acceptedResponse.status());
        assertEquals(0L, acceptedResponse.acceptedThroughIngressSequence());
        assertEquals(2L, acceptedResponse.acceptedLastIngressSequence());
        assertEquals(acceptedResponse, lostAcceptedResponseRetry);
        IngestionException alteredAcceptedBody = assertThrows(IngestionException.class, () ->
                service.ingestAgentTelemetry(new byte[]{9}, accepted, accepted.batchId(), certificate));
        assertEquals("ERR_REPLAY_CONFLICT", alteredAcceptedBody.getCode());
        verify(kafka, times(1)).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        verify(validator, times(1)).validateAndSnapshot(eq(accepted), eq(scope));
    }

    @Test
    void historicalNullBatchIdentityRetainsAcceptedReplayCompatibility() {
        String sensorId = "sensor-legacy-null-batch";
        assertEquals(ReplayReservation.Status.RESERVED, store.reserveBatch(tenantId, sensorId,
                streamId, 1, 1, "legacy-checksum", "original-batch", "corr").status());
        claimAndFinalize(sensorId, 1, 1, "legacy-checksum", "original-batch", "corr");
        pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark SET last_batch_id = NULL
                 WHERE tenant_id = ? AND sensor_id = ?
                """, tenantId, sensorId);
        assertEquals(ReplayReservation.Status.ALREADY_ACKED, store.reserveBatch(tenantId, sensorId,
                streamId, 1, 1, "legacy-checksum", "historical-retry", "corr").status());
        assertEquals(ReplayReservation.Status.REJECTED, store.reserveBatch(tenantId, sensorId,
                streamId, 1, 1, "changed-checksum", "historical-retry", "corr").status());
    }

    @Test
    void revokedAssignmentCannotCreateTerminalSkip() {
        String sensorId = "sensor-terminal-stale";
        UUID entityId = UUID.randomUUID();
        seedTerminalAuthority(sensorId, entityId);
        String checksum = "a".repeat(64);
        assertEquals(ReplayReservation.Status.RESERVED, store.reserveBatch(tenantId, sensorId,
                streamId, 1, 1, checksum, "stale-batch", "stale-corr").status());
        pg.jdbc().update("""
                UPDATE platform.agent_sensor_assignment SET status='REVOKED', revoked_at=now()
                 WHERE tenant_id=? AND agent_id=?
                """, tenantId, terminalAgentId(sensorId));
        IngestionException rejected = assertThrows(IngestionException.class, () ->
                store.terminallyRejectIdentityConflict(tenantId, sensorId, streamId, 1,
                        checksum, "stale-batch", "stale-corr", terminalRecord(sensorId, 1), entityId, 1L));
        assertEquals("ERR_AGENT_ASSIGNMENT_UNKNOWN", rejected.getCode());
        assertEquals(0, pg.jdbc().queryForObject("""
                SELECT count(*) FROM platform.sensor_agent_ingress_terminal_disposition
                 WHERE tenant_id=? AND sensor_id=?
                """, Integer.class, tenantId, sensorId));
        assertEquals(0L, pg.jdbc().queryForObject("""
                SELECT settled_through_sequence FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, Long.class, tenantId, sensorId));
    }

    @Test
    void invalidStreamAndHiddenInitialGapAreRejectedWithoutCreatingState() {
        String sensorId = "sensor-agent-new-stream";

        ReplayReservation invalidStream = store.reserveBatch(
                tenantId, sensorId, "epoch-process-local",
                1L, 1L, "checksum-a", "batch-a", "corr-a");
        assertEquals(ReplayReservation.Status.REJECTED, invalidStream.status());
        assertEquals("ERR_AGENT_STREAM_ID_INVALID", invalidStream.rejectionCode());

        ReplayReservation hiddenGap = store.reserveBatch(
                tenantId, sensorId, streamId,
                100L, 100L, "checksum-b", "batch-b", "corr-b");
        assertEquals(ReplayReservation.Status.REJECTED, hiddenGap.status());
        assertEquals("ERR_SEQUENCE_GAP", hiddenGap.rejectionCode());

        Integer rows = pg.jdbc().queryForObject(
                "SELECT count(*) FROM platform.sensor_agent_ingress_watermark WHERE tenant_id=? AND sensor_id=?",
                Integer.class, tenantId, sensorId
        );
        assertEquals(0, rows);
    }

    @Test
    void reserveClaimFinalizeCycleIsDurableBoundedAndIdempotent() {
        String sensorId = "sensor-agent-reservation-01";

        ReplayReservation first = store.reserveBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a", "corr-a");
        assertEquals(ReplayReservation.Status.RESERVED, first.status());
        assertEquals(0L, first.ackedThroughSequence());

        Map<String, Object> pending = pg.jdbc().queryForMap("""
                SELECT stream_id, acked_through_sequence, pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_batch_id, pending_correlation_id, pending_publisher_owner
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(streamId, pending.get("stream_id"));
        assertEquals(0L, ((Number) pending.get("acked_through_sequence")).longValue());
        assertEquals(1L, ((Number) pending.get("pending_first_sequence")).longValue());
        assertEquals(10L, ((Number) pending.get("pending_last_sequence")).longValue());
        assertEquals("checksum-a", pending.get("pending_checksum"));
        assertEquals("batch-a", pending.get("pending_batch_id"));
        assertEquals("corr-a", pending.get("pending_correlation_id"));
        assertNull(pending.get("pending_publisher_owner"));

        ReplayReservation exactPendingRetry = store.reserveBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a", "corr-a");
        assertEquals(ReplayReservation.Status.RETRY_PENDING, exactPendingRetry.status());

        ReplayReservation conflictingPending = store.reserveBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-tampered", "batch-tampered", "corr-x");
        assertEquals(ReplayReservation.Status.REJECTED, conflictingPending.status());
        assertEquals("ERR_IDEMPOTENCY_CONFLICT", conflictingPending.rejectionCode());

        ReplayReservation outOfOrderWhilePending = store.reserveBatch(
                tenantId, sensorId, streamId, 11L, 20L, "checksum-b", "batch-b", "corr-b");
        assertEquals(ReplayReservation.Status.REJECTED, outOfOrderWhilePending.status());
        assertEquals("ERR_PREVIOUS_BATCH_PENDING", outOfOrderWhilePending.rejectionCode());

        PublisherClaimResult claim = claimExact(store,
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a", "corr-a");
        assertEquals(PublisherClaimResult.Status.CLAIMED, claim.status());

        PublisherClaimResult busy = claimExact(store,
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a", "corr-a");
        assertEquals(PublisherClaimResult.Status.BUSY, busy.status());

        store.finalizeReservedBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a", "corr-a",
                claim.ownerToken(), claim.publisherEpoch());

        Map<String, Object> finalized = pg.jdbc().queryForMap("""
                SELECT acked_through_sequence, settled_through_sequence, last_checksum, pending_first_sequence,
                       pending_batch_id, pending_correlation_id, pending_publisher_owner
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(10L, ((Number) finalized.get("acked_through_sequence")).longValue());
        assertEquals(10L, ((Number) finalized.get("settled_through_sequence")).longValue());
        assertEquals("checksum-a", finalized.get("last_checksum"));
        assertNull(finalized.get("pending_first_sequence"));
        assertNull(finalized.get("pending_batch_id"));
        assertNull(finalized.get("pending_correlation_id"));
        assertNull(finalized.get("pending_publisher_owner"));

        ReplayReservation alreadyAcked = store.reserveBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a", "corr-replay");
        assertEquals(ReplayReservation.Status.ALREADY_ACKED, alreadyAcked.status());
        assertEquals(10L, alreadyAcked.ackedThroughSequence());

        ReplayReservation changedBatchIdentity = store.reserveBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a-replay", "corr-replay");
        assertEquals(ReplayReservation.Status.REJECTED, changedBatchIdentity.status());
        assertEquals("ERR_IDEMPOTENCY_CONFLICT", changedBatchIdentity.rejectionCode());

        ReplayReservation changedAcceptedChecksum = store.reserveBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-changed", "batch-a", "corr-replay");
        assertEquals(ReplayReservation.Status.REJECTED, changedAcceptedChecksum.status());
        assertEquals("ERR_REPLAY_CONFLICT", changedAcceptedChecksum.rejectionCode());

        assertEquals(ReplayReservation.Status.RESERVED, store.reserveBatch(
                tenantId, sensorId, streamId, 11L, 11L, "checksum-next", "batch-next", "corr-next").status());
        assertEquals(ReplayReservation.Status.ALREADY_ACKED, store.reserveBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a", "corr-replay").status());
    }

    @Test
    void independentFromTelemetryWatermarkTable() {
        String sensorId = "sensor-shared-01";
        assertEquals(ReplayReservation.Status.RESERVED,
                store.reserveBatch(tenantId, sensorId, streamId, 1L, 50L, "agent-chk", "agent-batch", "corr-1").status());
        claimAndFinalize(sensorId, 1L, 50L, "agent-chk", "agent-batch", "corr-1");

        Integer telemetryRows = pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_telemetry_watermark WHERE tenant_id=? AND sensor_id=?",
                Integer.class, tenantId, sensorId
        );
        assertEquals(0, telemetryRows);

        Integer agentRows = pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_agent_ingress_watermark WHERE tenant_id=? AND sensor_id=?",
                Integer.class, tenantId, sensorId
        );
        assertEquals(1, agentRows);
    }

    @Test
    void reservationRejectsRegressionGapAndCrossTenantInterference() {
        String sensorId = "sensor-agent-shared";
        UUID tenantId2 = UUID.fromString("018f0e0a-0000-7000-8000-000000000002");
        pg.jdbc().update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?, 'Tenant 2', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                ON CONFLICT (tenant_id) DO NOTHING
                """, tenantId2);

        assertEquals(ReplayReservation.Status.RESERVED,
                store.reserveBatch(tenantId, sensorId, streamId, 1L, 5L, "t1-a", "t1-a", "c1").status());
        claimAndFinalize(sensorId, 1L, 5L, "t1-a", "t1-a", "c1");

        ReplayReservation regression = store.reserveBatch(
                tenantId, sensorId, streamId, 4L, 4L, "t1-old", "t1-old", "c-old");
        assertEquals(ReplayReservation.Status.REJECTED, regression.status());
        assertEquals("ERR_SEQUENCE_REGRESSION", regression.rejectionCode());

        ReplayReservation gap = store.reserveBatch(
                tenantId, sensorId, streamId, 7L, 7L, "t1-gap", "t1-gap", "c-gap");
        assertEquals(ReplayReservation.Status.REJECTED, gap.status());
        assertEquals("ERR_SEQUENCE_GAP", gap.rejectionCode());

        ReplayReservation changedStream = store.reserveBatch(
                tenantId, sensorId, "stream-ffeeddccbbaa99887766554433221100",
                6L, 6L, "new-stream", "new-stream", "c-new");
        assertEquals(ReplayReservation.Status.REJECTED, changedStream.status());
        assertEquals("ERR_SENSOR_STREAM_CHANGED", changedStream.rejectionCode());

        ReplayReservation otherTenant = store.reserveBatch(
                tenantId2, sensorId, streamId, 1L, 1L, "t2-a", "t2-a", "c2");
        assertEquals(ReplayReservation.Status.RESERVED, otherTenant.status());
        assertEquals(0L, otherTenant.ackedThroughSequence());
    }

    @Test
    void databaseRejectsPartialPendingReservationState() {
        String sensorId = "sensor-agent-partial-pending";
        assertThrows(org.springframework.dao.DataIntegrityViolationException.class, () ->
                pg.jdbc().update("""
                        INSERT INTO platform.sensor_agent_ingress_watermark (
                            tenant_id, sensor_id, stream_id, acked_through_sequence,
                            settled_through_sequence,
                            last_first_sequence, last_last_sequence, last_checksum,
                            pending_first_sequence, pending_last_sequence,
                            pending_checksum, pending_reserved_at
                        ) VALUES (?, ?, ?, 0, 0, 0, 0, '', 1, 1, 'checksum', CURRENT_TIMESTAMP)
                        """, tenantId, sensorId, streamId)
        );

        Integer rows = pg.jdbc().queryForObject(
                "SELECT count(*) FROM platform.sensor_agent_ingress_watermark WHERE tenant_id=? AND sensor_id=?",
                Integer.class, tenantId, sensorId
        );
        assertEquals(0, rows);
    }

    @Test
    void concurrentExactReservationSerializesToOneOwnerAndSafeRetries() throws Exception {
        String sensorId = "sensor-concurrent-agent-01";
        int workers = 8;
        ExecutorService pool = Executors.newFixedThreadPool(workers);
        CountDownLatch startLatch = new CountDownLatch(1);
        CountDownLatch doneLatch = new CountDownLatch(workers);
        AtomicInteger reserved = new AtomicInteger();
        AtomicInteger retryPending = new AtomicInteger();
        AtomicInteger unexpected = new AtomicInteger();

        for (int i = 0; i < workers; i++) {
            pool.submit(() -> {
                try {
                    startLatch.await();
                    ReplayReservation result = store.reserveBatch(
                            tenantId, sensorId, streamId,
                            1L, 1L, "same-checksum", "same-batch", "same-corr");
                    if (result.status() == ReplayReservation.Status.RESERVED) {
                        reserved.incrementAndGet();
                    } else if (result.status() == ReplayReservation.Status.RETRY_PENDING) {
                        retryPending.incrementAndGet();
                    } else {
                        unexpected.incrementAndGet();
                    }
                } catch (Exception e) {
                    unexpected.incrementAndGet();
                } finally {
                    doneLatch.countDown();
                }
            });
        }

        startLatch.countDown();
        assertTrue(doneLatch.await(30, TimeUnit.SECONDS));
        pool.shutdown();

        assertEquals(1, reserved.get(), "exactly one transaction must create the pending reservation");
        assertEquals(workers - 1, retryPending.get(), "concurrent exact retries must observe the same reservation");
        assertEquals(0, unexpected.get());

        claimAndFinalize(sensorId, 1L, 1L, "same-checksum", "same-batch", "same-corr");

        Map<String, Object> state = pg.jdbc().queryForMap("""
                SELECT acked_through_sequence, pending_first_sequence, pending_batch_id,
                       pending_correlation_id, pending_publisher_owner
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(1L, ((Number) state.get("acked_through_sequence")).longValue());
        assertNull(state.get("pending_first_sequence"));
        assertNull(state.get("pending_batch_id"));
        assertNull(state.get("pending_correlation_id"));
        assertNull(state.get("pending_publisher_owner"));
    }

    @Test
    void expiredPublisherLeaseCannotFinalizeAndExactRetryCanTakeover() {
        String sensorId = "sensor-agent-lease-expire-01";
        assertEquals(ReplayReservation.Status.RESERVED,
                store.reserveBatch(tenantId, sensorId, streamId, 1L, 5L, "chk-exp", "batch-exp", "corr-exp").status());

        PublisherClaimResult claim = claimExact(store,
                tenantId, sensorId, streamId, 1L, 5L, "chk-exp", "batch-exp", "corr-exp");
        assertEquals(PublisherClaimResult.Status.CLAIMED, claim.status());

        int expired = pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_publisher_claimed_at = CURRENT_TIMESTAMP - INTERVAL '2 seconds',
                       pending_publisher_lease_until = CURRENT_TIMESTAMP - INTERVAL '1 second'
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(1, expired);

        IllegalStateException finalizeEx = assertThrows(IllegalStateException.class, () ->
                store.finalizeReservedBatch(
                        tenantId, sensorId, streamId, 1L, 5L, "chk-exp", "batch-exp", "corr-exp",
                        claim.ownerToken(), claim.publisherEpoch()));
        assertTrue(finalizeEx.getMessage().contains("lease expired")
                || finalizeEx.getMessage().contains("ownership changed"));

        Map<String, Object> pending = pg.jdbc().queryForMap("""
                SELECT acked_through_sequence, pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_batch_id, pending_correlation_id,
                       pending_publisher_owner, pending_publisher_epoch
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(0L, ((Number) pending.get("acked_through_sequence")).longValue());
        assertEquals(1L, ((Number) pending.get("pending_first_sequence")).longValue());
        assertEquals(5L, ((Number) pending.get("pending_last_sequence")).longValue());
        assertEquals("chk-exp", pending.get("pending_checksum"));
        assertEquals("batch-exp", pending.get("pending_batch_id"));
        assertEquals("corr-exp", pending.get("pending_correlation_id"));
        assertEquals(claim.ownerToken(), pending.get("pending_publisher_owner"));
        assertEquals(claim.publisherEpoch(), ((Number) pending.get("pending_publisher_epoch")).longValue());

        PublisherClaimResult takeover = claimExact(store,
                tenantId, sensorId, streamId, 1L, 5L, "chk-exp", "batch-exp", "corr-exp");
        assertEquals(PublisherClaimResult.Status.CLAIMED, takeover.status());
        assertEquals(claim.publisherEpoch() + 1, takeover.publisherEpoch());

        assertDoesNotThrow(() -> store.finalizeReservedBatch(
                tenantId, sensorId, streamId, 1L, 5L, "chk-exp", "batch-exp", "corr-exp",
                takeover.ownerToken(), takeover.publisherEpoch()));
        assertEquals(5L, pg.jdbc().queryForObject(
                "SELECT acked_through_sequence FROM platform.sensor_agent_ingress_watermark WHERE tenant_id=? AND sensor_id=?",
                Long.class, tenantId, sensorId));
    }

    @Test
    void staleOwnerAfterTakeoverCannotFinalizeOrReleaseNewerLease() {
        String sensorId = "sensor-agent-lease-stale-01";
        assertEquals(ReplayReservation.Status.RESERVED,
                store.reserveBatch(tenantId, sensorId, streamId, 1L, 1L, "chk-stale", "batch-stale", "corr-stale").status());

        PublisherClaimResult oldClaim = claimExact(store,
                tenantId, sensorId, streamId, 1L, 1L, "chk-stale", "batch-stale", "corr-stale");
        assertEquals(PublisherClaimResult.Status.CLAIMED, oldClaim.status());

        pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_publisher_claimed_at = CURRENT_TIMESTAMP - INTERVAL '2 seconds',
                       pending_publisher_lease_until = CURRENT_TIMESTAMP - INTERVAL '1 second'
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);

        PublisherClaimResult newClaim = claimExact(store,
                tenantId, sensorId, streamId, 1L, 1L, "chk-stale", "batch-stale", "corr-stale");
        assertEquals(PublisherClaimResult.Status.CLAIMED, newClaim.status());

        assertThrows(IllegalStateException.class, () ->
                store.finalizeReservedBatch(
                        tenantId, sensorId, streamId, 1L, 1L, "chk-stale", "batch-stale", "corr-stale",
                        oldClaim.ownerToken(), oldClaim.publisherEpoch()));

        store.releasePublisherLease(
                tenantId, sensorId, streamId, 1L, 1L, "chk-stale", "batch-stale", "corr-stale",
                oldClaim.ownerToken(), oldClaim.publisherEpoch());

        Map<String, Object> owned = pg.jdbc().queryForMap("""
                SELECT pending_publisher_owner, pending_publisher_epoch, pending_first_sequence
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(newClaim.ownerToken(), owned.get("pending_publisher_owner"));
        assertEquals(newClaim.publisherEpoch(), ((Number) owned.get("pending_publisher_epoch")).longValue());
        assertEquals(1L, ((Number) owned.get("pending_first_sequence")).longValue());

        assertDoesNotThrow(() -> store.finalizeReservedBatch(
                tenantId, sensorId, streamId, 1L, 1L, "chk-stale", "batch-stale", "corr-stale",
                newClaim.ownerToken(), newClaim.publisherEpoch()));
    }

    @Test
    void finalizeFailsWhenLockDelayPushesPastLeaseUsingWallClock() throws Exception {
        String sensorId = "sensor-agent-lease-lock-delay-01";
        assertEquals(ReplayReservation.Status.RESERVED,
                store.reserveBatch(tenantId, sensorId, streamId, 1L, 3L, "chk-ld", "batch-ld", "corr-ld").status());
        PublisherClaimResult claim = claimExact(store,
                tenantId, sensorId, streamId, 1L, 3L, "chk-ld", "batch-ld", "corr-ld");
        assertEquals(PublisherClaimResult.Status.CLAIMED, claim.status());

        assertEquals(1, pg.jdbc().update("""
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_publisher_claimed_at = clock_timestamp(),
                       pending_publisher_lease_until = clock_timestamp() + INTERVAL '250 milliseconds'
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId));

        CountDownLatch holderReady = new CountDownLatch(1);
        CountDownLatch releaseHold = new CountDownLatch(1);
        CountDownLatch finalizeStarted = new CountDownLatch(1);
        AtomicInteger finalizeOutcome = new AtomicInteger(0);
        ExecutorService pool = Executors.newFixedThreadPool(2);
        try {
            pool.submit(() -> {
                try (var conn = pg.dataSource().getConnection()) {
                    conn.setAutoCommit(false);
                    try (var ps = conn.prepareStatement(
                            "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))")) {
                        ps.setString(1, "sensor_agent_ingress_watermark:" + tenantId + ":" + sensorId);
                        ps.execute();
                    }
                    holderReady.countDown();
                    assertTrue(releaseHold.await(5, TimeUnit.SECONDS));
                    Boolean expired;
                    do {
                        expired = pg.jdbc().queryForObject("""
                                SELECT pending_publisher_lease_until <= clock_timestamp()
                                  FROM platform.sensor_agent_ingress_watermark
                                 WHERE tenant_id=? AND sensor_id=?
                                """, Boolean.class, tenantId, sensorId);
                        Thread.sleep(20);
                    } while (!Boolean.TRUE.equals(expired));
                    conn.commit();
                } catch (Exception e) {
                    throw new RuntimeException(e);
                }
            });

            assertTrue(holderReady.await(5, TimeUnit.SECONDS));
            pool.submit(() -> {
                finalizeStarted.countDown();
                try {
                    store.finalizeReservedBatch(
                            tenantId, sensorId, streamId, 1L, 3L, "chk-ld", "batch-ld", "corr-ld",
                            claim.ownerToken(), claim.publisherEpoch());
                    finalizeOutcome.set(1);
                } catch (IllegalStateException ex) {
                    assertTrue(ex.getMessage().contains("lease expired")
                            || ex.getMessage().contains("ownership changed"));
                    finalizeOutcome.set(-1);
                }
            });

            assertTrue(finalizeStarted.await(5, TimeUnit.SECONDS));
            Thread.sleep(50);
            releaseHold.countDown();

            pool.shutdown();
            assertTrue(pool.awaitTermination(15, TimeUnit.SECONDS));
            assertEquals(-1, finalizeOutcome.get(), "finalizer must fail after wall-clock lease expiry");

            Map<String, Object> pending = pg.jdbc().queryForMap("""
                    SELECT acked_through_sequence, pending_first_sequence, pending_last_sequence,
                           pending_batch_id, pending_publisher_owner
                      FROM platform.sensor_agent_ingress_watermark
                     WHERE tenant_id=? AND sensor_id=?
                    """, tenantId, sensorId);
            assertEquals(0L, ((Number) pending.get("acked_through_sequence")).longValue());
            assertEquals(1L, ((Number) pending.get("pending_first_sequence")).longValue());
            assertEquals(3L, ((Number) pending.get("pending_last_sequence")).longValue());
            assertEquals("batch-ld", pending.get("pending_batch_id"));
            assertEquals(claim.ownerToken(), pending.get("pending_publisher_owner"));

            PublisherClaimResult takeover = claimExact(store,
                    tenantId, sensorId, streamId, 1L, 3L, "chk-ld", "batch-ld", "corr-ld");
            assertEquals(PublisherClaimResult.Status.CLAIMED, takeover.status());
            assertDoesNotThrow(() -> store.finalizeReservedBatch(
                    tenantId, sensorId, streamId, 1L, 3L, "chk-ld", "batch-ld", "corr-ld",
                    takeover.ownerToken(), takeover.publisherEpoch()));
        } finally {
            pool.shutdownNow();
        }
    }
}
