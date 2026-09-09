package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.CanonicalObservation;
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
import java.util.List;
import java.util.UUID;

@Component
public class IngestionValidator {
    public static final int MAX_BATCH_BYTES = 2 * 1024 * 1024;
    public static final int MAX_RECORDS = 1000;
    public static final int MAX_RECORD_BYTES = 512 * 1024;

    // Maximum safe integer representable without precision loss in IEEE 754 64-bit float (2^53 - 1)
    private static final BigInteger MAX_SAFE_COUNTER64 = BigInteger.valueOf((1L << 53) - 1);

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

    public List<CanonicalObservation> validateAndExtract(
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
            List<CanonicalObservation> observations = extractCanonicalObservations(batch, scope);
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
        if (!claimedSensorId.equalsIgnoreCase(scope.sensorId().trim())) {
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

    public List<CanonicalObservation> extractCanonicalObservations(TelemetryBatchRequest batch, SensorScope scope) {
        List<CanonicalObservation> results = new ArrayList<>();
        String authoritativeTenantId = scope.tenantId().toString();
        String authoritativeSensorId = scope.sensorId();

        for (JsonNode recordNode : batch.records()) {
            long sequence = recordNode.path("sequence").asLong(0);
            if (sequence < batch.firstSequence() || sequence > batch.lastSequence()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_SEQUENCE_OUT_OF_BOUNDS",
                        "Record sequence " + sequence + " is outside batch range ["
                                + batch.firstSequence() + ".." + batch.lastSequence() + "]"
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
                                "Failed to parse record payload JSON: " + parseEx.getMessage()
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

            String protocol = envelopeNode.path("protocol").asText("UNKNOWN").toLowerCase();
            JsonNode protocolPayload = envelopeNode.path("protocolPayload");

            // Check SNMP style observations array
            if (protocolPayload.has("observations") && protocolPayload.get("observations").isArray()) {
                JsonNode obsArray = protocolPayload.get("observations");
                int obsCount = obsArray.size();
                int ordinal = 0;
                for (JsonNode obs : obsArray) {
                    String obsStatus = obs.path("status").asText("");
                    if (!"VALUE".equalsIgnoreCase(obsStatus) && !obsStatus.isBlank()) {
                        // Skip non-value / exception observations
                        ordinal++;
                        continue;
                    }

                    JsonNode valNode = obs.path("value");
                    Double numericVal = null;
                    if (valNode.path("integer").isIntegralNumber()) {
                        numericVal = valNode.path("integer").asDouble();
                    } else if (valNode.path("unsigned").isIntegralNumber()) {
                        numericVal = valNode.path("unsigned").asDouble();
                    } else if (valNode.hasNonNull("counter64")) {
                        String c64Str = valNode.path("counter64").asText().trim();
                        try {
                            BigInteger u64 = new BigInteger(c64Str);
                            if (u64.compareTo(BigInteger.ZERO) < 0 || u64.compareTo(MAX_SAFE_COUNTER64) > 0) {
                                throw new IngestionException(
                                        HttpStatus.BAD_REQUEST,
                                        "ERR_NUMERIC_PRECISION_EXCEEDED",
                                        "Counter64 value exceeds maximum safe representable float64 bounds: " + c64Str
                                );
                            }
                            numericVal = u64.doubleValue();
                        } catch (NumberFormatException nfe) {
                            throw new IngestionException(
                                    HttpStatus.BAD_REQUEST,
                                    "ERR_MALFORMED_NUMERIC_VALUE",
                                    "Invalid Counter64 integer representation: " + c64Str
                            );
                        }
                    }

                    if (numericVal != null && Double.isFinite(numericVal)) {
                        String metricId = obs.path("oid").asText(obs.path("sourceOid").asText(""));
                        if (metricId.isBlank()) {
                            metricId = "snmp.metric";
                        }
                        String stableSeqId = (obsCount > 1)
                                ? sequence + ":" + ordinal
                                : String.valueOf(sequence);

                        results.add(new CanonicalObservation(
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
                    }
                    ordinal++;
                }
            } else if (protocolPayload.has("metric_id") && protocolPayload.has("value")) {
                // Generic numeric metric with explicit metric_id and numeric value
                JsonNode valNode = protocolPayload.path("value");
                if (valNode.isNumber()) {
                    double val = valNode.asDouble();
                    if (Double.isFinite(val)) {
                        String metricId = protocolPayload.path("metric_id").asText();
                        results.add(new CanonicalObservation(
                                authoritativeTenantId,
                                claimedEntityId,
                                metricId,
                                authoritativeSensorId,
                                ts,
                                val,
                                qualityCode,
                                0,
                                String.valueOf(sequence),
                                protocol
                        ));
                    }
                }
            }
            // State, syslog, trap, event and flow records are NOT numeric telemetry;
            // they are not converted to 1.0.
        }

        return results;
    }
}
