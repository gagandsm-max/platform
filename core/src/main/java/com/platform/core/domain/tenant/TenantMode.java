package com.platform.core.domain.tenant;

public enum TenantMode {
    SINGLE,
    MSP_MASTER,
    MSP_CHILD;

    public static TenantMode from(String raw) {
        try {
            return TenantMode.valueOf(raw.trim().toUpperCase());
        } catch (Exception e) {
            throw new com.platform.core.domain.DomainException("VALIDATION_FAILED", "mode must be SINGLE, MSP_MASTER, or MSP_CHILD");
        }
    }
}
