package com.platform.core.controller;

import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.DomainException;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.logging.CorrelationIdFilter;
import com.platform.core.sensorcontrol.desiredstate.SensorControlDesiredChecksum;
import com.platform.core.sensorcontrol.desiredstate.SensorControlDesiredChecksumTest;
import com.platform.core.sensorcontrol.desiredstate.SensorControlDesiredLimits;
import com.platform.core.sensorcontrol.desiredstate.SensorControlDesiredStateService;
import com.platform.core.sensorcontrol.desiredstate.SensorControlObservationOutcome;
import com.platform.core.sensorcontrol.desiredstate.SensorControlState;
import com.platform.core.sensorcontrol.desiredstate.SensorRuntimeDesiredConfig;
import com.platform.core.sensorcontrol.desiredstate.SensorSchedulerPoolConfig;
import com.platform.core.sensorcontrol.desiredstate.SensorSchedulerPoolName;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.EnumMap;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;

import static org.hamcrest.Matchers.nullValue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.ArgumentMatchers.isNull;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.put;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

class SensorControlDesiredStateControllerTest {

    private static final String CERT_ATTR = "jakarta.servlet.request.X509Certificate";
    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID OTHER = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b1");
    private static final String SENSOR = "sensor-mtls-01";

    private MockMvc mockMvc;
    private SensorAuthorityResolver resolver;
    private SensorControlDesiredStateService service;

    @BeforeEach
    void setUp() {
        resolver = mock(SensorAuthorityResolver.class);
        service = mock(SensorControlDesiredStateService.class);
        mockMvc = MockMvcBuilders.standaloneSetup(new SensorControlDesiredStateController(resolver, service))
                .addFilters(new CorrelationIdFilter())
                .setControllerAdvice(new ApiExceptionHandler())
                .build();
    }

    @Test
    void missingClientCertReturns401() throws Exception {
        mockMvc.perform(get("/api/v1/sensor/control/desired-state"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("ERR_AUTH_CLIENT_CERT_REQUIRED"));
        verify(service, never()).findDesiredState(any(), any());
    }

    @Test
    void getNoDesiredReturns204() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));
        when(service.findDesiredState(TENANT, SENSOR)).thenReturn(Optional.empty());

        mockMvc.perform(get("/api/v1/sensor/control/desired-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert}))
                .andExpect(status().isNoContent());
    }

    @Test
    void getDesiredUsesMtlsScopeAndIgnoresClaimedIdentity() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));
        SensorControlState state = sampleState(TENANT, SENSOR);
        when(service.findDesiredState(TENANT, SENSOR)).thenReturn(Optional.of(state));

        mockMvc.perform(get("/api/v1/sensor/control/desired-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert})
                        .header("X-Claimed-Sensor-Id", "evil-sensor")
                        .header("X-Claimed-Tenant-Id", OTHER.toString()))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.schemaVersion").value(1))
                .andExpect(jsonPath("$.desiredGeneration").value(1))
                .andExpect(jsonPath("$.desiredChecksum").value(state.desiredChecksum()))
                .andExpect(jsonPath("$.configuration.logLevel").value("INFO"))
                .andExpect(jsonPath("$.appliedChecksum").value(nullValue()))
                .andExpect(jsonPath("$.observedChecksum").value(nullValue()))
                .andExpect(jsonPath("$.observedOutcome").value(nullValue()));

        verify(service).findDesiredState(TENANT, SENSOR);
        verify(service, never()).findDesiredState(eq(OTHER), any());
    }

    @Test
    void unknownSensorAuthorityFailsClosed() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenThrow(new IngestionException(
                HttpStatus.FORBIDDEN, "ERR_SENSOR_UNKNOWN", "unknown"));

        mockMvc.perform(get("/api/v1/sensor/control/desired-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert}))
                .andExpect(status().isForbidden())
                .andExpect(jsonPath("$.code").value("ERR_SENSOR_UNKNOWN"));
    }

    @Test
    void putAckWithoutCertFailsClosed() throws Exception {
        mockMvc.perform(put("/api/v1/sensor/control/applied-state")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(validAppliedBody("a".repeat(64))))
                .andExpect(status().isUnauthorized());
        verify(service, never()).acknowledgeAppliedState(
                any(), any(), anyInt(), anyLong(), anyString(), any(), any(), any());
    }

    @Test
    void putValidAckSucceeds() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));
        SensorControlState state = sampleState(TENANT, SENSOR);
        when(service.acknowledgeAppliedState(
                eq(TENANT), eq(SENSOR), eq(1), eq(1L), eq(state.desiredChecksum()),
                eq(SensorControlObservationOutcome.APPLIED), isNull(), isNull()))
                .thenReturn(appliedState(state));

        mockMvc.perform(put("/api/v1/sensor/control/applied-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert})
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(validAppliedBody(state.desiredChecksum())))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.status").value("ACCEPTED"))
                .andExpect(jsonPath("$.outcome").value("APPLIED"));

        verify(service).acknowledgeAppliedState(
                eq(TENANT), eq(SENSOR), eq(1), eq(1L), eq(state.desiredChecksum()),
                eq(SensorControlObservationOutcome.APPLIED), isNull(), isNull());
    }

    @Test
    void putAckRejectsUnknownProperty() throws Exception {
        expectAckValidationFailed("""
                {"schemaVersion":1,"generation":1,"checksum":"%s","outcome":"APPLIED","mystery":"x"}
                """.formatted("a".repeat(64)));
    }

    @Test
    void putAckRejectsTenantIdClaim() throws Exception {
        expectAckValidationFailed("""
                {"schemaVersion":1,"generation":1,"checksum":"%s","outcome":"APPLIED","tenantId":"%s"}
                """.formatted("a".repeat(64), OTHER));
    }

    @Test
    void putAckRejectsSensorIdClaim() throws Exception {
        expectAckValidationFailed("""
                {"schemaVersion":1,"generation":1,"checksum":"%s","outcome":"APPLIED","sensorId":"evil"}
                """.formatted("a".repeat(64)));
    }

    @Test
    void putAckRejectsActorClaim() throws Exception {
        expectAckValidationFailed("""
                {"schemaVersion":1,"generation":1,"checksum":"%s","outcome":"APPLIED","actor":"x"}
                """.formatted("a".repeat(64)));
    }

    @Test
    void putAckInvalidOutcomeIsValidationFailedNot500() throws Exception {
        expectAckValidationFailed("""
                {"schemaVersion":1,"generation":1,"checksum":"%s","outcome":"BOGUS"}
                """.formatted("a".repeat(64)));
    }

    @Test
    void putAckMalformedJsonIsValidationFailed() throws Exception {
        expectAckValidationFailed("{not-json");
    }

    @Test
    void putAckTrailingDocumentIsValidationFailed() throws Exception {
        expectAckValidationFailed("""
                {"schemaVersion":1,"generation":1,"checksum":"%s","outcome":"APPLIED"} {}
                """.formatted("a".repeat(64)));
    }

    @Test
    void putAckOversizedBodyIsValidationFailedAndDoesNotInvokeService() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));
        String pad = "x".repeat(SensorControlDesiredLimits.MAX_ACK_REQUEST_BYTES);
        String body = "{\"schemaVersion\":1,\"generation\":1,\"checksum\":\""
                + "a".repeat(64) + "\",\"outcome\":\"APPLIED\",\"rejectionMessage\":\"" + pad + "\"}";
        assert body.length() > SensorControlDesiredLimits.MAX_ACK_REQUEST_BYTES;

        mockMvc.perform(put("/api/v1/sensor/control/applied-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert})
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(body))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));

        verify(service, never()).acknowledgeAppliedState(
                any(), any(), anyInt(), anyLong(), anyString(), any(), any(), any());
    }

    @Test
    void putAckWithoutCertIgnoresOversizedBodyAndDoesNotInvokeService() throws Exception {
        String pad = "x".repeat(SensorControlDesiredLimits.MAX_ACK_REQUEST_BYTES + 64);
        mockMvc.perform(put("/api/v1/sensor/control/applied-state")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(pad))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("ERR_AUTH_CLIENT_CERT_REQUIRED"));
        verify(service, never()).acknowledgeAppliedState(
                any(), any(), anyInt(), anyLong(), anyString(), any(), any(), any());
        verify(resolver, never()).resolve(any());
    }

    @Test
    void putAckConflictMapsTo409() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));
        when(service.acknowledgeAppliedState(any(), any(), anyInt(), anyLong(), anyString(), any(), any(), any()))
                .thenThrow(new DomainException(
                        SensorControlDesiredStateService.ERR_ACK_STALE, "stale"));

        mockMvc.perform(put("/api/v1/sensor/control/applied-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert})
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(validAppliedBody("a".repeat(64))))
                .andExpect(status().isConflict())
                .andExpect(jsonPath("$.code").value("ERR_SENSOR_CONTROL_ACK_STALE"));
    }

    private void expectAckValidationFailed(String body) throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));

        mockMvc.perform(put("/api/v1/sensor/control/applied-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert})
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(body))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));

        verify(service, never()).acknowledgeAppliedState(
                any(), any(), anyInt(), anyLong(), anyString(), any(), any(), any());
    }

    private static String validAppliedBody(String checksum) {
        return """
                {"schemaVersion":1,"generation":1,"checksum":"%s","outcome":"APPLIED"}
                """.formatted(checksum);
    }

    private static SensorControlState sampleState(UUID tenantId, String sensorId) {
        EnumMap<SensorSchedulerPoolName, SensorSchedulerPoolConfig> pools =
                new EnumMap<>(SensorSchedulerPoolName.class);
        pools.put(SensorSchedulerPoolName.NETWORK_POLL, new SensorSchedulerPoolConfig(2, 8));
        pools.put(SensorSchedulerPoolName.SYNTHETIC, new SensorSchedulerPoolConfig(1, 4));
        SensorRuntimeDesiredConfig cfg = SensorControlDesiredChecksumTest.goldenConfig(pools);
        String checksum = SensorControlDesiredChecksum.compute(cfg);
        Instant now = Instant.parse("2026-01-01T00:00:00Z");
        return new SensorControlState(
                tenantId, sensorId, 1, 1L, checksum, cfg, "actor", now,
                0L, null, null, 0L, null, null, null, null, null, now);
    }

    private static SensorControlState appliedState(SensorControlState state) {
        Instant at = Instant.parse("2026-01-01T00:00:01Z");
        return new SensorControlState(
                state.tenantId(), state.sensorId(), 1, 1L, state.desiredChecksum(), state.desiredConfig(),
                "actor", Instant.parse("2026-01-01T00:00:00Z"),
                1L, state.desiredChecksum(), at,
                1L, state.desiredChecksum(), SensorControlObservationOutcome.APPLIED,
                null, null, at, at);
    }
}
