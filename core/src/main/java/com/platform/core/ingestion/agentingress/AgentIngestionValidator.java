package com.platform.core.ingestion.agentingress;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.agentcontrol.AgentAssignmentStatus;
import com.platform.core.agentcontrol.AgentControlRepository;
import com.platform.core.agentcontrol.AgentSensorAssignment;
import com.platform.core.agentcontrol.EntityAgentBinding;
import com.platform.core.domain.inventory.EntityIdentifier;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.IngestionMetrics;
import com.platform.core.ingestion.TransportSequenceLimits;
import com.platform.core.ingestion.agentingress.dto.AgentIngressRecord;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryBatchRequest;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.persistence.EntityIdentifierRepository;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;

import java.security.MessageDigest;
import java.security.cert.X509Certificate;
import java.time.Instant;
import java.time.format.DateTimeParseException;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashSet;
import java.util.List;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;
import java.util.regex.Pattern;

@Component
public class AgentIngestionValidator {

    public static final int MAX_BATCH_BYTES = 2 * 1024 * 1024;
    public static final int MAX_RECORDS = 1000;
    public static final int MAX_RECORD_BYTES = 512 * 1024;

    public static final String AGENT_ENTITY_NAMESPACE = "platform.agent";
    public static final String AGENT_HOST_ID_TYPE = "HOST_ID";
    public static final String SOURCE_TYPE_AGENT = "AGENT";
    public static final String SOURCE_PROTOCOL_AGENT_HOST = "AGENT_HOST";

    private static final Pattern AGENT_STREAM_ID = Pattern.compile("^stream-[0-9a-f]{32}$");

    private final ObjectMapper objectMapper;
    private final SensorAuthorityResolver authorityResolver;
    private final AgentControlRepository agentControlRepository;
    private final EntityIdentifierRepository entityIdentifierRepository;
    private final IngestionMetrics metrics;

    @Autowired
    public AgentIngestionValidator(
            ObjectMapper objectMapper,
            SensorAuthorityResolver authorityResolver,
            AgentControlRepository agentControlRepository,
            EntityIdentifierRepository entityIdentifierRepository,
            IngestionMetrics metrics
    ) {
        this.objectMapper = objectMapper != null ? objectMapper : new ObjectMapper();
        this.authorityResolver = authorityResolver;
        this.agentControlRepository = agentControlRepository;
        this.entityIdentifierRepository = entityIdentifierRepository;
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
            AgentTelemetryBatchRequest batch,
            SensorScope scope
    ) {
        if (scope == null) {
            metrics.recordBatchRejected("ERR_SENSOR_AUTHORITY_UNAVAILABLE");
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "Authoritative Sensor Scope is required; agent-telemetry ingestion is fail-closed"
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

    public void validateBatchBounds(AgentTelemetryBatchRequest batch, byte[] rawBody, SensorScope scope) {
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
        if (batch.schemaVersion() != 2) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_UNSUPPORTED_SCHEMA_VERSION",
                    "Unsupported schema version: " + batch.schemaVersion() + ", expected 2"
            );
        }
        if (batch.sensorId() == null || batch.sensorId().isBlank()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_SENSOR_ID_REQUIRED", "Sensor ID is required");
        }
        String claimedSensorId = batch.sensorId().trim();
        if (!claimedSensorId.equals(scope.sensorId().trim())) {
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_AUTH_IDENTITY_MISMATCH",
                    "Sensor ID '" + claimedSensorId + "' does not match authoritative scope sensorId '"
                            + scope.sensorId() + "'"
            );
        }
        if (!isValidAgentStreamId(batch.sensorAgentSpoolStreamId())) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_AGENT_STREAM_ID_INVALID",
                    "sensorAgentSpoolStreamId must match ^stream-[0-9a-f]{32}$"
            );
        }
        if (batch.batchId() == null || batch.batchId().isBlank()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_BATCH_ID_REQUIRED", "batchId is required");
        }
        if (batch.correlationId() == null || batch.correlationId().isBlank()) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_CORRELATION_ID_REQUIRED",
                    "correlationId is required"
            );
        }
        if (batch.firstIngressSequence() <= 0) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_BATCH_BOUNDS_INVALID",
                    "firstIngressSequence must be positive"
            );
        }
        if (!TransportSequenceLimits.isValidWireSequence(batch.firstIngressSequence())
                || !TransportSequenceLimits.isValidWireSequence(batch.lastIngressSequence())) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_SEQUENCE_OUT_OF_BOUNDS",
                    "Batch ingress sequences must be within 1..9007199254740991"
            );
        }
        if (batch.lastIngressSequence() < batch.firstIngressSequence()) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_BATCH_BOUNDS_INVALID",
                    "lastIngressSequence cannot be less than firstIngressSequence"
            );
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
        long expectedCount = batch.lastIngressSequence() - batch.firstIngressSequence() + 1;
        if (batch.records().size() != expectedCount || batch.recordCount() != batch.records().size()) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_SEQUENCE_DISCONTINUITY",
                    "Batch record count does not match ingress sequence range ["
                            + batch.firstIngressSequence() + ".." + batch.lastIngressSequence() + "]"
            );
        }

        Set<Long> seen = new HashSet<>();
        for (AgentIngressRecord record : batch.records()) {
            long seq = record.sensorIngressSequence();
            if (seq < batch.firstIngressSequence() || seq > batch.lastIngressSequence()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_SEQUENCE_OUT_OF_BOUNDS",
                        "Record sensorIngressSequence " + seq + " is outside batch bounds"
                );
            }
            if (!seen.add(seq)) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_DUPLICATE_SEQUENCE",
                        "Duplicate sensorIngressSequence " + seq + " detected in batch"
                );
            }
            try {
                byte[] recordBytes = objectMapper.writeValueAsBytes(record);
                if (recordBytes.length > MAX_RECORD_BYTES) {
                    throw new IngestionException(
                            HttpStatus.PAYLOAD_TOO_LARGE,
                            "ERR_RECORD_TOO_LARGE",
                            "Record sensorIngressSequence " + seq + " exceeds maximum size"
                    );
                }
            } catch (IngestionException ie) {
                throw ie;
            } catch (Exception e) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_MALFORMED_RECORD",
                        "Failed to inspect agent ingress record bounds"
                );
            }
        }
        for (long s = batch.firstIngressSequence(); s <= batch.lastIngressSequence(); s++) {
            if (!seen.contains(s)) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_MISSING_SEQUENCE",
                        "Missing sensorIngressSequence " + s + " in batch range"
                );
            }
        }
        validateChecksum(batch);
    }

    public void validateChecksum(AgentTelemetryBatchRequest batch) {
        if (batch.checksum() == null || batch.checksum().isBlank()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_CHECKSUM_REQUIRED", "Batch checksum is required");
        }
        String calculated = calculateStableChecksum(batch.records());
        if (!calculated.equalsIgnoreCase(batch.checksum())) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_CHECKSUM_MISMATCH",
                    "Batch checksum mismatch"
            );
        }
    }

    /** Durable Sensor agent-spool stream grammar: {@code ^stream-[0-9a-f]{32}$}. */
    public static boolean isValidAgentStreamId(String streamId) {
        return streamId != null && AGENT_STREAM_ID.matcher(streamId).matches();
    }

    /**
     * Stable checksum shared with Sensor agentdrain.
     * Uses scalar identity fields only (no nested JSON re-serialization).
     */
    static String calculateStableChecksum(List<AgentIngressRecord> records) {
        if (records == null || records.isEmpty()) {
            return "";
        }
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            List<AgentIngressRecord> sorted = new ArrayList<>(records);
            sorted.sort(Comparator.comparingLong(AgentIngressRecord::sensorIngressSequence));
            for (AgentIngressRecord record : sorted) {
                StringBuilder line = new StringBuilder();
                line.append(record.sensorIngressSequence()).append('|')
                        .append(nullToEmpty(record.agentBatchId())).append('|')
                        .append(record.agentFirstSequence()).append('|')
                        .append(record.agentLastSequence()).append('|')
                        .append(record.assignmentGeneration()).append('|')
                        .append(nullToEmpty(record.tenantId())).append('|')
                        .append(nullToEmpty(record.agentId())).append('|')
                        .append(nullToEmpty(record.acceptingSensorId()));
                if (record.records() != null) {
                    for (JsonNode delivery : record.records()) {
                        line.append('|')
                                .append(delivery.path("sequence").asLong(0)).append('|')
                                .append(delivery.path("id").asText("")).append('|')
                                .append(delivery.path("checksum").asText(""));
                    }
                }
                digest.update(line.toString().getBytes(java.nio.charset.StandardCharsets.UTF_8));
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

    private static String nullToEmpty(String s) {
        return s == null ? "" : s;
    }

    private void validateHostEvidence(UUID tenantId, UUID entityId, String hostId) {
        if (hostId.isBlank()) {
            return;
        }
        Optional<EntityIdentifier> observed = entityIdentifierRepository.findByResolutionKey(
                tenantId, AGENT_ENTITY_NAMESPACE, AGENT_HOST_ID_TYPE, hostId);
        if (observed.isPresent() && !entityId.equals(observed.get().entityId())) {
            throw new IngestionException(HttpStatus.CONFLICT,
                    "ERR_AGENT_IDENTITY_CONFLICT", "Observed host identity conflicts with canonical Agent binding");
        }
    }

    public List<CanonicalObservation> extractCanonicalObservations(
            AgentTelemetryBatchRequest batch,
            SensorScope scope
    ) {
        List<CanonicalObservation> results = new ArrayList<>();
        Instant ingestTime = Instant.now();
        String authoritativeSensorId = scope.sensorId();
        UUID authoritativeTenantId = scope.tenantId();

        List<AgentIngressRecord> sorted = new ArrayList<>(batch.records());
        sorted.sort(Comparator.comparingLong(AgentIngressRecord::sensorIngressSequence));

        for (AgentIngressRecord ingress : sorted) {
            validateIngressRecordAuthority(ingress, authoritativeSensorId, authoritativeTenantId);

            UUID recordTenantId = UUID.fromString(ingress.tenantId().trim());
            Optional<AgentSensorAssignment> assignment = agentControlRepository.findAssignmentByGeneration(
                    recordTenantId, ingress.agentId().trim(), ingress.assignmentGeneration()
            );
            if (assignment.isEmpty()) {
                throw new IngestionException(
                        HttpStatus.FORBIDDEN,
                        "ERR_AGENT_ASSIGNMENT_UNKNOWN",
                        "Unknown Agent assignment generation " + ingress.assignmentGeneration()
                                + " for agent " + ingress.agentId()
                );
            }
            AgentSensorAssignment asg = assignment.get();
            if (!asg.sensorId().equals(authoritativeSensorId)) {
                throw new IngestionException(
                        HttpStatus.FORBIDDEN,
                        "ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH",
                        "Historical assignment sensor does not match authenticated Sensor"
                );
            }
            if (!asg.sensorId().equals(ingress.acceptingSensorId().trim())) {
                throw new IngestionException(
                        HttpStatus.FORBIDDEN,
                        "ERR_AGENT_ACCEPTING_SENSOR_MISMATCH",
                        "acceptingSensorId does not match historical assignment sensor"
                );
            }

            if (asg.status() == AgentAssignmentStatus.REVOKED) {
                validateRevokedAssignmentTemporalAuthority(ingress, asg);
            }

            if (ingress.records() == null || ingress.records().isEmpty()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_EMPTY_RECORDS",
                        "Agent ingress record " + ingress.sensorIngressSequence() + " has no delivery records"
                );
            }

            var binding = agentControlRepository.findActiveEntityBinding(recordTenantId, ingress.agentId().trim())
                    .orElseThrow(() -> new IngestionException(HttpStatus.FORBIDDEN,
                            "ERR_AGENT_ENTITY_BINDING_INACTIVE", "Active canonical Agent binding is required"));
            if (!recordTenantId.equals(binding.tenantId()) || !ingress.agentId().trim().equals(binding.agentId())) {
                throw new IngestionException(HttpStatus.FORBIDDEN,
                        "ERR_AGENT_IDENTITY_CONFLICT", "Canonical Agent binding scope mismatch");
            }

            for (JsonNode deliveryRecord : ingress.records()) {
                results.addAll(extractFromDeliveryRecord(
                        deliveryRecord,
                        binding,
                        ingress,
                        authoritativeSensorId,
                        recordTenantId,
                        ingestTime,
                        batch.correlationId()
                ));
            }
        }
        return results;
    }

    private void validateRevokedAssignmentTemporalAuthority(
            AgentIngressRecord ingress,
            AgentSensorAssignment asg
    ) {
        if (ingress.acceptedAt() == null || ingress.acceptedAt().isBlank()) {
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_REQUIRED",
                    "Sensor acceptance timestamp is required for revoked assignment generation"
            );
        }
        Instant acceptedInstant;
        try {
            acceptedInstant = Instant.parse(ingress.acceptedAt().trim());
        } catch (DateTimeParseException e) {
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_INVALID",
                    "Sensor acceptance timestamp is malformed for revoked assignment generation"
            );
        }
        if (acceptedInstant.isBefore(asg.createdAt())) {
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_REVOKED_ASSIGNMENT_ACCEPTED_BEFORE_ACTIVATION",
                    "Sensor acceptance timestamp is earlier than assignment activation"
            );
        }
        if (asg.revokedAt() == null) {
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_REVOKED_ASSIGNMENT_REVOKED_AT_MISSING",
                    "Revoked assignment is missing authoritative revocation timestamp"
            );
        }
        if (acceptedInstant.isAfter(asg.revokedAt())) {
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION",
                    "Sensor acceptance timestamp is later than assignment revocation"
            );
        }
    }

    private void validateIngressRecordAuthority(
            AgentIngressRecord ingress,
            String authoritativeSensorId,
            UUID authoritativeTenantId
    ) {
        if (ingress.tenantId() == null || ingress.tenantId().isBlank()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_TENANT_ID_REQUIRED", "tenantId is required");
        }
        if (ingress.agentId() == null || ingress.agentId().isBlank()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_AGENT_ID_REQUIRED", "agentId is required");
        }
        if (ingress.acceptingSensorId() == null || ingress.acceptingSensorId().isBlank()) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_ACCEPTING_SENSOR_REQUIRED",
                    "acceptingSensorId is required"
            );
        }
        if (!ingress.acceptingSensorId().trim().equals(authoritativeSensorId.trim())) {
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_AUTH_IDENTITY_MISMATCH",
                    "acceptingSensorId does not match authenticated Sensor"
            );
        }
        UUID claimedTenant;
        try {
            claimedTenant = UUID.fromString(ingress.tenantId().trim());
        } catch (IllegalArgumentException e) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_TENANT_ID_INVALID", "tenantId must be a UUID");
        }
        if (!claimedTenant.equals(authoritativeTenantId)) {
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_TENANT_SCOPE_MISMATCH",
                    "Record tenant does not match authoritative Sensor tenant"
            );
        }
        if (ingress.assignmentGeneration() < 1) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_ASSIGNMENT_GENERATION_INVALID",
                    "assignmentGeneration must be positive"
            );
        }
    }

    private List<CanonicalObservation> extractFromDeliveryRecord(
            JsonNode deliveryRecord,
            EntityAgentBinding binding,
            AgentIngressRecord ingress,
            String authoritativeSensorId,
            UUID tenantId,
            Instant ingestTime,
            String batchCorrelationId
    ) {
        if (deliveryRecord == null || !deliveryRecord.isObject()) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_MALFORMED_RECORD",
                    "Delivery record must be a JSON object"
            );
        }
        long agentSeq = deliveryRecord.path("sequence").asLong(0);
        String agentRecordId = deliveryRecord.path("id").asText("").trim();
        JsonNode payloadNode = deliveryRecord.path("payload");
        JsonNode collection;
        if (payloadNode.isTextual()) {
            try {
                collection = objectMapper.readTree(payloadNode.asText());
            } catch (Exception e) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_MALFORMED_RECORD_PAYLOAD",
                        "Delivery record payload is not valid JSON"
                );
            }
        } else if (payloadNode.isObject()) {
            collection = payloadNode;
        } else if (payloadNode.isBinary()) {
            try {
                collection = objectMapper.readTree(payloadNode.binaryValue());
            } catch (Exception e) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_MALFORMED_RECORD_PAYLOAD",
                        "Delivery record payload bytes are not valid JSON"
                );
            }
        } else {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_MALFORMED_RECORD_PAYLOAD",
                    "Delivery record payload is missing"
            );
        }

        String hostId = collection.path("hostId").asText("").trim();
        if (hostId.isBlank()) {
            JsonNode observations = collection.path("observations");
            if (observations.isArray() && !observations.isEmpty()) {
                hostId = observations.get(0).path("hostId").asText("").trim();
            }
        }
        // Collect evidence first so hostile distinct host IDs cannot cause one
        // database roundtrip per observation. Memory remains record-byte bounded.
        Set<String> checkedHosts = new HashSet<>();
        if (!hostId.isBlank()) checkedHosts.add(hostId);
        JsonNode hostObservations = collection.path("observations");
        if (hostObservations.isArray()) {
            for (JsonNode obs : hostObservations) {
                String observedHost = obs.path("hostId").asText("").trim();
                if (!observedHost.isBlank()) checkedHosts.add(observedHost);
            }
        }
        if (checkedHosts.size() == 1) {
            validateHostEvidence(tenantId, binding.entityId(), checkedHosts.iterator().next());
        } else if (!checkedHosts.isEmpty() && entityIdentifierRepository.hasConflictingHostEvidence(
                tenantId, binding.entityId(), List.copyOf(checkedHosts))) {
            throw new IngestionException(HttpStatus.CONFLICT, "ERR_AGENT_IDENTITY_CONFLICT",
                    "Observed host identity conflicts with canonical Agent binding");
        }
        String entityId = binding.entityId().toString();
        String correlation = ingress.correlationId() != null && !ingress.correlationId().isBlank()
                ? ingress.correlationId()
                : batchCorrelationId;

        JsonNode observations = collection.path("observations");
        if (!observations.isArray() || observations.isEmpty()) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_NON_NUMERIC_RECORD",
                    "Agent delivery payload contains no observations"
            );
        }

        List<CanonicalObservation> out = new ArrayList<>();
        int ordinal = 0;
        for (JsonNode obs : observations) {
            if (!obs.isObject()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_UNSUPPORTED_RECORD_TYPE",
                        "Agent observation is not an object"
                );
            }
            String metricId = obs.path("metricKey").asText("").trim();
            if (metricId.isBlank()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_MISSING_METRIC_ID",
                        "Agent observation missing metricKey"
                );
            }
            JsonNode valueNode = obs.path("value");
            if (!valueNode.isNumber()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_NON_NUMERIC_RECORD",
                        "Agent observation value is non-numeric"
                );
            }
            double value = valueNode.asDouble();
            if (!Double.isFinite(value)) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_MALFORMED_NUMERIC_VALUE",
                        "Agent observation value is not finite"
                );
            }
            Instant ts;
            String observedAt = obs.path("observedAt").asText(collection.path("collectedAt").asText(""));
            if (observedAt == null || observedAt.isBlank()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_INVALID_TIMESTAMP",
                        "Agent observation observedAt/collectedAt is required"
                );
            }
            try {
                ts = Instant.parse(observedAt);
            } catch (Exception e) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_INVALID_TIMESTAMP",
                        "Invalid Agent observation timestamp"
                );
            }

            String agentObsId = obs.path("observationId").asText("").trim();
            String stableSeqId = tenantId + ":"
                    + authoritativeSensorId + ":"
                    + ingress.agentId().trim() + ":"
                    + ingress.assignmentGeneration() + ":"
                    + agentSeq + ":"
                    + ordinal + ":"
                    + metricId;

            out.add(new CanonicalObservation(
                    tenantId.toString(),
                    entityId,
                    metricId,
                    authoritativeSensorId,
                    ts,
                    value,
                    1,
                    0,
                    stableSeqId,
                    SOURCE_PROTOCOL_AGENT_HOST,
                    SOURCE_TYPE_AGENT,
                    ingress.agentId().trim(),
                    ingress.assignmentGeneration(),
                    agentObsId.isBlank() ? null : agentObsId,
                    agentSeq > 0 ? agentSeq : null,
                    agentRecordId.isBlank() ? null : agentRecordId,
                    correlation,
                    ingestTime
            ));
            ordinal++;
        }
        return out;
    }
}
