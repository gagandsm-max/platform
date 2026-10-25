package com.platform.core.ingestion.icmp;

import java.time.Instant;
import java.util.List;
import java.util.Objects;
import java.util.Optional;
import java.util.OptionalInt;

/**
 * Source-faithful BUILD-020 / PR #217 {@code icmpintegration.Evidence} domain.
 *
 * <p>Wire field {@code requestedSamples} is the sent probe count. Packet loss and
 * RTT aggregates are nullable and omitted for collection failures / unreachable
 * transmission-unknown cases — never replaced with dummy numeric DOWN.
 */
public record IcmpEvidence(
        int schemaVersion,
        String target,
        int ipVersion,
        String source,
        Instant startedAt,
        Instant completedAt,
        Integer requestedSamples,
        int received,
        Double packetLossPct,
        Long minRttMicros,
        Long avgRttMicros,
        Long maxRttMicros,
        List<IcmpSample> samples,
        String errorCode
) {
    public IcmpEvidence {
        Objects.requireNonNull(target, "target");
        Objects.requireNonNull(source, "source");
        Objects.requireNonNull(startedAt, "startedAt");
        Objects.requireNonNull(completedAt, "completedAt");
        samples = samples == null ? List.of() : List.copyOf(samples);
    }

    /** Sent probe count when present on the wire ({@code requestedSamples}). */
    public OptionalInt sent() {
        return requestedSamples == null ? OptionalInt.empty() : OptionalInt.of(requestedSamples);
    }

    public Optional<Double> packetLossPctOptional() {
        return Optional.ofNullable(packetLossPct);
    }

    public Optional<Long> minRttMicrosOptional() {
        return Optional.ofNullable(minRttMicros);
    }

    public Optional<Long> avgRttMicrosOptional() {
        return Optional.ofNullable(avgRttMicros);
    }

    public Optional<Long> maxRttMicrosOptional() {
        return Optional.ofNullable(maxRttMicros);
    }

    public Optional<String> errorCodeOptional() {
        return Optional.ofNullable(errorCode);
    }
}
