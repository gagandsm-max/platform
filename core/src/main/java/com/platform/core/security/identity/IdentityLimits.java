package com.platform.core.security.identity;

/** Defensive implementation limits; these are not commercial or licensing limits. */
public final class IdentityLimits {
    public static final int PROVIDER_INSTANCE_ID_MAX_LENGTH = 128;
    /** Maximum opaque external-subject length in Unicode code points. */
    public static final int EXTERNAL_SUBJECT_MAX_LENGTH = 512;

    private IdentityLimits() {
    }
}
