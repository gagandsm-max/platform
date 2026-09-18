package com.platform.core.domain.capabilitypack;

import java.util.List;
import java.util.Objects;
import java.util.Optional;

public record PackProtocol(
        String family,
        String requirement,
        boolean legacy,
        Optional<Integer> suggestedPort,
        List<String> transportModes,
        List<String> securityModes,
        Optional<String> credentialProfileType,
        List<String> prerequisiteCapabilities
) {
    public PackProtocol {
        Objects.requireNonNull(family, "family");
        Objects.requireNonNull(requirement, "requirement");
        suggestedPort = suggestedPort == null ? Optional.empty() : suggestedPort;
        transportModes = transportModes == null ? List.of() : List.copyOf(transportModes);
        securityModes = securityModes == null ? List.of() : List.copyOf(securityModes);
        credentialProfileType = credentialProfileType == null ? Optional.empty() : credentialProfileType;
        prerequisiteCapabilities = prerequisiteCapabilities == null ? List.of() : List.copyOf(prerequisiteCapabilities);
    }
}
