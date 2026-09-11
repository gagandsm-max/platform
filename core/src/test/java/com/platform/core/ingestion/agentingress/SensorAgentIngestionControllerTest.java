package com.platform.core.ingestion.agentingress;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryAckResponse;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.security.cert.X509Certificate;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

class SensorAgentIngestionControllerTest {

    private static final String CERT_ATTR = "jakarta.servlet.request.X509Certificate";

    private AgentIngestionService ingestionService;
    private MockMvc mockMvc;

    @BeforeEach
    void setUp() {
        ingestionService = mock(AgentIngestionService.class);
        SensorAgentIngestionController controller = new SensorAgentIngestionController(
                ingestionService, new ObjectMapper().findAndRegisterModules());
        mockMvc = MockMvcBuilders.standaloneSetup(controller)
                .setControllerAdvice(new ApiExceptionHandler())
                .build();
    }

    @Test
    void missingClientCertReturns401() throws Exception {
        mockMvc.perform(post("/api/v1/sensor/agent-telemetry")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"schemaVersion\":1}"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("ERR_AUTH_CLIENT_CERT_REQUIRED"));
        verify(ingestionService, never()).ingestAgentTelemetry(any(), any(), any());
    }

    @Test
    void withClientCertDelegatesToService() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(ingestionService.ingestAgentTelemetry(any(), any(), any()))
                .thenReturn(new AgentTelemetryAckResponse(
                        "ACCEPTED", "sensor-infra-01", "batch-1", 3L, "corr-1"));

        mockMvc.perform(post("/api/v1/sensor/agent-telemetry")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {"schemaVersion":1,"sensorId":"sensor-infra-01","batchId":"batch-1",
                                 "firstIngressSequence":1,"lastIngressSequence":3,"recordCount":1,
                                 "checksum":"abc","correlationId":"corr-1","records":[]}
                                """)
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert}))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.status").value("ACCEPTED"))
                .andExpect(jsonPath("$.acceptedThroughIngressSequence").value(3));
        verify(ingestionService).ingestAgentTelemetry(any(), any(), any());
    }
}
