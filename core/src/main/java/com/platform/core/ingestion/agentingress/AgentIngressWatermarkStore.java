package com.platform.core.ingestion.agentingress;

import com.platform.core.ingestion.replay.PublisherClaimResult;
import com.platform.core.ingestion.replay.ReplayReservation;
import com.platform.core.ingestion.agentingress.dto.AgentIngressRecord;

import java.util.UUID;

public interface AgentIngressWatermarkStore {

    long PUBLISHER_LEASE_SECONDS = 60L;

    ReplayReservation reserveBatch(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId,
            String correlationId
    );

    /** Bind the exact encoded request for new dispositions; null legacy digests remain replayable. */
    default ReplayReservation reserveBatchWithBody(
            UUID tenantId, String sensorId, String streamId, long firstSeq, long lastSeq,
            String checksum, String batchId, String correlationId, String requestSha256
    ) {
        return reserveBatch(tenantId, sensorId, streamId, firstSeq, lastSeq,
                checksum, batchId, correlationId);
    }

    /** Clear only this unclaimed, never-published pending generation. False means custody stays retryable. */
    boolean releaseUnpublishedPending(
            UUID tenantId, String sensorId, String streamId, long firstSeq, long lastSeq,
            String checksum, String batchId, String correlationId, String requestSha256,
            UUID pendingReservationId
    );

    ReplayReservation terminallyRejectIdentityConflict(
            UUID tenantId, String sensorId, String streamId, long sequence,
            String checksum, String batchId, String correlationId,
            AgentIngressRecord record, UUID entityId, Long bindingVersion
    );

    boolean terminalReplayMatchesRecord(UUID tenantId, String sensorId,
            UUID dispositionId, AgentIngressRecord record);

    PublisherClaimResult claimPublisherLease(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId,
            String correlationId
    );

    void releasePublisherLease(
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
    );

    void finalizeReservedBatch(
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
    );
}
