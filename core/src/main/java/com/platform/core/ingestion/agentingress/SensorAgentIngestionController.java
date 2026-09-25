package com.platform.core.ingestion.agentingress;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryAckResponse;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryBatchRequest;
import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.security.cert.X509Certificate;
import java.util.Map;
import java.util.Objects;

@RestController
@RequestMapping("/api/v1")
public class SensorAgentIngestionController {

    private static final Logger log = LoggerFactory.getLogger(SensorAgentIngestionController.class);

    private final AgentIngestionService ingestionService;
    private final ObjectMapper objectMapper;

    @Autowired
    public SensorAgentIngestionController(
            AgentIngestionService ingestionService,
            ObjectMapper objectMapper
    ) {
        this.ingestionService = Objects.requireNonNull(ingestionService, "ingestionService must not be null");
        this.objectMapper = objectMapper != null ? objectMapper : new ObjectMapper();
    }

    private X509Certificate extractClientCertificate(HttpServletRequest request) {
        Object certAttr = request.getAttribute("jakarta.servlet.request.X509Certificate");
        if (certAttr instanceof X509Certificate[] certs && certs.length > 0) {
            return certs[0];
        }
        return null;
    }

    @PostMapping(value = "/sensor/agent-telemetry", consumes = "application/json", produces = "application/json")
    public ResponseEntity<?> ingestAgentTelemetry(
            @RequestHeader(value = "Idempotency-Key", required = false) String idempotencyKey,
            @RequestBody(required = false) byte[] rawBody,
            HttpServletRequest request
    ) {
        X509Certificate clientCert = extractClientCertificate(request);
        if (clientCert == null) {
            log.warn("Rejected unauthenticated agent-telemetry batch: missing client certificate from remote={}",
                    request.getRemoteAddr());
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_AUTH_CLIENT_CERT_REQUIRED",
                    "message", "mTLS client certificate authentication required for agent-telemetry ingestion"
            ));
        }

        if (rawBody == null || rawBody.length == 0) {
            return ResponseEntity.badRequest().body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_EMPTY_BODY",
                    "message", "Agent telemetry request body must not be empty"
            ));
        }

        if (rawBody.length > AgentIngestionValidator.MAX_BATCH_BYTES) {
            return ResponseEntity.status(HttpStatus.PAYLOAD_TOO_LARGE).body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_BATCH_TOO_LARGE",
                    "message", "Agent telemetry batch body exceeds maximum allowed size"
            ));
        }

        AgentTelemetryBatchRequest batchRequest;
        try {
            batchRequest = objectMapper.readValue(rawBody, AgentTelemetryBatchRequest.class);
        } catch (Exception e) {
            log.warn("Malformed agent-telemetry batch JSON: code=ERR_MALFORMED_JSON, remoteAddr={}, failureClass={}, bodyBytes={}",
                    request.getRemoteAddr(), e.getClass().getSimpleName(), rawBody.length);
            return ResponseEntity.badRequest().body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_MALFORMED_JSON",
                    "message", "Failed to parse agent-telemetry batch JSON"
            ));
        }

        try {
            AgentTelemetryAckResponse ack = ingestionService.ingestAgentTelemetry(
                    rawBody, batchRequest, idempotencyKey, clientCert);
            return ResponseEntity.ok(ack);
        } catch (IngestionException e) {
            log.warn("Agent-telemetry ingestion rejected: code={}, status={} ({}), remoteAddr={}",
                    e.getCode(), e.getStatus().value(), e.getStatus().name(), request.getRemoteAddr());
            return ResponseEntity.status(e.getStatus()).body(Map.of(
                    "status", "ERROR",
                    "code", e.getCode(),
                    "message", e.getMessage()
            ));
        } catch (Exception e) {
            log.error("Agent-telemetry ingestion failed unexpectedly: failureClass={}, code=ERR_INTERNAL_ERROR, remoteAddr={}",
                    e.getClass().getSimpleName(), request.getRemoteAddr());
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR).body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_INTERNAL_ERROR",
                    "message", "Internal server error processing agent telemetry"
            ));
        }
    }
}
