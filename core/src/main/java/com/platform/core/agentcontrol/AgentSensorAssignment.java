package com.platform.core.agentcontrol;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Authoritative Agent→Sensor assignment fence (PostgreSQL {@code platform.agent_sensor_assignment}).
 * At most one ACTIVE assignment per agent; {@code generation} is a monotonic fence.
 */
public record AgentSensorAssignment(
        UUID assignmentId,
        UUID tenantId,
        String agentId,
        String sensorId,
        long generation,
        AgentAssignmentStatus status,
        Instant createdAt,
        Instant updatedAt,
        Instant revokedAt
) {
    public AgentSensorAssignment {
        Objects.requireNonNull(assignmentId, "assignmentId");
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(agentId, "agentId");
        Objects.requireNonNull(sensorId, "sensorId");
        Objects.requireNonNull(status, "status");
        Objects.requireNonNull(createdAt, "createdAt");
        Objects.requireNonNull(updatedAt, "updatedAt");
        if (generation < 1) {
            throw new IllegalArgumentException("generation must be >= 1");
        }
    }
}
