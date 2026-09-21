package com.platform.core.domain.recurring;

import java.util.Objects;

/** One recurring assignment in a Sensor desired-state snapshot (no secrets). */
public record DesiredRecurringDefinition(
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
        SnmpParams snmp
) {
    public DesiredRecurringDefinition {
        Objects.requireNonNull(assignmentId, "assignmentId");
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(entityId, "entityId");
        Objects.requireNonNull(monitorDefinitionId, "monitorDefinitionId");
        Objects.requireNonNull(schedulingClass, "schedulingClass");
        Objects.requireNonNull(executionClass, "executionClass");
        Objects.requireNonNull(credentialProfileRef, "credentialProfileRef");
        Objects.requireNonNull(protocol, "protocol");
        Objects.requireNonNull(snmp, "snmp");
    }
}
