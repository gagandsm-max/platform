package com.platform.core.sensorcontrol;

import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.http.SecurityAuditEmitter;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

/**
 * Unit coverage for Sensor disable vs revoke audit action identity.
 * Invokes real {@link SensorControlService} methods and captures emitted control events.
 */
class SensorControlAuditTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000213");
    private static final Instant NOW = Instant.parse("2026-10-27T00:00:00Z");
    private static final String PRIVATE_MATERIAL =
            "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n-----END PRIVATE KEY-----";
    private static final String FINGERPRINT =
            "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

    private SensorControlRepository repository;
    private SecurityAuditEmitter audit;
    private SensorControlService service;

    @BeforeEach
    void setUp() {
        repository = mock(SensorControlRepository.class);
        audit = mock(SecurityAuditEmitter.class);
        service = new SensorControlService(repository, audit);
    }

    @Test
    void disableAndRevokeEmitDistinctExactActionsWithCorrectMetadata() {
        SensorPrincipal disableActive = activePrincipal("sensor-disable");
        SensorPrincipal disableDisabled = new SensorPrincipal(
                "sensor-disable", TENANT, null, SensorRole.NETWORK,
                SensorLifecycleStatus.DISABLED, true, NOW, NOW);
        SensorPrincipal revokeActive = activePrincipal("sensor-revoke");
        SensorPrincipal revokeRevoked = revokedPrincipal("sensor-revoke");
        SensorCertificateCredential activeCred = activeCredential(revokeActive);

        when(repository.findPrincipalById("sensor-disable"))
                .thenReturn(Optional.of(disableActive), Optional.of(disableDisabled));
        when(repository.lockPrincipalForUpdate(TENANT, "sensor-disable")).thenReturn(disableActive);

        when(repository.findPrincipalById("sensor-revoke"))
                .thenReturn(Optional.of(revokeActive), Optional.of(revokeRevoked));
        when(repository.lockPrincipalForUpdate(TENANT, "sensor-revoke")).thenReturn(revokeActive);
        when(repository.listActiveCredentialsForSensor(TENANT, "sensor-revoke"))
                .thenReturn(List.of(activeCred));

        service.disableSensor("sensor-disable");
        service.revokeSensor("sensor-revoke");

        @SuppressWarnings("unchecked")
        ArgumentCaptor<Map<String, String>> disableMeta = ArgumentCaptor.forClass(Map.class);
        verify(audit).emitControlEvent(
                eq(AuditCategory.SECURITY_ADMINISTRATION),
                eq(AuditActions.SENSOR_DISABLE),
                eq(AuditOutcome.SUCCESS),
                eq(TENANT),
                eq("SENSOR_DISABLED"),
                disableMeta.capture());

        @SuppressWarnings("unchecked")
        ArgumentCaptor<Map<String, String>> revokeMeta = ArgumentCaptor.forClass(Map.class);
        verify(audit).emitControlEvent(
                eq(AuditCategory.SECURITY_ADMINISTRATION),
                eq(AuditActions.SENSOR_REVOKE),
                eq(AuditOutcome.SUCCESS),
                eq(TENANT),
                eq("SENSOR_REVOKED"),
                revokeMeta.capture());

        assertEquals("sensor.disable", AuditActions.SENSOR_DISABLE.value());
        assertEquals("sensor.revoke", AuditActions.SENSOR_REVOKE.value());
        assertNotEquals(AuditActions.SENSOR_DISABLE, AuditActions.SENSOR_REVOKE);
        assertNotEquals(AuditActions.SENSOR_DISABLE.value(), AuditActions.SENSOR_REVOKE.value());

        assertEquals(Map.of("sensor_id", "sensor-disable"), disableMeta.getValue());
        assertEquals(Map.of("sensor_id", "sensor-revoke"), revokeMeta.getValue());
        assertNoSecretLeak(disableMeta.getValue());
        assertNoSecretLeak(revokeMeta.getValue());

        verify(repository).updatePrincipalStatus(eq("sensor-disable"), eq(SensorLifecycleStatus.DISABLED), any());
        verify(repository).updatePrincipalStatus(eq("sensor-revoke"), eq(SensorLifecycleStatus.REVOKED), any());
        verify(repository).updateCredentialStatus(
                eq(activeCred.credentialId()), eq(SensorCredentialStatus.REVOKED), any(), any());
        // Disable must not revoke credentials (distinct lifecycle from revokeSensor).
        verify(repository, never()).listActiveCredentialsForSensor(TENANT, "sensor-disable");
    }

    @Test
    void revokeDoesNotLeakCredentialOrPrivateMaterialIntoMetadata() {
        SensorPrincipal principal = activePrincipal("sensor-secret");
        SensorCertificateCredential activeCred = activeCredential(principal);
        when(repository.findPrincipalById("sensor-secret"))
                .thenReturn(Optional.of(principal), Optional.of(revokedPrincipal("sensor-secret")));
        when(repository.lockPrincipalForUpdate(TENANT, "sensor-secret")).thenReturn(principal);
        when(repository.listActiveCredentialsForSensor(TENANT, "sensor-secret"))
                .thenReturn(List.of(activeCred));

        service.revokeSensor("sensor-secret");

        @SuppressWarnings("unchecked")
        ArgumentCaptor<Map<String, String>> meta = ArgumentCaptor.forClass(Map.class);
        verify(audit).emitControlEvent(
                eq(AuditCategory.SECURITY_ADMINISTRATION),
                eq(AuditActions.SENSOR_REVOKE),
                eq(AuditOutcome.SUCCESS),
                eq(TENANT),
                eq("SENSOR_REVOKED"),
                meta.capture());

        assertNoSecretLeak(meta.getValue());
        assertFalse(meta.getValue().containsKey("fingerprint_sha256"));
        assertFalse(meta.getValue().containsKey("credential_id"));
        assertFalse(String.join(",", meta.getValue().values()).contains(FINGERPRINT));
        assertFalse(String.join(",", meta.getValue().values()).contains(PRIVATE_MATERIAL));
    }

    @Test
    void missingPrincipalProducesNoSuccessAuditEvent() {
        when(repository.findPrincipalById("missing-sensor")).thenReturn(Optional.empty());

        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> service.revokeSensor("missing-sensor"));
        assertEquals(SensorControlException.ERR_SENSOR_NOT_FOUND, ex.getCode());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
        verify(repository, never()).updatePrincipalStatus(any(), any(), any());
        verify(repository, never()).lockPrincipalForUpdate(any(), any());
    }

    @Test
    void failureBeforeEmissionProducesNoSuccessAuditEvent() {
        SensorPrincipal principal = activePrincipal("sensor-lock-fail");
        when(repository.findPrincipalById("sensor-lock-fail")).thenReturn(Optional.of(principal));
        when(repository.lockPrincipalForUpdate(TENANT, "sensor-lock-fail"))
                .thenThrow(new IllegalStateException("lock unavailable"));

        assertThrows(IllegalStateException.class, () -> service.disableSensor("sensor-lock-fail"));
        assertThrows(IllegalStateException.class, () -> service.revokeSensor("sensor-lock-fail"));

        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
        verify(repository, never()).updatePrincipalStatus(any(), any(), any());
    }

    private static void assertNoSecretLeak(Map<String, String> metadata) {
        String joined = String.join(",", metadata.values()) + "," + String.join(",", metadata.keySet());
        assertFalse(joined.contains(PRIVATE_MATERIAL));
        assertFalse(joined.toLowerCase().contains("begin private key"));
        assertFalse(joined.contains("password"));
        assertFalse(joined.contains("private_key"));
        assertFalse(joined.contains("token"));
    }

    private static SensorPrincipal activePrincipal(String sensorId) {
        return new SensorPrincipal(
                sensorId, TENANT, null, SensorRole.NETWORK, SensorLifecycleStatus.ACTIVE, true, NOW, NOW);
    }

    private static SensorPrincipal revokedPrincipal(String sensorId) {
        return new SensorPrincipal(
                sensorId, TENANT, null, SensorRole.NETWORK, SensorLifecycleStatus.REVOKED, true, NOW, NOW);
    }

    private static SensorCertificateCredential activeCredential(SensorPrincipal principal) {
        return new SensorCertificateCredential(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000c1"),
                principal.tenantId(),
                principal.sensorId(),
                FINGERPRINT,
                null,
                "aa01",
                "CN=Platform Test CA",
                null,
                NOW.minusSeconds(60),
                NOW.plusSeconds(3600),
                SensorCredentialStatus.ACTIVE,
                NOW,
                null,
                NOW,
                NOW
        );
    }
}
