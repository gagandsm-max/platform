package com.platform.core.domain.capabilitypack;

import java.util.Objects;

public record PackDependency(String packId, String packVersion) {
    public PackDependency {
        Objects.requireNonNull(packId, "packId");
        Objects.requireNonNull(packVersion, "packVersion");
    }

    public String identityKey() {
        return packId + "@" + packVersion;
    }
}
