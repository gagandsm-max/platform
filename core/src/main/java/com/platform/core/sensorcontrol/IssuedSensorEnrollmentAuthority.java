package com.platform.core.sensorcontrol;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * One-time return of a newly issued enrollment bootstrap token.
 * Default rendering is REDACTED — never log or stringify the plaintext.
 */
public final class IssuedSensorEnrollmentAuthority {

    private final UUID authorityId;
    private final String plaintextToken;
    private final UUID tenantId;
    private final String deploymentId;
    private final UUID siteId;
    private final SensorRole role;
    private final Instant issuedAt;
    private final Instant expiresAt;

    public IssuedSensorEnrollmentAuthority(
            UUID authorityId,
            String plaintextToken,
            UUID tenantId,
            String deploymentId,
            UUID siteId,
            SensorRole role,
            Instant issuedAt,
            Instant expiresAt
    ) {
        this.authorityId = Objects.requireNonNull(authorityId, "authorityId");
        this.plaintextToken = Objects.requireNonNull(plaintextToken, "plaintextToken");
        this.tenantId = Objects.requireNonNull(tenantId, "tenantId");
        this.deploymentId = Objects.requireNonNull(deploymentId, "deploymentId");
        this.siteId = siteId;
        this.role = role;
        this.issuedAt = Objects.requireNonNull(issuedAt, "issuedAt");
        this.expiresAt = Objects.requireNonNull(expiresAt, "expiresAt");
        if (plaintextToken.isBlank()) {
            throw new IllegalArgumentException("plaintextToken must not be blank");
        }
    }

    public UUID authorityId() {
        return authorityId;
    }

    /** Plaintext bootstrap token — available once to the internal issuer caller. */
    public String plaintextToken() {
        return plaintextToken;
    }

    public UUID tenantId() {
        return tenantId;
    }

    public String deploymentId() {
        return deploymentId;
    }

    public UUID siteId() {
        return siteId;
    }

    public SensorRole role() {
        return role;
    }

    public Instant issuedAt() {
        return issuedAt;
    }

    public Instant expiresAt() {
        return expiresAt;
    }

    @Override
    public String toString() {
        return "IssuedSensorEnrollmentAuthority{authorityId=" + authorityId
                + ", tenantId=" + tenantId
                + ", deploymentId=" + deploymentId
                + ", token=[REDACTED]}";
    }
}
