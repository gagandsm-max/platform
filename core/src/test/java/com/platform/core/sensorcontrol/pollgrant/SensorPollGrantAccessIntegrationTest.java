package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.ingestion.authority.ResolvedSensorAuthority;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.sensorcontrol.SensorControlException;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.sensorcontrol.SensorPrincipal;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;
import java.util.concurrent.locks.LockSupport;

import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.AddressType;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Protocol;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.RevocationClass;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Status;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrantEligibility.Decision;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrantRepository.Termination;
import static org.junit.jupiter.api.Assertions.*;

/**
 * PostgreSQL evidence for Issue #229 G1b read-only grant accessor.
 * Does not wire REST, companion delivery, scheduler or typed admission.
 * Cross-tenant / other-sensor fixtures and lock-serialization proofs use the
 * existing ephemeral {@link PostgresTestSupport} only — no shared customer DB.
 */
@Tag("postgres")
class SensorPollGrantAccessIntegrationTest {

    private static final Instant ISSUED = Instant.parse("2026-11-20T12:00:00.123456Z");
    private static final String FINGERPRINT =
            "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc";
    private static final String OTHER_SENSOR_FINGERPRINT =
            "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd";
    private static final String OTHER_TENANT_FINGERPRINT =
            "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee";

    private PostgresTestSupport pg;
    private SensorControlRepository sensors;
    private SensorPollGrantRepository grants;
    private SensorPollGrantAccess access;

    private final UUID tenant = UUID.randomUUID();
    private final UUID otherTenant = UUID.randomUUID();
    private final UUID entity = UUID.randomUUID();
    private final UUID otherEntity = UUID.randomUUID();
    private final UUID endpoint = UUID.randomUUID();
    private final UUID otherEndpoint = UUID.randomUUID();
    private final UUID grant = UUID.randomUUID();
    private final UUID otherTenantGrant = UUID.randomUUID();
    private final UUID credential = UUID.randomUUID();
    private final UUID otherSensorCredential = UUID.randomUUID();
    private final UUID otherTenantCredential = UUID.randomUUID();
    private final String sensor = "access-" + UUID.randomUUID();
    private final String otherSensor = "access-other-" + UUID.randomUUID();
    private final String otherTenantSensor = "access-xt-" + UUID.randomUUID();
    private boolean fixtureOwned;

    @BeforeEach
    void start() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        sensors = new SensorControlRepository(pg.namedJdbc());
        grants = new SensorPollGrantRepository(pg.namedJdbc());
        access = new SensorPollGrantAccess(
                sensors, grants, Clock.fixed(ISSUED, ZoneOffset.UTC));

        pg.tx().executeWithoutResult(tx -> {
            for (UUID id : List.of(tenant, otherTenant)) {
                pg.jdbc().update("""
                        INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                        VALUES (?, ?, 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                        """, id, "access-fixture-" + id);
            }
            pg.jdbc().update("""
                    INSERT INTO platform.sensor (sensor_id, tenant_id, role, status,
                        allow_all_tenant_entities, created_at, updated_at)
                    VALUES (?, ?, 'NETWORK', 'ACTIVE', FALSE, NOW(), NOW())
                    """, sensor, tenant);
            pg.jdbc().update("""
                    INSERT INTO platform.sensor (sensor_id, tenant_id, role, status,
                        allow_all_tenant_entities, created_at, updated_at)
                    VALUES (?, ?, 'NETWORK', 'ACTIVE', TRUE, NOW(), NOW())
                    """, otherSensor, tenant);
            pg.jdbc().update("""
                    INSERT INTO platform.sensor (sensor_id, tenant_id, role, status,
                        allow_all_tenant_entities, created_at, updated_at)
                    VALUES (?, ?, 'NETWORK', 'ACTIVE', TRUE, NOW(), NOW())
                    """, otherTenantSensor, otherTenant);
            for (UUID id : List.of(entity, otherEntity)) {
                pg.jdbc().update("""
                        INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name,
                            lifecycle_state, first_seen_at, last_seen_at, created_by, created_at, updated_at)
                        VALUES (?, ?, 'NETWORK_DEVICE', 'access-fixture', 'ACTIVE', NOW(), NOW(), 'fixture', NOW(), NOW())
                        """, id, tenant);
            }
            UUID otherTenantEntity = UUID.randomUUID();
            pg.jdbc().update("""
                    INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name,
                        lifecycle_state, first_seen_at, last_seen_at, created_by, created_at, updated_at)
                    VALUES (?, ?, 'NETWORK_DEVICE', 'access-xt-fixture', 'ACTIVE', NOW(), NOW(), 'fixture', NOW(), NOW())
                    """, otherTenantEntity, otherTenant);
            pg.jdbc().update("""
                    INSERT INTO platform.sensor_allowed_entity (tenant_id, sensor_id, entity_id, created_at)
                    VALUES (?, ?, ?, NOW())
                    """, tenant, sensor, entity);
            pg.jdbc().update("""
                    INSERT INTO platform.entity_management_endpoint (endpoint_id, tenant_id, entity_id,
                        address_type, address, state, source, is_primary, created_by, created_at, updated_by, updated_at)
                    VALUES (?, ?, ?, 'IPV4', '192.0.2.77', 'ACTIVE', 'MANUAL', TRUE, 'fixture', NOW(), 'fixture', NOW())
                    """, endpoint, tenant, entity);
            pg.jdbc().update("""
                    INSERT INTO platform.entity_management_endpoint (endpoint_id, tenant_id, entity_id,
                        address_type, address, state, source, is_primary, created_by, created_at, updated_by, updated_at)
                    VALUES (?, ?, ?, 'IPV4', '192.0.2.88', 'ACTIVE', 'MANUAL', TRUE, 'fixture', NOW(), 'fixture', NOW())
                    """, otherEndpoint, tenant, otherEntity);
            UUID otherTenantEndpoint = UUID.randomUUID();
            pg.jdbc().update("""
                    INSERT INTO platform.entity_management_endpoint (endpoint_id, tenant_id, entity_id,
                        address_type, address, state, source, is_primary, created_by, created_at, updated_by, updated_at)
                    VALUES (?, ?, ?, 'IPV4', '192.0.2.99', 'ACTIVE', 'MANUAL', TRUE, 'fixture', NOW(), 'fixture', NOW())
                    """, otherTenantEndpoint, otherTenant, otherTenantEntity);
            insertEnrolledCredential(credential, tenant, sensor, FINGERPRINT);
            insertEnrolledCredential(otherSensorCredential, tenant, otherSensor, OTHER_SENSOR_FINGERPRINT);
            insertEnrolledCredential(otherTenantCredential, otherTenant, otherTenantSensor, OTHER_TENANT_FINGERPRINT);
            insertIssuedGrant(grant, tenant, sensor, entity, endpoint, "192.0.2.77", Protocol.ICMP, 3L, 1L);
            insertIssuedGrant(otherTenantGrant, otherTenant, otherTenantSensor, otherTenantEntity,
                    otherTenantEndpoint, "192.0.2.99", Protocol.ICMP, 1L, 0L);
        });
        fixtureOwned = true;
    }

    @AfterEach
    void stop() {
        try {
            if (fixtureOwned) {
                pg.tx().executeWithoutResult(tx -> {
                    pg.jdbc().update("DELETE FROM platform.sensor_poll_grant WHERE tenant_id IN (?, ?)",
                            tenant, otherTenant);
                    pg.jdbc().update(
                            "DELETE FROM platform.sensor_certificate_credential WHERE tenant_id IN (?, ?)",
                            tenant, otherTenant);
                    pg.jdbc().update("DELETE FROM platform.sensor_allowed_entity WHERE tenant_id IN (?, ?)",
                            tenant, otherTenant);
                    pg.jdbc().update("DELETE FROM platform.entity_management_endpoint WHERE tenant_id IN (?, ?)",
                            tenant, otherTenant);
                    pg.jdbc().update("DELETE FROM platform.entity WHERE tenant_id IN (?, ?)",
                            tenant, otherTenant);
                    pg.jdbc().update("DELETE FROM platform.sensor WHERE tenant_id IN (?, ?)",
                            tenant, otherTenant);
                    for (UUID id : List.of(tenant, otherTenant)) {
                        pg.jdbc().update("DELETE FROM platform.tenant WHERE tenant_id = ?", id);
                    }
                });
            }
        } finally {
            if (pg != null) {
                pg.stop();
            }
        }
    }

    @Test
    void authenticatesScopedLookupAndReturnsPersistedIssuedGrant() {
        SensorPollGrantEligibility outcome = pg.tx().execute(status ->
                access.evaluateForNewExecution(lookup(authority())));

        assertTrue(outcome.eligible());
        assertEquals(grant, outcome.grant().grantId());
        assertEquals(Protocol.ICMP, outcome.grant().protocol());
        assertEquals("192.0.2.77", outcome.grant().issuedAddress());
        assertEquals(3L, outcome.grant().grantGeneration());
        assertEquals(1L, outcome.grant().assignmentGeneration());
        assertEquals(ISSUED, outcome.evaluatedAt());
    }

    @Test
    void missingGrantIsNotVisibleUnderAuthenticatedScope() {
        assertEquals(Decision.NOT_VISIBLE, pg.tx().execute(status ->
                access.evaluateForNewExecution(new SensorPollGrantAccess.LookupRequest(
                        authority(), UUID.randomUUID(), null))).decision());
    }

    @Test
    void sameTenantOtherSensorCannotSeeGrant() {
        ResolvedSensorAuthority otherAuthority = new ResolvedSensorAuthority(
                new SensorScope(otherSensor, tenant, null, Set.of(), true),
                otherSensorCredential,
                OTHER_SENSOR_FINGERPRINT);
        assertEquals(Decision.NOT_VISIBLE, pg.tx().execute(status ->
                access.evaluateForNewExecution(new SensorPollGrantAccess.LookupRequest(
                        otherAuthority, grant, null))).decision());
    }

    @Test
    void otherTenantAuthorityCannotSeeLocalOrForeignGrantIds() {
        ResolvedSensorAuthority foreign = new ResolvedSensorAuthority(
                new SensorScope(otherTenantSensor, otherTenant, null, Set.of(), true),
                otherTenantCredential,
                OTHER_TENANT_FINGERPRINT);
        assertEquals(Decision.NOT_VISIBLE, pg.tx().execute(status ->
                access.evaluateForNewExecution(new SensorPollGrantAccess.LookupRequest(
                        foreign, grant, null))).decision(),
                "other-tenant probe of local grant id must not leak");
        assertEquals(Decision.ELIGIBLE, pg.tx().execute(status ->
                access.evaluateForNewExecution(new SensorPollGrantAccess.LookupRequest(
                        foreign, otherTenantGrant, null))).decision(),
                "control: foreign tenant may see its own enrolled grant");
        assertEquals(Decision.NOT_VISIBLE, pg.tx().execute(status ->
                access.evaluateForNewExecution(new SensorPollGrantAccess.LookupRequest(
                        authority(), otherTenantGrant, null))).decision(),
                "local authority must not observe other-tenant grant id");
    }

    @Test
    void revokeMakesGrantIneligibleForNewExecution() {
        pg.tx().executeWithoutResult(tx -> grants.terminate(
                tenant, sensor, grant,
                new Termination(Status.REVOKED, ISSUED.plusSeconds(1), "revoker",
                        RevocationClass.SECURITY, "security revoke")));

        assertEquals(Decision.NOT_ISSUED, pg.tx().execute(status ->
                access.evaluateForNewExecution(lookup(authority()))).decision());
    }

    @Test
    void expectedGenerationMismatchIsBindingFailure() {
        SensorPollGrantAccess.ExpectedBinding expected = new SensorPollGrantAccess.ExpectedBinding(
                entity, endpoint, Protocol.ICMP, 99L, 1L, "access-plan", "v1",
                AddressType.IPV4, "192.0.2.77");
        assertEquals(Decision.BINDING_MISMATCH, pg.tx().execute(status ->
                access.evaluateForNewExecution(new SensorPollGrantAccess.LookupRequest(
                        authority(), grant, expected))).decision());
    }

    @Test
    void matchingExpectedBindingRemainsEligible() {
        SensorPollGrantAccess.ExpectedBinding expected = new SensorPollGrantAccess.ExpectedBinding(
                entity, endpoint, Protocol.ICMP, 3L, 1L, "access-plan", "v1",
                AddressType.IPV4, "192.0.2.77");
        assertTrue(pg.tx().execute(status ->
                access.evaluateForNewExecution(new SensorPollGrantAccess.LookupRequest(
                        authority(), grant, expected))).eligible());
    }

    @Test
    void entityOutsideAllowlistDeniedEvenWhenGrantRowExists() {
        UUID outOfScopeGrant = UUID.randomUUID();
        pg.tx().executeWithoutResult(tx ->
                insertIssuedGrant(outOfScopeGrant, tenant, sensor, otherEntity, otherEndpoint, "192.0.2.88",
                        Protocol.SNMP, 1L, 0L));

        assertEquals(Decision.ENTITY_OUT_OF_SCOPE, pg.tx().execute(status ->
                access.evaluateForNewExecution(new SensorPollGrantAccess.LookupRequest(
                        authority(), outOfScopeGrant, null))).decision());
    }

    @Test
    void previouslyAuthorizedThenRevokedEntityIsDeniedUnderLiveIntersection() {
        // Scope snapshot still lists entity (stale resolved authority), but the live
        // allowlist row is deleted under the held principal fence.
        ResolvedSensorAuthority staleScopeStillListingEntity = authority();
        assertTrue(pg.tx().execute(status ->
                access.evaluateForNewExecution(lookup(staleScopeStillListingEntity))).eligible(),
                "fail-before baseline: entity must be eligible while still on the live allowlist");

        pg.tx().executeWithoutResult(tx -> pg.jdbc().update("""
                DELETE FROM platform.sensor_allowed_entity
                 WHERE tenant_id = ? AND sensor_id = ? AND entity_id = ?
                """, tenant, sensor, entity));

        assertEquals(Decision.ENTITY_OUT_OF_SCOPE, pg.tx().execute(status ->
                access.evaluateForNewExecution(lookup(staleScopeStillListingEntity))).decision(),
                "pass-after: stale scope hit must not bypass current deny-first allowlist");
    }

    @Test
    void windowInactiveWhenCoreClockOutsidePersistedWindow() {
        SensorPollGrantAccess expiredAccess = new SensorPollGrantAccess(
                sensors, grants, Clock.fixed(ISSUED.plusSeconds(120), ZoneOffset.UTC));
        assertEquals(Decision.WINDOW_INACTIVE, pg.tx().execute(status ->
                expiredAccess.evaluateForNewExecution(lookup(authority()))).decision());
    }

    @Test
    void inclusiveGrantWindowBoundariesRemainEligible() {
        SensorPollGrantAccess atStart = new SensorPollGrantAccess(
                sensors, grants, Clock.fixed(ISSUED.minusSeconds(30), ZoneOffset.UTC));
        assertTrue(pg.tx().execute(status ->
                atStart.evaluateForNewExecution(lookup(authority()))).eligible());

        SensorPollGrantAccess atEnd = new SensorPollGrantAccess(
                sensors, grants, Clock.fixed(ISSUED.plusSeconds(60), ZoneOffset.UTC));
        assertTrue(pg.tx().execute(status ->
                atEnd.evaluateForNewExecution(lookup(authority()))).eligible());
    }

    @Test
    void supersededCredentialIsInactiveWhetherGrantPresentOrMissing() {
        pg.tx().executeWithoutResult(tx -> pg.jdbc().update("""
                UPDATE platform.sensor_certificate_credential
                   SET status = 'SUPERSEDED', revoked_at = ?, updated_at = ?
                 WHERE credential_id = ? AND tenant_id = ?
                """, Timestamp.from(ISSUED), Timestamp.from(ISSUED), credential, tenant));

        assertEquals(Decision.CREDENTIAL_INACTIVE, pg.tx().execute(status ->
                access.evaluateForNewExecution(lookup(authority()))).decision());
        assertEquals(Decision.CREDENTIAL_INACTIVE, pg.tx().execute(status ->
                access.evaluateForNewExecution(new SensorPollGrantAccess.LookupRequest(
                        authority(), UUID.randomUUID(), null))).decision(),
                "missing grant must not change denial under SUPERSEDED credential");
    }

    @Test
    void revokedCredentialIsInactiveWhetherGrantPresentOrMissing() {
        pg.tx().executeWithoutResult(tx -> pg.jdbc().update("""
                UPDATE platform.sensor_certificate_credential
                   SET status = 'REVOKED', revoked_at = ?, updated_at = ?
                 WHERE credential_id = ? AND tenant_id = ?
                """, Timestamp.from(ISSUED), Timestamp.from(ISSUED), credential, tenant));

        assertEquals(Decision.CREDENTIAL_INACTIVE, pg.tx().execute(status ->
                access.evaluateForNewExecution(lookup(authority()))).decision());
        assertEquals(Decision.CREDENTIAL_INACTIVE, pg.tx().execute(status ->
                access.evaluateForNewExecution(new SensorPollGrantAccess.LookupRequest(
                        authority(), UUID.randomUUID(), null))).decision());
    }

    @Test
    void expiredCredentialIsInactiveWhetherGrantPresentOrMissing() {
        pg.tx().executeWithoutResult(tx -> pg.jdbc().update("""
                UPDATE platform.sensor_certificate_credential
                   SET not_after = ?::timestamptz - INTERVAL '1 second', updated_at = ?
                 WHERE credential_id = ? AND tenant_id = ?
                """, Timestamp.from(ISSUED), Timestamp.from(ISSUED), credential, tenant));

        assertEquals(Decision.CREDENTIAL_INACTIVE, pg.tx().execute(status ->
                access.evaluateForNewExecution(lookup(authority()))).decision());
        assertEquals(Decision.CREDENTIAL_INACTIVE, pg.tx().execute(status ->
                access.evaluateForNewExecution(new SensorPollGrantAccess.LookupRequest(
                        authority(), UUID.randomUUID(), null))).decision());
    }

    @Test
    void requiresCallerHeldTransaction() {
        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> access.evaluateForNewExecution(lookup(authority())));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, failure.getCode());
    }

    @Test
    void terminationHoldsGrantRowThenLookupWaitsAndSeesTerminalAfterCommit() throws Exception {
        CountDownLatch held = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        CountDownLatch aboutToLookup = new CountDownLatch(1);
        AtomicBoolean lookupCompleted = new AtomicBoolean(false);
        AtomicReference<SensorPollGrantEligibility> outcome = new AtomicReference<>();
        AtomicReference<Integer> holderPid = new AtomicReference<>();
        AtomicReference<Integer> contenderPid = new AtomicReference<>();
        ExecutorService pool = Executors.newFixedThreadPool(2);
        Future<?> holder = null;
        Future<?> lookup = null;
        try {
            holder = pool.submit(() -> {
                pg.tx().executeWithoutResult(status -> {
                    UUID lockedGrant = pg.jdbc().queryForObject("""
                            SELECT grant_id FROM platform.sensor_poll_grant
                             WHERE tenant_id = ? AND sensor_id = ? AND grant_id = ?
                             FOR UPDATE
                            """, UUID.class, tenant, sensor, grant);
                    assertEquals(grant, lockedGrant);
                    grants.terminate(tenant, sensor, grant,
                            new Termination(Status.REVOKED, ISSUED.plusSeconds(1), "revoker",
                                    RevocationClass.SECURITY, "held-revoke"));
                    holderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                    held.countDown();
                    await(release);
                });
                return null;
            });
            assertTrue(held.await(30, TimeUnit.SECONDS));
            assertNotNull(holderPid.get());

            lookup = pool.submit(() -> {
                SensorPollGrantEligibility result = pg.tx().execute(status -> {
                    contenderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                    aboutToLookup.countDown();
                    return access.evaluateForNewExecution(lookup(authority()));
                });
                outcome.set(result);
                lookupCompleted.set(true);
                return null;
            });

            assertTrue(aboutToLookup.await(30, TimeUnit.SECONDS));
            assertNotNull(contenderPid.get());
            assertNotEquals(holderPid.get(), contenderPid.get(),
                    "holder and lookup must be independent PostgreSQL backends");
            awaitDatabaseLockWait(contenderPid.get());
            assertFalse(lookupCompleted.get(),
                    "lookup must remain blocked while termination holds grant FOR UPDATE");
            release.countDown();
            holder.get(30, TimeUnit.SECONDS);
            holder = null;
            lookup.get(30, TimeUnit.SECONDS);
            lookup = null;
        } finally {
            release.countDown();
            shutdownPool(pool, holder, lookup);
        }
        assertEquals(Decision.NOT_ISSUED, outcome.get().decision());
    }

    @Test
    void heldEligibilityShareBlocksConcurrentTerminationUntilRelease() throws Exception {
        CountDownLatch held = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        CountDownLatch aboutToTerminate = new CountDownLatch(1);
        AtomicBoolean terminationCompleted = new AtomicBoolean(false);
        AtomicReference<SensorPollGrantRepository.TransitionOutcome> transition =
                new AtomicReference<>();
        AtomicReference<Integer> holderPid = new AtomicReference<>();
        AtomicReference<Integer> contenderPid = new AtomicReference<>();

        ExecutorService pool = Executors.newFixedThreadPool(2);
        Future<?> eligibility = null;
        Future<?> terminator = null;
        try {
            eligibility = pool.submit(() -> {
                pg.tx().execute(status -> {
                    SensorPollGrantEligibility result =
                            access.evaluateForNewExecution(lookup(authority()));
                    assertEquals(Decision.ELIGIBLE, result.decision());
                    holderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                    held.countDown();
                    await(release);
                    return result;
                });
                return null;
            });
            assertTrue(held.await(30, TimeUnit.SECONDS));
            assertNotNull(holderPid.get());

            terminator = pool.submit(() -> {
                SensorPollGrantRepository.TransitionOutcome result = pg.tx().execute(status -> {
                    contenderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                    aboutToTerminate.countDown();
                    return grants.terminate(tenant, sensor, grant,
                            new Termination(Status.SUPERSEDED, ISSUED.plusSeconds(2), "superseder",
                                    RevocationClass.SUPERSEDED, "wait-for-share"));
                });
                transition.set(result);
                terminationCompleted.set(true);
                return null;
            });

            assertTrue(aboutToTerminate.await(30, TimeUnit.SECONDS));
            assertNotNull(contenderPid.get());
            assertNotEquals(holderPid.get(), contenderPid.get());
            awaitDatabaseLockWait(contenderPid.get());
            assertFalse(terminationCompleted.get(),
                    "termination must wait while eligibility holds grant FOR SHARE");
            release.countDown();
            eligibility.get(30, TimeUnit.SECONDS);
            eligibility = null;
            terminator.get(30, TimeUnit.SECONDS);
            terminator = null;
        } finally {
            release.countDown();
            shutdownPool(pool, eligibility, terminator);
        }
        assertEquals(SensorPollGrantRepository.TransitionOutcome.TRANSITIONED, transition.get());
        assertEquals(Decision.NOT_ISSUED, pg.tx().execute(status ->
                access.evaluateForNewExecution(lookup(authority()))).decision());
    }

    @Test
    void principalForUpdateAllowlistRevokeThenStaleSnapshotLookupWaitsAndDenies() throws Exception {
        CountDownLatch held = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        CountDownLatch aboutToLookup = new CountDownLatch(1);
        AtomicBoolean lookupCompleted = new AtomicBoolean(false);
        AtomicReference<SensorPollGrantEligibility> outcome = new AtomicReference<>();
        AtomicReference<Integer> holderPid = new AtomicReference<>();
        AtomicReference<Integer> contenderPid = new AtomicReference<>();
        ResolvedSensorAuthority staleScope = authority();

        ExecutorService pool = Executors.newFixedThreadPool(2);
        Future<?> mutator = null;
        Future<?> lookup = null;
        try {
            mutator = pool.submit(() -> {
                pg.tx().executeWithoutResult(status -> {
                    SensorPrincipal locked = sensors.lockPrincipalForUpdate(tenant, sensor);
                    assertEquals(sensor, locked.sensorId());
                    pg.jdbc().update("""
                            DELETE FROM platform.sensor_allowed_entity
                             WHERE tenant_id = ? AND sensor_id = ? AND entity_id = ?
                            """, tenant, sensor, entity);
                    holderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                    held.countDown();
                    await(release);
                });
                return null;
            });
            assertTrue(held.await(30, TimeUnit.SECONDS));
            assertNotNull(holderPid.get());

            lookup = pool.submit(() -> {
                SensorPollGrantEligibility result = pg.tx().execute(status -> {
                    contenderPid.set(pg.jdbc().queryForObject("SELECT pg_backend_pid()", Integer.class));
                    aboutToLookup.countDown();
                    return access.evaluateForNewExecution(lookup(staleScope));
                });
                outcome.set(result);
                lookupCompleted.set(true);
                return null;
            });

            assertTrue(aboutToLookup.await(30, TimeUnit.SECONDS));
            assertNotNull(contenderPid.get());
            assertNotEquals(holderPid.get(), contenderPid.get());
            awaitDatabaseLockWait(contenderPid.get());
            assertFalse(lookupCompleted.get(),
                    "stale-scope lookup must wait on principal FOR UPDATE before deny");
            release.countDown();
            mutator.get(30, TimeUnit.SECONDS);
            mutator = null;
            lookup.get(30, TimeUnit.SECONDS);
            lookup = null;
        } finally {
            release.countDown();
            shutdownPool(pool, mutator, lookup);
        }
        assertEquals(Decision.ENTITY_OUT_OF_SCOPE, outcome.get().decision());
        assertNull(outcome.get().grant());
    }

    private void insertEnrolledCredential(
            UUID credentialId, UUID tenantId, String sensorId, String fingerprint) {
        pg.jdbc().update("""
                INSERT INTO platform.sensor_certificate_credential (
                    credential_id, tenant_id, sensor_id, fingerprint_sha256, serial_number,
                    issuer_dn, not_before, not_after, status, activated_at, created_at, updated_at)
                VALUES (?, ?, ?, ?, 'serial-access', 'CN=fixture-ca',
                        ?::timestamptz - INTERVAL '1 hour',
                        ?::timestamptz + INTERVAL '30 days',
                        'ACTIVE', ?, ?, ?)
                """, credentialId, tenantId, sensorId, fingerprint,
                Timestamp.from(ISSUED), Timestamp.from(ISSUED),
                Timestamp.from(ISSUED), Timestamp.from(ISSUED), Timestamp.from(ISSUED));
    }

    private void insertIssuedGrant(
            UUID grantId,
            UUID tenantId,
            String sensorId,
            UUID entityId,
            UUID endpointId,
            String address,
            Protocol protocol,
            long grantGeneration,
            long assignmentGeneration
    ) {
        pg.jdbc().update("""
                INSERT INTO platform.sensor_poll_grant (grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                    grant_generation, assignment_generation, protocol, issued_address_type, issued_address,
                    profile_plan_id, profile_plan_version, issued_at, window_start, window_end, issued_by,
                    status, updated_at)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'IPV4', ?,
                    'access-plan', 'v1', ?, ?, ?, 'fixture-only', 'ISSUED', ?)
                """, grantId, tenantId, sensorId, entityId, endpointId,
                grantGeneration, assignmentGeneration, protocol.name(), address,
                Timestamp.from(ISSUED),
                Timestamp.from(ISSUED.minusSeconds(30)),
                Timestamp.from(ISSUED.plusSeconds(60)),
                Timestamp.from(ISSUED));
    }

    private SensorPollGrantAccess.LookupRequest lookup(ResolvedSensorAuthority resolved) {
        return new SensorPollGrantAccess.LookupRequest(resolved, grant, null);
    }

    private ResolvedSensorAuthority authority() {
        return new ResolvedSensorAuthority(
                new SensorScope(sensor, tenant, null, Set.of(entity), false),
                credential,
                FINGERPRINT);
    }

    private Connection openContender() throws SQLException {
        Connection connection = pg.dataSource().getConnection();
        try {
            connection.setNetworkTimeout(Runnable::run, 5_000);
        } catch (SQLException | AbstractMethodError | UnsupportedOperationException ignored) {
            // statement queryTimeout remains mandatory below
        }
        return connection;
    }

    /**
     * Poll PostgreSQL until the exact contender backend is observed waiting on a
     * database lock. Time delay between polls is not itself proof of waiting.
     */
    private void awaitDatabaseLockWait(int backendPid) throws SQLException {
        long deadlineNanos = System.nanoTime() + TimeUnit.SECONDS.toNanos(30);
        try (Connection control = openContender()) {
            Integer controlPid = null;
            try (PreparedStatement pidStatement = control.prepareStatement("SELECT pg_backend_pid()")) {
                pidStatement.setQueryTimeout(5);
                try (ResultSet rs = pidStatement.executeQuery()) {
                    assertTrue(rs.next());
                    controlPid = rs.getInt(1);
                }
            }
            assertNotEquals(backendPid, controlPid,
                    "lock-wait control connection must be a distinct PostgreSQL backend");
            while (System.nanoTime() < deadlineNanos) {
                if (isBackendWaitingOnLock(control, backendPid)) {
                    return;
                }
                LockSupport.parkNanos(TimeUnit.MILLISECONDS.toNanos(25));
            }
        }
        fail("PostgreSQL never reported backend pid=" + backendPid
                + " waiting on Lock / ungranted pg_locks within timeout");
    }

    private static boolean isBackendWaitingOnLock(Connection control, int backendPid) throws SQLException {
        try (PreparedStatement statement = control.prepareStatement("""
                SELECT EXISTS (
                         SELECT 1 FROM pg_stat_activity
                          WHERE pid = ? AND wait_event_type = 'Lock'
                       )
                    OR EXISTS (
                         SELECT 1 FROM pg_locks
                          WHERE pid = ? AND NOT granted
                       )
                """)) {
            statement.setQueryTimeout(5);
            statement.setInt(1, backendPid);
            statement.setInt(2, backendPid);
            try (ResultSet result = statement.executeQuery()) {
                assertTrue(result.next());
                return result.getBoolean(1);
            }
        }
    }

    private static void await(CountDownLatch latch) {
        try {
            if (!latch.await(30, TimeUnit.SECONDS)) {
                throw new IllegalStateException("latch deadline exceeded");
            }
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new IllegalStateException("interrupted waiting for latch", e);
        }
    }

    private static void shutdownPool(ExecutorService pool, Future<?>... futures) throws InterruptedException {
        for (Future<?> future : futures) {
            if (future != null && !future.isDone()) {
                future.cancel(true);
            }
        }
        pool.shutdownNow();
        assertTrue(pool.awaitTermination(10, TimeUnit.SECONDS),
                "executor remained alive after shutdownNow/awaitTermination");
    }
}
