package com.platform.core.telemetry.read;

/**
 * Distinguishes Float64 numeric observations from exact Counter64 UInt64 observations.
 * The kinds must never be collapsed: Counter64 must remain unsigned/string-safe.
 */
public enum MetricValueKind {
    NUMERIC,
    COUNTER64
}
