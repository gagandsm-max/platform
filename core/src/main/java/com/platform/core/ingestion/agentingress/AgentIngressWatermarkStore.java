package com.platform.core.ingestion.agentingress;

import com.platform.core.ingestion.replay.PublisherClaimResult;
import com.platform.core.ingestion.replay.ReplayReservation;

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
