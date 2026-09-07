package com.platform.core.security.identity;

import java.util.Objects;
import java.util.regex.Pattern;

/** Stable configuration identity for one configured authentication-provider instance. */
public record ProviderInstanceId(String value) {
    private static final Pattern SAFE = Pattern.compile("[a-z0-9]+(?:[._-][a-z0-9]+)*");

    public ProviderInstanceId {
        Objects.requireNonNull(value, "providerInstanceId");
        if (value.isBlank()) {
            throw new IllegalArgumentException("providerInstanceId must not be blank");
        }
        if (value.length() > IdentityLimits.PROVIDER_INSTANCE_ID_MAX_LENGTH) {
            throw new IllegalArgumentException("providerInstanceId exceeds maximum length");
        }
        if (!SAFE.matcher(value).matches()) {
            throw new IllegalArgumentException("providerInstanceId has invalid format");
        }
    }

    @Override
    public String toString() {
        return value;
    }
}
