package com.platform.core.ingestion.replay;

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
class PostgresTelemetryWatermarkStoreTest {

    private PostgresTestSupport pg;
    private PostgresTelemetryWatermarkStore store;
    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private final String streamId = "stream-00112233445566778899aabbccddeeff";

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        store = new PostgresTelemetryWatermarkStore(pg.jdbc(), new DataSourceTransactionManager(pg.dataSource()));

        // Ensure default tenant exists
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
            String sensorId, long first, long last, String checksum, String batchId
    ) {
        PublisherClaimResult claim = store.claimPublisherLease(
                tenantId, sensorId, streamId, first, last, checksum, batchId);
        assertEquals(PublisherClaimResult.Status.CLAIMED, claim.status());
        assertNotNull(claim.ownerToken());
        assertTrue(claim.publisherEpoch() > 0);
        store.finalizeReservedBatch(
                tenantId, sensorId, streamId, first, last, checksum, batchId,
                claim.ownerToken(), claim.publisherEpoch());
    }

    @Test
    void invalidStreamAndHiddenInitialGapAreRejectedWithoutCreatingState() {
        String sensorId = "sensor-new-stream";

        ReplayReservation invalidStream = store.reserveBatch(
                tenantId, sensorId, "epoch-process-local",
                1L, 1L, "checksum-a", "batch-a");
        assertEquals(ReplayReservation.Status.REJECTED, invalidStream.status());
        assertEquals("ERR_TELEMETRY_STREAM_ID_INVALID", invalidStream.rejectionCode());

        ReplayReservation hiddenGap = store.reserveBatch(
                tenantId, sensorId, streamId,
                100L, 100L, "checksum-b", "batch-b");
        assertEquals(ReplayReservation.Status.REJECTED, hiddenGap.status());
        assertEquals("ERR_SEQUENCE_GAP", hiddenGap.rejectionCode());

        Integer rows = pg.jdbc().queryForObject(
                "SELECT count(*) FROM platform.sensor_telemetry_watermark WHERE tenant_id=? AND sensor_id=?",
                Integer.class,
                tenantId,
                sensorId
        );
        assertEquals(0, rows);
    }

    @Test
    void reserveClaimFinalizeCycleIsDurableBoundedAndIdempotent() {
        String sensorId = "sensor-reservation-01";

        ReplayReservation first = store.reserveBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a");
        assertEquals(ReplayReservation.Status.RESERVED, first.status());
        assertEquals(0L, first.ackedThroughSequence());

        Map<String, Object> pending = pg.jdbc().queryForMap("""
                SELECT stream_id, acked_through_sequence, pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_batch_id, pending_publisher_owner, pending_publisher_epoch
                  FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(streamId, pending.get("stream_id"));
        assertEquals(0L, ((Number) pending.get("acked_through_sequence")).longValue());
        assertEquals(1L, ((Number) pending.get("pending_first_sequence")).longValue());
        assertEquals(10L, ((Number) pending.get("pending_last_sequence")).longValue());
        assertEquals("checksum-a", pending.get("pending_checksum"));
        assertEquals("batch-a", pending.get("pending_batch_id"));
        assertNull(pending.get("pending_publisher_owner"));
        assertEquals(0L, ((Number) pending.get("pending_publisher_epoch")).longValue());

        ReplayReservation exactPendingRetry = store.reserveBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a");
        assertEquals(ReplayReservation.Status.RETRY_PENDING, exactPendingRetry.status());

        ReplayReservation conflictingPending = store.reserveBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-tampered", "batch-tampered");
        assertEquals(ReplayReservation.Status.REJECTED, conflictingPending.status());
        assertEquals("ERR_IDEMPOTENCY_CONFLICT", conflictingPending.rejectionCode());

        ReplayReservation outOfOrderWhilePending = store.reserveBatch(
                tenantId, sensorId, streamId, 11L, 20L, "checksum-b", "batch-b");
        assertEquals(ReplayReservation.Status.REJECTED, outOfOrderWhilePending.status());
        assertEquals("ERR_PREVIOUS_BATCH_PENDING", outOfOrderWhilePending.rejectionCode());

        PublisherClaimResult claim = store.claimPublisherLease(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a");
        assertEquals(PublisherClaimResult.Status.CLAIMED, claim.status());

        PublisherClaimResult busy = store.claimPublisherLease(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a");
        assertEquals(PublisherClaimResult.Status.BUSY, busy.status());
        assertEquals("ERR_BATCH_PUBLICATION_IN_PROGRESS", busy.rejectionCode());

        store.finalizeReservedBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a",
                claim.ownerToken(), claim.publisherEpoch());

        Map<String, Object> finalized = pg.jdbc().queryForMap("""
                SELECT acked_through_sequence, last_first_sequence, last_last_sequence,
                       last_checksum, pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_batch_id, pending_reserved_at,
                       pending_publisher_owner, pending_publisher_lease_until
                  FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(10L, ((Number) finalized.get("acked_through_sequence")).longValue());
        assertEquals(1L, ((Number) finalized.get("last_first_sequence")).longValue());
        assertEquals(10L, ((Number) finalized.get("last_last_sequence")).longValue());
        assertEquals("checksum-a", finalized.get("last_checksum"));
        assertNull(finalized.get("pending_first_sequence"));
        assertNull(finalized.get("pending_last_sequence"));
        assertNull(finalized.get("pending_checksum"));
        assertNull(finalized.get("pending_batch_id"));
        assertNull(finalized.get("pending_reserved_at"));
        assertNull(finalized.get("pending_publisher_owner"));
        assertNull(finalized.get("pending_publisher_lease_until"));

        ReplayReservation alreadyAcked = store.reserveBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a-replay");
        assertEquals(ReplayReservation.Status.ALREADY_ACKED, alreadyAcked.status());
        assertEquals(10L, alreadyAcked.ackedThroughSequence());

        // Finalize is itself idempotent for the exact accepted batch (no pending lease required).
        assertDoesNotThrow(() -> store.finalizeReservedBatch(
                tenantId, sensorId, streamId, 1L, 10L, "checksum-a", "batch-a-replay",
                UUID.randomUUID(), 1L));
    }

    @Test
    void releasePublisherLeaseKeepsPendingAndAllowsReclaim() {
        String sensorId = "sensor-lease-release-01";
        assertEquals(ReplayReservation.Status.RESERVED,
                store.reserveBatch(tenantId, sensorId, streamId, 1L, 1L, "chk", "batch-r").status());

        PublisherClaimResult claim = store.claimPublisherLease(
                tenantId, sensorId, streamId, 1L, 1L, "chk", "batch-r");
        assertEquals(PublisherClaimResult.Status.CLAIMED, claim.status());

        store.releasePublisherLease(
                tenantId, sensorId, streamId, 1L, 1L, "chk", "batch-r",
                claim.ownerToken(), claim.publisherEpoch());

        Map<String, Object> pending = pg.jdbc().queryForMap("""
                SELECT pending_first_sequence, pending_publisher_owner, pending_publisher_lease_until,
                       pending_publisher_epoch
                  FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(1L, ((Number) pending.get("pending_first_sequence")).longValue());
        assertNull(pending.get("pending_publisher_owner"));
        assertNull(pending.get("pending_publisher_lease_until"));
        assertEquals(claim.publisherEpoch(), ((Number) pending.get("pending_publisher_epoch")).longValue());

        PublisherClaimResult reclaim = store.claimPublisherLease(
                tenantId, sensorId, streamId, 1L, 1L, "chk", "batch-r");
        assertEquals(PublisherClaimResult.Status.CLAIMED, reclaim.status());
        assertEquals(claim.publisherEpoch() + 1, reclaim.publisherEpoch());
    }

    @Test
    void reservationRejectsRegressionGapAndCrossTenantInterference() {
        String sensorId = "sensor-reservation-shared";
        UUID tenantId2 = UUID.fromString("018f0e0a-0000-7000-8000-000000000002");
        pg.jdbc().update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?, 'Tenant 2', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                ON CONFLICT (tenant_id) DO NOTHING
                """, tenantId2);

        assertEquals(ReplayReservation.Status.RESERVED,
                store.reserveBatch(tenantId, sensorId, streamId, 1L, 5L, "t1-a", "t1-a").status());
        claimAndFinalize(sensorId, 1L, 5L, "t1-a", "t1-a");

        ReplayReservation regression = store.reserveBatch(
                tenantId, sensorId, streamId, 4L, 4L, "t1-old", "t1-old");
        assertEquals(ReplayReservation.Status.REJECTED, regression.status());
        assertEquals("ERR_SEQUENCE_REGRESSION", regression.rejectionCode());

        ReplayReservation gap = store.reserveBatch(
                tenantId, sensorId, streamId, 7L, 7L, "t1-gap", "t1-gap");
        assertEquals(ReplayReservation.Status.REJECTED, gap.status());
        assertEquals("ERR_SEQUENCE_GAP", gap.rejectionCode());

        ReplayReservation changedStream = store.reserveBatch(
                tenantId, sensorId, "stream-ffeeddccbbaa99887766554433221100",
                6L, 6L, "new-stream", "new-stream");
        assertEquals(ReplayReservation.Status.REJECTED, changedStream.status());
        assertEquals("ERR_SENSOR_STREAM_CHANGED", changedStream.rejectionCode());

        // Same sensor identifier in another tenant is an independent replay domain.
        ReplayReservation otherTenant = store.reserveBatch(
                tenantId2, sensorId, streamId, 1L, 1L, "t2-a", "t2-a");
        assertEquals(ReplayReservation.Status.RESERVED, otherTenant.status());
        assertEquals(0L, otherTenant.ackedThroughSequence());
    }

    @Test
    void legacyUnboundWatermarkBindsOnlyAfterValidContinuation() {
        String sensorId = "sensor-legacy-upgrade";
        pg.jdbc().update("""
                INSERT INTO platform.sensor_telemetry_watermark (
                    tenant_id, sensor_id, acked_through_sequence,
                    last_first_sequence, last_last_sequence, last_checksum,
                    last_batch_id, updated_at
                ) VALUES (?, ?, 5, 1, 5, 'legacy-checksum', 'legacy-batch', CURRENT_TIMESTAMP)
                """, tenantId, sensorId);

        ReplayReservation invalidGap = store.reserveBatch(
                tenantId, sensorId, streamId,
                7L, 7L, "gap-checksum", "gap-batch");
        assertEquals(ReplayReservation.Status.REJECTED, invalidGap.status());
        assertEquals("ERR_SEQUENCE_GAP", invalidGap.rejectionCode());
        assertNull(pg.jdbc().queryForObject(
                "SELECT stream_id FROM platform.sensor_telemetry_watermark WHERE tenant_id=? AND sensor_id=?",
                String.class,
                tenantId,
                sensorId
        ), "invalid continuation must not bind the legacy row to a stream");

        ReplayReservation validNext = store.reserveBatch(
                tenantId, sensorId, streamId,
                6L, 6L, "next-checksum", "next-batch");
        assertEquals(ReplayReservation.Status.RESERVED, validNext.status());
        assertEquals(streamId, pg.jdbc().queryForObject(
                "SELECT stream_id FROM platform.sensor_telemetry_watermark WHERE tenant_id=? AND sensor_id=?",
                String.class,
                tenantId,
                sensorId
        ));
    }

    @Test
    void databaseRejectsPartialPendingReservationState() {
        String sensorId = "sensor-partial-pending";
        assertThrows(org.springframework.dao.DataIntegrityViolationException.class, () ->
                pg.jdbc().update("""
                        INSERT INTO platform.sensor_telemetry_watermark (
                            tenant_id, sensor_id, stream_id, acked_through_sequence,
                            last_first_sequence, last_last_sequence, last_checksum,
                            pending_first_sequence, pending_last_sequence,
                            pending_checksum, pending_reserved_at
                        ) VALUES (?, ?, ?, 0, 0, 0, '', 1, 1, 'checksum', CURRENT_TIMESTAMP)
                        """, tenantId, sensorId, streamId)
        );

        Integer rows = pg.jdbc().queryForObject(
                "SELECT count(*) FROM platform.sensor_telemetry_watermark WHERE tenant_id=? AND sensor_id=?",
                Integer.class,
                tenantId,
                sensorId
        );
        assertEquals(0, rows);
    }

    @Test
    void concurrentExactReservationSerializesToOneOwnerAndSafeRetries() throws Exception {
        String sensorId = "sensor-concurrent-01";
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
                            1L, 1L, "same-checksum", "same-batch");
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

        claimAndFinalize(sensorId, 1L, 1L, "same-checksum", "same-batch");

        Map<String, Object> state = pg.jdbc().queryForMap("""
                SELECT acked_through_sequence, pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_batch_id, pending_reserved_at
                  FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id=? AND sensor_id=?
                """, tenantId, sensorId);
        assertEquals(1L, ((Number) state.get("acked_through_sequence")).longValue());
        assertNull(state.get("pending_first_sequence"));
        assertNull(state.get("pending_last_sequence"));
        assertNull(state.get("pending_checksum"));
        assertNull(state.get("pending_batch_id"));
        assertNull(state.get("pending_reserved_at"));
    }

}
