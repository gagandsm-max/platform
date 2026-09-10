package com.platform.core.security.identity;

import java.util.Optional;

/** Fail-closed default: no external identity is mapped until a directory is configured. */
public final class EmptyIdentityDirectory implements IdentityDirectory {
    @Override
    public Optional<MappedIdentity> find(ProviderIdentityKey key) {
        return Optional.empty();
    }
}
