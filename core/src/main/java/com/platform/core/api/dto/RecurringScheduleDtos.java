package com.platform.core.api.dto;

import java.time.Instant;
import java.util.List;

public final class RecurringScheduleDtos {
    private RecurringScheduleDtos() {
    }

    public record SnmpLimitsDto(
            Integer maxOids,
            Integer maxWalkEntries,
            Long maxBulkRepetitions,
            Integer maxResults,
            Integer maxResultBytes
    ) {
    }

    public record SnmpParamsDto(
            String target,
            Integer port,
            String version,
            String operation,
            List<String> oids,
            Long timeoutMs,
            SnmpLimitsDto limits
    ) {
    }

    public record AssignmentRequestItem(
            String assignmentId,
            String entityId,
            String monitorDefinitionId,
            String schedulingClass,
            String executionClass,
            Long cadenceMs,
            Long freshnessMs,
            Integer phaseVersion,
            Long executionTimeoutMs,
            String credentialProfileRef,
            String protocol,
            SnmpParamsDto snmp,
            String protocolParamsJson
    ) {
    }

    public record ReplaceAssignmentsRequest(
            Long expectedGeneration,
            List<AssignmentRequestItem> assignments
    ) {
    }

    public record AppliedStateRequest(Long generation, String checksum) {
    }

    public record AssignmentItem(
            String assignmentId,
            String entityId,
            String monitorDefinitionId,
            String schedulingClass,
            String executionClass,
            long cadenceMs,
            long freshnessMs,
            int phaseVersion,
            long executionTimeoutMs,
            String credentialProfileRef,
            String protocol,
            SnmpParamsDto snmp,
            String assignedBy,
            Instant assignedAt
    ) {
    }

    public record AssignmentsResponse(
            String sensorId,
            long desiredGeneration,
            String desiredChecksum,
            long appliedGeneration,
            String appliedChecksum,
            Instant appliedAt,
            List<AssignmentItem> assignments
    ) {
    }

    public record DesiredDefinitionItem(
            String assignmentId,
            String tenantId,
            String entityId,
            String monitorDefinitionId,
            String schedulingClass,
            String executionClass,
            long cadenceMs,
            long freshnessMs,
            int phaseVersion,
            long executionTimeoutMs,
            String credentialProfileRef,
            String protocol,
            SnmpParamsDto snmp
    ) {
    }

    public record SensorDesiredStateResponse(
            long generation,
            String checksum,
            long appliedGeneration,
            String appliedChecksum,
            List<DesiredDefinitionItem> definitions
    ) {
    }
}
