package com.platform.core.security.identity;

import java.time.Instant;

/**
 * Provider-neutral mapping boundary from successful external authentication to a stable Platform subject.
 * Implementations are responsible for mapping/disabled-state persistence later; this interface performs no I/O itself.
 */
@FunctionalInterface
public interface IdentityResolver {
    ResolvedAuthenticatedIdentity resolve(AuthenticatedExternalIdentity identity, Instant evaluatedAt)
            throws IdentityResolutionException;
}
