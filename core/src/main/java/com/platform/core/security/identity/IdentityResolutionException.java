package com.platform.core.security.identity;

import java.util.Objects;

/**
 * Resolution failure with a stable internal reason and deliberately generic message.
 *
 * <p>Unauthenticated protocol boundaries must not expose {@link #failure()} directly. They must map all
 * resolution failures to one external authentication failure while preserving the internal reason only for
 * access-controlled audit and operations.</p>
 */
public final class IdentityResolutionException extends RuntimeException {
    private static final long serialVersionUID = 1L;

    private final IdentityResolutionFailure failure;

    public IdentityResolutionException(IdentityResolutionFailure failure) {
        super("identity resolution failed");
        this.failure = Objects.requireNonNull(failure, "failure");
    }

    public IdentityResolutionFailure failure() {
        return failure;
    }
}
