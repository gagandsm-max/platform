package com.platform.core.domain.capabilitypack;

import java.util.List;
import java.util.Objects;
import java.util.Optional;

public record PackCapability(String id, Optional<String> description, List<String> requiresProtocols) {
    public PackCapability {
        Objects.requireNonNull(id, "id");
        description = description == null ? Optional.empty() : description;
        requiresProtocols = requiresProtocols == null ? List.of() : List.copyOf(requiresProtocols);
    }
}
