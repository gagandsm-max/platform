package com.platform.core.security.identity;

import java.util.Objects;

/** Opaque stable subject issued by one configured authentication provider. */
public record ExternalSubject(String value) {
    public ExternalSubject {
        Objects.requireNonNull(value, "externalSubject");
        if (value.isBlank()) {
            throw new IllegalArgumentException("externalSubject must not be blank");
        }
        if (value.length() > IdentityLimits.EXTERNAL_SUBJECT_MAX_LENGTH * 2
                || value.codePointCount(0, value.length()) > IdentityLimits.EXTERNAL_SUBJECT_MAX_LENGTH) {
            throw new IllegalArgumentException("externalSubject exceeds maximum length");
        }
        if (isUnicodeWhitespace(value.codePointAt(0))
                || isUnicodeWhitespace(value.codePointBefore(value.length()))) {
            throw new IllegalArgumentException("externalSubject must not contain leading or trailing whitespace");
        }
        if (containsUnsafeCodePoint(value)) {
            throw new IllegalArgumentException("externalSubject contains unsafe Unicode characters");
        }
    }

    private static boolean isUnicodeWhitespace(int codePoint) {
        return Character.isWhitespace(codePoint) || Character.isSpaceChar(codePoint);
    }

    private static boolean containsUnsafeCodePoint(String value) {
        return value.codePoints().anyMatch(codePoint -> switch (Character.getType(codePoint)) {
            case Character.CONTROL,
                    Character.FORMAT,
                    Character.LINE_SEPARATOR,
                    Character.PARAGRAPH_SEPARATOR,
                    Character.SURROGATE -> true;
            default -> false;
        });
    }

    @Override
    public String toString() {
        return "<external-subject>";
    }
}
