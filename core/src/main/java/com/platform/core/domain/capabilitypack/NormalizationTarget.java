package com.platform.core.domain.capabilitypack;

import java.util.Objects;
import java.util.Optional;

public record NormalizationTarget(String entityType, Optional<String> component) {
    public NormalizationTarget {
        Objects.requireNonNull(entityType, "entityType");
        component = component == null ? Optional.empty() : component;
    }
}
