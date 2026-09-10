package com.platform.core.controller;

import com.platform.core.agentcontrol.AgentAuthorityQueryService;
import com.platform.core.agentcontrol.AgentAuthorityView;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

/**
 * Sensor-facing Agent authority lookup. Authenticated via Sensor mTLS → SensorAuthorityResolver.
 * Not a customer administration API.
 *
 * <p>Production Agent Receiver must present its cached {@code assignment_generation} for fence check,
 * or compare the returned generation against its local cache. Lookup without
 * {@code assignment_generation} returns the current fence for the Sensor to store.
 *
 * <p>Go production {@code AgentIdentityVerifier}/{@code AssignmentAuthorizer} Core-calling adapters are
 * a follow-up after Sensor {@code AssignmentContext} gains a Generation field; leave Go fail-closed
 * defaults until that contract lands.
 */
@RestController
@RequestMapping("/api/v1/sensor/agent-authority")
public class SensorAgentAuthorityController {

    private static final Logger log = LoggerFactory.getLogger(SensorAgentAuthorityController.class);

    private final SensorAuthorityResolver authorityResolver;
    private final AgentAuthorityQueryService agentAuthorityQueryService;

    @Autowired
    public SensorAgentAuthorityController(
            SensorAuthorityResolver authorityResolver,
            AgentAuthorityQueryService agentAuthorityQueryService
    ) {
        this.authorityResolver = Objects.requireNonNull(authorityResolver, "authorityResolver");
        this.agentAuthorityQueryService = Objects.requireNonNull(
                agentAuthorityQueryService, "agentAuthorityQueryService");
    }

    @PostMapping(value = "/resolve", consumes = "application/json", produces = "application/json")
    public ResponseEntity<Map<String, Object>> resolve(
            @RequestBody(required = false) Map<String, Object> payload,
            HttpServletRequest request
    ) {
        X509Certificate clientCert = extractClientCertificate(request);
        if (clientCert == null) {
            log.warn("Rejected unauthenticated Agent authority resolve from remote={}", request.getRemoteAddr());
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of(
                    "status", "ERROR",
                    "code", "ERR_AUTH_CLIENT_CERT_REQUIRED",
                    "message", "mTLS client certificate authentication required for Sensor Agent authority lookup"
            ));
        }

        final SensorScope scope;
        try {
            scope = authorityResolver.resolve(clientCert);
            if (scope == null || scope.sensorId() == null || scope.sensorId().isBlank()) {
                throw new IngestionException(
                        HttpStatus.FORBIDDEN,
                        "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                        "Authoritative Sensor Scope is unavailable; Agent authority lookup is fail-closed"
                );
            }
        } catch (IngestionException e) {
            log.warn("Sensor Agent authority resolve rejected at Sensor authority: code={}", e.getCode());
            return ingestionError(e);
        }

        String fingerprint = extractFingerprint(payload);
        // Payload agent_id / tenant_id claims are ignored — fingerprint is the only Agent lookup key.
        try {
            Long claimedGeneration = extractClaimedGeneration(payload);
            AgentAuthorityView view = agentAuthorityQueryService.resolveForSensor(
                    scope, fingerprint, claimedGeneration);
            Map<String, Object> resp = new LinkedHashMap<>();
            resp.put("status", "ACCEPTED");
            resp.put("agent_id", view.agentId());
            resp.put("tenant_id", view.tenantId().toString());
            if (view.siteId() != null) {
                resp.put("site_id", view.siteId().toString());
            }
            resp.put("assignment_generation", view.assignmentGeneration());
            resp.put("sensor_id", view.sensorId());
            resp.put("resolved_at", Instant.now().toString());
            return ResponseEntity.ok(resp);
        } catch (IngestionException e) {
            log.warn("Agent authority resolve rejected: sensorId={}, code={}", scope.sensorId(), e.getCode());
            return ingestionError(e);
        }
    }

    private static String extractFingerprint(Map<String, Object> payload) {
        if (payload == null) {
            return null;
        }
        Object raw = payload.get("fingerprint_sha256");
        if (!(raw instanceof String s)) {
            return null;
        }
        String trimmed = s.trim();
        return trimmed.isEmpty() ? null : trimmed;
    }

    /**
     * Optional Sensor-cached fence claim. Absent → resolve without claim check.
     * Present non-number → fail closed as generation mismatch (do not silently skip fencing).
     */
    private static Long extractClaimedGeneration(Map<String, Object> payload) {
        if (payload == null || !payload.containsKey("assignment_generation")) {
            return null;
        }
        Object raw = payload.get("assignment_generation");
        if (raw instanceof Number n) {
            return n.longValue();
        }
        throw new IngestionException(
                HttpStatus.FORBIDDEN,
                "ERR_AGENT_ASSIGNMENT_GENERATION_MISMATCH",
                "assignment_generation claim must be a number when present"
        );
    }

    private static X509Certificate extractClientCertificate(HttpServletRequest request) {
        Object certAttr = request.getAttribute("jakarta.servlet.request.X509Certificate");
        if (certAttr instanceof X509Certificate[] certs && certs.length > 0) {
            return certs[0];
        }
        return null;
    }

    private static ResponseEntity<Map<String, Object>> ingestionError(IngestionException e) {
        return ResponseEntity.status(e.getStatus()).body(Map.of(
                "status", "ERROR",
                "code", e.getCode(),
                "message", e.getMessage()
        ));
    }
}
