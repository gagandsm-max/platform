package com.platform.core.sensorcontrol.pollgrant;

import java.time.Instant;
import java.util.Objects;

/**
 * Outcome of Issue #229 G1b read-only grant eligibility evaluation for <em>new</em>
 * execution authorization consideration.
 *
 * <p>This is not companion-wire delivery, typed admission, historical capture proof,
 * or G1B_RUNTIME activation. Denial reasons that would disclose cross-scope existence
 * collapse to {@link Decision#NOT_VISIBLE}.
 */
public record SensorPollGrantEligibility(
        Decision decision,
        SensorPollGrant grant,
        Instant evaluatedAt
) {
    public SensorPollGrantEligibility {
        Objects.requireNonNull(decision, "decision");
        if (decision == Decision.ELIGIBLE) {
            Objects.requireNonNull(grant, "grant");
            Objects.requireNonNull(evaluatedAt, "evaluatedAt");
            if (grant.status() != SensorPollGrant.Status.ISSUED) {
                throw new IllegalArgumentException("ELIGIBLE requires an ISSUED grant snapshot");
            }
        } else if (grant != null) {
            throw new IllegalArgumentException("Denied outcomes must not carry a grant snapshot");
        }
    }

    public static SensorPollGrantEligibility eligible(SensorPollGrant grant, Instant evaluatedAt) {
        return new SensorPollGrantEligibility(Decision.ELIGIBLE, grant, evaluatedAt);
    }

    public static SensorPollGrantEligibility denied(Decision decision) {
        if (decision == null || decision == Decision.ELIGIBLE) {
            throw new IllegalArgumentException("Denied decision required");
        }
        return new SensorPollGrantEligibility(decision, null, null);
    }

    public boolean eligible() {
        return decision == Decision.ELIGIBLE;
    }

    /**
     * Eligibility decision for Core-side new-execution consideration under V015 fields
     * and authenticated Sensor identity. Does not encode slot, incarnation, companion
     * cursor, or offline historical capture authority.
     */
    public enum Decision {
        /** Persisted ISSUED grant matches authenticated scope, window and optional bindings. */
        ELIGIBLE,
        /**
         * Missing grant or wrong tenant/Sensor scope. Deliberately nondisclosing relative
         * to cross-tenant probes.
         */
        NOT_VISIBLE,
        /** Locked principal is not ACTIVE. */
        PRINCIPAL_INACTIVE,
        /** Matched credential is not currently usable for this caller. */
        CREDENTIAL_INACTIVE,
        /** Grant entity is outside the authenticated Sensor entity scope. */
        ENTITY_OUT_OF_SCOPE,
        /** Grant exists in scope but is REVOKED or SUPERSEDED. */
        NOT_ISSUED,
        /**
         * Core evaluation time is outside the grant's persisted
         * {@code [window_start, window_end]} (inclusive). No extra grace is applied.
         */
        WINDOW_INACTIVE,
        /**
         * Optional Core-side expected binding does not match the persisted immutable
         * issuance snapshot. Never treat caller-supplied values as issuance proof.
         */
        BINDING_MISMATCH
    }
}
