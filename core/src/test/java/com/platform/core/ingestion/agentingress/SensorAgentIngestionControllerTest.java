package com.platform.core.ingestion.agentingress;

import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.core.read.ListAppender;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryAckResponse;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.security.cert.X509Certificate;
import java.util.stream.Collectors;

import static org.assertj.core.api.Assertions.assertThat;
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
    private static final String SENTINEL = "SUPER_SECRET_SENTINEL_DO_NOT_LOG";
    private static final String STREAM = "stream-00112233445566778899aabbccddeeff";
    private static final String VALID_BODY = """
            {"schemaVersion":2,"sensorId":"sensor-infra-01",
             "sensorAgentSpoolStreamId":"%s","batchId":"batch-1",
             "firstIngressSequence":1,"lastIngressSequence":3,"recordCount":1,
             "checksum":"abc","correlationId":"corr-1","records":[]}
            """.formatted(STREAM);

    private AgentIngestionService ingestionService;
    private MockMvc mockMvc;
    private ListAppender<ILoggingEvent> logAppender;
    private Logger controllerLogger;

    @BeforeEach
    void setUp() {
        ingestionService = mock(AgentIngestionService.class);
        SensorAgentIngestionController controller = new SensorAgentIngestionController(
                ingestionService, new ObjectMapper().findAndRegisterModules());
        mockMvc = MockMvcBuilders.standaloneSetup(controller)
                .setControllerAdvice(new ApiExceptionHandler())
                .build();

        controllerLogger = (Logger) LoggerFactory.getLogger(SensorAgentIngestionController.class);
        logAppender = new ListAppender<>();
        logAppender.start();
        controllerLogger.addAppender(logAppender);
    }

    @AfterEach
    void tearDown() {
        if (controllerLogger != null && logAppender != null) {
            controllerLogger.detachAppender(logAppender);
            logAppender.stop();
        }
    }

    @Test
    void missingClientCertReturns401() throws Exception {
        mockMvc.perform(post("/api/v1/sensor/agent-telemetry")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"schemaVersion\":2}"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("ERR_AUTH_CLIENT_CERT_REQUIRED"));
        verify(ingestionService, never()).ingestAgentTelemetry(any(), any(), any(), any());
    }

    @Test
    void withClientCertDelegatesToService() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(ingestionService.ingestAgentTelemetry(any(), any(), any(), any()))
                .thenReturn(new AgentTelemetryAckResponse(
                        "ACCEPTED", "sensor-infra-01", "batch-1", 3L, "corr-1"));

        mockMvc.perform(post("/api/v1/sensor/agent-telemetry")
                        .contentType(MediaType.APPLICATION_JSON)
                        .header("Idempotency-Key", "batch-1")
                        .content(VALID_BODY)
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert}))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.status").value("ACCEPTED"))
                .andExpect(jsonPath("$.acceptedThroughIngressSequence").value(3));
        verify(ingestionService).ingestAgentTelemetry(any(), any(), any(), any());
    }

    @Test
    void malformedJsonDoesNotLogPayloadOrSentinel() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        String malformedBody = "{not-json " + SENTINEL + "}";

        mockMvc.perform(post("/api/v1/sensor/agent-telemetry")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(malformedBody)
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert}))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("ERR_MALFORMED_JSON"))
                .andExpect(jsonPath("$.message").value("Failed to parse agent-telemetry batch JSON"));

        String captured = capturedLogText();
        assertThat(captured).doesNotContain(SENTINEL);
        assertThat(captured).doesNotContain(malformedBody);
        assertThat(captured).contains("ERR_MALFORMED_JSON");
        assertThat(captured).containsPattern("failureClass=\\w+");
        verify(ingestionService, never()).ingestAgentTelemetry(any(), any(), any(), any());
    }

    @Test
    void ingestionExceptionDoesNotLogExceptionMessageSentinel() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(ingestionService.ingestAgentTelemetry(any(), any(), any(), any()))
                .thenThrow(new IngestionException(
                        HttpStatus.FORBIDDEN,
                        "ERR_SCOPE_DENIED",
                        "denied because " + SENTINEL));

        mockMvc.perform(post("/api/v1/sensor/agent-telemetry")
                        .contentType(MediaType.APPLICATION_JSON)
                        .header("Idempotency-Key", "batch-1")
                        .content(VALID_BODY)
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert}))
                .andExpect(status().isForbidden())
                .andExpect(jsonPath("$.code").value("ERR_SCOPE_DENIED"))
                .andExpect(jsonPath("$.message").value("denied because " + SENTINEL));

        String captured = capturedLogText();
        assertThat(captured).doesNotContain(SENTINEL);
        assertThat(captured).contains("ERR_SCOPE_DENIED");
        assertThat(captured).contains("403");
        assertThat(captured).contains("FORBIDDEN");
        verify(ingestionService).ingestAgentTelemetry(any(), any(), any(), any());
    }

    @Test
    void unexpectedExceptionDoesNotLogThrowableMessageSentinel() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(ingestionService.ingestAgentTelemetry(any(), any(), any(), any()))
                .thenThrow(new RuntimeException("boom " + SENTINEL));

        mockMvc.perform(post("/api/v1/sensor/agent-telemetry")
                        .contentType(MediaType.APPLICATION_JSON)
                        .header("Idempotency-Key", "batch-1")
                        .content(VALID_BODY)
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert}))
                .andExpect(status().isInternalServerError())
                .andExpect(jsonPath("$.code").value("ERR_INTERNAL_ERROR"))
                .andExpect(jsonPath("$.message").value("Internal server error processing agent telemetry"));

        String captured = capturedLogText();
        assertThat(captured).doesNotContain(SENTINEL);
        assertThat(captured).contains("ERR_INTERNAL_ERROR");
        assertThat(captured).contains("failureClass=RuntimeException");
        verify(ingestionService).ingestAgentTelemetry(any(), any(), any(), any());
    }

    private String capturedLogText() {
        return logAppender.list.stream()
                .map(event -> {
                    StringBuilder sb = new StringBuilder(event.getFormattedMessage());
                    if (event.getThrowableProxy() != null) {
                        sb.append(' ').append(event.getThrowableProxy().getMessage());
                        if (event.getThrowableProxy().getCause() != null) {
                            sb.append(' ').append(event.getThrowableProxy().getCause().getMessage());
                        }
                    }
                    return sb.toString();
                })
                .collect(Collectors.joining("\n"));
    }
}
