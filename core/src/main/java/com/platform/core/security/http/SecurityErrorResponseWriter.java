package com.platform.core.security.http;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.logging.CorrelationIdFilter;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.slf4j.MDC;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;

import java.io.IOException;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;

@Component
public class SecurityErrorResponseWriter {

    private final ObjectMapper objectMapper;

    public SecurityErrorResponseWriter(ObjectMapper objectMapper) {
        this.objectMapper = objectMapper;
    }

    public void write(HttpServletResponse response, int status, String code, String message) throws IOException {
        response.setStatus(status);
        response.setContentType(MediaType.APPLICATION_JSON_VALUE);
        Map<String, Object> body = new LinkedHashMap<>();
        body.put("code", code);
        body.put("message", message);
        body.put("correlationId", correlationId());
        body.put("timestamp", Instant.now().toString());
        objectMapper.writeValue(response.getOutputStream(), body);
    }

    public void writeUnauthorized(HttpServletRequest request, HttpServletResponse response, String code) throws IOException {
        write(response, HttpServletResponse.SC_UNAUTHORIZED, code, "Authentication required");
    }

    public void writeNotFound(HttpServletResponse response) throws IOException {
        // Concealment: authentication-ok but authorization/tenant denials look like missing resources.
        write(response, HttpServletResponse.SC_NOT_FOUND, "RESOURCE_NOT_FOUND", "Resource not found");
    }

    private static String correlationId() {
        String fromMdc = MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY);
        return fromMdc == null ? "" : fromMdc;
    }
}
