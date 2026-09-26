package com.platform.core.controller;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.domain.DomainException;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.sensorcontrol.desiredstate.SensorControlDesiredConfigJson;
import com.platform.core.sensorcontrol.desiredstate.SensorControlDesiredLimits;
import com.platform.core.sensorcontrol.desiredstate.SensorControlDesiredStateService;
import com.platform.core.sensorcontrol.desiredstate.SensorControlObservationOutcome;
import com.platform.core.sensorcontrol.desiredstate.SensorControlState;
import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.math.BigDecimal;
import java.security.cert.X509Certificate;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;

/**
 * Sensor machine control desired-state GET and APPLIED/REJECTED ACK (BUILD-004 Slice 2).
 * Identity is exclusively mTLS → {@link SensorAuthorityResolver}; payload tenant/sensor claims ignored.
 */
@RestController
@RequestMapping("/api/v1/sensor/control")
public class SensorControlDesiredStateController {

    private static final Logger log = LoggerFactory.getLogger(SensorControlDesiredStateController.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final TypeReference<Map<String, Object>> MAP_TYPE = new TypeReference<>() {};

    private final SensorAuthorityResolver authorityResolver;
    private final SensorControlDesiredStateService desiredStateService;

    public SensorControlDesiredStateController(
            SensorAuthorityResolver authorityResolver,
            SensorControlDesiredStateService desiredStateService
    ) {
        this.authorityResolver = Objects.requireNonNull(authorityResolver, "authorityResolver");
        this.desiredStateService = Objects.requireNonNull(desiredStateService, "desiredStateService");
    }

    @GetMapping("/desired-state")
    public ResponseEntity<?> desiredState(HttpServletRequest request) {
        try {
            SensorScope scope = resolveScope(request);
            if (scope == null) {
                return clientCertRequired();
            }
            Optional<SensorControlState> state =
                    desiredStateService.findDesiredState(scope.tenantId(), scope.sensorId());
            if (state.isEmpty()) {
                return ResponseEntity.noContent().build();
            }
            return ResponseEntity.ok(toDesiredResponse(state.get()));
        } catch (IngestionException e) {
            return ingestionError(e);
        }
    }

    @PutMapping(path = "/applied-state", consumes = MediaType.APPLICATION_JSON_VALUE)
    public ResponseEntity<?> appliedState(
            @RequestBody(required = false) Map<String, Object> body,
            HttpServletRequest request
    ) {
        try {
            SensorScope scope = resolveScope(request);
            if (scope == null) {
                return clientCertRequired();
            }
            if (body == null) {
                throw new DomainException("VALIDATION_FAILED", "ACK body is required");
            }
            // Explicitly ignore any client-claimed identity / actor fields.
            body.remove("tenantId");
            body.remove("tenant_id");
            body.remove("sensorId");
            body.remove("sensor_id");
            body.remove("desired_by");
            body.remove("actor");

            int schemaVersion = requireInt(body.get("schemaVersion"), "schemaVersion");
            long generation = requireLong(body.get("generation"), "generation");
            String checksum = requireString(body.get("checksum"), "checksum");
            SensorControlObservationOutcome outcome =
                    SensorControlObservationOutcome.parse(requireString(body.get("outcome"), "outcome"));
            String rejectionCode = optionalString(body.get("rejectionCode"));
            String rejectionMessage = optionalString(body.get("rejectionMessage"));

            SensorControlState updated = desiredStateService.acknowledgeAppliedState(
                    scope.tenantId(),
                    scope.sensorId(),
                    schemaVersion,
                    generation,
                    checksum,
                    outcome,
                    rejectionCode,
                    rejectionMessage
            );

            Map<String, Object> resp = new LinkedHashMap<>();
            resp.put("status", "ACCEPTED");
            resp.put("schemaVersion", SensorControlDesiredLimits.SCHEMA_VERSION);
            resp.put("generation", updated.observedGeneration());
            resp.put("checksum", updated.observedChecksum());
            resp.put("outcome", updated.observedOutcome().name());
            return ResponseEntity.ok(resp);
        } catch (IngestionException e) {
            return ingestionError(e);
        }
    }

    private SensorScope resolveScope(HttpServletRequest request) throws IngestionException {
        X509Certificate clientCert = extractClientCertificate(request);
        if (clientCert == null) {
            log.warn("Rejected Sensor control API without client certificate from remote={}",
                    request.getRemoteAddr());
            return null;
        }
        SensorScope scope = authorityResolver.resolve(clientCert);
        if (scope == null || scope.sensorId() == null || scope.sensorId().isBlank()) {
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "Authoritative Sensor Scope is unavailable"
            );
        }
        return scope;
    }

    private static Map<String, Object> toDesiredResponse(SensorControlState state) {
        Map<String, Object> resp = new LinkedHashMap<>();
        resp.put("schemaVersion", state.desiredSchemaVersion());
        resp.put("desiredGeneration", state.desiredGeneration());
        resp.put("desiredChecksum", state.desiredChecksum());
        try {
            resp.put("configuration", MAPPER.readValue(
                    SensorControlDesiredConfigJson.toJson(state.desiredConfig()), MAP_TYPE));
        } catch (Exception ex) {
            throw new DomainException("INTERNAL_ERROR", "Failed to render desired configuration");
        }
        resp.put("appliedGeneration", state.appliedGeneration());
        resp.put("appliedChecksum", state.appliedChecksum());
        resp.put("observedGeneration", state.observedGeneration());
        resp.put("observedChecksum", state.observedChecksum());
        resp.put("observedOutcome",
                state.observedOutcome() == null ? null : state.observedOutcome().name());
        if (state.rejectionCode() != null) {
            resp.put("rejectionCode", state.rejectionCode());
        }
        return resp;
    }

    private static X509Certificate extractClientCertificate(HttpServletRequest request) {
        Object certAttr = request.getAttribute("jakarta.servlet.request.X509Certificate");
        if (certAttr instanceof X509Certificate[] certs && certs.length > 0) {
            return certs[0];
        }
        return null;
    }

    private static ResponseEntity<Map<String, Object>> clientCertRequired() {
        return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of(
                "status", "ERROR",
                "code", "ERR_AUTH_CLIENT_CERT_REQUIRED",
                "message", "mTLS client certificate authentication required for Sensor control transport"
        ));
    }

    private static ResponseEntity<Map<String, Object>> ingestionError(IngestionException e) {
        return ResponseEntity.status(e.getStatus()).body(Map.of(
                "status", "ERROR",
                "code", e.getCode(),
                "message", e.getMessage()
        ));
    }

    private static int requireInt(Object value, String field) {
        long v = requireLong(value, field);
        if (v < Integer.MIN_VALUE || v > Integer.MAX_VALUE) {
            throw new DomainException("VALIDATION_FAILED", field + " must be an exact integer");
        }
        return (int) v;
    }

    private static long requireLong(Object value, String field) {
        if (!(value instanceof Number number)) {
            throw new DomainException("VALIDATION_FAILED", field + " must be numeric");
        }
        try {
            return new BigDecimal(number.toString()).longValueExact();
        } catch (ArithmeticException | NumberFormatException ex) {
            throw new DomainException("VALIDATION_FAILED", field + " must be an exact integer");
        }
    }

    private static String requireString(Object value, String field) {
        if (!(value instanceof String s) || s.isBlank()) {
            throw new DomainException("VALIDATION_FAILED", field + " is required");
        }
        return s;
    }

    private static String optionalString(Object value) {
        if (value == null) {
            return null;
        }
        if (!(value instanceof String s)) {
            throw new DomainException("VALIDATION_FAILED", "optional string field must be text");
        }
        return s.isBlank() ? null : s;
    }
}
