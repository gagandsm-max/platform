package com.platform.core.security.audit;

import java.util.Objects;

/** Non-authoritative network provenance only; this value is never actor or resource identity. */
public record NetworkSourceAddress(String value) {
    public static final int MAX_LENGTH = 128;

    public NetworkSourceAddress {
        Objects.requireNonNull(value, "value");
        if (value.isBlank() || value.length() > MAX_LENGTH || !value.equals(value.strip()) || containsControl(value)) {
            throw new IllegalArgumentException("sourceAddress is invalid");
        }
    }

    private static boolean containsControl(String value) {
        for (int i = 0; i < value.length(); i++) {
            char c = value.charAt(i);
            if (c <= 0x1f || c == 0x7f) return true;
        }
        return false;
    }

    @Override
    public String toString() { return value; }
}
