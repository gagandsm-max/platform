package com.platform.core.domain;

import java.time.DateTimeException;
import java.time.ZoneId;
import java.util.Map;
import java.util.regex.Pattern;

public final class TextRules {
    public static final int NAME_MAX = 255;
    public static final int DESCRIPTION_MAX = 2000;
    public static final int TYPE_MAX = 64;
    public static final int NAMESPACE_MAX = 128;
    public static final int IDENTIFIER_VALUE_MAX = 512;
    public static final int TAG_KEY_MAX = 128;
    public static final int TAG_VALUE_MAX = 512;
    public static final int TIMEZONE_MAX = 64;
    public static final int CREATED_BY_MAX = 128;
    public static final int DEVICE_PACK_ID_MAX = 128;
    public static final int DEVICE_PACK_VERSION_MAX = 64;
    public static final int SCHEMA_KEY_MAX = 128;
    public static final int SOURCE_MAX = 64;

    private static final Pattern CONTROL = Pattern.compile("[\\x00-\\x1F\\x7F]");

    private TextRules() {
    }

    public static String requireTrimmed(String value, String field, int max, boolean required) {
        if (value == null) {
            if (required) {
                throw new DomainException("VALIDATION_FAILED", field + " is required", Map.of(field, "required"));
            }
            return null;
        }
        String trimmed = value.trim();
        if (trimmed.isEmpty()) {
            if (required) {
                throw new DomainException("VALIDATION_FAILED", field + " must not be blank", Map.of(field, "blank"));
            }
            return null;
        }
        if (trimmed.length() > max) {
            throw new DomainException("VALIDATION_FAILED", field + " exceeds max length " + max, Map.of(field, "maxLength"));
        }
        if (CONTROL.matcher(trimmed).find()) {
            throw new DomainException("VALIDATION_FAILED", field + " contains control characters", Map.of(field, "controlChars"));
        }
        return trimmed;
    }

    public static String requireTimezone(String timezone) {
        String tz = requireTrimmed(timezone, "timezone", TIMEZONE_MAX, true);
        try {
            ZoneId.of(tz);
        } catch (DateTimeException ex) {
            throw new DomainException("VALIDATION_FAILED", "timezone must be a valid IANA ZoneId", Map.of("timezone", "invalid"));
        }
        return tz;
    }

    public static String optionalTimezone(String timezone) {
        if (timezone == null || timezone.isBlank()) {
            return null;
        }
        return requireTimezone(timezone);
    }
}
