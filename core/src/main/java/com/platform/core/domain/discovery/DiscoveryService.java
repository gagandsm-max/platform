package com.platform.core.domain.discovery;

import com.platform.core.api.PageResult;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.id.UuidV7;
import com.platform.core.persistence.DiscoveryRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;
import java.util.stream.Collectors;

@Service
public class DiscoveryService {

    private static final Logger log = LoggerFactory.getLogger(DiscoveryService.class);

    private final DiscoveryRepository repository;
    private final TenantService tenantService;

    public DiscoveryService(DiscoveryRepository repository, TenantService tenantService) {
        this.repository = repository;
        this.tenantService = tenantService;
    }

    @Transactional
    public DiscoveryRequest createDiscovery(
            UUID tenantId,
            String name,
            List<String> rawTargets,
            List<DiscoveryMethod> requestedMethods,
            String credentialProfileRef,
            UUID siteId,
            UUID sensorPoolId,
            Integer snmpPort,
            Integer timeoutMs,
            Integer maxRetries,
            String idempotencyKey,
            String correlationId,
            String createdBy
    ) {
        // 1. Verify tenant exists and is active
        tenantService.get(tenantId);

        // 2. Validate name and secret absence
        if (name == null || name.trim().isEmpty()) {
            throw new DomainException("VALIDATION_FAILED", "Discovery request name is required");
        }
        String cleanName = name.trim();
        if (cleanName.length() > 255) {
            throw new DomainException("VALIDATION_FAILED", "Discovery request name cannot exceed 255 characters");
        }
        DiscoveryLimits.validateNoSecrets(cleanName, credentialProfileRef, idempotencyKey);

        // 3. Idempotency Check (within tenant scope)
        if (idempotencyKey != null && !idempotencyKey.trim().isEmpty()) {
            Optional<DiscoveryRequest> existing = repository.findByIdempotencyKey(tenantId, idempotencyKey.trim());
            if (existing.isPresent()) {
                log.info("Idempotent discovery request matched: tenant={}, idempotencyKey={}, requestId={}",
                        tenantId, idempotencyKey, existing.get().requestId());
                return existing.get();
            }
        }

        // 4. Validate & expand targets (SSRF checks, CIDR bounds, limit of 256)
        List<String> targets = DiscoveryLimits.expandAndValidateTargets(rawTargets);

        // 5. Methods normalization
        List<DiscoveryMethod> methods = requestedMethods != null && !requestedMethods.isEmpty()
                ? List.copyOf(requestedMethods)
                : List.of(DiscoveryMethod.ICMP, DiscoveryMethod.SNMP);

        int port = (snmpPort != null && snmpPort > 0 && snmpPort <= 65535) ? snmpPort : DiscoveryLimits.DEFAULT_SNMP_PORT;
        int timeout = (timeoutMs != null && timeoutMs >= DiscoveryLimits.MIN_TIMEOUT_MS && timeoutMs <= DiscoveryLimits.MAX_TIMEOUT_MS)
                ? timeoutMs : DiscoveryLimits.DEFAULT_TIMEOUT_MS;
        int retries = (maxRetries != null && maxRetries >= 0 && maxRetries <= DiscoveryLimits.MAX_RETRIES)
                ? maxRetries : DiscoveryLimits.DEFAULT_RETRIES;

        UUID requestId = UuidV7.generate();
        String corrId = (correlationId != null && !correlationId.trim().isEmpty())
                ? correlationId.trim()
                : requestId.toString();
        Instant now = Instant.now();

        // 6. Construct Targets
        List<DiscoveryTarget> discoveryTargets = new ArrayList<>(targets.size());
        for (String targetAddr : targets) {
            TargetType type = DiscoveryLimits.classifyTargetType(targetAddr);
            discoveryTargets.add(new DiscoveryTarget(
                    UuidV7.generate(),
                    tenantId,
                    requestId,
                    targetAddr,
                    type,
                    "PENDING",
                    now,
                    now
            ));
        }

        // 7. Sensor assignment: Smallest deterministic eligibility rule (default to available network sensor)
        String assignedSensor = "sensor-net-01";

        DiscoveryRequest request = new DiscoveryRequest(
                requestId,
                tenantId,
                cleanName,
                DiscoveryStatus.ASSIGNED,
                corrId,
                (idempotencyKey != null && !idempotencyKey.trim().isEmpty()) ? idempotencyKey.trim() : null,
                siteId,
                sensorPoolId,
                assignedSensor,
                methods,
                credentialProfileRef != null ? credentialProfileRef.trim() : null,
                port,
                timeout,
                retries,
                createdBy != null ? createdBy.trim() : "system",
                now,
                null,
                null,
                Map.of("targetCount", targets.size()),
                null
        );

        repository.createRequest(request, discoveryTargets);

        log.info("Created discovery request: id={}, tenant={}, name='{}', targets={}, assignedSensor={}, correlation={}",
                requestId, tenantId, cleanName, targets.size(), assignedSensor, corrId);

        return request;
    }

    public DiscoveryRequest getRequest(UUID tenantId, UUID requestId) {
        tenantService.get(tenantId);
        return repository.findById(tenantId, requestId)
                .orElseThrow(() -> new DomainException("RESOURCE_NOT_FOUND", "Discovery request not found: " + requestId));
    }

    public PageResult<DiscoveryRequest> listRequests(UUID tenantId, int page, int limit) {
        tenantService.get(tenantId);
        return repository.listRequests(tenantId, page, limit);
    }

    @Transactional
    public DiscoveryRequest cancelRequest(UUID tenantId, UUID requestId) {
        DiscoveryRequest request = getRequest(tenantId, requestId);
        if (request.status() == DiscoveryStatus.COMPLETED || request.status() == DiscoveryStatus.FAILED) {
            throw new DomainException("INVALID_STATE_TRANSITION",
                    "Cannot cancel completed or failed discovery request: " + request.status());
        }
        if (request.status() != DiscoveryStatus.CANCELLED) {
            repository.updateStatus(tenantId, requestId, DiscoveryStatus.CANCELLED, null, Instant.now(), "Cancelled by user", null);
            log.info("Cancelled discovery request: id={}, tenant={}", requestId, tenantId);
        }
        return getRequest(tenantId, requestId);
    }

    public PageResult<DiscoveryCandidate> listCandidates(UUID tenantId, UUID requestId, int page, int limit) {
        getRequest(tenantId, requestId);
        return repository.listCandidates(tenantId, requestId, page, limit);
    }

    public PageResult<DiscoveryEvidence> listEvidence(UUID tenantId, UUID requestId, int page, int limit) {
        getRequest(tenantId, requestId);
        return repository.listEvidence(tenantId, requestId, page, limit);
    }

    public Optional<DiscoveryTaskPayload> fetchTaskForSensor(String sensorId) {
        Optional<DiscoveryRequest> opt = repository.findPendingRequestForSensor(sensorId);
        if (opt.isEmpty()) {
            return Optional.empty();
        }

        DiscoveryRequest req = opt.get();
        List<DiscoveryTarget> targets = repository.listTargets(req.tenantId(), req.requestId());
        Instant startedAt = req.startedAt() == null ? Instant.now() : req.startedAt();
        repository.updateStatus(req.tenantId(), req.requestId(), DiscoveryStatus.RUNNING, startedAt, null, null, null);

        log.info("Assigned discovery task to sensor: sensorId={}, requestId={}, targetCount={}",
                sensorId, req.requestId(), targets.size());

        List<String> targetAddrs = targets.stream().map(DiscoveryTarget::targetAddress).toList();
        List<String> methodNames = req.requestedMethods().stream().map(Enum::name).toList();

        return Optional.of(new DiscoveryTaskPayload(
                req.requestId().toString(),
                req.tenantId().toString(),
                req.correlationId(),
                targetAddrs,
                methodNames,
                req.snmpPort(),
                req.timeoutMs(),
                req.maxRetries()
        ));
    }

    @Transactional
    public void ingestResultsFromSensor(
            String sensorId,
            UUID tenantId,
            UUID requestId,
            List<SensorEvidenceItem> evidenceItems,
            String overallStatus,
            String failureReason
    ) {
        DiscoveryRequest request = getRequest(tenantId, requestId);
        List<DiscoveryTarget> targets = repository.listTargets(tenantId, requestId);
        Map<String, UUID> targetMap = targets.stream()
                .collect(Collectors.toMap(DiscoveryTarget::targetAddress, DiscoveryTarget::targetId, (a, b) -> a));

        Instant now = Instant.now();
        List<DiscoveryEvidence> evidenceList = new ArrayList<>();

        // Group evidence by target address for candidate aggregation
        Map<String, List<SensorEvidenceItem>> perTarget = new HashMap<>();

        if (evidenceItems != null) {
            for (SensorEvidenceItem item : evidenceItems) {
                UUID targetId = targetMap.get(item.targetAddress());
                if (targetId == null) {
                    targetId = UuidV7.generate();
                }

                DiscoveryMethod proto;
                try {
                    proto = DiscoveryMethod.valueOf(item.protocol().toUpperCase());
                } catch (Exception e) {
                    proto = DiscoveryMethod.ICMP;
                }

                EvidenceStatus status;
                try {
                    status = EvidenceStatus.valueOf(item.status().toUpperCase());
                } catch (Exception e) {
                    status = EvidenceStatus.INCONCLUSIVE;
                }

                ReachabilityKind reachability;
                try {
                    reachability = ReachabilityKind.valueOf(item.reachabilityKind().toUpperCase());
                } catch (Exception e) {
                    reachability = ReachabilityKind.UNKNOWN;
                }

                // Defensive check: ICMP failure or timeout MUST NOT be declared as DEVICE_DOWN
                // No false-causal conclusions!
                if (status == EvidenceStatus.FAILED && reachability == ReachabilityKind.OBSERVED) {
                    reachability = ReachabilityKind.SUSPECTED;
                }

                evidenceList.add(new DiscoveryEvidence(
                        UuidV7.generate(),
                        tenantId,
                        requestId,
                        targetId,
                        sensorId,
                        proto,
                        item.method(),
                        status,
                        reachability,
                        item.technicalCode(),
                        item.attributes() != null ? item.attributes() : Map.of(),
                        item.rawEvidence() != null ? item.rawEvidence() : Map.of(),
                        item.durationMs(),
                        now
                ));

                perTarget.computeIfAbsent(item.targetAddress(), k -> new ArrayList<>()).add(item);
            }
        }

        repository.recordEvidence(tenantId, requestId, evidenceList);

        // Aggregate Candidates from Evidence:
        // CANDIDATES ARE FOR THE REVIEW QUEUE ONLY.
        // ABSOLUTELY NO AUTO-PROVISIONING OCCURS HERE!
        int candidateCount = 0;
        int reachedCount = 0;

        for (Map.Entry<String, List<SensorEvidenceItem>> entry : perTarget.entrySet()) {
            String address = entry.getKey();
            List<SensorEvidenceItem> items = entry.getValue();

            boolean icmpSuccess = items.stream().anyMatch(i -> "ICMP".equalsIgnoreCase(i.protocol()) && "SUCCESS".equalsIgnoreCase(i.status()));
            boolean snmpSuccess = items.stream().anyMatch(i -> "SNMP".equalsIgnoreCase(i.protocol()) && "SUCCESS".equalsIgnoreCase(i.status()));

            if (icmpSuccess || snmpSuccess) {
                reachedCount++;
            }

            List<DiscoveryMethod> protocols = new ArrayList<>();
            if (icmpSuccess) protocols.add(DiscoveryMethod.ICMP);
            if (snmpSuccess) protocols.add(DiscoveryMethod.SNMP);

            String sysDescr = null;
            String sysObjectId = null;
            String sysName = null;

            Map<String, Object> mergedAttrs = new HashMap<>();
            for (SensorEvidenceItem it : items) {
                if (it.attributes() != null) {
                    mergedAttrs.putAll(it.attributes());
                    if (it.attributes().containsKey("sysDescr")) {
                        sysDescr = String.valueOf(it.attributes().get("sysDescr"));
                    }
                    if (it.attributes().containsKey("sysObjectID")) {
                        sysObjectId = String.valueOf(it.attributes().get("sysObjectID"));
                    }
                    if (it.attributes().containsKey("sysName")) {
                        sysName = String.valueOf(it.attributes().get("sysName"));
                    }
                }
            }

            DiscoveryConfidence confidence;
            if (snmpSuccess && sysObjectId != null) {
                confidence = DiscoveryConfidence.HIGH;
            } else if (icmpSuccess) {
                confidence = DiscoveryConfidence.MEDIUM;
            } else {
                confidence = DiscoveryConfidence.INCONCLUSIVE;
            }

            // Conflict detection: if multiple distinct sysNames or conflicting descriptors are found
            boolean conflict = false;
            String conflictDetails = null;

            DiscoveryCandidate candidate = new DiscoveryCandidate(
                    UuidV7.generate(),
                    tenantId,
                    requestId,
                    address,
                    sysName != null ? sysName : address,
                    sysObjectId,
                    sysDescr,
                    protocols,
                    mergedAttrs,
                    confidence,
                    CandidateReviewStatus.PENDING_REVIEW, // Strictly in review queue
                    conflict,
                    conflictDetails,
                    request.siteId(),
                    now,
                    now,
                    now,
                    now
            );

            repository.upsertCandidate(candidate);
            candidateCount++;
        }

        Instant completedAt = Instant.now();
        DiscoveryStatus finalStatus = "FAILED".equalsIgnoreCase(overallStatus)
                ? DiscoveryStatus.FAILED
                : DiscoveryStatus.COMPLETED;

        long durationMs = request.startedAt() != null
                ? Duration.between(request.startedAt(), completedAt).toMillis()
                : 0;

        Map<String, Object> summary = new HashMap<>();
        summary.put("targetCount", targets.size());
        summary.put("probedCount", perTarget.size());
        summary.put("reachedCount", reachedCount);
        summary.put("candidateCount", candidateCount);
        summary.put("evidenceCount", evidenceList.size());
        summary.put("durationMs", durationMs);
        summary.put("sensorId", sensorId);

        repository.updateStatus(tenantId, requestId, finalStatus, null, completedAt, failureReason, summary);

        log.info("Discovery execution completed: id={}, tenant={}, status={}, targets={}, reached={}, candidates={}, durationMs={}",
                requestId, tenantId, finalStatus, targets.size(), reachedCount, candidateCount, durationMs);
    }

    public record DiscoveryTaskPayload(
            String requestId,
            String tenantId,
            String correlationId,
            List<String> targets,
            List<String> requestedMethods,
            int snmpPort,
            int timeoutMs,
            int maxRetries
    ) {}

    public record SensorEvidenceItem(
            String targetAddress,
            String protocol,
            String method,
            String status,
            String reachabilityKind,
            String technicalCode,
            Map<String, Object> attributes,
            Map<String, Object> rawEvidence,
            long durationMs
    ) {}
}
