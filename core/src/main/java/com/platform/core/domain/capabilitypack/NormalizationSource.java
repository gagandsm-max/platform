package com.platform.core.domain.capabilitypack;

import java.util.Objects;
import java.util.Optional;

public record NormalizationSource(String protocol, String field, Optional<String> referenceId) {
    public NormalizationSource {
        Objects.requireNonNull(protocol, "protocol");
        Objects.requireNonNull(field, "field");
        referenceId = referenceId == null ? Optional.empty() : referenceId;
    }
}
