package com.platform.core.domain.discovery;

import com.platform.core.api.PageResult;
import com.platform.core.control.ControlGenerationLimits;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.id.UuidV7;
import com.platform.core.persistence.DiscoveryRepository;
import com.platform.core.persistence.DiscoveryRepository.AssignedRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;
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

    @Transactional("transactionManager")
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
        tenantService.get(tenantId);
        String cleanName = boundedRequired(name, 255, "Discovery request name");
        String credentialRef = boundedOptional(credentialProfileRef, 128, "Credential Profile reference");
        String idem = boundedOptional(idempotencyKey, 128, "Idempotency key");
        String actor = boundedOptional(createdBy, 128, "Created-by value");
        String corr = boundedOptional(correlationId, 128, "Correlation ID");
        DiscoveryLimits.validateNoSecrets(cleanName, credentialRef, idem, actor, corr);

        List<String> targets = DiscoveryLimits.expandAndValidateTargets(rawTargets);
        List<DiscoveryMethod> methods = normalizeMethods(requestedMethods);
        int port = normalizePort(snmpPort);
        int timeout = normalizeTimeout(timeoutMs);
        int retries = normalizeRetries(maxRetries);

        if (idem != null) {
            Optional<DiscoveryRequest> existingOpt = repository.findByIdempotencyKey(tenantId, idem);
            if (existingOpt.isPresent()) {
                DiscoveryRequest existing = existingOpt.get();
                ensureIdempotentEquivalent(existing, targets, cleanName, methods, credentialRef, siteId,
                        sensorPoolId, port, timeout, retries);
                return existing;
            }
        }

        UUID requestId = UuidV7.generate();
        String corrId = corr != null ? corr : requestId.toString();
        Instant now = Instant.now();
        List<DiscoveryTarget> discoveryTargets = new ArrayList<>(targets.size());
        for (String targetAddr : targets) {
            discoveryTargets.add(new DiscoveryTarget(
                    UuidV7.generate(), tenantId, requestId, targetAddr,
                    DiscoveryLimits.classifyTargetType(targetAddr), "PENDING", now, now));
        }

        // Placement belongs to Core Sensor Control. Discovery must not guess a Sensor.
        DiscoveryRequest request = new DiscoveryRequest(
                requestId, tenantId, cleanName, DiscoveryStatus.PENDING, corrId, idem,
                siteId, sensorPoolId, null, methods, credentialRef, port, timeout, retries,
                actor == null ? "system" : actor, now, null, null,
                Map.of("targetCount", targets.size()), null
        );
        repository.createRequest(request, discoveryTargets);
        log.info("Discovery request created: requestId={}, tenantId={}, targetCount={}, status=PENDING",
                requestId, tenantId, targets.size());
        return request;
    }

    public DiscoveryRequest getRequest(UUID tenantId, UUID requestId) {
        tenantService.get(tenantId);
        return repository.findById(tenantId, requestId)
                .orElseThrow(() -> new DomainException("RESOURCE_NOT_FOUND", "Discovery request not found"));
    }

    /**
     * Locked read for placement: holds a row lock when called inside an active transaction.
     */
    public DiscoveryRequest getRequestForUpdate(UUID tenantId, UUID requestId) {
        tenantService.get(tenantId);
        return repository.findByIdForUpdate(tenantId, requestId)
                .orElseThrow(() -> new DomainException("RESOURCE_NOT_FOUND", "Discovery request not found"));
    }

    public PageResult<DiscoveryRequest> listRequests(UUID tenantId, int page, int limit) {
        tenantService.get(tenantId);
        return repository.listRequests(tenantId, page, limit);
    }

    /**
     * First placement only (PENDING + unassigned). Concurrent losers receive
     * {@code DISCOVERY_PLACEMENT_CONFLICT}; does not overwrite an existing assignment.
     * Exhausted {@code assignment_version} fails with {@code ERR_CONTROL_GENERATION_EXHAUSTED}.
     */
    @Transactional("transactionManager")
    public DiscoveryRequest assignSensorFirstPlacementFromControlPlane(UUID tenantId, UUID requestId, String sensorId) {
        tenantService.get(tenantId);
        String sensor = boundedRequired(sensorId, 128, "Sensor identity");
        DiscoveryLimits.validateNoSecrets(sensor);
        DiscoveryRequest locked = repository.findByIdForUpdate(tenantId, requestId)
                .orElseThrow(() -> new DomainException("RESOURCE_NOT_FOUND", "Discovery request not found"));
        long assignmentVersion = repository.findAssignmentVersion(tenantId, requestId)
                .orElseThrow(() -> new DomainException("RESOURCE_NOT_FOUND", "Discovery request not found"));
        if (locked.status() == DiscoveryStatus.PENDING
                && locked.assignedSensorId() == null
                && !ControlGenerationLimits.canIncrement(assignmentVersion)) {
            throw new DomainException(
                    ControlGenerationLimits.ERR_CONTROL_GENERATION_EXHAUSTED,
                    ControlGenerationLimits.EXHAUSTED_MESSAGE);
        }
        if (!repository.assignSensorFirstPlacement(tenantId, requestId, sensor)) {
            throw new DomainException("DISCOVERY_PLACEMENT_CONFLICT",
                    "Discovery request placement lost a concurrent assignment race or is no longer pending");
        }
        log.info("Discovery request first-placed by Core control plane: requestId={}, tenantId={}, sensorId={}",
                requestId, tenantId, sensor);
        return getRequest(tenantId, requestId);
    }

    /**
     * Internal control-plane boundary for intentional Sensor (re)assignment including ASSIGNED→ASSIGNED.
     * It is deliberately not exposed by the Discovery public API. The caller is responsible for supplying
     * an authoritative enrolled Sensor identity after normal Sensor eligibility/placement checks.
     * Exhausted {@code assignment_version} fails with {@code ERR_CONTROL_GENERATION_EXHAUSTED}.
     */
    @Transactional("transactionManager")
    public DiscoveryRequest assignSensorFromControlPlane(UUID tenantId, UUID requestId, String sensorId) {
        tenantService.get(tenantId);
        String sensor = boundedRequired(sensorId, 128, "Sensor identity");
        DiscoveryLimits.validateNoSecrets(sensor);
        DiscoveryRequest locked = repository.findByIdForUpdate(tenantId, requestId)
                .orElseThrow(() -> new DomainException("RESOURCE_NOT_FOUND", "Discovery request not found"));
        long assignmentVersion = repository.findAssignmentVersion(tenantId, requestId)
                .orElseThrow(() -> new DomainException("RESOURCE_NOT_FOUND", "Discovery request not found"));
        if ((locked.status() == DiscoveryStatus.PENDING || locked.status() == DiscoveryStatus.ASSIGNED)
                && !ControlGenerationLimits.canIncrement(assignmentVersion)) {
            throw new DomainException(
                    ControlGenerationLimits.ERR_CONTROL_GENERATION_EXHAUSTED,
                    ControlGenerationLimits.EXHAUSTED_MESSAGE);
        }
        if (!repository.assignSensor(tenantId, requestId, sensor)) {
            throw new DomainException("INVALID_STATE_TRANSITION", "Discovery request cannot be assigned in its current state");
        }
        log.info("Discovery request assigned by Core control plane: requestId={}, tenantId={}, sensorId={}",
                requestId, tenantId, sensor);
        return getRequest(tenantId, requestId);
    }

    @Transactional("transactionManager")
    public DiscoveryRequest cancelRequest(UUID tenantId, UUID requestId) {
        tenantService.get(tenantId);
        DiscoveryRequest request = repository.findByIdForUpdate(tenantId, requestId)
                .orElseThrow(() -> new DomainException("RESOURCE_NOT_FOUND", "Discovery request not found"));
        if (request.status() == DiscoveryStatus.COMPLETED || request.status() == DiscoveryStatus.FAILED) {
            throw new DomainException("INVALID_STATE_TRANSITION", "Terminal discovery request cannot be cancelled");
        }
        if (request.status() != DiscoveryStatus.CANCELLED) {
            repository.updateStatus(tenantId, requestId, DiscoveryStatus.CANCELLED, null, Instant.now(),
                    "Cancelled by user", null);
            log.info("Discovery request cancelled: requestId={}, tenantId={}", requestId, tenantId);
        }
        return repository.findById(tenantId, requestId).orElseThrow();
    }

    public PageResult<DiscoveryCandidate> listCandidates(UUID tenantId, UUID requestId, int page, int limit) {
        getRequest(tenantId, requestId);
        return repository.listCandidates(tenantId, requestId, page, limit);
    }

    public PageResult<DiscoveryEvidence> listEvidence(UUID tenantId, UUID requestId, int page, int limit) {
        getRequest(tenantId, requestId);
        return repository.listEvidence(tenantId, requestId, page, limit);
    }

    @Transactional("transactionManager")
    public Optional<DiscoveryTaskPayload> fetchTaskForSensor(String authenticatedSensorId) {
        String sensorId = boundedRequired(authenticatedSensorId, 128, "Authenticated Sensor identity");
        Optional<AssignedRequest> claimed = repository.claimAssignedRequestForSensor(sensorId, Instant.now());
        if (claimed.isEmpty()) return Optional.empty();

        AssignedRequest assignment = claimed.get();
        DiscoveryRequest req = assignment.request();
        List<DiscoveryTarget> targets = repository.listTargets(req.tenantId(), req.requestId());
        log.info("Discovery task claimed: requestId={}, tenantId={}, sensorId={}, assignmentVersion={}, targetCount={}",
                req.requestId(), req.tenantId(), sensorId, assignment.assignmentVersion(), targets.size());
        return Optional.of(new DiscoveryTaskPayload(
                req.requestId().toString(), req.tenantId().toString(), req.correlationId(), assignment.assignmentVersion(),
                targets.stream().map(DiscoveryTarget::targetAddress).toList(),
                req.requestedMethods().stream().map(Enum::name).toList(), req.credentialProfileRef(),
                req.snmpPort(), req.timeoutMs(), req.maxRetries()
        ));
    }

    @Transactional("transactionManager")
    public ResultIngestOutcome ingestResultsFromSensor(
            String authenticatedSensorId,
            UUID requestId,
            long assignmentVersion,
            List<SensorEvidenceItem> evidenceItems,
            String overallStatus,
            String failureReason
    ) {
        String sensorId = boundedRequired(authenticatedSensorId, 128, "Authenticated Sensor identity");
        if (!ControlGenerationLimits.isPositiveJsonSafe(assignmentVersion)) {
            throw new DomainException("VALIDATION_FAILED",
                    "assignment_version exceeds MAX_SAFE_CONTROL_GENERATION");
        }
        AssignedRequest locked = repository.findAssignedForUpdate(sensorId, requestId)
                .orElseThrow(() -> new DomainException("RESOURCE_NOT_FOUND", "Assigned discovery request not found"));
        DiscoveryRequest request = locked.request();
        UUID tenantId = request.tenantId();

        if (locked.assignmentVersion() != assignmentVersion) {
            throw new DomainException("STALE_ASSIGNMENT", "Discovery result assignment version is stale");
        }
        if (request.status() == DiscoveryStatus.COMPLETED || request.status() == DiscoveryStatus.FAILED) {
            // Transport retry after the authoritative transaction committed: do not duplicate evidence/candidates.
            return new ResultIngestOutcome(request.status(), true, tenantId);
        }
        if (request.status() == DiscoveryStatus.CANCELLED) {
            throw new DomainException("INVALID_STATE_TRANSITION", "Cancelled discovery request rejects stale Sensor results");
        }
        if (request.status() != DiscoveryStatus.RUNNING) {
            throw new DomainException("INVALID_STATE_TRANSITION", "Discovery results are accepted only for RUNNING requests");
        }

        DiscoveryStatus finalStatus = parseFinalStatus(overallStatus);
        String boundedFailure = boundedOptional(failureReason, 512, "Failure reason");
        DiscoveryLimits.validateNoSecrets(boundedFailure);
        if (finalStatus == DiscoveryStatus.FAILED && boundedFailure == null) {
            boundedFailure = "Sensor reported discovery execution failure";
        }

        List<SensorEvidenceItem> items = evidenceItems == null ? List.of() : List.copyOf(evidenceItems);
        if (items.size() > DiscoveryLimits.MAX_EVIDENCE_ITEMS) {
            throw new DomainException("QUERY_LIMIT_EXCEEDED", "Discovery evidence count exceeds defensive limit");
        }

        List<DiscoveryTarget> targets = repository.listTargets(tenantId, requestId);
        Map<String, UUID> targetMap = targets.stream().collect(Collectors.toMap(
                DiscoveryTarget::targetAddress, DiscoveryTarget::targetId, (a, b) -> a));
        Set<String> evidenceKeys = new HashSet<>();
        List<DiscoveryEvidence> evidenceList = new ArrayList<>(items.size());
        Map<String, List<SensorEvidenceItem>> perTarget = new HashMap<>();
        Instant now = Instant.now();

        for (SensorEvidenceItem item : items) {
            if (item == null) throw new DomainException("VALIDATION_FAILED", "Discovery evidence item cannot be null");
            String target = boundedRequired(item.targetAddress(), 512, "Evidence target");
            UUID targetId = targetMap.get(target);
            if (targetId == null) throw new DomainException("VALIDATION_FAILED", "Evidence target is not part of the assigned request");
            DiscoveryMethod protocol = parseProtocol(item.protocol());
            if (!request.requestedMethods().contains(protocol)) {
                throw new DomainException("VALIDATION_FAILED", "Evidence protocol was not requested for this discovery task");
            }
            String method = boundedRequired(item.method(), 32, "Evidence method");
            EvidenceStatus status = parseEvidenceStatus(item.status());
            ReachabilityKind reachability = parseReachability(item.reachabilityKind());
            String technicalCode = boundedOptional(item.technicalCode(), 128, "Evidence technical code");
            validateEvidenceSemantics(protocol, status, reachability, technicalCode);
            if (item.durationMs() < 0) throw new DomainException("VALIDATION_FAILED", "Evidence duration cannot be negative");
            Map<String, Object> attrs = item.attributes() == null ? Map.of() : Map.copyOf(item.attributes());
            Map<String, Object> raw = item.rawEvidence() == null ? Map.of() : Map.copyOf(item.rawEvidence());
            DiscoveryLimits.validateEvidenceValue(attrs);
            DiscoveryLimits.validateEvidenceValue(raw);
            validateIdentityAttributeBounds(attrs);
            String uniqueKey = target + "|" + protocol.name() + "|" + method;
            if (!evidenceKeys.add(uniqueKey)) {
                throw new DomainException("VALIDATION_FAILED", "Duplicate evidence method for discovery target");
            }

            evidenceList.add(new DiscoveryEvidence(
                    UuidV7.generate(), tenantId, requestId, targetId, sensorId, protocol, method, status,
                    reachability, technicalCode, attrs, raw, item.durationMs(), now));
            perTarget.computeIfAbsent(target, ignored -> new ArrayList<>()).add(item);
        }

        repository.recordEvidence(tenantId, requestId, evidenceList);
        int candidateCount = aggregateCandidates(request, perTarget, now);
        int reachedCount = (int) perTarget.values().stream().filter(DiscoveryService::hasSuccessfulReachabilityEvidence).count();
        Instant completedAt = Instant.now();
        long durationMs = request.startedAt() == null ? 0 : Math.max(0, Duration.between(request.startedAt(), completedAt).toMillis());
        Map<String, Object> summary = Map.of(
                "targetCount", targets.size(), "probedCount", perTarget.size(), "reachedCount", reachedCount,
                "candidateCount", candidateCount, "evidenceCount", evidenceList.size(), "durationMs", durationMs,
                "sensorId", sensorId, "assignmentVersion", assignmentVersion
        );
        repository.updateStatus(tenantId, requestId, finalStatus, null, completedAt, boundedFailure, summary);
        log.info("Discovery result accepted: requestId={}, tenantId={}, sensorId={}, status={}, evidenceCount={}, candidateCount={}",
                requestId, tenantId, sensorId, finalStatus, evidenceList.size(), candidateCount);
        return new ResultIngestOutcome(finalStatus, false, tenantId);
    }

    private int aggregateCandidates(DiscoveryRequest request, Map<String, List<SensorEvidenceItem>> perTarget, Instant now) {
        int candidateCount = 0;
        for (Map.Entry<String, List<SensorEvidenceItem>> entry : perTarget.entrySet()) {
            List<SensorEvidenceItem> items = entry.getValue();
            boolean icmpSuccess = items.stream().anyMatch(i -> "ICMP".equalsIgnoreCase(i.protocol()) && "SUCCESS".equalsIgnoreCase(i.status()));
            boolean snmpSuccess = items.stream().anyMatch(i -> "SNMP".equalsIgnoreCase(i.protocol()) && "SUCCESS".equalsIgnoreCase(i.status()));
            if (!icmpSuccess && !snmpSuccess) continue;

            List<DiscoveryMethod> protocols = new ArrayList<>();
            if (icmpSuccess) protocols.add(DiscoveryMethod.ICMP);
            if (snmpSuccess) protocols.add(DiscoveryMethod.SNMP);
            String sysDescr = null;
            String sysObjectId = null;
            String sysName = null;
            Map<String, Object> merged = new HashMap<>();
            for (SensorEvidenceItem item : items) {
                Map<String, Object> attrs = item.attributes();
                if (attrs == null) continue;
                merged.putAll(attrs);
                sysDescr = firstString(attrs, sysDescr, "sysDescr", "sys_descr");
                sysObjectId = firstString(attrs, sysObjectId, "sysObjectID", "sys_object_id", "sysObjectId");
                sysName = firstString(attrs, sysName, "sysName", "sys_name");
            }
            DiscoveryConfidence confidence = snmpSuccess && sysObjectId != null
                    ? DiscoveryConfidence.HIGH : (icmpSuccess ? DiscoveryConfidence.MEDIUM : DiscoveryConfidence.INCONCLUSIVE);
            DiscoveryCandidate candidate = new DiscoveryCandidate(
                    UuidV7.generate(), request.tenantId(), request.requestId(), entry.getKey(),
                    sysName != null ? sysName : entry.getKey(), sysObjectId, sysDescr, protocols, merged,
                    confidence, CandidateReviewStatus.PENDING_REVIEW, false, null, request.siteId(),
                    now, now, now, now
            );
            repository.upsertCandidate(candidate);
            candidateCount++;
        }
        return candidateCount;
    }

    private void ensureIdempotentEquivalent(DiscoveryRequest existing, List<String> targets, String name,
                                             List<DiscoveryMethod> methods, String credentialRef, UUID siteId,
                                             UUID sensorPoolId, int port, int timeout, int retries) {
        Set<String> existingTargets = repository.listTargets(existing.tenantId(), existing.requestId()).stream()
                .map(DiscoveryTarget::targetAddress).collect(Collectors.toCollection(LinkedHashSet::new));
        if (!Objects.equals(existing.name(), name) || !new HashSet<>(existingTargets).equals(new HashSet<>(targets)) ||
                !existing.requestedMethods().equals(methods) || !Objects.equals(existing.credentialProfileRef(), credentialRef) ||
                !Objects.equals(existing.siteId(), siteId) || !Objects.equals(existing.sensorPoolId(), sensorPoolId) ||
                existing.snmpPort() != port || existing.timeoutMs() != timeout || existing.maxRetries() != retries) {
            throw new DomainException("IDEMPOTENCY_CONFLICT", "Idempotency key is already bound to a different discovery request");
        }
    }

    private static List<DiscoveryMethod> normalizeMethods(List<DiscoveryMethod> input) {
        if (input == null || input.isEmpty()) return List.of(DiscoveryMethod.ICMP, DiscoveryMethod.SNMP);
        LinkedHashSet<DiscoveryMethod> unique = new LinkedHashSet<>();
        for (DiscoveryMethod method : input) {
            if (method == null) throw new DomainException("VALIDATION_FAILED", "Discovery method cannot be null");
            unique.add(method);
        }
        return List.copyOf(unique);
    }

    private static int normalizePort(Integer value) {
        if (value == null) return DiscoveryLimits.DEFAULT_SNMP_PORT;
        if (value < 1 || value > 65535) throw new DomainException("VALIDATION_FAILED", "SNMP port must be 1-65535");
        return value;
    }

    private static int normalizeTimeout(Integer value) {
        if (value == null) return DiscoveryLimits.DEFAULT_TIMEOUT_MS;
        if (value < DiscoveryLimits.MIN_TIMEOUT_MS || value > DiscoveryLimits.MAX_TIMEOUT_MS) {
            throw new DomainException("VALIDATION_FAILED", "Discovery timeout is outside defensive execution bounds");
        }
        return value;
    }

    private static int normalizeRetries(Integer value) {
        if (value == null) return DiscoveryLimits.DEFAULT_RETRIES;
        if (value < 0 || value > DiscoveryLimits.MAX_RETRIES) {
            throw new DomainException("VALIDATION_FAILED", "Discovery retry count is outside defensive execution bounds");
        }
        return value;
    }

    private static DiscoveryMethod parseProtocol(String value) {
        try { return DiscoveryMethod.valueOf(boundedRequired(value, 32, "Evidence protocol").toUpperCase()); }
        catch (IllegalArgumentException ex) { throw new DomainException("VALIDATION_FAILED", "Unsupported discovery evidence protocol"); }
    }

    private static EvidenceStatus parseEvidenceStatus(String value) {
        try { return EvidenceStatus.valueOf(boundedRequired(value, 32, "Evidence status").toUpperCase()); }
        catch (IllegalArgumentException ex) { throw new DomainException("VALIDATION_FAILED", "Unsupported discovery evidence status"); }
    }

    private static ReachabilityKind parseReachability(String value) {
        try { return ReachabilityKind.valueOf(boundedRequired(value, 32, "Evidence reachability").toUpperCase()); }
        catch (IllegalArgumentException ex) { throw new DomainException("VALIDATION_FAILED", "Unsupported discovery reachability kind"); }
    }

    private static DiscoveryStatus parseFinalStatus(String value) {
        String normalized = boundedRequired(value, 32, "Discovery result status").toUpperCase();
        if ("COMPLETED".equals(normalized)) return DiscoveryStatus.COMPLETED;
        if ("FAILED".equals(normalized)) return DiscoveryStatus.FAILED;
        throw new DomainException("VALIDATION_FAILED", "Sensor result status must be COMPLETED or FAILED");
    }

    private static void validateEvidenceSemantics(DiscoveryMethod protocol, EvidenceStatus status,
                                                   ReachabilityKind reachability, String technicalCode) {
        String code = technicalCode == null ? "" : technicalCode.toUpperCase();
        if (code.contains("DEVICE_DOWN") || code.contains("FIREWALL_BLOCKED") || code.contains("WRONG_CREDENTIAL")) {
            throw new DomainException("VALIDATION_FAILED", "Discovery evidence contains prohibited causal conclusion");
        }
        if (code.contains("TIMEOUT") || code.contains("NO_RESPONSE") || code.contains("RECEIVER_UNAVAILABLE") ||
                code.contains("CREDENTIAL_RESOLVER_UNAVAILABLE")) {
            if (status != EvidenceStatus.INCONCLUSIVE || reachability != ReachabilityKind.UNKNOWN) {
                throw new DomainException("VALIDATION_FAILED", "Timeout/unavailable discovery evidence must remain INCONCLUSIVE/UNKNOWN");
            }
        }
        if (protocol == DiscoveryMethod.ICMP && status == EvidenceStatus.FAILED && reachability == ReachabilityKind.OBSERVED) {
            throw new DomainException("VALIDATION_FAILED", "ICMP failure cannot assert observed device failure");
        }
        if (code.contains("AUTHENTICATION_FAILED") && (protocol != DiscoveryMethod.SNMP || status != EvidenceStatus.FAILED)) {
            throw new DomainException("VALIDATION_FAILED", "Authentication failure requires failed SNMP authentication evidence");
        }
    }

    private static void validateIdentityAttributeBounds(Map<String, Object> attrs) {
        validateAttr(attrs, 255, "sysName", "sys_name", "sysObjectID", "sysObjectId", "sys_object_id");
        validateAttr(attrs, 2000, "sysDescr", "sys_descr");
    }

    private static void validateAttr(Map<String, Object> attrs, int max, String... keys) {
        for (String key : keys) {
            Object value = attrs.get(key);
            if (value != null && String.valueOf(value).length() > max) {
                throw new DomainException("QUERY_LIMIT_EXCEEDED", "Discovery identity evidence exceeds defensive text bound");
            }
        }
    }

    private static String firstString(Map<String, Object> attrs, String current, String... keys) {
        if (current != null) return current;
        for (String key : keys) {
            Object value = attrs.get(key);
            if (value != null) return String.valueOf(value);
        }
        return null;
    }

    private static boolean hasSuccessfulReachabilityEvidence(List<SensorEvidenceItem> items) {
        return items.stream().anyMatch(i -> "SUCCESS".equalsIgnoreCase(i.status()) &&
                ("ICMP".equalsIgnoreCase(i.protocol()) || "SNMP".equalsIgnoreCase(i.protocol())));
    }

    private static String boundedRequired(String value, int max, String label) {
        if (value == null || value.trim().isEmpty()) throw new DomainException("VALIDATION_FAILED", label + " is required");
        String clean = value.trim();
        if (clean.length() > max) throw new DomainException("VALIDATION_FAILED", label + " exceeds maximum length");
        return clean;
    }

    private static String boundedOptional(String value, int max, String label) {
        if (value == null || value.trim().isEmpty()) return null;
        String clean = value.trim();
        if (clean.length() > max) throw new DomainException("VALIDATION_FAILED", label + " exceeds maximum length");
        return clean;
    }

    public record DiscoveryTaskPayload(
            String requestId,
            String tenantId,
            String correlationId,
            long assignmentVersion,
            List<String> targets,
            List<String> requestedMethods,
            String credentialProfileRef,
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

    public record ResultIngestOutcome(DiscoveryStatus status, boolean replay, UUID tenantId) {}
}