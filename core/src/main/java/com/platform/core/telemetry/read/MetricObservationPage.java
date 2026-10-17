package com.platform.core.telemetry.read;

import java.util.List;
import java.util.Objects;

/**
 * Bounded observation page after latest-N+1 selection and presentation reorder.
 * {@code truncated} is true when at least one older observation was excluded by the limit.
 */
public record MetricObservationPage(
        List<MetricObservationRow> observations,
        boolean truncated
) {
    public MetricObservationPage {
        Objects.requireNonNull(observations, "observations");
        observations = List.copyOf(observations);
    }

    public static MetricObservationPage empty() {
        return new MetricObservationPage(List.of(), false);
    }
}
