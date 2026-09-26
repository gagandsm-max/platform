package com.platform.core.ingestion.replay;

import java.util.UUID;

/**
 * Result of claiming exclusive publication ownership for one exact pending reservation.
 */
public record PublisherClaimResult(
        Status status,
        UUID ownerToken,
        long publisherEpoch,
        String rejectionCode,
        String rejectionMessage
) {
    public enum Status {
        CLAIMED,
        BUSY,
        REJECTED
    }

    public static PublisherClaimResult claimed(UUID ownerToken, long publisherEpoch) {
        return new PublisherClaimResult(Status.CLAIMED, ownerToken, publisherEpoch, null, null);
    }

    public static PublisherClaimResult busy() {
        return new PublisherClaimResult(
                Status.BUSY,
                null,
                0,
                "ERR_BATCH_PUBLICATION_IN_PROGRESS",
                "Exact pending batch publication is already leased by another publisher"
        );
    }

    public static PublisherClaimResult rejected(String code, String message) {
        return new PublisherClaimResult(Status.REJECTED, null, 0, code, message);
    }
}
