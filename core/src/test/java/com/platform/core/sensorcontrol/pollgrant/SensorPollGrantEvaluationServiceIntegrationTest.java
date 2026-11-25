package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.config.PersistenceConfig;
import com.platform.core.ingestion.authority.ResolvedSensorAuthority;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.sensorcontrol.SensorControlException;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.aop.support.AopUtils;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.support.TransactionSynchronizationManager;
import org.springframework.transaction.support.TransactionTemplate;

import javax.sql.DataSource;
import java.sql.Timestamp;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicReference;

import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Protocol;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.RevocationClass;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Status;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrantEligibility.Decision;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrantRepository.Termination;
import static org.junit.jupiter.api.Assertions.*;

/**
 * PostgreSQL evidence for the Issue #229 G1b Spring evaluation seam.
 * Does not wire REST, companion delivery, scheduler or typed admission.
 */
@Tag("postgres")
class SensorPollGrantEvaluationServiceIntegrationTest {

    private static final Instant ISSUED = Instant.parse("2026-11-20T12:00:00.123456Z");
    private static final String FINGERPRINT =
            "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd";

    private PostgresTestSupport pg;
    private AnnotationConfigApplicationContext context;
    private SensorPollGrantEvaluationService service;
    private SensorPollGrantAccess access;
    private SensorPollGrantRepository grants;

    private final UUID tenant = UUID.randomUUID();
    private final UUID otherTenant = UUID.randomUUID();
    private final UUID entity = UUID.randomUUID();
    private final UUID otherEntity = UUID.randomUUID();
    private final UUID endpoint = UUID.randomUUID();
    private final UUID otherEndpoint = UUID.randomUUID();
    private final UUID grant = UUID.randomUUID();
    private final UUID credential = UUID.randomUUID();
    private final String sensor = "eval-" + UUID.randomUUID();
    private final String otherSensor = "eval-other-" + UUID.randomUUID();
    private boolean fixtureOwned;

    @BeforeEach
    void start() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();

        SensorControlRepository sensors = new SensorControlRepository(pg.namedJdbc());
        SensorPollGrantRepository grantRepository = new SensorPollGrantRepository(pg.namedJdbc());
        // Fixed clock for deterministic window/eligibility; production bean uses system UTC.
        SensorPollGrantAccess accessBean = new SensorPollGrantAccess(
                sensors, grantRepository, Clock.fixed(ISSUED, ZoneOffset.UTC));

        context = new AnnotationConfigApplicationContext();
        context.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
        context.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
        context.register(PersistenceConfig.class);
        context.registerBean(SensorControlRepository.class, () -> sensors);
        context.registerBean(SensorPollGrantRepository.class, () -> grantRepository);
        context.registerBean(SensorPollGrantAccess.class, () -> accessBean);
        context.registerBean(SensorPollGrantEvaluationService.class);
        context.refresh();

        service = context.getBean(SensorPollGrantEvaluationService.class);
        access = context.getBean(SensorPollGrantAccess.class);
        grants = context.getBean(SensorPollGrantRepository.class);
        assertTrue(AopUtils.isAopProxy(service));

        pg.tx().executeWithoutResult(tx -> {
            for (UUID id : List.of(tenant, otherTenant)) {
                pg.jdbc().update("""
                        INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                        VALUES (?, ?, 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                        """, id, "eval-fixture-" + id);
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
                        VALUES (?, ?, 'NETWORK_DEVICE', 'eval-fixture', 'ACTIVE', NOW(), NOW(), 'fixture', NOW(), NOW())
                        """, id, tenant);
            }
            pg.jdbc().update("""
                    INSERT INTO platform.sensor_allowed_entity (tenant_id, sensor_id, entity_id, created_at)
                    VALUES (?, ?, ?, NOW())
                    """, tenant, sensor, entity);
            pg.jdbc().update("""
                    INSERT INTO platform.entity_management_endpoint (endpoint_id, tenant_id, entity_id,
                        address_type, address, state, source, is_primary, created_by, created_at, updated_by, updated_at)
                    VALUES (?, ?, ?, 'IPV4', '192.0.2.91', 'ACTIVE', 'MANUAL', TRUE, 'fixture', NOW(), 'fixture', NOW())
                    """, endpoint, tenant, entity);
            pg.jdbc().update("""
                    INSERT INTO platform.entity_management_endpoint (endpoint_id, tenant_id, entity_id,
                        address_type, address, state, source, is_primary, created_by, created_at, updated_by, updated_at)
                    VALUES (?, ?, ?, 'IPV4', '192.0.2.92', 'ACTIVE', 'MANUAL', TRUE, 'fixture', NOW(), 'fixture', NOW())
                    """, otherEndpoint, tenant, otherEntity);
            pg.jdbc().update("""
                    INSERT INTO platform.sensor_certificate_credential (
                        credential_id, tenant_id, sensor_id, fingerprint_sha256, serial_number,
                        issuer_dn, not_before, not_after, status, activated_at, created_at, updated_at)
                    VALUES (?, ?, ?, ?, 'serial-eval', 'CN=fixture-ca',
                            ?::timestamptz - INTERVAL '1 hour',
                            ?::timestamptz + INTERVAL '30 days',
                            'ACTIVE', ?, ?, ?)
                    """, credential, tenant, sensor, FINGERPRINT,
                    Timestamp.from(ISSUED), Timestamp.from(ISSUED),
                    Timestamp.from(ISSUED), Timestamp.from(ISSUED), Timestamp.from(ISSUED));
            insertIssuedGrant(grant, entity, endpoint, "192.0.2.91", Protocol.ICMP, 3L, 1L);
        });
        fixtureOwned = true;
    }

    @AfterEach
    void stop() {
        try {
            if (context != null) {
                context.close();
            }
        } finally {
            context = null;
        }
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
    void springProxyOpensTransactionAndReturnsEligibleIssuedGrant() {
        assertFalse(TransactionSynchronizationManager.isActualTransactionActive());

        SensorPollGrantEligibility outcome = service.evaluateForNewExecution(authority(), grant);

        assertTrue(outcome.eligible());
        assertEquals(Decision.ELIGIBLE, outcome.decision());
        assertEquals(grant, outcome.grant().grantId());
        assertEquals(Protocol.ICMP, outcome.grant().protocol());
        assertEquals(ISSUED, outcome.evaluatedAt());
        assertFalse(TransactionSynchronizationManager.isActualTransactionActive());
    }

    @Test
    void missingGrantIsNotVisibleUnderAuthenticatedScope() {
        SensorPollGrantEligibility outcome =
                service.evaluateForNewExecution(authority(), UUID.randomUUID());
        assertEquals(Decision.NOT_VISIBLE, outcome.decision());
        assertNull(outcome.grant());
        assertNull(outcome.evaluatedAt());
    }

    @Test
    void invalidSensorScopeIsNotVisibleWithoutGrantSnapshot() {
        UUID otherCredential = UUID.randomUUID();
        String otherFingerprint =
                "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee";
        pg.tx().executeWithoutResult(tx -> pg.jdbc().update("""
                INSERT INTO platform.sensor_certificate_credential (
                    credential_id, tenant_id, sensor_id, fingerprint_sha256, serial_number,
                    issuer_dn, not_before, not_after, status, activated_at, created_at, updated_at)
                VALUES (?, ?, ?, ?, 'serial-eval-other', 'CN=fixture-ca',
                        ?::timestamptz - INTERVAL '1 hour',
                        ?::timestamptz + INTERVAL '30 days',
                        'ACTIVE', ?, ?, ?)
                """, otherCredential, tenant, otherSensor, otherFingerprint,
                Timestamp.from(ISSUED), Timestamp.from(ISSUED),
                Timestamp.from(ISSUED), Timestamp.from(ISSUED), Timestamp.from(ISSUED)));

        ResolvedSensorAuthority otherSensorAuthority = new ResolvedSensorAuthority(
                new SensorScope(otherSensor, tenant, null, Set.of(entity), true),
                otherCredential,
                otherFingerprint);

        SensorPollGrantEligibility outcome =
                service.evaluateForNewExecution(otherSensorAuthority, grant);

        assertEquals(Decision.NOT_VISIBLE, outcome.decision());
        assertNull(outcome.grant());
    }

    @Test
    void entityOutsideLiveAllowlistIsDeniedUnderStrictIntersection() {
        UUID outOfScopeGrant = UUID.randomUUID();
        pg.tx().executeWithoutResult(tx ->
                insertIssuedGrant(outOfScopeGrant, otherEntity, otherEndpoint, "192.0.2.92",
                        Protocol.SNMP, 1L, 0L));

        SensorPollGrantEligibility outcome =
                service.evaluateForNewExecution(authority(), outOfScopeGrant);

        assertEquals(Decision.ENTITY_OUT_OF_SCOPE, outcome.decision());
        assertNull(outcome.grant());
    }

    @Test
    void staleRevokeDeniesNewExecutionWithoutGrantSnapshot() {
        pg.tx().executeWithoutResult(tx -> grants.terminate(
                tenant, sensor, grant,
                new Termination(Status.REVOKED, ISSUED.plusSeconds(1), "revoker",
                        RevocationClass.SECURITY, "security revoke")));

        SensorPollGrantEligibility outcome = service.evaluateForNewExecution(authority(), grant);

        assertEquals(Decision.NOT_ISSUED, outcome.decision());
        assertNull(outcome.grant());
        assertNull(outcome.evaluatedAt());
    }

    @Test
    void noLockLessPathExistsOutsideSpringTransactionProxy() {
        SensorControlException direct = assertThrows(SensorControlException.class,
                () -> access.evaluateForNewExecution(
                        new SensorPollGrantAccess.LookupRequest(authority(), grant)));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, direct.getCode());

        SensorPollGrantEvaluationService unproxied =
                new SensorPollGrantEvaluationService(access);
        SensorControlException withoutProxy = assertThrows(SensorControlException.class,
                () -> unproxied.evaluateForNewExecution(authority(), grant));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, withoutProxy.getCode());

        // Proxied seam remains the only path that establishes the required locks/TX.
        assertTrue(service.evaluateForNewExecution(authority(), grant).eligible());
    }

    @Test
    void joiningOuterTransactionDoesNotOpenReplacementTransaction() {
        PlatformTransactionManager tm =
                context.getBean("transactionManager", PlatformTransactionManager.class);
        AtomicReference<Object> resourceDuringCall = new AtomicReference<>();

        SensorPollGrantEligibility outcome = new TransactionTemplate(tm).execute(status -> {
            Object bound = TransactionSynchronizationManager.getResource(pg.dataSource());
            assertNotNull(bound, "outer caller-held TX must bind the DataSource");
            resourceDuringCall.set(bound);
            SensorPollGrantEligibility result = service.evaluateForNewExecution(authority(), grant);
            assertSame(bound, TransactionSynchronizationManager.getResource(pg.dataSource()),
                    "evaluation must join the outer TX, not replace it");
            return result;
        });

        assertTrue(outcome.eligible());
        assertNotNull(resourceDuringCall.get());
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
                    'eval-plan', 'v1', ?, ?, ?, 'fixture-only', 'ISSUED', ?)
                """, grantId, tenant, sensor, entityId, endpointId,
                grantGeneration, assignmentGeneration, protocol.name(), address,
                Timestamp.from(ISSUED),
                Timestamp.from(ISSUED.minusSeconds(30)),
                Timestamp.from(ISSUED.plusSeconds(60)),
                Timestamp.from(ISSUED));
    }

    private ResolvedSensorAuthority authority() {
        return new ResolvedSensorAuthority(
                new SensorScope(sensor, tenant, null, Set.of(entity), false),
                credential,
                FINGERPRINT);
    }
}
