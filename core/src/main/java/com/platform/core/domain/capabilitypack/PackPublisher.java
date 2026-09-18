package com.platform.core.domain.capabilitypack;

import java.util.Objects;

public record PackPublisher(String type, String name) {
    public PackPublisher {
        Objects.requireNonNull(type, "type");
        Objects.requireNonNull(name, "name");
    }
}
