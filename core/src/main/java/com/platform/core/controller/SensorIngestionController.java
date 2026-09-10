package com.platform.core.controller;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.discovery.DiscoveryLimits;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.discovery.DiscoveryService.DiscoveryTaskPayload;
import com.platform.core.domain.discovery.DiscoveryService.ResultIngestOutcome;
import com.platform.core.domain.discovery.DiscoveryService.SensorEvidenceItem;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.IngestionValidator;
import com.platform.core.ingestion.SensorIngestionService;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.math.BigDecimal;
import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;

@RestController
@RequestMapping("/api/v1")
public class SensorIngestionController {

    private static final Logger log = LoggerFactory.getLogger(SensorIngestionController.class);
    private static final Set<String> RESULT_KEYS = Set.of(
            "request_id", "assignment_version", "status", "failure_reason", "evidence");
    private static final Set<String> EVIDENCE_KEYS = Set.of(
            "target", "protocol", "method", "status", "reachability_kind", "technical_code",
            "attributes", "raw_evidence", "duration_ms");

    private final SensorIngestionService ingestionService;
    private final ObjectMapper objectMapper;
    private final DiscoveryService discoveryService;
    private final SensorAuthorityResolver authorityResolver;

    @Autowired
    public SensorIngestionController(
            SensorIngestionService ingestionService,
            ObjectMapper objectMapper,
            DiscoveryService discoveryService,
            SensorAuthorityResolver authorityResolver
    ) {
        this.ingestionService = Objects.requireNonNull(ingestionService, "ingestionService must not be null");
        this.objectMapper = objectMapper != null ? objectMapper : new ObjectMapper();
        this.discoveryService = Objects.requireNonNull(discoveryService, "discoveryService must not be null");
        this.authorityResolver = Objects.requireNonNull(authorityResolver, "authorityResolver must not be null");
    }

    private X509Certificate extractClientCertificate(HttpServletRequest request) {
        Object certAttr = request.getAttribute("jakarta.servlet.request.X509Certificate");
        if (certAttr instanceof X509Certificate[] certs && certs.length > 0) {
            return certs[0];
        }
        return null;
    }

    private String extractCommonName(String dn) {
        if (dn == null) return "UNKNOWN";
        for (String part : dn.split(",")) {
            part = part.trim();
            if (part.startsWith("CN=") || part.startsWith("cn=")) {
                return part.substring(3);
            }
        }
        return dn;
    }

    private ResponseEntity<Map<String, Object>> clientCertRequired(String message) {
        return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of(
                "status", "ERROR",
                "code", "ERR_AUTH_CLIENT_CERT_REQUIRED",
                "message", message
        ));
    }

    private ResponseEntity<Map<String, Object>> ingestionError(IngestionException e) {
        return ResponseEntity.status(e.getStatus()).body(Map.of(
                "status", "ERROR",
                "code", e.getCode(),
                "message", e.getMessage()
        ));
    }

    /**
     * Discovery machine endpoints require authoritative Sensor Control/enrollment resolution.
     * Certificate presence alone (CN) is not Sensor authority. Payload sensor/tenant claims are ignored.
     * When enrollment is unavailable the resolver fails closed.
     */
    private SensorScope resolveDiscoverySensorAuthority(X509Certificate clientCert, String operation) {
        SensorScope scope = authorityResolver.resolve(clientCert);
        if (scope == null || scope.sensorId() == null || scope.sensorId().isBlank()) {
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "Authoritative Sensor Scope is unavailable; discovery machine transport is fail-closed"
            );
        }
        log.debug("Resolved discovery Sensor authority: operation={}, sensorId={}, tenantId={}",
                operation, scope.sensorId(), scope.tenantId());
        return scope;
    }

    @RequestMapping(value = "/sensor/heartbeat", method = {RequestMethod.GET, RequestMethod.POST})
    public ResponseEntity<Map<String, Object>> sensorHeartbeat(
            @RequestBody(required = false) Map<String, Object> payload,
            HttpServletRequest request) {
        X509Certificate clientCert = extractClientCertificate(request);
        if (clientCert == null) {
            log.warn("Rejected unauthenticated sensor heartbeat: missing client certificate from remote={}", request.getRemoteAddr());
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_AUTH_CLIENT_CERT_REQUIRED",
                    "message", "mTLS client certificate authentication required for sensor transport"
            ));
        }

        String subjectDn = clientCert.getSubjectX500Principal().getName();
        String commonName = extractCommonName(subjectDn);
        String serialNumber = clientCert.getSerialNumber().toString(16);

        log.info("Authenticated sensor heartbeat: cn={}, serial={}, remote={}", commonName, serialNumber, request.getRemoteAddr());

        Map<String, Object> resp = new LinkedHashMap<>();
        resp.put("status", "ACCEPTED");
        resp.put("authenticated", true);
        resp.put("sensor_identity", commonName);
        resp.put("subject_dn", subjectDn);
        resp.put("received_at", Instant.now().toString());
        return ResponseEntity.ok(resp);
    }

    @GetMapping("/sensor/status")
    public ResponseEntity<Map<String, Object>> sensorStatus(HttpServletRequest request) {
        X509Certificate clientCert = extractClientCertificate(request);
        if (clientCert == null) {
            log.warn("Rejected unauthenticated sensor status probe from remote={}", request.getRemoteAddr());
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_AUTH_CLIENT_CERT_REQUIRED",
                    "message", "mTLS client certificate authentication required for sensor transport"
            ));
        }

        String commonName = extractCommonName(clientCert.getSubjectX500Principal().getName());
        return ResponseEntity.ok(Map.of(
                "status", "UP",
                "authenticated", true,
                "sensor_identity", commonName
        ));
    }

    @PostMapping(value = "/telemetry", consumes = "application/json", produces = "application/json")
    public ResponseEntity<?> ingestTelemetry(
            @RequestHeader(value = "Idempotency-Key", required = false) String idempotencyKey,
            @RequestHeader(value = "X-Correlation-ID", required = false) String correlationId,
            @RequestBody(required = false) byte[] rawBody,
            HttpServletRequest request) {
        X509Certificate clientCert = extractClientCertificate(request);
        if (clientCert == null) {
            log.warn("Rejected unauthenticated telemetry batch: missing client certificate from remote={}", request.getRemoteAddr());
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_AUTH_CLIENT_CERT_REQUIRED",
                    "message", "mTLS client certificate authentication required for telemetry ingestion"
            ));
        }

        if (rawBody == null || rawBody.length == 0) {
            return ResponseEntity.badRequest().body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_EMPTY_BODY",
                    "message", "Telemetry request body must not be empty"
            ));
        }

        if (rawBody.length > IngestionValidator.MAX_BATCH_BYTES) {
            log.warn("Telemetry batch body exceeds max size: {} > {}", rawBody.length, IngestionValidator.MAX_BATCH_BYTES);
            return ResponseEntity.status(HttpStatus.PAYLOAD_TOO_LARGE).body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_BATCH_TOO_LARGE",
                    "message", "Telemetry batch body exceeds maximum allowed size"
            ));
        }

        TelemetryBatchRequest batchRequest;
        try {
            batchRequest = objectMapper.readValue(rawBody, TelemetryBatchRequest.class);
        } catch (Exception e) {
            log.warn("Malformed telemetry batch JSON from remote={}: {}", request.getRemoteAddr(), e.getMessage());
            return ResponseEntity.badRequest().body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_MALFORMED_JSON",
                    "message", "Failed to parse telemetry batch JSON"
            ));
        }

        try {
            TelemetryAckResponse ack = ingestionService.ingestTelemetry(rawBody, batchRequest, idempotencyKey, clientCert);
            return ResponseEntity.ok(ack);
        } catch (IngestionException e) {
            log.warn("Ingestion rejected: code={}, message={}", e.getCode(), e.getMessage());
            return ResponseEntity.status(e.getStatus()).body(Map.of(
                    "status", "ERROR",
                    "code", e.getCode(),
                    "message", e.getMessage()
            ));
        } catch (Exception e) {
            log.error("Ingestion failed unexpectedly: {}", e.getMessage(), e);
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR).body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_INTERNAL_ERROR",
                    "message", "Internal server error processing telemetry"
            ));
        }
    }

    @PostMapping("/sensor/discovery/tasks")
    public ResponseEntity<Map<String, Object>> fetchDiscoveryTask(HttpServletRequest request) {
        X509Certificate clientCert = extractClientCertificate(request);
        if (clientCert == null) {
            log.warn("Rejected unauthenticated Sensor operation: operation=discovery-task-fetch, remote={}",
                    request.getRemoteAddr());
            return clientCertRequired("mTLS client certificate authentication required for discovery task fetch");
        }

        final SensorScope scope;
        try {
            scope = resolveDiscoverySensorAuthority(clientCert, "discovery-task-fetch");
        } catch (IngestionException e) {
            log.warn("Discovery task fetch rejected: code={}, message={}", e.getCode(), e.getMessage());
            return ingestionError(e);
        }

        String sensorId = scope.sensorId();
        Optional<DiscoveryTaskPayload> taskOpt = discoveryService.fetchTaskForSensor(sensorId);
        Map<String, Object> resp = new LinkedHashMap<>();
        if (taskOpt.isEmpty()) {
            resp.put("status", "NO_TASK");
            resp.put("has_task", false);
            return ResponseEntity.ok(resp);
        }
        DiscoveryTaskPayload task = taskOpt.get();
        resp.put("status", "ASSIGNED");
        resp.put("has_task", true);
        resp.put("request_id", task.requestId());
        resp.put("tenant_id", task.tenantId());
        resp.put("correlation_id", task.correlationId());
        resp.put("assignment_version", task.assignmentVersion());
        resp.put("targets", task.targets());
        resp.put("requested_methods", task.requestedMethods());
        if (task.credentialProfileRef() != null) {
            resp.put("credential_profile_ref", task.credentialProfileRef());
        }
        resp.put("snmp_port", task.snmpPort());
        resp.put("timeout_ms", task.timeoutMs());
        resp.put("max_retries", task.maxRetries());
        return ResponseEntity.ok(resp);
    }

    @PostMapping("/sensor/discovery/results")
    public ResponseEntity<Map<String, Object>> ingestDiscoveryResults(
            @RequestBody Map<String, Object> payload, HttpServletRequest request) {
        X509Certificate clientCert = extractClientCertificate(request);
        if (clientCert == null) {
            log.warn("Rejected unauthenticated Sensor operation: operation=discovery-result, remote={}",
                    request.getRemoteAddr());
            return clientCertRequired("mTLS client certificate authentication required for discovery result ingestion");
        }

        final SensorScope scope;
        try {
            scope = resolveDiscoverySensorAuthority(clientCert, "discovery-result");
        } catch (IngestionException e) {
            log.warn("Discovery result ingestion rejected: code={}, message={}", e.getCode(), e.getMessage());
            return ingestionError(e);
        }

        String sensorId = scope.sensorId();
        if (payload == null) {
            throw new DomainException("VALIDATION_FAILED", "Discovery result payload is required");
        }
        rejectUnknownKeys(payload, RESULT_KEYS, "discovery result");

        UUID requestId = parseUuid(payload.get("request_id"), "request_id");
        long assignmentVersion = parsePositiveLong(payload.get("assignment_version"), "assignment_version");
        String overallStatus = requiredString(payload.get("status"), "status");
        String failureReason = optionalString(payload.get("failure_reason"));

        Object rawEvidence = payload.get("evidence");
        if (!(rawEvidence instanceof List<?> rawList)) {
            throw new DomainException("VALIDATION_FAILED", "Discovery evidence must be an array");
        }
        if (rawList.size() > DiscoveryLimits.MAX_EVIDENCE_ITEMS) {
            throw new DomainException("QUERY_LIMIT_EXCEEDED", "Discovery evidence count exceeds defensive limit");
        }
        List<SensorEvidenceItem> items = new ArrayList<>(rawList.size());
        for (Object raw : rawList) {
            if (!(raw instanceof Map<?, ?> wildcard)) {
                throw new DomainException("VALIDATION_FAILED", "Discovery evidence entries must be objects");
            }
            Map<String, Object> evidence = stringKeyMap(wildcard);
            rejectUnknownKeys(evidence, EVIDENCE_KEYS, "discovery evidence");
            Map<String, Object> attrs = mapValue(evidence.get("attributes"), "attributes");
            Map<String, Object> rawEv = mapValue(evidence.get("raw_evidence"), "raw_evidence");
            items.add(new SensorEvidenceItem(
                    requiredString(evidence.get("target"), "target"),
                    requiredString(evidence.get("protocol"), "protocol"),
                    requiredString(evidence.get("method"), "method"),
                    requiredString(evidence.get("status"), "status"),
                    requiredString(evidence.get("reachability_kind"), "reachability_kind"),
                    optionalString(evidence.get("technical_code")), attrs, rawEv,
                    parseNonNegativeLong(evidence.get("duration_ms"), "duration_ms")
            ));
        }

        ResultIngestOutcome outcome = discoveryService.ingestResultsFromSensor(
                sensorId, requestId, assignmentVersion, items, overallStatus, failureReason);
        Map<String, Object> response = new LinkedHashMap<>();
        response.put("status", outcome.replay() ? "ALREADY_ACCEPTED" : "ACCEPTED");
        response.put("request_id", requestId.toString());
        response.put("sensor_identity", sensorId);
        response.put("received_at", Instant.now().toString());
        return ResponseEntity.ok(response);
    }

    private static void rejectUnknownKeys(Map<String, Object> map, Set<String> allowed, String label) {
        for (String key : map.keySet()) {
            if (!allowed.contains(key)) {
                throw new DomainException("VALIDATION_FAILED", "Unknown " + label + " field: " + key);
            }
        }
    }

    private static Map<String, Object> stringKeyMap(Map<?, ?> input) {
        Map<String, Object> out = new LinkedHashMap<>();
        for (Map.Entry<?, ?> entry : input.entrySet()) {
            if (!(entry.getKey() instanceof String key)) {
                throw new DomainException("VALIDATION_FAILED", "JSON object key must be text");
            }
            out.put(key, entry.getValue());
        }
        return out;
    }

    private static Map<String, Object> mapValue(Object value, String field) {
        if (value == null) {
            return Map.of();
        }
        if (!(value instanceof Map<?, ?> map)) {
            throw new DomainException("VALIDATION_FAILED", field + " must be an object");
        }
        return stringKeyMap(map);
    }

    private static UUID parseUuid(Object value, String field) {
        try {
            return UUID.fromString(requiredString(value, field));
        } catch (IllegalArgumentException ex) {
            throw new DomainException("VALIDATION_FAILED", field + " must be a UUID");
        }
    }

    private static long parsePositiveLong(Object value, String field) {
        long result = parseNonNegativeLong(value, field);
        if (result <= 0) {
            throw new DomainException("VALIDATION_FAILED", field + " must be greater than zero");
        }
        return result;
    }

    private static long parseNonNegativeLong(Object value, String field) {
        if (!(value instanceof Number number)) {
            throw new DomainException("VALIDATION_FAILED", field + " must be numeric");
        }
        final long result;
        try {
            result = new BigDecimal(number.toString()).longValueExact();
        } catch (NumberFormatException | ArithmeticException ex) {
            throw new DomainException("VALIDATION_FAILED", field + " must be an exact 64-bit integer");
        }
        if (result < 0) {
            throw new DomainException("VALIDATION_FAILED", field + " cannot be negative");
        }
        return result;
    }

    private static String requiredString(Object value, String field) {
        String out = optionalString(value);
        if (out == null) {
            throw new DomainException("VALIDATION_FAILED", field + " is required");
        }
        return out;
    }

    private static String optionalString(Object value) {
        if (value == null) {
            return null;
        }
        if (!(value instanceof String s)) {
            throw new DomainException("VALIDATION_FAILED", "Expected text value");
        }
        String out = s.trim();
        return out.isEmpty() ? null : out;
    }
}
