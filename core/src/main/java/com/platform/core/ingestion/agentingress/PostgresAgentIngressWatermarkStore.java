package com.platform.core.ingestion.agentingress;

import com.platform.core.ingestion.TransportSequenceLimits;
import com.platform.core.ingestion.replay.PublisherClaimResult;
import com.platform.core.ingestion.replay.ReplayReservation;
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
public class PostgresAgentIngressWatermarkStore implements AgentIngressWatermarkStore {

    private static final Pattern DURABLE_STREAM_ID = Pattern.compile("^stream-[0-9a-f]{32}$");

    private final JdbcTemplate jdbcTemplate;
    private final TransactionTemplate transactionTemplate;

    public PostgresAgentIngressWatermarkStore(
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
            String batchId,
            String correlationId
    ) {
        if (streamId == null || !DURABLE_STREAM_ID.matcher(streamId).matches()) {
            return ReplayReservation.rejected(
                    0,
                    "ERR_AGENT_STREAM_ID_INVALID",
                    "Agent-ingress stream identity is not a durable Sensor agent spool generation"
            );
        }
        if (!TransportSequenceLimits.isValidWireSequence(firstSeq)
                || !TransportSequenceLimits.isValidWireSequence(lastSeq)
                || lastSeq < firstSeq) {
            return ReplayReservation.rejected(
                    0,
                    "ERR_SEQUENCE_OUT_OF_BOUNDS",
                    "Agent-ingress sequence range is outside MAX_SAFE_SEQUENCE bounds"
            );
        }
        ReplayReservation result = transactionTemplate.execute(status ->
                reserveBatchInTransaction(
                        tenantId, sensorId, streamId, firstSeq, lastSeq, checksum, batchId, correlationId));
        if (result == null) {
            throw new IllegalStateException("Agent-ingress replay reservation transaction returned no result");
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
            String batchId,
            String correlationId
    ) {
        acquireSensorLock(tenantId, sensorId);

        List<ReplayRow> rows = jdbcTemplate.query(
                """
                SELECT stream_id, acked_through_sequence, last_first_sequence, last_last_sequence,
                       last_checksum, pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_batch_id, pending_correlation_id
                  FROM platform.sensor_agent_ingress_watermark
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
                        rs.getString("pending_batch_id"),
                        rs.getString("pending_correlation_id")
                ),
                tenantId,
                sensorId
        );

        if (rows.isEmpty()) {
            if (firstSeq != 1) {
                return ReplayReservation.rejected(
                        0,
                        "ERR_SEQUENCE_GAP",
                        "New agent-ingress stream must begin at sequence 1; received firstIngressSequence "
                                + firstSeq
                );
            }
            jdbcTemplate.update(
                    """
                    INSERT INTO platform.sensor_agent_ingress_watermark (
                        tenant_id, sensor_id, stream_id, acked_through_sequence,
                        last_first_sequence, last_last_sequence, last_checksum,
                        last_batch_id, updated_at,
                        pending_first_sequence, pending_last_sequence,
                        pending_checksum, pending_batch_id, pending_correlation_id, pending_reserved_at
                    ) VALUES (?, ?, ?, 0, 0, 0, '', NULL, CURRENT_TIMESTAMP, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
                    """,
                    tenantId, sensorId, streamId, firstSeq, lastSeq, checksum, batchId, correlationId
            );
            return ReplayReservation.reserved(0);
        }

        ReplayRow row = rows.get(0);

        boolean legacyUnboundStream = row.streamId() == null;
        if (!legacyUnboundStream && !row.streamId().equals(streamId)) {
            return ReplayReservation.rejected(
                    row.ackedThroughSequence(),
                    "ERR_SENSOR_STREAM_CHANGED",
                    "Sensor durable agent-ingress stream changed unexpectedly; explicit continuity reset/re-enrollment is required"
            );
        }

        if (row.pendingFirstSequence() != null) {
            if (legacyUnboundStream) {
                throw new IllegalStateException("Pending agent-ingress reservation has no durable stream binding");
            }
            boolean samePendingRange = row.pendingFirstSequence() == firstSeq
                    && row.pendingLastSequence() != null
                    && row.pendingLastSequence() == lastSeq;
            if (samePendingRange) {
                if (checksum.equalsIgnoreCase(row.pendingChecksum())
                        && batchIdEquals(row.pendingBatchId(), batchId)
                        && batchIdEquals(row.pendingCorrelationId(), correlationId)) {
                    return ReplayReservation.retryPending(row.ackedThroughSequence());
                }
                return ReplayReservation.rejected(
                        row.ackedThroughSequence(),
                        "ERR_IDEMPOTENCY_CONFLICT",
                        "Pending agent-ingress batch sequence range [" + firstSeq + ".." + lastSeq
                                + "] was reserved with a different payload checksum, batch identity, or correlationId"
                );
            }
            return ReplayReservation.rejected(
                    row.ackedThroughSequence(),
                    "ERR_PREVIOUS_BATCH_PENDING",
                    "A previous agent-ingress batch is still pending durable publication for this Sensor"
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
                    "Agent-ingress batch sequence range [" + firstSeq + ".." + lastSeq
                            + "] was previously accepted with a different payload checksum"
            );
        }

        if (firstSeq <= row.ackedThroughSequence()) {
            return ReplayReservation.rejected(
                    row.ackedThroughSequence(),
                    "ERR_SEQUENCE_REGRESSION",
                    "Batch firstIngressSequence " + firstSeq
                            + " has already been accepted (current watermark: "
                            + row.ackedThroughSequence() + ")"
            );
        }

        if (firstSeq > row.ackedThroughSequence() + 1) {
            return ReplayReservation.rejected(
                    row.ackedThroughSequence(),
                    "ERR_SEQUENCE_GAP",
                    "Batch firstIngressSequence " + firstSeq
                            + " leaves an unexpected gap from watermark "
                            + row.ackedThroughSequence()
            );
        }

        bindStreamIfNeeded(tenantId, sensorId, streamId, legacyUnboundStream);

        int updated = jdbcTemplate.update(
                """
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_first_sequence = ?,
                       pending_last_sequence = ?,
                       pending_checksum = ?,
                       pending_batch_id = ?,
                       pending_correlation_id = ?,
                       pending_reserved_at = CURRENT_TIMESTAMP,
                       pending_publisher_owner = NULL,
                       pending_publisher_claimed_at = NULL,
                       pending_publisher_lease_until = NULL,
                       updated_at = CURRENT_TIMESTAMP
                 WHERE tenant_id = ? AND sensor_id = ?
                """,
                firstSeq, lastSeq, checksum, batchId, correlationId, tenantId, sensorId
        );
        if (updated != 1) {
            throw new IllegalStateException("Failed to reserve agent-ingress replay watermark for Sensor");
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
            String batchId,
            String correlationId
    ) {
        PublisherClaimResult result = transactionTemplate.execute(status ->
                claimPublisherLeaseInTransaction(
                        tenantId, sensorId, streamId, firstSeq, lastSeq, checksum, batchId, correlationId));
        if (result == null) {
            throw new IllegalStateException("Agent-ingress publisher claim transaction returned no result");
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
            String batchId,
            String correlationId
    ) {
        acquireSensorLock(tenantId, sensorId);
        List<LeaseRow> rows = jdbcTemplate.query(
                """
                SELECT stream_id, pending_first_sequence, pending_last_sequence, pending_checksum,
                       pending_batch_id, pending_correlation_id,
                       pending_publisher_owner, pending_publisher_epoch, pending_publisher_lease_until
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id = ? AND sensor_id = ?
                 FOR UPDATE
                """,
                (rs, rowNum) -> new LeaseRow(
                        rs.getString("stream_id"),
                        (Long) rs.getObject("pending_first_sequence"),
                        (Long) rs.getObject("pending_last_sequence"),
                        rs.getString("pending_checksum"),
                        rs.getString("pending_batch_id"),
                        rs.getString("pending_correlation_id"),
                        (UUID) rs.getObject("pending_publisher_owner"),
                        rs.getLong("pending_publisher_epoch"),
                        rs.getTimestamp("pending_publisher_lease_until")
                ),
                tenantId,
                sensorId
        );
        if (rows.isEmpty()) {
            return PublisherClaimResult.rejected(
                    "ERR_PREVIOUS_BATCH_PENDING",
                    "No agent-ingress watermark row for publisher claim"
            );
        }
        LeaseRow row = rows.get(0);
        if (row.streamId() == null || !row.streamId().equals(streamId)) {
            return PublisherClaimResult.rejected(
                    "ERR_SENSOR_STREAM_CHANGED",
                    "Agent-ingress stream mismatch during publisher claim"
            );
        }
        if (!exactPending(row, firstSeq, lastSeq, checksum, batchId, correlationId)) {
            return PublisherClaimResult.rejected(
                    "ERR_PREVIOUS_BATCH_PENDING",
                    "Publisher claim requires the exact pending agent-ingress reservation identity"
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
                UPDATE platform.sensor_agent_ingress_watermark
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
            throw new IllegalStateException("Failed to claim agent-ingress publisher lease");
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
            String correlationId,
            UUID ownerToken,
            long publisherEpoch
    ) {
        transactionTemplate.executeWithoutResult(status -> {
            acquireSensorLock(tenantId, sensorId);
            jdbcTemplate.update(
                    """
                    UPDATE platform.sensor_agent_ingress_watermark
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
                       AND pending_correlation_id = ?
                       AND pending_publisher_owner = ?
                       AND pending_publisher_epoch = ?
                    """,
                    tenantId, sensorId, streamId, firstSeq, lastSeq, checksum, batchId, correlationId,
                    ownerToken, publisherEpoch
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
            String correlationId,
            UUID ownerToken,
            long publisherEpoch
    ) {
        transactionTemplate.executeWithoutResult(status ->
                finalizeReservedBatchInTransaction(
                        tenantId, sensorId, streamId, firstSeq, lastSeq, checksum, batchId, correlationId,
                        ownerToken, publisherEpoch));
    }

    private void finalizeReservedBatchInTransaction(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId,
            String correlationId,
            UUID ownerToken,
            long publisherEpoch
    ) {
        acquireSensorLock(tenantId, sensorId);

        List<FinalizeRow> rows = jdbcTemplate.query(
                """
                SELECT stream_id, acked_through_sequence, last_first_sequence, last_last_sequence,
                       last_checksum, pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_batch_id, pending_correlation_id,
                       pending_publisher_owner, pending_publisher_epoch, pending_publisher_lease_until
                  FROM platform.sensor_agent_ingress_watermark
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
                        rs.getString("pending_correlation_id"),
                        (UUID) rs.getObject("pending_publisher_owner"),
                        rs.getLong("pending_publisher_epoch"),
                        rs.getTimestamp("pending_publisher_lease_until")
                ),
                tenantId,
                sensorId
        );
        if (rows.isEmpty()) {
            throw new IllegalStateException("Agent-ingress replay reservation disappeared before finalize");
        }

        FinalizeRow row = rows.get(0);
        if (row.streamId() == null || !row.streamId().equals(streamId)) {
            throw new IllegalStateException("Agent-ingress stream identity changed before finalize");
        }
        if (row.pendingFirstSequence() == null) {
            if (row.lastFirstSequence() == firstSeq
                    && row.lastLastSequence() == lastSeq
                    && checksum.equalsIgnoreCase(row.lastChecksum())) {
                return;
            }
            throw new IllegalStateException("No matching pending agent-ingress batch exists for finalize");
        }

        if (row.pendingFirstSequence() != firstSeq
                || row.pendingLastSequence() == null
                || row.pendingLastSequence() != lastSeq
                || !checksum.equalsIgnoreCase(row.pendingChecksum())
                || !batchIdEquals(row.pendingBatchId(), batchId)
                || !batchIdEquals(row.pendingCorrelationId(), correlationId)) {
            throw new IllegalStateException("Pending agent-ingress batch changed before finalize");
        }
        if (ownerToken == null
                || row.owner() == null
                || !ownerToken.equals(row.owner())
                || row.publisherEpoch() != publisherEpoch) {
            throw new IllegalStateException("Stale or mismatched agent-ingress publisher cannot finalize");
        }
        if (row.leaseUntil() == null) {
            throw new IllegalStateException("Agent-ingress publisher lease missing; cannot finalize");
        }

        int updated = jdbcTemplate.update(
                """
                UPDATE platform.sensor_agent_ingress_watermark
                   SET acked_through_sequence = ?,
                       last_first_sequence = ?,
                       last_last_sequence = ?,
                       last_checksum = ?,
                       last_batch_id = ?,
                       pending_first_sequence = NULL,
                       pending_last_sequence = NULL,
                       pending_checksum = NULL,
                       pending_batch_id = NULL,
                       pending_correlation_id = NULL,
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
                    "Agent-ingress publisher lease expired or ownership changed before finalize; pending reservation retained");
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
                UPDATE platform.sensor_agent_ingress_watermark
                   SET stream_id = ?, updated_at = CURRENT_TIMESTAMP
                 WHERE tenant_id = ? AND sensor_id = ? AND stream_id IS NULL
                   AND pending_first_sequence IS NULL
                """,
                streamId, tenantId, sensorId
        );
        if (bound != 1) {
            throw new IllegalStateException("Failed to bind durable agent-ingress stream identity");
        }
    }

    private void acquireSensorLock(UUID tenantId, String sensorId) {
        jdbcTemplate.execute(
                "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
                (PreparedStatement ps) -> {
                    ps.setString(1, "sensor_agent_ingress_watermark:" + tenantId + ":" + sensorId);
                    ps.execute();
                    return null;
                }
        );
    }

    private static boolean exactPending(
            LeaseRow row,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId,
            String correlationId
    ) {
        return row.pendingFirstSequence() != null
                && row.pendingFirstSequence() == firstSeq
                && row.pendingLastSequence() != null
                && row.pendingLastSequence() == lastSeq
                && checksum.equalsIgnoreCase(row.pendingChecksum())
                && batchIdEquals(row.pendingBatchId(), batchId)
                && batchIdEquals(row.pendingCorrelationId(), correlationId);
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
            String pendingBatchId,
            String pendingCorrelationId
    ) {}

    private record LeaseRow(
            String streamId,
            Long pendingFirstSequence,
            Long pendingLastSequence,
            String pendingChecksum,
            String pendingBatchId,
            String pendingCorrelationId,
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
            String pendingCorrelationId,
            UUID owner,
            long publisherEpoch,
            Timestamp leaseUntil
    ) {}
}
