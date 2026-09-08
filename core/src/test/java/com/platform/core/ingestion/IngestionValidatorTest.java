package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
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

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class IngestionValidatorTest {

    private IngestionValidator validator;
    private ObjectMapper objectMapper;
    private IngestionMetrics metrics;

    @BeforeEach
    void setUp() {
        objectMapper = new ObjectMapper().findAndRegisterModules();
        metrics = new IngestionMetrics();
        validator = new IngestionValidator(objectMapper, metrics);
    }

    private X509Certificate createMockCert(String cn) {
        X509Certificate cert = mock(X509Certificate.class);
        when(cert.getSubjectX500Principal()).thenReturn(new X500Principal("CN=" + cn + ",OU=Sensors,O=Platform"));
        return cert;
    }

    @Test
    void testClientCertificateRequired() {
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "batch-1", 1, 1, "sum", List.of());
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(StandardCharsets.UTF_8), req, null));
        assertEquals(HttpStatus.UNAUTHORIZED, ex.getStatus());
        assertEquals("ERR_AUTH_CLIENT_CERT_REQUIRED", ex.getCode());
    }

    @Test
    void testSensorIdentityMismatchRejected() {
        X509Certificate cert = createMockCert("sensor-net-02.sensor.platform.local");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "batch-1", 1, 1, "sum", List.of());
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(StandardCharsets.UTF_8), req, cert));
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
        assertEquals("ERR_AUTH_IDENTITY_MISMATCH", ex.getCode());
    }

    @Test
    void testInvalidSequenceBoundsRejected() {
        X509Certificate cert = createMockCert("sensor-net-01.sensor.platform.local");
        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "batch-1", 10, 5, "sum", List.of());
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract("{}".getBytes(StandardCharsets.UTF_8), req, cert));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_BATCH_BOUNDS_INVALID", ex.getCode());
    }

    @Test
    void testChecksumMismatchRejected() throws Exception {
        X509Certificate cert = createMockCert("sensor-net-01.sensor.platform.local");
        String recordJson = "{\"sequence\":1,\"payload\":{\"metric_id\":\"sensor.heartbeat\",\"value\":1.0}}";
        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\"invalid_sum\",\"records\":[" + recordJson + "]}";

        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);
        IngestionException ex = assertThrows(IngestionException.class, () ->
                validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, cert));
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        assertEquals("ERR_CHECKSUM_MISMATCH", ex.getCode());
    }

    @Test
    void testValidBatchExtractsCanonicalObservations() throws Exception {
        X509Certificate cert = createMockCert("sensor-net-01.sensor.platform.local");
        String recordJson = "{\"sequence\":1,\"type\":\"TELEMETRY_SAMPLE\",\"payload\":{\"tenantId\":\"default\",\"entityId\":\"sensor-net-01\",\"protocol\":\"SENSOR\",\"protocolPayload\":{\"metric_id\":\"sensor.heartbeat\",\"value\":1.0}}}";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] recordBytes = recordJson.getBytes(StandardCharsets.UTF_8);
        byte[] hash = digest.digest(recordBytes);
        StringBuilder sb = new StringBuilder();
        for (byte b : hash) sb.append(String.format("%02x", b));
        String expectedChecksum = sb.toString();

        String json = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\"" + expectedChecksum + "\",\"records\":[" + recordJson + "]}";

        TelemetryBatchRequest req = objectMapper.readValue(json, TelemetryBatchRequest.class);
        List<CanonicalObservation> obs = validator.validateAndExtract(json.getBytes(StandardCharsets.UTF_8), req, cert);

        assertNotNull(obs);
        assertEquals(1, obs.size());
        CanonicalObservation first = obs.get(0);
        assertEquals("default", first.tenantId());
        assertEquals("sensor-net-01", first.entityId());
        assertEquals("sensor.heartbeat", first.metricId());
        assertEquals(1.0, first.value());
        assertEquals("1", first.sequenceId());
    }
}
