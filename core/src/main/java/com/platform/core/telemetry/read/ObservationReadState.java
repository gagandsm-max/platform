package com.platform.core.telemetry.read;

/**
 * Server-derived semantic state for a bounded per-entity metric read.
 * Missing/stale/degraded monitoring evidence must never collapse to DEVICE_DOWN.
 */
public enum ObservationReadState {
    OK,
    UNKNOWN,
    STALE,
    MONITORING_DEGRADED,
    NO_DATA,
    SOURCE_UNAVAILABLE
}
