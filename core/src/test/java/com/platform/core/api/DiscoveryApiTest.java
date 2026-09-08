package com.platform.core.api;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.api.PageResult;
import com.platform.core.api.dto.DiscoveryDtos.CreateDiscoveryRequest;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.discovery.CandidateReviewStatus;
import com.platform.core.domain.discovery.DiscoveryCandidate;
import com.platform.core.domain.discovery.DiscoveryConfidence;
import com.platform.core.domain.discovery.DiscoveryEvidence;
import com.platform.core.domain.discovery.DiscoveryMethod;
import com.platform.core.domain.discovery.DiscoveryRequest;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.discovery.DiscoveryStatus;
import com.platform.core.domain.discovery.EvidenceStatus;
import com.platform.core.domain.discovery.ReachabilityKind;
import com.platform.core.logging.CorrelationIdFilter;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.UUID;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.header;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

class DiscoveryApiTest {

    private MockMvc mockMvc;
    private DiscoveryService discoveryService;
    private final ObjectMapper mapper = new ObjectMapper().findAndRegisterModules();

    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000010");
    private final UUID requestId = UUID.fromString("018f0e0a-0000-7000-8000-000000000020");

    @BeforeEach
    void setUp() {
        discoveryService = mock(DiscoveryService.class);
        DiscoveryController discoveryController = new DiscoveryController(discoveryService);

        mockMvc = MockMvcBuilders.standaloneSetup(discoveryController)
                .addFilters(new CorrelationIdFilter())
                .setControllerAdvice(new ApiExceptionHandler())
                .build();
    }

    @Test
    void createDiscovery_ReturnsPendingUntilCorePlacementAssignsSensor() throws Exception {
        DiscoveryRequest req = new DiscoveryRequest(
                requestId,
                tenantId,
                "Office LAN",
                DiscoveryStatus.PENDING,
                "corr-disc-1",
                "idem-1",
                null,
                null,
                null,
                List.of(DiscoveryMethod.ICMP, DiscoveryMethod.SNMP),
                "prof-cisco",
                161,
                2000,
                2,
                "admin",
                Instant.now(),
                null,
                null,
                Map.of(),
                null
        );

        when(discoveryService.createDiscovery(
                eq(tenantId),
                eq("Office LAN"),
                eq(List.of("192.168.1.0/24")),
                eq(List.of(DiscoveryMethod.ICMP, DiscoveryMethod.SNMP)),
                eq("prof-cisco"),
                eq(null),
                eq(null),
                eq(161),
                eq(2000),
                eq(2),
                eq("idem-1"),
                eq("corr-disc-1"),
                eq("admin")
        )).thenReturn(req);

        CreateDiscoveryRequest payload = new CreateDiscoveryRequest(
                "Office LAN",
                List.of("192.168.1.0/24"),
                List.of("ICMP", "SNMP"),
                "prof-cisco",
                null,
                null,
                161,
                2000,
                2,
                "idem-1",
                "admin"
        );

        mockMvc.perform(post("/api/v1/tenants/" + tenantId + "/discovery")
                        .contentType(MediaType.APPLICATION_JSON)
                        .header("Idempotency-Key", "idem-1")
                        .header(CorrelationIdFilter.CORRELATION_ID_HEADER, "corr-disc-1")
                        .content(mapper.writeValueAsString(payload)))
                .andExpect(status().isCreated())
                .andExpect(header().string(CorrelationIdFilter.CORRELATION_ID_HEADER, "corr-disc-1"))
                .andExpect(jsonPath("$.requestId").value(requestId.toString()))
                .andExpect(jsonPath("$.name").value("Office LAN"))
                .andExpect(jsonPath("$.status").value("PENDING"))
                .andExpect(jsonPath("$.assignedSensorId").isEmpty());
    }

    @Test
    void createDiscovery_RejectsSecurityViolationWith400() throws Exception {
        when(discoveryService.createDiscovery(any(), any(), any(), any(), any(), any(), any(), any(), any(), any(), any(), any(), any()))
                .thenThrow(new DomainException("SECURITY_VIOLATION", "Loopback addresses are prohibited"));

        CreateDiscoveryRequest payload = new CreateDiscoveryRequest(
                "Malicious Scan",
                List.of("127.0.0.1"),
                List.of("ICMP"),
                null, null, null, null, null, null, null, null
        );

        mockMvc.perform(post("/api/v1/tenants/" + tenantId + "/discovery")
                        .contentType(MediaType.APPLICATION_JSON)
                        .header(CorrelationIdFilter.CORRELATION_ID_HEADER, "corr-sec-1")
                        .content(mapper.writeValueAsString(payload)))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("SECURITY_VIOLATION"))
                .andExpect(jsonPath("$.correlationId").value("corr-sec-1"));
    }

    @Test
    void getCandidates_ReturnsCandidatesInPendingReview() throws Exception {
        DiscoveryCandidate candidate = new DiscoveryCandidate(
                UUID.randomUUID(),
                tenantId,
                requestId,
                "192.168.1.100",
                "switch-01",
                "1.3.6.1.4.1.9.1.1745",
                "Cisco IOS",
                List.of(DiscoveryMethod.ICMP, DiscoveryMethod.SNMP),
                Map.of("sysDescr", "Cisco IOS"),
                DiscoveryConfidence.HIGH,
                CandidateReviewStatus.PENDING_REVIEW,
                false,
                null,
                null,
                Instant.now(),
                Instant.now(),
                Instant.now(),
                Instant.now()
        );

        when(discoveryService.listCandidates(eq(tenantId), eq(requestId), anyInt(), anyInt()))
                .thenReturn(new PageResult<>(List.of(candidate), 1, 50, 1L, false));

        mockMvc.perform(get("/api/v1/tenants/" + tenantId + "/discovery/" + requestId + "/candidates")
                        .header(CorrelationIdFilter.CORRELATION_ID_HEADER, "corr-cand-1"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.items[0].targetAddress").value("192.168.1.100"))
                .andExpect(jsonPath("$.items[0].discoveredName").value("switch-01"))
                .andExpect(jsonPath("$.items[0].confidence").value("HIGH"))
                .andExpect(jsonPath("$.items[0].reviewStatus").value("PENDING_REVIEW"));
    }

    @Test
    void getEvidence_ReturnsEvidences() throws Exception {
        DiscoveryEvidence evidence = new DiscoveryEvidence(
                UUID.randomUUID(),
                tenantId,
                requestId,
                UUID.randomUUID(),
                "sensor-01",
                DiscoveryMethod.ICMP,
                "ECHO",
                EvidenceStatus.SUCCESS,
                ReachabilityKind.OBSERVED,
                "ICMP_ECHO_REPLY",
                Map.of("rtt_micros", 1500),
                Map.of(),
                12L,
                Instant.now()
        );

        when(discoveryService.listEvidence(eq(tenantId), eq(requestId), anyInt(), anyInt()))
                .thenReturn(new PageResult<>(List.of(evidence), 1, 50, 1L, false));

        mockMvc.perform(get("/api/v1/tenants/" + tenantId + "/discovery/" + requestId + "/evidence")
                        .header(CorrelationIdFilter.CORRELATION_ID_HEADER, "corr-ev-1"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.items[0].protocol").value("ICMP"))
                .andExpect(jsonPath("$.items[0].status").value("SUCCESS"))
                .andExpect(jsonPath("$.items[0].sensorId").value("sensor-01"));
    }

    @Test
    void cancelDiscovery_ReturnsCancelled() throws Exception {
        DiscoveryRequest req = new DiscoveryRequest(
                requestId,
                tenantId,
                "Cancelled Scan",
                DiscoveryStatus.CANCELLED,
                "corr-cancel",
                null,
                null,
                null,
                "sensor-01",
                List.of(DiscoveryMethod.ICMP),
                null,
                161,
                2000,
                2,
                "admin",
                Instant.now(),
                null,
                Instant.now(),
                Map.of(),
                "User cancelled"
        );

        when(discoveryService.cancelRequest(tenantId, requestId)).thenReturn(req);

        mockMvc.perform(post("/api/v1/tenants/" + tenantId + "/discovery/" + requestId + "/cancel")
                        .header(CorrelationIdFilter.CORRELATION_ID_HEADER, "corr-cancel-1"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.status").value("CANCELLED"));
    }
}
