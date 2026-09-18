package com.platform.core.api.dto;

import java.time.Instant;

public final class CapabilityPackDtos {
    private CapabilityPackDtos() {
    }

    public record ActivateRequest(String packVersion, long expectedGeneration) {
    }

    public record PackVersionResponse(
            String packId,
            String packVersion,
            String schemaVersion,
            String packClass,
            String name,
            String publisherType,
            String publisherName,
            String certification,
            String manifestSha256,
            String createdBy,
            Instant createdAt,
            ActiveVersionSummary activeVersion
    ) {
    }

    public record PackVersionListItem(
            String packId,
            String packVersion,
            String schemaVersion,
            String packClass,
            String name,
            String publisherType,
            String publisherName,
            String certification,
            String manifestSha256,
            String createdBy,
            Instant createdAt
    ) {
    }

    public record ActiveVersionResponse(
            String packId,
            String packVersion,
            long generation,
            Instant activatedAt,
            String activatedBy
    ) {
    }

    public record ActiveVersionSummary(
            String packVersion,
            long generation,
            Instant activatedAt,
            String activatedBy
    ) {
    }
}
