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
        assertEquals("ERR_NO_CANONICAL_OBSERVATIONS", ex.getCode());
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
        assertEquals("105", first.sequenceId());
        assertEquals("snmp", first.sourceProtocol());
    }

    @Test
    void testSpringContextBeanInstantiationWithFailClosedResolver() {
        org.springframework.context.annotation.AnnotationConfigApplicationContext context =
                new org.springframework.context.annotation.AnnotationConfigApplicationContext();
        context.getDefaultListableBeanFactory().registerSingleton("objectMapper", objectMapper);
        context.getDefaultListableBeanFactory().registerSingleton("ingestionMetrics", metrics);
        context.register(FailClosedSensorAuthorityResolver.class);
        context.register(IngestionValidator.class);
        assertDoesNotThrow(context::refresh);
        assertNotNull(context.getBean(IngestionValidator.class));
        assertNotNull(context.getBean(com.platform.core.ingestion.authority.SensorAuthorityResolver.class));
        context.close();
    }
}
