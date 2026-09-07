package com.platform.core.security.identity;

import java.time.Instant;

/**
 * Provider-neutral mapping boundary from successful external authentication to a stable Platform subject.
 * Implementations are responsible for mapping/disabled-state persistence later; this interface performs no I/O itself.
 * Inputs and the evaluation time are required to be non-null. Provider-specific clock-skew policy is applied by the
 * adapter before constructing the authentication evidence, not by this resolver contract.
 */
@FunctionalInterface
public interface IdentityResolver {
    ResolvedAuthenticatedIdentity resolve(AuthenticatedExternalIdentity identity, Instant evaluatedAt)
            throws IdentityResolutionException;
}
