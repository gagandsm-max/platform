package com.platform.core.api.dto;

import java.time.Instant;
import java.util.List;

public final class CapabilityPackDtos {
    private CapabilityPackDtos() {
    }

    public record ActivateRequest(String packVersion, Long expectedGeneration) {
    }

    public record AssignRequest(String packVersion, Long expectedGeneration) {
    }

    public record AppliedStateRequest(Long generation, String checksum) {
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

    public record SensorAssignmentItem(
            String packId,
            String packVersion,
            String assignedBy,
            Instant assignedAt
    ) {
    }

    public record SensorAssignmentsResponse(
            String sensorId,
            long desiredGeneration,
            String desiredChecksum,
            long appliedGeneration,
            String appliedChecksum,
            Instant appliedAt,
            List<SensorAssignmentItem> assignments
    ) {
    }

    public record DesiredPackItem(
            String packId,
            String packVersion,
            String manifestSha256,
            int artifactBytes
    ) {
    }

    public record SensorDesiredStateResponse(
            long generation,
            String checksum,
            List<DesiredPackItem> packs
    ) {
    }
}
