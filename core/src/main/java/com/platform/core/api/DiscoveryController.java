package com.platform.core.api;

import com.platform.core.api.dto.DiscoveryDtos.CandidateResponse;
import com.platform.core.api.dto.DiscoveryDtos.CreateDiscoveryRequest;
import com.platform.core.api.dto.DiscoveryDtos.DiscoveryResponse;
import com.platform.core.api.dto.DiscoveryDtos.EvidenceResponse;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.discovery.DiscoveryCandidate;
import com.platform.core.domain.discovery.DiscoveryEvidence;
import com.platform.core.domain.discovery.DiscoveryMethod;
import com.platform.core.domain.discovery.DiscoveryRequest;
import com.platform.core.domain.discovery.DiscoveryService;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.UUID;

@RestController
@RequestMapping("/api/v1/tenants/{tenantId}/discovery")
public class DiscoveryController {

    private final DiscoveryService discoveryService;

    public DiscoveryController(DiscoveryService discoveryService) {
        this.discoveryService = discoveryService;
    }

    @PostMapping
    public ResponseEntity<DiscoveryResponse> create(
            @PathVariable UUID tenantId,
            @RequestBody CreateDiscoveryRequest request,
            @RequestHeader(value = "Idempotency-Key", required = false) String idempotencyKeyHeader,
            @RequestHeader(value = "X-Correlation-ID", required = false) String correlationIdHeader
    ) {
        String effectiveIdempotencyKey = (idempotencyKeyHeader != null && !idempotencyKeyHeader.trim().isEmpty())
                ? idempotencyKeyHeader.trim()
                : request.idempotencyKey();

        List<DiscoveryMethod> methods = null;
        if (request.requestedMethods() != null) {
            methods = new ArrayList<>();
            for (String rawMethod : request.requestedMethods()) {
                if (rawMethod == null || rawMethod.trim().isEmpty()) {
                    throw new DomainException("VALIDATION_FAILED", "Discovery method cannot be blank");
                }
                try {
                    methods.add(DiscoveryMethod.valueOf(rawMethod.trim().toUpperCase(Locale.ROOT)));
                } catch (IllegalArgumentException ex) {
                    throw new DomainException("VALIDATION_FAILED", "Unsupported discovery method");
                }
            }
        }

        DiscoveryRequest created = discoveryService.createDiscovery(
                tenantId,
                request.name(),
                request.targets(),
                methods,
                request.credentialProfileRef(),
                request.siteId(),
                request.sensorPoolId(),
                request.snmpPort(),
                request.timeoutMs(),
                request.maxRetries(),
                effectiveIdempotencyKey,
                correlationIdHeader,
                request.createdBy()
        );

        return ResponseEntity.status(HttpStatus.CREATED).body(toResponse(created));
    }

    @GetMapping("/{requestId}")
    public DiscoveryResponse get(@PathVariable UUID tenantId, @PathVariable UUID requestId) {
        return toResponse(discoveryService.getRequest(tenantId, requestId));
    }

    @GetMapping
    public PageResult<DiscoveryResponse> list(
            @PathVariable UUID tenantId,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(required = false) Integer limit
    ) {
        PageResult<DiscoveryRequest> result = discoveryService.listRequests(tenantId, page, PageResult.normalizeLimit(limit));
        return new PageResult<>(
                result.items().stream().map(this::toResponse).toList(),
                result.page(), result.limit(), result.total(), result.hasMore()
        );
    }

    @PostMapping("/{requestId}/cancel")
    public DiscoveryResponse cancel(@PathVariable UUID tenantId, @PathVariable UUID requestId) {
        return toResponse(discoveryService.cancelRequest(tenantId, requestId));
    }

    @GetMapping("/{requestId}/candidates")
    public PageResult<CandidateResponse> listCandidates(
            @PathVariable UUID tenantId,
            @PathVariable UUID requestId,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(required = false) Integer limit
    ) {
        PageResult<DiscoveryCandidate> result = discoveryService.listCandidates(tenantId, requestId, page, PageResult.normalizeLimit(limit));
        return new PageResult<>(
                result.items().stream().map(this::toCandidateResponse).toList(),
                result.page(), result.limit(), result.total(), result.hasMore()
        );
    }

    @GetMapping("/{requestId}/evidence")
    public PageResult<EvidenceResponse> listEvidence(
            @PathVariable UUID tenantId,
            @PathVariable UUID requestId,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(required = false) Integer limit
    ) {
        PageResult<DiscoveryEvidence> result = discoveryService.listEvidence(tenantId, requestId, page, PageResult.normalizeLimit(limit));
        return new PageResult<>(
                result.items().stream().map(this::toEvidenceResponse).toList(),
                result.page(), result.limit(), result.total(), result.hasMore()
        );
    }

    private DiscoveryResponse toResponse(DiscoveryRequest r) {
        List<String> methods = r.requestedMethods() != null
                ? r.requestedMethods().stream().map(Enum::name).toList()
                : List.of();
        return new DiscoveryResponse(
                r.requestId(), r.tenantId(), r.name(), r.status().name(), r.correlationId(), r.idempotencyKey(),
                r.siteId(), r.sensorPoolId(), r.assignedSensorId(), methods, r.credentialProfileRef(),
                r.snmpPort(), r.timeoutMs(), r.maxRetries(), r.createdBy(), r.createdAt(), r.startedAt(),
                r.completedAt(), r.summary(), r.failureReason()
        );
    }

    private CandidateResponse toCandidateResponse(DiscoveryCandidate c) {
        List<String> protocols = c.evidencedProtocols() != null
                ? c.evidencedProtocols().stream().map(Enum::name).toList()
                : List.of();
        return new CandidateResponse(
                c.candidateId(), c.tenantId(), c.requestId(), c.targetAddress(), c.discoveredName(), c.sysObjectId(),
                c.sysDescr(), protocols, c.attributes(), c.confidence().name(), c.reviewStatus().name(),
                c.conflictDetected(), c.conflictDetails(), c.siteId(), c.firstSeenAt(), c.lastSeenAt(), c.createdAt(), c.updatedAt()
        );
    }

    private EvidenceResponse toEvidenceResponse(DiscoveryEvidence e) {
        return new EvidenceResponse(
                e.evidenceId(), e.tenantId(), e.requestId(), e.targetId(), e.sensorId(), e.protocol().name(), e.method(),
                e.status().name(), e.reachabilityKind().name(), e.technicalCode(), e.attributes(), e.durationMs(), e.observedAt()
        );
    }
}
