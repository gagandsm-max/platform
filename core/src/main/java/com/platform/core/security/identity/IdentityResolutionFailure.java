package com.platform.core.security.identity;

/** Stable, non-sensitive outcomes for provider-identity resolution. */
public enum IdentityResolutionFailure {
    IDENTITY_NOT_MAPPED,
    IDENTITY_DISABLED,
    INVALID_IDENTITY,
    AUTHENTICATION_EXPIRED
}
