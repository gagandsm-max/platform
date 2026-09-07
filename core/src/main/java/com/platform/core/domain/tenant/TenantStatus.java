package com.platform.core.domain.tenant;

public enum TenantStatus {
    ACTIVE,
    SUSPENDED,
    RETIRED;

    public static TenantStatus from(String raw) {
        try {
            return TenantStatus.valueOf(raw.trim().toUpperCase());
        } catch (Exception e) {
            throw new com.platform.core.domain.DomainException("VALIDATION_FAILED", "status must be ACTIVE, SUSPENDED, or RETIRED");
        }
    }
}
