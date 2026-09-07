package com.platform.core.security.audit;

/**
 * Defensive engineering bounds for the audit contract (not licensing limits).
 * {@link #METADATA_VALUE_MAX} and {@link #METADATA_TOTAL_UTF8_BYTES_MAX} are UTF-8 byte limits.
 */
public final class AuditLimits {
    public static final int CORRELATION_ID_MAX = 128;
    public static final int ACTION_MAX = 128;
    public static final int RESOURCE_TYPE_MAX = 64;
    public static final int REASON_CODE_MAX = 128;
    public static final int METADATA_MAX_ENTRIES = 32;
    public static final int METADATA_KEY_MAX = 64;
    /** Maximum UTF-8 byte length of one metadata value. */
    public static final int METADATA_VALUE_MAX = 512;
    public static final int METADATA_TOTAL_UTF8_BYTES_MAX = 8192;

    private AuditLimits() {}
}
