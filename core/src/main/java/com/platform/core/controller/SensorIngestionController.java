package com.platform.core.controller;

import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
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
}
