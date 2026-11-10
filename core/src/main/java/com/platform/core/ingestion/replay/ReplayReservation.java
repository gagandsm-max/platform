package com.platform.core.ingestion.replay;

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
        java.util.UUID pendingReservationId
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
                pendingReservationId);
    }

    public static ReplayReservation retryPending(long ackedThroughSequence) {
        return retryPending(ackedThroughSequence, null);
    }

    public static ReplayReservation retryPending(long ackedThroughSequence, java.util.UUID pendingReservationId) {
        return new ReplayReservation(Status.RETRY_PENDING, ackedThroughSequence, null, null, null, 0,
                pendingReservationId);
    }

    public static ReplayReservation alreadyAcked(long ackedThroughSequence) {
        return new ReplayReservation(Status.ALREADY_ACKED, ackedThroughSequence, null, null, null, 0, null);
    }

    public static ReplayReservation rejected(long ackedThroughSequence, String code, String message) {
        return new ReplayReservation(Status.REJECTED, ackedThroughSequence, code, message, null, 0, null);
    }

    public static ReplayReservation terminallyRejected(long ackedThroughSequence,
            long settledThroughSequence, java.util.UUID dispositionId, String reasonCode) {
        return new ReplayReservation(Status.TERMINALLY_REJECTED, ackedThroughSequence,
                reasonCode, null, dispositionId, settledThroughSequence, null);
    }
}
