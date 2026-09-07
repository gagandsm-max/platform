package com.platform.core.security.identity;

/** Explicit authentication assurance established by the provider adapter. */
public record AuthenticationAssurance(boolean mfaSatisfied) {
    public static AuthenticationAssurance withoutVerifiedMfa() {
        return new AuthenticationAssurance(false);
    }

    public static AuthenticationAssurance withVerifiedMfa() {
        return new AuthenticationAssurance(true);
    }
}
