package com.platform.core.ingestion.replay;

import java.util.UUID;

public interface TelemetryWatermarkStore {

    long PUBLISHER_LEASE_SECONDS = 60L;

    /**
     * Durably reserves the next sequential batch before Kafka publication. The reservation
     * is committed independently so a Core crash after Kafka commit cannot forget which
     * checksum owned the sequence range. A retry of the same range/checksum may republish;
     * a different payload for the pending range is rejected.
     */
    ReplayReservation reserveBatch(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId
    );

    /**
     * Claims exclusive publisher ownership for the exact pending reservation.
     * Unexpired foreign lease returns BUSY without Kafka publication.
     * Expired lease may be taken over only for the exact same pending identity.
     */
    PublisherClaimResult claimPublisherLease(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId
    );

    /**
     * Releases only the caller's own unexpired/own lease after a known Kafka abort.
     * Pending reservation identity is retained.
     */
    void releasePublisherLease(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId,
            UUID ownerToken,
            long publisherEpoch
    );

    /**
     * Finalizes a previously reserved batch only after Kafka publication commits.
     * Requires matching publisher owner token and epoch. Advances ACK watermark and clears pending/lease.
     */
    void finalizeReservedBatch(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId,
            UUID ownerToken,
            long publisherEpoch
    );
}
