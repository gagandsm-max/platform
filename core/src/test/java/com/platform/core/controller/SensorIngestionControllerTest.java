package com.platform.core.controller;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.SensorIngestionService;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.mock.web.MockHttpServletRequest;

import javax.security.auth.x500.X500Principal;
import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.security.cert.X509Certificate;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

class SensorIngestionControllerTest {

    private SensorIngestionService ingestionService;
    private ObjectMapper objectMapper;
    private SensorIngestionController controller;

    @BeforeEach
    void setUp() {
        ingestionService = mock(SensorIngestionService.class);
        objectMapper = new ObjectMapper().findAndRegisterModules();
        controller = new SensorIngestionController(ingestionService, objectMapper);
    }

    private X509Certificate createMockCert(String cn) {
        X509Certificate cert = mock(X509Certificate.class);
        when(cert.getSubjectX500Principal()).thenReturn(new X500Principal("CN=" + cn + ",OU=Sensors,O=Platform"));
        when(cert.getSerialNumber()).thenReturn(BigInteger.valueOf(0x1234));
        return cert;
    }

    @Test
    void testConstructorRequiresNonNullService() {
        assertThrows(NullPointerException.class, () -> new SensorIngestionController(null, objectMapper));
    }

    @Test
    void testMissingClientCertReturns401() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        ResponseEntity<?> resp = controller.ingestTelemetry(null, null, "{}".getBytes(StandardCharsets.UTF_8), request);
        assertEquals(HttpStatus.UNAUTHORIZED, resp.getStatusCode());
        assertTrue(resp.getBody() instanceof Map);
        assertEquals("ERR_AUTH_CLIENT_CERT_REQUIRED", ((Map<?, ?>) resp.getBody()).get("code"));
    }

    @Test
    void testEmptyBodyReturns400() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute("jakarta.servlet.request.X509Certificate", new X509Certificate[]{createMockCert("sensor-net-01")});

        ResponseEntity<?> resp = controller.ingestTelemetry(null, null, new byte[0], request);
        assertEquals(HttpStatus.BAD_REQUEST, resp.getStatusCode());
        assertEquals("ERR_EMPTY_BODY", ((Map<?, ?>) resp.getBody()).get("code"));
    }

    @Test
    void testMalformedJsonReturns400() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute("jakarta.servlet.request.X509Certificate", new X509Certificate[]{createMockCert("sensor-net-01")});

        ResponseEntity<?> resp = controller.ingestTelemetry(null, null, "{not-json}".getBytes(StandardCharsets.UTF_8), request);
        assertEquals(HttpStatus.BAD_REQUEST, resp.getStatusCode());
        assertEquals("ERR_MALFORMED_JSON", ((Map<?, ?>) resp.getBody()).get("code"));
    }

    @Test
    void testIngestionServiceRejectionPropagatesCodeAndStatus() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute("jakarta.servlet.request.X509Certificate", new X509Certificate[]{createMockCert("sensor-net-01")});

        TelemetryBatchRequest req = new TelemetryBatchRequest(1, "sensor-net-01", "epoch-1", "b-1", 1, 1, "sum", List.of());
        byte[] body = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\"sum\",\"records\":[]}".getBytes(StandardCharsets.UTF_8);

        when(ingestionService.ingestTelemetry(any(), any(), any(), any()))
                .thenThrow(new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_AUTHORITY_UNAVAILABLE", "Authority unavailable"));

        ResponseEntity<?> resp = controller.ingestTelemetry("key-1", "corr-1", body, request);
        assertEquals(HttpStatus.FORBIDDEN, resp.getStatusCode());
        assertEquals("ERR_SENSOR_AUTHORITY_UNAVAILABLE", ((Map<?, ?>) resp.getBody()).get("code"));
    }

    @Test
    void testUnexpectedInternalErrorDoesNotLeakBackendText() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute("jakarta.servlet.request.X509Certificate", new X509Certificate[]{createMockCert("sensor-net-01")});

        byte[] body = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":1,\"checksum\":\"sum\",\"records\":[]}".getBytes(StandardCharsets.UTF_8);

        when(ingestionService.ingestTelemetry(any(), any(), any(), any()))
                .thenThrow(new RuntimeException("Postgres connection timeout at 10.0.0.1:5432 with password secret"));

        ResponseEntity<?> resp = controller.ingestTelemetry("key-1", "corr-1", body, request);
        assertEquals(HttpStatus.INTERNAL_SERVER_ERROR, resp.getStatusCode());
        Map<?, ?> errorBody = (Map<?, ?>) resp.getBody();
        assertEquals("ERR_INTERNAL_ERROR", errorBody.get("code"));
        assertEquals("Internal server error processing telemetry", errorBody.get("message"));
        assertFalse(errorBody.get("message").toString().contains("Postgres"));
        assertFalse(errorBody.get("message").toString().contains("secret"));
    }

    @Test
    void testSuccessfulIngestionReturnsAcceptedAck() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute("jakarta.servlet.request.X509Certificate", new X509Certificate[]{createMockCert("sensor-net-01")});

        byte[] body = "{\"schemaVersion\":1,\"sensorId\":\"sensor-net-01\",\"bootEpoch\":\"epoch-1\",\"batchId\":\"b-1\",\"firstSequence\":1,\"lastSequence\":100,\"checksum\":\"sum\",\"records\":[]}".getBytes(StandardCharsets.UTF_8);

        when(ingestionService.ingestTelemetry(any(), any(), any(), any()))
                .thenReturn(new TelemetryAckResponse("ACCEPTED", 100L, "sensor-net-01"));

        ResponseEntity<?> resp = controller.ingestTelemetry("key-1", "corr-1", body, request);
        assertEquals(HttpStatus.OK, resp.getStatusCode());
        assertTrue(resp.getBody() instanceof TelemetryAckResponse);
        TelemetryAckResponse ack = (TelemetryAckResponse) resp.getBody();
        assertEquals("ACCEPTED", ack.status());
        assertEquals(100L, ack.acceptedThroughSequence());
    }
}
