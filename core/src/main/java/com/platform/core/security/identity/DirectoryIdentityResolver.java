package com.platform.core.security.identity;

import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.Objects;

/**
 * Provider-neutral resolver backed by {@link IdentityDirectory}.
 * Does not consult JWT permission/role claims.
 */
@Component
public final class DirectoryIdentityResolver implements IdentityResolver {

    private final IdentityDirectory directory;

    public DirectoryIdentityResolver(IdentityDirectory directory) {
        this.directory = Objects.requireNonNull(directory, "directory");
    }

    @Override
    public ResolvedAuthenticatedIdentity resolve(AuthenticatedExternalIdentity identity, Instant evaluatedAt)
            throws IdentityResolutionException {
        Objects.requireNonNull(identity, "identity");
        Objects.requireNonNull(evaluatedAt, "evaluatedAt");
        if (!identity.isCurrentAt(evaluatedAt)) {
            throw new IdentityResolutionException(IdentityResolutionFailure.AUTHENTICATION_EXPIRED);
        }
        IdentityDirectory.MappedIdentity mapped = directory.find(identity.providerIdentityKey())
                .orElseThrow(() -> new IdentityResolutionException(IdentityResolutionFailure.IDENTITY_NOT_MAPPED));
        if (mapped.disabled()) {
            throw new IdentityResolutionException(IdentityResolutionFailure.IDENTITY_DISABLED);
        }
        return ResolvedAuthenticatedIdentity.current(mapped.platformSubject(), identity, evaluatedAt);
    }
}
