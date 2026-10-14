package com.platform.core.ingestion.icmp;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Deterministic contract tests for Issue #218 Slice A ICMP typed decoder.
 * Fixtures mirror PR #217 {@code icmpintegration.Evidence} + {@code observation.Envelope}.
 * Decoder remains NOT_WIRED to production ingestion.
 */
class IcmpObservationDecoderTest {

    private static final String COLLECTED_AT = "2026-10-11T14:00:00Z";
    private static final String STARTED_AT = "2026-10-11T13:59:59.900Z";
    private static final String TENANT_ID = "11111111-1111-1111-1111-111111111111";
    private static final String ENTITY_ID = "22222222-2222-2222-2222-222222222222";

    private ObjectMapper objectMapper;
    private IcmpObservationDecoder decoder;

    @BeforeEach
    void setUp() {
        objectMapper = new ObjectMapper().findAndRegisterModules();
        decoder = new IcmpObservationDecoder(objectMapper);
    }

    @Test
    void decodesSourceFaithfulGoodIpv4Observation() throws Exception {
        String json = goodIpv4EnvelopeJson();
        IcmpObservationV1 obs = decoder.decode(json);

        assertEquals(1, obs.schemaVersion());
        assertEquals(IcmpObservationV1.RECORD_TYPE, obs.recordType());
        assertEquals(IcmpObservationV1.PROTOCOL, obs.protocol());
        assertEquals("ECHO", obs.operation());
        assertEquals("COLLECTED", obs.status());
        assertEquals(IcmpQuality.GOOD, obs.quality());
        assertEquals(TENANT_ID, obs.tenantId());
        assertEquals(ENTITY_ID, obs.entityId());
        assertEquals("sensor-net-01", obs.sensorId());
        assertEquals("icmp-job-1", obs.job().jobId());
        assertEquals("icmp-assign-1", obs.job().assignmentKey());
        assertEquals(1L, obs.job().assignmentVersion());
        assertEquals(1L, obs.job().attempt());

        IcmpEvidence evidence = obs.evidence();
        assertEquals("127.0.0.1", evidence.target());
        assertEquals(4, evidence.ipVersion());
        assertEquals("SENSOR_PING_SOCKET", evidence.source());
        assertEquals(2, evidence.requestedSamples());
        assertEquals(2, evidence.sent().orElseThrow());
        assertEquals(2, evidence.received());
        assertEquals(0.0d, evidence.packetLossPct());
        assertEquals(120L, evidence.minRttMicros());
        assertEquals(150L, evidence.avgRttMicros());
        assertEquals(180L, evidence.maxRttMicros());
        assertNull(evidence.errorCode());
        assertEquals(2, evidence.samples().size());
        assertTrue(evidence.samples().get(0).success());
        assertEquals(120L, evidence.samples().get(0).rttMicros());
        assertNull(evidence.samples().get(0).errorCode());
    }

    @Test
    void preservesPartialQualityWithNullableLossSemantics() {
        IcmpObservationV1 obs = decoder.decode(partialLossEnvelopeJson());
        assertEquals(IcmpQuality.PARTIAL, obs.quality());
        assertEquals("COLLECTED_WITH_LOSS", obs.status());
        assertEquals(2, obs.evidence().sent().orElseThrow());
        assertEquals(1, obs.evidence().received());
        assertEquals(50.0d, obs.evidence().packetLossPct());
        assertEquals(1000L, obs.evidence().avgRttMicros());
        assertFalse(obs.evidence().samples().get(1).success());
        assertEquals("ICMP_TIMEOUT", obs.evidence().samples().get(1).errorCode());
        assertNull(obs.evidence().samples().get(1).rttMicros());
    }

    @Test
    void preservesUnknownQualityWhenUnreachableOmitsPacketLoss() {
        IcmpObservationV1 obs = decoder.decode(unreachableEnvelopeJson());
        assertEquals(IcmpQuality.UNKNOWN, obs.quality());
        assertEquals("PARTIAL", obs.status());
        assertNull(obs.evidence().packetLossPct());
        assertTrue(obs.evidence().packetLossPctOptional().isEmpty());
        assertEquals("ICMP_UNREACHABLE", obs.evidence().samples().get(1).errorCode());
        assertNotEquals("DEVICE_DOWN", obs.status());
        assertNotEquals("DOWN", obs.quality().name());
    }

    @Test
    void preservesUnknownCollectionFailureWithoutDummyNumericDown() {
        IcmpObservationV1 obs = decoder.decode(collectionFailedEnvelopeJson());
        assertEquals(IcmpQuality.UNKNOWN, obs.quality());
        assertEquals("COLLECTION_FAILED", obs.status());
        assertTrue(obs.evidence().sent().isEmpty());
        assertNull(obs.evidence().requestedSamples());
        assertEquals(0, obs.evidence().received());
        assertNull(obs.evidence().packetLossPct());
        assertNull(obs.evidence().avgRttMicros());
        assertTrue(obs.evidence().samples().isEmpty());
        assertEquals("ICMP_PERMISSION_DENIED", obs.evidence().errorCode());
        // No invented numeric DOWN / loss projection.
        assertFalse(obs.evidence().errorCode().contains("DEVICE_DOWN"));
    }

    @Test
    void decodesIpv6TargetAndProtocolVersion() {
        IcmpObservationV1 obs = decoder.decode(goodIpv6EnvelopeJson());
        assertEquals(6, obs.evidence().ipVersion());
        assertEquals("2001:db8::1", obs.evidence().target());
        assertEquals("6", obs.protocolVersion());
        assertEquals(IcmpQuality.GOOD, obs.quality());
    }

    @Test
    void acceptsSourceFaithfulZeroMicrosecondSuccessOmitempty() {
        // Go Sample.RTTMicros omitempty: RTT < 1µs serializes without rttMicros.
        IcmpObservationV1 obs = decoder.decode(zeroMicrosecondOmitemptyEnvelopeJson());
        assertEquals("COLLECTED", obs.status());
        assertEquals(IcmpQuality.GOOD, obs.quality());
        assertEquals(1, obs.evidence().received());
        assertEquals(0L, obs.evidence().minRttMicros());
        assertEquals(0L, obs.evidence().avgRttMicros());
        assertEquals(0L, obs.evidence().maxRttMicros());
        IcmpSample sample = obs.evidence().samples().get(0);
        assertTrue(sample.success());
        assertEquals(0L, sample.rttMicros());
        assertNull(sample.errorCode());
    }

    @Test
    void rejectsGoodQualityDespiteObservedLoss() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(partialLossEnvelopeJson());
        root.put("status", "COLLECTED");
        root.put("quality", "GOOD");
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES, ex.getCode());
    }

    @Test
    void rejectsGoodQualityWithUnreachableSample() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(unreachableEnvelopeJson());
        root.put("status", "COLLECTED");
        root.put("quality", "GOOD");
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES, ex.getCode());
    }

    @Test
    void rejectsCollectedWithLossWithIncompatibleQuality() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(partialLossEnvelopeJson());
        root.put("quality", "GOOD");
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INVALID_QUALITY, ex.getCode());

        root.put("quality", "UNKNOWN");
        ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INVALID_QUALITY, ex.getCode());
    }

    @Test
    void rejectsCollectionFailedWithoutUnknownQuality() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(collectionFailedEnvelopeJson());
        root.put("quality", "PARTIAL");
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INVALID_QUALITY, ex.getCode());
    }

    @Test
    void rejectsJsonNodeEnvelopeLargerThanBound() throws Exception {
        ObjectNode root = objectMapper.createObjectNode();
        root.put("pad", "x".repeat(IcmpObservationDecoder.MAX_ENVELOPE_BYTES));
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS, ex.getCode());
    }

    @Test
    void rejectsCollectedAtNotEqualToCompletedAt() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(goodIpv4EnvelopeJson());
        root.put("collectedAt", "2026-10-11T14:00:01Z");
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES, ex.getCode());
        assertTrue(ex.getMessage().contains("collectedAt"));
    }

    @Test
    void rejectsMalformedEnvelopeJson() {
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode("{not-json"));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_MALFORMED_ENVELOPE, ex.getCode());
    }

    @Test
    void rejectsUnsupportedSchemaVersion() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(goodIpv4EnvelopeJson());
        root.put("schemaVersion", 2);
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_UNSUPPORTED_SCHEMA, ex.getCode());
    }

    @Test
    void rejectsInvalidQualityIncludingDeviceDown() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(goodIpv4EnvelopeJson());
        root.put("quality", "DEVICE_DOWN");
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INVALID_QUALITY, ex.getCode());

        root.put("quality", "DEGRADED");
        ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INVALID_QUALITY, ex.getCode());
    }

    @Test
    void rejectsTargetIpVersionMismatch() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(goodIpv4EnvelopeJson());
        ((ObjectNode) root.get("protocolPayload")).put("ipVersion", 6);
        root.put("protocolVersion", "6");
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET, ex.getCode());
    }

    @Test
    void rejectsHostnameTarget() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(goodIpv4EnvelopeJson());
        ((ObjectNode) root.get("protocolPayload")).put("target", "example.com");
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INVALID_TARGET, ex.getCode());
    }

    @Test
    void rejectsInconsistentPacketLoss() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(partialLossEnvelopeJson());
        ((ObjectNode) root.get("protocolPayload")).put("packetLossPct", 99.0d);
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES, ex.getCode());
    }

    @Test
    void rejectsInventedRttWhenNothingReceived() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(allTimeoutEnvelopeJson());
        ObjectNode payload = (ObjectNode) root.get("protocolPayload");
        payload.put("minRttMicros", 0);
        payload.put("avgRttMicros", 0);
        payload.put("maxRttMicros", 0);
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INCONSISTENT_SAMPLES, ex.getCode());
    }

    @Test
    void rejectsNumericProjectionMasqueradeInsideProtocolPayload() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(goodIpv4EnvelopeJson());
        ObjectNode payload = (ObjectNode) root.get("protocolPayload");
        payload.put("metric_id", "icmp.loss");
        payload.put("value", 100);
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_UNEXPECTED_FIELD, ex.getCode());
    }

    @Test
    void rejectsUnknownEvidenceField() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(goodIpv4EnvelopeJson());
        ((ObjectNode) root.get("protocolPayload")).put("deviceDown", true);
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_MALFORMED_EVIDENCE, ex.getCode());
    }

    @Test
    void rejectsOversizeSampleCount() throws Exception {
        ObjectNode root = (ObjectNode) objectMapper.readTree(goodIpv4EnvelopeJson());
        ObjectNode payload = (ObjectNode) root.get("protocolPayload");
        payload.put("requestedSamples", 9);
        payload.put("received", 9);
        payload.putArray("samples");
        for (int i = 1; i <= 9; i++) {
            ObjectNode sample = payload.withArray("samples").addObject();
            sample.put("sequence", i);
            sample.put("success", true);
            sample.put("rttMicros", 100);
        }
        payload.put("packetLossPct", 0.0d);
        payload.put("minRttMicros", 100);
        payload.put("avgRttMicros", 100);
        payload.put("maxRttMicros", 100);
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(root));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS, ex.getCode());
    }

    @Test
    void rejectsEnvelopeLargerThanBound() {
        byte[] huge = ("{\"pad\":\"" + "x".repeat(IcmpObservationDecoder.MAX_ENVELOPE_BYTES) + "\"}")
                .getBytes(StandardCharsets.UTF_8);
        IcmpDecodeException ex = assertThrows(IcmpDecodeException.class, () -> decoder.decode(huge));
        assertEquals(IcmpDecodeException.Code.ERR_ICMP_INVALID_BOUNDS, ex.getCode());
    }

    @Test
    void qualityEnumValuesRemainDistinct() {
        assertEquals(3, IcmpQuality.values().length);
        assertEquals(IcmpQuality.GOOD, IcmpQuality.parseRequired("good"));
        assertEquals(IcmpQuality.PARTIAL, IcmpQuality.parseRequired("PARTIAL"));
        assertEquals(IcmpQuality.UNKNOWN, IcmpQuality.parseRequired("Unknown"));
        assertThrows(IcmpDecodeException.class, () -> IcmpQuality.parseRequired("DOWN"));
    }

    // --- source-faithful fixtures (PR #217 / characterization helpers) ---

    private String goodIpv4EnvelopeJson() {
        return envelope(
                "COLLECTED",
                "GOOD",
                "4",
                """
                {
                  "schemaVersion":1,
                  "target":"127.0.0.1",
                  "ipVersion":4,
                  "source":"SENSOR_PING_SOCKET",
                  "startedAt":"%s",
                  "completedAt":"%s",
                  "requestedSamples":2,
                  "received":2,
                  "packetLossPct":0.0,
                  "minRttMicros":120,
                  "avgRttMicros":150,
                  "maxRttMicros":180,
                  "samples":[
                    {"sequence":1,"success":true,"rttMicros":120},
                    {"sequence":2,"success":true,"rttMicros":180}
                  ]
                }
                """.formatted(STARTED_AT, COLLECTED_AT)
        );
    }

    /**
     * Source-faithful Go wire: successful Echo Reply with RTT &lt; 1µs omits
     * {@code rttMicros} via {@code omitempty}; Evidence pointer aggregates still
     * emit explicit zeros when received &gt; 0.
     */
    private String zeroMicrosecondOmitemptyEnvelopeJson() {
        return envelope(
                "COLLECTED",
                "GOOD",
                "4",
                """
                {
                  "schemaVersion":1,
                  "target":"127.0.0.1",
                  "ipVersion":4,
                  "source":"SENSOR_PING_SOCKET",
                  "startedAt":"%s",
                  "completedAt":"%s",
                  "requestedSamples":1,
                  "received":1,
                  "packetLossPct":0.0,
                  "minRttMicros":0,
                  "avgRttMicros":0,
                  "maxRttMicros":0,
                  "samples":[
                    {"sequence":1,"success":true}
                  ]
                }
                """.formatted(STARTED_AT, COLLECTED_AT)
        );
    }

    private String goodIpv6EnvelopeJson() {
        return envelope(
                "COLLECTED",
                "GOOD",
                "6",
                """
                {
                  "schemaVersion":1,
                  "target":"2001:db8::1",
                  "ipVersion":6,
                  "source":"SENSOR_PING_SOCKET",
                  "startedAt":"%s",
                  "completedAt":"%s",
                  "requestedSamples":1,
                  "received":1,
                  "packetLossPct":0.0,
                  "minRttMicros":200,
                  "avgRttMicros":200,
                  "maxRttMicros":200,
                  "samples":[
                    {"sequence":1,"success":true,"rttMicros":200}
                  ]
                }
                """.formatted(STARTED_AT, COLLECTED_AT)
        );
    }

    private String partialLossEnvelopeJson() {
        return envelope(
                "COLLECTED_WITH_LOSS",
                "PARTIAL",
                "4",
                """
                {
                  "schemaVersion":1,
                  "target":"127.0.0.1",
                  "ipVersion":4,
                  "source":"SENSOR_PING_SOCKET",
                  "startedAt":"%s",
                  "completedAt":"%s",
                  "requestedSamples":2,
                  "received":1,
                  "packetLossPct":50.0,
                  "minRttMicros":1000,
                  "avgRttMicros":1000,
                  "maxRttMicros":1000,
                  "samples":[
                    {"sequence":1,"success":true,"rttMicros":1000},
                    {"sequence":2,"success":false,"errorCode":"ICMP_TIMEOUT"}
                  ]
                }
                """.formatted(STARTED_AT, COLLECTED_AT)
        );
    }

    private String unreachableEnvelopeJson() {
        return envelope(
                "PARTIAL",
                "UNKNOWN",
                "4",
                """
                {
                  "schemaVersion":1,
                  "target":"127.0.0.1",
                  "ipVersion":4,
                  "source":"SENSOR_PING_SOCKET",
                  "startedAt":"%s",
                  "completedAt":"%s",
                  "requestedSamples":2,
                  "received":1,
                  "minRttMicros":1000,
                  "avgRttMicros":1000,
                  "maxRttMicros":1000,
                  "samples":[
                    {"sequence":1,"success":true,"rttMicros":1000},
                    {"sequence":2,"success":false,"errorCode":"ICMP_UNREACHABLE"}
                  ]
                }
                """.formatted(STARTED_AT, COLLECTED_AT)
        );
    }

    private String allTimeoutEnvelopeJson() {
        return envelope(
                "COLLECTED_WITH_LOSS",
                "PARTIAL",
                "4",
                """
                {
                  "schemaVersion":1,
                  "target":"127.0.0.1",
                  "ipVersion":4,
                  "source":"SENSOR_PING_SOCKET",
                  "startedAt":"%s",
                  "completedAt":"%s",
                  "requestedSamples":2,
                  "received":0,
                  "packetLossPct":100.0,
                  "samples":[
                    {"sequence":1,"success":false,"errorCode":"ICMP_TIMEOUT"},
                    {"sequence":2,"success":false,"errorCode":"ICMP_TIMEOUT"}
                  ]
                }
                """.formatted(STARTED_AT, COLLECTED_AT)
        );
    }

    private String collectionFailedEnvelopeJson() {
        return envelope(
                "COLLECTION_FAILED",
                "UNKNOWN",
                "4",
                """
                {
                  "schemaVersion":1,
                  "target":"127.0.0.1",
                  "ipVersion":4,
                  "source":"SENSOR_PING_SOCKET",
                  "startedAt":"%s",
                  "completedAt":"%s",
                  "received":0,
                  "errorCode":"ICMP_PERMISSION_DENIED"
                }
                """.formatted(STARTED_AT, COLLECTED_AT)
        );
    }

    private String envelope(String status, String quality, String protocolVersion, String evidenceJson) {
        // Match characterization / PR #217 envelope field set and semantics.
        return """
                {
                  "schemaVersion":1,
                  "batchId":"icmp-aabbccddeeff00112233445566778899",
                  "recordType":"icmp_observation_v1",
                  "collectedAt":"%s",
                  "sensorId":"sensor-net-01",
                  "tenantId":"%s",
                  "entityId":"%s",
                  "job":{"jobId":"icmp-job-1","assignmentKey":"icmp-assign-1","assignmentVersion":1,"attempt":1},
                  "protocol":"ICMP",
                  "protocolVersion":"%s",
                  "operation":"ECHO",
                  "status":"%s",
                  "quality":"%s",
                  "protocolSchemaVersion":1,
                  "protocolPayload":%s
                }
                """.formatted(COLLECTED_AT, TENANT_ID, ENTITY_ID, protocolVersion, status, quality, evidenceJson);
    }
}
