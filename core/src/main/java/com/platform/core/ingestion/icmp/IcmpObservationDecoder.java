package com.platform.core.ingestion.icmp;

import com.fasterxml.jackson.core.JsonParser;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.IOException;
import java.io.OutputStream;
import java.math.BigDecimal;
import java.math.BigInteger;
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
    /**
     * Maximum positive RTT in microseconds that Go {@code time.Duration} can publish via
     * {@code Duration.Microseconds()}. Sensor ICMP sample and aggregate RTT fields are
     * produced exclusively from {@code time.Duration} nanoseconds
     * ({@code sensor/internal/protocol/icmp/icmp.go}); values above this bound are outside
     * the source representation domain (not an operational timeout policy).
     */
    public static final long MAX_RTT_MICROS = Long.MAX_VALUE / 1000L;
    /**
     * Mother B000-D016 / Platform_05 §7.0: JSON-carried assignment/config/policy/generation
     * counters MUST NOT exceed {@code MAX_SAFE_SEQUENCE} ({@code 2^53-1}). Do not widen to
     * full Go {@code uint64} wire range contrary to that governing bound.
     */
    public static final long MAX_ASSIGNMENT_VERSION = 9007199254740991L;
    /** Sensor {@code observation.JobCorrelation.Attempt} is {@code uint32}. */
    public static final long MAX_ATTEMPT = 4_294_967_295L;
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
        try (JsonParser parser = objectMapper.createParser(utf8Json)) {
            JsonNode tree = objectMapper.readTree(parser);
            // Match Sensor observation.DecodeBounded: require EOF after one value.
            if (parser.nextToken() != null) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE,
                        "ICMP observation envelope contains trailing JSON data"
                );
            }
            return decode(tree);
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
        // JsonNode callers must not bypass the documented envelope size bound.
        enforceEnvelopeByteBound(envelopeNode);
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
        validateCollectedAtCompletedAt(collectedAt, evidence.completedAt());
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

    private void enforceEnvelopeByteBound(JsonNode envelopeNode) {
        // Count serialized UTF-8 bytes without retaining an arbitrarily large copy.
        BoundingOutputStream bound = new BoundingOutputStream(MAX_ENVELOPE_BYTES);
        try {
            objectMapper.writeValue(bound, envelopeNode);
        } catch (Exception ex) {
            if (isBoundExceeded(ex)) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS,
                        "ICMP observation envelope exceeds maximum of " + MAX_ENVELOPE_BYTES + " bytes"
                );
            }
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE,
                    "ICMP observation envelope cannot be measured for size bounds",
                    ex
            );
        }
    }

    private static boolean isBoundExceeded(Throwable ex) {
        for (Throwable cursor = ex; cursor != null; cursor = cursor.getCause()) {
            if (cursor instanceof BoundExceededException) {
                return true;
            }
        }
        return false;
    }

    private static void validateCollectedAtCompletedAt(Instant collectedAt, Instant completedAt) {
        // Sensor icmpintegration sets Envelope.CollectedAt = Evidence.CompletedAt.
        // Consistency only — Sensor-authored timestamps are not Core grant authority.
        if (!collectedAt.equals(completedAt)) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                    "ICMP collectedAt must equal evidence completedAt"
            );
        }
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
        long assignmentVersion = requireWireCounter(jobNode, "assignmentVersion", MAX_ASSIGNMENT_VERSION,
                IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS, "assignmentVersion");
        long attempt = requireWireCounter(jobNode, "attempt", MAX_ATTEMPT,
                IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS, "attempt");
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
                // Go Sample.RTTMicros uses omitempty: sub-microsecond success serializes without
                // rttMicros. Preserve wire zero — never invent a positive RTT.
                if (rttMicros == null) {
                    rttMicros = 0L;
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

        if (errorCode != null) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                    "ICMP collection-level errorCode is reserved for collection failures"
            );
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
            validateRttSummariesAgainstSamples(minRtt, avgRtt, maxRtt, samples);
        }
    }

    /**
     * Sensor derives min/max/avg from raw {@code time.Duration} nanoseconds, then truncates
     * each published microsecond field via {@code Duration.Microseconds()}. Sample
     * {@code rttMicros} values are already truncated, so avg may differ from the integer
     * mean of sample micros by at most the residual sub-microsecond budget — never reject
     * that Sensor-faithful rounding window.
     */
    private static void validateRttSummariesAgainstSamples(
            long minRtt,
            long avgRtt,
            long maxRtt,
            List<IcmpSample> samples
    ) {
        long sampleMin = Long.MAX_VALUE;
        long sampleMax = Long.MIN_VALUE;
        // Exact sum — signed long addition / *1000 can overflow for large but
        // representable sample RTT values across up to MAX_SAMPLES successes.
        BigInteger sumMicros = BigInteger.ZERO;
        int successCount = 0;
        for (IcmpSample sample : samples) {
            if (!sample.success()) {
                continue;
            }
            long rtt = sample.rttMicros() != null ? sample.rttMicros() : 0L;
            sampleMin = Math.min(sampleMin, rtt);
            sampleMax = Math.max(sampleMax, rtt);
            sumMicros = sumMicros.add(BigInteger.valueOf(rtt));
            successCount++;
        }
        if (successCount == 0) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                    "ICMP RTT aggregates require at least one successful sample"
            );
        }
        if (minRtt != sampleMin || maxRtt != sampleMax) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                    "ICMP min/max RTT must match successful sample rttMicros"
            );
        }
        // Reconstruct the inclusive avg window from truncated sample micros:
        // raw_ns_i ∈ [si*1000, si*1000+999], so avg_us ∈ [low, high] below.
        // Matches Go time.Duration nanosecond→microsecond truncation
        // (Duration.Microseconds / integer division toward zero).
        BigInteger n = BigInteger.valueOf(successCount);
        BigInteger thousand = BigInteger.valueOf(1000L);
        BigInteger sumNanosLow = sumMicros.multiply(thousand);
        BigInteger sumNanosHigh = sumNanosLow.add(BigInteger.valueOf(999L).multiply(n));
        long avgLow = sumNanosLow.divide(n).divide(thousand).longValueExact();
        long avgHigh = sumNanosHigh.divide(n).divide(thousand).longValueExact();
        if (avgRtt < avgLow || avgRtt > avgHigh) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                    "ICMP avgRttMicros is inconsistent with successful sample rttMicros"
            );
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

        boolean hasUnreachable = evidence.samples().stream()
                .anyMatch(s -> "ICMP_UNREACHABLE".equals(s.errorCode()));
        boolean observedLoss = evidence.requestedSamples() != null
                && evidence.received() < evidence.requestedSamples();
        boolean lossPctPositive = evidence.packetLossPct() != null && evidence.packetLossPct() > 0.0d;

        // Preserve BUILD-020 icmpintegration status/quality producer matrix. Never translate
        // missing evidence into DEVICE_DOWN (already rejected by quality/status allow-lists).
        switch (status) {
            case "COLLECTION_FAILED" -> {
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
            case "COLLECTED" -> {
                if (quality != IcmpQuality.GOOD) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INVALID_QUALITY,
                            "COLLECTED ICMP observations require quality GOOD"
                    );
                }
                if (evidence.requestedSamples() == null) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                            "COLLECTED requires requestedSamples/sent"
                    );
                }
                if (hasUnreachable || observedLoss || lossPctPositive) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                            "COLLECTED/GOOD is incompatible with observed loss or unreachable samples"
                    );
                }
            }
            case "COLLECTED_WITH_LOSS" -> {
                if (quality != IcmpQuality.PARTIAL) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INVALID_QUALITY,
                            "COLLECTED_WITH_LOSS ICMP observations require quality PARTIAL"
                    );
                }
                if (evidence.requestedSamples() == null) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                            "COLLECTED_WITH_LOSS requires requestedSamples/sent"
                    );
                }
                if (hasUnreachable) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                            "ICMP_UNREACHABLE samples require status PARTIAL with quality UNKNOWN"
                    );
                }
                if (!observedLoss) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                            "COLLECTED_WITH_LOSS requires received < requestedSamples/sent"
                    );
                }
            }
            case "PARTIAL" -> {
                if (quality != IcmpQuality.UNKNOWN) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INVALID_QUALITY,
                            "PARTIAL ICMP status requires quality UNKNOWN"
                    );
                }
                if (evidence.requestedSamples() == null) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                            "PARTIAL status requires requestedSamples/sent"
                    );
                }
                if (!hasUnreachable) {
                    throw new IcmpDecodeException(
                            IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES,
                            "PARTIAL status requires at least one ICMP_UNREACHABLE sample"
                    );
                }
            }
            default -> throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE,
                    "Unsupported ICMP status '" + status + "'"
            );
        }
    }

    private static void validateTarget(String target, int ipVersion) {
        // Match Go net.ParseIP: reject scoped IPv6 (%zone) and ambiguous IPv4 (leading zeros).
        if (target.indexOf('%') >= 0
                || target.indexOf('/') >= 0
                || target.indexOf(' ') >= 0) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET,
                    "ICMP target must be an IP literal"
            );
        }
        if (ipVersion == 4) {
            if (!isStrictIpv4Literal(target)) {
                throw new IcmpDecodeException(
                        IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET,
                        "ICMP target must be an IP literal"
                );
            }
            return;
        }
        if (!isStrictIpv6LiteralShape(target)) {
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
        // Require a true 16-byte IPv6 result; reject forms Java collapses to Inet4Address
        // (aligned with Go validate rejecting ip.To4() != nil for IPVersion 6).
        if (raw == null || raw.length != 16) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET,
                    "ICMP target does not match IPv6 selection"
            );
        }
    }

    /** Go {@code net.ParseIP} IPv4: dotted-decimal, no leading zeros per octet. */
    private static boolean isStrictIpv4Literal(String target) {
        String[] parts = target.split("\\.", -1);
        if (parts.length != 4) {
            return false;
        }
        for (String part : parts) {
            if (part.isEmpty() || part.length() > 3) {
                return false;
            }
            if (part.length() > 1 && part.charAt(0) == '0') {
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

    /** Character-class gate before InetAddress; rejects zones and non-literal text. */
    private static boolean isStrictIpv6LiteralShape(String target) {
        if (target.indexOf(':') < 0) {
            return false;
        }
        for (int i = 0; i < target.length(); i++) {
            char c = target.charAt(i);
            boolean ok = (c >= '0' && c <= '9')
                    || (c >= 'a' && c <= 'f')
                    || (c >= 'A' && c <= 'F')
                    || c == ':'
                    || c == '.';
            if (!ok) {
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

    private static long requireWireCounter(
            JsonNode node,
            String field,
            long maxInclusive,
            IcmpDecodeException.Code boundsCode,
            String label
    ) {
        JsonNode child = node.get(field);
        if (child == null || child.isNull() || !child.isIntegralNumber()) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE,
                    label + " must be a non-negative JSON integer"
            );
        }
        // BigInteger implies value outside signed int64 — still outside B000-D016 / uint32 caps.
        if (child.isBigInteger()) {
            throw new IcmpDecodeException(
                    boundsCode,
                    label + " exceeds governing wire-counter maximum of " + maxInclusive
            );
        }
        long value = child.asLong();
        if (value < 0 || value > maxInclusive) {
            throw new IcmpDecodeException(
                    boundsCode,
                    label + " must be within 0.." + maxInclusive
            );
        }
        return value;
    }

    /** Discards serialized bytes while enforcing {@link #MAX_ENVELOPE_BYTES}. */
    private static final class BoundingOutputStream extends OutputStream {
        private final int maxBytes;
        private int count;

        BoundingOutputStream(int maxBytes) {
            this.maxBytes = maxBytes;
        }

        @Override
        public void write(int b) throws IOException {
            // Check before increment so count cannot wrap past Integer.MAX_VALUE
            // before the 512 KiB bound is applied.
            if (count >= maxBytes) {
                throw new BoundExceededException();
            }
            count++;
        }

        @Override
        public void write(byte[] b, int off, int len) throws IOException {
            if (len < 0) {
                throw new IndexOutOfBoundsException();
            }
            // Remaining-capacity check avoids int overflow of count += len when len
            // is large (e.g. near Integer.MAX_VALUE) before the bound is applied.
            if (len > maxBytes - count) {
                throw new BoundExceededException();
            }
            count += len;
        }
    }

    private static final class BoundExceededException extends IOException {
        BoundExceededException() {
            super("envelope byte bound exceeded");
        }
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
        // Zero remains valid (Go omitempty sub-µs success and explicit wire zeros).
        if (value > MAX_RTT_MICROS) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS,
                    "Field '" + field + "' exceeds maximum representable Sensor RTT of "
                            + MAX_RTT_MICROS + " microseconds"
            );
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
