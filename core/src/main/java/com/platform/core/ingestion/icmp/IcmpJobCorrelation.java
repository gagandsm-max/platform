package com.platform.core.ingestion.icmp;

import java.math.BigInteger;
import java.util.Objects;

/**
 * Source-faithful Sensor {@code observation.JobCorrelation} claim fields.
 * Slice A preserves them as payload provenance only — Issue #229 owns
 * Core-issued grant authority before any future production admission.
 *
 * <p>{@code assignmentVersion} uses {@link BigInteger} so the exact Go
 * {@code uint64} domain through {@code 18446744073709551615} is preserved
 * without signed-{@code long} truncation. Presence of a value is not
 * authorization.
 */
public record IcmpJobCorrelation(
        String jobId,
        String assignmentKey,
        BigInteger assignmentVersion,
        long attempt
) {
    public IcmpJobCorrelation {
        Objects.requireNonNull(jobId, "jobId");
        Objects.requireNonNull(assignmentKey, "assignmentKey");
        Objects.requireNonNull(assignmentVersion, "assignmentVersion");
    }
}
