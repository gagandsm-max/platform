package com.platform.core.telemetry.read;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.inventory.EntityService;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.time.Instant;
import java.util.List;

/**
 * Per-entity metric observation read orchestration.
 * Proves parent entity existence under tenant scope before querying ClickHouse.
 * Distinguishes historical observation-window freshness from current monitoring liveness.
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
        MetricObservationPage page = reader.read(query);
        return classify(query, page, evaluatedAt == null ? Instant.now() : evaluatedAt);
    }

    EntityMetricObservationResult classify(
            MetricObservationQuery query,
            MetricObservationPage page,
            Instant evaluatedAt
    ) {
        String view = query.selector().valueKind() == MetricValueKind.COUNTER64
                ? "platform.metric_counter"
                : "platform.metric_numeric";
        MetricObservationProvenance provenance = new MetricObservationProvenance(
                view,
                ClickHouseMetricObservationReader.SELECTION_ORDERING
                        + " then present "
                        + ClickHouseMetricObservationReader.PRESENTATION_ORDERING,
                "clickhouse-canonical-final-view"
        );

        List<MetricObservationRow> rows = page == null ? List.of() : page.observations();
        boolean truncated = page != null && page.truncated();

        if (rows.isEmpty()) {
            return new EntityMetricObservationResult(
                    query,
                    ObservationReadState.NO_DATA,
                    FreshnessClass.MISSING,
                    List.of(),
                    truncated,
                    provenance,
                    "No observations in the requested window; NO_DATA, not DEVICE_DOWN."
            );
        }

        for (MetricObservationRow row : rows) {
            int quality = row.quality();
            if (quality != 0 && quality != 1 && quality != 2) {
                throw new DomainException(
                        "DATA_INTEGRITY_ERROR",
                        "Telemetry store returned malformed observation data");
            }
        }

        boolean historicalWindow = isHistoricalWindow(query.toExclusive(), evaluatedAt);
        boolean hasUnknownQuality = rows.stream().anyMatch(r -> r.quality() == 0);
        boolean hasDegradedQuality = rows.stream().anyMatch(r -> r.quality() == 2);
        boolean allGoodQuality = rows.stream().allMatch(r -> r.quality() == 1);

        ObservationReadState state;
        FreshnessClass freshness;
        String note;

        if (historicalWindow) {
            freshness = FreshnessClass.HISTORICAL;
            if (hasDegradedQuality) {
                state = ObservationReadState.MONITORING_DEGRADED;
                note = "Historical observation window with degraded quality; not current-liveness STALE; not DEVICE_DOWN.";
            } else if (hasUnknownQuality) {
                state = ObservationReadState.UNKNOWN;
                note = "Historical observation window with unknown quality; not promoted to healthy; not DEVICE_DOWN.";
            } else if (allGoodQuality) {
                state = ObservationReadState.OK;
                note = "Historical observation window returned; freshness is HISTORICAL, not current-liveness FRESH.";
            } else {
                throw new DomainException(
                        "DATA_INTEGRITY_ERROR",
                        "Telemetry store returned malformed observation data");
            }
        } else {
            Instant newest = rows.get(rows.size() - 1).timestamp();
            Duration age = Duration.between(newest, evaluatedAt);
            boolean stale = age.compareTo(freshnessThreshold) > 0;

            if (stale && hasDegradedQuality) {
                state = ObservationReadState.MONITORING_DEGRADED;
                freshness = FreshnessClass.STALE;
                note = "Stale observations with degraded quality; not DEVICE_DOWN.";
            } else if (stale && hasUnknownQuality) {
                state = ObservationReadState.UNKNOWN;
                freshness = FreshnessClass.STALE;
                note = "Stale observations with unknown quality; not promoted to healthy; not DEVICE_DOWN.";
            } else if (stale && allGoodQuality) {
                state = ObservationReadState.STALE;
                freshness = FreshnessClass.STALE;
                note = "Newest observation exceeds freshness threshold; not DEVICE_DOWN.";
            } else if (hasDegradedQuality) {
                state = ObservationReadState.MONITORING_DEGRADED;
                freshness = FreshnessClass.FRESH;
                note = "Fresh observations with degraded quality; not DEVICE_DOWN.";
            } else if (hasUnknownQuality) {
                state = ObservationReadState.UNKNOWN;
                freshness = FreshnessClass.FRESH;
                note = "Fresh observations with unknown quality; not promoted to healthy; not DEVICE_DOWN.";
            } else if (allGoodQuality) {
                state = ObservationReadState.OK;
                freshness = FreshnessClass.FRESH;
                note = "Bounded observations returned from canonical view.";
            } else {
                throw new DomainException(
                        "DATA_INTEGRITY_ERROR",
                        "Telemetry store returned malformed observation data");
            }
        }

        if (truncated) {
            note = note + " Result truncated to latest " + query.limit() + " observations.";
        }

        return new EntityMetricObservationResult(
                query, state, freshness, List.copyOf(rows), truncated, provenance, note);
    }

    /**
     * A window whose exclusive end is entirely before the live freshness horizon is historical:
     * wall-clock advance alone must not reclassify valid history as current-monitoring STALE.
     */
    boolean isHistoricalWindow(Instant toExclusive, Instant evaluatedAt) {
        Instant liveHorizonStart = evaluatedAt.minus(freshnessThreshold);
        return toExclusive.isBefore(liveHorizonStart) || toExclusive.equals(liveHorizonStart);
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
            boolean truncated,
            MetricObservationProvenance provenance,
            String qualityNote
    ) {
    }
}
