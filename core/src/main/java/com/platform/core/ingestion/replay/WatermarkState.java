package com.platform.core.ingestion.replay;

import java.time.Instant;
import java.util.UUID;

/**
 * Immutable durable watermark state for a single Sensor ingress stream.
 *
 * <p>Telemetry Sensor spool ingestion uses {@link ReplayReservation} / pending-reservation
 * semantics. Agent-ingress watermarking still uses this simple last-accepted range record.
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
