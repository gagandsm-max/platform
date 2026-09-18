package com.platform.core.domain.capabilitypack;

import java.util.Objects;
import java.util.Optional;

public record PackReference(
        String id,
        String kind,
        Optional<String> version,
        Optional<String> packagePath,
        Optional<String> sourceReference,
        String provenanceId
) {
    public PackReference {
        Objects.requireNonNull(id, "id");
        Objects.requireNonNull(kind, "kind");
        Objects.requireNonNull(provenanceId, "provenanceId");
        version = version == null ? Optional.empty() : version;
        packagePath = packagePath == null ? Optional.empty() : packagePath;
        sourceReference = sourceReference == null ? Optional.empty() : sourceReference;
    }
}
