package com.platform.core.controller;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.discovery.DiscoveryStatus;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.SensorIngestionService;
import com.platform.core.ingestion.authority.FailClosedSensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.authority.TestSensorAuthorityResolver;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import javax.security.auth.x500.X500Principal;
import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.security.cert.X509Certificate;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyList;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

class SensorIngestionControllerTest {

    private static final String CERT_ATTR = "jakarta.servlet.request.X509Certificate";
    private final UUID requestId = UUID.fromString("018f0e0a-0000-7000-8000-000000000321");
    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000111");

    private SensorIngestionService ingestionService;
    private ObjectMapper objectMapper;
    private DiscoveryService discoveryService;
    private TestSensorAuthorityResolver authorityResolver;
    private SensorIngestionController controller;
    private MockMvc mockMvc;
    private X509Certificate sensorACertificate;

    @BeforeEach
    void setUp() {
        ingestionService = mock(SensorIngestionService.class);
        objectMapper = new ObjectMapper().findAndRegisterModules();
        discoveryService = mock(DiscoveryService.class);
        authorityResolver = new TestSensorAuthorityResolver();
        authorityResolver.registerScope("sensor-a", new SensorScope(
                "sensor-a", tenantId, null, Set.of(), true));
        controller = new SensorIngestionController(
                ingestionService, objectMapper, discoveryService, authorityResolver);
        mockMvc = MockMvcBuilders.standaloneSetup(controller)
                .setControllerAdvice(new ApiExceptionHandler())
                .build();
        sensorACertificate = certificate("sensor-a");
    }

    private X509Certificate createMockCert(String cn) {
        X509Certificate cert = mock(X509Certificate.class);
        when(cert.getSubjectX500Principal()).thenReturn(new X500Principal("CN=" + cn + ",OU=Sensors,O=Platform"));
        when(cert.getSerialNumber()).thenReturn(BigInteger.valueOf(0x1234));
        return cert;
    }

    private static X509Certificate certificate(String commonName) {
        X509Certificate certificate = mock(X509Certificate.class);
        when(certificate.getSubjectX500Principal()).thenReturn(new X500Principal("CN=" + commonName + ",O=Platform"));
        when(certificate.getSerialNumber()).thenReturn(BigInteger.ONE);
        return certificate;
    }

    // --- Main telemetry controller tests ---

    @Test
    void testConstructorRequiresNonNullService() {
        assertThrows(NullPointerException.class, () ->
                new SensorIngestionController(null, objectMapper, discoveryService, authorityResolver));
        assertThrows(NullPointerException.class, () ->
                new SensorIngestionController(ingestionService, objectMapper, null, authorityResolver));
        assertThrows(NullPointerException.class, () ->
                new SensorIngestionController(ingestionService, objectMapper, discoveryService, null));
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
    void testBodyExceedingMaxBatchBytesReturns413() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute("jakarta.servlet.request.X509Certificate", new X509Certificate[]{createMockCert("sensor-net-01")});

        byte[] largeBody = new byte[com.platform.core.ingestion.IngestionValidator.MAX_BATCH_BYTES + 1];
        ResponseEntity<?> resp = controller.ingestTelemetry(null, null, largeBody, request);
        assertEquals(HttpStatus.PAYLOAD_TOO_LARGE, resp.getStatusCode());
        assertEquals("ERR_BATCH_TOO_LARGE", ((Map<?, ?>) resp.getBody()).get("code"));
    }

    @Test
    void testMalformedJsonReturns400WithSanitizedMessage() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute("jakarta.servlet.request.X509Certificate", new X509Certificate[]{createMockCert("sensor-net-01")});

        ResponseEntity<?> resp = controller.ingestTelemetry(null, null, "{not-json: broken}".getBytes(StandardCharsets.UTF_8), request);
        assertEquals(HttpStatus.BAD_REQUEST, resp.getStatusCode());
        Map<?, ?> body = (Map<?, ?>) resp.getBody();
        assertEquals("ERR_MALFORMED_JSON", body.get("code"));
        assertEquals("Failed to parse telemetry batch JSON", body.get("message"));
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

    // --- Discovery machine transport tests ---

    @Test
    void taskFetchUsesAuthoritativeSensorIdentityAndIgnoresJsonSensorChoice() throws Exception {
        when(discoveryService.fetchTaskForSensor("sensor-a")).thenReturn(Optional.empty());

        mockMvc.perform(post("/api/v1/sensor/discovery/tasks")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{sensorACertificate})
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"sensor_id\":\"sensor-b\"}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.has_task").value(false));

        verify(discoveryService).fetchTaskForSensor("sensor-a");
        verify(discoveryService, never()).fetchTaskForSensor("sensor-b");
    }

    @Test
    void resultPayloadCannotSupplySensorOrTenantAuthority() throws Exception {
        mockMvc.perform(post("/api/v1/sensor/discovery/results")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{sensorACertificate})
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "request_id":"018f0e0a-0000-7000-8000-000000000321",
                                  "assignment_version":7,
                                  "status":"COMPLETED",
                                  "evidence":[],
                                  "sensor_id":"sensor-b",
                                  "tenant_id":"018f0e0a-0000-7000-8000-000000000999"
                                }
                                """))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));

        verify(discoveryService, never()).ingestResultsFromSensor(
                eq("sensor-a"), eq(requestId), eq(7L), anyList(), eq("COMPLETED"), eq(null));
    }

    @Test
    void validResultUsesAuthoritativeSensorIdentityAndAuthoritativeRequestTenantIsNotInWirePayload() throws Exception {
        when(discoveryService.ingestResultsFromSensor(
                eq("sensor-a"), eq(requestId), eq(7L), anyList(), eq("COMPLETED"), eq(null)))
                .thenReturn(new DiscoveryService.ResultIngestOutcome(
                        DiscoveryStatus.COMPLETED, false, tenantId));

        mockMvc.perform(post("/api/v1/sensor/discovery/results")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{sensorACertificate})
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "request_id":"018f0e0a-0000-7000-8000-000000000321",
                                  "assignment_version":7,
                                  "status":"COMPLETED",
                                  "evidence":[]
                                }
                                """))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.status").value("ACCEPTED"))
                .andExpect(jsonPath("$.sensor_identity").value("sensor-a"));

        verify(discoveryService).ingestResultsFromSensor(
                eq("sensor-a"), eq(requestId), eq(7L), anyList(), eq("COMPLETED"), eq(null));
    }

    @Test
    void fractionalAssignmentFenceIsRejectedInsteadOfTruncated() throws Exception {
        mockMvc.perform(post("/api/v1/sensor/discovery/results")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{sensorACertificate})
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "request_id":"018f0e0a-0000-7000-8000-000000000321",
                                  "assignment_version":7.9,
                                  "status":"COMPLETED",
                                  "evidence":[]
                                }
                                """))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));

        verify(discoveryService, never()).ingestResultsFromSensor(
                eq("sensor-a"), eq(requestId), eq(7L), anyList(), eq("COMPLETED"), eq(null));
    }

    @Test
    void discoveryTransportRequiresClientCertificate() throws Exception {
        mockMvc.perform(post("/api/v1/sensor/discovery/tasks"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("ERR_AUTH_CLIENT_CERT_REQUIRED"));
        verify(discoveryService, never()).fetchTaskForSensor("sensor-a");
    }

    @Test
    void discoveryTransportFailsClosedWhenSensorAuthorityUnavailable() throws Exception {
        SensorAuthorityResolver failClosed = new FailClosedSensorAuthorityResolver();
        SensorIngestionController failClosedController = new SensorIngestionController(
                ingestionService, objectMapper, discoveryService, failClosed);
        MockMvc failClosedMvc = MockMvcBuilders.standaloneSetup(failClosedController)
                .setControllerAdvice(new ApiExceptionHandler())
                .build();

        failClosedMvc.perform(post("/api/v1/sensor/discovery/tasks")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{sensorACertificate}))
                .andExpect(status().isForbidden())
                .andExpect(jsonPath("$.code").value("ERR_SENSOR_AUTHORITY_UNAVAILABLE"));

        verify(discoveryService, never()).fetchTaskForSensor("sensor-a");
    }

    @Test
    void certificateCnAloneIsNotDiscoverySensorAuthority() throws Exception {
        // Cert CN is sensor-a, but authority resolver has no registered scope for that CN
        // and no default — must fail closed; CN must not become sensor identity.
        authorityResolver.clear();

        mockMvc.perform(post("/api/v1/sensor/discovery/tasks")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{sensorACertificate}))
                .andExpect(status().isForbidden())
                .andExpect(jsonPath("$.code").value("ERR_SENSOR_AUTHORITY_UNAVAILABLE"));

        verify(discoveryService, never()).fetchTaskForSensor("sensor-a");
    }
}
