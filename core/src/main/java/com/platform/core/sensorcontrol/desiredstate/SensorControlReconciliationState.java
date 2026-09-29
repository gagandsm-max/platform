package com.platform.core.sensorcontrol.desiredstate;

/**
 * Derived reconciliation view — not a persisted column.
 */
public enum SensorControlReconciliationState {
    CURRENT,
    REJECTED,
    PENDING;

    public static SensorControlReconciliationState derive(SensorControlState state) {
        if (state == null) {
            return PENDING;
        }
        // Latest observed REJECTED for the exact desired generation/checksum wins over
        // historical applied success (restart-runtime re-proof may reject while applied*).
        if (state.observedGeneration() == state.desiredGeneration()
                && state.observedOutcome() == SensorControlObservationOutcome.REJECTED
                && state.desiredChecksum() != null
                && state.desiredChecksum().equals(state.observedChecksum())) {
            return REJECTED;
        }
        if (state.appliedGeneration() == state.desiredGeneration()
                && state.desiredChecksum() != null
                && state.desiredChecksum().equals(state.appliedChecksum())) {
            return CURRENT;
        }
        return PENDING;
    }
}
