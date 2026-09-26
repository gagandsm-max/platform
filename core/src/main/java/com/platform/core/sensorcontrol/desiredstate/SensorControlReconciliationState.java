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
        if (state.appliedGeneration() == state.desiredGeneration()
                && state.desiredChecksum() != null
                && state.desiredChecksum().equals(state.appliedChecksum())) {
            return CURRENT;
        }
        if (state.observedGeneration() == state.desiredGeneration()
                && state.observedOutcome() == SensorControlObservationOutcome.REJECTED) {
            return REJECTED;
        }
        return PENDING;
    }
}
