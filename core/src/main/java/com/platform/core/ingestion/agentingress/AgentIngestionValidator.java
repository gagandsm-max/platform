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
import com.platform.core.ingestion.agentingress.dto.SensorAcceptanceEvidenceV1;
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
import java.time.DateTimeException;
import java.time.format.DateTimeParseException;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashSet;
import java.util.HexFormat;
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

    /** Immutable structure and server-derived scope checks precede all disposition lookup. */
    public void validateImmutableBatch(byte[] rawBody, AgentTelemetryBatchRequest batch, SensorScope scope) {
        validateBatchBounds(batch, rawBody, scope);
        for (AgentIngressRecord ingress : batch.records()) {
            validateIngressRecordAuthority(ingress, scope.sensorId(), scope.tenantId());
            acceptanceInterval(ingress);
        }
    }

    /** A new terminal skip requires current, not merely historical, assignment authority. */
    public EntityAgentBinding validateCurrentTerminalAuthority(AgentIngressRecord ingress, SensorScope scope) {
        validateIngressRecordAuthority(ingress, scope.sensorId(), scope.tenantId());
        AgentSensorAssignment assignment = agentControlRepository.findAssignmentByGeneration(
                scope.tenantId(), ingress.agentId().trim(), ingress.assignmentGeneration())
                .orElseThrow(() -> new IngestionException(HttpStatus.FORBIDDEN,
                        "ERR_AGENT_ASSIGNMENT_UNKNOWN", "Current Agent assignment is required"));
        if (assignment.status() != AgentAssignmentStatus.ACTIVE
                || !scope.sensorId().equals(assignment.sensorId())) {
            throw new IngestionException(HttpStatus.FORBIDDEN,
                    "ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH", "Current Agent assignment does not authorize this Sensor");
        }
        return agentControlRepository.findActiveEntityBinding(scope.tenantId(), ingress.agentId().trim())
                .orElseThrow(() -> new IngestionException(HttpStatus.FORBIDDEN,
                        "ERR_AGENT_ENTITY_BINDING_INACTIVE", "Active Agent binding is required"));
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
            if (record == null) {
                throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_MALFORMED_RECORD",
                        "Null Agent ingress record is not allowed");
            }
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
        return validateAndSnapshot(batch, scope).observations();
    }

    /** Binding rows are ordered by ingress sequence and checkpointed with the canonical digest. */
    public record ValidatedBatch(List<CanonicalObservation> observations,
            List<PendingCanonicalBindingV1> bindings) {}

    public ValidatedBatch validateAndSnapshot(AgentTelemetryBatchRequest batch, SensorScope scope) {
        return validateAndSnapshot(batch, scope, false);
    }

    public ValidatedBatch validateLegacyAndSnapshot(AgentTelemetryBatchRequest batch, SensorScope scope) {
        return validateAndSnapshot(batch, scope, true);
    }

    private ValidatedBatch validateAndSnapshot(AgentTelemetryBatchRequest batch, SensorScope scope,
            boolean requireHistoricalEntityProof) {
        List<CanonicalObservation> results = new ArrayList<>();
        List<PendingCanonicalBindingV1> bindings = new ArrayList<>();
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
            } else if (requireHistoricalEntityProof) {
                ClockInterval interval = acceptanceInterval(ingress);
                if (interval == null || asg.createdAt() == null
                        || interval.lower().isBefore(asg.createdAt())) {
                    throw new IngestionException(HttpStatus.FORBIDDEN,
                            "ERR_AGENT_AUTHORITY_ORDER_UNCERTAIN",
                            "Qualified Sensor acceptance precedes the historical assignment");
                }
            }

            if (ingress.records() == null || ingress.records().isEmpty()) {
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_EMPTY_RECORDS",
                        "Agent ingress record " + ingress.sensorIngressSequence() + " has no delivery records"
                );
            }

            boolean historical = requireHistoricalEntityProof || asg.status() == AgentAssignmentStatus.REVOKED;
            EntityAgentBinding activeBinding = null;
            if (!historical) {
                activeBinding = agentControlRepository.findActiveEntityBinding(
                        recordTenantId, ingress.agentId().trim())
                        .orElseThrow(() -> new IngestionException(HttpStatus.FORBIDDEN,
                                "ERR_AGENT_ENTITY_BINDING_INACTIVE", "Active canonical Agent binding is required"));
            }
            String hostId = recordHostId(ingress);
            EntityIdentifier identifier = entityIdentifierRepository.findByResolutionKey(
                    recordTenantId, AGENT_ENTITY_NAMESPACE, AGENT_HOST_ID_TYPE, hostId)
                    .orElseThrow(() -> new IngestionException(HttpStatus.FORBIDDEN,
                            "ERR_AGENT_ENTITY_IDENTIFIER_UNPROVEN", "Canonical host identifier is unavailable"));
            if (!recordTenantId.equals(identifier.tenantId()) || identifier.entityId() == null
                    || !AGENT_ENTITY_NAMESPACE.equals(identifier.namespace())
                    || !AGENT_HOST_ID_TYPE.equals(identifier.identifierType())
                    || !hostId.equals(identifier.identifierValue())) {
                throw new IngestionException(HttpStatus.FORBIDDEN,
                        "ERR_AGENT_IDENTITY_CONFLICT", "Canonical host identifier scope mismatch");
            }
            long bindingVersion;
            if (historical) {
                ClockInterval interval = acceptanceInterval(ingress);
                if (interval == null || identifier.firstSeenAt() == null
                        || identifier.firstSeenAt().isAfter(interval.lower())
                        || !identifierCreatedNoLaterThan(identifier.identifierId(), interval.lower())
                        || identifier.confidence() == null
                        || identifier.confidence().compareTo(java.math.BigDecimal.ONE) != 0) {
                    throw new IngestionException(HttpStatus.FORBIDDEN,
                            "ERR_AGENT_LEGACY_ENTITY_UNPROVEN", "Historical canonical entity mapping is unproven");
                }
                bindingVersion = asg.generation();
            } else {
                if (!recordTenantId.equals(activeBinding.tenantId())
                        || !ingress.agentId().trim().equals(activeBinding.agentId())
                        || !identifier.entityId().equals(activeBinding.entityId())) {
                    throw new IngestionException(HttpStatus.FORBIDDEN,
                            "ERR_AGENT_IDENTITY_CONFLICT", "Canonical Agent binding or host mapping conflicts");
                }
                bindingVersion = activeBinding.reinstallGeneration();
            }
            PendingCanonicalBindingV1 canonical = new PendingCanonicalBindingV1(
                    ingress.sensorIngressSequence(), ingress.agentId().trim(),
                    ingress.assignmentGeneration(), hostId, identifier.identifierId(),
                    identifier.entityId(), bindingVersion);
            bindings.add(canonical);

            for (JsonNode deliveryRecord : ingress.records()) {
                results.addAll(extractFromDeliveryRecord(
                        deliveryRecord,
                        canonical.entityId(), !historical, hostId,
                        ingress,
                        authoritativeSensorId,
                        recordTenantId,
                        ingestTime,
                        batch.correlationId()
                ));
            }
        }
        return new ValidatedBatch(List.copyOf(results), List.copyOf(bindings));
    }

    /** Exact validated retry uses only the persisted canonical binding snapshot. */
    public List<CanonicalObservation> extractValidatedSnapshot(
            AgentTelemetryBatchRequest batch, SensorScope scope, List<PendingCanonicalBindingV1> bindings) {
        if (bindings == null || bindings.size() != batch.records().size()) {
            throw new IngestionException(HttpStatus.SERVICE_UNAVAILABLE,
                    "ERR_AGENT_AUTHORITY_CHECKPOINT_UNAVAILABLE", "Validated entity snapshot is incomplete");
        }
        List<AgentIngressRecord> sorted = new ArrayList<>(batch.records());
        sorted.sort(Comparator.comparingLong(AgentIngressRecord::sensorIngressSequence));
        List<CanonicalObservation> results = new ArrayList<>();
        Instant ingestTime = Instant.now();
        for (int i = 0; i < sorted.size(); i++) {
            AgentIngressRecord ingress = sorted.get(i);
            validateIngressRecordAuthority(ingress, scope.sensorId(), scope.tenantId());
            PendingCanonicalBindingV1 binding = bindings.get(i);
            if (binding == null || binding.entityId() == null || binding.entityIdentifierId() == null
                    || binding.sensorIngressSequence() != ingress.sensorIngressSequence()
                    || !binding.agentId().equals(ingress.agentId().trim())
                    || binding.assignmentGeneration() != ingress.assignmentGeneration()
                    || ingress.records() == null || ingress.records().isEmpty()
                    || !binding.hostId().equals(recordHostId(ingress))) {
                throw new IngestionException(HttpStatus.SERVICE_UNAVAILABLE,
                        "ERR_AGENT_AUTHORITY_CHECKPOINT_UNAVAILABLE", "Validated record snapshot is incomplete");
            }
            for (JsonNode delivery : ingress.records()) {
                results.addAll(extractFromDeliveryRecord(delivery, binding.entityId(), false, binding.hostId(),
                        ingress, scope.sensorId(), scope.tenantId(), ingestTime, batch.correlationId()));
            }
        }
        return results;
    }

    /** Stable canonical effect digest excludes only the per-attempt Core ingest time. */
    public String canonicalBatchDigest(List<CanonicalObservation> observations) {
        try {
            List<CanonicalObservation> stable = observations.stream().map(o -> new CanonicalObservation(
                    o.tenantId(), o.entityId(), o.metricId(), o.sensorId(), o.timestamp(),
                    o.value(), o.quality(), o.flags(), o.sequenceId(), o.sourceProtocol(),
                    o.sourceType(), o.agentId(), o.assignmentGeneration(), o.agentObservationId(),
                    o.agentSourceSequence(), o.agentSourceRecordId(), o.correlationId(), null)).toList();
            return HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256")
                    .digest(objectMapper.writeValueAsBytes(stable)));
        } catch (Exception e) {
            throw new IngestionException(HttpStatus.SERVICE_UNAVAILABLE,
                    "ERR_AGENT_CANONICAL_DIGEST_UNAVAILABLE", "Canonical observation digest could not be computed", e);
        }
    }

    private void validateRevokedAssignmentTemporalAuthority(
            AgentIngressRecord ingress,
            AgentSensorAssignment asg
    ) {
        ClockInterval interval = acceptanceInterval(ingress);
        if (interval == null || asg.createdAt() == null || asg.revokedAt() == null
                || interval.lower().isBefore(asg.createdAt())
                || !interval.upper().isBefore(asg.revokedAt())) {
            throw new IngestionException(HttpStatus.FORBIDDEN,
                    "ERR_AGENT_AUTHORITY_ORDER_UNCERTAIN",
                    "Qualified Sensor acceptance interval does not fit the historical assignment");
        }
    }

    private static boolean identifierCreatedNoLaterThan(UUID identifierId, Instant boundary) {
        if (identifierId == null || identifierId.version() != 7) return false;
        long mintedAtMs = identifierId.getMostSignificantBits() >>> 16;
        return !Instant.ofEpochMilli(mintedAtMs).isAfter(boundary);
    }

    private JsonNode collectionNode(JsonNode deliveryRecord) {
        if (deliveryRecord == null || !deliveryRecord.isObject()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST,
                    "ERR_MALFORMED_RECORD", "Delivery record must be a JSON object");
        }
        JsonNode payloadNode = deliveryRecord.path("payload");
        if (payloadNode.isObject()) return payloadNode;
        try {
            if (payloadNode.isTextual()) return objectMapper.readTree(payloadNode.asText());
            if (payloadNode.isBinary()) return objectMapper.readTree(payloadNode.binaryValue());
        } catch (Exception e) {
            throw new IngestionException(HttpStatus.BAD_REQUEST,
                    "ERR_MALFORMED_RECORD_PAYLOAD", "Delivery record payload is not valid JSON");
        }
        throw new IngestionException(HttpStatus.BAD_REQUEST,
                "ERR_MALFORMED_RECORD_PAYLOAD", "Delivery record payload is missing");
    }

    private String recordHostId(AgentIngressRecord ingress) {
        String expected = null;
        for (JsonNode delivery : ingress.records()) {
            JsonNode collection = collectionNode(delivery);
            String host = collection.path("hostId").asText("").trim();
            JsonNode observations = collection.path("observations");
            if (host.isBlank() && observations.isArray() && !observations.isEmpty()) {
                host = observations.get(0).path("hostId").asText("").trim();
            }
            if (host.isBlank() || (expected != null && !expected.equals(host))) {
                throw new IngestionException(HttpStatus.CONFLICT,
                        "ERR_AGENT_IDENTITY_CONFLICT", "Ingress record host identity is missing or conflicting");
            }
            if (observations.isArray()) {
                for (JsonNode observation : observations) {
                    String observedHost = observation.path("hostId").asText("").trim();
                    if (!observedHost.isBlank() && !host.equals(observedHost)) {
                        throw new IngestionException(HttpStatus.CONFLICT,
                                "ERR_AGENT_IDENTITY_CONFLICT", "Observation host identity conflicts with ingress record");
                    }
                }
            }
            expected = host;
        }
        if (expected == null) {
            throw new IngestionException(HttpStatus.BAD_REQUEST,
                    "ERR_EMPTY_RECORDS", "Agent ingress record has no delivery records");
        }
        return expected;
    }

    private record ClockInterval(Instant lower, Instant upper) {}

    private static IngestionException invalidAcceptanceEvidence() {
        return new IngestionException(HttpStatus.BAD_REQUEST,
                "ERR_SENSOR_ACCEPTANCE_EVIDENCE_INVALID", "Sensor acceptance evidence is malformed or unbounded");
    }

    private static long boundedClockInteger(JsonNode node, long maximum) {
        if (node == null || !node.isIntegralNumber() || !node.canConvertToLong()) {
            throw invalidAcceptanceEvidence();
        }
        long value = node.longValue();
        if (value < 0 || value > maximum) throw invalidAcceptanceEvidence();
        return value;
    }

    /** Null means the Sensor could not prove a bounded historical interval. */
    private ClockInterval acceptanceInterval(AgentIngressRecord ingress) {
        SensorAcceptanceEvidenceV1 evidence = ingress.acceptanceEvidence();
        if (evidence == null) return null; // pre-evidence Sensor: ACTIVE only
        if (evidence.evidenceVersion() == null || !evidence.evidenceVersion().isIntegralNumber()
                || !evidence.evidenceVersion().canConvertToInt()
                || evidence.evidenceVersion().intValue() != 1
                || evidence.acceptedAtUtc() == null || !evidence.acceptedAtUtc().endsWith("Z")
                || evidence.clockSource() == null || evidence.clockSource().isBlank()
                || evidence.sensorServiceInstanceId() == null || evidence.sensorServiceInstanceId().isBlank()
                || evidence.durableSpoolSequence() == null
                || !evidence.durableSpoolSequence().isIntegralNumber()
                || !evidence.durableSpoolSequence().canConvertToLong()
                || evidence.durableSpoolSequence().longValue() != ingress.sensorIngressSequence()) {
            throw invalidAcceptanceEvidence();
        }
        Instant accepted;
        try {
            accepted = Instant.parse(evidence.acceptedAtUtc());
            if (ingress.acceptedAt() != null && !ingress.acceptedAt().isBlank()
                    && !accepted.equals(Instant.parse(ingress.acceptedAt()))) {
                throw invalidAcceptanceEvidence();
            }
        } catch (DateTimeParseException e) {
            throw invalidAcceptanceEvidence();
        }
        if ("UNQUALIFIED".equals(evidence.clockQuality())) {
            if (evidence.clockMaxErrorMs() != null || evidence.clockSyncAgeMs() != null) {
                throw invalidAcceptanceEvidence();
            }
            return null;
        }
        if (!"QUALIFIED".equals(evidence.clockQuality())) throw invalidAcceptanceEvidence();
        long errorMs = boundedClockInteger(evidence.clockMaxErrorMs(), 5000);
        boundedClockInteger(evidence.clockSyncAgeMs(), 300000);
        try {
            Instant lower = accepted.minusMillis(errorMs);
            Instant upper = accepted.plusMillis(errorMs);
            if (lower.isAfter(accepted) || upper.isBefore(accepted) || lower.isAfter(upper)) {
                throw invalidAcceptanceEvidence();
            }
            return new ClockInterval(lower, upper);
        } catch (DateTimeException | ArithmeticException e) {
            throw invalidAcceptanceEvidence();
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
            UUID canonicalEntityId,
            boolean validateCurrentHostEvidence,
            String expectedHostId,
            AgentIngressRecord ingress,
            String authoritativeSensorId,
            UUID tenantId,
            Instant ingestTime,
            String batchCorrelationId
    ) {
        if (deliveryRecord == null || !deliveryRecord.isObject()) {
            throw new IngestionException(HttpStatus.BAD_REQUEST,
                    "ERR_MALFORMED_RECORD", "Delivery record must be a JSON object");
        }
        long agentSeq = deliveryRecord.path("sequence").asLong(0);
        String agentRecordId = deliveryRecord.path("id").asText("").trim();
        JsonNode collection = collectionNode(deliveryRecord);

        String hostId = collection.path("hostId").asText("").trim();
        if (hostId.isBlank()) {
            JsonNode observations = collection.path("observations");
            if (observations.isArray() && !observations.isEmpty()) {
                hostId = observations.get(0).path("hostId").asText("").trim();
            }
        }
        if (expectedHostId != null && !expectedHostId.equals(hostId)) {
            throw new IngestionException(HttpStatus.CONFLICT,
                    "ERR_AGENT_IDENTITY_CONFLICT", "Replay host identity differs from canonical binding");
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
        if (validateCurrentHostEvidence && checkedHosts.size() == 1) {
            validateHostEvidence(tenantId, canonicalEntityId, checkedHosts.iterator().next());
        } else if (validateCurrentHostEvidence && !checkedHosts.isEmpty() && entityIdentifierRepository.hasConflictingHostEvidence(
                tenantId, canonicalEntityId, List.copyOf(checkedHosts))) {
            throw new IngestionException(HttpStatus.CONFLICT, "ERR_AGENT_IDENTITY_CONFLICT",
                    "Observed host identity conflicts with canonical Agent binding");
        }
        String entityId = canonicalEntityId.toString();
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
