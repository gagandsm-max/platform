package com.platform.core.ingestion.dto;

import com.fasterxml.jackson.annotation.JsonFormat;
import com.fasterxml.jackson.annotation.JsonInclude;

import java.time.Instant;

/**
 * Canonical observation record matching ClickHouse metric_numeric schema
 * and Kafka telemetry.numeric.v1 contract.
 * Additive optional Agent provenance fields are Kafka-JSON only (NON_NULL).
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
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
        String sourceProtocol,
        String sourceType,
        String agentId,
        Long assignmentGeneration,
        String agentObservationId,
        Long agentSourceSequence,
        String agentSourceRecordId,
        String correlationId,
        @JsonFormat(shape = JsonFormat.Shape.STRING)
        Instant ingestTime
) {
    /** Backward-compatible constructor for Sensor telemetry path (no Agent provenance). */
    public CanonicalObservation(
            String tenantId,
            String entityId,
            String metricId,
            String sensorId,
            Instant timestamp,
            double value,
            int quality,
            int flags,
            String sequenceId,
            String sourceProtocol
    ) {
        this(tenantId, entityId, metricId, sensorId, timestamp, value, quality, flags,
                sequenceId, sourceProtocol, null, null, null, null, null, null, null, null);
    }
}
