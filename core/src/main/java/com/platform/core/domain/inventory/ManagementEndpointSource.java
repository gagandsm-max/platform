package com.platform.core.domain.inventory;

import com.platform.core.domain.DomainException;

/**
 * Provenance of a management endpoint. DISCOVERY/PROVISIONING require an explicit
 * governed create call — discovery observations alone never become authority.
 */
public enum ManagementEndpointSource {
    MANUAL,
    DISCOVERY,
    PROVISIONING;

    public static ManagementEndpointSource from(String raw) {
        if (raw == null || raw.isBlank()) {
            return MANUAL;
        }
        try {
            return ManagementEndpointSource.valueOf(raw.trim().toUpperCase());
        } catch (IllegalArgumentException ex) {
            throw new DomainException("VALIDATION_FAILED", "source must be MANUAL, DISCOVERY, or PROVISIONING");
        }
    }
}
