package com.platform.core.telemetry.read;

/**
 * Freshness context for a bounded observation-window read.
 * {@link #HISTORICAL} means the query window ended outside the live horizon —
 * valid history must not be labelled current-monitoring {@link #STALE} or blanket {@link #FRESH}.
 */
public enum FreshnessClass {
    FRESH,
    STALE,
    HISTORICAL,
    MISSING,
    UNKNOWN
}
