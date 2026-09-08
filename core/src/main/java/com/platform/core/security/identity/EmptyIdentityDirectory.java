package com.platform.core.security.identity;

import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.stereotype.Component;

import java.util.Optional;

/** Fail-closed default: no external identity is mapped until a directory is configured. */
@Component
@ConditionalOnMissingBean(IdentityDirectory.class)
public final class EmptyIdentityDirectory implements IdentityDirectory {
    @Override
    public Optional<MappedIdentity> find(ProviderIdentityKey key) {
        return Optional.empty();
    }
}
