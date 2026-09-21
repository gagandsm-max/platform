package com.platform.core.controller;

import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.recurring.DesiredRecurringDefinition;
import com.platform.core.domain.recurring.SensorRecurringAssignmentService;
import com.platform.core.domain.recurring.SensorRecurringDesiredState;
import com.platform.core.domain.recurring.SnmpParams;
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
import java.util.List;
import java.util.Set;
import java.util.UUID;

import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.doThrow;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.put;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

class SensorRecurringScheduleControllerTest {

    private static final String CERT_ATTR = "jakarta.servlet.request.X509Certificate";
    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final String SENSOR = "sensor-mtls-01";

    private MockMvc mockMvc;
    private SensorAuthorityResolver resolver;
    private SensorRecurringAssignmentService assignments;

    @BeforeEach
    void setUp() {
        resolver = mock(SensorAuthorityResolver.class);
        assignments = mock(SensorRecurringAssignmentService.class);
        mockMvc = MockMvcBuilders.standaloneSetup(new SensorRecurringScheduleController(resolver, assignments))
                .addFilters(new CorrelationIdFilter())
                .setControllerAdvice(new ApiExceptionHandler())
                .build();
    }

    @Test
    void missingClientCertReturns401() throws Exception {
        mockMvc.perform(get("/api/v1/sensor/recurring-schedules/desired-state"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("ERR_AUTH_CLIENT_CERT_REQUIRED"));
    }

    @Test
    void desiredStateUsesMtlsScopeAndIgnoresClaimedIds() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));
        SnmpParams.SnmpLimits limits = new SnmpParams.SnmpLimits(16, 100, 10, 100, 65_536);
        SnmpParams snmp = new SnmpParams(
                "127.0.0.1", 161, "2c", "GET", List.of(".1.3.6.1.2.1.1.3.0"), 1_000L, limits);
        when(assignments.getDesiredState(TENANT, SENSOR)).thenReturn(new SensorRecurringDesiredState(
                2L,
                "a".repeat(64),
                1L,
                "c".repeat(64),
                List.of(new DesiredRecurringDefinition(
                        "assign-1",
                        TENANT.toString(),
                        "entity-1",
                        "mon-1",
                        "LIVENESS",
                        "NETWORK_POLL",
                        60_000L,
                        90_000L,
                        1,
                        5_000L,
                        "cred-1",
                        "SNMP",
                        snmp
                ))));

        mockMvc.perform(get("/api/v1/sensor/recurring-schedules/desired-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert})
                        .header("X-Claimed-Sensor-Id", "evil-sensor")
                        .header("X-Claimed-Tenant-Id", UUID.randomUUID().toString()))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.generation").value(2))
                .andExpect(jsonPath("$.appliedGeneration").value(1))
                .andExpect(jsonPath("$.appliedChecksum").value("c".repeat(64)))
                .andExpect(jsonPath("$.definitions[0].assignmentId").value("assign-1"))
                .andExpect(jsonPath("$.definitions[0].snmp.target").value("127.0.0.1"));

        verify(assignments).getDesiredState(TENANT, SENSOR);
    }

    @Test
    void appliedStateConflictCodes() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));
        doThrow(new DomainException(
                SensorRecurringAssignmentService.ERR_STATE_STALE, "stale"))
                .when(assignments).ackAppliedState(eq(TENANT), eq(SENSOR), eq(1L), eq("a".repeat(64)));

        mockMvc.perform(put("/api/v1/sensor/recurring-schedules/applied-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert})
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"generation\":1,\"checksum\":\"" + "a".repeat(64) + "\"}"))
                .andExpect(status().isConflict())
                .andExpect(jsonPath("$.code").value("ERR_SENSOR_RECURRING_STATE_STALE"));
    }

    @Test
    void appliedStateChecksumMismatchConflict() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenReturn(new SensorScope(SENSOR, TENANT, null, Set.of(), true));
        doThrow(new DomainException(
                SensorRecurringAssignmentService.ERR_CHECKSUM_MISMATCH, "mismatch"))
                .when(assignments).ackAppliedState(eq(TENANT), eq(SENSOR), eq(1L), eq("b".repeat(64)));

        mockMvc.perform(put("/api/v1/sensor/recurring-schedules/applied-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert})
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"generation\":1,\"checksum\":\"" + "b".repeat(64) + "\"}"))
                .andExpect(status().isConflict())
                .andExpect(jsonPath("$.code").value("ERR_SENSOR_RECURRING_CHECKSUM_MISMATCH"));
    }

    @Test
    void authorityUnavailablePropagates() throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        when(resolver.resolve(cert)).thenThrow(new IngestionException(
                HttpStatus.FORBIDDEN, "ERR_SENSOR_AUTHORITY_UNAVAILABLE", "unavailable"));

        mockMvc.perform(get("/api/v1/sensor/recurring-schedules/desired-state")
                        .requestAttr(CERT_ATTR, new X509Certificate[]{cert}))
                .andExpect(status().isForbidden())
                .andExpect(jsonPath("$.code").value("ERR_SENSOR_AUTHORITY_UNAVAILABLE"));
    }
}
