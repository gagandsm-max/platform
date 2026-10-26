package com.platform.core.ingestion.authority;

import java.util.Objects;
import java.util.UUID;

/**
 * Immutable server-derived Sensor identity observation.
 *
 * <p>Binds the resolved {@link SensorScope} to the matched enrolled credential identity
 * (credential id + DER fingerprint). This is an identity observation only — not proof of
 * future grant admission, issuance, or historical capture authority.
 */
public record ResolvedSensorAuthority(
        SensorScope scope,
        UUID credentialId,
        String fingerprintSha256
) {
    public ResolvedSensorAuthority {
        Objects.requireNonNull(scope, "scope must not be null");
        Objects.requireNonNull(credentialId, "credentialId must not be null");
        Objects.requireNonNull(fingerprintSha256, "fingerprintSha256 must not be null");
        if (fingerprintSha256.isBlank()) {
            throw new IllegalArgumentException("fingerprintSha256 must not be blank");
        }
    }
}
