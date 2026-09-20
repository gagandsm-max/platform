package com.platform.core.ingestion.replay;

/**
 * Result of reserving one strictly sequential Sensor telemetry batch.
 * PostgreSQL keeps at most one pending reservation per tenant/Sensor.
 */
public record ReplayReservation(
        Status status,
        long ackedThroughSequence,
        String rejectionCode,
        String rejectionMessage
) {
    public enum Status {
        RESERVED,
        RETRY_PENDING,
        ALREADY_ACKED,
        REJECTED
    }

    public static ReplayReservation reserved(long ackedThroughSequence) {
        return new ReplayReservation(Status.RESERVED, ackedThroughSequence, null, null);
    }

    public static ReplayReservation retryPending(long ackedThroughSequence) {
        return new ReplayReservation(Status.RETRY_PENDING, ackedThroughSequence, null, null);
    }

    public static ReplayReservation alreadyAcked(long ackedThroughSequence) {
        return new ReplayReservation(Status.ALREADY_ACKED, ackedThroughSequence, null, null);
    }

    public static ReplayReservation rejected(long ackedThroughSequence, String code, String message) {
        return new ReplayReservation(Status.REJECTED, ackedThroughSequence, code, message);
    }
}
