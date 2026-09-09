package com.platform.core.security.http;

/**
 * Internal authentication/authorization failure mapped to bounded HTTP responses.
 * Messages must remain generic; deny reasons are for audit only.
 */
public final class UnauthorizedAccessException extends RuntimeException {
    private final String code;
    private final int httpStatus;

    public UnauthorizedAccessException(String code, String message) {
        this(code, message, 401);
    }

    public UnauthorizedAccessException(String code, String message, int httpStatus) {
        super(message);
        this.code = code;
        this.httpStatus = httpStatus;
    }

    public String code() {
        return code;
    }

    public int httpStatus() {
        return httpStatus;
    }
}
