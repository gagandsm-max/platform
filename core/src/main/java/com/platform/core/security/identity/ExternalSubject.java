package com.platform.core.security.identity;

import java.util.Objects;

/** Opaque stable subject issued by one configured authentication provider. */
public record ExternalSubject(String value) {
    public ExternalSubject {
        Objects.requireNonNull(value, "externalSubject");
        if (value.isBlank()) {
            throw new IllegalArgumentException("externalSubject must not be blank");
        }
        if (!value.equals(value.strip())) {
            throw new IllegalArgumentException("externalSubject must not contain leading or trailing whitespace");
        }
        if (value.length() > IdentityLimits.EXTERNAL_SUBJECT_MAX_LENGTH) {
            throw new IllegalArgumentException("externalSubject exceeds maximum length");
        }
        if (containsControlCharacter(value)) {
            throw new IllegalArgumentException("externalSubject contains control characters");
        }
    }

    private static boolean containsControlCharacter(String value) {
        return value.codePoints().anyMatch(Character::isISOControl);
    }

    @Override
    public String toString() {
        return "<external-subject>";
    }
}
