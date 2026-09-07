package com.platform.core.domain.inventory;

public enum LifecycleState {
    DISCOVERED,
    ACTIVE,
    QUIESCED,
    RETIRED;

    public static LifecycleState from(String raw) {
        try {
            return LifecycleState.valueOf(raw.trim().toUpperCase());
        } catch (Exception e) {
            throw new com.platform.core.domain.DomainException("VALIDATION_FAILED", "lifecycleState is invalid");
        }
    }
}
