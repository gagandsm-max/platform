package com.platform.core.api.error;

import com.platform.core.domain.DomainException;
import com.platform.core.logging.CorrelationIdFilter;
import com.platform.core.migration.MigrationException;
import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.dao.DataAccessException;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.http.converter.HttpMessageNotReadableException;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;

import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;

@RestControllerAdvice
public class ApiExceptionHandler {

    private static final Logger log = LoggerFactory.getLogger(ApiExceptionHandler.class);

    @ExceptionHandler(DomainException.class)
    public ResponseEntity<Map<String, Object>> handleDomain(DomainException ex, HttpServletRequest request) {
        HttpStatus status = mapStatus(ex.getCode());
        return ResponseEntity.status(status).body(errorBody(ex.getCode(), ex.getMessage(), ex.getFieldErrors()));
    }

    @ExceptionHandler(DuplicateKeyException.class)
    public ResponseEntity<Map<String, Object>> handleDuplicate(DuplicateKeyException ex) {
        return ResponseEntity.status(HttpStatus.CONFLICT)
                .body(errorBody("RESOURCE_CONFLICT", "Resource conflicts with an existing record", Map.of()));
    }

    @ExceptionHandler(HttpMessageNotReadableException.class)
    public ResponseEntity<Map<String, Object>> handleUnreadable(HttpMessageNotReadableException ex) {
        return ResponseEntity.badRequest()
                .body(errorBody("VALIDATION_FAILED", "Request body is malformed or unreadable", Map.of()));
    }

    @ExceptionHandler(DataAccessException.class)
    public ResponseEntity<Map<String, Object>> handleDataAccess(DataAccessException ex) {
        log.error("Database access failure", ex);
        return ResponseEntity.status(HttpStatus.SERVICE_UNAVAILABLE)
                .body(errorBody("DATABASE_UNAVAILABLE", "Database temporarily unavailable", Map.of()));
    }

    @ExceptionHandler(MigrationException.class)
    public ResponseEntity<Map<String, Object>> handleMigration(MigrationException ex) {
        return ResponseEntity.status(HttpStatus.SERVICE_UNAVAILABLE)
                .body(errorBody(ex.getCode(), ex.getMessage(), Map.of()));
    }

    @ExceptionHandler(Exception.class)
    public ResponseEntity<Map<String, Object>> handleGeneric(Exception ex) {
        log.error("Unhandled API failure", ex);
        return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                .body(errorBody("INTERNAL_ERROR", "Unexpected internal error", Map.of()));
    }

    private static Map<String, Object> errorBody(String code, String message, Map<String, String> fieldErrors) {
        Map<String, Object> body = new LinkedHashMap<>();
        body.put("code", code);
        body.put("message", message);
        body.put("correlationId", correlationId());
        body.put("timestamp", Instant.now().toString());
        if (fieldErrors != null && !fieldErrors.isEmpty()) {
            body.put("fieldErrors", fieldErrors);
        }
        return body;
    }

    private static String correlationId() {
        String fromMdc = MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY);
        return fromMdc == null ? "" : fromMdc;
    }

    private static HttpStatus mapStatus(String code) {
        return switch (code) {
            case "VALIDATION_FAILED", "QUERY_LIMIT_EXCEEDED" -> HttpStatus.BAD_REQUEST;
            case "TENANT_NOT_FOUND", "SITE_NOT_FOUND", "GROUP_NOT_FOUND", "TAG_NOT_FOUND",
                 "ENTITY_NOT_FOUND", "IDENTIFIER_NOT_FOUND" -> HttpStatus.NOT_FOUND;
            case "IDENTIFIER_CONFLICT", "RESOURCE_CONFLICT", "HIERARCHY_CYCLE", "CROSS_TENANT_REFERENCE" ->
                    HttpStatus.CONFLICT;
            case "DATABASE_UNAVAILABLE" -> HttpStatus.SERVICE_UNAVAILABLE;
            default -> HttpStatus.BAD_REQUEST;
        };
    }
}
