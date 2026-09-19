package com.platform.core.domain.capabilitypack;

import java.util.Objects;

/** One pack in a Sensor desired-state snapshot (metadata only; no bytes/credentials). */
public record DesiredPackEntry(
        String packId,
        String packVersion,
        String manifestSha256,
        int artifactBytes
) {
    public DesiredPackEntry {
        Objects.requireNonNull(packId, "packId");
        Objects.requireNonNull(packVersion, "packVersion");
        Objects.requireNonNull(manifestSha256, "manifestSha256");
        if (artifactBytes < 0) {
            throw new IllegalArgumentException("artifactBytes must be >= 0");
        }
    }
}
