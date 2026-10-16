package com.platform.core.telemetry.read;

import com.platform.core.domain.inventory.EntityService;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.time.Instant;
import java.util.List;

/**
 * Per-entity metric observation read orchestration.
 * Proves parent entity existence under tenant scope before querying ClickHouse.
 */
@Service
public class EntityMetricObservationService {

    private final EntityService entities;
    private final ClickHouseMetricObservationReader reader;
    private final Duration freshnessThreshold;

    public EntityMetricObservationService(
            EntityService entities,
            ClickHouseMetricObservationReader reader,
            @Value("${platform.telemetry.read.freshness-threshold-seconds:300}") long freshnessSeconds
    ) {
        this.entities = entities;
        this.reader = reader;
        this.freshnessThreshold = Duration.ofSeconds(Math.max(1, freshnessSeconds));
    }

    public EntityMetricObservationResult read(MetricObservationQuery query, Instant evaluatedAt) {
        // Tenant+entity existence (404 ENTITY_NOT_FOUND). Authorization is filter-enforced separately.
        entities.get(query.tenantId(), query.entityId());
        List<MetricObservationRow> rows = reader.read(query);
        return classify(query, rows, evaluatedAt == null ? Instant.now() : evaluatedAt);
    }

    EntityMetricObservationResult classify(
            MetricObservationQuery query,
            List<MetricObservationRow> rows,
            Instant evaluatedAt
    ) {
        String view = query.selector().valueKind() == MetricValueKind.COUNTER64
                ? "platform.metric_counter"
                : "platform.metric_numeric";
        MetricObservationProvenance provenance = new MetricObservationProvenance(
                view,
                "ts ASC, sequence_id ASC",
                "clickhouse-canonical-final-view"
        );

        if (rows == null || rows.isEmpty()) {
            return new EntityMetricObservationResult(
                    query,
                    ObservationReadState.UNKNOWN,
                    FreshnessClass.MISSING,
                    List.of(),
                    provenance,
                    "No observations in the requested window; not DEVICE_DOWN."
            );
        }

        Instant newest = rows.get(rows.size() - 1).timestamp();
        Duration age = Duration.between(newest, evaluatedAt);
        boolean stale = age.compareTo(freshnessThreshold) > 0;
        boolean degradedQuality = rows.stream().anyMatch(r -> r.quality() == 2);

        ObservationReadState state;
        FreshnessClass freshness;
        String note;
        if (stale && degradedQuality) {
            state = ObservationReadState.MONITORING_DEGRADED;
            freshness = FreshnessClass.STALE;
            note = "Stale observations with degraded quality; not DEVICE_DOWN.";
        } else if (stale) {
            state = ObservationReadState.STALE;
            freshness = FreshnessClass.STALE;
            note = "Newest observation exceeds freshness threshold; not DEVICE_DOWN.";
        } else if (degradedQuality) {
            state = ObservationReadState.MONITORING_DEGRADED;
            freshness = FreshnessClass.FRESH;
            note = "Fresh observations with degraded quality; not DEVICE_DOWN.";
        } else {
            state = ObservationReadState.OK;
            freshness = FreshnessClass.FRESH;
            note = "Bounded observations returned from canonical view.";
        }

        return new EntityMetricObservationResult(query, state, freshness, List.copyOf(rows), provenance, note);
    }

    public record MetricObservationProvenance(
            String sourceView,
            String ordering,
            String provenanceClass
    ) {
    }

    public record EntityMetricObservationResult(
            MetricObservationQuery query,
            ObservationReadState observationState,
            FreshnessClass freshness,
            List<MetricObservationRow> observations,
            MetricObservationProvenance provenance,
            String qualityNote
    ) {
    }
}
