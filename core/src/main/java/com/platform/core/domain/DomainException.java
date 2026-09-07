package com.platform.core.domain;

import java.util.Map;

public class DomainException extends RuntimeException {

    private final String code;
    private final Map<String, String> fieldErrors;

    public DomainException(String code, String message) {
        this(code, message, Map.of());
    }

    public DomainException(String code, String message, Map<String, String> fieldErrors) {
        super(message);
        this.code = code;
        this.fieldErrors = fieldErrors == null ? Map.of() : Map.copyOf(fieldErrors);
    }

    public String getCode() {
        return code;
    }

    public Map<String, String> getFieldErrors() {
        return fieldErrors;
    }
}
