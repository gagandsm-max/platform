package com.platform.core.ingestion.agentingress;

import com.platform.core.ingestion.replay.PublisherClaimResult;
import com.platform.core.ingestion.replay.ReplayReservation;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;

import java.util.Map;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;

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
        PublisherClaimResult claim = store.claimPublisherLease(
                tenantId, sensorId, streamId, first, last, checksum, batchId, correlationId);
        assertEquals(PublisherClaimResult.Status.CLAIMED, claim.status());
        store.finalizeReservedBatch(
                tenantId, sensorId, streamId, first, last, checksum, batchId, correlationId,
                claim.ownerToken(), claim.publisherEpoch());
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

        PublisherClaimResult claim = store.claimPublisherLease(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a", "corr-a");
        assertEquals(PublisherClaimResult.Status.CLAIMED, claim.status());

        PublisherClaimResult busy = store.claimPublisherLease(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a", "corr-a");
        assertEquals(PublisherClaimResult.Status.BUSY, busy.status());

        store.finalizeReservedBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a", "corr-a",
                claim.ownerToken(), claim.publisherEpoch());

        Map<String, Object> finalized = pg.jdbc().queryForMap("""
                SELECT acked_through_sequence, last_checksum, pending_first_sequence,
                       pending_batch_id, pending_correlation_id, pending_publisher_owner
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(10L, ((Number) finalized.get("acked_through_sequence")).longValue());
        assertEquals("checksum-a", finalized.get("last_checksum"));
        assertNull(finalized.get("pending_first_sequence"));
        assertNull(finalized.get("pending_batch_id"));
        assertNull(finalized.get("pending_correlation_id"));
        assertNull(finalized.get("pending_publisher_owner"));

        ReplayReservation alreadyAcked = store.reserveBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a-replay", "corr-replay");
        assertEquals(ReplayReservation.Status.ALREADY_ACKED, alreadyAcked.status());
        assertEquals(10L, alreadyAcked.ackedThroughSequence());
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
                            last_first_sequence, last_last_sequence, last_checksum,
                            pending_first_sequence, pending_last_sequence,
                            pending_checksum, pending_reserved_at
                        ) VALUES (?, ?, ?, 0, 0, 0, '', 1, 1, 'checksum', CURRENT_TIMESTAMP)
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
}
