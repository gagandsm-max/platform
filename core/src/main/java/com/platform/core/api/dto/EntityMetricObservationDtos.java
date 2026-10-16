package com.platform.core.api.dto;

import com.fasterxml.jackson.annotation.JsonInclude;

import java.util.List;
import java.util.UUID;

public final class EntityMetricObservationDtos {
    private EntityMetricObservationDtos() {
    }

    @JsonInclude(JsonInclude.Include.NON_NULL)
    public record ObservationItem(
            String timestamp,
            Double value,
            String counterValue,
            int quality,
            int flags,
            String sensorId,
            String sequenceId,
            String sourceProtocol
    ) {
    }

    public record Provenance(
            String sourceView,
            String ordering,
            String provenanceClass
    ) {
    }

    public record MetricObservationResponse(
            UUID tenantId,
            UUID entityId,
            String metricId,
            String valueKind,
            String from,
            String to,
            int limit,
            int returned,
            String observationState,
            String freshness,
            String qualityNote,
            Provenance provenance,
            List<ObservationItem> observations
    ) {
    }
}
