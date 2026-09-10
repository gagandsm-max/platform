package com.platform.core.sensorcontrol;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Replaceable certificate credential binding (PostgreSQL {@code platform.sensor_certificate_credential}).
 * No private key material is stored.
 */
public record SensorCertificateCredential(
        UUID credentialId,
        UUID tenantId,
        String sensorId,
        String fingerprintSha256,
        String spkiSha256,
        String serialNumber,
        String issuerDn,
        String issuerFingerprintSha256,
        Instant notBefore,
        Instant notAfter,
        SensorCredentialStatus status,
        Instant activatedAt,
        Instant revokedAt,
        Instant createdAt,
        Instant updatedAt
) {
    public SensorCertificateCredential {
        Objects.requireNonNull(credentialId, "credentialId");
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(sensorId, "sensorId");
        Objects.requireNonNull(fingerprintSha256, "fingerprintSha256");
        Objects.requireNonNull(serialNumber, "serialNumber");
        Objects.requireNonNull(issuerDn, "issuerDn");
        Objects.requireNonNull(notBefore, "notBefore");
        Objects.requireNonNull(notAfter, "notAfter");
        Objects.requireNonNull(status, "status");
        Objects.requireNonNull(activatedAt, "activatedAt");
        Objects.requireNonNull(createdAt, "createdAt");
        Objects.requireNonNull(updatedAt, "updatedAt");
        if (!fingerprintSha256.matches("^[a-f0-9]{64}$")) {
            throw new IllegalArgumentException("fingerprintSha256 must be lowercase hex SHA-256");
        }
        if (!notAfter.isAfter(notBefore)) {
            throw new IllegalArgumentException("notAfter must be after notBefore");
        }
    }

    public boolean isValidAt(Instant now) {
        Objects.requireNonNull(now, "now");
        return status == SensorCredentialStatus.ACTIVE
                && !now.isBefore(notBefore)
                && !now.isAfter(notAfter);
    }
}
