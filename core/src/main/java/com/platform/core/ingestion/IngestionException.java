package com.platform.core.ingestion;

import org.springframework.http.HttpStatus;

public class IngestionException extends RuntimeException {
    private final HttpStatus status;
    private final String code;

    public IngestionException(HttpStatus status, String code, String message) {
        super(message);
        this.status = status;
        this.code = code;
    }

    public IngestionException(HttpStatus status, String code, String message, Throwable cause) {
        super(message, cause);
        this.status = status;
        this.code = code;
    }

    public HttpStatus getStatus() { return status; }
    public String getCode() { return code; }
}
