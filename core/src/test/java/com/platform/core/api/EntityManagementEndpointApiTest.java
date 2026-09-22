package com.platform.core.api;

import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.inventory.EntityManagementEndpoint;
import com.platform.core.domain.inventory.EntityManagementEndpointService;
import com.platform.core.domain.inventory.ManagementEndpointAddressType;
import com.platform.core.domain.inventory.ManagementEndpointSource;
import com.platform.core.domain.inventory.ManagementEndpointState;
import com.platform.core.logging.CorrelationIdFilter;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.MediaType;
import org.springframework.http.converter.json.MappingJackson2HttpMessageConverter;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.time.Instant;
import java.util.UUID;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyBoolean;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

/**
 * Public REST create is MANUAL-only. Unknown {@code source} properties fail closed
 * under Platform Jackson fail-on-unknown-properties.
 */
class EntityManagementEndpointApiTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");

    private MockMvc mockMvc;
    private EntityManagementEndpointService endpoints;

    @BeforeEach
    void setUp() {
        endpoints = mock(EntityManagementEndpointService.class);
        ObjectMapper mapper = new ObjectMapper().findAndRegisterModules();
        mapper.configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, true);
        MappingJackson2HttpMessageConverter converter = new MappingJackson2HttpMessageConverter(mapper);
        mockMvc = MockMvcBuilders.standaloneSetup(new EntityManagementEndpointController(endpoints))
                .addFilters(new CorrelationIdFilter())
                .setControllerAdvice(new ApiExceptionHandler())
                .setMessageConverters(converter)
                .build();
    }

    @Test
    void publicCreateIsAlwaysManual() throws Exception {
        Instant now = Instant.parse("2026-06-01T00:00:00Z");
        EntityManagementEndpoint created = new EntityManagementEndpoint(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000f1"),
                TENANT, ENTITY, ManagementEndpointAddressType.IPV4, "10.0.0.1",
                ManagementEndpointState.ACTIVE, ManagementEndpointSource.MANUAL, true,
                "actor", now, "actor", now);
        when(endpoints.createManual(eq(TENANT), eq(ENTITY), eq(ManagementEndpointAddressType.IPV4),
                eq("10.0.0.1"), eq(true))).thenReturn(created);

        mockMvc.perform(post("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/management-endpoints")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {"addressType":"IPV4","address":"10.0.0.1","primary":true}
                                """))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.source").value("MANUAL"))
                .andExpect(jsonPath("$.address").value("10.0.0.1"));

        verify(endpoints).createManual(TENANT, ENTITY, ManagementEndpointAddressType.IPV4, "10.0.0.1", true);
        verify(endpoints, never()).createFromApprovedSource(any(), any(), any(), any(), any(), anyBoolean());
    }

    @Test
    void publicCreateRejectsSourceDiscoveryProperty() throws Exception {
        mockMvc.perform(post("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/management-endpoints")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {"addressType":"IPV4","address":"10.0.0.1","primary":false,"source":"DISCOVERY"}
                                """))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));

        verify(endpoints, never()).createManual(any(), any(), any(), any(), anyBoolean());
        verify(endpoints, never()).createFromApprovedSource(any(), any(), any(), any(), any(), anyBoolean());
    }

    @Test
    void publicCreateRejectsSourceProvisioningProperty() throws Exception {
        mockMvc.perform(post("/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/management-endpoints")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {"addressType":"FQDN","address":"router01.example.com","primary":true,"source":"PROVISIONING"}
                                """))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));

        verify(endpoints, never()).createManual(any(), any(), any(), any(), anyBoolean());
        verify(endpoints, never()).createFromApprovedSource(any(), any(), any(), any(), any(), anyBoolean());
    }
}
