package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.authority.FailClosedSensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.authority.TestSensorAuthorityResolver;
import com.platform.core.ingestion.dto.CanonicalCounterObservation;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryObservationBatch;
import java.math.BigInteger;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;

import javax.security.auth.x500.X500Principal;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.cert.X509Certificate;
import java.util.Base64;
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
    void testSensorIdentityCaseMismatchRejected() {
        SensorScope caseScope = new SensorScope("sensor-a", tenantId, null, Set.of(entityId), false);
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "SENSOR-A", "epoch-1", "batch-1", 1, 1, "sum", List.of());
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(StandardCharsets.UTF_8), req, caseScope));
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
    void testCounter64ExactMaxPreserved() throws Exception {
        // Full UInt64 max must be preserved exactly as Counter64.
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

        TelemetryObservationBatch batch = validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope);
        assertEquals(0, batch.numericObservations().size());
        assertEquals(1, batch.counterObservations().size());
        assertEquals("18446744073709551615", batch.counterObservations().get(0).counterValue());
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

        TelemetryObservationBatch obsBatch = validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope);

        assertNotNull(obsBatch);
        assertEquals(1, obsBatch.numericObservations().size());
        CanonicalObservation first = obsBatch.numericObservations().get(0);
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
    void testLogicalObservationIdentityUsesDurableSpoolGeneration() throws Exception {
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

        String streamA = "stream-00112233445566778899aabbccddeeff";
        String streamB = "stream-ffeeddccbbaa99887766554433221100";

        String firstJson = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"" + streamA +
                "\",\"batchId\":\"b-1\",\"firstSequence\":105,\"lastSequence\":105,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        String processRestartRetryJson = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"" + streamA +
                "\",\"batchId\":\"b-2\",\"firstSequence\":105,\"lastSequence\":105,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        String recreatedSpoolJson = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"" + streamB +
                "\",\"batchId\":\"b-3\",\"firstSequence\":105,\"lastSequence\":105,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";

        TelemetryBatchRequest firstRequest = objectMapper.readValue(firstJson, TelemetryBatchRequest.class);
        TelemetryBatchRequest restartRequest = objectMapper.readValue(processRestartRetryJson, TelemetryBatchRequest.class);
        TelemetryBatchRequest recreatedRequest = objectMapper.readValue(recreatedSpoolJson, TelemetryBatchRequest.class);

        TelemetryObservationBatch first = validator.validateAndExtract(
                firstJson.getBytes(StandardCharsets.UTF_8), firstRequest, testScope);
        TelemetryObservationBatch retry = validator.validateAndExtract(
                processRestartRetryJson.getBytes(StandardCharsets.UTF_8), restartRequest, testScope);
        TelemetryObservationBatch recreated = validator.validateAndExtract(
                recreatedSpoolJson.getBytes(StandardCharsets.UTF_8), recreatedRequest, testScope);

        assertEquals(first.numericObservations().get(0).sequenceId(), retry.numericObservations().get(0).sequenceId(),
                "Sensor process restart must retain the durable spool generation identity");
        assertNotEquals(first.numericObservations().get(0).sequenceId(), recreated.numericObservations().get(0).sequenceId(),
                "a recreated spool generation must not collide with historical sequence identity");
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

        TelemetryObservationBatch obs = validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope);
        assertEquals(2, obs.numericObservations().size());
        assertEquals(10.0, obs.numericObservations().get(0).value());
        assertEquals(20.0, obs.numericObservations().get(1).value());
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

        TelemetryObservationBatch obs = validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope);
        assertNotNull(obs);
        assertEquals(1, obs.numericObservations().size());
        assertEquals(12345.0, obs.numericObservations().get(0).value());
        assertEquals(".1.3.6.1.2.1.1.3.0", obs.numericObservations().get(0).metricId());
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

        TelemetryObservationBatch obs = validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope);
        assertNotNull(obs);
        assertEquals(2, obs.numericObservations().size());
        assertEquals(1, obs.counterObservations().size());
        assertEquals(3, obs.size());

        assertEquals(".1.3.6.1.2.1.1.3.0", obs.numericObservations().get(0).metricId());
        assertEquals(12345.0, obs.numericObservations().get(0).value());
        assertTrue(obs.numericObservations().get(0).sequenceId().endsWith(":0:.1.3.6.1.2.1.1.3.0"));

        assertEquals(".1.3.6.1.2.1.2.2.1.10.1", obs.numericObservations().get(1).metricId());
        assertEquals(42.0, obs.numericObservations().get(1).value());
        assertTrue(obs.numericObservations().get(1).sequenceId().endsWith(":1:.1.3.6.1.2.1.2.2.1.10.1"));

        assertEquals(".1.3.6.1.2.1.31.1.1.1.10.1", obs.counterObservations().get(0).metricId());
        assertEquals("99999999", obs.counterObservations().get(0).counterValue());
        assertTrue(obs.counterObservations().get(0).sequenceId().endsWith(":2:.1.3.6.1.2.1.31.1.1.1.10.1"));
    }

    @Test
    void testCounter64ValidBoundaryValuesExact() throws Exception {
        String[] values = {
                "0", "1", "9007199254740991", "9007199254740992", "9007199254740993",
                "9223372036854775807", "9223372036854775808", "18446744073709551614", "18446744073709551615"
        };
        for (String value : values) {
            String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                    "\"tenantId\":\"" + tenantId + "\"," +
                    "\"entityId\":\"" + entityId + "\"," +
                    "\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                    "\"protocol\":\"SNMP\"," +
                    "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.31.1.1.1.6.1\",\"status\":\"VALUE\",\"value\":{\"counter64\":\"" + value + "\"}}]}}}";
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
            StringBuilder sb = new StringBuilder();
            for (byte b : hash) sb.append(String.format("%02x", b));
            String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-" + value + "\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                    + sb + "\",\"records\":[" + recordJson + "]}";
            TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);
            TelemetryObservationBatch batch = validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope);
            assertEquals(1, batch.counterObservations().size(), "value=" + value);
            assertEquals(value, batch.counterObservations().get(0).counterValue());
            assertEquals(new BigInteger(value), batch.counterObservations().get(0).exactValue());
        }
    }

    @Test
    void testCounter64InvalidBoundariesRejected() throws Exception {
        String[] invalid = {"-1", "+1", " 1", "1 ", "18446744073709551616", "abc", "1.5", "1e19", ""};
        for (String value : invalid) {
            String counterJson = value.isEmpty() ? "\"\"" : "\"" + value + "\"";
            String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                    "\"tenantId\":\"" + tenantId + "\"," +
                    "\"entityId\":\"" + entityId + "\"," +
                    "\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                    "\"protocol\":\"SNMP\"," +
                    "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.31.1.1.1.6.1\",\"status\":\"VALUE\",\"value\":{\"counter64\":" + counterJson + "}}]}}}";
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
            StringBuilder sb = new StringBuilder();
            for (byte b : hash) sb.append(String.format("%02x", b));
            String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-bad\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                    + sb + "\",\"records\":[" + recordJson + "]}";
            TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);
            IngestionException ex = assertThrows(IngestionException.class, () ->
                    validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope),
                    "expected reject for " + value);
            assertEquals("ERR_MALFORMED_COUNTER64", ex.getCode(), "value=" + value);
        }
    }

    @Test
    void testCounter64KindMismatchRejected() throws Exception {
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\"," +
                "\"entityId\":\"" + entityId + "\"," +
                "\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\"," +
                "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.31.1.1.1.6.1\",\"status\":\"VALUE\",\"value\":{\"kind\":\"INTEGER\",\"counter64\":\"9007199254740993\"}}]}}}";
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));
        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-kind\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals("ERR_VALUE_KIND_MISMATCH", ex.getCode());
    }

    @Test
    void testCounter64MultipleNumericFieldsRejected() throws Exception {
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\"," +
                "\"entityId\":\"" + entityId + "\"," +
                "\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\"," +
                "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.31.1.1.1.6.1\",\"status\":\"VALUE\",\"value\":{\"kind\":\"COUNTER64\",\"integer\":1,\"counter64\":\"2\"}}]}}}";
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));
        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-ambiguous\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals("ERR_AMBIGUOUS_NUMERIC_VALUE", ex.getCode());
    }

    @Test
    void testCounter64JsonNumberRejected() throws Exception {
        String recordJson = "{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{" +
                "\"tenantId\":\"" + tenantId + "\"," +
                "\"entityId\":\"" + entityId + "\"," +
                "\"collectedAt\":\"2026-09-09T10:00:00Z\"," +
                "\"protocol\":\"SNMP\"," +
                "\"protocolPayload\":{\"observations\":[{\"oid\":\".1.3.6.1.2.1.31.1.1.1.6.1\",\"status\":\"VALUE\",\"value\":{\"counter64\":18446744073709551615}}]}}}";
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(recordJson.getBytes(StandardCharsets.UTF_8));
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));
        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-num\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + sb + "\",\"records\":[" + recordJson + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals("ERR_COUNTER64_JSON_NUMBER", ex.getCode());
    }

    // =========================================================================
    // ISSUE #218 — BUILD-020 icmp_observation_v1 CHARACTERIZATION (numeric-only Core)
    //
    // CURRENT IMPLEMENTATION CHARACTERIZATION ONLY — not intended permanent
    // product behavior. These tests document CURRENT rejection of source-faithful
    // ICMP structured evidence produced by icmpintegration.Executor
    // (RecordType=icmp_observation_v1; Evidence has target/ipVersion/received/
    // packetLossPct/RTT/samples/errorCode — NO observations[] and NO metric_id+value).
    //
    // Wire fixtures mirror Go spool.Record ([]byte payload → base64) + uploader
    // makeBatch checksum over exact json.Marshal(record) bytes so Core's textual/
    // base64 decoding path is exercised. Keep helpers reusable for the future
    // typed-acceptance implementation.
    //
    // When approved typed Core ICMP ingestion lands, replace expected-rejection
    // assertions with typed-acceptance + complete-accounting tests. Do not
    // permanently freeze numeric-only behavior.
    // =========================================================================

    private static final String GO_SPOOL_CREATED_AT = "2026-10-11T14:00:00Z";

    /**
     * Source-faithful BUILD-020 observation.Envelope + Evidence protocolPayload
     * as written by icmpintegration.Executor (PR #217). RTT/loss values are
     * authentic successful-collection Evidence fields, not Core numeric facts.
     */
    private String build020IcmpObservationEnvelopePayload() {
        return "{"
                + "\"schemaVersion\":1,"
                + "\"batchId\":\"icmp-aabbccddeeff00112233445566778899\","
                + "\"recordType\":\"icmp_observation_v1\","
                + "\"collectedAt\":\"" + GO_SPOOL_CREATED_AT + "\","
                + "\"sensorId\":\"sensor-net-01\","
                + "\"tenantId\":\"" + tenantId + "\","
                + "\"entityId\":\"" + entityId + "\","
                + "\"job\":{\"jobId\":\"icmp-job-1\",\"assignmentKey\":\"icmp-assign-1\",\"assignmentVersion\":1,\"attempt\":1},"
                + "\"protocol\":\"ICMP\","
                + "\"protocolVersion\":\"4\","
                + "\"operation\":\"ECHO\","
                + "\"status\":\"COLLECTED\","
                + "\"quality\":\"GOOD\","
                + "\"protocolSchemaVersion\":1,"
                + "\"protocolPayload\":{"
                + "\"schemaVersion\":1,"
                + "\"target\":\"127.0.0.1\","
                + "\"ipVersion\":4,"
                + "\"source\":\"SENSOR_PING_SOCKET\","
                + "\"startedAt\":\"2026-10-11T13:59:59.900Z\","
                + "\"completedAt\":\"" + GO_SPOOL_CREATED_AT + "\","
                + "\"requestedSamples\":2,"
                + "\"received\":2,"
                + "\"packetLossPct\":0.0,"
                + "\"minRttMicros\":120,"
                + "\"avgRttMicros\":150,"
                + "\"maxRttMicros\":180,"
                + "\"samples\":["
                + "{\"sequence\":1,\"success\":true,\"rttMicros\":120},"
                + "{\"sequence\":2,\"success\":true,\"rttMicros\":180}"
                + "]"
                + "}"
                + "}";
    }

    /**
     * Authentic observation.Envelope carrying SNMP numeric observations[], matching
     * field order from sensor/internal/observation/envelope.go for future reuse.
     */
    private String snmpNumericObservationEnvelopePayload(long unsignedValue) {
        return "{"
                + "\"schemaVersion\":1,"
                + "\"batchId\":\"snmp-aabbccddeeff00112233445566778899\","
                + "\"recordType\":\"SNMP_OBSERVATION_BATCH\","
                + "\"collectedAt\":\"" + GO_SPOOL_CREATED_AT + "\","
                + "\"sensorId\":\"sensor-net-01\","
                + "\"tenantId\":\"" + tenantId + "\","
                + "\"entityId\":\"" + entityId + "\","
                + "\"job\":{\"jobId\":\"snmp-job-1\",\"assignmentKey\":\"snmp-assign-1\",\"assignmentVersion\":1,\"attempt\":1},"
                + "\"protocol\":\"SNMP\","
                + "\"protocolVersion\":\"2c\","
                + "\"operation\":\"GET\","
                + "\"status\":\"COLLECTED\","
                + "\"quality\":\"GOOD\","
                + "\"protocolSchemaVersion\":1,"
                + "\"protocolPayload\":{"
                + "\"schemaVersion\":1,"
                + "\"version\":\"2c\","
                + "\"operation\":\"GET\","
                + "\"collectedAt\":\"" + GO_SPOOL_CREATED_AT + "\","
                + "\"observations\":[{"
                + "\"oid\":\".1.3.6.1.2.1.1.3.0\","
                + "\"sourceOid\":\".1.3.6.1.2.1.1.3.0\","
                + "\"berType\":66,"
                + "\"berTypeName\":\"Gauge32\","
                + "\"collectedAt\":\"" + GO_SPOOL_CREATED_AT + "\","
                + "\"quality\":\"GOOD\","
                + "\"status\":\"VALUE\","
                + "\"value\":{\"kind\":\"UNSIGNED\",\"unsigned\":" + unsignedValue + "}"
                + "}]"
                + "}"
                + "}";
    }

    /**
     * Faithful Go spool.Record wire JSON (sensor/internal/spool/types.go Record +
     * Append checksum/id contract). Payload []byte serializes as base64; record
     * checksum is SHA-256 of the original unencoded payload bytes; id is
     * rec-%020d-%016x-prefix. Field order matches Go encoding/json struct order.
     */
    private String goStyleSpoolRecordJson(long sequence, String recordType, byte[] payloadBytes)
            throws Exception {
        String payloadChecksum = sha256Hex(payloadBytes);
        String recordId = String.format("rec-%020d-%s", sequence, payloadChecksum.substring(0, 16));
        String payloadB64 = Base64.getEncoder().encodeToString(payloadBytes);
        return "{"
                + "\"schemaVersion\":1,"
                + "\"sequence\":" + sequence + ","
                + "\"id\":\"" + recordId + "\","
                + "\"class\":\"METRIC\","
                + "\"type\":\"" + recordType + "\","
                + "\"createdAt\":\"" + GO_SPOOL_CREATED_AT + "\","
                + "\"contentLength\":" + payloadBytes.length + ","
                + "\"checksum\":\"" + payloadChecksum + "\","
                + "\"payload\":\"" + payloadB64 + "\""
                + "}";
    }

    private String goStyleSpoolRecordJson(long sequence, String recordType, String payloadUtf8)
            throws Exception {
        return goStyleSpoolRecordJson(sequence, recordType, payloadUtf8.getBytes(StandardCharsets.UTF_8));
    }

    /** Reusable BUILD-020 ICMP spool wire record for typed-acceptance later. */
    private String build020IcmpSpoolRecordJson(long sequence) throws Exception {
        return goStyleSpoolRecordJson(sequence, "icmp_observation_v1", build020IcmpObservationEnvelopePayload());
    }

    /** Positive SNMP numeric control using the same Go-style spool serialization. */
    private String validSnmpNumericGoStyleSpoolRecordJson(long sequence, long unsignedValue)
            throws Exception {
        return goStyleSpoolRecordJson(
                sequence, "SNMP_OBSERVATION_BATCH", snmpNumericObservationEnvelopePayload(unsignedValue));
    }

    /**
     * Negative control: same Go spool metadata shape with deliberately corrupt
     * base64 so failure is ERR_MALFORMED_RECORD_PAYLOAD, not unsupported ICMP.
     */
    private String corruptBase64GoStyleSpoolRecordJson(long sequence) throws Exception {
        byte[] authenticPayload = build020IcmpObservationEnvelopePayload().getBytes(StandardCharsets.UTF_8);
        String payloadChecksum = sha256Hex(authenticPayload);
        String recordId = String.format("rec-%020d-%s", sequence, payloadChecksum.substring(0, 16));
        // Invalid base64 alphabet — cannot decode and is not JSON text either.
        String corruptB64 = "!!!not-valid-base64!!!";
        return "{"
                + "\"schemaVersion\":1,"
                + "\"sequence\":" + sequence + ","
                + "\"id\":\"" + recordId + "\","
                + "\"class\":\"METRIC\","
                + "\"type\":\"icmp_observation_v1\","
                + "\"createdAt\":\"" + GO_SPOOL_CREATED_AT + "\","
                + "\"contentLength\":" + authenticPayload.length + ","
                + "\"checksum\":\"" + payloadChecksum + "\","
                + "\"payload\":\"" + corruptB64 + "\""
                + "}";
    }

    private String sha256Hex(byte[] bytes) throws Exception {
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] hash = digest.digest(bytes);
        StringBuilder sb = new StringBuilder(hash.length * 2);
        for (byte b : hash) {
            sb.append(String.format("%02x", b));
        }
        return sb.toString();
    }

    /** Outer upload checksum = SHA-256 over concatenated exact serialized spool record bytes. */
    private String checksumOfRecords(String... recordJsons) throws Exception {
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        for (String recordJson : recordJsons) {
            digest.update(recordJson.getBytes(StandardCharsets.UTF_8));
        }
        StringBuilder sb = new StringBuilder();
        for (byte b : digest.digest()) {
            sb.append(String.format("%02x", b));
        }
        return sb.toString();
    }

    private void assertUnsupportedIcmpCharacterization(IngestionException ex) {
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_UNSUPPORTED_RECORD_TYPE", ex.getCode(),
                "CHARACTERIZATION: authentic Go-wire ICMP must reach typed-shape rejection, not earlier decode/identity failures.");
        assertTrue(ex.getMessage().contains("unsupported telemetry payload shape"));
        assertNotEquals("ERR_MALFORMED_RECORD_PAYLOAD", ex.getCode());
        assertNotEquals("ERR_CHECKSUM_MISMATCH", ex.getCode());
        assertNotEquals("ERR_CHECKSUM_REQUIRED", ex.getCode());
        assertNotEquals("ERR_ENTITY_ID_REQUIRED", ex.getCode());
        assertNotEquals("ERR_ENTITY_SCOPE_UNAUTHORIZED", ex.getCode());
        assertNotEquals("ERR_TENANT_SCOPE_MISMATCH", ex.getCode());
        assertNotEquals("ERR_INVALID_TIMESTAMP", ex.getCode());
        assertNotEquals("ERR_MISSING_PROTOCOL", ex.getCode());
    }

    @Test
    void issue218_goStyleSnmpNumericControlAcceptedThroughRealValidator() throws Exception {
        // Positive control: authentic Go-style spool + base64 path accepts SNMP numeric.
        String snmp = validSnmpNumericGoStyleSpoolRecordJson(1, 12345);
        String checksum = checksumOfRecords(snmp);
        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\","
                + "\"batchId\":\"b-snmp-gostyle\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + checksum + "\",\"records\":[" + snmp + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        TelemetryObservationBatch batch = validator.validateAndExtract(
                json.getBytes(StandardCharsets.UTF_8), req, testScope);
        assertEquals(1, batch.numericObservations().size());
        assertEquals(12345.0, batch.numericObservations().get(0).value());
        assertEquals(".1.3.6.1.2.1.1.3.0", batch.numericObservations().get(0).metricId());
    }

    @Test
    void issue218_corruptBase64PayloadFailsDifferentlyFromUnsupportedIcmp() throws Exception {
        // Negative control: malformed base64 must not masquerade as ICMP characterization.
        String corrupt = corruptBase64GoStyleSpoolRecordJson(1);
        String checksum = checksumOfRecords(corrupt);
        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\","
                + "\"batchId\":\"b-corrupt-b64\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + checksum + "\",\"records\":[" + corrupt + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertEquals("ERR_MALFORMED_RECORD_PAYLOAD", ex.getCode(),
                "Corrupt base64 must fail at decode, proving authentic ICMP rejection is not a fixture artifact.");
        assertNotEquals("ERR_UNSUPPORTED_RECORD_TYPE", ex.getCode());
    }

    @Test
    void issue218_sourceFaithfulIcmpObservationRejectedByNumericOnlyCore() throws Exception {
        // CURRENT CHARACTERIZATION: valid Go-wire BUILD-020 icmp_observation_v1 is rejected today.
        String icmp = build020IcmpSpoolRecordJson(1);
        assertTrue(icmp.contains("\"payload\":\""), "payload must be textual/base64 on the wire");
        assertFalse(icmp.contains("\"payload\":{"), "must not embed JSON object under payload");
        String checksum = checksumOfRecords(icmp);
        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\","
                + "\"batchId\":\"b-icmp-alone\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\""
                + checksum + "\",\"records\":[" + icmp + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertUnsupportedIcmpCharacterization(ex);
    }

    @Test
    void issue218_mixedNumericSnmpThenIcmpBatchRejectedWithoutPartialSiblingPublish() throws Exception {
        // CURRENT CHARACTERIZATION: mixed SNMP + ICMP rejects the whole batch (no sibling leak).
        String snmp = validSnmpNumericGoStyleSpoolRecordJson(1, 12345);
        String icmp = build020IcmpSpoolRecordJson(2);
        String checksum = checksumOfRecords(snmp, icmp);
        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\","
                + "\"batchId\":\"b-mixed-snmp-icmp\",\"firstSequence\":1,\"lastSequence\":2,\"checksum\":\""
                + checksum + "\",\"records\":[" + snmp + "," + icmp + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertUnsupportedIcmpCharacterization(ex);
    }

    @Test
    void issue218_reverseOrderIcmpThenNumericBatchRejectedEquivalently() throws Exception {
        // CURRENT CHARACTERIZATION: reverse-order ICMP + SNMP exhibits equivalent atomic rejection.
        String icmp = build020IcmpSpoolRecordJson(1);
        String snmp = validSnmpNumericGoStyleSpoolRecordJson(2, 67890);
        String checksum = checksumOfRecords(icmp, snmp);
        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\","
                + "\"batchId\":\"b-mixed-icmp-snmp-rev\",\"firstSequence\":1,\"lastSequence\":2,\"checksum\":\""
                + checksum + "\",\"records\":[" + icmp + "," + snmp + "]}";
        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, testScope));
        assertUnsupportedIcmpCharacterization(ex);
    }
}
