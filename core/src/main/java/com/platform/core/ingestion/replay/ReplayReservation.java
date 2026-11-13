package com.platform.core.ingestion.replay;

import com.platform.core.ingestion.agentingress.PendingCanonicalBindingV1;

/**
 * Result of reserving one strictly sequential Sensor telemetry batch.
 * PostgreSQL keeps at most one pending reservation per tenant/Sensor.
 */
public record ReplayReservation(
        Status status,
        long ackedThroughSequence,
        String rejectionCode,
        String rejectionMessage,
        java.util.UUID terminalDispositionId,
        long settledThroughSequence,
        java.util.UUID pendingReservationId,
        String pendingAuthorityState,
        java.util.List<PendingCanonicalBindingV1> pendingBindings,
        String pendingCanonicalBatchSha256
) {
    public enum Status {
        RESERVED,
        RETRY_PENDING,
        ALREADY_ACKED,
        TERMINALLY_REJECTED,
        REJECTED
    }

    public static ReplayReservation reserved(long ackedThroughSequence) {
        return reserved(ackedThroughSequence, null);
    }

    public static ReplayReservation reserved(long ackedThroughSequence, java.util.UUID pendingReservationId) {
        return new ReplayReservation(Status.RESERVED, ackedThroughSequence, null, null, null, 0,
                pendingReservationId, "UNVALIDATED", null, null);
    }

    public static ReplayReservation retryPending(long ackedThroughSequence) {
        return retryPending(ackedThroughSequence, null);
    }

    public static ReplayReservation retryPending(long ackedThroughSequence, java.util.UUID pendingReservationId) {
        return new ReplayReservation(Status.RETRY_PENDING, ackedThroughSequence, null, null, null, 0,
                pendingReservationId, "UNVALIDATED", null, null);
    }

    public static ReplayReservation retryPending(long ackedThroughSequence,
            java.util.UUID pendingReservationId, String authorityState,
            java.util.List<PendingCanonicalBindingV1> bindings, String canonicalBatchSha256) {
        return new ReplayReservation(Status.RETRY_PENDING, ackedThroughSequence, null, null, null, 0,
                pendingReservationId, authorityState,
                bindings == null ? null : java.util.List.copyOf(bindings), canonicalBatchSha256);
    }

    public static ReplayReservation alreadyAcked(long ackedThroughSequence) {
        return new ReplayReservation(Status.ALREADY_ACKED, ackedThroughSequence, null, null, null, 0, null, null, null, null);
    }

    public static ReplayReservation rejected(long ackedThroughSequence, String code, String message) {
        return new ReplayReservation(Status.REJECTED, ackedThroughSequence, code, message, null, 0, null, null, null, null);
    }

    public static ReplayReservation terminallyRejected(long ackedThroughSequence,
            long settledThroughSequence, java.util.UUID dispositionId, String reasonCode) {
        return new ReplayReservation(Status.TERMINALLY_REJECTED, ackedThroughSequence,
                reasonCode, null, dispositionId, settledThroughSequence, null, null, null, null);
    }
}
