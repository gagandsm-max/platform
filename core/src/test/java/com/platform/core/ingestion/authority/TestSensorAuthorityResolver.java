package com.platform.core.ingestion.authority;

import com.platform.core.ingestion.IngestionException;
import org.springframework.http.HttpStatus;

import java.security.cert.X509Certificate;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Deterministic test-only Sensor authority resolver.
 *
 * Explicitly located under src/test to ensure it is never included in production builds.
 */
public class TestSensorAuthorityResolver implements SensorAuthorityResolver {

    private final Map<String, SensorScope> scopesByCommonName = new ConcurrentHashMap<>();
    private SensorScope defaultScope;

    public TestSensorAuthorityResolver() {}

    public TestSensorAuthorityResolver(SensorScope defaultScope) {
        this.defaultScope = defaultScope;
    }

    public void registerScope(String commonName, SensorScope scope) {
        scopesByCommonName.put(commonName, scope);
    }

    public void setDefaultScope(SensorScope scope) {
        this.defaultScope = scope;
    }

    public void clear() {
        scopesByCommonName.clear();
        defaultScope = null;
    }

    @Override
    public SensorScope resolve(X509Certificate clientCertificate) {
        if (clientCertificate == null) {
            throw new IngestionException(HttpStatus.UNAUTHORIZED, "ERR_AUTH_CLIENT_CERT_REQUIRED",
                    "mTLS client certificate authentication required for sensor authority resolution");
        }
        String dn = clientCertificate.getSubjectX500Principal().getName();
        String cn = extractCommonName(dn);
        SensorScope scope = scopesByCommonName.get(cn);
        if (scope == null && defaultScope != null) {
            scope = defaultScope;
        }
        if (scope == null) {
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "No test authority scope registered for sensor subject: " + cn);
        }
        return scope;
    }

    private static String extractCommonName(String dn) {
        if (dn == null) return "UNKNOWN";
        for (String part : dn.split(",")) {
            part = part.trim();
            if (part.startsWith("CN=") || part.startsWith("cn=")) {
                return part.substring(3);
            }
        }
        return dn;
    }
}
