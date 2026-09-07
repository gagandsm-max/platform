package com.platform.core.migration;

public class MigrationException extends RuntimeException {

    private final String code;

    public MigrationException(String code, String message) {
        super(message);
        this.code = code;
    }

    public MigrationException(String code, String message, Throwable cause) {
        super(message, cause);
        this.code = code;
    }

    public String getCode() {
        return code;
    }
}
