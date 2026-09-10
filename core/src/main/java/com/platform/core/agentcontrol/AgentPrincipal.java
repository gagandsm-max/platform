package com.platform.core.agentcontrol;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Authoritative enrolled Agent principal (PostgreSQL {@code platform.agent}).
 * Agent ID is case-sensitive (exact VARCHAR equality).
 */
public record AgentPrincipal(
        String agentId,
        UUID tenantId,
        UUID siteId,
        AgentLifecycleStatus status,
        Instant createdAt,
        Instant updatedAt
) {
    public AgentPrincipal {
        Objects.requireNonNull(agentId, "agentId");
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(status, "status");
        Objects.requireNonNull(createdAt, "createdAt");
        Objects.requireNonNull(updatedAt, "updatedAt");
        if (agentId.isBlank() || agentId.length() > 128) {
            throw new IllegalArgumentException("agentId must be 1..128 characters");
        }
    }
}
