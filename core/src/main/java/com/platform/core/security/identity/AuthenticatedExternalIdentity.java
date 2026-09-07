package com.platform.core.security.identity;

import java.time.Instant;
import java.util.Objects;

/**
 * Successful authentication evidence emitted by a provider adapter.
 * It intentionally carries no raw token, assertion, password, claim map or authorization state.
 */
public final class AuthenticatedExternalIdentity {
    private final IdentityProviderType providerType;
    private final ProviderInstanceId providerInstanceId;
    private final ExternalSubject externalSubject;
    private final Instant authenticatedAt;
    private final AuthenticationAssurance assurance;
    private final Instant expiresAt;

    public AuthenticatedExternalIdentity(
            IdentityProviderType providerType,
            ProviderInstanceId providerInstanceId,
            ExternalSubject externalSubject,
            Instant authenticatedAt,
            AuthenticationAssurance assurance,
            Instant expiresAt
    ) {
        this.providerType = Objects.requireNonNull(providerType, "providerType");
        this.providerInstanceId = Objects.requireNonNull(providerInstanceId, "providerInstanceId");
        this.externalSubject = Objects.requireNonNull(externalSubject, "externalSubject");
        this.authenticatedAt = Objects.requireNonNull(authenticatedAt, "authenticatedAt");
        this.assurance = Objects.requireNonNull(assurance, "assurance");
        if (expiresAt != null && expiresAt.isBefore(authenticatedAt)) {
            throw new IllegalArgumentException("expiresAt must not be before authenticatedAt");
        }
        this.expiresAt = expiresAt;
    }

    public IdentityProviderType providerType() {
        return providerType;
    }

    public ProviderInstanceId providerInstanceId() {
        return providerInstanceId;
    }

    public ExternalSubject externalSubject() {
        return externalSubject;
    }

    public Instant authenticatedAt() {
        return authenticatedAt;
    }

    public AuthenticationAssurance assurance() {
        return assurance;
    }

    public Instant expiresAt() {
        return expiresAt;
    }

    public ProviderIdentityKey providerIdentityKey() {
        return new ProviderIdentityKey(providerType, providerInstanceId, externalSubject);
    }

    /**
     * Deterministic freshness check using an explicit evaluation instant and an exclusive expiry boundary.
     * Provider clock-skew tolerance belongs in the adapter that constructs this evidence.
     */
    public boolean isCurrentAt(Instant evaluatedAt) {
        Objects.requireNonNull(evaluatedAt, "evaluatedAt");
        if (evaluatedAt.isBefore(authenticatedAt)) {
            return false;
        }
        return expiresAt == null || evaluatedAt.isBefore(expiresAt);
    }

    @Override
    public boolean equals(Object other) {
        if (this == other) {
            return true;
        }
        if (!(other instanceof AuthenticatedExternalIdentity that)) {
            return false;
        }
        return providerType == that.providerType
                && providerInstanceId.equals(that.providerInstanceId)
                && externalSubject.equals(that.externalSubject)
                && authenticatedAt.equals(that.authenticatedAt)
                && assurance.equals(that.assurance)
                && Objects.equals(expiresAt, that.expiresAt);
    }

    @Override
    public int hashCode() {
        return Objects.hash(providerType, providerInstanceId, externalSubject, authenticatedAt, assurance, expiresAt);
    }

    @Override
    public String toString() {
        return "AuthenticatedExternalIdentity[providerType=" + providerType
                + ", providerInstanceId=" + providerInstanceId
                + ", externalSubject=<external-subject>"
                + ", authenticatedAt=" + authenticatedAt
                + ", mfaSatisfied=" + assurance.mfaSatisfied()
                + ", expiresAt=" + expiresAt + "]";
    }
}
