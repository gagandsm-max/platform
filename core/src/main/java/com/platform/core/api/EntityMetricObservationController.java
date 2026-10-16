package com.platform.core.api;

import com.platform.core.api.dto.EntityMetricObservationDtos.MetricObservationResponse;
import com.platform.core.api.dto.EntityMetricObservationDtos.ObservationItem;
import com.platform.core.api.dto.EntityMetricObservationDtos.Provenance;
import com.platform.core.telemetry.read.EntityMetricObservationService;
import com.platform.core.telemetry.read.EntityMetricObservationService.EntityMetricObservationResult;
import com.platform.core.telemetry.read.MetricObservationQuery;
import com.platform.core.telemetry.read.MetricObservationRow;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.time.Instant;
import java.util.UUID;

/**
 * Bounded, read-only per-entity metric observation API.
 * Authorization: nested entity GET → ENTITY_READ on the entity resource id
 * (see {@code AuthorizationRouteMapper}); tenant path id is a selector, not proof.
 */
@RestController
@RequestMapping("/api/v1/tenants/{tenantId}/entities/{entityId}/metric-observations")
public class EntityMetricObservationController {

    private final EntityMetricObservationService service;

    public EntityMetricObservationController(EntityMetricObservationService service) {
        this.service = service;
    }

    @GetMapping
    public MetricObservationResponse list(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @RequestParam String metricId,
            @RequestParam String valueKind,
            @RequestParam Instant from,
            @RequestParam Instant to,
            @RequestParam(required = false) Integer limit
    ) {
        MetricObservationQuery query = MetricObservationQuery.parse(
                tenantId, entityId, metricId, valueKind, from, to, limit);
        EntityMetricObservationResult result = service.read(query, Instant.now());
        return toResponse(result);
    }

    private static MetricObservationResponse toResponse(EntityMetricObservationResult result) {
        MetricObservationQuery query = result.query();
        return new MetricObservationResponse(
                query.tenantId(),
                query.entityId(),
                query.selector().metricId(),
                query.selector().valueKind().name(),
                query.fromInclusive().toString(),
                query.toExclusive().toString(),
                query.limit(),
                result.observations().size(),
                result.observationState().name(),
                result.freshness().name(),
                result.qualityNote(),
                new Provenance(
                        result.provenance().sourceView(),
                        result.provenance().ordering(),
                        result.provenance().provenanceClass()
                ),
                result.observations().stream().map(EntityMetricObservationController::toItem).toList()
        );
    }

    private static ObservationItem toItem(MetricObservationRow row) {
        if (row.counterValue() != null) {
            return new ObservationItem(
                    row.timestamp().toString(),
                    null,
                    row.counterValue(),
                    row.quality(),
                    row.flags(),
                    row.sensorId(),
                    row.sequenceId(),
                    row.sourceProtocol()
            );
        }
        return new ObservationItem(
                row.timestamp().toString(),
                row.numericValue(),
                null,
                row.quality(),
                row.flags(),
                row.sensorId(),
                row.sequenceId(),
                row.sourceProtocol()
        );
    }
}
