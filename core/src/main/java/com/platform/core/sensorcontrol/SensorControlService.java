package com.platform.core.sensorcontrol;

import com.platform.core.id.UuidV7;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.http.SecurityAuditEmitter;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.UUID;

/**
 * Internal Sensor Control operations (enrollment, credential bind/rotate/revoke, disable/revoke, entity scope).
 * No public REST controllers — callers are Core control-plane components only.
 */
@Service
public class SensorControlService {

    private final SensorControlRepository repository;
    private final SecurityAuditEmitter auditEmitter;

    public SensorControlService(SensorControlRepository repository, SecurityAuditEmitter auditEmitter) {
        this.repository = Objects.requireNonNull(repository, "repository");
        this.auditEmitter = Objects.requireNonNull(auditEmitter, "auditEmitter");
    }

    @Transactional("transactionManager")
    public SensorPrincipal enrollSensor(
            String sensorId,
            UUID tenantId,
            UUID siteId,
            SensorRole role,
            boolean allowAllTenantEntities
    ) {
        String id = requireSensorId(sensorId);
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(role, "role");

        if (!repository.tenantExists(tenantId)) {
            throw new SensorControlException("TENANT_NOT_FOUND", "Tenant not found");
        }
        if (siteId != null && !repository.siteExists(tenantId, siteId)) {
            throw new SensorControlException(SensorControlException.ERR_SENSOR_SITE_INVALID,
                    "Site does not exist for tenant");
        }
        if (repository.findPrincipalById(id).isPresent()) {
            throw new SensorControlException(SensorControlException.ERR_SENSOR_ALREADY_EXISTS,
                    "Sensor already enrolled");
        }

        Instant now = Instant.now();
        SensorPrincipal principal = new SensorPrincipal(
                id, tenantId, siteId, role, SensorLifecycleStatus.ACTIVE,
                allowAllTenantEntities, now, now
        );
        try {
            repository.insertPrincipal(principal);
        } catch (DuplicateKeyException e) {
            throw new SensorControlException(SensorControlException.ERR_SENSOR_ALREADY_EXISTS,
                    "Sensor already enrolled");
        }

        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.SENSOR_ENROLL,
                AuditOutcome.SUCCESS,
                tenantId,
                "SENSOR_ENROLLED",
                Map.of("sensor_id", id, "role", role.name())
        );
        return principal;
    }

    @Transactional("transactionManager")
    public SensorCertificateCredential bindCertificate(String sensorId, X509Certificate certificate) {
        Objects.requireNonNull(certificate, "certificate");
        SensorPrincipal principal = requirePrincipal(sensorId);
        // Serialize with rotate/revoke/disable; re-read lifecycle under the principal row lock.
        principal = requireActivePrincipalForCredentialIssuance(
                repository.lockPrincipalForUpdate(principal.tenantId(), principal.sensorId()));
        Instant now = Instant.now();
        Instant notBefore = certificate.getNotBefore().toInstant();
        Instant notAfter = certificate.getNotAfter().toInstant();
        if (!notAfter.isAfter(notBefore)) {
            throw new SensorControlException(SensorControlException.ERR_SENSOR_CREDENTIAL_INVALID,
                    "Certificate validity window is invalid");
        }

        String fingerprint = CertificateFingerprints.sha256DerHex(certificate);
        SensorCertificateCredential credential = buildCredential(
                principal,
                fingerprint,
                CertificateFingerprints.spkiSha256Hex(certificate),
                certificate.getSerialNumber().toString(16),
                certificate.getIssuerX500Principal().getName(),
                null,
                notBefore,
                notAfter,
                now
        );
        repository.insertCredential(credential);
        auditEmitter.emitControlEvent(
                AuditCategory.CREDENTIAL,
                AuditActions.SENSOR_CREDENTIAL_BIND,
                AuditOutcome.SUCCESS,
                principal.tenantId(),
                "SENSOR_CREDENTIAL_BOUND",
                Map.of("sensor_id", principal.sensorId(), "fingerprint_sha256", fingerprint)
        );
        return credential;
    }

    @Transactional("transactionManager")
    public SensorCertificateCredential bindCertificateFromMetadata(
            String sensorId,
            String fingerprintSha256,
            String spkiSha256,
            String serialNumber,
            String issuerDn,
            String issuerFingerprintSha256,
            Instant notBefore,
            Instant notAfter
    ) {
        SensorPrincipal principal = requirePrincipal(sensorId);
        Objects.requireNonNull(fingerprintSha256, "fingerprintSha256");
        Objects.requireNonNull(serialNumber, "serialNumber");
        Objects.requireNonNull(issuerDn, "issuerDn");
        Objects.requireNonNull(notBefore, "notBefore");
        Objects.requireNonNull(notAfter, "notAfter");
        principal = requireActivePrincipalForCredentialIssuance(
                repository.lockPrincipalForUpdate(principal.tenantId(), principal.sensorId()));
        if (!fingerprintSha256.matches("^[a-f0-9]{64}$")) {
            throw new SensorControlException(SensorControlException.ERR_SENSOR_CREDENTIAL_INVALID,
                    "fingerprint must be lowercase hex SHA-256");
        }
        if (!notAfter.isAfter(notBefore)) {
            throw new SensorControlException(SensorControlException.ERR_SENSOR_CREDENTIAL_INVALID,
                    "Certificate validity window is invalid");
        }

        Instant now = Instant.now();
        SensorCertificateCredential credential = buildCredential(
                principal,
                fingerprintSha256,
                spkiSha256,
                serialNumber,
                issuerDn,
                issuerFingerprintSha256,
                notBefore,
                notAfter,
                now
        );
        repository.insertCredential(credential);
        auditEmitter.emitControlEvent(
                AuditCategory.CREDENTIAL,
                AuditActions.SENSOR_CREDENTIAL_BIND,
                AuditOutcome.SUCCESS,
                principal.tenantId(),
                "SENSOR_CREDENTIAL_BOUND",
                Map.of("sensor_id", principal.sensorId(), "fingerprint_sha256", fingerprintSha256)
        );
        return credential;
    }

    /**
     * Rotate certificate for the same sensor. Supersedes prior ACTIVE credentials.
     * Tenant and site of the principal are never changed by rotation.
     */
    @Transactional("transactionManager")
    public SensorCertificateCredential rotateCertificate(String sensorId, X509Certificate newCertificate) {
        SensorPrincipal principal = requirePrincipal(sensorId);
        Objects.requireNonNull(newCertificate, "newCertificate");
        // Serialize credential mutations; re-read lifecycle under the principal row lock.
        principal = requireActivePrincipalForCredentialIssuance(
                repository.lockPrincipalForUpdate(principal.tenantId(), principal.sensorId()));
        Instant now = Instant.now();
        Instant notBefore = newCertificate.getNotBefore().toInstant();
        Instant notAfter = newCertificate.getNotAfter().toInstant();
        if (!notAfter.isAfter(notBefore)) {
            throw new SensorControlException(SensorControlException.ERR_SENSOR_CREDENTIAL_INVALID,
                    "Certificate validity window is invalid");
        }

        List<SensorCertificateCredential> active =
                repository.listActiveCredentialsForSensor(principal.tenantId(), principal.sensorId());
        for (SensorCertificateCredential existing : active) {
            repository.updateCredentialStatus(
                    existing.credentialId(), SensorCredentialStatus.SUPERSEDED, now, now);
        }

        String fingerprint = CertificateFingerprints.sha256DerHex(newCertificate);
        SensorCertificateCredential bound = buildCredential(
                principal,
                fingerprint,
                CertificateFingerprints.spkiSha256Hex(newCertificate),
                newCertificate.getSerialNumber().toString(16),
                newCertificate.getIssuerX500Principal().getName(),
                null,
                notBefore,
                notAfter,
                now
        );
        repository.insertCredential(bound);
        auditEmitter.emitControlEvent(
                AuditCategory.CREDENTIAL,
                AuditActions.SENSOR_CREDENTIAL_ROTATE,
                AuditOutcome.SUCCESS,
                principal.tenantId(),
                "SENSOR_CREDENTIAL_ROTATED",
                Map.of(
                        "sensor_id", principal.sensorId(),
                        "fingerprint_sha256", fingerprint,
                        "superseded_count", String.valueOf(active.size())
                )
        );
        return bound;
    }

    @Transactional("transactionManager")
    public void revokeCredential(String fingerprintSha256) {
        Objects.requireNonNull(fingerprintSha256, "fingerprintSha256");
        SensorCertificateCredential credential = repository.findCredentialByFingerprint(fingerprintSha256)
                .orElseThrow(() -> new SensorControlException(
                        SensorControlException.ERR_SENSOR_CREDENTIAL_NOT_FOUND,
                        "Credential fingerprint not found"));
        repository.lockPrincipalForUpdate(credential.tenantId(), credential.sensorId());
        Instant now = Instant.now();
        repository.updateCredentialStatus(
                credential.credentialId(), SensorCredentialStatus.REVOKED, now, now);
        auditEmitter.emitControlEvent(
                AuditCategory.CREDENTIAL,
                AuditActions.SENSOR_CREDENTIAL_REVOKE,
                AuditOutcome.SUCCESS,
                credential.tenantId(),
                "SENSOR_CREDENTIAL_REVOKED",
                Map.of("sensor_id", credential.sensorId(), "fingerprint_sha256", fingerprintSha256)
        );
    }

    @Transactional("transactionManager")
    public SensorPrincipal disableSensor(String sensorId) {
        SensorPrincipal principal = requirePrincipal(sensorId);
        // Same principal lock order as bind/rotate/revoke so disable cannot race past credential issuance.
        principal = repository.lockPrincipalForUpdate(principal.tenantId(), principal.sensorId());
        Instant now = Instant.now();
        repository.updatePrincipalStatus(principal.sensorId(), SensorLifecycleStatus.DISABLED, now);
        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.SENSOR_DISABLE,
                AuditOutcome.SUCCESS,
                principal.tenantId(),
                "SENSOR_DISABLED",
                Map.of("sensor_id", principal.sensorId())
        );
        return repository.findPrincipalById(principal.sensorId()).orElseThrow();
    }

    @Transactional("transactionManager")
    public SensorPrincipal revokeSensor(String sensorId) {
        SensorPrincipal principal = requirePrincipal(sensorId);
        principal = repository.lockPrincipalForUpdate(principal.tenantId(), principal.sensorId());
        Instant now = Instant.now();
        repository.updatePrincipalStatus(principal.sensorId(), SensorLifecycleStatus.REVOKED, now);
        List<SensorCertificateCredential> active =
                repository.listActiveCredentialsForSensor(principal.tenantId(), principal.sensorId());
        for (SensorCertificateCredential existing : active) {
            repository.updateCredentialStatus(
                    existing.credentialId(), SensorCredentialStatus.REVOKED, now, now);
        }
        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.SENSOR_DISABLE,
                AuditOutcome.SUCCESS,
                principal.tenantId(),
                "SENSOR_REVOKED",
                Map.of("sensor_id", principal.sensorId())
        );
        return repository.findPrincipalById(principal.sensorId()).orElseThrow();
    }

    /**
     * Replace entity allowlist. When {@code allowAllTenantEntities} is true, allowlist is cleared
     * and the principal flag is set; otherwise the provided entity set becomes the exclusive allowlist.
     *
     * <p>Acquires the principal {@code FOR UPDATE} before entity existence checks or child allowlist
     * DML and uses that locked row. The later flag UPDATE is not a substitute for this initial lock.
     */
    @Transactional("transactionManager")
    public void replaceEntityAllowlist(String sensorId, boolean allowAllTenantEntities, Set<UUID> entityIds) {
        SensorPrincipal unlocked = requirePrincipal(sensorId);
        SensorPrincipal principal = repository.lockPrincipalForUpdate(unlocked.tenantId(), unlocked.sensorId());
        Instant now = Instant.now();
        int allowlistSize = entityIds == null ? 0 : entityIds.size();
        if (allowAllTenantEntities) {
            repository.replaceAllowedEntities(principal.tenantId(), principal.sensorId(), Set.of(), now);
            repository.updateAllowAllTenantEntities(principal.sensorId(), true, now);
        } else {
            Set<UUID> ids = entityIds == null ? Set.of() : Set.copyOf(entityIds);
            for (UUID entityId : ids) {
                if (!repository.entityExists(principal.tenantId(), entityId)) {
                    throw new SensorControlException(SensorControlException.ERR_SENSOR_ENTITY_INVALID,
                            "Entity not found in tenant: " + entityId);
                }
            }
            repository.replaceAllowedEntities(principal.tenantId(), principal.sensorId(), ids, now);
            repository.updateAllowAllTenantEntities(principal.sensorId(), false, now);
        }
        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.SENSOR_SCOPE_REPLACE,
                AuditOutcome.SUCCESS,
                principal.tenantId(),
                "SENSOR_ENTITY_SCOPE_UPDATED",
                Map.of(
                        "sensor_id", principal.sensorId(),
                        "allow_all_tenant_entities", String.valueOf(allowAllTenantEntities),
                        "allowlist_size", String.valueOf(allowlistSize)
                )
        );
    }

    public SensorPrincipal requirePrincipal(String sensorId) {
        return repository.findPrincipalById(requireSensorId(sensorId))
                .orElseThrow(() -> new SensorControlException(
                        SensorControlException.ERR_SENSOR_NOT_FOUND, "Sensor not found"));
    }

    /**
     * Credential bind/rotate may issue ACTIVE credentials only for ACTIVE principals.
     * Must be evaluated on the principal row read under {@code FOR UPDATE}.
     */
    private static SensorPrincipal requireActivePrincipalForCredentialIssuance(SensorPrincipal principal) {
        if (principal.status() != SensorLifecycleStatus.ACTIVE) {
            throw new SensorControlException(SensorControlException.ERR_SENSOR_STATE_INVALID,
                    "Sensor is not ACTIVE");
        }
        return principal;
    }

    private static SensorCertificateCredential buildCredential(
            SensorPrincipal principal,
            String fingerprint,
            String spkiSha256,
            String serialNumber,
            String issuerDn,
            String issuerFingerprintSha256,
            Instant notBefore,
            Instant notAfter,
            Instant now
    ) {
        return new SensorCertificateCredential(
                UuidV7.generate(),
                principal.tenantId(),
                principal.sensorId(),
                fingerprint,
                spkiSha256,
                serialNumber,
                issuerDn,
                issuerFingerprintSha256,
                notBefore,
                notAfter,
                SensorCredentialStatus.ACTIVE,
                now,
                null,
                now,
                now
        );
    }

    private static String requireSensorId(String sensorId) {
        if (sensorId == null || sensorId.isBlank() || sensorId.length() > 128) {
            throw new SensorControlException(SensorControlException.ERR_SENSOR_STATE_INVALID,
                    "sensorId must be 1..128 characters");
        }
        return sensorId.trim();
    }
}
