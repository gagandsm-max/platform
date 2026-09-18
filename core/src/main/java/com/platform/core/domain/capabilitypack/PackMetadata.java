package com.platform.core.domain.capabilitypack;

import java.util.Optional;

public record PackMetadata(
        Optional<String> vendor,
        Optional<String> family,
        Optional<String> product,
        Optional<String> description
) {
    public PackMetadata {
        vendor = vendor == null ? Optional.empty() : vendor;
        family = family == null ? Optional.empty() : family;
        product = product == null ? Optional.empty() : product;
        description = description == null ? Optional.empty() : description;
    }

    public static PackMetadata empty() {
        return new PackMetadata(Optional.empty(), Optional.empty(), Optional.empty(), Optional.empty());
    }
}
