package com.platform.core.controller;
 
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.discovery.DiscoveryService.DiscoveryTaskPayload;
import com.platform.core.domain.discovery.DiscoveryService.SensorEvidenceItem;
import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

@RestController
@RequestMapping("/api/v1")
public class SensorIngestionController {

    private static final Logger log = LoggerFactory.getLogger(SensorIngestionController.class);

    private final DiscoveryService discoveryService;

    public SensorIngestionController() {
        this.discoveryService = null;
    }

    @org.springframework.beans.factory.annotation.Autowired
    public SensorIngestionController(DiscoveryService discoveryService) {
        this.discoveryService = discoveryService;
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
                    "message", "mTLS client certificate authentication required for sensor ingestion"
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

    @PostMapping("/telemetry")
    public ResponseEntity<Map<String, Object>> ingestTelemetry(
            @RequestBody(required = false) Map<String, Object> payload,
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

        String commonName = extractCommonName(clientCert.getSubjectX500Principal().getName());
        log.info("Ingesting telemetry batch from sensor: cn={}, remote={}", commonName, request.getRemoteAddr());

        return ResponseEntity.ok(Map.of(
                "status", "ACCEPTED",
                "sensor_identity", commonName,
                "timestamp", Instant.now().toString()
        ));
    }

    @PostMapping("/sensor/discovery/tasks")
    public ResponseEntity<Map<String, Object>> fetchDiscoveryTask(
            @RequestBody(required = false) Map<String, Object> payload,
            HttpServletRequest request
    ) {
        X509Certificate clientCert = extractClientCertificate(request);
        if (clientCert == null) {
            log.warn("Rejected unauthenticated discovery task poll: missing client certificate from remote={}", request.getRemoteAddr());
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_AUTH_CLIENT_CERT_REQUIRED",
                    "message", "mTLS client certificate authentication required for sensor discovery task fetch"
            ));
        }

        String commonName = extractCommonName(clientCert.getSubjectX500Principal().getName());
        String sensorId = payload != null && payload.containsKey("sensor_id")
                ? String.valueOf(payload.get("sensor_id"))
                : commonName;

        if (discoveryService == null) {
            return ResponseEntity.ok(Map.of("status", "NO_TASK", "has_task", false));
        }

        Optional<DiscoveryTaskPayload> taskOpt = discoveryService.fetchTaskForSensor(sensorId);
        Map<String, Object> resp = new LinkedHashMap<>();
        if (taskOpt.isPresent()) {
            DiscoveryTaskPayload task = taskOpt.get();
            resp.put("status", "ASSIGNED");
            resp.put("has_task", true);
            resp.put("request_id", task.requestId());
            resp.put("tenant_id", task.tenantId());
            resp.put("correlation_id", task.correlationId());
            resp.put("targets", task.targets());
            resp.put("requested_methods", task.requestedMethods());
            resp.put("snmp_port", task.snmpPort());
            resp.put("timeout_ms", task.timeoutMs());
            resp.put("max_retries", task.maxRetries());
        } else {
            resp.put("status", "NO_TASK");
            resp.put("has_task", false);
        }
        return ResponseEntity.ok(resp);
    }

    @PostMapping("/sensor/discovery/results")
    public ResponseEntity<Map<String, Object>> ingestDiscoveryResults(
            @RequestBody Map<String, Object> payload,
            HttpServletRequest request
    ) {
        X509Certificate clientCert = extractClientCertificate(request);
        if (clientCert == null) {
            log.warn("Rejected unauthenticated discovery result submission: missing client certificate from remote={}", request.getRemoteAddr());
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_AUTH_CLIENT_CERT_REQUIRED",
                    "message", "mTLS client certificate authentication required for sensor discovery ingestion"
            ));
        }

        String commonName = extractCommonName(clientCert.getSubjectX500Principal().getName());
        String sensorId = payload.containsKey("sensor_id") ? String.valueOf(payload.get("sensor_id")) : commonName;
        UUID tenantId = UUID.fromString(String.valueOf(payload.get("tenant_id")));
        UUID requestId = UUID.fromString(String.valueOf(payload.get("request_id")));
        String overallStatus = payload.containsKey("status") ? String.valueOf(payload.get("status")) : "COMPLETED";
        String failureReason = payload.containsKey("failure_reason") ? String.valueOf(payload.get("failure_reason")) : null;

        if (discoveryService == null) {
            return ResponseEntity.status(HttpStatus.SERVICE_UNAVAILABLE).body(Map.of(
                    "status", "ERROR",
                    "message", "Discovery service unavailable"
            ));
        }

        List<SensorEvidenceItem> items = new ArrayList<>();
        if (payload.get("evidence") instanceof List<?> rawList) {
            for (Object raw : rawList) {
                if (raw instanceof Map<?, ?> m) {
                    String target = String.valueOf(m.get("target"));
                    String protocol = String.valueOf(m.get("protocol"));
                    String method = m.containsKey("method") ? String.valueOf(m.get("method")) : protocol;
                    String status = String.valueOf(m.get("status"));
                    String reachability = m.containsKey("reachability_kind") ? String.valueOf(m.get("reachability_kind")) : "UNKNOWN";
                    String techCode = m.containsKey("technical_code") ? String.valueOf(m.get("technical_code")) : null;
                    @SuppressWarnings("unchecked")
                    Map<String, Object> attrs = m.containsKey("attributes") && m.get("attributes") instanceof Map<?, ?>
                            ? (Map<String, Object>) m.get("attributes") : Map.of();
                    @SuppressWarnings("unchecked")
                    Map<String, Object> rawEv = m.containsKey("raw_evidence") && m.get("raw_evidence") instanceof Map<?, ?>
                            ? (Map<String, Object>) m.get("raw_evidence") : Map.of();
                    long duration = m.containsKey("duration_ms") && m.get("duration_ms") instanceof Number n
                            ? n.longValue() : 0L;

                    items.add(new SensorEvidenceItem(
                            target, protocol, method, status, reachability, techCode, attrs, rawEv, duration
                    ));
                }
            }
        }

        discoveryService.ingestResultsFromSensor(sensorId, tenantId, requestId, items, overallStatus, failureReason);

        return ResponseEntity.ok(Map.of(
                "status", "ACCEPTED",
                "request_id", requestId.toString(),
                "sensor_identity", commonName,
                "received_at", Instant.now().toString()
        ));
    }
}
