package com.platform.core.controller;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.SensorIngestionService;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;

@RestController
@RequestMapping("/api/v1")
public class SensorIngestionController {

    private static final Logger log = LoggerFactory.getLogger(SensorIngestionController.class);

    private final SensorIngestionService ingestionService;
    private final ObjectMapper objectMapper;

    public SensorIngestionController() {
        this.ingestionService = null;
        this.objectMapper = new ObjectMapper().findAndRegisterModules();
    }

    @Autowired
    public SensorIngestionController(
            @Autowired(required = false) SensorIngestionService ingestionService,
            ObjectMapper objectMapper
    ) {
        this.ingestionService = ingestionService;
        this.objectMapper = objectMapper != null ? objectMapper : new ObjectMapper();
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

        TelemetryBatchRequest batchRequest;
        try {
            batchRequest = objectMapper.readValue(rawBody, TelemetryBatchRequest.class);
        } catch (Exception e) {
            log.warn("Malformed telemetry batch JSON from remote={}: {}", request.getRemoteAddr(), e.getMessage());
            return ResponseEntity.badRequest().body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_MALFORMED_JSON",
                    "message", "Failed to parse telemetry batch JSON: " + e.getMessage()
            ));
        }

        if (ingestionService != null) {
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

        String commonName = extractCommonName(clientCert.getSubjectX500Principal().getName());
        return ResponseEntity.ok(new TelemetryAckResponse("ACCEPTED", batchRequest.lastSequence(), commonName));
    }
}
