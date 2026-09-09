package com.platform.core.ingestion.replay;

import java.util.UUID;

public interface TelemetryWatermarkStore {

    /**
     * Obtains the current watermark state for the given sensor with transactional serialization.
     * Must be called within an active transaction.
     *
     * @param sensorId authoritative sensor identifier
     * @return current WatermarkState, or null if no watermark has been recorded yet
     */
    WatermarkState getWatermarkForUpdate(String sensorId);

    /**
     * Atomically creates or updates the durable watermark state for the sensor.
     *
     * @param sensorId authoritative sensor identifier
     * @param tenantId authoritative tenant UUID
     * @param ackedThrough newly accepted durable sequence watermark
     * @param firstSeq first sequence of the accepted batch
     * @param lastSeq last sequence of the accepted batch
     * @param checksum checksum of the accepted batch
     * @param batchId batch identifier (diagnostic/correlation only)
     */
    void updateWatermark(String sensorId, UUID tenantId, long ackedThrough, long firstSeq, long lastSeq, String checksum, String batchId);
}
