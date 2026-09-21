package com.platform.core.domain.recurring;

import java.util.List;
import java.util.Objects;

/** Sensor pull snapshot of desired recurring schedule assignments. */
public record SensorRecurringDesiredState(
        long generation,
        String checksum,
        long appliedGeneration,
        String appliedChecksum,
        List<DesiredRecurringDefinition> definitions
) {
    public SensorRecurringDesiredState {
        Objects.requireNonNull(definitions, "definitions");
        definitions = List.copyOf(definitions);
        if (generation < 0 || appliedGeneration < 0) {
            throw new IllegalArgumentException("generation values must be >= 0");
        }
        if (appliedGeneration > generation) {
            throw new IllegalArgumentException("appliedGeneration must not exceed desired generation");
        }
    }

    public static SensorRecurringDesiredState empty() {
        return new SensorRecurringDesiredState(0L, null, 0L, null, List.of());
    }
}
