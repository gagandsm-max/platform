package com.platform.core.controller;

import com.platform.core.agentcontrol.AgentAuthorityQueryService;
import com.platform.core.agentcontrol.AgentAuthorityView;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.authority.TestSensorAuthorityResolver;
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
import java.security.cert.X509Certificate;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.ArgumentMatchers.isNull;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

class SensorAgentAuthorityControllerTest {

    private static final String CERT_ATTR = "jakarta.servlet.request.X509Certificate";
    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000111");
    private static final String FP =
            "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";

    private TestSensorAuthorityResolver authorityResolver;
    private AgentAuthorityQueryService queryService;
    private SensorAgentAuthorityController controller;
    private MockMvc mockMvc;
    private X509Certificate sensorCert;

    @BeforeEach
    void setUp() {
        authorityResolver = new TestSensorAuthorityResolver();
        // CN is "spoofed-cn"; authoritative Sensor ID remains sensor-infra-01.
        authorityResolver.registerScope("spoofed-cn", new SensorScope(
                "sensor-infra-01", TENANT, null, Set.of(), true));
        queryService = mock(AgentAuthorityQueryService.class);
        controller = new SensorAgentAuthorityController(authorityResolver, queryService);
        mockMvc = MockMvcBuilders.standaloneSetup(controller)
                .setControllerAdvice(new ApiExceptionHandler())
                .build();
        sensorCert = certificate("spoofed-cn");
    }

    private static X509Certificate certificate(String commonName) {
        X509Certificate certificate = mock(X509Certificate.class);
        when(certificate.getSubjectX500Principal()).thenReturn(new X500Principal("CN=" + commonName + ",O=Platform"));
        when(certificate.getSerialNumber()).thenReturn(BigInteger.ONE);
        return certificate;
    }

    @Test
    void resolveAcceptedUsesAuthoritativeSensorNotCn() {
        when(queryService.resolveForSensor(any(), eq(FP), isNull())).thenReturn(
                new AgentAuthorityView("agent-01", TENANT, null, 2L, "sensor-infra-01"));

        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute(CERT_ATTR, new X509Certificate[]{sensorCert});

        ResponseEntity<Map<String, Object>> resp = controller.resolve(
                Map.of("fingerprint_sha256", FP, "agent_id", "payload-spoof", "tenant_id", "other"),
                request);
        assertEquals(HttpStatus.OK, resp.getStatusCode());
        assertEquals("ACCEPTED", resp.getBody().get("status"));
        assertEquals("agent-01", resp.getBody().get("agent_id"));
        assertEquals("sensor-infra-01", resp.getBody().get("sensor_id"));
        assertEquals(2L, ((Number) resp.getBody().get("assignment_generation")).longValue());
        verify(queryService).resolveForSensor(any(SensorScope.class), eq(FP), isNull());
    }

    @Test
    void resolvePassesClaimedAssignmentGeneration() {
        when(queryService.resolveForSensor(any(), eq(FP), eq(7L))).thenReturn(
                new AgentAuthorityView("agent-01", TENANT, null, 7L, "sensor-infra-01"));

        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute(CERT_ATTR, new X509Certificate[]{sensorCert});

        ResponseEntity<Map<String, Object>> resp = controller.resolve(
                Map.of("fingerprint_sha256", FP, "assignment_generation", 7),
                request);
        assertEquals(HttpStatus.OK, resp.getStatusCode());
        verify(queryService).resolveForSensor(any(SensorScope.class), eq(FP), eq(7L));
    }

    @Test
    void resolveIgnoresPayloadAgentAndTenantOverrides() throws Exception {
        when(queryService.resolveForSensor(any(), eq(FP), isNull())).thenReturn(
                new AgentAuthorityView("agent-01", TENANT, null, 1L, "sensor-infra-01"));

        mockMvc.perform(post("/api/v1/sensor/agent-authority/resolve")
                        .contentType(MediaType.APPLICATION_JSON)
                        .requestAttr(CERT_ATTR, new X509Certificate[]{sensorCert})
                        .content("""
                                {"fingerprint_sha256":"%s","agent_id":"AGENT-OVERRIDE","tenant_id":"%s"}
                                """.formatted(FP, UUID.randomUUID())))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.status").value("ACCEPTED"))
                .andExpect(jsonPath("$.agent_id").value("agent-01"));

        verify(queryService).resolveForSensor(any(SensorScope.class), eq(FP), isNull());
        verify(queryService, never()).resolveForSensor(any(), eq("AGENT-OVERRIDE"), any());
    }

    @Test
    void wrongPayloadIdsStillResolveFromFingerprintOnly() {
        when(queryService.resolveForSensor(any(), eq(FP), isNull())).thenReturn(
                new AgentAuthorityView("agent-enrolled", TENANT, null, 1L, "sensor-infra-01"));

        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute(CERT_ATTR, new X509Certificate[]{sensorCert});

        ResponseEntity<Map<String, Object>> resp = controller.resolve(
                Map.of(
                        "fingerprint_sha256", FP,
                        "agent_id", "wrong-agent",
                        "tenant_id", UUID.randomUUID().toString()
                ),
                request);
        assertEquals(HttpStatus.OK, resp.getStatusCode());
        assertEquals("agent-enrolled", resp.getBody().get("agent_id"));
        assertEquals(TENANT.toString(), resp.getBody().get("tenant_id"));
        verify(queryService).resolveForSensor(any(SensorScope.class), eq(FP), isNull());
        verify(queryService, never()).resolveForSensor(any(), eq("wrong-agent"), any());
    }

    @Test
    void fingerprintRejectionIgnoresPayloadAgentId() {
        when(queryService.resolveForSensor(any(), eq(FP), isNull())).thenThrow(
                new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_CREDENTIAL_UNKNOWN", "unknown"));

        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute(CERT_ATTR, new X509Certificate[]{sensorCert});

        ResponseEntity<Map<String, Object>> resp = controller.resolve(
                Map.of("fingerprint_sha256", FP, "agent_id", "spoof-agent"),
                request);
        assertEquals(HttpStatus.FORBIDDEN, resp.getStatusCode());
        assertEquals("ERR_AGENT_CREDENTIAL_UNKNOWN", resp.getBody().get("code"));
        verify(queryService).resolveForSensor(any(SensorScope.class), eq(FP), isNull());
    }

    @Test
    void missingClientCertUnauthorized() {
        ResponseEntity<Map<String, Object>> resp = controller.resolve(Map.of("fingerprint_sha256", FP),
                new MockHttpServletRequest());
        assertEquals(HttpStatus.UNAUTHORIZED, resp.getStatusCode());
        assertEquals("ERR_AUTH_CLIENT_CERT_REQUIRED", resp.getBody().get("code"));
        verify(queryService, never()).resolveForSensor(any(), any(), any());
    }

    @Test
    void cnSpoofWithoutEnrollmentFailsClosed() {
        authorityResolver.clear();
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute(CERT_ATTR, new X509Certificate[]{sensorCert});
        ResponseEntity<Map<String, Object>> resp = controller.resolve(
                Map.of("fingerprint_sha256", FP), request);
        assertEquals(HttpStatus.FORBIDDEN, resp.getStatusCode());
        assertEquals("ERR_SENSOR_AUTHORITY_UNAVAILABLE", resp.getBody().get("code"));
        verify(queryService, never()).resolveForSensor(any(), any(), any());
    }

    @Test
    void agentAuthorityErrorsPropagate() {
        when(queryService.resolveForSensor(any(), eq(FP), isNull())).thenThrow(
                new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_CREDENTIAL_UNKNOWN", "unknown"));
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute(CERT_ATTR, new X509Certificate[]{sensorCert});
        ResponseEntity<Map<String, Object>> resp = controller.resolve(
                Map.of("fingerprint_sha256", FP), request);
        assertEquals(HttpStatus.FORBIDDEN, resp.getStatusCode());
        assertEquals("ERR_AGENT_CREDENTIAL_UNKNOWN", resp.getBody().get("code"));
    }

    @Test
    void missingFingerprintReturnsUnknown() {
        when(queryService.resolveForSensor(any(), eq(null), isNull())).thenThrow(
                new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_CREDENTIAL_UNKNOWN", "missing"));
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute(CERT_ATTR, new X509Certificate[]{sensorCert});
        ResponseEntity<Map<String, Object>> resp = controller.resolve(Map.of(), request);
        assertEquals(HttpStatus.FORBIDDEN, resp.getStatusCode());
        assertNull(resp.getBody().get("agent_id"));
    }

    @Test
    void nonNumericAssignmentGenerationFailsClosed() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setAttribute(CERT_ATTR, new X509Certificate[]{sensorCert});
        ResponseEntity<Map<String, Object>> resp = controller.resolve(
                Map.of("fingerprint_sha256", FP, "assignment_generation", "not-a-number"),
                request);
        assertEquals(HttpStatus.FORBIDDEN, resp.getStatusCode());
        assertEquals("ERR_AGENT_ASSIGNMENT_GENERATION_MISMATCH", resp.getBody().get("code"));
        verify(queryService, never()).resolveForSensor(any(), any(), any());
    }
}
