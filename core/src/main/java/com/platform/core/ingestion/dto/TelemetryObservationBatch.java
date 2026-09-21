package com.platform.core.ingestion.dto;

import java.util.List;
import java.util.Objects;

/**
 * Mixed Sensor upload batch: ordinary float numeric gauges plus exact Counter64 observations.
 * Both lists publish inside one Kafka transaction (PR #81 durability unchanged).
 */
public record TelemetryObservationBatch(
        List<CanonicalObservation> numericObservations,
        List<CanonicalCounterObservation> counterObservations
) {
    public TelemetryObservationBatch {
        numericObservations = List.copyOf(Objects.requireNonNullElse(numericObservations, List.of()));
        counterObservations = List.copyOf(Objects.requireNonNullElse(counterObservations, List.of()));
    }

    public static TelemetryObservationBatch ofNumeric(List<CanonicalObservation> numeric) {
        return new TelemetryObservationBatch(numeric, List.of());
    }

    public boolean isEmpty() {
        return numericObservations.isEmpty() && counterObservations.isEmpty();
    }

    public int size() {
        return numericObservations.size() + counterObservations.size();
    }
}
