package com.platform.core.domain.capabilitypack;

import java.util.Objects;
import java.util.Optional;

public record NormalizationTransform(String type, Optional<Double> factor, Optional<Double> offset) {
    public NormalizationTransform {
        Objects.requireNonNull(type, "type");
        factor = factor == null ? Optional.empty() : factor;
        offset = offset == null ? Optional.empty() : offset;
    }
}
