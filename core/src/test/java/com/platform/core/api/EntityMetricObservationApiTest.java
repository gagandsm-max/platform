package com.platform.core.api;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.DomainException;
import com.platform.core.logging.CorrelationIdFilter;
import com.platform.core.telemetry.read.AllowedMetricSelector;
import com.platform.core.telemetry.read.EntityMetricObservationService;
import com.platform.core.telemetry.read.EntityMetricObservationService.EntityMetricObservationResult;
import com.platform.core.telemetry.read.EntityMetricObservationService.MetricObservationProvenance;
import com.platform.core.telemetry.read.FreshnessClass;
import com.platform.core.telemetry.read.MetricObservationQuery;
import com.platform.core.telemetry.read.MetricObservationRow;
import com.platform.core.telemetry.read.ObservationReadState;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.format.datetime.standard.DateTimeFormatterRegistrar;
import org.springframework.format.support.DefaultFormattingConversionService;
import org.springframework.http.converter.json.MappingJackson2HttpMessageConverter;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.time.Instant;
import java.util.List;
import java.util.UUID;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

class EntityMetricObservationApiTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");

    private MockMvc mockMvc;
    private EntityMetricObservationService service;

    @BeforeEach
    void setUp() {
        service = mock(EntityMetricObservationService.class);
        ObjectMapper mapper = new ObjectMapper().findAndRegisterModules();
        MappingJackson2HttpMessageConverter converter = new MappingJackson2HttpMessageConverter(mapper);
        DefaultFormattingConversionService conversionService = new DefaultFormattingConversionService();
        DateTimeFormatterRegistrar registrar = new DateTimeFormatterRegistrar();
        registrar.setUseIsoFormat(true);
        registrar.registerFormatters(conversionService);
        mockMvc = MockMvcBuilders.standaloneSetup(new EntityMetricObservationController(service))
                .addFilters(new CorrelationIdFilter())
                .setControllerAdvice(new ApiExceptionHandler())
                .setConversionService(conversionService)
                .setMessageConverters(converter)
                .build();
    }

    @Test
    void acceptedAuthorizedEntityReturnsObservationsWithCounter64String() throws Exception {
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, ".1.3.6.1.2.1.31.1.1.1.6.1", "COUNTER64",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                10
        );
        String huge = "18446744073709551615";
        when(service.read(eq(query), any(Instant.class))).thenReturn(new EntityMetricObservationResult(
                query,
                ObservationReadState.OK,
                FreshnessClass.FRESH,
                List.of(new MetricObservationRow(
                        Instant.parse("2026-10-16T00:30:00Z"), null, huge, 1, 0, "s1", "seq-1", "snmp")),
                false,
                new MetricObservationProvenance(
                        "platform.metric_counter",
                        "ts DESC, sensor_id DESC, sequence_id DESC then present ts ASC, sensor_id ASC, sequence_id ASC",
                        "clickhouse-canonical-final-view"),
                "Bounded observations returned from canonical view."
        ));

        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/metric-observations")
                        .param("metricId", ".1.3.6.1.2.1.31.1.1.1.6.1")
                        .param("valueKind", "COUNTER64")
                        .param("from", "2026-10-16T00:00:00Z")
                        .param("to", "2026-10-16T01:00:00Z")
                        .param("limit", "10"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.metricId").value(".1.3.6.1.2.1.31.1.1.1.6.1"))
                .andExpect(jsonPath("$.valueKind").value("COUNTER64"))
                .andExpect(jsonPath("$.observationState").value("OK"))
                .andExpect(jsonPath("$.truncated").value(false))
                .andExpect(jsonPath("$.observations[0].counterValue").value(huge))
                .andExpect(jsonPath("$.observations[0].value").doesNotExist())
                .andExpect(jsonPath("$.provenance.sourceView").value("platform.metric_counter"));
    }

    @Test
    void agentCpuKeyAcceptedAtApiBoundary() throws Exception {
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, AllowedMetricSelector.AGENT_CPU_UTILIZATION, "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                10
        );
        when(service.read(eq(query), any(Instant.class))).thenReturn(new EntityMetricObservationResult(
                query,
                ObservationReadState.OK,
                FreshnessClass.FRESH,
                List.of(new MetricObservationRow(
                        Instant.parse("2026-10-16T00:30:00Z"), 12.5, null, 1, 0, "agent-1", "seq-1", "agent")),
                true,
                new MetricObservationProvenance(
                        "platform.metric_numeric",
                        "ts DESC, sensor_id DESC, sequence_id DESC then present ts ASC, sensor_id ASC, sequence_id ASC",
                        "clickhouse-canonical-final-view"),
                "Bounded observations returned from canonical view. Result truncated to latest 10 observations."
        ));

        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/metric-observations")
                        .param("metricId", "system.cpu.utilization")
                        .param("valueKind", "NUMERIC")
                        .param("from", "2026-10-16T00:00:00Z")
                        .param("to", "2026-10-16T01:00:00Z")
                        .param("limit", "10"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.metricId").value("system.cpu.utilization"))
                .andExpect(jsonPath("$.truncated").value(true))
                .andExpect(jsonPath("$.observations[0].value").value(12.5));
    }

    @Test
    void malformedBoundsReturnValidationFailed() throws Exception {
        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/metric-observations")
                        .param("metricId", "system.cpu.utilization")
                        .param("valueKind", "NUMERIC")
                        .param("from", "2026-10-16T02:00:00Z")
                        .param("to", "2026-10-16T01:00:00Z"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));
    }

    @Test
    void unknownMetricSelectorRejected() throws Exception {
        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/metric-observations")
                        .param("metricId", "not.allowlisted")
                        .param("valueKind", "NUMERIC")
                        .param("from", "2026-10-16T00:00:00Z")
                        .param("to", "2026-10-16T01:00:00Z"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));
    }

    @Test
    void oidLookalikeRejectedAtApiBoundary() throws Exception {
        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/metric-observations")
                        .param("metricId", ".1.3.6.1.2.1.2.2.1.81.1")
                        .param("valueKind", "NUMERIC")
                        .param("from", "2026-10-16T00:00:00Z")
                        .param("to", "2026-10-16T01:00:00Z"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));
    }

    @Test
    void missingEntityReturns404() throws Exception {
        when(service.read(any(), any())).thenThrow(new DomainException("ENTITY_NOT_FOUND", "Entity not found"));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/metric-observations")
                        .param("metricId", AllowedMetricSelector.AGENT_CPU_UTILIZATION)
                        .param("valueKind", "NUMERIC")
                        .param("from", "2026-10-16T00:00:00Z")
                        .param("to", "2026-10-16T01:00:00Z"))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("ENTITY_NOT_FOUND"));
    }

    @Test
    void clickHouseOfflineReturns503Sanitized() throws Exception {
        when(service.read(any(), any()))
                .thenThrow(new DomainException("DATABASE_UNAVAILABLE", "Telemetry store temporarily unavailable"));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/metric-observations")
                        .param("metricId", "system.cpu.utilization")
                        .param("valueKind", "NUMERIC")
                        .param("from", "2026-10-16T00:00:00Z")
                        .param("to", "2026-10-16T01:00:00Z"))
                .andExpect(status().isServiceUnavailable())
                .andExpect(jsonPath("$.code").value("DATABASE_UNAVAILABLE"))
                .andExpect(jsonPath("$.message").value("Telemetry store temporarily unavailable"));
    }

    @Test
    void malformedBackendPayloadReturns500DataIntegrity() throws Exception {
        when(service.read(any(), any()))
                .thenThrow(new DomainException(
                        "DATA_INTEGRITY_ERROR",
                        "Telemetry store returned malformed observation data"));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/metric-observations")
                        .param("metricId", "system.cpu.utilization")
                        .param("valueKind", "NUMERIC")
                        .param("from", "2026-10-16T00:00:00Z")
                        .param("to", "2026-10-16T01:00:00Z"))
                .andExpect(status().isInternalServerError())
                .andExpect(jsonPath("$.code").value("DATA_INTEGRITY_ERROR"))
                .andExpect(jsonPath("$.message").value("Telemetry store returned malformed observation data"));
    }

    @Test
    void limitAboveCapRejected() throws Exception {
        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/metric-observations")
                        .param("metricId", "system.cpu.utilization")
                        .param("valueKind", "NUMERIC")
                        .param("from", "2026-10-16T00:00:00Z")
                        .param("to", "2026-10-16T01:00:00Z")
                        .param("limit", "501"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("QUERY_LIMIT_EXCEEDED"));
    }
}
