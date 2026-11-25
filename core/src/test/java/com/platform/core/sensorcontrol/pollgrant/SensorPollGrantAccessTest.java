package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.ingestion.authority.ResolvedSensorAuthority;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.sensorcontrol.SensorCertificateCredential;
import com.platform.core.sensorcontrol.SensorControlException;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.sensorcontrol.SensorCredentialStatus;
import com.platform.core.sensorcontrol.SensorLifecycleStatus;
import com.platform.core.sensorcontrol.SensorPrincipal;
import com.platform.core.sensorcontrol.SensorRole;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.EnumSource;
import org.junit.jupiter.params.provider.ValueSource;
import org.mockito.InOrder;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.AddressType;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Protocol;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Status;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrantEligibility.Decision;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

/**
 * Unit coverage for Issue #229 G1b read-only grant accessor. No public API,
 * companion wire, scheduler or typed admission wiring. Proves credential-status
 * and post-lock time nondisclosure of grant existence (internal denial oracle).
 */
class SensorPollGrantAccessTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000401");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-000000000402");
    private static final UUID ENDPOINT = UUID.fromString("018f0e0a-0000-7000-8000-000000000403");
    private static final UUID GRANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000404");
    private static final UUID CREDENTIAL = UUID.fromString("018f0e0a-0000-7000-8000-000000000405");
    private static final Instant NOW = Instant.parse("2026-11-20T12:00:00Z");
    private static final String SENSOR = "sensor-access-unit";
    private static final String FINGERPRINT =
            "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

    private SensorControlRepository sensors;
    private SensorPollGrantRepository grants;
    private SensorPollGrantAccess access;

    @BeforeEach
    void setUp() {
        sensors = mock(SensorControlRepository.class);
        grants = mock(SensorPollGrantRepository.class);
        access = new SensorPollGrantAccess(
                sensors, grants, Clock.fixed(NOW, ZoneOffset.UTC));
    }

    @AfterEach
    void noOp() {
        // no transaction synchronization owned by this unit suite
    }

    @Test
    void eligibleIssuedGrantUnderAuthenticatedAuthority() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(issuedGrant()));

        SensorPollGrantEligibility outcome = access.evaluateForNewExecution(request(null));

        assertTrue(outcome.eligible());
        assertEquals(Decision.ELIGIBLE, outcome.decision());
        assertEquals(GRANT, outcome.grant().grantId());
        assertEquals(NOW, outcome.evaluatedAt());
        InOrder order = inOrder(grants, sensors);
        order.verify(grants).requireHeldCallerTransaction("evaluateForNewExecution");
        order.verify(sensors).lockPrincipalForShare(TENANT, SENSOR);
        order.verify(sensors).lockCredentialForShare(CREDENTIAL);
        order.verify(grants).findForShare(TENANT, SENSOR, GRANT);
        verify(sensors, never()).listAllowedEntityIds(any(), any());
    }

    @Test
    void activeValidMissingGrantIsNotVisible() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.empty());

        assertEquals(Decision.NOT_VISIBLE,
                access.evaluateForNewExecution(request(null)).decision());
        verify(grants).findForShare(TENANT, SENSOR, GRANT);
    }

    @Test
    void coreTimeSampledAfterFindForShareForPresentAndAbsentOutcomes() {
        assertCoreTimeFollowsFindForShare(Optional.of(issuedGrant()), Decision.ELIGIBLE);
        reset(sensors, grants);
        access = new SensorPollGrantAccess(sensors, grants, Clock.fixed(NOW, ZoneOffset.UTC));
        assertCoreTimeFollowsFindForShare(Optional.empty(), Decision.NOT_VISIBLE);
    }

    private void assertCoreTimeFollowsFindForShare(
            Optional<SensorPollGrant> locked, Decision expected) {
        AtomicBoolean clockSampled = new AtomicBoolean(false);
        AtomicBoolean grantLookupReturned = new AtomicBoolean(false);
        Clock gated = new Clock() {
            @Override
            public Instant instant() {
                assertTrue(grantLookupReturned.get(),
                        "Core time must follow findForShare for present and absent outcomes");
                clockSampled.set(true);
                return NOW;
            }

            @Override
            public ZoneOffset getZone() {
                return ZoneOffset.UTC;
            }

            @Override
            public Clock withZone(java.time.ZoneId zone) {
                return this;
            }
        };
        access = new SensorPollGrantAccess(sensors, grants, gated);
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenAnswer(inv -> {
            assertFalse(clockSampled.get());
            return activePrincipal(true);
        });
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenAnswer(inv -> {
            assertFalse(clockSampled.get());
            return activeCredential();
        });
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenAnswer(inv -> {
            assertFalse(clockSampled.get());
            grantLookupReturned.set(true);
            return locked;
        });

        assertEquals(expected, access.evaluateForNewExecution(request(null)).decision());
        assertTrue(clockSampled.get());
    }

    @ParameterizedTest
    @EnumSource(value = SensorCredentialStatus.class, names = {"REVOKED", "SUPERSEDED"})
    void nonActiveCredentialStatusDeniesWithoutGrantLookupWhetherPresentOrMissing(
            SensorCredentialStatus status) {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(credential(
                status, NOW.minusSeconds(3600), NOW.plusSeconds(3600)));

        assertEquals(Decision.CREDENTIAL_INACTIVE,
                access.evaluateForNewExecution(request(null)).decision());
        verify(grants, never()).findForShare(any(), any(), any());

        // Even if a stubbed present grant were available, status must short-circuit first.
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(issuedGrant()));
        assertEquals(Decision.CREDENTIAL_INACTIVE,
                access.evaluateForNewExecution(request(null)).decision());
        verify(grants, never()).findForShare(any(), any(), any());
    }

    @ParameterizedTest
    @ValueSource(booleans = {true, false})
    void expiredCredentialDeniesUniformlyWithoutExistenceOracle(boolean grantPresent) {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(credential(
                SensorCredentialStatus.ACTIVE,
                NOW.minusSeconds(7200), NOW.minusSeconds(1)));
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(
                grantPresent ? Optional.of(issuedGrant()) : Optional.empty());

        SensorPollGrantEligibility outcome = access.evaluateForNewExecution(request(null));
        assertEquals(Decision.CREDENTIAL_INACTIVE, outcome.decision());
        assertNull(outcome.grant());
        verify(grants).findForShare(TENANT, SENSOR, GRANT);
    }

    @ParameterizedTest
    @ValueSource(booleans = {true, false})
    void notYetValidCredentialDeniesUniformlyWithoutExistenceOracle(boolean grantPresent) {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(credential(
                SensorCredentialStatus.ACTIVE,
                NOW.plusSeconds(1), NOW.plusSeconds(3600)));
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(
                grantPresent ? Optional.of(issuedGrant()) : Optional.empty());

        assertEquals(Decision.CREDENTIAL_INACTIVE,
                access.evaluateForNewExecution(request(null)).decision());
        verify(grants).findForShare(TENANT, SENSOR, GRANT);
    }

    @ParameterizedTest
    @ValueSource(booleans = {true, false})
    void credentialExpiryDuringGrantLockWaitDeniesBothOutcomes(boolean grantPresent) {
        AtomicReference<Instant> now = new AtomicReference<>(NOW);
        Clock mutable = mutableClock(now);
        access = new SensorPollGrantAccess(sensors, grants, mutable);
        Instant notAfter = NOW.plusSeconds(30);
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(credential(
                SensorCredentialStatus.ACTIVE, NOW.minusSeconds(3600), notAfter));
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenAnswer(inv -> {
            // Simulate wait: credential expires before Core time is sampled.
            now.set(notAfter.plusSeconds(1));
            return grantPresent ? Optional.of(issuedGrant()) : Optional.empty();
        });

        SensorPollGrantEligibility outcome = access.evaluateForNewExecution(request(null));
        assertEquals(Decision.CREDENTIAL_INACTIVE, outcome.decision());
        assertNull(outcome.grant());
        verify(grants).findForShare(TENANT, SENSOR, GRANT);
    }

    @Test
    void credentialInclusiveTimeBoundariesRemainEligible() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(issuedGrant()));

        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(credential(
                SensorCredentialStatus.ACTIVE, NOW, NOW.plusSeconds(3600)));
        assertTrue(access.evaluateForNewExecution(request(null)).eligible(),
                "evaluatedAt == notBefore must be inclusive");

        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(credential(
                SensorCredentialStatus.ACTIVE, NOW.minusSeconds(3600), NOW));
        assertTrue(access.evaluateForNewExecution(request(null)).eligible(),
                "evaluatedAt == notAfter must be inclusive");
    }

    @Test
    void windowExpiryAfterLockWaitDeniesWithoutEligibleSnapshot() {
        AtomicReference<Instant> now = new AtomicReference<>(NOW);
        access = new SensorPollGrantAccess(sensors, grants, mutableClock(now));
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenAnswer(inv -> {
            now.set(NOW.plusSeconds(120));
            return activePrincipal(true);
        });
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential(
                NOW.minusSeconds(3600), NOW.plusSeconds(3600)));
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(issuedGrant()));

        SensorPollGrantEligibility outcome = access.evaluateForNewExecution(request(null));
        assertEquals(Decision.WINDOW_INACTIVE, outcome.decision());
        assertNull(outcome.grant());
    }

    @Test
    void grantWindowInclusiveBoundariesRemainEligible() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        SensorPollGrant atStart = new SensorPollGrant(
                GRANT, TENANT, SENSOR, ENTITY, ENDPOINT, 1L, 0L, Protocol.ICMP,
                AddressType.IPV4, "192.0.2.10", "plan", "v1",
                NOW.minusSeconds(10), NOW, NOW.plusSeconds(60),
                "issuer", Status.ISSUED, null, null, null, null, NOW.minusSeconds(10));
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(atStart));
        assertTrue(access.evaluateForNewExecution(request(null)).eligible());

        SensorPollGrant atEnd = new SensorPollGrant(
                GRANT, TENANT, SENSOR, ENTITY, ENDPOINT, 1L, 0L, Protocol.ICMP,
                AddressType.IPV4, "192.0.2.10", "plan", "v1",
                NOW.minusSeconds(10), NOW.minusSeconds(30), NOW,
                "issuer", Status.ISSUED, null, null, null, null, NOW.minusSeconds(10));
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(atEnd));
        assertTrue(access.evaluateForNewExecution(request(null)).eligible());
    }

    @Test
    void missingAndWrongScopeGrantsAreNotVisible() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.empty());

        assertEquals(Decision.NOT_VISIBLE,
                access.evaluateForNewExecution(request(null)).decision());
    }

    @Test
    void revokedGrantIsNotIssued() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        SensorPollGrant revoked = new SensorPollGrant(
                GRANT, TENANT, SENSOR, ENTITY, ENDPOINT, 1L, 0L, Protocol.ICMP,
                AddressType.IPV4, "192.0.2.10", "plan", "v1",
                NOW.minusSeconds(10), NOW.minusSeconds(30), NOW.plusSeconds(60),
                "issuer", Status.REVOKED, NOW.minusSeconds(1), "revoker",
                SensorPollGrant.RevocationClass.SECURITY, "security", NOW.minusSeconds(1));
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(revoked));

        assertEquals(Decision.NOT_ISSUED,
                access.evaluateForNewExecution(request(null)).decision());
    }

    @Test
    void supersededGrantIsNotIssued() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        SensorPollGrant superseded = new SensorPollGrant(
                GRANT, TENANT, SENSOR, ENTITY, ENDPOINT, 1L, 0L, Protocol.ICMP,
                AddressType.IPV4, "192.0.2.10", "plan", "v1",
                NOW.minusSeconds(10), NOW.minusSeconds(30), NOW.plusSeconds(60),
                "issuer", Status.SUPERSEDED, NOW.minusSeconds(1), "issuer",
                SensorPollGrant.RevocationClass.SUPERSEDED, "superseded", NOW.minusSeconds(1));
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(superseded));

        assertEquals(Decision.NOT_ISSUED,
                access.evaluateForNewExecution(request(null)).decision());
    }

    @Test
    void entityOutsideAllowlistIsDenied() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(false));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(issuedGrant()));

        SensorPollGrantEligibility outcome = access.evaluateForNewExecution(
                new SensorPollGrantAccess.LookupRequest(
                        authority(false, Set.of()), GRANT, null));
        assertEquals(Decision.ENTITY_OUT_OF_SCOPE, outcome.decision());
        // Scope miss fails closed without consulting live allowlist (no widen path).
        verify(sensors, never()).listAllowedEntityIds(any(), any());
    }

    @Test
    void previouslyAuthorizedThenRevokedEntityRequiresLiveAllowlistIntersection() {
        // Fail-before / pass-after for the P1 stale-scope trust bug: entity remains in
        // the previously resolved SensorScope snapshot, but live deny-first allowlist
        // under the locked principal no longer contains it.
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(false));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        when(sensors.listAllowedEntityIds(TENANT, SENSOR)).thenReturn(Set.of());
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(issuedGrant()));

        SensorPollGrantEligibility outcome = access.evaluateForNewExecution(
                new SensorPollGrantAccess.LookupRequest(
                        authority(false, Set.of(ENTITY)), GRANT, null));
        assertEquals(Decision.ENTITY_OUT_OF_SCOPE, outcome.decision());
        assertNull(outcome.grant());
        verify(sensors).listAllowedEntityIds(TENANT, SENSOR);
    }

    @Test
    void scopeAndLiveAllowlistIntersectionRemainsEligible() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(false));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        when(sensors.listAllowedEntityIds(TENANT, SENSOR)).thenReturn(Set.of(ENTITY));
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(issuedGrant()));

        assertTrue(access.evaluateForNewExecution(
                new SensorPollGrantAccess.LookupRequest(
                        authority(false, Set.of(ENTITY)), GRANT, null)).eligible());
        verify(sensors).listAllowedEntityIds(TENANT, SENSOR);
    }

    @Test
    void liveAllowlistAdditionBeyondScopeSnapshotDoesNotWiden() {
        UUID newlyGranted = UUID.fromString("018f0e0a-0000-7000-8000-000000000499");
        SensorPollGrant otherEntityGrant = new SensorPollGrant(
                GRANT, TENANT, SENSOR, newlyGranted, ENDPOINT, 1L, 0L, Protocol.ICMP,
                AddressType.IPV4, "192.0.2.10", "plan", "v1",
                NOW.minusSeconds(10), NOW.minusSeconds(30), NOW.plusSeconds(60),
                "issuer", Status.ISSUED, null, null, null, null, NOW.minusSeconds(10));
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(false));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        when(sensors.listAllowedEntityIds(TENANT, SENSOR)).thenReturn(Set.of(ENTITY, newlyGranted));
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(otherEntityGrant));

        // Scope snapshot still only allows ENTITY; live addition of newlyGranted must not widen.
        assertEquals(Decision.ENTITY_OUT_OF_SCOPE, access.evaluateForNewExecution(
                new SensorPollGrantAccess.LookupRequest(
                        authority(false, Set.of(ENTITY)), GRANT, null)).decision());
        verify(sensors, never()).listAllowedEntityIds(any(), any());
    }

    @Test
    void staleAllowAllScopeStillIntersectsLiveDenyFirstAllowlist() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(false));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        when(sensors.listAllowedEntityIds(TENANT, SENSOR)).thenReturn(Set.of());
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(issuedGrant()));

        assertEquals(Decision.ENTITY_OUT_OF_SCOPE, access.evaluateForNewExecution(
                new SensorPollGrantAccess.LookupRequest(
                        authority(true, Set.of()), GRANT, null)).decision());
        verify(sensors).listAllowedEntityIds(TENANT, SENSOR);
    }

    @Test
    void expectedBindingMismatchDeniesEvenWhenGrantOtherwiseEligible() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(issuedGrant()));

        SensorPollGrantAccess.ExpectedBinding expected = new SensorPollGrantAccess.ExpectedBinding(
                ENTITY, ENDPOINT, Protocol.SNMP, 1L, 0L, "plan", "v1",
                AddressType.IPV4, "192.0.2.10");
        assertEquals(Decision.BINDING_MISMATCH,
                access.evaluateForNewExecution(request(expected)).decision());
    }

    @Test
    void expectedBindingMatchRemainsEligible() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());
        when(grants.findForShare(TENANT, SENSOR, GRANT)).thenReturn(Optional.of(issuedGrant()));

        SensorPollGrantAccess.ExpectedBinding expected = new SensorPollGrantAccess.ExpectedBinding(
                ENTITY, ENDPOINT, Protocol.ICMP, 1L, 0L, "plan", "v1",
                AddressType.IPV4, "192.0.2.10");
        assertTrue(access.evaluateForNewExecution(request(expected)).eligible());
    }

    @Test
    void inactivePrincipalIsDeniedWithoutGrantLookup() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(new SensorPrincipal(
                SENSOR, TENANT, null, SensorRole.NETWORK, SensorLifecycleStatus.DISABLED,
                true, NOW.minusSeconds(60), NOW.minusSeconds(60)));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(activeCredential());

        assertEquals(Decision.PRINCIPAL_INACTIVE,
                access.evaluateForNewExecution(request(null)).decision());
        verify(sensors, never()).lockCredentialForShare(any());
        verify(grants, never()).findForShare(any(), any(), any());
    }

    @Test
    void credentialIdentityMismatchIsDeniedWithoutGrantLookup() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(activePrincipal(true));
        SensorCertificateCredential other = new SensorCertificateCredential(
                CREDENTIAL, TENANT, SENSOR,
                "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
                null, "serial", "CN=fixture", null,
                NOW.minusSeconds(3600), NOW.plusSeconds(3600),
                SensorCredentialStatus.ACTIVE, NOW.minusSeconds(3600), null,
                NOW.minusSeconds(3600), NOW.minusSeconds(3600));
        when(sensors.lockCredentialForShare(CREDENTIAL)).thenReturn(other);

        assertEquals(Decision.CREDENTIAL_INACTIVE,
                access.evaluateForNewExecution(request(null)).decision());
        verify(grants, never()).findForShare(any(), any(), any());
    }

    @Test
    void nullRequestAndMissingIdentityFailClosed() {
        SensorControlException missingRequest = assertThrows(SensorControlException.class,
                () -> access.evaluateForNewExecution(null));
        assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID, missingRequest.getCode());

        SensorControlException missingAuthority = assertThrows(SensorControlException.class,
                () -> access.evaluateForNewExecution(
                        new SensorPollGrantAccess.LookupRequest(null, GRANT, null)));
        assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID, missingAuthority.getCode());

        SensorControlException missingGrant = assertThrows(SensorControlException.class,
                () -> access.evaluateForNewExecution(
                        new SensorPollGrantAccess.LookupRequest(authority(true, Set.of()), null, null)));
        assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID, missingGrant.getCode());
        verify(grants, never()).requireHeldCallerTransaction(any());
    }

    @Test
    void classIsNotSpringWiredPublicSurface() {
        assertFalse(SensorPollGrantAccess.class.isAnnotationPresent(
                org.springframework.stereotype.Service.class));
        assertFalse(SensorPollGrantAccess.class.isAnnotationPresent(
                org.springframework.stereotype.Component.class));
        assertFalse(SensorPollGrantAccess.class.isAnnotationPresent(
                org.springframework.web.bind.annotation.RestController.class));
    }

    private SensorPollGrantAccess.LookupRequest request(
            SensorPollGrantAccess.ExpectedBinding expected) {
        return new SensorPollGrantAccess.LookupRequest(authority(true, Set.of()), GRANT, expected);
    }

    private static ResolvedSensorAuthority authority(boolean allowAll, Set<UUID> allowed) {
        return new ResolvedSensorAuthority(
                new SensorScope(SENSOR, TENANT, null, allowed, allowAll),
                CREDENTIAL,
                FINGERPRINT);
    }

    private static SensorPrincipal activePrincipal(boolean allowAll) {
        return new SensorPrincipal(
                SENSOR, TENANT, null, SensorRole.NETWORK, SensorLifecycleStatus.ACTIVE,
                allowAll, NOW.minusSeconds(60), NOW.minusSeconds(60));
    }

    private static SensorCertificateCredential activeCredential() {
        return activeCredential(NOW.minusSeconds(3600), NOW.plusSeconds(3600));
    }

    private static SensorCertificateCredential activeCredential(Instant notBefore, Instant notAfter) {
        return credential(SensorCredentialStatus.ACTIVE, notBefore, notAfter);
    }

    private static SensorCertificateCredential credential(
            SensorCredentialStatus status, Instant notBefore, Instant notAfter) {
        return new SensorCertificateCredential(
                CREDENTIAL, TENANT, SENSOR, FINGERPRINT, null, "serial", "CN=fixture",
                null, notBefore, notAfter, status,
                notBefore, status == SensorCredentialStatus.ACTIVE ? null : NOW.minusSeconds(10),
                notBefore, notBefore);
    }

    private static SensorPollGrant issuedGrant() {
        return new SensorPollGrant(
                GRANT, TENANT, SENSOR, ENTITY, ENDPOINT, 1L, 0L, Protocol.ICMP,
                AddressType.IPV4, "192.0.2.10", "plan", "v1",
                NOW.minusSeconds(10), NOW.minusSeconds(30), NOW.plusSeconds(60),
                "issuer", Status.ISSUED, null, null, null, null, NOW.minusSeconds(10));
    }

    private static Clock mutableClock(AtomicReference<Instant> now) {
        return new Clock() {
            @Override
            public Instant instant() {
                return now.get();
            }

            @Override
            public ZoneOffset getZone() {
                return ZoneOffset.UTC;
            }

            @Override
            public Clock withZone(java.time.ZoneId zone) {
                return this;
            }
        };
    }
}
