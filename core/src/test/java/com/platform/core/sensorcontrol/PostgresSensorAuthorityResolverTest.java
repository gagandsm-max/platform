package com.platform.core.sensorcontrol;

import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.ResolvedSensorAuthority;
import com.platform.core.ingestion.authority.SensorScope;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;

import java.security.cert.X509Certificate;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneId;
import java.time.ZoneOffset;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class PostgresSensorAuthorityResolverTest {

    private static final Instant NOW = Instant.parse("2026-09-10T06:00:00Z");
    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private static final UUID SITE = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");

    private SensorControlRepository repository;
    private SensorAuthorityMetrics metrics;
    private PostgresSensorAuthorityResolver resolver;
    private X509Certificate cert;
    private String fingerprint;

    @BeforeEach
    void setUp() throws Exception {
        repository = mock(SensorControlRepository.class);
        metrics = new SensorAuthorityMetrics();
        resolver = new PostgresSensorAuthorityResolver(repository, metrics, Clock.fixed(NOW, ZoneOffset.UTC));
        cert = mock(X509Certificate.class);
        when(cert.getEncoded()).thenReturn(new byte[]{1, 2, 3, 4});
        fingerprint = CertificateFingerprints.sha256DerHex(cert);
    }

    private SensorCertificateCredential credential(SensorCredentialStatus status, Instant notBefore, Instant notAfter) {
        return new SensorCertificateCredential(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000c1"),
                TENANT,
                "sensor-net-01",
                fingerprint,
                null,
                "1",
                "CN=Platform Test CA",
                null,
                notBefore,
                notAfter,
                status,
                notBefore,
                status == SensorCredentialStatus.ACTIVE ? null : NOW,
                notBefore,
                NOW
        );
    }

    private SensorPrincipal principal(SensorLifecycleStatus status, boolean allowAll) {
        return new SensorPrincipal(
                "sensor-net-01", TENANT, SITE, SensorRole.NETWORK, status, allowAll, NOW, NOW);
    }

    @Test
    void resolveSuccessBuildsScopeFromAllowlist() {
        SensorCertificateCredential active = credential(SensorCredentialStatus.ACTIVE,
                NOW.minusSeconds(60), NOW.plusSeconds(3600));
        when(repository.findCredentialByFingerprint(fingerprint)).thenReturn(Optional.of(active));
        when(repository.findPrincipalByTenantAndId(TENANT, "sensor-net-01"))
                .thenReturn(Optional.of(principal(SensorLifecycleStatus.ACTIVE, false)));
        when(repository.tenantExists(TENANT)).thenReturn(true);
        when(repository.siteExists(TENANT, SITE)).thenReturn(true);
        when(repository.listAllowedEntityIds(TENANT, "sensor-net-01")).thenReturn(Set.of(ENTITY));

        SensorScope scope = resolver.resolve(cert);
        assertEquals("sensor-net-01", scope.sensorId());
        assertEquals(TENANT, scope.tenantId());
        assertEquals(SITE, scope.siteId());
        assertTrue(scope.isEntityAllowed(ENTITY));
        assertEquals(1, metrics.getResolveSuccesses());
    }

    @Test
    void resolveAuthorityReturnsDerMatchedCredentialIdentity() {
        SensorCertificateCredential active = credential(SensorCredentialStatus.ACTIVE,
                NOW.minusSeconds(60), NOW.plusSeconds(3600));
        when(repository.findCredentialByFingerprint(fingerprint)).thenReturn(Optional.of(active));
        when(repository.findPrincipalByTenantAndId(TENANT, "sensor-net-01"))
                .thenReturn(Optional.of(principal(SensorLifecycleStatus.ACTIVE, false)));
        when(repository.tenantExists(TENANT)).thenReturn(true);
        when(repository.siteExists(TENANT, SITE)).thenReturn(true);
        when(repository.listAllowedEntityIds(TENANT, "sensor-net-01")).thenReturn(Set.of(ENTITY));

        ResolvedSensorAuthority authority = resolver.resolveAuthority(cert);
        assertEquals(active.credentialId(), authority.credentialId());
        assertEquals(fingerprint, authority.fingerprintSha256());
        assertEquals("sensor-net-01", authority.scope().sensorId());
        assertEquals(TENANT, authority.scope().tenantId());
        assertTrue(authority.scope().isEntityAllowed(ENTITY));
        verify(repository, never()).lockPrincipalForShare(any(), any());
        verify(repository, never()).lockCredentialForShare(any());
    }

    @Test
    void resolveAuthorityHoldingShareUsesLockedPrincipalThenCredential() {
        SensorCertificateCredential active = credential(SensorCredentialStatus.ACTIVE,
                NOW.minusSeconds(60), NOW.plusSeconds(3600));
        SensorPrincipal lockedPrincipal = principal(SensorLifecycleStatus.ACTIVE, true);
        when(repository.findCredentialByFingerprint(fingerprint)).thenReturn(Optional.of(active));
        when(repository.lockPrincipalForShare(TENANT, "sensor-net-01")).thenReturn(lockedPrincipal);
        when(repository.lockCredentialForShare(active.credentialId())).thenReturn(active);
        when(repository.tenantExists(TENANT)).thenReturn(true);
        when(repository.siteExists(TENANT, SITE)).thenReturn(true);

        ResolvedSensorAuthority authority = resolver.resolveAuthorityHoldingShare(cert);
        assertEquals(active.credentialId(), authority.credentialId());
        assertEquals(fingerprint, authority.fingerprintSha256());
        assertEquals(lockedPrincipal.sensorId(), authority.scope().sensorId());
        verify(repository).lockPrincipalForShare(TENANT, "sensor-net-01");
        verify(repository).lockCredentialForShare(active.credentialId());
        verify(repository, never()).findPrincipalByTenantAndId(any(), any());
    }

    @Test
    void resolveAuthorityHoldingShareWithoutTransactionFailsClosed() {
        SensorCertificateCredential active = credential(SensorCredentialStatus.ACTIVE,
                NOW.minusSeconds(60), NOW.plusSeconds(3600));
        when(repository.findCredentialByFingerprint(fingerprint)).thenReturn(Optional.of(active));
        when(repository.lockPrincipalForShare(TENANT, "sensor-net-01"))
                .thenThrow(new SensorControlException(
                        SensorControlException.ERR_SENSOR_STATE_INVALID,
                        "Active PostgreSQL transaction required for lockPrincipalForShare; "
                                + "unlocked lookup is not held authority"));

        IngestionException ex = assertThrows(IngestionException.class,
                () -> resolver.resolveAuthorityHoldingShare(cert));
        assertEquals("ERR_SENSOR_AUTHORITY_UNAVAILABLE", ex.getCode());
    }

    @Test
    void resolveAuthorityHoldingShareRejectsWhenClockAdvancesPastExpiryDuringLockWait() {
        Instant tipTime = NOW;
        Instant afterLocks = NOW.plusSeconds(3601);
        Instant notBefore = NOW.minusSeconds(60);
        Instant notAfter = NOW.plusSeconds(3600);
        AtomicReference<Instant> clockInstant = new AtomicReference<>(tipTime);
        Clock advancing = new Clock() {
            @Override
            public ZoneId getZone() {
                return ZoneOffset.UTC;
            }

            @Override
            public Clock withZone(ZoneId zone) {
                return this;
            }

            @Override
            public Instant instant() {
                return clockInstant.get();
            }
        };
        resolver = new PostgresSensorAuthorityResolver(repository, metrics, advancing);

        SensorCertificateCredential active = credential(SensorCredentialStatus.ACTIVE, notBefore, notAfter);
        SensorPrincipal lockedPrincipal = principal(SensorLifecycleStatus.ACTIVE, true);
        when(repository.findCredentialByFingerprint(fingerprint)).thenReturn(Optional.of(active));
        when(repository.lockPrincipalForShare(TENANT, "sensor-net-01")).thenAnswer(invocation -> {
            clockInstant.set(afterLocks);
            return lockedPrincipal;
        });
        when(repository.lockCredentialForShare(active.credentialId())).thenReturn(active);

        IngestionException ex = assertThrows(IngestionException.class,
                () -> resolver.resolveAuthorityHoldingShare(cert));
        assertEquals("ERR_SENSOR_CREDENTIAL_EXPIRED", ex.getCode());
        assertEquals(1, metrics.getReasonCount(SensorAuthorityMetrics.REASON_CREDENTIAL_EXPIRED));
        verify(repository).lockPrincipalForShare(TENANT, "sensor-net-01");
        verify(repository).lockCredentialForShare(active.credentialId());
    }

    @Test
    void resolveAuthorityHoldingShareAcceptsWhenStillValidAfterLockWaitClockAdvance() {
        Instant tipTime = NOW;
        Instant afterLocks = NOW.plusSeconds(30);
        Instant notBefore = NOW.minusSeconds(60);
        Instant notAfter = NOW.plusSeconds(3600);
        AtomicReference<Instant> clockInstant = new AtomicReference<>(tipTime);
        Clock advancing = new Clock() {
            @Override
            public ZoneId getZone() {
                return ZoneOffset.UTC;
            }

            @Override
            public Clock withZone(ZoneId zone) {
                return this;
            }

            @Override
            public Instant instant() {
                return clockInstant.get();
            }
        };
        resolver = new PostgresSensorAuthorityResolver(repository, metrics, advancing);

        SensorCertificateCredential active = credential(SensorCredentialStatus.ACTIVE, notBefore, notAfter);
        SensorPrincipal lockedPrincipal = principal(SensorLifecycleStatus.ACTIVE, true);
        when(repository.findCredentialByFingerprint(fingerprint)).thenReturn(Optional.of(active));
        when(repository.lockPrincipalForShare(TENANT, "sensor-net-01")).thenAnswer(invocation -> {
            clockInstant.set(afterLocks);
            return lockedPrincipal;
        });
        when(repository.lockCredentialForShare(active.credentialId())).thenReturn(active);
        when(repository.tenantExists(TENANT)).thenReturn(true);
        when(repository.siteExists(TENANT, SITE)).thenReturn(true);

        ResolvedSensorAuthority authority = resolver.resolveAuthorityHoldingShare(cert);
        assertEquals(active.credentialId(), authority.credentialId());
        assertEquals(fingerprint, authority.fingerprintSha256());
        assertEquals(1, metrics.getResolveSuccesses());
    }

    @Test
    void wrongFingerprintFailsClosedAndDoesNotInventFromCn() throws Exception {
        when(cert.getSubjectX500Principal())
                .thenReturn(new javax.security.auth.x500.X500Principal("CN=sensor-net-01,O=Platform"));
        when(repository.findCredentialByFingerprint(eq(fingerprint))).thenReturn(Optional.empty());

        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolveAuthority(cert));
        assertEquals("ERR_SENSOR_CREDENTIAL_UNKNOWN", ex.getCode());
    }

    @Test
    void unknownFingerprintFailsClosed() {
        when(repository.findCredentialByFingerprint(any())).thenReturn(Optional.empty());
        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_CREDENTIAL_UNKNOWN", ex.getCode());
        assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
        assertEquals(1, metrics.getReasonCount(SensorAuthorityMetrics.REASON_UNKNOWN_FINGERPRINT));
    }

    @Test
    void revokedCredentialFailsClosed() {
        when(repository.findCredentialByFingerprint(fingerprint))
                .thenReturn(Optional.of(credential(SensorCredentialStatus.REVOKED,
                        NOW.minusSeconds(60), NOW.plusSeconds(3600))));
        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_CREDENTIAL_REVOKED", ex.getCode());
    }

    @Test
    void supersededCredentialFailsClosed() {
        when(repository.findCredentialByFingerprint(fingerprint))
                .thenReturn(Optional.of(credential(SensorCredentialStatus.SUPERSEDED,
                        NOW.minusSeconds(60), NOW.plusSeconds(3600))));
        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_CREDENTIAL_REVOKED", ex.getCode());
    }

    @Test
    void expiredCredentialFailsClosed() {
        when(repository.findCredentialByFingerprint(fingerprint))
                .thenReturn(Optional.of(credential(SensorCredentialStatus.ACTIVE,
                        NOW.minusSeconds(7200), NOW.minusSeconds(60))));
        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_CREDENTIAL_EXPIRED", ex.getCode());
    }

    @Test
    void disabledSensorFailsClosed() {
        when(repository.findCredentialByFingerprint(fingerprint))
                .thenReturn(Optional.of(credential(SensorCredentialStatus.ACTIVE,
                        NOW.minusSeconds(60), NOW.plusSeconds(3600))));
        when(repository.findPrincipalByTenantAndId(TENANT, "sensor-net-01"))
                .thenReturn(Optional.of(principal(SensorLifecycleStatus.DISABLED, true)));
        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_DISABLED", ex.getCode());
    }

    @Test
    void revokedSensorFailsClosed() {
        when(repository.findCredentialByFingerprint(fingerprint))
                .thenReturn(Optional.of(credential(SensorCredentialStatus.ACTIVE,
                        NOW.minusSeconds(60), NOW.plusSeconds(3600))));
        when(repository.findPrincipalByTenantAndId(TENANT, "sensor-net-01"))
                .thenReturn(Optional.of(principal(SensorLifecycleStatus.REVOKED, true)));
        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_DISABLED", ex.getCode());
        assertEquals(1, metrics.getReasonCount(SensorAuthorityMetrics.REASON_SENSOR_REVOKED));
    }

    @Test
    void cnCannotAuthorizeWithoutEnrollment() {
        when(repository.findCredentialByFingerprint(any())).thenReturn(Optional.empty());
        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_CREDENTIAL_UNKNOWN", ex.getCode());
    }

    @Test
    void dnsSanCannotAuthorizeWithoutEnrollment() throws Exception {
        // CN/DNS SAN may advertise an enrolled sensor id; authority is fingerprint-only.
        when(cert.getSubjectX500Principal())
                .thenReturn(new javax.security.auth.x500.X500Principal("CN=sensor-net-01,O=Platform"));
        when(cert.getSubjectAlternativeNames()).thenReturn(java.util.List.of(
                java.util.List.of(2, "sensor-net-01.sensor.platform.local")
        ));
        when(repository.findCredentialByFingerprint(any())).thenReturn(Optional.empty());

        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_CREDENTIAL_UNKNOWN", ex.getCode());
        assertEquals(1, metrics.getReasonCount(SensorAuthorityMetrics.REASON_UNKNOWN_FINGERPRINT));
    }

    @Test
    void dbFailureFailsClosed() {
        when(repository.findCredentialByFingerprint(any())).thenThrow(new RuntimeException("connection reset"));
        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_AUTHORITY_UNAVAILABLE", ex.getCode());
        assertEquals(1, metrics.getReasonCount(SensorAuthorityMetrics.REASON_DB_ERROR));
    }

    @Test
    void nullCertificateRequiresClientCert() {
        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(null));
        assertEquals(HttpStatus.UNAUTHORIZED, ex.getStatus());
        assertEquals("ERR_AUTH_CLIENT_CERT_REQUIRED", ex.getCode());
    }

    @Test
    void invalidSiteFailsClosed() {
        when(repository.findCredentialByFingerprint(fingerprint))
                .thenReturn(Optional.of(credential(SensorCredentialStatus.ACTIVE,
                        NOW.minusSeconds(60), NOW.plusSeconds(3600))));
        when(repository.findPrincipalByTenantAndId(eq(TENANT), eq("sensor-net-01")))
                .thenReturn(Optional.of(principal(SensorLifecycleStatus.ACTIVE, true)));
        when(repository.tenantExists(TENANT)).thenReturn(true);
        when(repository.siteExists(TENANT, SITE)).thenReturn(false);

        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_AUTHORITY_UNAVAILABLE", ex.getCode());
        assertEquals(1, metrics.getReasonCount(SensorAuthorityMetrics.REASON_SITE_INVALID));
    }
}
