package com.platform.core.security.audit;

import java.util.Objects;
import java.util.regex.Pattern;

public record AuditResourceType(String value) {
    private static final Pattern SAFE = Pattern.compile("[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*");

    public AuditResourceType {
        Objects.requireNonNull(value, "value");
        if (value.isBlank() || value.length() > AuditLimits.RESOURCE_TYPE_MAX || !SAFE.matcher(value).matches()) {
            throw new IllegalArgumentException("resourceType is invalid");
        }
    }

    @Override
    public String toString() { return value; }
}
