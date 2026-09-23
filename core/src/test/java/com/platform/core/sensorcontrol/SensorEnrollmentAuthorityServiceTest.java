package com.platform.core.sensorcontrol;

import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.http.SecurityAuditEmitter;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;

import java.time.Duration;
import java.time.Instant;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.doAnswer;
import static org.mockito.Mockito.doNothing;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class SensorEnrollmentAuthorityServiceTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID SITE = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b1");
    private static final UUID OTHER_SITE = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b2");

    private SensorEnrollmentAuthorityRepository authorities;
    private SensorControlRepository sensors;
    private SecurityAuditEmitter audit;
    private SensorEnrollmentAuthorityService service;

    @BeforeEach
    void setUp() {
        authorities = mock(SensorEnrollmentAuthorityRepository.class);
        sensors = mock(SensorControlRepository.class);
        audit = mock(SecurityAuditEmitter.class);
        service = new SensorEnrollmentAuthorityService(authorities, sensors, audit);
        when(sensors.tenantExists(TENANT)).thenReturn(true);
        when(sensors.siteExists(TENANT, SITE)).thenReturn(true);
        when(sensors.siteExists(TENANT, OTHER_SITE)).thenReturn(false);
        doNothing().when(authorities).insert(any());
    }

    @Test
    void issueReturnsPlaintextAndPersistsDigestOnly() {
        AtomicReference<SensorEnrollmentAuthority> inserted = new AtomicReference<>();
        doAnswer(inv -> {
            inserted.set(inv.getArgument(0));
            return null;
        }).when(authorities).insert(any());

        IssuedSensorEnrollmentAuthority issued = service.issue(
                TENANT, "deploy-a", SITE, SensorRole.NETWORK, Duration.ofHours(1));

        assertFalse(issued.plaintextToken().isBlank());
        assertTrue(issued.plaintextToken().length() >= 40);
        assertFalse(issued.toString().contains(issued.plaintextToken()));
        assertTrue(issued.toString().contains("[REDACTED]"));

        SensorEnrollmentAuthority row = inserted.get();
        assertEquals(64, row.tokenHashSha256().length());
        assertEquals(SensorEnrollmentAuthorityService.sha256Hex(issued.plaintextToken()), row.tokenHashSha256());
        assertFalse(row.tokenHashSha256().contains(issued.plaintextToken()));

        @SuppressWarnings("unchecked")
        ArgumentCaptor<Map<String, String>> meta = ArgumentCaptor.forClass(Map.class);
        verify(audit).emitControlEvent(
                eq(AuditCategory.SECURITY_ADMINISTRATION),
                eq(AuditActions.SENSOR_ENROLLMENT_AUTHORITY_ISSUE),
                eq(AuditOutcome.SUCCESS),
                eq(TENANT),
                eq("SENSOR_ENROLLMENT_AUTHORITY_ISSUED"),
                meta.capture());
        String joined = String.join(",", meta.getValue().values());
        assertFalse(joined.contains(issued.plaintextToken()));
        assertFalse(joined.contains(row.tokenHashSha256()));
    }

    @Test
    void issueRejectsInvalidTenantSiteAndTtl() {
        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_TTL_INVALID,
                assertThrows(SensorControlException.class,
                        () -> service.issue(TENANT, "d", null, null, Duration.ZERO)).getCode());

        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_TTL_INVALID,
                assertThrows(SensorControlException.class,
                        () -> service.issue(TENANT, "d", null, null, Duration.ofHours(25))).getCode());

        when(sensors.tenantExists(TENANT)).thenReturn(false);
        assertEquals("TENANT_NOT_FOUND",
                assertThrows(SensorControlException.class,
                        () -> service.issue(TENANT, "d", null, null, Duration.ofMinutes(5))).getCode());

        when(sensors.tenantExists(TENANT)).thenReturn(true);
        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_SCOPE_INVALID,
                assertThrows(SensorControlException.class,
                        () -> service.issue(TENANT, "d", OTHER_SITE, null, Duration.ofMinutes(5))).getCode());

        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_SCOPE_INVALID,
                assertThrows(SensorControlException.class,
                        () -> service.issue(TENANT, "  ", null, null, Duration.ofMinutes(5))).getCode());
    }

    @Test
    void twoIssuesProduceDifferentTokens() {
        IssuedSensorEnrollmentAuthority a = service.issue(TENANT, "d1", null, null, Duration.ofMinutes(10));
        IssuedSensorEnrollmentAuthority b = service.issue(TENANT, "d1", null, null, Duration.ofMinutes(10));
        assertNotEquals(a.plaintextToken(), b.plaintextToken());
    }

    @Test
    void consumeSuccessAndReplayUsesSafeError() {
        Instant now = Instant.now();
        String token = "test-token-value-abcdefghijklmnopqrstuvwxyz012345";
        String digest = SensorEnrollmentAuthorityService.sha256Hex(token);
        UUID authorityId = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");
        SensorEnrollmentAuthority row = new SensorEnrollmentAuthority(
                authorityId,
                digest, TENANT, "deploy-a", SITE, SensorRole.NETWORK,
                now.minusSeconds(10), now.plusSeconds(3600), null, null, now, now);
        when(authorities.findByTokenHash(digest)).thenReturn(Optional.of(row));
        when(authorities.tryConsume(eq(authorityId), any(Instant.class))).thenReturn(true);

        ConsumedSensorEnrollmentAuthority consumed = service.consume(token, SensorRole.NETWORK, SITE);
        assertEquals(authorityId, consumed.authorityId());
        assertEquals(TENANT, consumed.tenantId());

        when(authorities.tryConsume(eq(authorityId), any(Instant.class))).thenReturn(false);
        SensorControlException replay = assertThrows(SensorControlException.class,
                () -> service.consume(token, SensorRole.NETWORK, SITE));
        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID, replay.getCode());
        assertFalse(replay.getMessage().contains(token));
    }

    @Test
    void scopeMismatchDoesNotConsume() {
        Instant now = Instant.now();
        String token = "scope-mismatch-token-abcdefghijklmnopqrstuvwxyz";
        String digest = SensorEnrollmentAuthorityService.sha256Hex(token);
        SensorEnrollmentAuthority row = new SensorEnrollmentAuthority(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000e2"),
                digest, TENANT, "deploy-a", SITE, SensorRole.NETWORK,
                now.minusSeconds(10), now.plusSeconds(3600), null, null, now, now);
        when(authorities.findByTokenHash(digest)).thenReturn(Optional.of(row));

        SensorControlException wrongRole = assertThrows(SensorControlException.class,
                () -> service.consume(token, SensorRole.INFRASTRUCTURE, SITE));
        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID, wrongRole.getCode());
        verify(authorities, never()).tryConsume(any(), any());
    }

    @Test
    void revokeIsDeterministicAndBlocksConsumed() {
        Instant now = Instant.now();
        UUID id = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e3");
        SensorEnrollmentAuthority active = new SensorEnrollmentAuthority(
                id, "a".repeat(64), TENANT, "deploy-a", null, null,
                now.minusSeconds(10), now.plusSeconds(3600), null, null, now, now);
        when(authorities.findById(id)).thenReturn(Optional.of(active));
        when(authorities.tryRevoke(eq(id), any(Instant.class))).thenReturn(true);
        service.revoke(id);
        verify(audit).emitControlEvent(
                eq(AuditCategory.SECURITY_ADMINISTRATION),
                eq(AuditActions.SENSOR_ENROLLMENT_AUTHORITY_REVOKE),
                eq(AuditOutcome.SUCCESS),
                eq(TENANT),
                eq("SENSOR_ENROLLMENT_AUTHORITY_REVOKED"),
                any());

        SensorEnrollmentAuthority alreadyRevoked = new SensorEnrollmentAuthority(
                id, "a".repeat(64), TENANT, "deploy-a", null, null,
                now.minusSeconds(10), now.plusSeconds(3600), null, now, now, now);
        when(authorities.findById(id)).thenReturn(Optional.of(alreadyRevoked));
        service.revoke(id);

        SensorEnrollmentAuthority consumed = new SensorEnrollmentAuthority(
                id, "a".repeat(64), TENANT, "deploy-a", null, null,
                now.minusSeconds(10), now.plusSeconds(3600), now, null, now, now);
        when(authorities.findById(id)).thenReturn(Optional.of(consumed));
        assertEquals(SensorControlException.ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID,
                assertThrows(SensorControlException.class, () -> service.revoke(id)).getCode());
    }
}
