package com.platform.core.controller;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.discovery.DiscoveryLimits;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.discovery.DiscoveryService.DiscoveryTaskPayload;
import com.platform.core.domain.discovery.DiscoveryService.ResultIngestOutcome;
import com.platform.core.domain.discovery.DiscoveryService.SensorEvidenceItem;
import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import javax.naming.InvalidNameException;
import javax.naming.ldap.LdapName;
import javax.naming.ldap.Rdn;
import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
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

    private final DiscoveryService discoveryService;

    public SensorIngestionController(DiscoveryService discoveryService) {
        this.discoveryService = discoveryService;
    }

    private X509Certificate extractClientCertificate(HttpServletRequest request) {
        Object certAttr = request.getAttribute("jakarta.servlet.request.X509Certificate");
        if (certAttr instanceof X509Certificate[] certs && certs.length > 0) return certs[0];
        return null;
    }

    private String extractCommonName(X509Certificate certificate) {
        if (certificate == null) return null;
        try {
            LdapName name = new LdapName(certificate.getSubjectX500Principal().getName());
            for (Rdn rdn : name.getRdns()) {
                if ("CN".equalsIgnoreCase(rdn.getType())) {
                    String value = String.valueOf(rdn.getValue()).trim();
                    return value.isEmpty() || value.length() > 128 ? null : value;
                }
            }
        } catch (InvalidNameException ignored) {
            return null;
        }
        return null;
    }

    private String authenticatedSensorId(HttpServletRequest request, String operation) {
        X509Certificate cert = extractClientCertificate(request);
        String sensorId = extractCommonName(cert);
        if (cert == null || sensorId == null) {
            log.warn("Rejected unauthenticated Sensor operation: operation={}, remote={}", operation, request.getRemoteAddr());
            return null;
        }
        return sensorId;
    }

    private ResponseEntity<Map<String, Object>> unauthorized(String message) {
        return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of(
                "status", "ERROR", "code", "ERR_AUTH_CLIENT_CERT_REQUIRED", "message", message));
    }

    @RequestMapping(value = "/sensor/heartbeat", method = {RequestMethod.GET, RequestMethod.POST})
    public ResponseEntity<Map<String, Object>> sensorHeartbeat(
            @RequestBody(required = false) Map<String, Object> payload, HttpServletRequest request) {
        X509Certificate clientCert = extractClientCertificate(request);
        String commonName = extractCommonName(clientCert);
        if (clientCert == null || commonName == null) return unauthorized("Valid mTLS Sensor identity is required");
        String subjectDn = clientCert.getSubjectX500Principal().getName();
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
        String sensorId = authenticatedSensorId(request, "status");
        if (sensorId == null) return unauthorized("Valid mTLS Sensor identity is required");
        return ResponseEntity.ok(Map.of("status", "UP", "authenticated", true, "sensor_identity", sensorId));
    }

    @PostMapping("/telemetry")
    public ResponseEntity<Map<String, Object>> ingestTelemetry(
            @RequestBody(required = false) Map<String, Object> payload, HttpServletRequest request) {
        String sensorId = authenticatedSensorId(request, "telemetry");
        if (sensorId == null) return unauthorized("Valid mTLS Sensor identity is required for telemetry ingestion");
        log.info("Ingesting telemetry batch from authenticated sensor: cn={}, remote={}", sensorId, request.getRemoteAddr());
        return ResponseEntity.ok(Map.of(
                "status", "ACCEPTED", "sensor_identity", sensorId, "timestamp", Instant.now().toString()));
    }

    @PostMapping("/sensor/discovery/tasks")
    public ResponseEntity<Map<String, Object>> fetchDiscoveryTask(HttpServletRequest request) {
        String sensorId = authenticatedSensorId(request, "discovery-task-fetch");
        if (sensorId == null) return unauthorized("Valid mTLS Sensor identity is required for discovery task fetch");

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
        if (task.credentialProfileRef() != null) resp.put("credential_profile_ref", task.credentialProfileRef());
        resp.put("snmp_port", task.snmpPort());
        resp.put("timeout_ms", task.timeoutMs());
        resp.put("max_retries", task.maxRetries());
        return ResponseEntity.ok(resp);
    }

    @PostMapping("/sensor/discovery/results")
    public ResponseEntity<Map<String, Object>> ingestDiscoveryResults(
            @RequestBody Map<String, Object> payload, HttpServletRequest request) {
        String sensorId = authenticatedSensorId(request, "discovery-result");
        if (sensorId == null) return unauthorized("Valid mTLS Sensor identity is required for discovery result ingestion");
        if (payload == null) throw new DomainException("VALIDATION_FAILED", "Discovery result payload is required");
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
            if (!allowed.contains(key)) throw new DomainException("VALIDATION_FAILED", "Unknown " + label + " field: " + key);
        }
    }

    private static Map<String, Object> stringKeyMap(Map<?, ?> input) {
        Map<String, Object> out = new LinkedHashMap<>();
        for (Map.Entry<?, ?> entry : input.entrySet()) {
            if (!(entry.getKey() instanceof String key)) throw new DomainException("VALIDATION_FAILED", "JSON object key must be text");
            out.put(key, entry.getValue());
        }
        return out;
    }

    private static Map<String, Object> mapValue(Object value, String field) {
        if (value == null) return Map.of();
        if (!(value instanceof Map<?, ?> map)) throw new DomainException("VALIDATION_FAILED", field + " must be an object");
        return stringKeyMap(map);
    }

    private static UUID parseUuid(Object value, String field) {
        try { return UUID.fromString(requiredString(value, field)); }
        catch (IllegalArgumentException ex) { throw new DomainException("VALIDATION_FAILED", field + " must be a UUID"); }
    }

    private static long parsePositiveLong(Object value, String field) {
        long result = parseNonNegativeLong(value, field);
        if (result <= 0) throw new DomainException("VALIDATION_FAILED", field + " must be greater than zero");
        return result;
    }

    private static long parseNonNegativeLong(Object value, String field) {
        if (!(value instanceof Number number)) throw new DomainException("VALIDATION_FAILED", field + " must be numeric");
        long result = number.longValue();
        if (result < 0) throw new DomainException("VALIDATION_FAILED", field + " cannot be negative");
        return result;
    }

    private static String requiredString(Object value, String field) {
        String out = optionalString(value);
        if (out == null) throw new DomainException("VALIDATION_FAILED", field + " is required");
        return out;
    }

    private static String optionalString(Object value) {
        if (value == null) return null;
        if (!(value instanceof String s)) throw new DomainException("VALIDATION_FAILED", "Expected text value");
        String out = s.trim();
        return out.isEmpty() ? null : out;
    }
}