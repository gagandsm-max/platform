package com.platform.core.domain.inventory;

import com.platform.core.domain.DomainException;

public enum ManagementEndpointAddressType {
    IPV4,
    IPV6,
    FQDN;

    public static ManagementEndpointAddressType from(String raw) {
        if (raw == null || raw.isBlank()) {
            throw new DomainException("VALIDATION_FAILED", "addressType is required");
        }
        try {
            return ManagementEndpointAddressType.valueOf(raw.trim().toUpperCase());
        } catch (IllegalArgumentException ex) {
            throw new DomainException("VALIDATION_FAILED", "addressType must be IPV4, IPV6, or FQDN");
        }
    }
}
