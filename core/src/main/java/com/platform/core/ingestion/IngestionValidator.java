package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;

@Component
public class IngestionValidator {
    public static final int MAX_BATCH_BYTES = 2 * 1024 * 1024;
    public static final int MAX_RECORDS = 1000;
    public static final int MAX_RECORD_BYTES = 512 * 1024;

    private final ObjectMapper objectMapper;
    private final IngestionMetrics metrics;

    public IngestionValidator(ObjectMapper objectMapper) {
        this(objectMapper, new IngestionMetrics());
    }

    @org.springframework.beans.factory.annotation.Autowired
    public IngestionValidator(ObjectMapper objectMapper, IngestionMetrics metrics) {
        this.objectMapper = objectMapper;
        this.metrics = metrics != null ? metrics : new IngestionMetrics();
    }

    public List<CanonicalObservation> validateAndExtract(byte[] rawBody, TelemetryBatchRequest batch, java.security.cert.X509Certificate clientCert) {
        if (clientCert == null) {
            metrics.recordBatchRejected("ERR_AUTH_CLIENT_CERT_REQUIRED");
            throw new IngestionException(HttpStatus.UNAUTHORIZED, "ERR_AUTH_CLIENT_CERT_REQUIRED",
                    "mTLS client certificate authentication required for telemetry ingestion");
        }
        String certIdentity = extractCommonName(clientCert.getSubjectX500Principal().getName());
        try {
            validate(batch, rawBody, certIdentity);
            return extractCanonicalObservations(batch);
        } catch (IngestionException e) {
            metrics.recordBatchRejected(e.getCode());
            throw e;
        }
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

    public void validate(TelemetryBatchRequest batch, byte[] rawBody, String trustedCertIdentity) {
        if (rawBody != null && rawBody.length > MAX_BATCH_BYTES) {
            throw new IngestionException(HttpStatus.PAYLOAD_TOO_LARGE, "ERR_BATCH_TOO_LARGE",
                    "Batch payload size exceeds maximum limit of " + MAX_BATCH_BYTES + " bytes");
        }

        if (batch == null) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_EMPTY_BODY", "Request body is empty");
        }

        if (batch.schemaVersion() != 1) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_UNSUPPORTED_SCHEMA_VERSION",
                    "Unsupported schema version: " + batch.schemaVersion() + ", expected 1");
        }

        if (batch.sensorId() == null || batch.sensorId().isBlank()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_SENSOR_ID_REQUIRED", "Sensor ID is required");
        }

        // Sensor identity verification against verified client certificate
        if (trustedCertIdentity != null && !trustedCertIdentity.isBlank()) {
            String sid = batch.sensorId().trim();
            String cid = trustedCertIdentity.trim();
            boolean match = cid.equalsIgnoreCase(sid) || cid.startsWith(sid + ".") || sid.startsWith(cid + ".");
            if (!match) {
                throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AUTH_IDENTITY_MISMATCH",
                        "Sensor ID '" + sid + "' does not match verified client certificate identity '" + cid + "'");
            }
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
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_RECORDS_LIMIT_EXCEEDED",
                    "Record count exceeds maximum of " + MAX_RECORDS);
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

        throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_CHECKSUM_MISMATCH",
                "Batch checksum mismatch: expected=" + batch.checksum() + ", calculated=" + (calculated != null ? calculated : nodeCalculated));
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

    public List<CanonicalObservation> extractCanonicalObservations(TelemetryBatchRequest batch) {
        List<CanonicalObservation> results = new ArrayList<>();
        String sensorId = batch.sensorId();

        for (JsonNode recordNode : batch.records()) {
            long sequence = recordNode.path("sequence").asLong(0);
            String recordType = recordNode.path("type").asText("");
            JsonNode payloadNode = recordNode.path("payload");

            JsonNode envelopeNode = payloadNode;
            if (payloadNode.isTextual()) {
                try {
                    byte[] decoded = Base64.getDecoder().decode(payloadNode.asText());
                    envelopeNode = objectMapper.readTree(decoded);
                } catch (Exception e) {
                    try {
                        envelopeNode = objectMapper.readTree(payloadNode.asText());
                    } catch (Exception ignored) {
                    }
                }
            }

            String tenantId = envelopeNode.path("tenantId").asText("default");
            String entityId = envelopeNode.path("entityId").asText(sensorId);
            String envSensorId = envelopeNode.path("sensorId").asText(sensorId);
            String protocol = envelopeNode.path("protocol").asText("UNKNOWN");
            String collectedAtStr = envelopeNode.path("collectedAt").asText();
            Instant ts;
            try {
                ts = (collectedAtStr != null && !collectedAtStr.isBlank())
                        ? Instant.parse(collectedAtStr)
                        : Instant.now();
            } catch (Exception ex) {
                ts = Instant.now();
            }

            JsonNode protocolPayload = envelopeNode.path("protocolPayload");

            // Check if protocolPayload has observations array (SNMP style)
            if (protocolPayload.has("observations") && protocolPayload.get("observations").isArray()) {
                for (JsonNode obs : protocolPayload.get("observations")) {
                    String metricId = obs.path("oid").asText(obs.path("sourceOid").asText("snmp.metric"));
                    double val = 0.0;
                    JsonNode valNode = obs.path("value");
                    if (valNode.path("integer").isNumber()) {
                        val = valNode.path("integer").asDouble();
                    } else if (valNode.path("unsigned").isNumber()) {
                        val = valNode.path("unsigned").asDouble();
                    } else if (valNode.path("counter64").isTextual()) {
                        try {
                            val = Double.parseDouble(valNode.path("counter64").asText());
                        } catch (Exception ignored) {}
                    }
                    results.add(new CanonicalObservation(
                            tenantId, entityId, metricId, envSensorId, ts, val, 1, 0,
                            String.valueOf(sequence), "snmp"
                    ));
                }
            } else if (protocolPayload.has("metric_id") && protocolPayload.has("value")) {
                String metricId = protocolPayload.path("metric_id").asText("metric.generic");
                double val = protocolPayload.path("value").asDouble(0.0);
                results.add(new CanonicalObservation(
                        tenantId, entityId, metricId, envSensorId, ts, val, 1, 0,
                        String.valueOf(sequence), protocol.toLowerCase()
                ));
            } else if (protocolPayload.has("status")) {
                results.add(new CanonicalObservation(
                        tenantId, entityId, "sensor.heartbeat", envSensorId, ts, 1.0, 1, 0,
                        String.valueOf(sequence), "sensor"
                ));
            } else {
                results.add(new CanonicalObservation(
                        tenantId, entityId, recordType.toLowerCase().replace('_', '.'), envSensorId, ts, 1.0, 1, 0,
                        String.valueOf(sequence), protocol.toLowerCase()
                ));
            }
        }
        return results;
    }
}
