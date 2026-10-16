package com.platform.core.telemetry.read;

import java.time.Instant;

/**
 * One canonical observation row from {@code platform.metric_numeric} or
 * {@code platform.metric_counter}. Counter64 values are decimal strings only.
 */
public record MetricObservationRow(
        Instant timestamp,
        Double numericValue,
        String counterValue,
        int quality,
        int flags,
        String sensorId,
        String sequenceId,
        String sourceProtocol
) {
}
