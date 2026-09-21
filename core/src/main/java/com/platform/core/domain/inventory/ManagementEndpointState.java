package com.platform.core.domain.inventory;

import com.platform.core.domain.DomainException;

public enum ManagementEndpointState {
    ACTIVE,
    INACTIVE;

    public static ManagementEndpointState from(String raw) {
        if (raw == null || raw.isBlank()) {
            throw new DomainException("VALIDATION_FAILED", "state is required");
        }
        try {
            return ManagementEndpointState.valueOf(raw.trim().toUpperCase());
        } catch (IllegalArgumentException ex) {
            throw new DomainException("VALIDATION_FAILED", "state must be ACTIVE or INACTIVE");
        }
    }
}
