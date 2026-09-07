package com.platform.core.security.audit;

import java.util.Objects;
import java.util.regex.Pattern;

/** Canonical lowercase dotted action. Exact equality only; no wildcards or prefix authority. */
public record AuditAction(String value) {
    private static final Pattern SAFE = Pattern.compile("[a-z][a-z0-9]*(?:\\.[a-z][a-z0-9_-]*)+");

    public AuditAction {
        Objects.requireNonNull(value, "value");
        if (value.isBlank()
                || value.length() > AuditLimits.ACTION_MAX
                || !value.equals(value.strip())
                || !SAFE.matcher(value).matches()) {
            throw new IllegalArgumentException("audit action is invalid");
        }
    }

    @Override
    public String toString() {
        return value;
    }
}
