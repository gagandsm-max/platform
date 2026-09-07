package com.platform.core.security.identity;

import java.util.Objects;

/** Provider-scoped identity key; provider type, configured instance and opaque subject all participate in equality. */
public record ProviderIdentityKey(
        IdentityProviderType providerType,
        ProviderInstanceId providerInstanceId,
        ExternalSubject externalSubject
) {
    public ProviderIdentityKey {
        Objects.requireNonNull(providerType, "providerType");
        Objects.requireNonNull(providerInstanceId, "providerInstanceId");
        Objects.requireNonNull(externalSubject, "externalSubject");
    }
}
