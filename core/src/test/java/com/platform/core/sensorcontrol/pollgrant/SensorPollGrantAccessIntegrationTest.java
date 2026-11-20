package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.ingestion.authority.ResolvedSensorAuthority;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.sensorcontrol.SensorControlException;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;

import java.sql.Timestamp;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;
import java.util.Set;
import java.util.UUID;

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
 */
@Tag("postgres")
class SensorPollGrantAccessIntegrationTest {

    private static final Instant ISSUED = Instant.parse("2026-11-20T12:00:00.123456Z");
    private static final String FINGERPRINT =
            "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc";

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
    private final UUID credential = UUID.randomUUID();
    private final String sensor = "access-" + UUID.randomUUID();
    private final String otherSensor = "access-other-" + UUID.randomUUID();
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
            for (UUID id : List.of(entity, otherEntity)) {
                pg.jdbc().update("""
                        INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name,
                            lifecycle_state, first_seen_at, last_seen_at, created_by, created_at, updated_at)
                        VALUES (?, ?, 'NETWORK_DEVICE', 'access-fixture', 'ACTIVE', NOW(), NOW(), 'fixture', NOW(), NOW())
                        """, id, tenant);
            }
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
            pg.jdbc().update("""
                    INSERT INTO platform.sensor_certificate_credential (
                        credential_id, tenant_id, sensor_id, fingerprint_sha256, serial_number,
                        issuer_dn, not_before, not_after, status, activated_at, created_at, updated_at)
                    VALUES (?, ?, ?, ?, 'serial-access', 'CN=fixture-ca',
                            ?::timestamptz - INTERVAL '1 hour',
                            ?::timestamptz + INTERVAL '30 days',
                            'ACTIVE', ?, ?, ?)
                    """, credential, tenant, sensor, FINGERPRINT,
                    Timestamp.from(ISSUED), Timestamp.from(ISSUED),
                    Timestamp.from(ISSUED), Timestamp.from(ISSUED), Timestamp.from(ISSUED));
            insertIssuedGrant(grant, entity, endpoint, "192.0.2.77", Protocol.ICMP, 3L, 1L);
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
                insertIssuedGrant(outOfScopeGrant, otherEntity, otherEndpoint, "192.0.2.88",
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
    void supersededCredentialIsInactive() {
        pg.tx().executeWithoutResult(tx -> pg.jdbc().update("""
                UPDATE platform.sensor_certificate_credential
                   SET status = 'SUPERSEDED', revoked_at = ?, updated_at = ?
                 WHERE credential_id = ? AND tenant_id = ?
                """, Timestamp.from(ISSUED), Timestamp.from(ISSUED), credential, tenant));

        assertEquals(Decision.CREDENTIAL_INACTIVE, pg.tx().execute(status ->
                access.evaluateForNewExecution(lookup(authority()))).decision());
    }

    @Test
    void requiresCallerHeldTransaction() {
        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> access.evaluateForNewExecution(lookup(authority())));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, failure.getCode());
    }

    private void insertIssuedGrant(
            UUID grantId,
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
                """, grantId, tenant, sensor, entityId, endpointId,
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
}
