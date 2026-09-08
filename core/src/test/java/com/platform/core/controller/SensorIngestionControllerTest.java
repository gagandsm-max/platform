package com.platform.core.controller;

import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.discovery.DiscoveryStatus;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import javax.security.auth.x500.X500Principal;
import java.math.BigInteger;
import java.security.cert.X509Certificate;
import java.util.List;
import java.util.Optional;
import java.util.UUID;

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

    private DiscoveryService discoveryService;
    private MockMvc mockMvc;
    private X509Certificate sensorACertificate;

    @BeforeEach
    void setUp() {
        discoveryService = mock(DiscoveryService.class);
        mockMvc = MockMvcBuilders.standaloneSetup(new SensorIngestionController(discoveryService))
                .setControllerAdvice(new ApiExceptionHandler())
                .build();
        sensorACertificate = certificate("sensor-a");
    }

    @Test
    void taskFetchUsesCertificateIdentityAndIgnoresJsonSensorChoice() throws Exception {
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

        verify(discoveryService, never()).ingestResultsFromSensor(eq("sensor-a"), eq(requestId), eq(7L), anyList(), eq("COMPLETED"), eq(null));
    }

    @Test
    void validResultUsesCertificateIdentityAndAuthoritativeRequestTenantIsNotInWirePayload() throws Exception {
        when(discoveryService.ingestResultsFromSensor(
                eq("sensor-a"), eq(requestId), eq(7L), anyList(), eq("COMPLETED"), eq(null)))
                .thenReturn(new DiscoveryService.ResultIngestOutcome(
                        DiscoveryStatus.COMPLETED, false,
                        UUID.fromString("018f0e0a-0000-7000-8000-000000000111")));

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

        verify(discoveryService, never()).ingestResultsFromSensor(eq("sensor-a"), eq(requestId), eq(7L), anyList(), eq("COMPLETED"), eq(null));
    }

    @Test
    void discoveryTransportRequiresClientCertificate() throws Exception {
        mockMvc.perform(post("/api/v1/sensor/discovery/tasks"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("ERR_AUTH_CLIENT_CERT_REQUIRED"));
        verify(discoveryService, never()).fetchTaskForSensor("sensor-a");
    }

    private static X509Certificate certificate(String commonName) {
        X509Certificate certificate = mock(X509Certificate.class);
        when(certificate.getSubjectX500Principal()).thenReturn(new X500Principal("CN=" + commonName + ",O=Platform"));
        when(certificate.getSerialNumber()).thenReturn(BigInteger.ONE);
        return certificate;
    }
}
