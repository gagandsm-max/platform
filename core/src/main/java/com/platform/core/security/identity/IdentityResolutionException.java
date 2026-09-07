package com.platform.core.security.identity;

import java.util.Objects;

/** Resolution failure with a stable code and deliberately generic message. */
public final class IdentityResolutionException extends RuntimeException {
    private static final long serialVersionUID = 1L;

    private final IdentityResolutionFailure failure;

    public IdentityResolutionException(IdentityResolutionFailure failure) {
        super(messageFor(Objects.requireNonNull(failure, "failure")));
        this.failure = failure;
    }

    public IdentityResolutionFailure failure() {
        return failure;
    }

    private static String messageFor(IdentityResolutionFailure failure) {
        return switch (failure) {
            case IDENTITY_NOT_MAPPED -> "identity is not mapped";
            case IDENTITY_DISABLED -> "identity is disabled";
            case INVALID_IDENTITY -> "identity is invalid";
            case AUTHENTICATION_EXPIRED -> "authentication evidence is expired";
        };
    }
}
