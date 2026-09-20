package com.platform.core.ingestion.replay;

import java.util.UUID;

public interface TelemetryWatermarkStore {

    /**
     * Obtains the current watermark state for the given tenant and sensor with transactional serialization.
     * Must be called within an active transaction.
     *
     * @param tenantId authoritative tenant UUID
     * @param sensorId authoritative sensor identifier
     * @return current WatermarkState, or null if no watermark has been recorded yet
     */
    WatermarkState getWatermarkForUpdate(UUID tenantId, String sensorId);

    /**
     * Atomically creates or updates the durable watermark state for the tenant and sensor.
     *
     * @param tenantId authoritative tenant UUID
     * @param sensorId authoritative sensor identifier
     * @param ackedThrough newly accepted durable sequence watermark
     * @param firstSeq first sequence of the accepted batch
     * @param lastSeq last sequence of the accepted batch
     * @param checksum checksum of the accepted batch
     * @param batchId batch identifier (diagnostic/correlation only)
     */
    void updateWatermark(UUID tenantId, String sensorId, long ackedThrough, long firstSeq, long lastSeq, String checksum, String batchId);

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
