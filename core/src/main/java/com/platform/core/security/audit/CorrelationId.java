package com.platform.core.security.audit;

import java.util.Objects;
import java.util.regex.Pattern;

/**
 * Correlation identifier matching Core {@code X-Correlation-ID} grammar
 * ({@code CorrelationIdFilter}: max 128, {@code [A-Za-z0-9._:-]+}).
 * Independent of audit event identity.
 */
public record CorrelationId(String value) {
    private static final Pattern SAFE = Pattern.compile("[A-Za-z0-9._:-]+");

    public CorrelationId {
        Objects.requireNonNull(value, "value");
        if (value.isBlank()
                || value.length() > AuditLimits.CORRELATION_ID_MAX
                || !value.equals(value.strip())
                || !SAFE.matcher(value).matches()) {
            throw new IllegalArgumentException("correlationId is invalid");
        }
    }

    @Override
    public String toString() {
        return value;
    }
}
