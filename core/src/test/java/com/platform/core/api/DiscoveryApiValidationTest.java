package com.platform.core.api;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.api.dto.DiscoveryDtos.CreateDiscoveryRequest;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.discovery.DiscoveryService;
import org.junit.jupiter.api.Test;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.util.List;
import java.util.UUID;

import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

class DiscoveryApiValidationTest {

    @Test
    void unsupportedDiscoveryMethodReturnsValidationErrorInsteadOfServerError() throws Exception {
        DiscoveryService service = mock(DiscoveryService.class);
        MockMvc mockMvc = MockMvcBuilders.standaloneSetup(new DiscoveryController(service))
                .setControllerAdvice(new ApiExceptionHandler())
                .build();
        CreateDiscoveryRequest request = new CreateDiscoveryRequest(
                "Invalid method", List.of("192.168.1.10"), List.of("SSH"),
                null, null, null, null, null, null, null, null);

        mockMvc.perform(post("/api/v1/tenants/" + UUID.fromString("018f0e0a-0000-7000-8000-000000000010") + "/discovery")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(new ObjectMapper().writeValueAsString(request)))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_FAILED"));

        verifyNoInteractions(service);
    }
}
