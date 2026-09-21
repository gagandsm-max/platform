package com.platform.core.ingestion.dto;

import com.fasterxml.jackson.annotation.JsonFormat;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.math.BigInteger;
import java.time.Instant;
import java.util.Objects;

/**
 * Exact raw Counter64 observation for Kafka {@code telemetry.numeric.v1} and
 * ClickHouse {@code metric_counter_raw}. The unsigned value is serialized as a
 * decimal STRING so JSON/JavaScript Number cannot lose precision.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public record CanonicalCounterObservation(
        @JsonProperty("observationKind") String observationKind,
        String tenantId,
        String entityId,
        String metricId,
        String sensorId,
        @JsonFormat(shape = JsonFormat.Shape.STRING)
        Instant timestamp,
        /** Canonical unsigned decimal string in {@code [0, 2^64-1]}. */
        String counterValue,
        int quality,
        int flags,
        String sequenceId,
        String sourceProtocol
) {
    public static final String KIND = "COUNTER64";
    public static final BigInteger MAX_UINT64 = new BigInteger("18446744073709551615");

    public CanonicalCounterObservation {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(entityId, "entityId");
        Objects.requireNonNull(metricId, "metricId");
        Objects.requireNonNull(sensorId, "sensorId");
        Objects.requireNonNull(timestamp, "timestamp");
        Objects.requireNonNull(counterValue, "counterValue");
        Objects.requireNonNull(sequenceId, "sequenceId");
        Objects.requireNonNull(sourceProtocol, "sourceProtocol");
        if (observationKind == null || observationKind.isBlank()) {
            observationKind = KIND;
        }
        if (!KIND.equals(observationKind)) {
            throw new IllegalArgumentException("observationKind must be COUNTER64");
        }
        BigInteger exact = parseExactUnsigned64(counterValue);
        // Normalize to canonical decimal form (no leading zeros except "0").
        counterValue = exact.toString();
    }

    public CanonicalCounterObservation(
            String tenantId,
            String entityId,
            String metricId,
            String sensorId,
            Instant timestamp,
            BigInteger exactValue,
            int quality,
            int flags,
            String sequenceId,
            String sourceProtocol
    ) {
        this(KIND, tenantId, entityId, metricId, sensorId, timestamp,
                Objects.requireNonNull(exactValue, "exactValue").toString(),
                quality, flags, sequenceId, sourceProtocol);
    }

    public BigInteger exactValue() {
        return new BigInteger(counterValue);
    }

    public static BigInteger parseExactUnsigned64(String decimal) {
        if (decimal == null) {
            throw new IllegalArgumentException("counterValue must not be null");
        }
        if (decimal.isEmpty()) {
            throw new IllegalArgumentException("counterValue must not be empty");
        }
        if (!decimal.matches("^[0-9]+$")) {
            throw new IllegalArgumentException("counterValue must be unsigned decimal digits only");
        }
        BigInteger value;
        try {
            value = new BigInteger(decimal);
        } catch (NumberFormatException e) {
            throw new IllegalArgumentException("counterValue is not a valid integer", e);
        }
        if (value.signum() < 0 || value.compareTo(MAX_UINT64) > 0) {
            throw new IllegalArgumentException("counterValue outside unsigned 64-bit range");
        }
        return value;
    }
}
