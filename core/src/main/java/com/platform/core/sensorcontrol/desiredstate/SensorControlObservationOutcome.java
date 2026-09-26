package com.platform.core.sensorcontrol.desiredstate;

public enum SensorControlObservationOutcome {
    APPLIED,
    REJECTED;

    public static SensorControlObservationOutcome parse(String raw) {
        if (raw == null || raw.isBlank()) {
            throw new IllegalArgumentException("outcome is required");
        }
        try {
            return SensorControlObservationOutcome.valueOf(raw.trim());
        } catch (IllegalArgumentException ex) {
            throw new IllegalArgumentException("outcome must be APPLIED or REJECTED");
        }
    }
}
