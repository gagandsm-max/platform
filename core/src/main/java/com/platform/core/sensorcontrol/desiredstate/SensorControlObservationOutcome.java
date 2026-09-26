package com.platform.core.sensorcontrol.desiredstate;

import com.platform.core.domain.DomainException;

public enum SensorControlObservationOutcome {
    APPLIED,
    REJECTED;

    /**
     * Exact enum-name match only ({@code APPLIED} / {@code REJECTED}).
     * Invalid values raise {@code VALIDATION_FAILED} (HTTP 400), never an unchecked 500 path.
     */
    public static SensorControlObservationOutcome parse(String raw) {
        if (raw == null || raw.isBlank()) {
            throw new DomainException("VALIDATION_FAILED", "outcome is required");
        }
        try {
            return SensorControlObservationOutcome.valueOf(raw);
        } catch (IllegalArgumentException ex) {
            throw new DomainException("VALIDATION_FAILED", "outcome must be APPLIED or REJECTED");
        }
    }
}
