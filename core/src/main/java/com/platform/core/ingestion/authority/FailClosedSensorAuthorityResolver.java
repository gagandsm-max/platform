package com.platform.core.ingestion.authority;

import com.platform.core.ingestion.IngestionException;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;

import java.security.cert.X509Certificate;

/**
 * Production authority resolver: FAIL CLOSED.
 *
 * Architectural Fact:
 * The repository does NOT currently implement authoritative Core Sensor Control/enrollment
 * persistence capable of resolving verified client certificate -> sensor -> tenant -> site -> allowed entities.
 *
 * Until actual Sensor Control/enrollment exists, this resolver unconditionally fails closed.
 */
@Component
@ConditionalOnMissingBean(SensorAuthorityResolver.class)
public class FailClosedSensorAuthorityResolver implements SensorAuthorityResolver {

    private static final Logger log = LoggerFactory.getLogger(FailClosedSensorAuthorityResolver.class);

    @Override
    public SensorScope resolve(X509Certificate clientCertificate) {
        if (clientCertificate == null) {
            throw new IngestionException(HttpStatus.UNAUTHORIZED, "ERR_AUTH_CLIENT_CERT_REQUIRED",
                    "mTLS client certificate authentication required for sensor authority resolution");
        }
        String dn = clientCertificate.getSubjectX500Principal().getName();
        log.warn("Sensor authority resolution rejected (fail-closed): sensor control enrollment unavailable for subject={}", dn);
        throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                "Authoritative Sensor Control and certificate enrollment is not implemented; telemetry ingestion is fail-closed");
    }
}
