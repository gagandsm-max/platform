package com.platform.core.domain.discovery;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.UUID;

public record DiscoveryCandidate(
        UUID candidateId,
        UUID tenantId,
        UUID requestId,
        String targetAddress,
        String discoveredName,
        String sysObjectId,
        String sysDescr,
        List<DiscoveryMethod> evidencedProtocols,
        Map<String, Object> attributes,
        DiscoveryConfidence confidence,
        CandidateReviewStatus reviewStatus,
        boolean conflictDetected,
        String conflictDetails,
        UUID siteId,
        Instant firstSeenAt,
        Instant lastSeenAt,
        Instant createdAt,
        Instant updatedAt
) {
}
