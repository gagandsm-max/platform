package com.platform.core.ingestion.agentingress;

import com.platform.core.ingestion.TransportSequenceLimits;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.agentingress.dto.AgentIngressRecord;
import com.platform.core.ingestion.replay.PublisherClaimResult;
import com.platform.core.ingestion.replay.ReplayReservation;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Repository;
import org.springframework.http.HttpStatus;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.support.TransactionTemplate;

import java.sql.PreparedStatement;
import java.sql.Timestamp;
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
        return reserveBatchWithBody(tenantId, sensorId, streamId, firstSeq, lastSeq,
                checksum, batchId, correlationId, null);
    }

    @Override
    public ReplayReservation reserveBatchWithBody(
            UUID tenantId, String sensorId, String streamId, long firstSeq, long lastSeq,
            String checksum, String batchId, String correlationId, String requestSha256
    ) {
        if (requestSha256 != null && !requestSha256.matches("^[0-9a-f]{64}$")) {
            throw new IllegalArgumentException("Agent-ingress request digest is invalid");
        }
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
                        tenantId, sensorId, streamId, firstSeq, lastSeq, checksum, batchId,
                        correlationId, requestSha256));
        if (result == null) {
            throw new IllegalStateException("Agent-ingress replay reservation transaction returned no result");
        }
        return result;
    }

    @Override
    public boolean terminalReplayMatchesRecord(UUID tenantId, String sensorId,
            UUID dispositionId, AgentIngressRecord record) {
        if (record == null || dispositionId == null) return false;
        Integer matches = jdbcTemplate.queryForObject(
                """
                SELECT count(*) FROM platform.sensor_agent_ingress_terminal_disposition
                 WHERE tenant_id = ? AND sensor_id = ? AND disposition_id = ?
                   AND sensor_ingress_record_id = ? AND agent_batch_id = ?
                   AND agent_id = ? AND assignment_generation = ?
                """, Integer.class, tenantId, sensorId, dispositionId,
                record.sensorIngressRecordId(), record.agentBatchId(),
                record.agentId(), record.assignmentGeneration());
        return matches != null && matches == 1;
    }

    @Override
    public ReplayReservation terminallyRejectIdentityConflict(
            UUID tenantId, String sensorId, String streamId, long sequence,
            String checksum, String batchId, String correlationId,
            AgentIngressRecord record, UUID entityId, Long bindingVersion
    ) {
        if (record == null || record.sensorIngressSequence() != sequence
                || record.sensorIngressRecordId() == null || record.sensorIngressRecordId().isBlank()
                || record.agentBatchId() == null || record.agentBatchId().isBlank()) {
            throw new IllegalArgumentException("Terminal Agent-ingress evidence identity is incomplete");
        }
        ReplayReservation result = transactionTemplate.execute(status -> {
            acquireSensorLock(tenantId, sensorId);
            List<TerminalRow> prior = jdbcTemplate.query(
                    """
                    SELECT disposition_id, last_sequence, checksum, request_sha256,
                           batch_id, correlation_id, reason_code
                      FROM platform.sensor_agent_ingress_terminal_disposition
                     WHERE tenant_id = ? AND sensor_id = ? AND stream_id = ? AND first_sequence = ?
                    """,
                    (rs, n) -> new TerminalRow((UUID) rs.getObject("disposition_id"),
                            rs.getLong("last_sequence"), rs.getString("checksum"),
                            rs.getString("request_sha256"),
                            rs.getString("batch_id"), rs.getString("correlation_id"),
                            rs.getString("reason_code")), tenantId, sensorId, streamId, sequence);
            if (!prior.isEmpty()) {
                TerminalRow t = prior.get(0);
                if (t.lastSequence() != sequence || !checksum.equalsIgnoreCase(t.checksum())) {
                    return ReplayReservation.rejected(0, "ERR_REPLAY_CONFLICT",
                            "Agent-ingress settled sequence has a different payload checksum");
                }
                if (!batchIdEquals(t.batchId(), batchId)
                        || !batchIdEquals(t.correlationId(), correlationId)
                        || !terminalReplayMatchesRecord(tenantId, sensorId, t.id(), record)) {
                    return ReplayReservation.rejected(0, "ERR_IDEMPOTENCY_CONFLICT",
                            "Agent-ingress terminal disposition identity differs");
                }
                Long settled = jdbcTemplate.queryForObject(
                        "SELECT settled_through_sequence FROM platform.sensor_agent_ingress_watermark "
                                + "WHERE tenant_id = ? AND sensor_id = ?", Long.class, tenantId, sensorId);
                Long acked = jdbcTemplate.queryForObject(
                        "SELECT acked_through_sequence FROM platform.sensor_agent_ingress_watermark "
                                + "WHERE tenant_id = ? AND sensor_id = ?", Long.class, tenantId, sensorId);
                if (settled == null || acked == null || settled < sequence) {
                    throw new IllegalStateException("Terminal disposition lacks durable continuity watermark");
                }
                return ReplayReservation.terminallyRejected(acked, settled, t.id(), t.reasonCode());
            }

            List<TerminalPendingRow> rows = jdbcTemplate.query(
                    """
                    SELECT stream_id, acked_through_sequence, settled_through_sequence,
                           pending_first_sequence, pending_last_sequence, pending_checksum,
                           pending_request_sha256,
                           pending_batch_id, pending_correlation_id, pending_publisher_owner
                      FROM platform.sensor_agent_ingress_watermark
                     WHERE tenant_id = ? AND sensor_id = ? FOR UPDATE
                    """,
                    (rs, n) -> new TerminalPendingRow(rs.getString("stream_id"),
                            rs.getLong("acked_through_sequence"), rs.getLong("settled_through_sequence"),
                            (Long) rs.getObject("pending_first_sequence"),
                            (Long) rs.getObject("pending_last_sequence"),
                            rs.getString("pending_checksum"), rs.getString("pending_request_sha256"),
                            rs.getString("pending_batch_id"),
                            rs.getString("pending_correlation_id"),
                            (UUID) rs.getObject("pending_publisher_owner")), tenantId, sensorId);
            if (rows.size() != 1) {
                throw new IllegalStateException("No pending Agent-ingress watermark for terminal disposition");
            }
            TerminalPendingRow row = rows.get(0);
            if (!streamId.equals(row.streamId()) || row.settledThroughSequence() != sequence - 1
                    || row.pendingFirstSequence() == null || row.pendingFirstSequence() != sequence
                    || row.pendingLastSequence() == null || row.pendingLastSequence() != sequence
                    || !checksum.equalsIgnoreCase(row.pendingChecksum())
                    || !batchIdEquals(batchId, row.pendingBatchId())
                    || !batchIdEquals(correlationId, row.pendingCorrelationId())
                    || row.publisherOwner() != null) {
                throw new IllegalStateException("Agent-ingress terminal candidate is not the exact unclaimed head");
            }
            List<String> currentSensors = jdbcTemplate.query(
                    """
                    SELECT sensor_id FROM platform.sensor
                     WHERE tenant_id = ? AND sensor_id = ? AND status = 'ACTIVE' FOR SHARE
                    """, (rs, n) -> rs.getString("sensor_id"), tenantId, sensorId);
            if (currentSensors.size() != 1) {
                throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                        "Current Sensor authority is required for terminal settlement");
            }
            List<UUID> currentAssignments = jdbcTemplate.query(
                    """
                    SELECT assignment_id FROM platform.agent_sensor_assignment
                     WHERE tenant_id = ? AND sensor_id = ? AND agent_id = ?
                       AND generation = ? AND status = 'ACTIVE' FOR SHARE
                    """, (rs, n) -> (UUID) rs.getObject("assignment_id"),
                    tenantId, sensorId, record.agentId().trim(), record.assignmentGeneration());
            if (currentAssignments.size() != 1 || entityId == null || bindingVersion == null) {
                throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_ASSIGNMENT_UNKNOWN",
                        "Current Agent assignment and binding are required for terminal settlement");
            }
            List<UUID> currentBindings = jdbcTemplate.query(
                    """
                    SELECT entity_id FROM platform.entity_agent_binding
                     WHERE tenant_id = ? AND agent_id = ? AND entity_id = ?
                       AND reinstall_generation = ? AND binding_state = 'ACTIVE' FOR SHARE
                    """, (rs, n) -> (UUID) rs.getObject("entity_id"),
                    tenantId, record.agentId().trim(), entityId, bindingVersion);
            if (currentBindings.size() != 1) {
                throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_ENTITY_BINDING_INACTIVE",
                        "Current Agent binding changed before terminal settlement");
            }
            UUID dispositionId = UUID.randomUUID();
            jdbcTemplate.update(
                    """
                    INSERT INTO platform.sensor_agent_ingress_terminal_disposition (
                        disposition_id, tenant_id, sensor_id, stream_id, first_sequence, last_sequence,
                        checksum, request_sha256, batch_id, sensor_ingress_record_id, agent_batch_id, correlation_id,
                        agent_id, assignment_generation, entity_id, binding_version,
                        reason_code, disposition_state
                    ) VALUES (?, ?, ?, ?, ?, ?, lower(?), ?, ?, ?, ?, ?, ?, ?, ?, ?,
                              'ERR_AGENT_IDENTITY_CONFLICT', 'TERMINALLY_REJECTED')
                    """,
                    dispositionId, tenantId, sensorId, streamId, sequence, sequence,
                    checksum, row.pendingRequestSha256(), batchId,
                    record.sensorIngressRecordId(), record.agentBatchId(),
                    correlationId, record.agentId(), record.assignmentGeneration(), entityId, bindingVersion);
            int updated = jdbcTemplate.update(
                    """
                    UPDATE platform.sensor_agent_ingress_watermark
                       SET settled_through_sequence = ?,
                           pending_first_sequence = NULL, pending_last_sequence = NULL,
                           pending_checksum = NULL, pending_request_sha256 = NULL,
                           pending_batch_id = NULL,
                           pending_correlation_id = NULL, pending_reserved_at = NULL,
                           pending_publisher_owner = NULL, pending_publisher_claimed_at = NULL,
                           pending_publisher_lease_until = NULL, updated_at = CURRENT_TIMESTAMP
                     WHERE tenant_id = ? AND sensor_id = ? AND stream_id = ?
                       AND settled_through_sequence = ?
                    """, sequence, tenantId, sensorId, streamId, sequence - 1);
            if (updated != 1) {
                throw new IllegalStateException("Agent-ingress terminal continuity advance failed");
            }
            return ReplayReservation.terminallyRejected(
                    row.ackedThroughSequence(), sequence, dispositionId, "ERR_AGENT_IDENTITY_CONFLICT");
        });
        if (result == null) throw new IllegalStateException("Terminal disposition transaction returned no result");
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
            String correlationId,
            String requestSha256
    ) {
        acquireSensorLock(tenantId, sensorId);

        List<TerminalRow> terminal = jdbcTemplate.query(
                """
                SELECT disposition_id, last_sequence, checksum, request_sha256,
                       batch_id, correlation_id,
                       reason_code
                  FROM platform.sensor_agent_ingress_terminal_disposition
                 WHERE tenant_id = ? AND sensor_id = ? AND stream_id = ? AND first_sequence = ?
                """,
                (rs, rowNum) -> new TerminalRow(
                        (UUID) rs.getObject("disposition_id"), rs.getLong("last_sequence"),
                        rs.getString("checksum"), rs.getString("request_sha256"),
                        rs.getString("batch_id"),
                        rs.getString("correlation_id"), rs.getString("reason_code")),
                tenantId, sensorId, streamId, firstSeq);
        if (!terminal.isEmpty()) {
            TerminalRow t = terminal.get(0);
            if (t.lastSequence() != lastSeq || !checksum.equalsIgnoreCase(t.checksum())) {
                return ReplayReservation.rejected(0, "ERR_REPLAY_CONFLICT",
                        "Agent-ingress settled sequence has a different payload checksum");
            }
            if (requestSha256 != null && t.requestSha256() != null
                    && !requestSha256.equals(t.requestSha256())) {
                return ReplayReservation.rejected(0, "ERR_REPLAY_CONFLICT",
                        "Agent-ingress terminal request bytes differ from durable disposition");
            }
            if (!batchIdEquals(t.batchId(), batchId)
                    || !batchIdEquals(t.correlationId(), correlationId)) {
                return ReplayReservation.rejected(0, "ERR_IDEMPOTENCY_CONFLICT",
                        "Agent-ingress sequence has a different durable terminal identity");
            }
            Long settled = jdbcTemplate.queryForObject(
                    "SELECT settled_through_sequence FROM platform.sensor_agent_ingress_watermark "
                            + "WHERE tenant_id = ? AND sensor_id = ?", Long.class, tenantId, sensorId);
            Long acked = jdbcTemplate.queryForObject(
                    "SELECT acked_through_sequence FROM platform.sensor_agent_ingress_watermark "
                            + "WHERE tenant_id = ? AND sensor_id = ?", Long.class, tenantId, sensorId);
            if (settled == null || acked == null || settled < lastSeq) {
                throw new IllegalStateException("Terminal disposition lacks durable continuity watermark");
            }
            return ReplayReservation.terminallyRejected(acked, settled, t.id(), t.reasonCode());
        }

        List<ReplayRow> rows = jdbcTemplate.query(
                """
                SELECT stream_id, acked_through_sequence, settled_through_sequence,
                       last_first_sequence, last_last_sequence,
                       last_checksum, last_batch_id, last_request_sha256,
                       pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_request_sha256,
                       pending_batch_id, pending_correlation_id
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id = ? AND sensor_id = ?
                 FOR UPDATE
                """,
                (rs, rowNum) -> new ReplayRow(
                        rs.getString("stream_id"),
                        rs.getLong("acked_through_sequence"),
                        rs.getLong("settled_through_sequence"),
                        rs.getLong("last_first_sequence"),
                        rs.getLong("last_last_sequence"),
                        rs.getString("last_checksum"),
                        rs.getString("last_batch_id"),
                        rs.getString("last_request_sha256"),
                        (Long) rs.getObject("pending_first_sequence"),
                        (Long) rs.getObject("pending_last_sequence"),
                        rs.getString("pending_checksum"),
                        rs.getString("pending_request_sha256"),
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
                        settled_through_sequence,
                        last_first_sequence, last_last_sequence, last_checksum,
                        last_batch_id, last_request_sha256, updated_at,
                        pending_first_sequence, pending_last_sequence,
                        pending_checksum, pending_request_sha256,
                        pending_batch_id, pending_correlation_id, pending_reserved_at
                    ) VALUES (?, ?, ?, 0, 0, 0, 0, '', NULL, NULL,
                              CURRENT_TIMESTAMP, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
                    """,
                    tenantId, sensorId, streamId, firstSeq, lastSeq, checksum,
                    requestSha256, batchId, correlationId
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

        // A lost accepted response is an exact durable replay even if a later
        // batch has since acquired the single pending publisher slot.
        if (firstSeq == row.lastFirstSequence() && lastSeq == row.lastLastSequence()) {
            if (!checksum.equalsIgnoreCase(row.lastChecksum())) {
                return ReplayReservation.rejected(row.ackedThroughSequence(), "ERR_REPLAY_CONFLICT",
                        "Agent-ingress accepted range has a different payload checksum");
            }
            if (requestSha256 != null && row.lastRequestSha256() != null
                    && !requestSha256.equals(row.lastRequestSha256())) {
                return ReplayReservation.rejected(row.ackedThroughSequence(), "ERR_REPLAY_CONFLICT",
                        "Agent-ingress accepted request bytes differ from durable disposition");
            }
            if (row.lastBatchId() == null || batchIdEquals(row.lastBatchId(), batchId)) {
                bindStreamIfNeeded(tenantId, sensorId, streamId, legacyUnboundStream);
                return ReplayReservation.alreadyAcked(row.ackedThroughSequence());
            }
            return ReplayReservation.rejected(
                    row.ackedThroughSequence(),
                    "ERR_IDEMPOTENCY_CONFLICT",
                    "Agent-ingress batch sequence range [" + firstSeq + ".." + lastSeq
                            + "] was previously accepted with a different payload checksum or batch identity"
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
                if (requestSha256 != null && row.pendingRequestSha256() != null
                        && !requestSha256.equals(row.pendingRequestSha256())) {
                    return ReplayReservation.rejected(row.ackedThroughSequence(), "ERR_REPLAY_CONFLICT",
                            "Agent-ingress pending request bytes differ from durable reservation");
                }
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

        if (firstSeq <= row.settledThroughSequence()) {
            return ReplayReservation.rejected(
                    row.ackedThroughSequence(),
                    "ERR_SEQUENCE_REGRESSION",
                    "Batch firstIngressSequence " + firstSeq
                            + " has already been settled (current watermark: "
                            + row.settledThroughSequence() + ")"
            );
        }

        if (firstSeq > row.settledThroughSequence() + 1) {
            return ReplayReservation.rejected(
                    row.ackedThroughSequence(),
                    "ERR_SEQUENCE_GAP",
                    "Batch firstIngressSequence " + firstSeq
                            + " leaves an unexpected gap from watermark "
                            + row.settledThroughSequence()
            );
        }

        bindStreamIfNeeded(tenantId, sensorId, streamId, legacyUnboundStream);

        int updated = jdbcTemplate.update(
                """
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_first_sequence = ?,
                       pending_last_sequence = ?,
                       pending_checksum = ?,
                       pending_request_sha256 = ?,
                       pending_batch_id = ?,
                       pending_correlation_id = ?,
                       pending_reserved_at = CURRENT_TIMESTAMP,
                       pending_publisher_owner = NULL,
                       pending_publisher_claimed_at = NULL,
                       pending_publisher_lease_until = NULL,
                       updated_at = CURRENT_TIMESTAMP
                 WHERE tenant_id = ? AND sensor_id = ?
                """,
                firstSeq, lastSeq, checksum, requestSha256, batchId, correlationId,
                tenantId, sensorId
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
                       pending_publisher_owner, pending_publisher_epoch, pending_publisher_lease_until,
                       (pending_publisher_owner IS NOT NULL
                        AND pending_publisher_lease_until IS NOT NULL
                        AND pending_publisher_lease_until > clock_timestamp()) AS lease_active
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
                        rs.getTimestamp("pending_publisher_lease_until"),
                        rs.getBoolean("lease_active")
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
        if (row.leaseActive()) {
            return PublisherClaimResult.busy();
        }
        UUID owner = UUID.randomUUID();
        long epoch = row.publisherEpoch() + 1;
        int updated = jdbcTemplate.update(
                """
                UPDATE platform.sensor_agent_ingress_watermark
                   SET pending_publisher_owner = ?,
                       pending_publisher_epoch = ?,
                       pending_publisher_claimed_at = clock_timestamp(),
                       pending_publisher_lease_until = clock_timestamp() + make_interval(secs => ?),
                       updated_at = CURRENT_TIMESTAMP
                 WHERE tenant_id = ? AND sensor_id = ?
                """,
                owner, epoch, PUBLISHER_LEASE_SECONDS, tenantId, sensorId
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
                SELECT stream_id, acked_through_sequence, settled_through_sequence,
                       last_first_sequence, last_last_sequence,
                       last_checksum, last_batch_id, pending_first_sequence, pending_last_sequence,
                       pending_checksum, pending_batch_id, pending_correlation_id,
                       pending_publisher_owner, pending_publisher_epoch, pending_publisher_lease_until
                  FROM platform.sensor_agent_ingress_watermark
                 WHERE tenant_id = ? AND sensor_id = ?
                 FOR UPDATE
                """,
                (rs, rowNum) -> new FinalizeRow(
                        rs.getString("stream_id"),
                        rs.getLong("acked_through_sequence"),
                        rs.getLong("settled_through_sequence"),
                        rs.getLong("last_first_sequence"),
                        rs.getLong("last_last_sequence"),
                        rs.getString("last_checksum"),
                        rs.getString("last_batch_id"),
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
                    && checksum.equalsIgnoreCase(row.lastChecksum())
                    && (row.lastBatchId() == null || batchIdEquals(row.lastBatchId(), batchId))) {
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
        if (row.settledThroughSequence() != firstSeq - 1) {
            throw new IllegalStateException("Agent-ingress settled continuity changed before finalize");
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
                   SET acked_through_sequence = CASE
                           WHEN acked_through_sequence = ? THEN ? ELSE acked_through_sequence END,
                       settled_through_sequence = ?,
                       last_first_sequence = ?,
                       last_last_sequence = ?,
                       last_checksum = ?,
                       last_batch_id = ?,
                       last_request_sha256 = pending_request_sha256,
                       pending_first_sequence = NULL,
                       pending_last_sequence = NULL,
                       pending_checksum = NULL,
                       pending_request_sha256 = NULL,
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
                   AND pending_publisher_lease_until > clock_timestamp()
                """,
                firstSeq - 1, lastSeq, lastSeq, firstSeq, lastSeq, checksum, batchId,
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
            long settledThroughSequence,
            long lastFirstSequence,
            long lastLastSequence,
            String lastChecksum,
            String lastBatchId,
            String lastRequestSha256,
            Long pendingFirstSequence,
            Long pendingLastSequence,
            String pendingChecksum,
            String pendingRequestSha256,
            String pendingBatchId,
            String pendingCorrelationId
    ) {}

    private record TerminalRow(UUID id, long lastSequence, String checksum, String requestSha256,
            String batchId, String correlationId, String reasonCode) {}

    private record TerminalPendingRow(String streamId, long ackedThroughSequence,
            long settledThroughSequence, Long pendingFirstSequence, Long pendingLastSequence,
            String pendingChecksum, String pendingRequestSha256,
            String pendingBatchId, String pendingCorrelationId,
            UUID publisherOwner) {}

    private record LeaseRow(
            String streamId,
            Long pendingFirstSequence,
            Long pendingLastSequence,
            String pendingChecksum,
            String pendingBatchId,
            String pendingCorrelationId,
            UUID owner,
            long publisherEpoch,
            Timestamp leaseUntil,
            boolean leaseActive
    ) {}

    private record FinalizeRow(
            String streamId,
            long ackedThroughSequence,
            long settledThroughSequence,
            long lastFirstSequence,
            long lastLastSequence,
            String lastChecksum,
            String lastBatchId,
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
