package com.platform.core.ingestion.dto;

import com.fasterxml.jackson.annotation.JsonFormat;
import java.time.Instant;

/**
 * Canonical observation record matching ClickHouse metric_numeric schema
 * and Kafka telemetry.numeric.v1 contract.
 */
public record CanonicalObservation(
        String tenantId,
        String entityId,
        String metricId,
        String sensorId,
        @JsonFormat(shape = JsonFormat.Shape.STRING)
        Instant timestamp,
        double value,
        int quality,
        int flags,
        String sequenceId,
        String sourceProtocol
) {}
