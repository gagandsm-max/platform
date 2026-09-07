package com.platform.core.security.audit;

import java.nio.charset.StandardCharsets;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.regex.Pattern;

/**
 * Bounded diagnostic metadata for an audit fact.
 *
 * <p>This contract rejects known secret-bearing field <em>names</em> (and nested separator variants).
 * It cannot reliably detect secrets embedded in arbitrary values (e.g. {@code note=password=...}).
 * Emitters remain responsible for never supplying secrets, tokens, credentials, or raw payloads.
 */
public final class AuditMetadata {
    private static final Pattern SAFE_KEY = Pattern.compile("[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*");
    private static final Set<String> FORBIDDEN = Set.of(
            "password", "passwd", "secret", "token", "access_token", "accesstoken",
            "refresh_token", "refreshtoken", "id_token", "idtoken",
            "authorization", "cookie", "set_cookie", "setcookie",
            "private_key", "privatekey", "client_secret", "clientsecret",
            "api_key", "apikey", "credential_value", "credentialvalue"
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
            int valueBytes = value.getBytes(StandardCharsets.UTF_8).length;
            if (valueBytes > AuditLimits.METADATA_VALUE_MAX || containsUnsafeText(value)) {
                throw new IllegalArgumentException("audit metadata value is invalid");
            }
            totalBytes = Math.addExact(totalBytes, key.getBytes(StandardCharsets.UTF_8).length);
            totalBytes = Math.addExact(totalBytes, valueBytes);
            if (totalBytes > AuditLimits.METADATA_TOTAL_UTF8_BYTES_MAX) {
                throw new IllegalArgumentException("audit metadata exceeds total size limit");
            }
            copy.put(key, value);
        }
        this.values = Collections.unmodifiableMap(copy);
    }

    public static AuditMetadata empty() {
        return EMPTY;
    }

    public Map<String, String> values() {
        return values;
    }

    public int size() {
        return values.size();
    }

    /**
     * Forbidden classification after normalizing separators to '_'.
     * {@code auth.token}/{@code my_token} are rejected; innocent {@code token_count} is allowed
     * (does not use startsWith("token_") alone).
     */
    static boolean isForbidden(String key) {
        String normalized = key.replace('-', '_').replace('.', '_');
        for (String forbidden : FORBIDDEN) {
            if (normalized.equals(forbidden)) {
                return true;
            }
            if (forbidden.equals("token")) {
                // Exact handled above. Allow token_count; reject *.token nesting.
                if (normalized.endsWith("_" + forbidden) || normalized.contains("_" + forbidden + "_")) {
                    return true;
                }
                continue;
            }
            if (normalized.startsWith(forbidden + "_")
                    || normalized.endsWith("_" + forbidden)
                    || normalized.contains("_" + forbidden + "_")) {
                return true;
            }
        }
        return false;
    }

    private static boolean containsUnsafeText(String value) {
        return value.codePoints().anyMatch(AuditMetadata::isUnsafeCodePoint);
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
        return "AuditMetadata[entries=" + values.size() + "]";
    }
}
