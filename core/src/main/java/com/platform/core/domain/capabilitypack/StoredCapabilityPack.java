package com.platform.core.domain.capabilitypack;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Immutable tenant-scoped Capability Pack artifact row (exact validated manifest bytes).
 */
public record StoredCapabilityPack(
        UUID tenantId,
        String packId,
        String packVersion,
        String schemaVersion,
        String packClass,
        String name,
        String publisherType,
        String publisherName,
        String certification,
        String manifestSha256,
        byte[] manifestBytes,
        String createdBy,
        Instant createdAt
) {
    public StoredCapabilityPack {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(packId, "packId");
        Objects.requireNonNull(packVersion, "packVersion");
        Objects.requireNonNull(schemaVersion, "schemaVersion");
        Objects.requireNonNull(packClass, "packClass");
        Objects.requireNonNull(name, "name");
        Objects.requireNonNull(publisherType, "publisherType");
        Objects.requireNonNull(publisherName, "publisherName");
        Objects.requireNonNull(certification, "certification");
        Objects.requireNonNull(manifestSha256, "manifestSha256");
        Objects.requireNonNull(manifestBytes, "manifestBytes");
        Objects.requireNonNull(createdBy, "createdBy");
        Objects.requireNonNull(createdAt, "createdAt");
        manifestBytes = manifestBytes.clone();
    }

    @Override
    public byte[] manifestBytes() {
        return manifestBytes.clone();
    }
}
