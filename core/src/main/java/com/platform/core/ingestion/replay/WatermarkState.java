package com.platform.core.ingestion.replay;

import java.time.Instant;
import java.util.UUID;

/**
 * Immutable durable replay watermark state for a single Sensor.
 */
public record WatermarkState(
        String sensorId,
        UUID tenantId,
        long ackedThroughSequence,
        long lastFirstSequence,
        long lastLastSequence,
        String lastChecksum,
        String lastBatchId,
        Instant updatedAt
) {}
