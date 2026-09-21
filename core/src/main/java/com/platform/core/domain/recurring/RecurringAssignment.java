package com.platform.core.domain.recurring;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/** Desired Sensor recurring schedule assignment row (no secrets). */
public record RecurringAssignment(
        UUID tenantId,
        String sensorId,
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
        String protocolParamsJson,
        String assignedBy,
        Instant assignedAt
) {
    public RecurringAssignment {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(sensorId, "sensorId");
        Objects.requireNonNull(assignmentId, "assignmentId");
        Objects.requireNonNull(entityId, "entityId");
        Objects.requireNonNull(monitorDefinitionId, "monitorDefinitionId");
        Objects.requireNonNull(schedulingClass, "schedulingClass");
        Objects.requireNonNull(executionClass, "executionClass");
        Objects.requireNonNull(credentialProfileRef, "credentialProfileRef");
        Objects.requireNonNull(protocol, "protocol");
        Objects.requireNonNull(protocolParamsJson, "protocolParamsJson");
        Objects.requireNonNull(assignedBy, "assignedBy");
        Objects.requireNonNull(assignedAt, "assignedAt");
    }
}
