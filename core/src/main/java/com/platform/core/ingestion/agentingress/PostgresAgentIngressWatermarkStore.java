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
import java.util.function.Supplier;
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
                           pending_batch_id, pending_correlation_id, pending_reservation_id,
                           pending_publisher_owner,
                           pending_authority_state, pending_publication_attempted
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
                            (UUID) rs.getObject("pending_reservation_id"),
                            (UUID) rs.getObject("pending_publisher_owner"),
                            rs.getString("pending_authority_state"),
                            (Boolean) rs.getObject("pending_publication_attempted")), tenantId, sensorId);
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
                    || row.publisherOwner() != null
                    || !"UNVALIDATED".equals(row.authorityState())
                    || !Boolean.FALSE.equals(row.publicationAttempted())) {
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
                           pending_reservation_id = NULL, pending_publication_attempted = FALSE,
                           pending_authority_state = 'NONE', pending_authority_validated_at = NULL,
                           pending_canonical_batch_sha256 = NULL,
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
            jdbcTemplate.update("DELETE FROM platform.sensor_agent_ingress_pending_canonical_binding "
                    + "WHERE tenant_id = ? AND sensor_id = ? AND reservation_id = ?",
                    tenantId, sensorId, row.reservationId());
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
                       pending_batch_id, pending_correlation_id, pending_reservation_id,
                       pending_authority_state, pending_canonical_batch_sha256
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
                        rs.getString("pending_correlation_id"),
                        (UUID) rs.getObject("pending_reservation_id"),
                        rs.getString("pending_authority_state"),
                        rs.getString("pending_canonical_batch_sha256")
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
            UUID pendingReservationId = UUID.randomUUID();
            jdbcTemplate.update(
                    """
                    INSERT INTO platform.sensor_agent_ingress_watermark (
                        tenant_id, sensor_id, stream_id, acked_through_sequence,
                        settled_through_sequence,
                        last_first_sequence, last_last_sequence, last_checksum,
                        last_batch_id, last_request_sha256, updated_at,
                        pending_first_sequence, pending_last_sequence,
                        pending_checksum, pending_request_sha256,
                        pending_batch_id, pending_correlation_id, pending_reserved_at,
                        pending_reservation_id, pending_authority_state, pending_publication_attempted
                    ) VALUES (?, ?, ?, 0, 0, 0, 0, '', NULL, NULL,
                              CURRENT_TIMESTAMP, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, 'UNVALIDATED', FALSE)
                    """,
                    tenantId, sensorId, streamId, firstSeq, lastSeq, checksum,
                    requestSha256, batchId, correlationId, pendingReservationId
            );
            return ReplayReservation.reserved(0, pendingReservationId);
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
                if ("LEGACY_UNKNOWN".equals(row.pendingAuthorityState())
                        && row.pendingRequestSha256() == null) {
                    return ReplayReservation.rejected(row.ackedThroughSequence(),
                            "ERR_AGENT_LEGACY_PENDING_RECONCILIATION_REQUIRED",
                            "Legacy pending request digest is unavailable; retain custody for reconciliation");
                }
                if (requestSha256 != null && (row.pendingRequestSha256() == null
                        || !requestSha256.equals(row.pendingRequestSha256()))) {
                    return ReplayReservation.rejected(row.ackedThroughSequence(), "ERR_REPLAY_CONFLICT",
                            "Agent-ingress pending request bytes differ from durable reservation");
                }
                if (checksum.equalsIgnoreCase(row.pendingChecksum())
                        && batchIdEquals(row.pendingBatchId(), batchId)
                        && batchIdEquals(row.pendingCorrelationId(), correlationId)) {
                    List<PendingCanonicalBindingV1> bindings = "AUTHORITY_VALIDATED".equals(row.pendingAuthorityState())
                            ? loadBindings(tenantId, sensorId, row.pendingReservationId()) : null;
                    return ReplayReservation.retryPending(row.ackedThroughSequence(),
                            row.pendingReservationId(), row.pendingAuthorityState(), bindings,
                            row.pendingCanonicalBatchSha256());
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

        UUID pendingReservationId = UUID.randomUUID();
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
                       pending_reservation_id = ?,
                       pending_authority_state = 'UNVALIDATED',
                       pending_authority_validated_at = NULL,
                       pending_canonical_batch_sha256 = NULL,
                       pending_publication_attempted = FALSE,
                       pending_publisher_owner = NULL,
                       pending_publisher_claimed_at = NULL,
                       pending_publisher_lease_until = NULL,
                       updated_at = CURRENT_TIMESTAMP
                 WHERE tenant_id = ? AND sensor_id = ?
                """,
                firstSeq, lastSeq, checksum, requestSha256, batchId, correlationId,
                pendingReservationId,
                tenantId, sensorId
        );
        if (updated != 1) {
            throw new IllegalStateException("Failed to reserve agent-ingress replay watermark for Sensor");
        }
        return ReplayReservation.reserved(row.ackedThroughSequence(), pendingReservationId);
    }

    @Override
    public boolean releaseUnpublishedPending(
            UUID tenantId, String sensorId, String streamId, long firstSeq, long lastSeq,
            String checksum, String batchId, String correlationId, String requestSha256,
            UUID pendingReservationId
    ) {
        if (pendingReservationId == null || requestSha256 == null) return false;
        Boolean released = transactionTemplate.execute(status -> {
            acquireSensorLock(tenantId, sensorId);
            int releasedRows = jdbcTemplate.update(
                    """
                    UPDATE platform.sensor_agent_ingress_watermark
                       SET pending_first_sequence = NULL, pending_last_sequence = NULL,
                           pending_checksum = NULL, pending_request_sha256 = NULL,
                           pending_batch_id = NULL, pending_correlation_id = NULL,
                           pending_reserved_at = NULL, pending_reservation_id = NULL,
                           pending_authority_state = 'NONE',
                           pending_authority_validated_at = NULL, pending_canonical_batch_sha256 = NULL,
                           pending_publication_attempted = FALSE,
                           updated_at = CURRENT_TIMESTAMP
                     WHERE tenant_id = ? AND sensor_id = ? AND stream_id = ?
                       AND settled_through_sequence = ?
                       AND pending_first_sequence = ? AND pending_last_sequence = ?
                       AND lower(pending_checksum) = lower(?)
                       AND pending_batch_id = ? AND pending_correlation_id = ?
                       AND pending_request_sha256 = ? AND pending_reservation_id = ?
                       AND pending_publisher_owner IS NULL
                       AND pending_publisher_claimed_at IS NULL
                       AND pending_publisher_lease_until IS NULL
                       AND pending_publication_attempted = FALSE
                       AND pending_authority_state = 'UNVALIDATED'
                       AND NOT EXISTS (
                           SELECT 1 FROM platform.sensor_agent_ingress_terminal_disposition d
                            WHERE d.tenant_id = ? AND d.sensor_id = ? AND d.stream_id = ?
                              AND d.first_sequence = ?
                       )
                    """,
                    tenantId, sensorId, streamId, firstSeq - 1, firstSeq, lastSeq,
                    checksum, batchId, correlationId, requestSha256, pendingReservationId,
                    tenantId, sensorId, streamId, firstSeq
            );
            if (releasedRows == 1) {
                jdbcTemplate.update("DELETE FROM platform.sensor_agent_ingress_pending_canonical_binding "
                        + "WHERE tenant_id = ? AND sensor_id = ? AND reservation_id = ?",
                        tenantId, sensorId, pendingReservationId);
            }
            return releasedRows == 1;
        });
        return Boolean.TRUE.equals(released);
    }

    @Override
    public AuthorityCheckpoint checkpointAuthority(UUID tenantId, String sensorId, String streamId,
            long firstSeq, long lastSeq, String checksum, String batchId, String correlationId,
            String requestSha256, UUID reservationId, Supplier<AuthorityCheckpoint> validate) {
        if (requestSha256 == null || reservationId == null || validate == null) return null;
        return transactionTemplate.execute(status -> {
            acquireSensorLock(tenantId, sensorId);
            List<UUID> exact = jdbcTemplate.query("""
                    SELECT pending_reservation_id FROM platform.sensor_agent_ingress_watermark
                     WHERE tenant_id = ? AND sensor_id = ? AND stream_id = ?
                       AND pending_first_sequence = ? AND pending_last_sequence = ?
                       AND lower(pending_checksum) = lower(?)
                       AND pending_batch_id = ? AND pending_correlation_id = ?
                       AND pending_request_sha256 = ? AND pending_reservation_id = ?
                       AND pending_authority_state IN ('UNVALIDATED', 'LEGACY_UNKNOWN')
                       AND pending_publisher_owner IS NULL
                       AND pending_publisher_claimed_at IS NULL
                       AND pending_publisher_lease_until IS NULL
                     FOR UPDATE
                    """, (rs, n) -> (UUID) rs.getObject(1), tenantId, sensorId, streamId,
                    firstSeq, lastSeq, checksum, batchId, correlationId, requestSha256, reservationId);
            if (exact.size() != 1) return null;
            AuthorityCheckpoint checkpoint = validate.get();
            if (checkpoint == null || checkpoint.observations() == null
                    || checkpoint.observations().isEmpty() || checkpoint.bindings() == null
                    || checkpoint.bindings().size() != lastSeq - firstSeq + 1
                    || checkpoint.canonicalBatchSha256() == null
                    || !checkpoint.canonicalBatchSha256().matches("^[0-9a-f]{64}$")) {
                throw new IllegalStateException("Incomplete Agent-ingress canonical authority checkpoint");
            }
            for (int i = 0; i < checkpoint.bindings().size(); i++) {
                PendingCanonicalBindingV1 binding = checkpoint.bindings().get(i);
                if (binding == null || binding.sensorIngressSequence() != firstSeq + i
                        || binding.agentId() == null || binding.agentId().isBlank()
                        || binding.hostId() == null || binding.hostId().isBlank()
                        || binding.entityIdentifierId() == null || binding.entityId() == null
                        || binding.assignmentGeneration() < 1 || binding.bindingVersion() < 1) {
                    throw new IllegalStateException("Incomplete Agent-ingress canonical binding row");
                }
                jdbcTemplate.update("""
                        INSERT INTO platform.sensor_agent_ingress_pending_canonical_binding
                          (tenant_id, sensor_id, reservation_id, sensor_ingress_sequence,
                           agent_id, assignment_generation, host_id, entity_identifier_id,
                           entity_id, binding_version, validated_at)
                        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, clock_timestamp())
                        """, tenantId, sensorId, reservationId, binding.sensorIngressSequence(),
                        binding.agentId(), binding.assignmentGeneration(), binding.hostId(),
                        binding.entityIdentifierId(), binding.entityId(), binding.bindingVersion());
            }
            int updated = jdbcTemplate.update("""
                    UPDATE platform.sensor_agent_ingress_watermark
                       SET pending_authority_state = 'AUTHORITY_VALIDATED',
                           pending_authority_validated_at = clock_timestamp(),
                           pending_canonical_batch_sha256 = ?, pending_publication_attempted = FALSE,
                           updated_at = CURRENT_TIMESTAMP
                     WHERE tenant_id = ? AND sensor_id = ? AND pending_reservation_id = ?
                       AND pending_authority_state IN ('UNVALIDATED', 'LEGACY_UNKNOWN')
                    """, checkpoint.canonicalBatchSha256(), tenantId, sensorId, reservationId);
            if (updated != 1) throw new IllegalStateException("Agent-ingress checkpoint transition failed");
            return checkpoint;
        });
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
        return claimPublisherLeaseExact(tenantId, sensorId, streamId, firstSeq, lastSeq,
                checksum, batchId, correlationId, null, null);
    }

    @Override
    public PublisherClaimResult claimPublisherLeaseExact(
            UUID tenantId, String sensorId, String streamId, long firstSeq, long lastSeq,
            String checksum, String batchId, String correlationId, String requestSha256,
            UUID reservationId) {
        PublisherClaimResult result = transactionTemplate.execute(status ->
                claimPublisherLeaseInTransaction(
                        tenantId, sensorId, streamId, firstSeq, lastSeq, checksum, batchId,
                        correlationId, requestSha256, reservationId));
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
            String correlationId,
            String requestSha256,
            UUID reservationId
    ) {
        acquireSensorLock(tenantId, sensorId);
        List<LeaseRow> rows = jdbcTemplate.query(
                """
                SELECT stream_id, pending_first_sequence, pending_last_sequence, pending_checksum,
                       pending_batch_id, pending_correlation_id,
                       pending_request_sha256, pending_reservation_id, pending_authority_state,
                       pending_canonical_batch_sha256,
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
                        rs.getString("pending_request_sha256"),
                        (UUID) rs.getObject("pending_reservation_id"),
                        rs.getString("pending_authority_state"),
                        rs.getString("pending_canonical_batch_sha256"),
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
        if (!"AUTHORITY_VALIDATED".equals(row.authorityState())
                || requestSha256 == null || !requestSha256.equals(row.requestSha256())
                || reservationId == null || !reservationId.equals(row.reservationId())
                || row.canonicalBatchSha256() == null
                || !row.canonicalBatchSha256().matches("^[0-9a-f]{64}$")
                || !bindingSnapshotComplete(loadBindings(tenantId, sensorId, reservationId), firstSeq, lastSeq)) {
            return PublisherClaimResult.rejected("ERR_AGENT_AUTHORITY_CHECKPOINT_REQUIRED",
                    "Publisher claim requires exact validated request authority");
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
                       pending_publication_attempted = TRUE,
                       pending_publisher_claimed_at = clock_timestamp(),
                       pending_publisher_lease_until = clock_timestamp() + make_interval(secs => ?),
                       updated_at = CURRENT_TIMESTAMP
                 WHERE tenant_id = ? AND sensor_id = ?
                   AND stream_id = ? AND pending_reservation_id = ?
                   AND pending_request_sha256 = ?
                   AND pending_authority_state = 'AUTHORITY_VALIDATED'
                """,
                owner, epoch, PUBLISHER_LEASE_SECONDS, tenantId, sensorId,
                streamId, reservationId, requestSha256
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
                       pending_checksum, pending_batch_id, pending_correlation_id, pending_reservation_id,
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
                        (UUID) rs.getObject("pending_reservation_id"),
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
                       pending_reservation_id = NULL,
                       pending_publication_attempted = FALSE,
                       pending_authority_state = 'NONE', pending_authority_validated_at = NULL,
                       pending_canonical_batch_sha256 = NULL,
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
        jdbcTemplate.update("DELETE FROM platform.sensor_agent_ingress_pending_canonical_binding "
                + "WHERE tenant_id = ? AND sensor_id = ? AND reservation_id = ?",
                tenantId, sensorId, row.reservationId());
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

    private List<PendingCanonicalBindingV1> loadBindings(UUID tenantId, String sensorId, UUID reservationId) {
        if (reservationId == null) return List.of();
        return jdbcTemplate.query("""
                SELECT sensor_ingress_sequence, agent_id, assignment_generation, host_id,
                       entity_identifier_id, entity_id, binding_version
                  FROM platform.sensor_agent_ingress_pending_canonical_binding
                 WHERE tenant_id = ? AND sensor_id = ? AND reservation_id = ?
                 ORDER BY sensor_ingress_sequence
                """, (rs, n) -> new PendingCanonicalBindingV1(
                rs.getLong("sensor_ingress_sequence"), rs.getString("agent_id"),
                rs.getLong("assignment_generation"), rs.getString("host_id"),
                (UUID) rs.getObject("entity_identifier_id"), (UUID) rs.getObject("entity_id"),
                rs.getLong("binding_version")), tenantId, sensorId, reservationId);
    }

    private static boolean bindingSnapshotComplete(List<PendingCanonicalBindingV1> bindings,
            long firstSeq, long lastSeq) {
        if (bindings.size() != lastSeq - firstSeq + 1) return false;
        for (int i = 0; i < bindings.size(); i++) {
            if (bindings.get(i).sensorIngressSequence() != firstSeq + i) return false;
        }
        return true;
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
            String pendingCorrelationId,
            UUID pendingReservationId,
            String pendingAuthorityState,
            String pendingCanonicalBatchSha256
    ) {}

    private record TerminalRow(UUID id, long lastSequence, String checksum, String requestSha256,
            String batchId, String correlationId, String reasonCode) {}

    private record TerminalPendingRow(String streamId, long ackedThroughSequence,
            long settledThroughSequence, Long pendingFirstSequence, Long pendingLastSequence,
            String pendingChecksum, String pendingRequestSha256,
            String pendingBatchId, String pendingCorrelationId,
            UUID reservationId,
            UUID publisherOwner, String authorityState, Boolean publicationAttempted) {}

    private record LeaseRow(
            String streamId,
            Long pendingFirstSequence,
            Long pendingLastSequence,
            String pendingChecksum,
            String pendingBatchId,
            String pendingCorrelationId,
            String requestSha256,
            UUID reservationId,
            String authorityState,
            String canonicalBatchSha256,
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
            UUID reservationId,
            UUID owner,
            long publisherEpoch,
            Timestamp leaseUntil
    ) {}
}
