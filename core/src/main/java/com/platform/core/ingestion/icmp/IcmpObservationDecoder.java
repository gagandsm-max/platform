package com.platform.core.ingestion.icmp;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.math.BigDecimal;
import java.math.MathContext;
import java.net.InetAddress;
import java.net.UnknownHostException;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.time.format.DateTimeParseException;
import java.util.ArrayList;
import java.util.Iterator;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * Deterministic decoder for BUILD-020 / PR #217 {@code icmp_observation_v1}.
 *
 * <p><b>NOT_WIRED</b> into {@link com.platform.core.ingestion.IngestionValidator}
 * or any production admission/ACK path. Slice A contract only.
 */
public final class IcmpObservationDecoder {

    /** Matches Sensor ICMP bootstrap {@code MaxSamples: 8}. */
    public static final int MAX_SAMPLES = 8;
    public static final int MAX_ENVELOPE_BYTES = 512 * 1024;
    public static final int MAX_IDENTITY_CHARS = 256;
    public static final int MAX_BATCH_ID_CHARS = 128;
    public static final int MAX_RECORD_TYPE_CHARS = 128;
    public static final int MAX_PROTOCOL_FIELD_CHARS = 64;
    public static final int MAX_ERROR_CODE_CHARS = 128;
    public static final int MAX_SOURCE_CHARS = 128;
    public static final String EXPECTED_SOURCE = "SENSOR_PING_SOCKET";
    public static final String EXPECTED_OPERATION = "ECHO";

    private static final Set<String> ENVELOPE_FIELDS = Set.of(
            "schemaVersion", "batchId", "recordType", "collectedAt", "sensorId", "tenantId",
            "entityId", "job", "protocol", "protocolVersion", "operation", "status", "quality",
            "protocolSchemaVersion", "protocolPayload"
    );
    private static final Set<String> JOB_FIELDS = Set.of(
            "jobId", "assignmentKey", "assignmentVersion", "attempt"
    );
    private static final Set<String> EVIDENCE_FIELDS = Set.of(
            "schemaVersion", "target", "ipVersion", "source", "startedAt", "completedAt",
            "requestedSamples", "received", "packetLossPct", "minRttMicros", "avgRttMicros",
            "maxRttMicros", "samples", "errorCode"
    );
    private static final Set<String> SAMPLE_FIELDS = Set.of(
            "sequence", "success", "rttMicros", "errorCode"
    );
    private static final Set<String> ALLOWED_SAMPLE_ERROR_CODES = Set.of(
            "ICMP_TIMEOUT", "ICMP_UNREACHABLE"
    );
    private static final Set<String> ALLOWED_STATUSES = Set.of(
            "COLLECTED", "COLLECTED_WITH_LOSS", "PARTIAL", "COLLECTION_FAILED"
    );
    private static final BigDecimal LOSS_EPSILON = new BigDecimal("0.000001");

    private final ObjectMapper objectMapper;

    public IcmpObservationDecoder() {
        this(new ObjectMapper().findAndRegisterModules());
    }

    public IcmpObservationDecoder(ObjectMapper objectMapper) {
        this.objectMapper = objectMapper != null ? objectMapper : new ObjectMapper().findAndRegisterModules();
    }

    public IcmpObservationV1 decode(byte[] utf8Json) {
        if (utf8Json == null || utf8Json.length == 0) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE,
                    "ICMP observation envelope bytes are required"
            );
        }
        if (utf8Json.length > MAX_ENVELOPE_BYTES) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS,
                    "ICMP observation envelope exceeds maximum of " + MAX_ENVELOPE_BYTES + " bytes"
            );
        }
        try {
            return decode(objectMapper.readTree(utf8Json));
        } catch (IcmpDecodeException ex) {
            throw ex;
        } catch (Exception ex) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE,
                    "ICMP observation envelope is not valid JSON",
                    ex
            );
        }
    }

    public IcmpObservationV1 decode(String utf8Json) {
        if (utf8Json == null) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE,
                    "ICMP observation envelope text is required"
            );
        }
        return decode(utf8Json.getBytes(StandardCharsets.UTF_8));
    }

    public IcmpObservationV1 decode(JsonNode envelopeNode) {
        if (envelopeNode == null || envelopeNode.isNull() || !envelopeNode.isObject()) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE,
                    "ICMP observation envelope must be a JSON object"
            );
        }
        rejectUnknownFields(envelopeNode, ENVELOPE_FIELDS, IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE, "envelope");

        int schemaVersion = requirePositiveInt(envelopeNode, "schemaVersion",
                IcmpDecodeException.Code.ERR_ICMP_UNSUPPORTED_SCHEMA, "envelope schemaVersion");
        if (schemaVersion != IcmpObservationV1.ENVELOPE_SCHEMA_VERSION) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_UNSUPPORTED_SCHEMA,
                    "Unsupported ICMP envelope schemaVersion " + schemaVersion
                            + "; expected " + IcmpObservationV1.ENVELOPE_SCHEMA_VERSION
            );
        }

        String recordType = requireBoundedText(envelopeNode, "recordType", 1, MAX_RECORD_TYPE_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE);
        if (!IcmpObservationV1.RECORD_TYPE.equals(recordType)) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_UNSUPPORTED_SCHEMA,
                    "Unsupported ICMP recordType '" + recordType + "'; expected "
                            + IcmpObservationV1.RECORD_TYPE
            );
        }

        String protocol = requireBoundedText(envelopeNode, "protocol", 1, MAX_PROTOCOL_FIELD_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE);
        if (!IcmpObservationV1.PROTOCOL.equalsIgnoreCase(protocol)) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_UNSUPPORTED_SCHEMA,
                    "Unsupported protocol '" + protocol + "'; expected ICMP"
            );
        }

        int protocolSchemaVersion = requirePositiveInt(envelopeNode, "protocolSchemaVersion",
                IcmpDecodeException.Code.ERR_ICMP_UNSUPPORTED_SCHEMA, "protocolSchemaVersion");
        if (protocolSchemaVersion != IcmpObservationV1.PROTOCOL_SCHEMA_VERSION) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_UNSUPPORTED_SCHEMA,
                    "Unsupported protocolSchemaVersion " + protocolSchemaVersion
                            + "; expected " + IcmpObservationV1.PROTOCOL_SCHEMA_VERSION
            );
        }

        String batchId = requireBoundedText(envelopeNode, "batchId", 1, MAX_BATCH_ID_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE);
        Instant collectedAt = requireInstant(envelopeNode, "collectedAt",
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE);
        String sensorId = requireBoundedText(envelopeNode, "sensorId", 1, MAX_IDENTITY_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE);
        String tenantId = requireBoundedText(envelopeNode, "tenantId", 1, MAX_IDENTITY_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE);
        String entityId = requireBoundedText(envelopeNode, "entityId", 1, MAX_IDENTITY_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE);
        IcmpJobCorrelation job = decodeJob(envelopeNode.get("job"));

        String protocolVersion = requireBoundedText(envelopeNode, "protocolVersion", 1, MAX_PROTOCOL_FIELD_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE);
        String operation = requireBoundedText(envelopeNode, "operation", 1, MAX_PROTOCOL_FIELD_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE);
        if (!EXPECTED_OPERATION.equals(operation)) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE,
                    "Unsupported ICMP operation '" + operation + "'; expected " + EXPECTED_OPERATION
            );
        }
        String status = requireBoundedText(envelopeNode, "status", 1, MAX_PROTOCOL_FIELD_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE);
        if (!ALLOWED_STATUSES.contains(status)) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE,
                    "Unsupported ICMP status '" + status + "'"
            );
        }

        IcmpQuality quality = IcmpQuality.parseRequired(textOrNull(envelopeNode, "quality"));
        JsonNode payload = envelopeNode.get("protocolPayload");
        if (payload == null || payload.isNull() || !payload.isObject()) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE,
                    "ICMP protocolPayload Evidence object is required"
            );
        }
        // Reject numeric-projection masquerades — ICMP evidence is not metric_id/value or SNMP observations[].
        if (payload.has("observations") || payload.has("metric_id") || payload.has("value")) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_UNEXPECTED_FIELD,
                    "ICMP protocolPayload must not carry SNMP observations[] or metric_id/value numeric projection"
            );
        }

        IcmpEvidence evidence = decodeEvidence(payload);
        validateEnvelopeEvidenceAgreement(protocolVersion, status, quality, evidence);

        return new IcmpObservationV1(
                schemaVersion,
                batchId,
                recordType,
                collectedAt,
                sensorId,
                tenantId,
                entityId,
                job,
                IcmpObservationV1.PROTOCOL,
                protocolVersion,
                operation,
                status,
                quality,
                protocolSchemaVersion,
                evidence
        );
    }

    private IcmpJobCorrelation decodeJob(JsonNode jobNode) {
        if (jobNode == null || jobNode.isNull() || !jobNode.isObject()) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE,
                    "ICMP job correlation object is required"
            );
        }
        rejectUnknownFields(jobNode, JOB_FIELDS, IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE, "job");
        String jobId = requireBoundedText(jobNode, "jobId", 1, MAX_IDENTITY_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE);
        String assignmentKey = requireBoundedText(jobNode, "assignmentKey", 1, MAX_IDENTITY_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE);
        long assignmentVersion = requireNonNegativeLong(jobNode, "assignmentVersion",
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE, "assignmentVersion");
        long attempt = requireNonNegativeLong(jobNode, "attempt",
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE, "attempt");
        return new IcmpJobCorrelation(jobId, assignmentKey, assignmentVersion, attempt);
    }

    private IcmpEvidence decodeEvidence(JsonNode evidenceNode) {
        rejectUnknownFields(evidenceNode, EVIDENCE_FIELDS, IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE, "evidence");

        int schemaVersion = requirePositiveInt(evidenceNode, "schemaVersion",
                IcmpDecodeException.Code.ERR_ICMP_UNSUPPORTED_SCHEMA, "evidence schemaVersion");
        if (schemaVersion != IcmpObservationV1.EVIDENCE_SCHEMA_VERSION) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_UNSUPPORTED_SCHEMA,
                    "Unsupported ICMP evidence schemaVersion " + schemaVersion
                            + "; expected " + IcmpObservationV1.EVIDENCE_SCHEMA_VERSION
            );
        }

        String target = requireBoundedText(evidenceNode, "target", 1, MAX_IDENTITY_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET);
        int ipVersion = requirePositiveInt(evidenceNode, "ipVersion",
                IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET, "ipVersion");
        if (ipVersion != 4 && ipVersion != 6) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET,
                    "ICMP ipVersion must be 4 or 6"
            );
        }
        validateTarget(target, ipVersion);

        String source = requireBoundedText(evidenceNode, "source", 1, MAX_SOURCE_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE);
        if (!EXPECTED_SOURCE.equals(source)) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE,
                    "Unsupported ICMP source vantage '" + source + "'; expected " + EXPECTED_SOURCE
            );
        }

        Instant startedAt = requireInstant(evidenceNode, "startedAt",
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE);
        Instant completedAt = requireInstant(evidenceNode, "completedAt",
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE);
        if (completedAt.isBefore(startedAt)) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS,
                    "ICMP completedAt must not precede startedAt"
            );
        }

        Integer requestedSamples = optionalBoundedInt(evidenceNode, "requestedSamples", 1, MAX_SAMPLES,
                IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS);
        int received = requireNonNegativeInt(evidenceNode, "received",
                IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS, "received");
        if (requestedSamples != null && received > requestedSamples) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                    "ICMP received (" + received + ") exceeds requestedSamples/sent (" + requestedSamples + ")"
            );
        }
        if (requestedSamples == null && received != 0) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                    "ICMP received must be 0 when requestedSamples/sent is omitted"
            );
        }

        Double packetLossPct = optionalFiniteDouble(evidenceNode, "packetLossPct",
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE);
        Long minRtt = optionalNonNegativeLong(evidenceNode, "minRttMicros",
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE);
        Long avgRtt = optionalNonNegativeLong(evidenceNode, "avgRttMicros",
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE);
        Long maxRtt = optionalNonNegativeLong(evidenceNode, "maxRttMicros",
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE);
        String errorCode = optionalBoundedText(evidenceNode, "errorCode", 1, MAX_ERROR_CODE_CHARS,
                IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE);

        List<IcmpSample> samples = decodeSamples(evidenceNode.get("samples"));
        validateSampleAggregate(requestedSamples, received, packetLossPct, minRtt, avgRtt, maxRtt, samples, errorCode);

        return new IcmpEvidence(
                schemaVersion,
                target,
                ipVersion,
                source,
                startedAt,
                completedAt,
                requestedSamples,
                received,
                packetLossPct,
                minRtt,
                avgRtt,
                maxRtt,
                samples,
                errorCode
        );
    }

    private List<IcmpSample> decodeSamples(JsonNode samplesNode) {
        if (samplesNode == null || samplesNode.isNull()) {
            return List.of();
        }
        if (!samplesNode.isArray()) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE,
                    "ICMP samples must be a JSON array when present"
            );
        }
        if (samplesNode.size() > MAX_SAMPLES) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS,
                    "ICMP samples exceed maximum of " + MAX_SAMPLES
            );
        }
        List<IcmpSample> samples = new ArrayList<>(samplesNode.size());
        for (int i = 0; i < samplesNode.size(); i++) {
            JsonNode sampleNode = samplesNode.get(i);
            if (sampleNode == null || !sampleNode.isObject()) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE,
                        "ICMP sample at index " + i + " must be an object"
                );
            }
            rejectUnknownFields(sampleNode, SAMPLE_FIELDS, IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE, "sample");
            int sequence = requirePositiveInt(sampleNode, "sequence",
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES, "sample.sequence");
            if (!sampleNode.has("success") || !sampleNode.get("success").isBoolean()) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE,
                        "ICMP sample sequence " + sequence + " requires boolean success"
                );
            }
            boolean success = sampleNode.get("success").asBoolean();
            Long rttMicros = optionalNonNegativeLong(sampleNode, "rttMicros",
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE);
            String sampleError = optionalBoundedText(sampleNode, "errorCode", 1, MAX_ERROR_CODE_CHARS,
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE);
            if (success) {
                if (sampleError != null) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                            "Successful ICMP sample sequence " + sequence + " must not carry errorCode"
                    );
                }
                if (rttMicros == null) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                            "Successful ICMP sample sequence " + sequence + " requires rttMicros"
                    );
                }
            } else {
                if (rttMicros != null) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                            "Failed ICMP sample sequence " + sequence + " must omit rttMicros"
                    );
                }
                if (sampleError == null || !ALLOWED_SAMPLE_ERROR_CODES.contains(sampleError)) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                            "Failed ICMP sample sequence " + sequence
                                    + " requires errorCode ICMP_TIMEOUT or ICMP_UNREACHABLE"
                    );
                }
            }
            samples.add(new IcmpSample(sequence, success, rttMicros, sampleError));
        }
        return List.copyOf(samples);
    }

    private void validateSampleAggregate(
            Integer requestedSamples,
            int received,
            Double packetLossPct,
            Long minRtt,
            Long avgRtt,
            Long maxRtt,
            List<IcmpSample> samples,
            String errorCode
    ) {
        if (requestedSamples == null) {
            // Collection-failure Evidence: no samples, no invented loss/RTT.
            if (!samples.isEmpty()) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                        "ICMP samples must be omitted when requestedSamples/sent is omitted"
                );
            }
            if (packetLossPct != null || minRtt != null || avgRtt != null || maxRtt != null) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                        "ICMP packet loss and RTT must be omitted when probe transmission is unknown"
                );
            }
            if (errorCode == null || errorCode.isBlank()) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE,
                        "ICMP errorCode is required when requestedSamples/sent is omitted"
                );
            }
            return;
        }

        if (samples.isEmpty()) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                    "ICMP samples are required when requestedSamples/sent is present"
            );
        }
        if (samples.size() != requestedSamples) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                    "ICMP samples length (" + samples.size() + ") must equal requestedSamples/sent ("
                            + requestedSamples + ")"
            );
        }

        int successCount = 0;
        for (int i = 0; i < samples.size(); i++) {
            IcmpSample sample = samples.get(i);
            if (sample.sequence() != i + 1) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                        "ICMP sample sequences must be contiguous starting at 1"
                );
            }
            if (sample.success()) {
                successCount++;
            }
        }
        if (successCount != received) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                    "ICMP received (" + received + ") does not match successful sample count (" + successCount + ")"
            );
        }

        boolean hasUnreachable = samples.stream()
                .anyMatch(s -> "ICMP_UNREACHABLE".equals(s.errorCode()));
        if (hasUnreachable) {
            // Sensor clears packetLossPct when transmission is unknown due to unreachable.
            if (packetLossPct != null) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                        "ICMP packetLossPct must be omitted when any sample is ICMP_UNREACHABLE"
                );
            }
        } else if (packetLossPct == null) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                    "ICMP packetLossPct is required for completed probe sets without unreachable samples"
            );
        } else {
            BigDecimal expected = BigDecimal.valueOf(requestedSamples - received)
                    .multiply(BigDecimal.valueOf(100))
                    .divide(BigDecimal.valueOf(requestedSamples), MathContext.DECIMAL64);
            BigDecimal actual = BigDecimal.valueOf(packetLossPct);
            if (actual.subtract(expected).abs().compareTo(LOSS_EPSILON) > 0) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                        "ICMP packetLossPct " + packetLossPct + " is inconsistent with sent/received"
                );
            }
            if (packetLossPct < 0.0d || packetLossPct > 100.0d) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS,
                        "ICMP packetLossPct must be within [0, 100]"
                );
            }
        }

        if (received == 0) {
            if (minRtt != null || avgRtt != null || maxRtt != null) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                        "ICMP RTT aggregates must be omitted when received is 0"
                );
            }
        } else {
            if (minRtt == null || avgRtt == null || maxRtt == null) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                        "ICMP RTT aggregates are required when received > 0"
                );
            }
            if (avgRtt < minRtt || maxRtt < avgRtt) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                        "ICMP RTT aggregates must satisfy min <= avg <= max"
                );
            }
        }
    }

    private void validateEnvelopeEvidenceAgreement(
            String protocolVersion,
            String status,
            IcmpQuality quality,
            IcmpEvidence evidence
    ) {
        String expectedVersion = Integer.toString(evidence.ipVersion());
        if (!expectedVersion.equals(protocolVersion)) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                    "ICMP protocolVersion '" + protocolVersion
                            + "' must match evidence ipVersion " + evidence.ipVersion()
            );
        }
        if ("COLLECTION_FAILED".equals(status)) {
            if (quality != IcmpQuality.UNKNOWN) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INVALID_QUALITY,
                        "COLLECTION_FAILED ICMP observations require quality UNKNOWN"
                );
            }
            if (evidence.requestedSamples() != null || evidence.errorCode() == null) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                        "COLLECTION_FAILED requires omitted requestedSamples and a present errorCode"
                );
            }
        }
        // Never invent or accept DEVICE_DOWN as quality/status — already rejected by enums/sets.
    }

    private static void validateTarget(String target, int ipVersion) {
        if (target.indexOf('/') >= 0 || target.indexOf(' ') >= 0 || !isIpLiteralText(target)) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET,
                    "ICMP target must be an IP literal"
            );
        }
        final InetAddress address;
        try {
            address = InetAddress.getByName(target);
        } catch (UnknownHostException ex) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET,
                    "ICMP target must be an IP literal",
                    ex
            );
        }
        byte[] raw = address.getAddress();
        if (raw == null) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET,
                    "ICMP target must be an IP literal"
            );
        }
        boolean ipv4 = raw.length == 4;
        boolean ipv6 = raw.length == 16;
        if (ipVersion == 4 && !ipv4) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET,
                    "ICMP target does not match IPv4 selection"
            );
        }
        if (ipVersion == 6 && (!ipv6 || target.indexOf(':') < 0)) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET,
                    "ICMP target does not match IPv6 selection"
            );
        }
    }

    private static boolean isIpLiteralText(String target) {
        if (target.indexOf(':') >= 0) {
            return true;
        }
        String[] parts = target.split("\\.");
        if (parts.length != 4) {
            return false;
        }
        for (String part : parts) {
            if (part.isEmpty() || part.length() > 3) {
                return false;
            }
            for (int i = 0; i < part.length(); i++) {
                char c = part.charAt(i);
                if (c < '0' || c > '9') {
                    return false;
                }
            }
            int value = Integer.parseInt(part);
            if (value < 0 || value > 255) {
                return false;
            }
        }
        return true;
    }

    private static void rejectUnknownFields(
            JsonNode node,
            Set<String> allowed,
            IcmpDecodeException.Code code,
            String where
    ) {
        Iterator<Map.Entry<String, JsonNode>> fields = node.fields();
        while (fields.hasNext()) {
            String name = fields.next().getKey();
            if (!allowed.contains(name)) {
                throw new IcmpDecodeException(
                        code,
                        "Unexpected " + where + " field '" + name + "'"
                );
            }
        }
    }

    private static String textOrNull(JsonNode node, String field) {
        JsonNode child = node.get(field);
        if (child == null || child.isNull()) {
            return null;
        }
        if (!child.isTextual()) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE,
                    "Field '" + field + "' must be a JSON string"
            );
        }
        return child.asText();
    }

    private static String requireBoundedText(
            JsonNode node,
            String field,
            int minInclusive,
            int maxInclusive,
            IcmpDecodeException.Code code
    ) {
        JsonNode child = node.get(field);
        if (child == null || child.isNull() || !child.isTextual()) {
            throw new IcmpDecodeException(code, "Field '" + field + "' must be a non-null JSON string");
        }
        String value = child.asText();
        if (value.length() < minInclusive || value.length() > maxInclusive) {
            throw new IcmpDecodeException(
                    code,
                    "Field '" + field + "' length must be within " + minInclusive + ".." + maxInclusive
            );
        }
        if (value.isBlank()) {
            throw new IcmpDecodeException(code, "Field '" + field + "' must not be blank");
        }
        return value;
    }

    private static String optionalBoundedText(
            JsonNode node,
            String field,
            int minInclusive,
            int maxInclusive,
            IcmpDecodeException.Code code
    ) {
        if (!node.has(field) || node.get(field).isNull()) {
            return null;
        }
        return requireBoundedText(node, field, minInclusive, maxInclusive, code);
    }

    private static Instant requireInstant(JsonNode node, String field, IcmpDecodeException.Code code) {
        String raw = requireBoundedText(node, field, 1, 64, code);
        try {
            return Instant.parse(raw);
        } catch (DateTimeParseException ex) {
            throw new IcmpDecodeException(code, "Field '" + field + "' must be an ISO-8601 instant", ex);
        }
    }

    private static int requirePositiveInt(JsonNode node, String field, IcmpDecodeException.Code code, String label) {
        JsonNode child = node.get(field);
        if (child == null || child.isNull() || !child.isIntegralNumber() || child.isBigInteger()) {
            throw new IcmpDecodeException(code, label + " must be a positive JSON integer");
        }
        long value = child.asLong();
        if (value <= 0 || value > Integer.MAX_VALUE) {
            throw new IcmpDecodeException(code, label + " must be a positive integer");
        }
        return (int) value;
    }

    private static int requireNonNegativeInt(JsonNode node, String field, IcmpDecodeException.Code code, String label) {
        JsonNode child = node.get(field);
        if (child == null || child.isNull() || !child.isIntegralNumber() || child.isBigInteger()) {
            throw new IcmpDecodeException(code, label + " must be a non-negative JSON integer");
        }
        long value = child.asLong();
        if (value < 0 || value > Integer.MAX_VALUE) {
            throw new IcmpDecodeException(code, label + " must be a non-negative integer");
        }
        return (int) value;
    }

    private static Integer optionalBoundedInt(
            JsonNode node,
            String field,
            int minInclusive,
            int maxInclusive,
            IcmpDecodeException.Code code
    ) {
        if (!node.has(field) || node.get(field).isNull()) {
            return null;
        }
        JsonNode child = node.get(field);
        if (!child.isIntegralNumber() || child.isBigInteger()) {
            throw new IcmpDecodeException(code, "Field '" + field + "' must be a JSON integer");
        }
        long value = child.asLong();
        if (value < minInclusive || value > maxInclusive) {
            throw new IcmpDecodeException(
                    code,
                    "Field '" + field + "' must be within " + minInclusive + ".." + maxInclusive
            );
        }
        return (int) value;
    }

    private static long requireNonNegativeLong(JsonNode node, String field, IcmpDecodeException.Code code, String label) {
        JsonNode child = node.get(field);
        if (child == null || child.isNull() || !child.isIntegralNumber() || child.isBigInteger()) {
            throw new IcmpDecodeException(code, label + " must be a non-negative JSON integer");
        }
        long value = child.asLong();
        if (value < 0) {
            throw new IcmpDecodeException(code, label + " must be a non-negative integer");
        }
        return value;
    }

    private static Long optionalNonNegativeLong(JsonNode node, String field, IcmpDecodeException.Code code) {
        if (!node.has(field) || node.get(field).isNull()) {
            return null;
        }
        JsonNode child = node.get(field);
        if (!child.isIntegralNumber() || child.isBigInteger()) {
            throw new IcmpDecodeException(code, "Field '" + field + "' must be a JSON integer");
        }
        long value = child.asLong();
        if (value < 0) {
            throw new IcmpDecodeException(code, "Field '" + field + "' must be non-negative");
        }
        return value;
    }

    private static Double optionalFiniteDouble(JsonNode node, String field, IcmpDecodeException.Code code) {
        if (!node.has(field) || node.get(field).isNull()) {
            return null;
        }
        JsonNode child = node.get(field);
        if (!child.isNumber()) {
            throw new IcmpDecodeException(code, "Field '" + field + "' must be a JSON number");
        }
        double value = child.asDouble();
        if (!Double.isFinite(value)) {
            throw new IcmpDecodeException(code, "Field '" + field + "' must be a finite number");
        }
        return value;
    }
}
