package com.platform.core.agentcontrol;

import java.util.Objects;
import java.util.UUID;

/**
 * Bounded Agent authority DTO returned to an authenticated Sensor.
 * Contains only the minimum facts needed for Agent Receiver verification.
 *
 * <p>Go production {@code AgentIdentityVerifier}/{@code AssignmentAuthorizer} adapters that call Core
 * are a follow-up after Sensor {@code AssignmentContext} gains a Generation field; leave Go
 * fail-closed defaults until that contract lands (do not wire incomplete Core-calling adapters).
 */
public record AgentAuthorityView(
        String agentId,
        UUID tenantId,
        UUID siteId,
        long assignmentGeneration,
        String sensorId
) {
    public AgentAuthorityView {
        Objects.requireNonNull(agentId, "agentId");
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(sensorId, "sensorId");
        if (assignmentGeneration < 1) {
            throw new IllegalArgumentException("assignmentGeneration must be >= 1");
        }
    }
}
