package com.platform.core.sensorcontrol;

import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;

import java.security.cert.X509Certificate;
import java.time.Clock;
import java.time.Instant;
import java.util.Objects;
import java.util.Set;
import java.util.UUID;

/**
 * Production Sensor authority: TLS client certificate DER fingerprint → enrolled credential → ACTIVE sensor → scope.
 * Never uses CN/SAN for authority. Fail-closed on unknown/expired/revoked credentials, disabled sensors, and DB errors.
 */
public class PostgresSensorAuthorityResolver implements SensorAuthorityResolver {

    private static final Logger log = LoggerFactory.getLogger(PostgresSensorAuthorityResolver.class);

    private final SensorControlRepository repository;
    private final SensorAuthorityMetrics metrics;
    private final Clock clock;

    public PostgresSensorAuthorityResolver(SensorControlRepository repository, SensorAuthorityMetrics metrics) {
        this(repository, metrics, Clock.systemUTC());
    }

    public PostgresSensorAuthorityResolver(
            SensorControlRepository repository,
            SensorAuthorityMetrics metrics,
            Clock clock
    ) {
        this.repository = Objects.requireNonNull(repository, "repository");
        this.metrics = Objects.requireNonNull(metrics, "metrics");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    @Override
    public SensorScope resolve(X509Certificate clientCertificate) {
        if (clientCertificate == null) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_CLIENT_CERT_REQUIRED);
            throw new IngestionException(HttpStatus.UNAUTHORIZED, "ERR_AUTH_CLIENT_CERT_REQUIRED",
                    "mTLS client certificate authentication required for sensor authority resolution");
        }

        final String fingerprint;
        try {
            fingerprint = CertificateFingerprints.sha256DerHex(clientCertificate);
        } catch (RuntimeException e) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_AUTHORITY_UNAVAILABLE);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "Unable to derive certificate fingerprint for sensor authority");
        }

        try {
            return resolveByFingerprint(fingerprint);
        } catch (IngestionException e) {
            throw e;
        } catch (RuntimeException e) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_DB_ERROR);
            log.warn("Sensor authority resolution failed closed due to persistence error");
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "Authoritative Sensor Control resolution failed; telemetry ingestion is fail-closed", e);
        }
    }

    private SensorScope resolveByFingerprint(String fingerprint) {
        Instant now = clock.instant();
        var credentialOpt = repository.findCredentialByFingerprint(fingerprint);
        if (credentialOpt.isEmpty()) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_UNKNOWN_FINGERPRINT);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_CREDENTIAL_UNKNOWN",
                    "No enrolled Sensor credential matches the client certificate");
        }

        SensorCertificateCredential credential = credentialOpt.get();
        if (credential.status() == SensorCredentialStatus.REVOKED) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_CREDENTIAL_REVOKED);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_CREDENTIAL_REVOKED",
                    "Sensor credential is revoked");
        }
        if (credential.status() == SensorCredentialStatus.SUPERSEDED) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_CREDENTIAL_SUPERSEDED);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_CREDENTIAL_REVOKED",
                    "Sensor credential has been superseded");
        }
        if (credential.status() != SensorCredentialStatus.ACTIVE) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_CREDENTIAL_REVOKED);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_CREDENTIAL_REVOKED",
                    "Sensor credential is not active");
        }
        if (now.isBefore(credential.notBefore()) || now.isAfter(credential.notAfter())) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_CREDENTIAL_EXPIRED);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_CREDENTIAL_EXPIRED",
                    "Sensor credential is outside its validity window");
        }

        SensorPrincipal principal = repository.findPrincipalByTenantAndId(credential.tenantId(), credential.sensorId())
                .orElse(null);
        if (principal == null) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_AUTHORITY_UNAVAILABLE);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "Sensor principal is missing for enrolled credential");
        }
        if (principal.status() == SensorLifecycleStatus.DISABLED) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_SENSOR_DISABLED);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_DISABLED",
                    "Sensor is disabled");
        }
        if (principal.status() == SensorLifecycleStatus.REVOKED) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_SENSOR_REVOKED);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_DISABLED",
                    "Sensor is revoked");
        }
        if (principal.status() != SensorLifecycleStatus.ACTIVE) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_SENSOR_DISABLED);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_DISABLED",
                    "Sensor is not active");
        }

        if (!repository.tenantExists(principal.tenantId())) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_TENANT_MISSING);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "Sensor tenant is not present");
        }
        if (principal.siteId() != null && !repository.siteExists(principal.tenantId(), principal.siteId())) {
            metrics.recordRejection(SensorAuthorityMetrics.REASON_SITE_INVALID);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "Sensor site is invalid for tenant");
        }

        Set<UUID> allowed = principal.allowAllTenantEntities()
                ? Set.of()
                : Set.copyOf(repository.listAllowedEntityIds(principal.tenantId(), principal.sensorId()));

        metrics.recordResolved();
        return new SensorScope(
                principal.sensorId(),
                principal.tenantId(),
                principal.siteId(),
                allowed,
                principal.allowAllTenantEntities()
        );
    }
}
