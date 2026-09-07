package com.platform.core.security.identity;

import java.time.Instant;
import java.util.Objects;

/** Stable Platform subject plus the provider-neutral authentication evidence that established it. */
public final class ResolvedAuthenticatedIdentity {
    private final PlatformSubject platformSubject;
    private final AuthenticatedExternalIdentity authenticationEvidence;

    private ResolvedAuthenticatedIdentity(
            PlatformSubject platformSubject,
            AuthenticatedExternalIdentity authenticationEvidence
    ) {
        this.platformSubject = platformSubject;
        this.authenticationEvidence = authenticationEvidence;
    }

    /** Creates a resolved identity only when the supplied authentication evidence is current at evaluatedAt. */
    public static ResolvedAuthenticatedIdentity current(
            PlatformSubject platformSubject,
            AuthenticatedExternalIdentity authenticationEvidence,
            Instant evaluatedAt
    ) {
        Objects.requireNonNull(platformSubject, "platformSubject");
        Objects.requireNonNull(authenticationEvidence, "authenticationEvidence");
        Objects.requireNonNull(evaluatedAt, "evaluatedAt");
        if (evaluatedAt.isBefore(authenticationEvidence.authenticatedAt())) {
            throw new IdentityResolutionException(IdentityResolutionFailure.INVALID_IDENTITY);
        }
        if (!authenticationEvidence.isCurrentAt(evaluatedAt)) {
            throw new IdentityResolutionException(IdentityResolutionFailure.AUTHENTICATION_EXPIRED);
        }
        return new ResolvedAuthenticatedIdentity(platformSubject, authenticationEvidence);
    }

    public PlatformSubject platformSubject() {
        return platformSubject;
    }

    public AuthenticatedExternalIdentity authenticationEvidence() {
        return authenticationEvidence;
    }

    public AuthenticationAssurance assurance() {
        return authenticationEvidence.assurance();
    }

    @Override
    public boolean equals(Object other) {
        if (this == other) {
            return true;
        }
        if (!(other instanceof ResolvedAuthenticatedIdentity that)) {
            return false;
        }
        return platformSubject.equals(that.platformSubject)
                && authenticationEvidence.equals(that.authenticationEvidence);
    }

    @Override
    public int hashCode() {
        return Objects.hash(platformSubject, authenticationEvidence);
    }

    @Override
    public String toString() {
        return "ResolvedAuthenticatedIdentity[platformSubject=" + platformSubject
                + ", providerType=" + authenticationEvidence.providerType()
                + ", providerInstanceId=" + authenticationEvidence.providerInstanceId()
                + ", mfaSatisfied=" + assurance().mfaSatisfied() + "]";
    }
}
