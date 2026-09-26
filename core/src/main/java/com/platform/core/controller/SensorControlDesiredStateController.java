package com.platform.core.controller;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.JsonNode;
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
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.io.IOException;
import java.io.InputStream;
import java.math.BigDecimal;
import java.security.cert.X509Certificate;
import java.util.Arrays;
import java.util.Iterator;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;

/**
 * Sensor machine control desired-state GET and APPLIED/REJECTED ACK (BUILD-004 Slice 2).
 * Identity is exclusively mTLS → {@link SensorAuthorityResolver}.
 * ACK bodies are read only after authority resolution, under a hard byte ceiling,
 * and reject unknown / identity fields (no silent strip).
 */
@RestController
@RequestMapping("/api/v1/sensor/control")
public class SensorControlDesiredStateController {

    private static final Logger log = LoggerFactory.getLogger(SensorControlDesiredStateController.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final ObjectMapper ACK_MAPPER = new ObjectMapper()
            .enable(DeserializationFeature.FAIL_ON_TRAILING_TOKENS);
    private static final TypeReference<Map<String, Object>> MAP_TYPE = new TypeReference<>() {};

    private static final Set<String> ACK_ALLOWED_FIELDS = Set.of(
            "schemaVersion",
            "generation",
            "checksum",
            "outcome",
            "rejectionCode",
            "rejectionMessage"
    );

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
    public ResponseEntity<?> appliedState(HttpServletRequest request) {
        try {
            // 1–2. Authority before any body materialization.
            SensorScope scope = resolveScope(request);
            if (scope == null) {
                return clientCertRequired();
            }

            // 3–6. Bounded read then strict parse (covers missing/dishonest Content-Length and chunked).
            byte[] rawBody = readBoundedBody(
                    request.getInputStream(),
                    SensorControlDesiredLimits.MAX_ACK_REQUEST_BYTES);
            ParsedAck ack = parseAppliedAck(rawBody);

            SensorControlState updated = desiredStateService.acknowledgeAppliedState(
                    scope.tenantId(),
                    scope.sensorId(),
                    ack.schemaVersion(),
                    ack.generation(),
                    ack.checksum(),
                    ack.outcome(),
                    ack.rejectionCode(),
                    ack.rejectionMessage()
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
        } catch (IOException e) {
            throw new DomainException("VALIDATION_FAILED", "ACK body is unreadable");
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

    /**
     * Reads at most {@code maxBytes + 1} from the stream. Oversized bodies fail closed
     * without relying on Content-Length.
     */
    static byte[] readBoundedBody(InputStream in, int maxBytes) throws IOException {
        Objects.requireNonNull(in, "in");
        if (maxBytes < 0) {
            throw new DomainException("VALIDATION_FAILED", "invalid ACK size bound");
        }
        byte[] buf = new byte[maxBytes + 1];
        int offset = 0;
        while (offset < buf.length) {
            int n = in.read(buf, offset, buf.length - offset);
            if (n < 0) {
                break;
            }
            offset += n;
        }
        if (offset > maxBytes) {
            throw new DomainException("VALIDATION_FAILED", "ACK body exceeds maximum size");
        }
        return Arrays.copyOf(buf, offset);
    }

    static ParsedAck parseAppliedAck(byte[] rawBody) {
        if (rawBody == null || rawBody.length == 0) {
            throw new DomainException("VALIDATION_FAILED", "ACK body is required");
        }
        final JsonNode root;
        try {
            root = ACK_MAPPER.readValue(rawBody, JsonNode.class);
        } catch (IOException ex) {
            throw new DomainException("VALIDATION_FAILED", "ACK body is malformed");
        }
        if (root == null || !root.isObject()) {
            throw new DomainException("VALIDATION_FAILED", "ACK body must be a JSON object");
        }

        Iterator<String> names = root.fieldNames();
        while (names.hasNext()) {
            String name = names.next();
            if (!ACK_ALLOWED_FIELDS.contains(name)) {
                throw new DomainException("VALIDATION_FAILED", "unknown ACK field: " + name);
            }
        }

        int schemaVersion = requireExactInt(root.get("schemaVersion"), "schemaVersion");
        long generation = requireExactLong(root.get("generation"), "generation");
        String checksum = requireNonBlankString(root.get("checksum"), "checksum");
        SensorControlObservationOutcome outcome =
                SensorControlObservationOutcome.parse(requireNonBlankString(root.get("outcome"), "outcome"));
        String rejectionCode = optionalPresentString(root, "rejectionCode");
        String rejectionMessage = optionalPresentString(root, "rejectionMessage");
        return new ParsedAck(schemaVersion, generation, checksum, outcome, rejectionCode, rejectionMessage);
    }

    /**
     * Present property must be a non-null, non-blank JSON string. Absent → null.
     * Explicit null / empty / non-string → VALIDATION_FAILED (never coerced to absent).
     */
    private static String optionalPresentString(JsonNode root, String field) {
        if (!root.has(field)) {
            return null;
        }
        JsonNode node = root.get(field);
        if (node == null || node.isNull() || !node.isTextual()) {
            throw new DomainException("VALIDATION_FAILED", field + " must be a non-null string when present");
        }
        String value = node.asText();
        if (value.isBlank()) {
            throw new DomainException("VALIDATION_FAILED", field + " must not be blank when present");
        }
        return value;
    }

    private static String requireNonBlankString(JsonNode node, String field) {
        if (node == null || node.isNull() || !node.isTextual()) {
            throw new DomainException("VALIDATION_FAILED", field + " is required");
        }
        String value = node.asText();
        if (value.isBlank()) {
            throw new DomainException("VALIDATION_FAILED", field + " is required");
        }
        return value;
    }

    private static int requireExactInt(JsonNode node, String field) {
        long v = requireExactLong(node, field);
        if (v < Integer.MIN_VALUE || v > Integer.MAX_VALUE) {
            throw new DomainException("VALIDATION_FAILED", field + " must be an exact integer");
        }
        return (int) v;
    }

    private static long requireExactLong(JsonNode node, String field) {
        if (node == null || node.isNull() || !node.isNumber() || !node.isIntegralNumber()) {
            throw new DomainException("VALIDATION_FAILED", field + " must be an exact integer");
        }
        try {
            return new BigDecimal(node.numberValue().toString()).longValueExact();
        } catch (ArithmeticException | NumberFormatException ex) {
            throw new DomainException("VALIDATION_FAILED", field + " must be an exact integer");
        }
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

    record ParsedAck(
            int schemaVersion,
            long generation,
            String checksum,
            SensorControlObservationOutcome outcome,
            String rejectionCode,
            String rejectionMessage
    ) {}
}
