package com.platform.core.ingestion.authority;

import org.junit.jupiter.api.Test;

import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class ResolvedSensorAuthorityTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private static final UUID CREDENTIAL = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c1");
    private static final String FINGERPRINT =
            "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

    @Test
    void retainsScopeCredentialIdAndDerFingerprint() {
        SensorScope scope = new SensorScope("sensor-net-01", TENANT, null, Set.of(), true);
        ResolvedSensorAuthority authority = new ResolvedSensorAuthority(scope, CREDENTIAL, FINGERPRINT);

        assertEquals(scope, authority.scope());
        assertEquals(CREDENTIAL, authority.credentialId());
        assertEquals(FINGERPRINT, authority.fingerprintSha256());
    }

    @Test
    void rejectsNullComponents() {
        SensorScope scope = new SensorScope("sensor-net-01", TENANT, null, Set.of(), true);
        assertThrows(NullPointerException.class,
                () -> new ResolvedSensorAuthority(null, CREDENTIAL, FINGERPRINT));
        assertThrows(NullPointerException.class,
                () -> new ResolvedSensorAuthority(scope, null, FINGERPRINT));
        assertThrows(NullPointerException.class,
                () -> new ResolvedSensorAuthority(scope, CREDENTIAL, null));
    }

    @Test
    void rejectsBlankFingerprint() {
        SensorScope scope = new SensorScope("sensor-net-01", TENANT, null, Set.of(), true);
        assertThrows(IllegalArgumentException.class,
                () -> new ResolvedSensorAuthority(scope, CREDENTIAL, "  "));
    }

    @Test
    void interfaceDefaultResolveAuthorityFailsClosedWithoutInventingCredential() {
        SensorAuthorityResolver resolver = clientCertificate ->
                new SensorScope("sensor-from-scope-only", TENANT, null, Set.of(), true);

        var ex = assertThrows(com.platform.core.ingestion.IngestionException.class,
                () -> resolver.resolveAuthority(null));
        assertEquals("ERR_SENSOR_AUTHORITY_UNAVAILABLE", ex.getCode());
    }
}
