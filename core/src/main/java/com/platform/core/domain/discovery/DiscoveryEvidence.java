package com.platform.core.domain.discovery;

import java.time.Instant;
import java.util.Map;
import java.util.UUID;

public record DiscoveryEvidence(
        UUID evidenceId,
        UUID tenantId,
        UUID requestId,
        UUID targetId,
        String sensorId,
        DiscoveryMethod protocol,
        String method,
        EvidenceStatus status,
        ReachabilityKind reachabilityKind,
        String technicalCode,
        Map<String, Object> attributes,
        Map<String, Object> rawEvidence,
        long durationMs,
        Instant observedAt
) {
}
