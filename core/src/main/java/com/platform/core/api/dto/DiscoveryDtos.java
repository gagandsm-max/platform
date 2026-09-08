package com.platform.core.api.dto;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.UUID;

public final class DiscoveryDtos {

    private DiscoveryDtos() {
    }

    public record CreateDiscoveryRequest(
            String name,
            List<String> targets,
            List<String> requestedMethods,
            String credentialProfileRef,
            UUID siteId,
            UUID sensorPoolId,
            Integer snmpPort,
            Integer timeoutMs,
            Integer maxRetries,
            String idempotencyKey,
            String createdBy
    ) {}

    public record DiscoveryResponse(
            UUID requestId,
            UUID tenantId,
            String name,
            String status,
            String correlationId,
            String idempotencyKey,
            UUID siteId,
            UUID sensorPoolId,
            String assignedSensorId,
            List<String> requestedMethods,
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
    ) {}

    public record CandidateResponse(
            UUID candidateId,
            UUID tenantId,
            UUID requestId,
            String targetAddress,
            String discoveredName,
            String sysObjectId,
            String sysDescr,
            List<String> evidencedProtocols,
            Map<String, Object> attributes,
            String confidence,
            String reviewStatus,
            boolean conflictDetected,
            String conflictDetails,
            UUID siteId,
            Instant firstSeenAt,
            Instant lastSeenAt,
            Instant createdAt,
            Instant updatedAt
    ) {}

    public record EvidenceResponse(
            UUID evidenceId,
            UUID tenantId,
            UUID requestId,
            UUID targetId,
            String sensorId,
            String protocol,
            String method,
            String status,
            String reachabilityKind,
            String technicalCode,
            Map<String, Object> attributes,
            long durationMs,
            Instant observedAt
    ) {}
}
