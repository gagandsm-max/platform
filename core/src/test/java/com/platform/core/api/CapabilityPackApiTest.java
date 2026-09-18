package com.platform.core.api;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.api.dto.CapabilityPackDtos;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.capabilitypack.CapabilityPackActivation;
import com.platform.core.domain.capabilitypack.CapabilityPackCatalogService;
import com.platform.core.domain.capabilitypack.CapabilityPackLoader;
import com.platform.core.domain.capabilitypack.CapabilityPackRegistrationResult;
import com.platform.core.domain.capabilitypack.CapabilityPackTestSupport;
import com.platform.core.domain.capabilitypack.StoredCapabilityPack;
import com.platform.core.logging.CorrelationIdFilter;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.time.Instant;
import java.util.Optional;
import java.util.UUID;

import static org.hamcrest.Matchers.not;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.delete;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.put;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

class CapabilityPackApiTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final String PACK_ID = "fixture.fictional.base";

    private MockMvc mockMvc;
    private CapabilityPackCatalogService catalog;
    private final ObjectMapper mapper = new ObjectMapper().findAndRegisterModules();

    @BeforeEach
    void setUp() {
        catalog = mock(CapabilityPackCatalogService.class);
        mockMvc = MockMvcBuilders.standaloneSetup(new CapabilityPackController(catalog))
                .addFilters(new CorrelationIdFilter())
                .setControllerAdvice(new ApiExceptionHandler())
                .build();
    }

    @AfterEach
    void tearDown() {
        // no-op; MockMvc does not retain security context
    }

    @Test
    void registerReturnsCreatedWithoutManifestBytes() throws Exception {
        byte[] body = CapabilityPackTestSupport.minimalManifestBytes(PACK_ID, "1.0.0", "Base");
        StoredCapabilityPack pack = samplePack(body);
        when(catalog.register(eq(TENANT), eq(PACK_ID), eq("1.0.0"), eq(body)))
                .thenReturn(new CapabilityPackRegistrationResult(pack, true));
        when(catalog.findActive(TENANT, PACK_ID)).thenReturn(Optional.empty());

        mockMvc.perform(put("/api/v1/tenants/" + TENANT + "/capability-packs/" + PACK_ID + "/versions/1.0.0")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(body))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.packId").value(PACK_ID))
                .andExpect(jsonPath("$.manifestSha256").value(pack.manifestSha256()))
                .andExpect(jsonPath("$.manifestBytes").doesNotExist())
                .andExpect(jsonPath("$.manifest_bytes").doesNotExist());
    }

    @Test
    void pathIdentityMismatchIsSanitized() throws Exception {
        byte[] body = CapabilityPackTestSupport.minimalManifestBytes(PACK_ID, "1.0.0", "Base");
        when(catalog.register(eq(TENANT), eq(PACK_ID), eq("1.0.0"), any()))
                .thenThrow(new DomainException(
                        CapabilityPackCatalogService.ERR_PATH_IDENTITY_MISMATCH,
                        "Path packId/packVersion must exactly match the manifest identity"));

        mockMvc.perform(put("/api/v1/tenants/" + TENANT + "/capability-packs/" + PACK_ID + "/versions/1.0.0")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(body))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value(CapabilityPackCatalogService.ERR_PATH_IDENTITY_MISMATCH))
                .andExpect(jsonPath("$.message", not(org.hamcrest.Matchers.containsString("BEGIN PRIVATE"))));
    }

    @Test
    void oversizedBodyRejectedBeforeService() throws Exception {
        byte[] oversized = new byte[CapabilityPackLoader.MAX_MANIFEST_BYTES + 1];
        mockMvc.perform(put("/api/v1/tenants/" + TENANT + "/capability-packs/" + PACK_ID + "/versions/1.0.0")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(oversized))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("MANIFEST_TOO_LARGE"));
    }

    @Test
    void paginationBounded() throws Exception {
        when(catalog.list(eq(TENANT), eq(1), eq(200)))
                .thenReturn(new PageResult<>(java.util.List.of(), 1, 200, 0, false));
        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/capability-packs")
                        .param("page", "1")
                        .param("limit", "200"))
                .andExpect(status().isOk());

        mockMvc.perform(get("/api/v1/tenants/" + TENANT + "/capability-packs")
                        .param("limit", "201"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("QUERY_LIMIT_EXCEEDED"));
    }

    @Test
    void activateAndDeactivateWireExpectedGeneration() throws Exception {
        CapabilityPackActivation active = new CapabilityPackActivation(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000f1"),
                TENANT,
                PACK_ID,
                "1.0.0",
                1L,
                CapabilityPackActivation.STATUS_ACTIVE,
                "actor",
                Instant.parse("2026-01-01T00:00:00Z"),
                null,
                null,
                "corr"
        );
        when(catalog.activate(TENANT, PACK_ID, "1.0.0", 0L)).thenReturn(active);
        when(catalog.deactivate(TENANT, PACK_ID, 1L)).thenReturn(active);

        mockMvc.perform(put("/api/v1/tenants/" + TENANT + "/capability-packs/" + PACK_ID + "/active-version")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(mapper.writeValueAsString(new CapabilityPackDtos.ActivateRequest("1.0.0", 0L))))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.generation").value(1))
                .andExpect(jsonPath("$.packVersion").value("1.0.0"));

        mockMvc.perform(delete("/api/v1/tenants/" + TENANT + "/capability-packs/" + PACK_ID + "/active-version")
                        .param("expectedGeneration", "1"))
                .andExpect(status().isNoContent());
        verify(catalog).deactivate(TENANT, PACK_ID, 1L);
    }

    @Test
    void activateRejectsMissingExpectedGenerationWithoutInvokingCatalog() throws Exception {
        mockMvc.perform(put("/api/v1/tenants/" + TENANT + "/capability-packs/" + PACK_ID + "/active-version")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"packVersion\":\"1.0.0\"}"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));
        verify(catalog, never()).activate(any(), anyString(), anyString(), anyLong());
    }

    @Test
    void activateAcceptsExplicitZeroExpectedGeneration() throws Exception {
        CapabilityPackActivation active = new CapabilityPackActivation(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000f2"),
                TENANT,
                PACK_ID,
                "1.0.0",
                1L,
                CapabilityPackActivation.STATUS_ACTIVE,
                "actor",
                Instant.parse("2026-01-01T00:00:00Z"),
                null,
                null,
                "corr"
        );
        when(catalog.activate(TENANT, PACK_ID, "1.0.0", 0L)).thenReturn(active);

        mockMvc.perform(put("/api/v1/tenants/" + TENANT + "/capability-packs/" + PACK_ID + "/active-version")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"packVersion\":\"1.0.0\",\"expectedGeneration\":0}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.generation").value(1));
        verify(catalog).activate(TENANT, PACK_ID, "1.0.0", 0L);
    }

    @Test
    void activateRejectsExplicitNullExpectedGenerationWithoutInvokingCatalog() throws Exception {
        mockMvc.perform(put("/api/v1/tenants/" + TENANT + "/capability-packs/" + PACK_ID + "/active-version")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"packVersion\":\"1.0.0\",\"expectedGeneration\":null}"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));
        verify(catalog, never()).activate(any(), anyString(), anyString(), anyLong());
    }

    @Test
    void activateRejectsNegativeExpectedGenerationWithoutMutation() throws Exception {
        when(catalog.activate(TENANT, PACK_ID, "1.0.0", -1L))
                .thenThrow(new DomainException("VALIDATION_FAILED", "expectedGeneration must be >= 0"));

        mockMvc.perform(put("/api/v1/tenants/" + TENANT + "/capability-packs/" + PACK_ID + "/active-version")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"packVersion\":\"1.0.0\",\"expectedGeneration\":-1}"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));
    }

    @Test
    void malformedTenantUuidRejectedSafely() throws Exception {
        mockMvc.perform(get("/api/v1/tenants/not-a-uuid/capability-packs"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));
    }

    @Test
    void unsupportedMethodDeniedByMvc() throws Exception {
        mockMvc.perform(post("/api/v1/tenants/" + TENANT + "/capability-packs"))
                .andExpect(status().isMethodNotAllowed());
    }

    private static StoredCapabilityPack samplePack(byte[] body) {
        return new StoredCapabilityPack(
                TENANT,
                PACK_ID,
                "1.0.0",
                "1.0.0",
                "DEVICE_NETWORK",
                "Fictional Test Pack",
                "CUSTOMER",
                "Fictional Publisher",
                "EXPERIMENTAL",
                CapabilityPackTestSupport.sha256(body),
                body,
                "actor",
                Instant.parse("2026-01-01T00:00:00Z")
        );
    }
}
