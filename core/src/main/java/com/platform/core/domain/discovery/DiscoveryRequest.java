package com.platform.core.domain.discovery;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.UUID;

public record DiscoveryRequest(
        UUID requestId,
        UUID tenantId,
        String name,
        DiscoveryStatus status,
        String correlationId,
        String idempotencyKey,
        UUID siteId,
        UUID sensorPoolId,
        String assignedSensorId,
        List<DiscoveryMethod> requestedMethods,
        String credentialProfileRef,
        int snmpPort,
        int timeoutMs,
        int maxRetries,
        String createdBy,
        Instant createdAt,
        Instant startedAt,
        Instant completedAt,
        Map<String, Object> summary,
        String failureReason
) {
}
