package com.platform.core.ingestion.icmp;

import java.util.Objects;

/**
 * Source-faithful Sensor {@code observation.JobCorrelation} claim fields.
 * Slice A preserves them as payload provenance only — Issue #229 owns
 * Core-issued grant authority before any future production admission.
 */
public record IcmpJobCorrelation(
        String jobId,
        String assignmentKey,
        long assignmentVersion,
        long attempt
) {
    public IcmpJobCorrelation {
        Objects.requireNonNull(jobId, "jobId");
        Objects.requireNonNull(assignmentKey, "assignmentKey");
    }
}
