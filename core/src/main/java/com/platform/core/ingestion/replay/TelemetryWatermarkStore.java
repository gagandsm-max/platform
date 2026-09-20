package com.platform.core.ingestion.replay;

import java.util.UUID;

public interface TelemetryWatermarkStore {

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
     * Finalizes a previously reserved batch only after Kafka publication commits.
     * Advances the durable ACK watermark and clears the single pending reservation.
     */
    void finalizeReservedBatch(
            UUID tenantId,
            String sensorId,
            String streamId,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId
    );
}
