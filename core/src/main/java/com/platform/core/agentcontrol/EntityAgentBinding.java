package com.platform.core.agentcontrol;

import java.time.Instant;
import java.util.UUID;

/** Current Platform_20 section 6.3 authority; hostId is never a binding key. */
public record EntityAgentBinding(
        UUID tenantId, UUID entityId, String agentId, String certificateIdentity,
        String systemName, String osIdentity, Instant firstEnrolledAt,
        Instant lastHeartbeatAt, String bindingState, long reinstallGeneration
) {}
