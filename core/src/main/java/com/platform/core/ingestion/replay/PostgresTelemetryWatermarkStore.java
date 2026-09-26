package com.platform.core.ingestion.replay;

import com.platform.core.ingestion.TransportSequenceLimits;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Repository;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.support.TransactionTemplate;

import java.sql.PreparedStatement;
import java.sql.Timestamp;
import java.time.Instant;
import java.util.List;
import java.util.UUID;
import java.util.regex.Pattern;

@Repository
public class PostgresTelemetryWatermarkStore implements TelemetryWatermarkStore {

    private static final Pattern DURABLE_STREAM_ID = Pattern.compile("^stream-[0-9a-f]{32}$");

    private final JdbcTemplate jdbcTemplate;
    private final TransactionTemplate transactionTemplate;

    public PostgresTelemetryWatermarkStore(
            JdbcTemplate jdbcTemplate,
            @Qualifier("transactionManager") PlatformTransactionManager transactionManager
    ) {
        this.jdbcTemplate = jdbcTemplate;
        this.transactionTemplate = new TransactionTemplate(transactionManager);
        this.transactionTemplate.setPropagationBehavior(
                org.springframework.transaction.TransactionDefinition.PROPAGATION_REQUIRES_NEW
        );
    }

    @Override
    public ReplayReservation reserveBatch(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId
    ) {
        if (streamId == null || !DURABLE_STREAM_ID.matcher(streamId).matches()) {
            return ReplayReservation.rejected(
                    0,
                    "ERR_TELEMETRY_STREAM_ID_INVALID",
                    "Telemetry stream identity is not a durable Sensor spool generation"
            );
        }
        if (!TransportSequenceLimits.isValidWireSequence(firstSeq)
                || !TransportSequenceLimits.isValidWireSequence(lastSeq)
                || lastSeq < firstSeq) {
            return ReplayReservation.rejected(
                    0,
                    "ERR_SEQUENCE_OUT_OF_BOUNDS",
                    "Telemetry sequence range is outside MAX_SAFE_SEQUENCE bounds"
            );
        }
        ReplayReservation result = transactionTemplate.execute(status ->
                reserveBatchInTransaction(tenantId, sensorId, streamId, firstSeq, lastSeq, checksum, batchId));
        if (result == null) {
            throw new IllegalStateException("Telemetry replay reservation transaction returned no result");
        }
        return result;
    }

    private ReplayReservation reserveBatchInTransaction(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId
    ) {
        acquireSensorLock(tenantId, sensorId);

        List<ReplayRow> rows = jdbcTemplate.query(
                """
                SELECT stream_id, acked_through_sequence, last_first_sequence, last_last_sequence,
                       last_checksum, pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_batch_id
                  FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id = ? AND sensor_id = ?
                 FOR UPDATE
                """,
                (rs, rowNum) -> new ReplayRow(
                        rs.getString("stream_id"),
                        rs.getLong("acked_through_sequence"),
                        rs.getLong("last_first_sequence"),
                        rs.getLong("last_last_sequence"),
                        rs.getString("last_checksum"),
                        (Long) rs.getObject("pending_first_sequence"),
                        (Long) rs.getObject("pending_last_sequence"),
                        rs.getString("pending_checksum"),
                        rs.getString("pending_batch_id")
                ),
                tenantId,
                sensorId
        );

        if (rows.isEmpty()) {
            if (firstSeq != 1) {
                return ReplayReservation.rejected(
                        0,
                        "ERR_SEQUENCE_GAP",
                        "New telemetry stream must begin at sequence 1; received firstSequence " + firstSeq
                );
            }
            jdbcTemplate.update(
                    """
                    INSERT INTO platform.sensor_telemetry_watermark (
                        tenant_id, sensor_id, stream_id, acked_through_sequence,
                        last_first_sequence, last_last_sequence, last_checksum,
                        last_batch_id, updated_at,
                        pending_first_sequence, pending_last_sequence,
                        pending_checksum, pending_batch_id, pending_reserved_at
                    ) VALUES (?, ?, ?, 0, 0, 0, '', NULL, CURRENT_TIMESTAMP, ?, ?, ?, ?, CURRENT_TIMESTAMP)
                    """,
                    tenantId, sensorId, streamId, firstSeq, lastSeq, checksum, batchId
            );
            return ReplayReservation.reserved(0);
        }

        ReplayRow row = rows.get(0);

        boolean legacyUnboundStream = row.streamId() == null;
        if (!legacyUnboundStream && !row.streamId().equals(streamId)) {
            return ReplayReservation.rejected(
                    row.ackedThroughSequence(),
                    "ERR_SENSOR_STREAM_CHANGED",
                    "Sensor durable telemetry stream changed unexpectedly; explicit continuity reset/re-enrollment is required"
            );
        }

        if (row.pendingFirstSequence() != null) {
            if (legacyUnboundStream) {
                throw new IllegalStateException("Pending replay reservation has no durable stream binding");
            }
            boolean samePendingRange = row.pendingFirstSequence() == firstSeq
                    && row.pendingLastSequence() != null
                    && row.pendingLastSequence() == lastSeq;
            if (samePendingRange) {
                if (checksum.equalsIgnoreCase(row.pendingChecksum())
                        && batchIdEquals(row.pendingBatchId(), batchId)) {
                    return ReplayReservation.retryPending(row.ackedThroughSequence());
                }
                return ReplayReservation.rejected(
                        row.ackedThroughSequence(),
                        "ERR_IDEMPOTENCY_CONFLICT",
                        "Pending batch sequence range [" + firstSeq + ".." + lastSeq
                                + "] was reserved with a different payload checksum or batch identity"
                );
            }
            return ReplayReservation.rejected(
                    row.ackedThroughSequence(),
                    "ERR_PREVIOUS_BATCH_PENDING",
                    "A previous telemetry batch is still pending durable publication for this Sensor"
            );
        }

        if (firstSeq == row.lastFirstSequence() && lastSeq == row.lastLastSequence()) {
            if (checksum.equalsIgnoreCase(row.lastChecksum())) {
                bindStreamIfNeeded(tenantId, sensorId, streamId, legacyUnboundStream);
                return ReplayReservation.alreadyAcked(row.ackedThroughSequence());
            }
            return ReplayReservation.rejected(
                    row.ackedThroughSequence(),
                    "ERR_IDEMPOTENCY_CONFLICT",
                    "Batch sequence range [" + firstSeq + ".." + lastSeq
                            + "] was previously accepted with a different payload checksum"
            );
        }

        if (firstSeq <= row.ackedThroughSequence()) {
            return ReplayReservation.rejected(
                    row.ackedThroughSequence(),
                    "ERR_SEQUENCE_REGRESSION",
                    "Batch firstSequence " + firstSeq
                            + " has already been accepted (current watermark: "
                            + row.ackedThroughSequence() + ")"
            );
        }

        if (firstSeq > row.ackedThroughSequence() + 1) {
            return ReplayReservation.rejected(
                    row.ackedThroughSequence(),
                    "ERR_SEQUENCE_GAP",
                    "Batch firstSequence " + firstSeq
                            + " leaves an unexpected gap from watermark "
                            + row.ackedThroughSequence()
            );
        }

        bindStreamIfNeeded(tenantId, sensorId, streamId, legacyUnboundStream);

        int updated = jdbcTemplate.update(
                """
                UPDATE platform.sensor_telemetry_watermark
                   SET pending_first_sequence = ?,
                       pending_last_sequence = ?,
                       pending_checksum = ?,
                       pending_batch_id = ?,
                       pending_reserved_at = CURRENT_TIMESTAMP,
                       pending_publisher_owner = NULL,
                       pending_publisher_claimed_at = NULL,
                       pending_publisher_lease_until = NULL,
                       updated_at = CURRENT_TIMESTAMP
                 WHERE tenant_id = ? AND sensor_id = ?
                """,
                firstSeq, lastSeq, checksum, batchId, tenantId, sensorId
        );
        if (updated != 1) {
            throw new IllegalStateException("Failed to reserve telemetry replay watermark for Sensor");
        }
        return ReplayReservation.reserved(row.ackedThroughSequence());
    }

    @Override
    public PublisherClaimResult claimPublisherLease(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId
    ) {
        PublisherClaimResult result = transactionTemplate.execute(status ->
                claimPublisherLeaseInTransaction(tenantId, sensorId, streamId, firstSeq, lastSeq, checksum, batchId));
        if (result == null) {
            throw new IllegalStateException("Telemetry publisher claim transaction returned no result");
        }
        return result;
    }

    private PublisherClaimResult claimPublisherLeaseInTransaction(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId
    ) {
        acquireSensorLock(tenantId, sensorId);
        List<LeaseRow> rows = jdbcTemplate.query(
                """
                SELECT stream_id, pending_first_sequence, pending_last_sequence, pending_checksum, pending_batch_id,
                       pending_publisher_owner, pending_publisher_epoch, pending_publisher_lease_until
                  FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id = ? AND sensor_id = ?
                 FOR UPDATE
                """,
                (rs, rowNum) -> new LeaseRow(
                        rs.getString("stream_id"),
                        (Long) rs.getObject("pending_first_sequence"),
                        (Long) rs.getObject("pending_last_sequence"),
                        rs.getString("pending_checksum"),
                        rs.getString("pending_batch_id"),
                        (UUID) rs.getObject("pending_publisher_owner"),
                        rs.getLong("pending_publisher_epoch"),
                        rs.getTimestamp("pending_publisher_lease_until")
                ),
                tenantId,
                sensorId
        );
        if (rows.isEmpty()) {
            return PublisherClaimResult.rejected("ERR_PREVIOUS_BATCH_PENDING", "No telemetry watermark row for publisher claim");
        }
        LeaseRow row = rows.get(0);
        if (row.streamId() == null || !row.streamId().equals(streamId)) {
            return PublisherClaimResult.rejected("ERR_SENSOR_STREAM_CHANGED", "Telemetry stream mismatch during publisher claim");
        }
        if (!exactPending(row, firstSeq, lastSeq, checksum, batchId)) {
            return PublisherClaimResult.rejected(
                    "ERR_PREVIOUS_BATCH_PENDING",
                    "Publisher claim requires the exact pending telemetry reservation identity"
            );
        }
        Instant now = Instant.now();
        if (row.owner() != null && row.leaseUntil() != null && row.leaseUntil().toInstant().isAfter(now)) {
            return PublisherClaimResult.busy();
        }
        UUID owner = UUID.randomUUID();
        long epoch = row.publisherEpoch() + 1;
        Instant leaseUntil = now.plusSeconds(PUBLISHER_LEASE_SECONDS);
        int updated = jdbcTemplate.update(
                """
                UPDATE platform.sensor_telemetry_watermark
                   SET pending_publisher_owner = ?,
                       pending_publisher_epoch = ?,
                       pending_publisher_claimed_at = ?,
                       pending_publisher_lease_until = ?,
                       updated_at = CURRENT_TIMESTAMP
                 WHERE tenant_id = ? AND sensor_id = ?
                """,
                owner, epoch, Timestamp.from(now), Timestamp.from(leaseUntil), tenantId, sensorId
        );
        if (updated != 1) {
            throw new IllegalStateException("Failed to claim telemetry publisher lease");
        }
        return PublisherClaimResult.claimed(owner, epoch);
    }

    @Override
    public void releasePublisherLease(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId,
            UUID ownerToken,
            long publisherEpoch
    ) {
        transactionTemplate.executeWithoutResult(status -> {
            acquireSensorLock(tenantId, sensorId);
            jdbcTemplate.update(
                    """
                    UPDATE platform.sensor_telemetry_watermark
                       SET pending_publisher_owner = NULL,
                           pending_publisher_claimed_at = NULL,
                           pending_publisher_lease_until = NULL,
                           updated_at = CURRENT_TIMESTAMP
                     WHERE tenant_id = ?
                       AND sensor_id = ?
                       AND stream_id = ?
                       AND pending_first_sequence = ?
                       AND pending_last_sequence = ?
                       AND lower(pending_checksum) = lower(?)
                       AND pending_batch_id = ?
                       AND pending_publisher_owner = ?
                       AND pending_publisher_epoch = ?
                    """,
                    tenantId, sensorId, streamId, firstSeq, lastSeq, checksum, batchId, ownerToken, publisherEpoch
            );
        });
    }

    @Override
    public void finalizeReservedBatch(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId,
            UUID ownerToken,
            long publisherEpoch
    ) {
        transactionTemplate.executeWithoutResult(status ->
                finalizeReservedBatchInTransaction(
                        tenantId, sensorId, streamId, firstSeq, lastSeq, checksum, batchId, ownerToken, publisherEpoch));
    }

    private void finalizeReservedBatchInTransaction(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId,
            UUID ownerToken,
            long publisherEpoch
    ) {
        acquireSensorLock(tenantId, sensorId);

        List<FinalizeRow> rows = jdbcTemplate.query(
                """
                SELECT stream_id, acked_through_sequence, last_first_sequence, last_last_sequence,
                       last_checksum, pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_batch_id,
                       pending_publisher_owner, pending_publisher_epoch, pending_publisher_lease_until
                  FROM platform.sensor_telemetry_watermark
                 WHERE tenant_id = ? AND sensor_id = ?
                 FOR UPDATE
                """,
                (rs, rowNum) -> new FinalizeRow(
                        rs.getString("stream_id"),
                        rs.getLong("acked_through_sequence"),
                        rs.getLong("last_first_sequence"),
                        rs.getLong("last_last_sequence"),
                        rs.getString("last_checksum"),
                        (Long) rs.getObject("pending_first_sequence"),
                        (Long) rs.getObject("pending_last_sequence"),
                        rs.getString("pending_checksum"),
                        rs.getString("pending_batch_id"),
                        (UUID) rs.getObject("pending_publisher_owner"),
                        rs.getLong("pending_publisher_epoch"),
                        rs.getTimestamp("pending_publisher_lease_until")
                ),
                tenantId,
                sensorId
        );
        if (rows.isEmpty()) {
            throw new IllegalStateException("Telemetry replay reservation disappeared before finalize");
        }

        FinalizeRow row = rows.get(0);
        if (row.streamId() == null || !row.streamId().equals(streamId)) {
            throw new IllegalStateException("Telemetry stream identity changed before finalize");
        }
        if (row.pendingFirstSequence() == null) {
            if (row.lastFirstSequence() == firstSeq
                    && row.lastLastSequence() == lastSeq
                    && checksum.equalsIgnoreCase(row.lastChecksum())) {
                return;
            }
            throw new IllegalStateException("No matching pending telemetry batch exists for finalize");
        }

        if (row.pendingFirstSequence() != firstSeq
                || row.pendingLastSequence() == null
                || row.pendingLastSequence() != lastSeq
                || !checksum.equalsIgnoreCase(row.pendingChecksum())
                || !batchIdEquals(row.pendingBatchId(), batchId)) {
            throw new IllegalStateException("Pending telemetry batch changed before finalize");
        }
        if (ownerToken == null
                || row.owner() == null
                || !ownerToken.equals(row.owner())
                || row.publisherEpoch() != publisherEpoch) {
            throw new IllegalStateException("Stale or mismatched telemetry publisher cannot finalize");
        }
        if (row.leaseUntil() == null) {
            throw new IllegalStateException("Telemetry publisher lease missing; cannot finalize");
        }

        int updated = jdbcTemplate.update(
                """
                UPDATE platform.sensor_telemetry_watermark
                   SET acked_through_sequence = ?,
                       last_first_sequence = ?,
                       last_last_sequence = ?,
                       last_checksum = ?,
                       last_batch_id = ?,
                       pending_first_sequence = NULL,
                       pending_last_sequence = NULL,
                       pending_checksum = NULL,
                       pending_batch_id = NULL,
                       pending_reserved_at = NULL,
                       pending_publisher_owner = NULL,
                       pending_publisher_claimed_at = NULL,
                       pending_publisher_lease_until = NULL,
                       updated_at = CURRENT_TIMESTAMP
                 WHERE tenant_id = ? AND sensor_id = ?
                   AND pending_publisher_owner = ?
                   AND pending_publisher_epoch = ?
                   AND pending_publisher_lease_until IS NOT NULL
                   AND pending_publisher_lease_until > CURRENT_TIMESTAMP
                """,
                lastSeq, firstSeq, lastSeq, checksum, batchId,
                tenantId, sensorId, ownerToken, publisherEpoch
        );
        if (updated != 1) {
            throw new IllegalStateException(
                    "Telemetry publisher lease expired or ownership changed before finalize; pending reservation retained");
        }
    }

    private void bindStreamIfNeeded(
            UUID tenantId,
            String sensorId,
            String streamId,
            boolean legacyUnboundStream
    ) {
        if (!legacyUnboundStream) {
            return;
        }
        int bound = jdbcTemplate.update(
                """
                UPDATE platform.sensor_telemetry_watermark
                   SET stream_id = ?, updated_at = CURRENT_TIMESTAMP
                 WHERE tenant_id = ? AND sensor_id = ? AND stream_id IS NULL
                   AND pending_first_sequence IS NULL
                """,
                streamId, tenantId, sensorId
        );
        if (bound != 1) {
            throw new IllegalStateException("Failed to bind durable telemetry stream identity");
        }
    }

    private void acquireSensorLock(UUID tenantId, String sensorId) {
        jdbcTemplate.execute(
                "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
                (PreparedStatement ps) -> {
                    ps.setString(1, "sensor_telemetry_watermark:" + tenantId + ":" + sensorId);
                    ps.execute();
                    return null;
                }
        );
    }

    private static boolean exactPending(LeaseRow row, long firstSeq, long lastSeq, String checksum, String batchId) {
        return row.pendingFirstSequence() != null
                && row.pendingFirstSequence() == firstSeq
                && row.pendingLastSequence() != null
                && row.pendingLastSequence() == lastSeq
                && checksum.equalsIgnoreCase(row.pendingChecksum())
                && batchIdEquals(row.pendingBatchId(), batchId);
    }

    private static boolean batchIdEquals(String left, String right) {
        if (left == null || right == null) {
            return left == null && right == null;
        }
        return left.equals(right);
    }

    private record ReplayRow(
            String streamId,
            long ackedThroughSequence,
            long lastFirstSequence,
            long lastLastSequence,
            String lastChecksum,
            Long pendingFirstSequence,
            Long pendingLastSequence,
            String pendingChecksum,
            String pendingBatchId
    ) {}

    private record LeaseRow(
            String streamId,
            Long pendingFirstSequence,
            Long pendingLastSequence,
            String pendingChecksum,
            String pendingBatchId,
            UUID owner,
            long publisherEpoch,
            Timestamp leaseUntil
    ) {}

    private record FinalizeRow(
            String streamId,
            long ackedThroughSequence,
            long lastFirstSequence,
            long lastLastSequence,
            String lastChecksum,
            Long pendingFirstSequence,
            Long pendingLastSequence,
            String pendingChecksum,
            String pendingBatchId,
            UUID owner,
            long publisherEpoch,
            Timestamp leaseUntil
    ) {}
}
