package com.platform.core.domain.capabilitypack;

import java.util.Objects;
import java.util.Optional;

public record PackProvenance(
        String id,
        String sourceType,
        String sourceReference,
        Optional<String> license,
        String redistribution
) {
    public PackProvenance {
        Objects.requireNonNull(id, "id");
        Objects.requireNonNull(sourceType, "sourceType");
        Objects.requireNonNull(sourceReference, "sourceReference");
        Objects.requireNonNull(redistribution, "redistribution");
        license = license == null ? Optional.empty() : license;
    }
}
