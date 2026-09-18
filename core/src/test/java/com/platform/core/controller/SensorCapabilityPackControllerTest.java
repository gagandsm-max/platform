package com.platform.core.controller;

import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.capabilitypack.DesiredPackEntry;
import com.platform.core.domain.capabilitypack.SensorCapabilityPackAssignmentService;
import com.platform.core.domain.capabilitypack.SensorPackDesiredState;
import com.platform.core.domain.capabilitypack.StoredCapabilityPack;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.logging.CorrelationIdFilter;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.List;
import java.util.Set;
import java.util.UUID;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.doThrow;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.put;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.content;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.header;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

class SensorCapabilityPackControllerTest {

    private static final String CERT_ATTR = "jakarta.servlet.request.X509Certificate";
    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final String SENSOR = "sensor-mtls-01";

    private MockMvc mockMvc;
    private SensorAuthorityResolver resolver;
    private SensorCapabilityPackAssignmentService assignments;

    @BeforeEach
    void setUp() {
        resolver = mock(SensorAuthorityResolver.class);
        assignments = mock(SensorCapabilityPackAssignmentService.class);
        mockMvc = MockMvcBuilders.standaloneSetup(new SensorCapabilityPackController(resolver, assignments))
                .addFilters(new CorrelationIdFilter())
                .setControllerAdvice(new ApiExceptionHandler())
                .build();
    }

    @Test
    void missingClientCertReturns401() throws Exception {
        mockMvc.perform(get("/api/v1/sensor/capability-packs/desired-state"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("ERR_AUTH_CLIENT_CERT_REQUIRED"));
    }

    @Test
    void desiredStateUsesMtlsScopeAndIgnoresClaimedIds() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));
        when(assignments.getDesiredState(TENANT, SENSOR)).thenReturn(new SensorPackDesiredState(
                2L, "a".repeat(64), List.of(new DesiredPackEntry("pack.a", "1.0.0", "b".repeat(64), 12))));

        mockMvc.perform(get("/api/v1/sensor/capability-packs/desired-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert})
                        .header("X-Claimed-Sensor-Id", "evil-sensor"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.generation").value(2))
                .andExpect(jsonPath("$.packs[0].packId").value("pack.a"))
                .andExpect(jsonPath("$.packs[0].manifestBytes").doesNotExist());

        verify(assignments).getDesiredState(TENANT, SENSOR);
    }

    @Test
    void artifactReturnsExactBytesAndIdentityHeaders() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));
        byte[] bytes = "{\"schemaVersion\":\"1.0.0\"}".getBytes();
        StoredCapabilityPack pack = new StoredCapabilityPack(
                TENANT, "pack.a", "1.0.0", "1.0.0", "DEVICE_NETWORK", "n",
                "CUSTOMER", "p", "EXPERIMENTAL", "c".repeat(64), bytes, "actor", Instant.now());
        when(assignments.requireAssignedArtifact(TENANT, SENSOR, "pack.a", "1.0.0")).thenReturn(pack);

        mockMvc.perform(get("/api/v1/sensor/capability-packs/artifacts/pack.a/versions/1.0.0")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert}))
                .andExpect(status().isOk())
                .andExpect(header().string(SensorCapabilityPackController.HDR_PACK_SHA256, "c".repeat(64)))
                .andExpect(header().string(SensorCapabilityPackController.HDR_PACK_ID, "pack.a"))
                .andExpect(header().string(SensorCapabilityPackController.HDR_PACK_VERSION, "1.0.0"))
                .andExpect(content().bytes(bytes));
    }

    @Test
    void unassignedArtifactConcealedAs404() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));
        when(assignments.requireAssignedArtifact(any(), any(), any(), any()))
                .thenThrow(new DomainException(
                        SensorCapabilityPackAssignmentService.ERR_ARTIFACT_NOT_FOUND, "missing"));

        mockMvc.perform(get("/api/v1/sensor/capability-packs/artifacts/pack.x/versions/1.0.0")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert}))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("ERR_NOT_FOUND"));
    }

    @Test
    void appliedStateConflictCodes() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));
        doThrow(new DomainException(
                SensorCapabilityPackAssignmentService.ERR_PACK_STATE_STALE, "stale"))
                .when(assignments).ackAppliedState(eq(TENANT), eq(SENSOR), eq(1L), eq("a".repeat(64)));

        mockMvc.perform(put("/api/v1/sensor/capability-packs/applied-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert})
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"generation\":1,\"checksum\":\"" + "a".repeat(64) + "\"}"))
                .andExpect(status().isConflict())
                .andExpect(jsonPath("$.code").value("ERR_SENSOR_PACK_STATE_STALE"));
    }

    @Test
    void authorityUnavailablePropagates() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenThrow(new IngestionException(
                HttpStatus.FORBIDDEN, "ERR_SENSOR_AUTHORITY_UNAVAILABLE", "unavailable"));

        mockMvc.perform(get("/api/v1/sensor/capability-packs/desired-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert}))
                .andExpect(status().isForbidden())
                .andExpect(jsonPath("$.code").value("ERR_SENSOR_AUTHORITY_UNAVAILABLE"));
    }
}
