package com.platform.core.security.audit;

import java.nio.charset.StandardCharsets;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.regex.Pattern;

public final class AuditMetadata {
    private static final Pattern SAFE_KEY = Pattern.compile("[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*");
    private static final Set<String> FORBIDDEN = Set.of(
            "password", "passwd", "secret", "token", "access_token", "refresh_token", "id_token",
            "authorization", "cookie", "set_cookie", "private_key", "client_secret", "api_key", "credential_value"
    );
    private static final AuditMetadata EMPTY = new AuditMetadata(Map.of());
    private final Map<String, String> values;

    public AuditMetadata(Map<String, String> input) {
        Objects.requireNonNull(input, "input");
        if (input.size() > AuditLimits.METADATA_MAX_ENTRIES) {
            throw new IllegalArgumentException("audit metadata has too many entries");
        }
        LinkedHashMap<String, String> copy = new LinkedHashMap<>();
        int totalBytes = 0;
        for (Map.Entry<String, String> entry : input.entrySet()) {
            String key = Objects.requireNonNull(entry.getKey(), "metadata key");
            String value = Objects.requireNonNull(entry.getValue(), "metadata value");
            if (key.length() > AuditLimits.METADATA_KEY_MAX || !SAFE_KEY.matcher(key).matches()) {
                throw new IllegalArgumentException("audit metadata key is invalid");
            }
            if (isForbidden(key)) {
                throw new IllegalArgumentException("audit metadata key is forbidden");
            }
            if (value.length() > AuditLimits.METADATA_VALUE_MAX || containsControl(value)) {
                throw new IllegalArgumentException("audit metadata value is invalid");
            }
            totalBytes = Math.addExact(totalBytes, key.getBytes(StandardCharsets.UTF_8).length);
            totalBytes = Math.addExact(totalBytes, value.getBytes(StandardCharsets.UTF_8).length);
            if (totalBytes > AuditLimits.METADATA_TOTAL_UTF8_BYTES_MAX) {
                throw new IllegalArgumentException("audit metadata exceeds total size limit");
            }
            copy.put(key, value);
        }
        this.values = Collections.unmodifiableMap(copy);
    }

    public static AuditMetadata empty() { return EMPTY; }
    public Map<String, String> values() { return values; }
    public int size() { return values.size(); }

    private static boolean isForbidden(String key) {
        String normalized = key.replace('-', '_').replace('.', '_');
        for (String forbidden : FORBIDDEN) {
            if (normalized.equals(forbidden)
                    || normalized.startsWith(forbidden + "_")
                    || normalized.endsWith("_" + forbidden)
                    || normalized.contains("_" + forbidden + "_")) {
                return true;
            }
        }
        return false;
    }

    private static boolean containsControl(String value) {
        for (int i = 0; i < value.length(); i++) {
            char c = value.charAt(i);
            if (c <= 0x1f || c == 0x7f) return true;
        }
        return false;
    }

    @Override
    public String toString() { return "AuditMetadata[entries=" + values.size() + "]"; }
}
