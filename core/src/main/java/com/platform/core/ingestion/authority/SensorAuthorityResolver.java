package com.platform.core.ingestion.authority;

import com.platform.core.ingestion.IngestionException;
import org.springframework.http.HttpStatus;

import java.security.cert.X509Certificate;

/**
 * Server-side authority interface for Sensor machine identity and tenant/entity scope.
 *
 * Consumes authenticated TLS client-certificate evidence and resolves an authoritative
 * SensorScope. Payload claims (sensorId, tenantId, entityId) cannot grant authority.
 */
public interface SensorAuthorityResolver {

    /**
     * Resolves the authoritative SensorScope for the given verified client certificate.
     *
     * @param clientCertificate verified TLS client certificate
     * @return authoritative SensorScope
     * @throws com.platform.core.ingestion.IngestionException if authority is unavailable or unauthorized
     */
    SensorScope resolve(X509Certificate clientCertificate);

    /**
     * Credential-aware identity observation from the matched server credential.
     *
     * <p>Fail-closed when credential identity cannot be established from authoritative
     * server state. Never derives credential identity from request fields, CN/SAN, or a
     * plain {@link SensorScope}. Result is an identity observation, not admission proof.
     *
     * @param clientCertificate verified TLS client certificate
     * @return immutable resolved authority including credential id and DER fingerprint
     * @throws com.platform.core.ingestion.IngestionException if authority is unavailable or unauthorized
     */
    default ResolvedSensorAuthority resolveAuthority(X509Certificate clientCertificate) {
        throw new IngestionException(
                HttpStatus.FORBIDDEN,
                "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                "Credential-aware Sensor authority resolution is unavailable");
    }
}
