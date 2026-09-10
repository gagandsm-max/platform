package com.platform.core.security.identity;

import java.util.Optional;

/**
 * Maps a provider identity key to a stable Platform subject.
 * Production deployments must supply a persistence-backed directory; the default is empty (fail closed).
 */
public interface IdentityDirectory {

    Optional<MappedIdentity> find(ProviderIdentityKey key);

    record MappedIdentity(PlatformSubject platformSubject, boolean disabled) {
    }
}
