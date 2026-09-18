package com.platform.core.domain.capabilitypack;

import java.util.Objects;
import java.util.Optional;

public record NormalizationMapping(
        String key,
        NormalizationSource source,
        String valueType,
        Optional<String> unit,
        NormalizationTarget target,
        String qualityFallback,
        String provenanceId,
        Optional<NormalizationTransform> transform
) {
    public NormalizationMapping {
        Objects.requireNonNull(key, "key");
        Objects.requireNonNull(source, "source");
        Objects.requireNonNull(valueType, "valueType");
        Objects.requireNonNull(target, "target");
        Objects.requireNonNull(qualityFallback, "qualityFallback");
        Objects.requireNonNull(provenanceId, "provenanceId");
        unit = unit == null ? Optional.empty() : unit;
        transform = transform == null ? Optional.empty() : transform;
    }
}
