package com.platform.core.security.audit;

import java.net.InetAddress;
import java.net.UnknownHostException;
import java.util.Objects;

/**
 * Non-authoritative network provenance only. Never actor or resource identity.
 * Accepts IP literals only (IPv4 / IPv6). Does not accept hostnames and does not perform DNS lookup.
 */
public record NetworkSourceAddress(String value) {
    public static final int MAX_LENGTH = 128;

    public NetworkSourceAddress {
        Objects.requireNonNull(value, "value");
        if (value.isBlank()
                || value.length() > MAX_LENGTH
                || !value.equals(value.strip())
                || containsUnsafe(value)
                || !isIpLiteralShape(value)) {
            throw new IllegalArgumentException("sourceAddress is invalid");
        }
        try {
            // For IP literals, InetAddress validates format without DNS resolution.
            InetAddress address = InetAddress.getByName(value);
            value = address.getHostAddress();
            if (value.length() > MAX_LENGTH) {
                throw new IllegalArgumentException("sourceAddress is invalid");
            }
        } catch (UnknownHostException ex) {
            throw new IllegalArgumentException("sourceAddress is invalid");
        }
    }

    /**
     * Reject hostnames before touching InetAddress so DNS cannot be triggered.
     */
    static boolean isIpLiteralShape(String candidate) {
        if (candidate.indexOf('%') >= 0) {
            return false; // zone identifiers out of scope
        }
        if (candidate.indexOf(':') >= 0) {
            for (int i = 0; i < candidate.length(); i++) {
                char c = candidate.charAt(i);
                boolean ok = (c >= '0' && c <= '9')
                        || (c >= 'a' && c <= 'f')
                        || (c >= 'A' && c <= 'F')
                        || c == ':'
                        || c == '.';
                if (!ok) {
                    return false;
                }
            }
            return true;
        }
        boolean hasDot = false;
        for (int i = 0; i < candidate.length(); i++) {
            char c = candidate.charAt(i);
            if (c == '.') {
                hasDot = true;
            } else if (c < '0' || c > '9') {
                return false;
            }
        }
        return hasDot;
    }

    private static boolean containsUnsafe(String value) {
        return value.codePoints().anyMatch(NetworkSourceAddress::isUnsafeCodePoint);
    }

    private static boolean isUnsafeCodePoint(int codePoint) {
        if (Character.isISOControl(codePoint)) {
            return true;
        }
        int type = Character.getType(codePoint);
        return type == Character.FORMAT
                || type == Character.LINE_SEPARATOR
                || type == Character.PARAGRAPH_SEPARATOR;
    }

    @Override
    public String toString() {
        return value;
    }
}
