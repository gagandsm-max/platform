package com.platform.core.telemetry.read;

import com.platform.core.domain.DomainException;

import java.time.Duration;
import java.time.Instant;
import java.util.Locale;
import java.util.Objects;
import java.util.UUID;

/**
 * Validated, bounded read request. All bounds are enforced server-side before any CH call.
 */
public record MetricObservationQuery(
        UUID tenantId,
        UUID entityId,
        AllowedMetricSelector selector,
        Instant fromInclusive,
        Instant toExclusive,
        int limit
) {
    public static final int DEFAULT_LIMIT = 100;
    public static final int MAX_LIMIT = 500;
    public static final Duration MAX_WINDOW = Duration.ofHours(24);

    public MetricObservationQuery {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(entityId, "entityId");
        Objects.requireNonNull(selector, "selector");
        Objects.requireNonNull(fromInclusive, "fromInclusive");
        Objects.requireNonNull(toExclusive, "toExclusive");
        if (!toExclusive.isAfter(fromInclusive)) {
            throw new DomainException("VALIDATION_FAILED", "to must be strictly after from");
        }
        if (Duration.between(fromInclusive, toExclusive).compareTo(MAX_WINDOW) > 0) {
            throw new DomainException("QUERY_LIMIT_EXCEEDED",
                    "time window must be at most " + MAX_WINDOW.toHours() + " hours");
        }
        if (limit < 1 || limit > MAX_LIMIT) {
            throw new DomainException("QUERY_LIMIT_EXCEEDED",
                    "limit must be between 1 and " + MAX_LIMIT);
        }
    }

    public static MetricObservationQuery parse(
            UUID tenantId,
            UUID entityId,
            String metricId,
            String valueKindRaw,
            Instant from,
            Instant to,
            Integer limit
    ) {
        if (from == null || to == null) {
            throw new DomainException("VALIDATION_FAILED", "from and to Instant bounds are required");
        }
        MetricValueKind kind;
        try {
            kind = MetricValueKind.valueOf(valueKindRaw == null ? "" : valueKindRaw.trim().toUpperCase(Locale.ROOT));
        } catch (IllegalArgumentException ex) {
            throw new DomainException("VALIDATION_FAILED", "valueKind must be NUMERIC or COUNTER64");
        }
        AllowedMetricSelector selector = AllowedMetricSelector.find(metricId, kind)
                .orElseThrow(() -> new DomainException("VALIDATION_FAILED",
                        "metricId/valueKind is not an allowlisted Phase-1 selector"));
        int resolvedLimit = limit == null ? DEFAULT_LIMIT : limit;
        return new MetricObservationQuery(tenantId, entityId, selector, from, to, resolvedLimit);
    }
}
