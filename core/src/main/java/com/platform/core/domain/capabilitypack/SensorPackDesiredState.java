package com.platform.core.domain.capabilitypack;

import java.util.List;
import java.util.Objects;

/** Sensor pull snapshot of desired Capability Pack assignments. */
public record SensorPackDesiredState(
        long generation,
        String checksum,
        long appliedGeneration,
        String appliedChecksum,
        List<DesiredPackEntry> packs
) {
    public SensorPackDesiredState {
        Objects.requireNonNull(packs, "packs");
        packs = List.copyOf(packs);
        if (generation < 0 || appliedGeneration < 0) {
            throw new IllegalArgumentException("generation values must be >= 0");
        }
        if (appliedGeneration > generation) {
            throw new IllegalArgumentException("appliedGeneration must not exceed desired generation");
        }
    }

    public static SensorPackDesiredState empty() {
        return new SensorPackDesiredState(0L, null, 0L, null, List.of());
    }
}
