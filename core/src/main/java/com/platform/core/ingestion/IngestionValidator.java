package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.CanonicalCounterObservation;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryObservationBatch;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;

import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Base64;
import java.util.Comparator;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.UUID;

@Component
public class IngestionValidator {
    public static final int MAX_BATCH_BYTES = 2 * 1024 * 1024;
    public static final int MAX_RECORDS = 1000;
    public static final int MAX_RECORD_BYTES = 512 * 1024;

    // Maximum safe integer representable without precision loss in IEEE 754 64-bit float (2^53 - 1)
    private final ObjectMapper objectMapper;
    private final SensorAuthorityResolver authorityResolver;
    private final IngestionMetrics metrics;

    public IngestionValidator(ObjectMapper objectMapper, SensorAuthorityResolver authorityResolver) {
        this(objectMapper, authorityResolver, new IngestionMetrics());
    }

    @Autowired
    public IngestionValidator(
            ObjectMapper objectMapper,
            SensorAuthorityResolver authorityResolver,
            IngestionMetrics metrics
    ) {
        this.objectMapper = objectMapper != null ? objectMapper : new ObjectMapper();
        this.authorityResolver = authorityResolver;
        this.metrics = metrics != null ? metrics : new IngestionMetrics();
    }

    public SensorScope resolveAuthority(X509Certificate clientCert) {
        if (authorityResolver == null) {
            metrics.recordBatchRejected("ERR_SENSOR_AUTHORITY_UNAVAILABLE");
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "No Sensor authority resolver is configured; failing closed"
            );
        }
        return authorityResolver.resolve(clientCert);
    }

    public TelemetryObservationBatch validateAndExtract(
            byte[] rawBody,
            TelemetryBatchRequest batch,
            SensorScope scope
    ) {
        if (scope == null) {
            metrics.recordBatchRejected("ERR_SENSOR_AUTHORITY_UNAVAILABLE");
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "Authoritative Sensor Scope is required; telemetry ingestion is fail-closed"
            );
        }

        try {
            validateBatchBounds(batch, rawBody, scope);
            TelemetryObservationBatch observations = extractCanonicalObservations(batch, scope);
            if (observations.isEmpty()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_NO_CANONICAL_OBSERVATIONS",
                        "Batch contains zero supported canonical numeric observations"
                );
            }
            return observations;
        } catch (IngestionException e) {
            metrics.recordBatchRejected(e.getCode());
            throw e;
        }
    }

    public void validateBatchBounds(TelemetryBatchRequest batch, byte[] rawBody, SensorScope scope) {
        if (rawBody != null && rawBody.length > MAX_BATCH_BYTES) {
            throw new IngestionException(
                    HttpStatus.PAYLOAD_TOO_LARGE,
                    "ERR_BATCH_TOO_LARGE",
                    "Batch payload size exceeds maximum limit of " + MAX_BATCH_BYTES + " bytes"
            );
        }

        if (batch == null) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_EMPTY_BODY", "Request body is empty");
        }

        if (batch.schemaVersion() != 1) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_UNSUPPORTED_SCHEMA_VERSION",
                    "Unsupported schema version: " + batch.schemaVersion() + ", expected 1"
            );
        }

        if (batch.sensorId() == null || batch.sensorId().isBlank()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_SENSOR_ID_REQUIRED", "Sensor ID is required");
        }

        // Sensor identity verification against authoritative scope
        String claimedSensorId = batch.sensorId().trim();
        if (!claimedSensorId.equals(scope.sensorId().trim())) {
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_AUTH_IDENTITY_MISMATCH",
                    "Sensor ID '" + claimedSensorId + "' does not match authoritative scope sensorId '" + scope.sensorId() + "'"
            );
        }

        if (batch.bootEpoch() == null || batch.bootEpoch().isBlank()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_BOOT_EPOCH_REQUIRED", "bootEpoch is required");
        }

        if (batch.batchId() == null || batch.batchId().isBlank()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_BATCH_ID_REQUIRED", "batchId is required");
        }

        if (batch.firstSequence() <= 0) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_BATCH_BOUNDS_INVALID", "firstSequence must be positive");
        }

        if (!TransportSequenceLimits.isValidWireSequence(batch.firstSequence())
                || !TransportSequenceLimits.isValidWireSequence(batch.lastSequence())) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_SEQUENCE_OUT_OF_BOUNDS",
                    "Batch sequences must be within 1..9007199254740991"
            );
        }

        if (batch.lastSequence() < batch.firstSequence()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_BATCH_BOUNDS_INVALID", "lastSequence cannot be less than firstSequence");
        }

        if (batch.records() == null || batch.records().isEmpty()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_EMPTY_RECORDS", "Batch contains no records");
        }

        if (batch.records().size() > MAX_RECORDS) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_RECORDS_LIMIT_EXCEEDED",
                    "Record count exceeds maximum of " + MAX_RECORDS
            );
        }

        long expectedCount = batch.lastSequence() - batch.firstSequence() + 1;
        if (batch.records().size() != expectedCount) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_SEQUENCE_DISCONTINUITY",
                    "Batch record count (" + batch.records().size() + ") does not match sequence range ["
                            + batch.firstSequence() + ".." + batch.lastSequence() + "] (expected " + expectedCount + ")"
            );
        }

        Set<Long> seenSequences = new HashSet<>();
        for (JsonNode recordNode : batch.records()) {
            if (!recordNode.hasNonNull("sequence") || !recordNode.path("sequence").isIntegralNumber()) {
                throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_INVALID_SEQUENCE", "Record missing valid integer sequence");
            }
            long seq = recordNode.path("sequence").asLong();
            if (seq < batch.firstSequence() || seq > batch.lastSequence()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_SEQUENCE_OUT_OF_BOUNDS",
                        "Record sequence " + seq + " is outside batch bounds [" + batch.firstSequence() + ".." + batch.lastSequence() + "]"
                );
            }
            if (!seenSequences.add(seq)) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_DUPLICATE_SEQUENCE",
                        "Duplicate sequence " + seq + " detected in batch"
                );
            }

            try {
                byte[] recordBytes = objectMapper.writeValueAsBytes(recordNode);
                if (recordBytes.length > MAX_RECORD_BYTES) {
                    throw new IngestionException(
                            HttpStatus.PAYLOAD_TOO_LARGE,
                            "ERR_RECORD_TOO_LARGE",
                            "Record sequence " + seq + " size (" + recordBytes.length + " bytes) exceeds maximum limit of " + MAX_RECORD_BYTES + " bytes"
                    );
                }
            } catch (IngestionException ie) {
                throw ie;
            } catch (Exception e) {
                throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_MALFORMED_RECORD", "Failed to inspect record bounds");
            }
        }

        for (long s = batch.firstSequence(); s <= batch.lastSequence(); s++) {
            if (!seenSequences.contains(s)) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_MISSING_SEQUENCE",
                        "Missing sequence " + s + " in batch range [" + batch.firstSequence() + ".." + batch.lastSequence() + "]"
                );
            }
        }

        validateChecksum(batch, rawBody);
    }

    public void validateChecksum(TelemetryBatchRequest batch, byte[] rawBody) {
        if (batch.checksum() == null || batch.checksum().isBlank()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_CHECKSUM_REQUIRED", "Batch checksum is required");
        }

        String calculated = calculateRawRecordsChecksum(rawBody);
        if (calculated != null && calculated.equalsIgnoreCase(batch.checksum())) {
            return;
        }

        String nodeCalculated = calculateNodeChecksum(batch.records());
        if (nodeCalculated.equalsIgnoreCase(batch.checksum())) {
            return;
        }

        throw new IngestionException(
                HttpStatus.BAD_REQUEST,
                "ERR_CHECKSUM_MISMATCH",
                "Batch checksum mismatch: expected=" + batch.checksum() + ", calculated=" + (calculated != null ? calculated : nodeCalculated)
        );
    }

    private String calculateRawRecordsChecksum(byte[] rawBody) {
        if (rawBody == null || rawBody.length == 0) return null;
        try {
            int recordsIdx = -1;
            byte[] pattern = "\"records\"".getBytes(StandardCharsets.UTF_8);
            for (int i = 0; i <= rawBody.length - pattern.length; i++) {
                boolean match = true;
                for (int j = 0; j < pattern.length; j++) {
                    if (rawBody[i + j] != pattern[j]) {
                        match = false;
                        break;
                    }
                }
                if (match) {
                    recordsIdx = i + pattern.length;
                    break;
                }
            }
            if (recordsIdx == -1) return null;

            int arrayStart = -1;
            for (int i = recordsIdx; i < rawBody.length; i++) {
                if (rawBody[i] == '[') {
                    arrayStart = i + 1;
                    break;
                }
            }
            if (arrayStart == -1) return null;

            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            int depth = 0;
            boolean inStr = false;
            boolean escape = false;
            int objStart = -1;
            int objCount = 0;

            for (int i = arrayStart; i < rawBody.length; i++) {
                byte b = rawBody[i];
                if (escape) {
                    escape = false;
                    continue;
                }
                if (b == '\\') {
                    escape = true;
                    continue;
                }
                if (b == '"') {
                    inStr = !inStr;
                    continue;
                }
                if (!inStr) {
                    if (b == '{') {
                        if (depth == 0) objStart = i;
                        depth++;
                    } else if (b == '}') {
                        depth--;
                        if (depth == 0 && objStart != -1) {
                            digest.update(rawBody, objStart, (i - objStart + 1));
                            objCount++;
                            objStart = -1;
                        }
                    } else if (b == ']' && depth == 0) {
                        break;
                    }
                }
            }
            if (objCount == 0) return null;
            byte[] hash = digest.digest();
            StringBuilder sb = new StringBuilder(hash.length * 2);
            for (byte b : hash) {
                sb.append(String.format("%02x", b));
            }
            return sb.toString();
        } catch (Exception e) {
            return null;
        }
    }

    private String calculateNodeChecksum(List<JsonNode> records) {
        if (records == null || records.isEmpty()) return "";
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            for (JsonNode node : records) {
                digest.update(objectMapper.writeValueAsBytes(node));
            }
            byte[] hash = digest.digest();
            StringBuilder sb = new StringBuilder(hash.length * 2);
            for (byte b : hash) {
                sb.append(String.format("%02x", b));
            }
            return sb.toString();
        } catch (Exception e) {
            return "";
        }
    }

    public TelemetryObservationBatch extractCanonicalObservations(TelemetryBatchRequest batch, SensorScope scope) {
        List<CanonicalObservation> numericResults = new ArrayList<>();
        List<CanonicalCounterObservation> counterResults = new ArrayList<>();
        String authoritativeTenantId = scope.tenantId().toString();
        String authoritativeSensorId = scope.sensorId();

        // Process records deterministically in ascending sequence order
        List<JsonNode> sortedRecords = new ArrayList<>(batch.records());
        sortedRecords.sort(Comparator.comparingLong(r -> r.path("sequence").asLong(0)));

        for (JsonNode recordNode : sortedRecords) {
            int initialCount = numericResults.size() + counterResults.size();
            long sequence = recordNode.path("sequence").asLong(0);

            // P0-3: Fail closed on non-numeric telemetry / unsupported record types
            String recordType = recordNode.path("type").asText("").trim().toUpperCase();
            if ("STATE".equals(recordType) || "EVENT".equals(recordType) || "TRAP".equals(recordType)
                    || "SYSLOG".equals(recordType) || "FLOW".equals(recordType)) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_UNSUPPORTED_RECORD_TYPE",
                        "Record sequence " + sequence + " contains unsupported record type: " + recordType
                );
            }

            JsonNode payloadNode = recordNode.path("payload");
            JsonNode envelopeNode = payloadNode;
            if (payloadNode.isTextual()) {
                try {
                    byte[] decoded = Base64.getDecoder().decode(payloadNode.asText());
                    envelopeNode = objectMapper.readTree(decoded);
                } catch (Exception e) {
                    try {
                        envelopeNode = objectMapper.readTree(payloadNode.asText());
                    } catch (Exception parseEx) {
                        throw new IngestionException(
                                HttpStatus.BAD_REQUEST,
                                "ERR_MALFORMED_RECORD_PAYLOAD",
                                "Record sequence " + sequence + " has malformed payload JSON"
                        );
                    }
                }
            }

            // Tenant verification: payload claim cannot override authoritative tenant
            if (envelopeNode.hasNonNull("tenantId")) {
                String claimedTenant = envelopeNode.path("tenantId").asText().trim();
                if (!claimedTenant.equalsIgnoreCase(authoritativeTenantId)) {
                    throw new IngestionException(
                            HttpStatus.FORBIDDEN,
                            "ERR_TENANT_SCOPE_MISMATCH",
                            "Record claimed tenant '" + claimedTenant + "' does not match authoritative scope tenant '" + authoritativeTenantId + "'"
                    );
                }
            }

            // Entity authorization check
            String claimedEntityId = envelopeNode.path("entityId").asText(null);
            if (claimedEntityId == null || claimedEntityId.isBlank()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_ENTITY_ID_REQUIRED",
                        "Record entityId is required"
                );
            }
            if (!scope.isEntityAllowed(claimedEntityId)) {
                throw new IngestionException(
                        HttpStatus.FORBIDDEN,
                        "ERR_ENTITY_SCOPE_UNAUTHORIZED",
                        "Entity '" + claimedEntityId + "' is not authorized in sensor scope"
                );
            }

            // Timestamp verification: event time must not be fabricated
            String collectedAtStr = envelopeNode.path("collectedAt").asText(null);
            if (collectedAtStr == null || collectedAtStr.isBlank()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_INVALID_TIMESTAMP",
                        "Record collectedAt timestamp is required; current time fabrication is prohibited"
                );
            }
            Instant ts;
            try {
                ts = Instant.parse(collectedAtStr);
            } catch (Exception ex) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_INVALID_TIMESTAMP",
                        "Invalid collectedAt ISO-8601 timestamp: " + collectedAtStr
                );
            }

            // Quality mapping: faithfully preserved, never fabricated
            String qualityStr = envelopeNode.path("quality").asText("UNKNOWN").toUpperCase();
            int qualityCode = switch (qualityStr) {
                case "GOOD" -> 1;
                case "DEGRADED" -> 2;
                default -> 0;
            };

            String protocol = envelopeNode.path("protocol").asText("").trim().toLowerCase();
            if (protocol.isBlank()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_MISSING_PROTOCOL",
                        "Record sequence " + sequence + " is missing required source protocol"
                );
            }

            JsonNode protocolPayload = envelopeNode.path("protocolPayload");

            // Check SNMP style observations array
            if (protocolPayload.has("observations")) {
                if (!protocolPayload.get("observations").isArray()) {
                    throw new IngestionException(
                            HttpStatus.BAD_REQUEST,
                            "ERR_MALFORMED_RECORD_PAYLOAD",
                            "Record sequence " + sequence + " observations field must be a JSON array"
                    );
                }
                if (!"snmp".equals(protocol)) {
                    throw new IngestionException(
                            HttpStatus.BAD_REQUEST,
                            "ERR_UNSUPPORTED_PROTOCOL",
                            "Protocol '" + protocol + "' at sequence " + sequence + " does not match SNMP observation schema"
                    );
                }

                JsonNode obsArray = protocolPayload.get("observations");
                if (obsArray.isEmpty()) {
                    throw new IngestionException(
                            HttpStatus.BAD_REQUEST,
                            "ERR_NON_NUMERIC_RECORD",
                            "Record sequence " + sequence + " contains empty observations array"
                    );
                }

                int ordinal = 0;
                for (JsonNode obs : obsArray) {
                    if (!obs.isObject()) {
                        throw new IngestionException(
                                HttpStatus.BAD_REQUEST,
                                "ERR_UNSUPPORTED_RECORD_TYPE",
                                "Observation at sequence " + sequence + " ordinal " + ordinal + " is not an object"
                        );
                    }

                    String obsStatus = obs.path("status").asText("").trim();
                    if (!"VALUE".equalsIgnoreCase(obsStatus)) {
                        throw new IngestionException(
                                HttpStatus.BAD_REQUEST,
                                "ERR_UNSUPPORTED_RECORD_TYPE",
                                "Observation at sequence " + sequence + " ordinal " + ordinal + " has non-VALUE status: " + (obsStatus.isEmpty() ? "MISSING" : obsStatus)
                        );
                    }

                    String metricId = obs.path("oid").asText(obs.path("sourceOid").asText("")).trim();
                    if (metricId.isBlank()) {
                        throw new IngestionException(
                                HttpStatus.BAD_REQUEST,
                                "ERR_MISSING_METRIC_ID",
                                "SNMP observation at sequence " + sequence + " ordinal " + ordinal + " is missing required OID"
                        );
                    }

                    JsonNode valNode = obs.path("value");
                    if (valNode.isMissingNode() || valNode.isNull() || !valNode.isObject()) {
                        throw new IngestionException(
                                HttpStatus.BAD_REQUEST,
                                "ERR_NON_NUMERIC_RECORD",
                                "Observation at sequence " + sequence + " ordinal " + ordinal + " missing value object"
                        );
                    }

                    // Check explicit value kind if present. A discriminator must agree
                    // with the single concrete numeric field; ambiguous typed values fail closed.
                    String declaredKind = null;
                    if (valNode.hasNonNull("kind")) {
                        declaredKind = valNode.path("kind").asText("").trim().toUpperCase();
                        if (!"INTEGER".equals(declaredKind) && !"UNSIGNED".equals(declaredKind) && !"COUNTER64".equals(declaredKind)) {
                            throw new IngestionException(
                                    HttpStatus.BAD_REQUEST,
                                    "ERR_UNSUPPORTED_RECORD_TYPE",
                                    "Observation at sequence " + sequence + " ordinal " + ordinal + " contains unsupported value kind: " + declaredKind
                            );
                        }
                    }

                    int concreteNumericFields = (valNode.has("integer") ? 1 : 0)
                            + (valNode.has("unsigned") ? 1 : 0)
                            + (valNode.has("counter64") ? 1 : 0);
                    if (concreteNumericFields > 1) {
                        throw new IngestionException(
                                HttpStatus.BAD_REQUEST,
                                "ERR_AMBIGUOUS_NUMERIC_VALUE",
                                "Observation at sequence " + sequence + " ordinal " + ordinal
                                        + " contains multiple numeric value fields"
                        );
                    }

                    // Reject text/string/bytes/opaque/ip/bits payload types
                    if (valNode.hasNonNull("text") || valNode.hasNonNull("string") || valNode.hasNonNull("bytes")
                            || valNode.hasNonNull("opaque") || valNode.hasNonNull("ip") || valNode.hasNonNull("ipAddress")
                            || valNode.hasNonNull("bits") || valNode.hasNonNull("bitLength") || valNode.hasNonNull("oid")) {
                        throw new IngestionException(
                                HttpStatus.BAD_REQUEST,
                                "ERR_NON_NUMERIC_RECORD",
                                "Observation at sequence " + sequence + " ordinal " + ordinal + " contains non-numeric payload; fail closed"
                        );
                    }

                    Double numericVal = null;
                    if (valNode.hasNonNull("integer")) {
                        if (declaredKind != null && !"INTEGER".equals(declaredKind)) {
                            throw new IngestionException(
                                    HttpStatus.BAD_REQUEST,
                                    "ERR_VALUE_KIND_MISMATCH",
                                    "Observation at sequence " + sequence + " ordinal " + ordinal
                                            + " declares kind " + declaredKind + " but carries integer"
                            );
                        }
                        if (valNode.path("integer").isIntegralNumber()) {
                            numericVal = valNode.path("integer").asDouble();
                        } else {
                            throw new IngestionException(
                                    HttpStatus.BAD_REQUEST,
                                    "ERR_MALFORMED_NUMERIC_VALUE",
                                    "Observation at sequence " + sequence + " ordinal " + ordinal + " has malformed integer value"
                            );
                        }
                    } else if (valNode.hasNonNull("unsigned")) {
                        if (declaredKind != null && !"UNSIGNED".equals(declaredKind)) {
                            throw new IngestionException(
                                    HttpStatus.BAD_REQUEST,
                                    "ERR_VALUE_KIND_MISMATCH",
                                    "Observation at sequence " + sequence + " ordinal " + ordinal
                                            + " declares kind " + declaredKind + " but carries unsigned"
                            );
                        }
                        if (valNode.path("unsigned").isIntegralNumber()) {
                            numericVal = valNode.path("unsigned").asDouble();
                        } else {
                            throw new IngestionException(
                                    HttpStatus.BAD_REQUEST,
                                    "ERR_MALFORMED_NUMERIC_VALUE",
                                    "Observation at sequence " + sequence + " ordinal " + ordinal + " has malformed unsigned value"
                            );
                        }
                    } else if (valNode.has("counter64")) {
                        if (declaredKind != null && !"COUNTER64".equals(declaredKind)) {
                            throw new IngestionException(
                                    HttpStatus.BAD_REQUEST,
                                    "ERR_VALUE_KIND_MISMATCH",
                                    "Observation at sequence " + sequence + " ordinal " + ordinal
                                            + " declares kind " + declaredKind + " but carries counter64"
                            );
                        }
                        JsonNode c64Node = valNode.get("counter64");
                        if (c64Node == null || c64Node.isNull()) {
                            throw new IngestionException(
                                    HttpStatus.BAD_REQUEST,
                                    "ERR_MALFORMED_COUNTER64",
                                    "Observation at sequence " + sequence + " ordinal " + ordinal + " has empty Counter64 value"
                            );
                        }
                        if (!c64Node.isTextual()) {
                            throw new IngestionException(
                                    HttpStatus.BAD_REQUEST,
                                    "ERR_COUNTER64_JSON_NUMBER",
                                    "Observation at sequence " + sequence + " ordinal " + ordinal
                                            + " Counter64 must be a decimal string, not a JSON Number"
                            );
                        }
                        String c64Str = c64Node.asText();
                        if (c64Str.isEmpty()) {
                            throw new IngestionException(
                                    HttpStatus.BAD_REQUEST,
                                    "ERR_MALFORMED_COUNTER64",
                                    "Observation at sequence " + sequence + " ordinal " + ordinal + " has empty Counter64 value"
                            );
                        }
                        final BigInteger exactCounter;
                        try {
                            exactCounter = CanonicalCounterObservation.parseExactUnsigned64(c64Str);
                        } catch (IllegalArgumentException ex) {
                            throw new IngestionException(
                                    HttpStatus.BAD_REQUEST,
                                    "ERR_MALFORMED_COUNTER64",
                                    "Observation at sequence " + sequence + " ordinal " + ordinal
                                            + " has invalid Counter64 representation"
                            );
                        }

                        String stableCounterSeqId = authoritativeTenantId + ":"
                                + authoritativeSensorId + ":"
                                + batch.bootEpoch() + ":"
                                + sequence + ":"
                                + ordinal + ":"
                                + metricId;

                        counterResults.add(new CanonicalCounterObservation(
                                authoritativeTenantId,
                                claimedEntityId,
                                metricId,
                                authoritativeSensorId,
                                ts,
                                exactCounter,
                                qualityCode,
                                0,
                                stableCounterSeqId,
                                "snmp"
                        ));
                        ordinal++;
                        continue;
                    } else {
                        throw new IngestionException(
                                HttpStatus.BAD_REQUEST,
                                "ERR_NON_NUMERIC_RECORD",
                                "Observation at sequence " + sequence + " ordinal " + ordinal + " does not contain supported numeric field; fail closed"
                        );
                    }

                    if (numericVal == null || !Double.isFinite(numericVal)) {
                        throw new IngestionException(
                                HttpStatus.BAD_REQUEST,
                                "ERR_MALFORMED_NUMERIC_VALUE",
                                "Observation at sequence " + sequence + " ordinal " + ordinal + " is not a finite numeric value"
                        );
                    }

                    // P0 replay identity uses the durable Sensor spool stream generation.
                    // The v1 wire field remains named bootEpoch for compatibility, but the Sensor now
                    // supplies its persisted spool StreamID: stable across process restart and new only
                    // when the durable spool itself is recreated.
                    // <tenantId>:<sensorId>:<streamId>:<sequence>:<ordinal>:<metricId>
                    String stableSeqId = authoritativeTenantId + ":"
                            + authoritativeSensorId + ":"
                            + batch.bootEpoch() + ":"
                            + sequence + ":"
                            + ordinal + ":"
                            + metricId;

                    numericResults.add(new CanonicalObservation(
                            authoritativeTenantId,
                            claimedEntityId,
                            metricId,
                            authoritativeSensorId,
                            ts,
                            numericVal,
                            qualityCode,
                            0,
                            stableSeqId,
                            "snmp"
                    ));
                    ordinal++;
                }
            } else if (protocolPayload.has("metric_id") && protocolPayload.has("value")) {
                // Generic numeric metric with explicit metric_id and numeric value
                String metricId = protocolPayload.path("metric_id").asText("").trim();
                if (metricId.isBlank()) {
                    throw new IngestionException(
                            HttpStatus.BAD_REQUEST,
                            "ERR_MISSING_METRIC_ID",
                            "Record sequence " + sequence + " is missing required metric_id"
                    );
                }

                JsonNode valNode = protocolPayload.path("value");
                if (valNode.isNumber()) {
                    double val = valNode.asDouble();
                    if (Double.isFinite(val)) {
                        String stableSeqId = authoritativeTenantId + ":"
                                + authoritativeSensorId + ":"
                                + batch.bootEpoch() + ":"
                                + sequence + ":0:"
                                + metricId;

                        numericResults.add(new CanonicalObservation(
                                authoritativeTenantId,
                                claimedEntityId,
                                metricId,
                                authoritativeSensorId,
                                ts,
                                val,
                                qualityCode,
                                0,
                                stableSeqId,
                                protocol
                        ));
                    } else {
                        throw new IngestionException(
                                HttpStatus.BAD_REQUEST,
                                "ERR_MALFORMED_NUMERIC_VALUE",
                                "Record sequence " + sequence + " metric value is not finite"
                        );
                    }
                } else {
                    throw new IngestionException(
                            HttpStatus.BAD_REQUEST,
                            "ERR_NON_NUMERIC_RECORD",
                            "Record sequence " + sequence + " value is non-numeric; partial source record ACK is prohibited"
                    );
                }
            } else {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_UNSUPPORTED_RECORD_TYPE",
                        "Record sequence " + sequence + " contains unsupported telemetry payload shape; fail closed"
                );
            }

            // Every source spool record in an accepted batch must be completely accounted for.
            // If any source record contains unsupported telemetry or yields zero numeric observations,
            // reject the entire batch (fail closed at Sensor spool).
            if (numericResults.size() + counterResults.size() == initialCount) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_NON_NUMERIC_RECORD",
                        "Record sequence " + sequence + " contains no valid numeric facts; partial source record ACK is prohibited"
                );
            }
        }

        return new TelemetryObservationBatch(numericResults, counterResults);
    }
}
