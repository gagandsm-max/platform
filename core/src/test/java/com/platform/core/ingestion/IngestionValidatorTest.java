package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.authority.FailClosedSensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.authority.TestSensorAuthorityResolver;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;

import javax.security.auth.x500.X500Principal;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.cert.X509Certificate;
import java.util.List;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class IngestionValidatorTest {

    private IngestionValidator validator;
    private ObjectMapper objectMapper;
    private IngestionMetrics metrics;
    private TestSensorAuthorityResolver testAuthorityResolver;

    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private final UUID entityId = UUID.fromString("018f0e0a-0000-7000-8000-000000000011");
    private final SensorScope testScope = new SensorScope("sensor-net-01", tenantId, null, Set.of(entityId), false);

    @BeforeEach
    void setUp() {
        objectMapper = new ObjectMapper().findAndRegisterModules();
        metrics = new IngestionMetrics();
        testAuthorityResolver = new TestSensorAuthorityResolver();
        testAuthorityResolver.registerScope("sensor-net-01.sensor.platform.local", testScope);
        testAuthorityResolver.registerScope("sensor-net-01", testScope);
        validator = new IngestionValidator(objectMapper, testAuthorityResolver, metrics);
    }

    private X509Certificate createMockCert(String cn) {
        X509Certificate cert = mock(X509Certificate.class);
        when(cert.getSubjectX500Principal()).thenReturn(new X500Principal("CN=" + cn + ",OU=Sensors,O=Platform"));
        return cert;
    }

    @Test
    void testFailClosedAuthorityResolverRejectsWhenAuthorityUnavailable() {
        IngestionValidator failClosedValidator = new IngestionValidator(
                objectMapper, new FailClosedSensorAuthorityResolver(), metrics
        );
        X509Certificate cert = createMockCert("sensor-net-01");
        IngestionException ex = assertThrows(IngestionException.class, () ->
                failClosedValidator.resolveAuthority(cert));
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
        assertEquals("ERR_SENSOR_AUTHORITY_UNAVAILABLE", ex.getCode());
    }

    @Test
    void testClientCertificateRequiredForAuthority() {
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.resolveAuthority(null));
        assertEquals(HttpStatus.UNAUTHORIZED, ex.getStatus());
        assertEquals("ERR_AUTH_CLIENT_CERT_REQUIRED", ex.getCode());
    }

    @Test
    void testSensorIdentityMismatchRejected() {
        X509Certificate cert = createMockCert("sensor-net-01");
        // Payload claims sensor-net-02, but resolved scope is sensor-net-01
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-02", "epoch-1", "batch-1", 1, 1, "sum", List.of());
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
        assertEquals("ERR_AUTH_IDENTITY_MISMATCH", ex.getCode());
    }

    @Test
    void testPayloadTenantCannotOverrideResolvedTenant() throws Exception {
        UUID otherTenant = UUID.fromString("018f0e0a-0000-7000-8000-000000000099");
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + otherTenant + "\"," +
                "\"entityId\":\"" + entityId + "\"," +
                "\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\"," +
                "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":12345}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
        assertEquals("ERR_TENANT_SCOPE_MISMATCH", ex.getCode());
    }

    @Test
    void testPayloadEntityOutsideResolvedAuthorityRejected() throws Exception {
        UUID unauthorizedEntity = UUID.fromString("018f0e0a-0000-7000-8000-000000000088");
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\"," +
                "\"entityId\":\"" + unauthorizedEntity + "\"," +
                "\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\"," +
                "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":12345}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
        assertEquals("ERR_ENTITY_SCOPE_UNAUTHORIZED", ex.getCode());
    }

    @Test
    void testMissingCollectedAtRejectedWithoutCurrentTimeFabrication() throws Exception {
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\"," +
                "\"entityId\":\"" + entityId + "\"," +
                "\"protocol\":\"SNMP\"," +
                "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":12345}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_INVALID_TIMESTAMP", ex.getCode());
    }

    @Test
    void testCounter64PrecisionExceededRejected() throws Exception {
        // 2^64 - 1 exceeds float64 exact representable precision (2^53 - 1)
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\"," +
                "\"entityId\":\"" + entityId + "\"," +
                "\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\"," +
                "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.31.1.1.1.10.1\",\"status\":\"VALUE\",\"value\":{\"counter64\":\"18446744073709551615\"}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_NUMERIC_PRECISION_EXCEEDED", ex.getCode());
    }

    @Test
    void testZeroCanonicalObservationsRejectedNoAck() throws Exception {
        // String observation has no numeric observations
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\"," +
                "\"entityId\":\"" + entityId + "\"," +
                "\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\"," +
                "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.1.0\",\"status\":\"VALUE\",\"value\":{\"text\":\"Linux Router\"}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_NON_NUMERIC_RECORD", ex.getCode());
    }

    @Test
    void testValidSnmpNumericBatchExtractsCanonicalObservation() throws Exception {
        String recordJson = "{\"sequence\":105,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\"," +
                "\"entityId\":\"" + entityId + "\"," +
                "\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\"," +
                "\"quality\":\"GOOD\"," +
                "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":1234567}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":105,\"lastSequence\":105,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        List<CanonicalObservation> obs = validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope);

        assertNotNull(obs);
        assertEquals(1, obs.size());
        CanonicalObservation first = obs.get(0);
        assertEquals(tenantId.toString(), first.tenantId());
        assertEquals(entityId.toString(), first.entityId());
        assertEquals(".1.3.6.1.2.1.1.3.0", first.metricId());
        assertEquals("sensor-net-01", first.sensorId());
        assertEquals(1234567.0, first.value());
        assertEquals(1, first.quality());
        String expectedSequenceId = tenantId + ":sensor-net-01:epoch-1:105:0:.1.3.6.1.2.1.1.3.0";
        assertEquals(expectedSequenceId, first.sequenceId());
        assertEquals("snmp", first.sourceProtocol());
    }

    @Test
    void testSequenceContinuityDiscontinuityRejected() throws Exception {
        // Range 1..3, but only sequences 1 and 3 are present (count = 2 != 3)
        String rec1 = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":1}}]}}}";
        String rec3 = "{\"sequence\":3,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":3}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        digest.update(rec1.getBytes(StandardCharsets.UTF_8));
        digest.update(rec3.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : digest.digest()) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":3,\"checksum\":\""
                + sb + "\",\"records\":[" + rec1 + "," + rec3 + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_SEQUENCE_DISCONTINUITY", ex.getCode());
    }

    @Test
    void testSequenceContinuityDuplicateRejected() throws Exception {
        // Range 1..2, but sequences are [1, 1]
        String rec1 = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":1}}]}}}";
        String rec1dup = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":2}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        digest.update(rec1.getBytes(StandardCharsets.UTF_8));
        digest.update(rec1dup.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : digest.digest()) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":2,\"checksum\":\""
                + sb + "\",\"records\":[" + rec1 + "," + rec1dup + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_DUPLICATE_SEQUENCE", ex.getCode());
    }

    @Test
    void testSequenceContinuityOutOfBoundsRejected() throws Exception {
        // Range 1..2, but sequences are [1, 99]
        String rec1 = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":1}}]}}}";
        String rec99 = "{\"sequence\":99,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":99}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        digest.update(rec1.getBytes(StandardCharsets.UTF_8));
        digest.update(rec99.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : digest.digest()) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":2,\"checksum\":\""
                + sb + "\",\"records\":[" + rec1 + "," + rec99 + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_SEQUENCE_OUT_OF_BOUNDS", ex.getCode());
    }

    @Test
    void testOutOfOrderRecordsSortedAscendingDeterministically() throws Exception {
        // Range 1..2, records provided in reverse order [2, 1]
        String rec1 = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":10}}]}}}";
        String rec2 = "{\"sequence\":2,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:01Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":20}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        digest.update(rec2.getBytes(StandardCharsets.UTF_8));
        digest.update(rec1.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : digest.digest()) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":2,\"checksum\":\""
                + sb + "\",\"records\":[" + rec2 + "," + rec1 + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        List<CanonicalObservation> obs = validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope);
        assertEquals(2, obs.size());
        assertEquals(10.0, obs.get(0).value());
        assertEquals(20.0, obs.get(1).value());
    }

    @Test
    void testUnsupportedRecordTypeRejectedWithoutAck() throws Exception {
        // Non-numeric record type (e.g. STATE)
        String recordJson = "{\"sequence\":1,\"type\":\"STATE\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"status\":\"UP\"}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_UNSUPPORTED_RECORD_TYPE", ex.getCode());
    }

    @Test
    void testUnsupportedProtocolRejected() throws Exception {
        // Non-SNMP protocol (e.g. GNMI)
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"GNMI\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":1}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_UNSUPPORTED_PROTOCOL", ex.getCode());
    }

    @Test
    void testMissingOidRejectedWithoutFallback() throws Exception {
        // Missing OID (empty/blank)
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[{\"oid\":\"\",\"status\":\"VALUE\",\"value\":{\"unsigned\":1}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_MISSING_METRIC_ID", ex.getCode());
    }

    @Test
    void testRecordExceedingMaxRecordBytesRejected() throws Exception {
        // Create an oversized record payload (> 512 KB)
        StringBuilder hugePad = new StringBuilder();
        for (int i = 0; i < 600 * 1024; i++) hugePad.append("A");

        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"padding\":\"" + hugePad + "\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":1}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.PAYLOAD_TOO_LARGE, ex.getStatus());
        assertEquals("ERR_RECORD_TOO_LARGE", ex.getCode());
    }

    @Test
    void testSpringContextBeanInstantiationWithFailClosedResolver() {
        org.springframework.context.annotation.AnnotationConfigApplicationContext context =
                new org.springframework.context.annotation.AnnotationConfigApplicationContext();
        context.getDefaultListableBeanFactory().registerSingleton("objectMapper", objectMapper);
        context.getDefaultListableBeanFactory().registerSingleton("ingestionMetrics", metrics);
        context.register(com.platform.core.ingestion.authority.SensorAuthorityConfiguration.class);
        context.register(IngestionValidator.class);
        assertDoesNotThrow(context::refresh);
        assertNotNull(context.getBean(IngestionValidator.class));
        assertNotNull(context.getBean(com.platform.core.ingestion.authority.SensorAuthorityResolver.class));
        context.close();
    }

    // =========================================================================
    // MANDATORY P0 OBSERVATION ACCOUNTING TESTS
    // =========================================================================

    @Test
    void test1_UnsupportedOnlySnmpSourceRecordRejected() throws Exception {
        // 1. unsupported-only SNMP source record -> REJECT
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.1.0\",\"status\":\"VALUE\",\"value\":{\"text\":\"Unsupported Router\"}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_NON_NUMERIC_RECORD", ex.getCode());
    }

    @Test
    void test2_BatchWithValidNumericAndUnsupportedOnlySourceRecordsRejected() throws Exception {
        // 2. batch with:
        //    source record A = valid numeric
        //    source record B = unsupported-only
        //    -> ENTIRE BATCH REJECT
        String recA = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":12345}}]}}}";
        String recB = "{\"sequence\":2,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.1.0\",\"status\":\"VALUE\",\"value\":{\"text\":\"String Only\"}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        digest.update(recA.getBytes(StandardCharsets.UTF_8));
        digest.update(recB.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : digest.digest()) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":2,\"checksum\":\""
                + sb + "\",\"records\":[" + recA + "," + recB + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_NON_NUMERIC_RECORD", ex.getCode());
    }

    @Test
    void test3_SingleSourceRecordWithMixedValidNumericAndTextUnsupportedRejected() throws Exception {
        // 3. ONE source record containing:
        //    observation A = valid numeric VALUE
        //    observation B = text/string unsupported value
        //    -> REJECT
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[" +
                "{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":12345}}," +
                "{\"oid\":\".1.3.6.1.2.1.1.1.0\",\"status\":\"VALUE\",\"value\":{\"text\":\"Linux Router\"}}" +
                "]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_NON_NUMERIC_RECORD", ex.getCode());
    }

    @Test
    void test4_SingleSourceRecordWithMixedValidNumericAndSnmpExceptionRejected() throws Exception {
        // 4. ONE source record containing:
        //    observation A = valid numeric VALUE
        //    observation B = SNMP exception / non-VALUE
        //    -> REJECT
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[" +
                "{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":12345}}," +
                "{\"oid\":\".1.3.6.1.2.1.1.5.0\",\"status\":\"EXCEPTION\",\"value\":{\"kind\":\"EXCEPTION\"}}" +
                "]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_UNSUPPORTED_RECORD_TYPE", ex.getCode());
    }

    @Test
    void test5_SingleSourceRecordWithMixedValidNumericAndUnsupportedValueKindRejected() throws Exception {
        // 5. ONE source record containing:
        //    observation A = valid numeric VALUE
        //    observation B = unsupported value kind (e.g. IP_ADDRESS / OPAQUE)
        //    -> REJECT
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"protocolPayload\":{\"observations\":[" +
                "{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":12345}}," +
                "{\"oid\":\".1.3.6.1.2.1.1.4.0\",\"status\":\"VALUE\",\"value\":{\"kind\":\"IP_ADDRESS\",\"ip\":\"192.168.1.1\"}}" +
                "]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_UNSUPPORTED_RECORD_TYPE", ex.getCode());
    }

    @Test
    void test6_SingleSourceRecordWithOnlyFullySupportedNumericObservationAccepted() throws Exception {
        // 6. source record containing only fully supported numeric SNMP observations -> ACCEPT
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"quality\":\"GOOD\"," +
                "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":12345}}]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        List<CanonicalObservation> obs = validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope);
        assertNotNull(obs);
        assertEquals(1, obs.size());
        assertEquals(12345.0, obs.get(0).value());
        assertEquals(".1.3.6.1.2.1.1.3.0", obs.get(0).metricId());
    }

    @Test
    void test7_MultipleSupportedNumericObservationsInOneSourceRecordAccepted() throws Exception {
        // 7. multiple supported numeric observations inside one source record
        //    -> ACCEPT and produce all expected canonical observations
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\",\"entityId\":\"" + entityId + "\",\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\",\"quality\":\"GOOD\"," +
                "\"protocolPayload\":{\"observations\":[" +
                "{\"oid\":\".1.3.6.1.2.1.1.3.0\",\"status\":\"VALUE\",\"value\":{\"unsigned\":12345}}," +
                "{\"oid\":\".1.3.6.1.2.1.2.2.1.10.1\",\"status\":\"VALUE\",\"value\":{\"integer\":42}}," +
                "{\"oid\":\".1.3.6.1.2.1.31.1.1.1.10.1\",\"status\":\"VALUE\",\"value\":{\"counter64\":\"99999999\"}}" +
                "]}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        List<CanonicalObservation> obs = validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope);
        assertNotNull(obs);
        assertEquals(3, obs.size());

        assertEquals(".1.3.6.1.2.1.1.3.0", obs.get(0).metricId());
        assertEquals(12345.0, obs.get(0).value());
        assertTrue(obs.get(0).sequenceId().endsWith(":0:.1.3.6.1.2.1.1.3.0"));

        assertEquals(".1.3.6.1.2.1.2.2.1.10.1", obs.get(1).metricId());
        assertEquals(42.0, obs.get(1).value());
        assertTrue(obs.get(1).sequenceId().endsWith(":1:.1.3.6.1.2.1.2.2.1.10.1"));

        assertEquals(".1.3.6.1.2.1.31.1.1.1.10.1", obs.get(2).metricId());
        assertEquals(99999999.0, obs.get(2).value());
        assertTrue(obs.get(2).sequenceId().endsWith(":2:.1.3.6.1.2.1.31.1.1.1.10.1"));
    }
}
