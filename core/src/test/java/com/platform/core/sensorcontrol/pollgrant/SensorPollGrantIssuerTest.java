package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.domain.inventory.EntityManagementEndpoint;
import com.platform.core.domain.inventory.ManagementEndpointAddressType;
import com.platform.core.domain.inventory.ManagementEndpointSource;
import com.platform.core.domain.inventory.ManagementEndpointState;
import com.platform.core.persistence.EntityManagementEndpointRepository;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.sensorcontrol.SensorControlException;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.sensorcontrol.SensorLifecycleStatus;
import com.platform.core.sensorcontrol.SensorPrincipal;
import com.platform.core.sensorcontrol.SensorRole;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.transaction.support.TransactionSynchronization;
import org.springframework.transaction.support.TransactionSynchronizationManager;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Protocol;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Status;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.*;

/**
 * Unit coverage for Issue #229 G1a internal issuer. No public API or scheduler wiring.
 */
class SensorPollGrantIssuerTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000301");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-000000000302");
    private static final UUID ENDPOINT = UUID.fromString("018f0e0a-0000-7000-8000-000000000303");
    private static final UUID GRANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000304");
    private static final Instant ISSUED = Instant.parse("2026-10-28T12:00:00Z");
    private static final String SENSOR = "sensor-issuer-unit";

    private SensorControlRepository sensors;
    private EntityManagementEndpointRepository endpoints;
    private SensorPollGrantRepository grants;
    private SecurityAuditEmitter audit;
    private SensorPollGrantIssuer issuer;

    @BeforeEach
    void setUp() {
        sensors = mock(SensorControlRepository.class);
        endpoints = mock(EntityManagementEndpointRepository.class);
        grants = mock(SensorPollGrantRepository.class);
        audit = mock(SecurityAuditEmitter.class);
        TransactionSynchronizationManager.initSynchronization();
        TransactionSynchronizationManager.setActualTransactionActive(true);
        issuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(ISSUED, ZoneOffset.UTC),
                () -> GRANT);
    }

    @AfterEach
    void tearDown() {
        TransactionSynchronizationManager.clear();
    }

    @Test
    void allowAllIssuesPersistedRowAndAuditsOnlyAfterCommitWithoutUnsafeMetadata() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(active(true));
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT))
                .thenReturn(Optional.of(activeEndpoint("2001:db8::10")));
        SensorPollGrant persisted = samplePersisted("2001:db8::10", SensorPollGrant.AddressType.IPV6);
        when(grants.insert(any())).thenReturn(persisted);

        SensorPollGrant row = issuer.issue(request(Protocol.SNMP, 1L, 0L,
                "planned-profile", "v1", ISSUED.minusSeconds(1), ISSUED.plusSeconds(60)));

        assertSame(persisted, row);
        assertEquals(Status.ISSUED, row.status());
        verify(sensors, never()).listAllowedEntityIds(any(), any());
        verify(sensors).lockPrincipalForShare(TENANT, SENSOR);
        verify(sensors, never()).lockPrincipalForUpdate(any(), any());
        verify(grants).requireHeldCallerTransaction("issue");
        verify(grants).insert(any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());

        invokeAfterCommitCallbacks();
        assertAuditSafeMetadata(persisted);
    }

    @Test
    void allowlistIssuesWhenEntityPresent() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(active(false));
        when(sensors.listAllowedEntityIds(TENANT, SENSOR)).thenReturn(Set.of(ENTITY));
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT))
                .thenReturn(Optional.of(activeEndpoint("192.0.2.10")));
        SensorPollGrant persisted = new SensorPollGrant(
                GRANT, TENANT, SENSOR, ENTITY, ENDPOINT, 1L, 0L, Protocol.ICMP,
                SensorPollGrant.AddressType.IPV4, "192.0.2.10",
                "planned-profile", "v1", ISSUED, ISSUED.minusSeconds(1), ISSUED.plusSeconds(60),
                "internal-actor", Status.ISSUED, null, null, null, null, ISSUED);
        when(grants.insert(any())).thenReturn(persisted);

        SensorPollGrant row = issuer.issue(request(Protocol.ICMP, 1L, 0L,
                "planned-profile", "v1", ISSUED.minusSeconds(1), ISSUED.plusSeconds(60)));

        assertEquals(Protocol.ICMP, row.protocol());
        assertEquals(SensorPollGrant.AddressType.IPV4, row.issuedAddressType());
        verify(grants).insert(any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
        invokeAfterCommitCallbacks();
        verify(audit).emitControlEvent(any(), eq(AuditActions.SENSOR_POLL_GRANT_ISSUE),
                eq(AuditOutcome.SUCCESS), eq(TENANT), eq("SENSOR_POLL_GRANT_ISSUED"), any());
    }

    @Test
    void coreTimeIsSampledOnlyAfterPrincipalAndEndpointLocks() {
        AtomicBoolean clockSampled = new AtomicBoolean(false);
        AtomicBoolean locksHeld = new AtomicBoolean(false);
        Clock gated = new Clock() {
            @Override
            public Instant instant() {
                assertTrue(locksHeld.get(), "Core time must be sampled only after both locks");
                clockSampled.set(true);
                return ISSUED;
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
        issuer = new SensorPollGrantIssuer(sensors, endpoints, grants, audit, gated, () -> GRANT);
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenAnswer(invocation -> {
            assertFalse(clockSampled.get());
            return active(true);
        });
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT)).thenAnswer(invocation -> {
            assertFalse(clockSampled.get());
            locksHeld.set(true);
            return Optional.of(activeEndpoint("2001:db8::10"));
        });
        when(grants.insert(any())).thenAnswer(invocation -> {
            assertTrue(clockSampled.get());
            return samplePersisted("2001:db8::10", SensorPollGrant.AddressType.IPV6);
        });

        issuer.issue(validRequest());
        assertTrue(clockSampled.get());
        verify(grants).requireHeldCallerTransaction("issue");
    }

    @Test
    void windowExpiryAfterLockWaitRejectsWithoutInsertOrAudit() {
        AtomicReference<Instant> now = new AtomicReference<>(ISSUED);
        Clock mutable = new Clock() {
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
        issuer = new SensorPollGrantIssuer(sensors, endpoints, grants, audit, mutable, () -> GRANT);
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenAnswer(invocation -> {
            now.set(ISSUED.plusSeconds(120));
            return active(true);
        });
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT))
                .thenReturn(Optional.of(activeEndpoint("2001:db8::10")));

        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> issuer.issue(request(Protocol.SNMP, 1L, 0L, "planned-profile", "v1",
                        ISSUED.minusSeconds(1), ISSUED.plusSeconds(60))));
        assertEquals(SensorControlException.ERR_POLL_GRANT_WINDOW_INVALID, failure.getCode());
        verify(grants, never()).insert(any());
        assertTrue(TransactionSynchronizationManager.getSynchronizations().isEmpty());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
    }

    @Test
    void missingSynchronizationFailsClosedBeforeAuthorityOrMutation() {
        TransactionSynchronizationManager.clear();
        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> issuer.issue(validRequest()));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, failure.getCode());
        assertTrue(failure.getMessage().toLowerCase().contains("synchronization"));
        verify(sensors, never()).lockPrincipalForShare(any(), any());
        verify(grants, never()).insert(any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
    }

    @Test
    void requireHeldCallerTransactionFailureBlocksBeforeLocks() {
        doThrow(new SensorControlException(
                SensorControlException.ERR_SENSOR_STATE_INVALID, "Active PostgreSQL transaction required for issue"))
                .when(grants).requireHeldCallerTransaction("issue");

        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> issuer.issue(validRequest()));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, failure.getCode());
        verify(sensors, never()).lockPrincipalForShare(any(), any());
        verify(grants, never()).insert(any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
    }

    @Test
    void missingSensorFailsClosedWithoutInsertOrAudit() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenThrow(new SensorControlException(
                SensorControlException.ERR_SENSOR_NOT_FOUND, "Sensor not found"));

        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> issuer.issue(validRequest()));
        assertEquals(SensorControlException.ERR_SENSOR_NOT_FOUND, failure.getCode());
        verify(grants, never()).insert(any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
        verify(endpoints, never()).findByTenantEntityAndIdForShare(any(), any(), any());
        assertTrue(TransactionSynchronizationManager.getSynchronizations().isEmpty());
    }

    @Test
    void inactiveSensorFailsClosedWithoutInsertOrAudit() {
        SensorPrincipal disabled = new SensorPrincipal(
                SENSOR, TENANT, null, SensorRole.NETWORK, SensorLifecycleStatus.DISABLED,
                true, ISSUED, ISSUED);
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(disabled);

        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> issuer.issue(validRequest()));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, failure.getCode());
        verify(grants, never()).insert(any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
    }

    @Test
    void entityOutsideAllowlistFailsClosed() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(active(false));
        when(sensors.listAllowedEntityIds(TENANT, SENSOR)).thenReturn(Set.of(UUID.randomUUID()));

        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> issuer.issue(validRequest()));
        assertEquals(SensorControlException.ERR_POLL_GRANT_ENTITY_OUT_OF_SCOPE, failure.getCode());
        verify(endpoints, never()).findByTenantEntityAndIdForShare(any(), any(), any());
        verify(grants, never()).insert(any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
    }

    @Test
    void missingAndInactiveEndpointShareUnavailableCode() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(active(true));
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT))
                .thenReturn(Optional.empty());
        assertEquals(SensorControlException.ERR_POLL_GRANT_ENDPOINT_UNAVAILABLE,
                assertThrows(SensorControlException.class, () -> issuer.issue(validRequest())).getCode());

        EntityManagementEndpoint inactive = new EntityManagementEndpoint(
                ENDPOINT, TENANT, ENTITY, ManagementEndpointAddressType.FQDN, "host.example",
                ManagementEndpointState.INACTIVE, ManagementEndpointSource.MANUAL, false,
                "fixture", ISSUED, "fixture", ISSUED);
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT))
                .thenReturn(Optional.of(inactive));
        assertEquals(SensorControlException.ERR_POLL_GRANT_ENDPOINT_UNAVAILABLE,
                assertThrows(SensorControlException.class, () -> issuer.issue(validRequest())).getCode());
        verify(grants, never()).insert(any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
    }

    @Test
    void invalidWindowGenerationsAndPlanFailBeforeAuthority() {
        assertEquals(SensorControlException.ERR_POLL_GRANT_WINDOW_INVALID,
                assertThrows(SensorControlException.class, () -> issuer.issue(
                        request(Protocol.SNMP, 1L, 0L, "planned-profile", "v1",
                                ISSUED.plusSeconds(10), ISSUED))).getCode());
        assertEquals(SensorControlException.ERR_POLL_GRANT_GENERATION_INVALID,
                assertThrows(SensorControlException.class, () -> issuer.issue(
                        request(Protocol.SNMP, 0L, 0L, "planned-profile", "v1",
                                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60)))).getCode());
        assertEquals(SensorControlException.ERR_POLL_GRANT_GENERATION_INVALID,
                assertThrows(SensorControlException.class, () -> issuer.issue(
                        request(Protocol.SNMP, 1L, -1L, "planned-profile", "v1",
                                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60)))).getCode());
        assertEquals(SensorControlException.ERR_POLL_GRANT_PLAN_INVALID,
                assertThrows(SensorControlException.class, () -> issuer.issue(
                        request(Protocol.SNMP, 1L, 0L, " ", "v1",
                                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60)))).getCode());
        verify(sensors, never()).lockPrincipalForShare(any(), any());
        verify(grants, never()).requireHeldCallerTransaction(any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
    }

    @Test
    void typedInvalidActorAndRequiredRequestFields() {
        assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                assertThrows(SensorControlException.class, () -> issuer.issue(null)).getCode());
        assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                assertThrows(SensorControlException.class, () -> issuer.issue(
                        new SensorPollGrantIssuer.IssueRequest(
                                null, SENSOR, ENTITY, ENDPOINT, Protocol.SNMP,
                                1L, 0L, "planned-profile", "v1",
                                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), "actor"))).getCode());
        assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                assertThrows(SensorControlException.class, () -> issuer.issue(
                        new SensorPollGrantIssuer.IssueRequest(
                                TENANT, SENSOR, ENTITY, ENDPOINT, Protocol.SNMP,
                                1L, 0L, "planned-profile", "v1",
                                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), null))).getCode());
        assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                assertThrows(SensorControlException.class, () -> issuer.issue(
                        new SensorPollGrantIssuer.IssueRequest(
                                TENANT, SENSOR, ENTITY, ENDPOINT, Protocol.SNMP,
                                1L, 0L, "planned-profile", "v1",
                                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), ""))).getCode());
        assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                assertThrows(SensorControlException.class, () -> issuer.issue(
                        new SensorPollGrantIssuer.IssueRequest(
                                TENANT, SENSOR, ENTITY, ENDPOINT, null,
                                1L, 0L, "planned-profile", "v1",
                                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), "actor"))).getCode());
        assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                assertThrows(SensorControlException.class, () -> issuer.issue(
                        new SensorPollGrantIssuer.IssueRequest(
                                TENANT, SENSOR, ENTITY, ENDPOINT, Protocol.SNMP,
                                1L, 0L, "planned-profile", "v1",
                                null, ISSUED.plusSeconds(60), "actor"))).getCode());
        verify(sensors, never()).lockPrincipalForShare(any(), any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
    }

    @Test
    void whitespaceOnlyActorsRejectWithoutTrimOrAuthority() {
        for (String actor : List.of(" ", "\t", "\n", " \t\n\r ")) {
            assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                    assertThrows(SensorControlException.class, () -> issuer.issue(
                            new SensorPollGrantIssuer.IssueRequest(
                                    TENANT, SENSOR, ENTITY, ENDPOINT, Protocol.SNMP,
                                    1L, 0L, "planned-profile", "v1",
                                    ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), actor))).getCode(),
                    "whitespace-only actor must be request-invalid: " + actor.codePoints().toArray());
        }
        // Nonblank opaque actor with surrounding whitespace remains permitted (no trim).
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(active(true));
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT))
                .thenReturn(Optional.of(activeEndpoint("2001:db8::10")));
        when(grants.insert(any())).thenAnswer(invocation -> invocation.getArgument(0));
        SensorPollGrant row = issuer.issue(new SensorPollGrantIssuer.IssueRequest(
                TENANT, SENSOR, ENTITY, ENDPOINT, Protocol.SNMP,
                1L, 0L, "planned-profile", "v1",
                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60), " internal-actor "));
        assertEquals(" internal-actor ", row.issuedBy());
        verify(sensors).lockPrincipalForShare(TENANT, SENSOR);
        verify(grants).insert(any());
    }

    @Test
    void unsupportedWindowTimestampsRejectBeforeAuthority() {
        Instant justBelowMin = SensorPollGrantIssuer.PG_TIMESTAMPTZ_MIN.minusNanos(1);
        Instant exclusiveUpper = SensorPollGrantIssuer.PG_TIMESTAMPTZ_EXCLUSIVE_END;
        Instant pastExclusive = exclusiveUpper.plusNanos(1);
        // PostgreSQL internal Julian-day-zero bound; earlier than pgJDBC finite OffsetDateTime min.
        Instant postgresqlInternalJulianDayZero = Instant.ofEpochSecond(-210866803200L);
        assertTrue(postgresqlInternalJulianDayZero.isBefore(SensorPollGrantIssuer.PG_TIMESTAMPTZ_MIN));

        assertWindowRejectedBeforeAuthority(Instant.MIN, Instant.MIN.plusSeconds(60));
        assertWindowRejectedBeforeAuthority(ISSUED.minusSeconds(1), Instant.MAX);
        assertWindowRejectedBeforeAuthority(justBelowMin, SensorPollGrantIssuer.PG_TIMESTAMPTZ_MIN.plusSeconds(60));
        assertWindowRejectedBeforeAuthority(ISSUED.minusSeconds(1), justBelowMin);
        assertWindowRejectedBeforeAuthority(
                postgresqlInternalJulianDayZero, postgresqlInternalJulianDayZero.plusSeconds(60));
        assertWindowRejectedBeforeAuthority(ISSUED.minusSeconds(1), postgresqlInternalJulianDayZero);
        assertWindowRejectedBeforeAuthority(exclusiveUpper, exclusiveUpper.plusSeconds(1));
        assertWindowRejectedBeforeAuthority(ISSUED.minusSeconds(1), exclusiveUpper);
        assertWindowRejectedBeforeAuthority(pastExclusive, pastExclusive.plusSeconds(1));
        assertWindowRejectedBeforeAuthority(ISSUED.minusSeconds(1), pastExclusive);

        verify(sensors, never()).lockPrincipalForShare(any(), any());
        verify(grants, never()).requireHeldCallerTransaction(any());
        verify(endpoints, never()).findByTenantEntityAndIdForShare(any(), any(), any());
        verify(grants, never()).insert(any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
    }

    private void assertWindowRejectedBeforeAuthority(Instant windowStart, Instant windowEnd) {
        assertEquals(SensorControlException.ERR_POLL_GRANT_WINDOW_INVALID,
                assertThrows(SensorControlException.class, () -> issuer.issue(
                        request(Protocol.SNMP, 1L, 0L, "planned-profile", "v1",
                                windowStart, windowEnd))).getCode(),
                "window " + windowStart + " .. " + windowEnd);
    }

    @Test
    void exactMinimumAndMaximumAcceptedWindowReachInsert() {
        Instant min = SensorPollGrantIssuer.PG_TIMESTAMPTZ_MIN;
        Instant maxAccepted = SensorPollGrantIssuer.PG_TIMESTAMPTZ_MAX_ACCEPTED;
        Instant nearMin = min.plusSeconds(1);
        Instant nearMax = maxAccepted.minusSeconds(1);

        issuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(nearMin, ZoneOffset.UTC),
                () -> GRANT);
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(active(true));
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT))
                .thenReturn(Optional.of(activeEndpoint("2001:db8::10")));
        when(grants.insert(any())).thenAnswer(invocation -> invocation.getArgument(0));
        SensorPollGrant minRow = issuer.issue(request(Protocol.SNMP, 1L, 0L, "planned-profile", "v1",
                min, min.plusSeconds(60)));
        assertEquals(min, minRow.windowStart());
        assertEquals(nearMin, minRow.issuedAt());

        clearInvocations(sensors, endpoints, grants, audit);
        issuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(nearMax, ZoneOffset.UTC),
                () -> GRANT);
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(active(true));
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT))
                .thenReturn(Optional.of(activeEndpoint("2001:db8::10")));
        when(grants.insert(any())).thenAnswer(invocation -> invocation.getArgument(0));
        SensorPollGrant maxRow = issuer.issue(request(Protocol.SNMP, 1L, 0L, "planned-profile", "v1",
                maxAccepted.minusSeconds(60), maxAccepted));
        assertEquals(maxAccepted, maxRow.windowEnd());
        assertEquals(nearMax, maxRow.issuedAt());
        verify(grants, times(1)).insert(any());
    }

    @Test
    void issuedAtOutsidePersistenceEnvelopeFailsAsInternalState() {
        Instant impossible = Instant.MAX;
        Clock broken = new Clock() {
            @Override
            public Instant instant() {
                return impossible;
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
        issuer = new SensorPollGrantIssuer(sensors, endpoints, grants, audit, broken, () -> GRANT);
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(active(true));
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT))
                .thenReturn(Optional.of(activeEndpoint("2001:db8::10")));

        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> issuer.issue(validRequest()));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, failure.getCode());
        assertFalse(failure.getMessage().toLowerCase().contains("timestamp"));
        assertFalse(failure.getMessage().toLowerCase().contains("jdbc"));
        assertFalse(failure.getMessage().toLowerCase().contains("illegal"));
        verify(grants, never()).insert(any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
        assertTrue(TransactionSynchronizationManager.getSynchronizations().isEmpty());
    }

    @Test
    void nullGeneratedGrantIdFailsAsInternalStateWithoutInsertOrAudit() {
        issuer = new SensorPollGrantIssuer(
                sensors, endpoints, grants, audit,
                Clock.fixed(ISSUED, ZoneOffset.UTC),
                () -> null);
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(active(true));
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT))
                .thenReturn(Optional.of(activeEndpoint("2001:db8::10")));

        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> issuer.issue(validRequest()));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, failure.getCode());
        assertNotEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID, failure.getCode());
        verify(grants, never()).insert(any());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
        assertTrue(TransactionSynchronizationManager.getSynchronizations().isEmpty());
    }

    @Test
    void duplicateInsertMapsToConflictWithoutSuccessAudit() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(active(true));
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT))
                .thenReturn(Optional.of(activeEndpoint("2001:db8::10")));
        when(grants.insert(any())).thenThrow(new DuplicateKeyException("fixture"));

        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> issuer.issue(validRequest()));
        assertEquals(SensorControlException.ERR_POLL_GRANT_CONFLICT, failure.getCode());
        assertFalse(failure.getMessage().toLowerCase().contains("duplicate"));
        assertFalse(failure.getMessage().toLowerCase().contains("sql"));
        assertTrue(TransactionSynchronizationManager.getSynchronizations().isEmpty());
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
    }

    @Test
    void rollbackOnlyPathDoesNotEmitRegisteredAudit() {
        when(sensors.lockPrincipalForShare(TENANT, SENSOR)).thenReturn(active(true));
        when(endpoints.findByTenantEntityAndIdForShare(TENANT, ENTITY, ENDPOINT))
                .thenReturn(Optional.of(activeEndpoint("2001:db8::10")));
        when(grants.insert(any())).thenReturn(samplePersisted("2001:db8::10", SensorPollGrant.AddressType.IPV6));

        issuer.issue(validRequest());
        assertEquals(1, TransactionSynchronizationManager.getSynchronizations().size());
        for (TransactionSynchronization sync : TransactionSynchronizationManager.getSynchronizations()) {
            sync.afterCompletion(TransactionSynchronization.STATUS_ROLLED_BACK);
        }
        verify(audit, never()).emitControlEvent(any(), any(), any(), any(), any(), any());
    }

    @Test
    void noDefaultOrSeededGrantWithoutExplicitIssueCall() {
        verifyNoInteractions(sensors, endpoints, grants, audit);
        assertEquals("sensor.poll_grant.issue", AuditActions.SENSOR_POLL_GRANT_ISSUE.value());
        assertFalse(SensorPollGrantIssuer.class.isAnnotationPresent(
                org.springframework.stereotype.Service.class));
        assertFalse(SensorPollGrantIssuer.class.isAnnotationPresent(
                org.springframework.web.bind.annotation.RestController.class));
        assertFalse(SensorPollGrantIssuer.class.isAnnotationPresent(
                org.springframework.scheduling.annotation.Scheduled.class));
    }

    private void invokeAfterCommitCallbacks() {
        List<TransactionSynchronization> syncs =
                List.copyOf(TransactionSynchronizationManager.getSynchronizations());
        assertEquals(1, syncs.size(), "exactly one afterCommit registration");
        for (TransactionSynchronization sync : syncs) {
            sync.afterCommit();
        }
    }

    private void assertAuditSafeMetadata(SensorPollGrant row) {
        @SuppressWarnings("unchecked")
        ArgumentCaptor<Map<String, String>> meta = ArgumentCaptor.forClass(Map.class);
        verify(audit).emitControlEvent(
                eq(AuditCategory.SECURITY_ADMINISTRATION),
                eq(AuditActions.SENSOR_POLL_GRANT_ISSUE),
                eq(AuditOutcome.SUCCESS),
                eq(TENANT),
                eq("SENSOR_POLL_GRANT_ISSUED"),
                meta.capture());
        Map<String, String> values = meta.getValue();
        assertEquals(Set.of(
                "grant_id", "sensor_id", "entity_id", "endpoint_id",
                "protocol", "grant_generation", "assignment_generation"), values.keySet());
        assertEquals(row.grantId().toString(), values.get("grant_id"));
        assertEquals(SENSOR, values.get("sensor_id"));
        assertFalse(values.containsKey("issued_by"));
        assertFalse(values.containsKey("profile_plan_id"));
        assertFalse(values.containsKey("profile_plan_version"));
        assertFalse(values.containsKey("issued_address"));
        assertFalse(values.containsKey("address"));
        assertFalse(values.values().stream().anyMatch(v -> v.contains("2001:db8")));
        assertFalse(values.values().stream().anyMatch(v -> v.toLowerCase().contains("secret")));
    }

    private SensorPrincipal active(boolean allowAll) {
        return new SensorPrincipal(
                SENSOR, TENANT, null, SensorRole.NETWORK, SensorLifecycleStatus.ACTIVE,
                allowAll, ISSUED, ISSUED);
    }

    private EntityManagementEndpoint activeEndpoint(String address) {
        ManagementEndpointAddressType type = address.contains(":")
                ? ManagementEndpointAddressType.IPV6
                : ManagementEndpointAddressType.IPV4;
        return new EntityManagementEndpoint(
                ENDPOINT, TENANT, ENTITY, type, address, ManagementEndpointState.ACTIVE,
                ManagementEndpointSource.MANUAL, true, "fixture", ISSUED, "fixture", ISSUED);
    }

    private SensorPollGrant samplePersisted(String address, SensorPollGrant.AddressType type) {
        return new SensorPollGrant(
                GRANT, TENANT, SENSOR, ENTITY, ENDPOINT, 1L, 0L, Protocol.SNMP, type, address,
                "planned-profile", "v1", ISSUED, ISSUED.minusSeconds(1), ISSUED.plusSeconds(60),
                "internal-actor", Status.ISSUED, null, null, null, null, ISSUED);
    }

    private SensorPollGrantIssuer.IssueRequest validRequest() {
        return request(Protocol.SNMP, 1L, 0L, "planned-profile", "v1",
                ISSUED.minusSeconds(1), ISSUED.plusSeconds(60));
    }

    private static SensorPollGrantIssuer.IssueRequest request(
            Protocol protocol,
            long grantGeneration,
            long assignmentGeneration,
            String planId,
            String planVersion,
            Instant windowStart,
            Instant windowEnd
    ) {
        return new SensorPollGrantIssuer.IssueRequest(
                TENANT, SENSOR, ENTITY, ENDPOINT, protocol,
                grantGeneration, assignmentGeneration, planId, planVersion,
                windowStart, windowEnd, "internal-actor");
    }
}
