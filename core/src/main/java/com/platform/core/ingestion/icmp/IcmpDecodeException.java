package com.platform.core.ingestion.icmp;

/**
 * Fail-closed decode/validation failure for the unwired ICMP typed contract.
 * Not an ingestion ACK path exception; production admission remains unchanged.
 */
public final class IcmpDecodeException extends RuntimeException {
    public enum Code {
        ERR_ICMP_UNSUPPORTED_SCHEMA,
        ERR_ICMP_MALFORMED_ENVELOPE,
        ERR_ICMP_MALFORMED_EVIDENCE,
        ERR_ICMP_INVALID_QUALITY,
        ERR_ICMP_INVALID_TARGET,
        ERR_ICMP_INVALID_BOUNDS,
        ERR_ICMP_INCONSISTENT_SAMPLES,
        ERR_ICMP_UNEXPECTED_FIELD
    }

    private final Code code;

    public IcmpDecodeException(Code code, String message) {
        super(message);
        this.code = code;
    }

    public IcmpDecodeException(Code code, String message, Throwable cause) {
        super(message, cause);
        this.code = code;
    }

    public Code getCode() {
        return code;
    }

    public String getCodeName() {
        return code.name();
    }
}
