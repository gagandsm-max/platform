package com.platform.core.security.authorization;

import java.util.Objects;
import java.util.regex.Pattern;

/**
 * Platform-owned permission identifier. Keys are canonical lowercase dot-separated values.
 */
public record Permission(String value) {
    public static final int MAX_LENGTH = 96;
    private static final Pattern CANONICAL = Pattern.compile(
            "[a-z][a-z0-9_-]*(?:\\.[a-z][a-z0-9_-]*)+"
    );

    public Permission {
        Objects.requireNonNull(value, "permission value must not be null");
        if (value.isBlank()) {
            throw new IllegalArgumentException("permission value must not be blank");
        }
        if (value.length() > MAX_LENGTH) {
            throw new IllegalArgumentException("permission value exceeds maximum length");
        }
        if (!CANONICAL.matcher(value).matches()) {
            throw new IllegalArgumentException("permission value must be canonical lowercase dot-separated text");
        }
    }

    public static Permission of(String value) {
        return new Permission(value);
    }

    @Override
    public String toString() {
        return value;
    }
}
