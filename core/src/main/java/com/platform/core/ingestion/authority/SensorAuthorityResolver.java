package com.platform.core.ingestion.authority;

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
}
