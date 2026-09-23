package com.platform.core.sensorcontrol;

import com.platform.core.id.UuidV7;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.http.SecurityAuditEmitter;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.security.SecureRandom;
import java.time.Duration;
import java.time.Instant;
import java.util.Base64;
import java.util.HexFormat;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;

/**
 * Internal Sensor enrollment bootstrap-token authority (B1).
 * No public REST — future enrollment orchestration consumes this service.
 */
@Service
public class SensorEnrollmentAuthorityService {

    /** Hard maximum TTL for B1 short-lived enrollment authorities. */
    public static final Duration MAX_TTL = Duration.ofHours(24);

    private static final int TOKEN_ENTROPY_BYTES = 32;
    private static final int MAX_TOKEN_CHARS = 512;
    private static final int MAX_DEPLOYMENT_ID_CHARS = 128;
    private static final int MAX_DIGEST_INSERT_RETRIES = 3;

    private final SensorEnrollmentAuthorityRepository authorities;
    private final SensorControlRepository sensors;
    private final SecurityAuditEmitter auditEmitter;
    private final SecureRandom secureRandom;

    public SensorEnrollmentAuthorityService(
            SensorEnrollmentAuthorityRepository authorities,
            SensorControlRepository sensors,
            SecurityAuditEmitter auditEmitter
    ) {
        this(authorities, sensors, auditEmitter, new SecureRandom());
    }

    SensorEnrollmentAuthorityService(
            SensorEnrollmentAuthorityRepository authorities,
            SensorControlRepository sensors,
            SecurityAuditEmitter auditEmitter,
            SecureRandom secureRandom
    ) {
        this.authorities = Objects.requireNonNull(authorities, "authorities");
        this.sensors = Objects.requireNonNull(sensors, "sensors");
        this.auditEmitter = Objects.requireNonNull(auditEmitter, "auditEmitter");
        this.secureRandom = Objects.requireNonNull(secureRandom, "secureRandom");
    }

    @Transactional("transactionManager")
    public IssuedSensorEnrollmentAuthority issue(
            UUID tenantId,
            String deploymentId,
            UUID siteId,
            SensorRole role,
            Duration ttl
    ) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(ttl, "ttl");

        String deployment = requireDeploymentId(deploymentId);
        if (ttl.isZero() || ttl.isNegative() || ttl.compareTo(MAX_TTL) > 0) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_TTL_INVALID,
                    "Enrollment authority TTL must be positive and at most 24 hours");
        }
        if (!sensors.tenantExists(tenantId)) {
            throw new SensorControlException("TENANT_NOT_FOUND", "Tenant not found");
        }
        if (siteId != null && !sensors.siteExists(tenantId, siteId)) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_SCOPE_INVALID,
                    "Site does not exist for tenant");
        }

        Instant now = Instant.now();
        Instant expiresAt = now.plus(ttl);
        for (int attempt = 0; attempt < MAX_DIGEST_INSERT_RETRIES; attempt++) {
            String plaintext = generateToken();
            String digest = sha256Hex(plaintext);
            UUID authorityId = UuidV7.generate();
            SensorEnrollmentAuthority row = new SensorEnrollmentAuthority(
                    authorityId,
                    digest,
                    tenantId,
                    deployment,
                    siteId,
                    role,
                    now,
                    expiresAt,
                    null,
                    null,
                    now,
                    now
            );
            try {
                authorities.insert(row);
            } catch (DuplicateKeyException ex) {
                continue;
            }
            auditIssue(row);
            return new IssuedSensorEnrollmentAuthority(
                    authorityId, plaintext, tenantId, deployment, siteId, role, now, expiresAt);
        }
        throw new SensorControlException(
                SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID,
                "Unable to issue enrollment authority");
    }

    /**
     * Atomically consume a presented bootstrap token for the future enrollment orchestrator.
     * All failure modes map to {@link SensorControlException#ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID}.
     */
    @Transactional("transactionManager")
    public ConsumedSensorEnrollmentAuthority consume(
            String presentedToken,
            SensorRole requestedRole,
            UUID requestedSiteId
    ) {
        if (presentedToken == null || presentedToken.isBlank() || presentedToken.length() > MAX_TOKEN_CHARS) {
            throw invalidAuthority();
        }
        Objects.requireNonNull(requestedRole, "requestedRole");

        String digest = sha256Hex(presentedToken);
        SensorEnrollmentAuthority authority = authorities.findByTokenHash(digest)
                .orElseThrow(this::invalidAuthority);

        Instant now = Instant.now();
        if (authority.isExpired(now) || authority.isRevoked() || authority.isConsumed()) {
            throw invalidAuthority();
        }
        if (authority.role() != null && authority.role() != requestedRole) {
            throw invalidAuthority();
        }
        if (authority.siteId() != null) {
            if (requestedSiteId == null || !authority.siteId().equals(requestedSiteId)) {
                throw invalidAuthority();
            }
        }
        if (requestedSiteId != null && !sensors.siteExists(authority.tenantId(), requestedSiteId)) {
            throw invalidAuthority();
        }

        if (!authorities.tryConsume(authority.authorityId(), now)) {
            throw invalidAuthority();
        }

        UUID effectiveSite = authority.siteId() != null ? authority.siteId() : requestedSiteId;
        return new ConsumedSensorEnrollmentAuthority(
                authority.authorityId(),
                authority.tenantId(),
                authority.deploymentId(),
                effectiveSite,
                requestedRole,
                authority.expiresAt(),
                now
        );
    }

    @Transactional("transactionManager")
    public void revoke(UUID authorityId) {
        Objects.requireNonNull(authorityId, "authorityId");
        SensorEnrollmentAuthority authority = authorities.findById(authorityId)
                .orElseThrow(() -> new SensorControlException(
                        SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID,
                        "Enrollment authority not found"));

        if (authority.isConsumed()) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID,
                    "Consumed enrollment authority cannot be revoked");
        }
        if (authority.isRevoked()) {
            // Deterministic no-op success for already-revoked authorities.
            return;
        }

        Instant now = Instant.now();
        if (!authorities.tryRevoke(authorityId, now)) {
            // Concurrent consume/revoke race — fail closed without restoring.
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID,
                    "Enrollment authority cannot be revoked");
        }
        auditRevoke(authority);
    }

    private void auditIssue(SensorEnrollmentAuthority authority) {
        Map<String, String> metadata = new LinkedHashMap<>();
        metadata.put("authority_id", authority.authorityId().toString());
        metadata.put("deployment_id", authority.deploymentId());
        if (authority.siteId() != null) {
            metadata.put("site_id", authority.siteId().toString());
        }
        if (authority.role() != null) {
            metadata.put("role", authority.role().name());
        }
        metadata.put("expires_at", authority.expiresAt().toString());
        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.SENSOR_ENROLLMENT_AUTHORITY_ISSUE,
                AuditOutcome.SUCCESS,
                authority.tenantId(),
                "SENSOR_ENROLLMENT_AUTHORITY_ISSUED",
                metadata
        );
    }

    private void auditRevoke(SensorEnrollmentAuthority authority) {
        Map<String, String> metadata = new LinkedHashMap<>();
        metadata.put("authority_id", authority.authorityId().toString());
        metadata.put("deployment_id", authority.deploymentId());
        if (authority.siteId() != null) {
            metadata.put("site_id", authority.siteId().toString());
        }
        if (authority.role() != null) {
            metadata.put("role", authority.role().name());
        }
        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.SENSOR_ENROLLMENT_AUTHORITY_REVOKE,
                AuditOutcome.SUCCESS,
                authority.tenantId(),
                "SENSOR_ENROLLMENT_AUTHORITY_REVOKED",
                metadata
        );
    }

    private String generateToken() {
        byte[] entropy = new byte[TOKEN_ENTROPY_BYTES];
        secureRandom.nextBytes(entropy);
        return Base64.getUrlEncoder().withoutPadding().encodeToString(entropy);
    }

    static String sha256Hex(String value) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] hash = digest.digest(value.getBytes(StandardCharsets.UTF_8));
            return HexFormat.of().formatHex(hash);
        } catch (NoSuchAlgorithmException ex) {
            throw new IllegalStateException("SHA-256 unavailable", ex);
        }
    }

    private static String requireDeploymentId(String deploymentId) {
        if (deploymentId == null || deploymentId.isBlank()) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_SCOPE_INVALID,
                    "Deployment ID is required");
        }
        String trimmed = deploymentId.trim();
        if (trimmed.length() > MAX_DEPLOYMENT_ID_CHARS) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_SCOPE_INVALID,
                    "Deployment ID exceeds maximum length");
        }
        return trimmed;
    }

    private SensorControlException invalidAuthority() {
        return new SensorControlException(
                SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID,
                "Enrollment authority is invalid");
    }
}
