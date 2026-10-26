package com.platform.core.ingestion.icmp;

import java.time.Instant;
import java.util.Objects;

/**
 * Canonical Core domain for Sensor {@code icmp_observation_v1} envelope + Evidence.
 * Lossless typed contract — not a numeric projection and not DEVICE_DOWN authority.
 */
public record IcmpObservationV1(
        int schemaVersion,
        String batchId,
        String recordType,
        Instant collectedAt,
        String sensorId,
        String tenantId,
        String entityId,
        IcmpJobCorrelation job,
        String protocol,
        String protocolVersion,
        String operation,
        String status,
        IcmpQuality quality,
        int protocolSchemaVersion,
        IcmpEvidence evidence
) {
    public static final String RECORD_TYPE = "icmp_observation_v1";
    public static final String PROTOCOL = "ICMP";
    public static final int ENVELOPE_SCHEMA_VERSION = 1;
    public static final int PROTOCOL_SCHEMA_VERSION = 1;
    public static final int EVIDENCE_SCHEMA_VERSION = 1;

    public IcmpObservationV1 {
        Objects.requireNonNull(batchId, "batchId");
        Objects.requireNonNull(recordType, "recordType");
        Objects.requireNonNull(collectedAt, "collectedAt");
        Objects.requireNonNull(sensorId, "sensorId");
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(entityId, "entityId");
        Objects.requireNonNull(job, "job");
        Objects.requireNonNull(protocol, "protocol");
        Objects.requireNonNull(protocolVersion, "protocolVersion");
        Objects.requireNonNull(operation, "operation");
        Objects.requireNonNull(status, "status");
        Objects.requireNonNull(quality, "quality");
        Objects.requireNonNull(evidence, "evidence");
    }
}
