package com.platform.core.security.audit;

import java.util.Objects;
import java.util.regex.Pattern;

/** Machine-safe reason code. Not an exception message or stack trace. */
public record AuditReasonCode(String value) {
    private static final Pattern SAFE = Pattern.compile("[A-Z][A-Z0-9_]*");

    public AuditReasonCode {
        Objects.requireNonNull(value, "value");
        if (value.isBlank()
                || value.length() > AuditLimits.REASON_CODE_MAX
                || !value.equals(value.strip())
                || !SAFE.matcher(value).matches()) {
            throw new IllegalArgumentException("reasonCode is invalid");
        }
    }

    @Override
    public String toString() {
        return value;
    }
}
