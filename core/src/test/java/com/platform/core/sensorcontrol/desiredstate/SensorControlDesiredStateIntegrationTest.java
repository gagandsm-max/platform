package com.platform.core.sensorcontrol.desiredstate;

import com.platform.core.control.ControlGenerationLimits;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.SensorControlDesiredStateRepository;
import com.platform.core.persistence.TenantRepository;
import com.platform.core.security.authorization.AuthenticationState;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.http.PlatformAuthenticationToken;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.security.identity.AuthenticatedExternalIdentity;
import com.platform.core.security.identity.AuthenticationAssurance;
import com.platform.core.security.identity.ExternalSubject;
import com.platform.core.security.identity.IdentityProviderType;
import com.platform.core.security.identity.PlatformSubject;
import com.platform.core.security.identity.ProviderInstanceId;
import com.platform.core.security.identity.ResolvedAuthenticatedIdentity;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.sensorcontrol.SensorControlService;
import com.platform.core.sensorcontrol.SensorRole;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.security.core.context.SecurityContextHolder;

import java.time.Instant;
import java.util.EnumMap;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

@Tag("postgres")
class SensorControlDesiredStateIntegrationTest {

    private PostgresTestSupport pg;
    private JdbcTemplate jdbc;
    private TenantService tenants;
    private SensorControlService sensors;
    private SensorControlDesiredStateService service;
    private Tenant tenantA;
    private Tenant tenantB;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        NamedParameterJdbcTemplate named = pg.namedJdbc();
        jdbc = pg.jdbc();
        tenants = new TenantService(new TenantRepository(named));
        sensors = new SensorControlService(new SensorControlRepository(named), new SecurityAuditEmitter());
        service = new SensorControlDesiredStateService(
                new SensorControlDesiredStateRepository(named), new SecurityAuditEmitter());
        tenantA = tenants.create("Control Desired A", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        tenantB = tenants.create("Control Desired B", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        authenticateAs("control-actor");
    }

    @AfterEach
    void tearDown() {
        SecurityContextHolder.clearContext();
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void mutationIdempotencyExhaustionAndAckMatrix() {
        sensors.enrollSensor("sensor-a", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        SensorRuntimeDesiredConfig cfg1 = sampleConfig(2, 8);
        SensorRuntimeDesiredConfig cfg2 = sampleConfig(3, 8);

        SensorControlState first = service.setDesiredConfiguration(tenantA.tenantId(), "sensor-a", 0L, cfg1);
        assertEquals(1L, first.desiredGeneration());
        assertEquals(SensorControlDesiredChecksum.compute(cfg1), first.desiredChecksum());
        assertEquals("control-actor", first.desiredBy());
        assertEquals(0L, first.appliedGeneration());

        SensorControlState same = service.setDesiredConfiguration(tenantA.tenantId(), "sensor-a", 1L, cfg1);
        assertEquals(1L, same.desiredGeneration());
        assertEquals(first.desiredChecksum(), same.desiredChecksum());

        SensorControlState changed = service.setDesiredConfiguration(tenantA.tenantId(), "sensor-a", 1L, cfg2);
        assertEquals(2L, changed.desiredGeneration());
        assertNotEquals(first.desiredChecksum(), changed.desiredChecksum());

        DomainException stale = assertThrows(DomainException.class,
                () -> service.setDesiredConfiguration(tenantA.tenantId(), "sensor-a", 1L, cfg1));
        assertEquals(SensorControlDesiredStateService.ERR_GENERATION_CONFLICT, stale.getCode());

        long maxSafe = ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION;
        jdbc.update("""
                UPDATE platform.sensor_control_state
                   SET desired_generation = ?
                 WHERE tenant_id = ? AND sensor_id = 'sensor-a'
                """, maxSafe, tenantA.tenantId());
        SensorControlState atMax = service.setDesiredConfiguration(
                tenantA.tenantId(), "sensor-a", maxSafe, cfg2);
        assertEquals(maxSafe, atMax.desiredGeneration());

        DomainException exhausted = assertThrows(DomainException.class,
                () -> service.setDesiredConfiguration(tenantA.tenantId(), "sensor-a", maxSafe, cfg1));
        assertEquals(ControlGenerationLimits.ERR_CONTROL_GENERATION_EXHAUSTED, exhausted.getCode());
        assertEquals(maxSafe,
                service.findDesiredState(tenantA.tenantId(), "sensor-a").orElseThrow().desiredGeneration());
        assertEquals(SensorControlDesiredChecksum.compute(cfg2),
                service.findDesiredState(tenantA.tenantId(), "sensor-a").orElseThrow().desiredChecksum());

        // Reset to generation 2 for ACK matrix
        jdbc.update("""
                UPDATE platform.sensor_control_state
                   SET desired_generation = 2,
                       desired_checksum = ?,
                       desired_config_json = CAST(? AS jsonb),
                       applied_generation = 0, applied_checksum = NULL, applied_at = NULL,
                       observed_generation = 0, observed_checksum = NULL, observed_outcome = NULL,
                       rejection_code = NULL, rejection_message = NULL, observed_at = NULL
                 WHERE tenant_id = ? AND sensor_id = 'sensor-a'
                """,
                SensorControlDesiredChecksum.compute(cfg2),
                SensorControlDesiredConfigJson.toJson(cfg2),
                tenantA.tenantId());

        String checksum = SensorControlDesiredChecksum.compute(cfg2);
        SensorControlState rejected = service.acknowledgeAppliedState(
                tenantA.tenantId(), "sensor-a", 1, 2L, checksum,
                SensorControlObservationOutcome.REJECTED, "CONFIG_REJECTED", "bad pool");
        assertEquals(0L, rejected.appliedGeneration());
        assertEquals(2L, rejected.observedGeneration());
        assertEquals(SensorControlObservationOutcome.REJECTED, rejected.observedOutcome());
        assertEquals("CONFIG_REJECTED", rejected.rejectionCode());

        SensorControlState dupReject = service.acknowledgeAppliedState(
                tenantA.tenantId(), "sensor-a", 1, 2L, checksum,
                SensorControlObservationOutcome.REJECTED, "CONFIG_REJECTED", "bad pool");
        assertEquals(SensorControlObservationOutcome.REJECTED, dupReject.observedOutcome());

        DomainException conflictReject = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-a", 1, 2L, checksum,
                        SensorControlObservationOutcome.REJECTED, "OTHER", null));
        assertEquals(SensorControlDesiredStateService.ERR_OUTCOME_CONFLICT, conflictReject.getCode());

        SensorControlState applied = service.acknowledgeAppliedState(
                tenantA.tenantId(), "sensor-a", 1, 2L, checksum,
                SensorControlObservationOutcome.APPLIED, null, null);
        assertEquals(2L, applied.appliedGeneration());
        assertEquals(checksum, applied.appliedChecksum());
        assertEquals(SensorControlObservationOutcome.APPLIED, applied.observedOutcome());
        assertNull(applied.rejectionCode());
        assertEquals(SensorControlReconciliationState.CURRENT, applied.reconciliationState());

        SensorControlState dupApplied = service.acknowledgeAppliedState(
                tenantA.tenantId(), "sensor-a", 1, 2L, checksum,
                SensorControlObservationOutcome.APPLIED, null, null);
        assertEquals(2L, dupApplied.appliedGeneration());
        assertEquals(SensorControlReconciliationState.CURRENT, dupApplied.reconciliationState());

        SensorControlState restartRejected = service.acknowledgeAppliedState(
                tenantA.tenantId(), "sensor-a", 1, 2L, checksum,
                SensorControlObservationOutcome.REJECTED, "SENSOR_CONTROL_RESTART_REQUIRED",
                "scheduler drift");
        assertEquals(2L, restartRejected.appliedGeneration());
        assertEquals(checksum, restartRejected.appliedChecksum());
        assertEquals(2L, restartRejected.observedGeneration());
        assertEquals(checksum, restartRejected.observedChecksum());
        assertEquals(SensorControlObservationOutcome.REJECTED, restartRejected.observedOutcome());
        assertEquals("SENSOR_CONTROL_RESTART_REQUIRED", restartRejected.rejectionCode());
        assertEquals(SensorControlReconciliationState.REJECTED, restartRejected.reconciliationState());

        SensorControlState dupRestartRejected = service.acknowledgeAppliedState(
                tenantA.tenantId(), "sensor-a", 1, 2L, checksum,
                SensorControlObservationOutcome.REJECTED, "SENSOR_CONTROL_RESTART_REQUIRED",
                "scheduler drift");
        assertEquals(SensorControlObservationOutcome.REJECTED, dupRestartRejected.observedOutcome());
        assertEquals(SensorControlReconciliationState.REJECTED, dupRestartRejected.reconciliationState());

        DomainException conflictAfterRestart = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-a", 1, 2L, checksum,
                        SensorControlObservationOutcome.REJECTED, "OTHER_CODE", null));
        assertEquals(SensorControlDesiredStateService.ERR_OUTCOME_CONFLICT, conflictAfterRestart.getCode());

        SensorControlState recovered = service.acknowledgeAppliedState(
                tenantA.tenantId(), "sensor-a", 1, 2L, checksum,
                SensorControlObservationOutcome.APPLIED, null, null);
        assertEquals(2L, recovered.appliedGeneration());
        assertEquals(SensorControlObservationOutcome.APPLIED, recovered.observedOutcome());
        assertEquals(SensorControlReconciliationState.CURRENT, recovered.reconciliationState());

        DomainException staleAck = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-a", 1, 1L, checksum,
                        SensorControlObservationOutcome.APPLIED, null, null));
        assertEquals(SensorControlDesiredStateService.ERR_ACK_STALE, staleAck.getCode());

        DomainException futureAck = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-a", 1, 3L, checksum,
                        SensorControlObservationOutcome.APPLIED, null, null));
        assertEquals(SensorControlDesiredStateService.ERR_ACK_FUTURE, futureAck.getCode());

        DomainException mismatch = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-a", 1, 2L, "a".repeat(64),
                        SensorControlObservationOutcome.APPLIED, null, null));
        assertEquals(SensorControlDesiredStateService.ERR_CHECKSUM_MISMATCH, mismatch.getCode());

        DomainException schema = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-a", 2, 2L, checksum,
                        SensorControlObservationOutcome.APPLIED, null, null));
        assertEquals(SensorControlDesiredStateService.ERR_SCHEMA_UNSUPPORTED, schema.getCode());

        DomainException genZero = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-a", 1, 0L, checksum,
                        SensorControlObservationOutcome.APPLIED, null, null));
        assertEquals("VALIDATION_FAILED", genZero.getCode());

        DomainException genOver = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-a", 1, maxSafe + 1L, checksum,
                        SensorControlObservationOutcome.APPLIED, null, null));
        assertEquals("VALIDATION_FAILED", genOver.getCode());

        DomainException badChecksum = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-a", 1, 2L, "ZZ",
                        SensorControlObservationOutcome.APPLIED, null, null));
        assertEquals("VALIDATION_FAILED", badChecksum.getCode());

        DomainException rejectNoCode = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-a", 1, 2L, checksum,
                        SensorControlObservationOutcome.REJECTED, null, null));
        assertEquals("VALIDATION_FAILED", rejectNoCode.getCode());

        DomainException appliedWithReject = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-a", 1, 2L, checksum,
                        SensorControlObservationOutcome.APPLIED, "X", null));
        assertEquals("VALIDATION_FAILED", appliedWithReject.getCode());

        // REJECTED must preserve prior successfully applied generation.
        SensorRuntimeDesiredConfig cfg3 = sampleConfig(4, 8);
        SensorControlState desired3 = service.setDesiredConfiguration(
                tenantA.tenantId(), "sensor-a", 2L, cfg3);
        assertEquals(3L, desired3.desiredGeneration());
        assertEquals(2L, desired3.appliedGeneration());
        String checksum3 = SensorControlDesiredChecksum.compute(cfg3);
        SensorControlState rejectedNewer = service.acknowledgeAppliedState(
                tenantA.tenantId(), "sensor-a", 1, 3L, checksum3,
                SensorControlObservationOutcome.REJECTED, "CONFIG_REJECTED", "pool too large");
        assertEquals(2L, rejectedNewer.appliedGeneration());
        assertEquals(checksum, rejectedNewer.appliedChecksum());
        assertEquals(3L, rejectedNewer.observedGeneration());
        assertEquals(SensorControlObservationOutcome.REJECTED, rejectedNewer.observedOutcome());
        assertEquals(SensorControlReconciliationState.REJECTED, rejectedNewer.reconciliationState());
    }

    @Test
    void inactiveAndRevokedSensorsRejectMutation() {
        sensors.enrollSensor("sensor-inactive", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        sensors.disableSensor("sensor-inactive");
        DomainException inactive = assertThrows(DomainException.class,
                () -> service.setDesiredConfiguration(
                        tenantA.tenantId(), "sensor-inactive", 0L, sampleConfig(1, 1)));
        assertEquals(SensorControlDesiredStateService.ERR_SENSOR_NOT_ACTIVE, inactive.getCode());

        sensors.enrollSensor("sensor-rev", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        sensors.revokeSensor("sensor-rev");
        DomainException revoked = assertThrows(DomainException.class,
                () -> service.setDesiredConfiguration(
                        tenantA.tenantId(), "sensor-rev", 0L, sampleConfig(1, 1)));
        assertEquals(SensorControlDesiredStateService.ERR_SENSOR_NOT_ACTIVE, revoked.getCode());
    }

    @Test
    void disabledAndRevokedSensorsRejectAckWithoutMutatingState() {
        SensorRuntimeDesiredConfig cfg = sampleConfig(2, 8);
        String checksum = SensorControlDesiredChecksum.compute(cfg);

        sensors.enrollSensor("sensor-ack-dis", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        SensorControlState desiredDis = service.setDesiredConfiguration(
                tenantA.tenantId(), "sensor-ack-dis", 0L, cfg);
        assertEquals(1L, desiredDis.desiredGeneration());
        assertEquals(0L, desiredDis.appliedGeneration());
        assertEquals(0L, desiredDis.observedGeneration());

        sensors.disableSensor("sensor-ack-dis");
        DomainException disabledAck = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-ack-dis", 1, 1L, checksum,
                        SensorControlObservationOutcome.APPLIED, null, null));
        assertEquals(SensorControlDesiredStateService.ERR_SENSOR_NOT_ACTIVE, disabledAck.getCode());
        SensorControlState afterDisable = service.findDesiredState(tenantA.tenantId(), "sensor-ack-dis")
                .orElseThrow();
        assertEquals(1L, afterDisable.desiredGeneration());
        assertEquals(0L, afterDisable.appliedGeneration());
        assertNull(afterDisable.appliedChecksum());
        assertEquals(0L, afterDisable.observedGeneration());
        assertNull(afterDisable.observedOutcome());

        sensors.enrollSensor("sensor-ack-rev", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        SensorControlState desiredRev = service.setDesiredConfiguration(
                tenantA.tenantId(), "sensor-ack-rev", 0L, cfg);
        assertEquals(1L, desiredRev.desiredGeneration());

        sensors.revokeSensor("sensor-ack-rev");
        DomainException revokedAck = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-ack-rev", 1, 1L, checksum,
                        SensorControlObservationOutcome.REJECTED, "CONFIG_REJECTED", null));
        assertEquals(SensorControlDesiredStateService.ERR_SENSOR_NOT_ACTIVE, revokedAck.getCode());
        SensorControlState afterRevoke = service.findDesiredState(tenantA.tenantId(), "sensor-ack-rev")
                .orElseThrow();
        assertEquals(1L, afterRevoke.desiredGeneration());
        assertEquals(0L, afterRevoke.appliedGeneration());
        assertNull(afterRevoke.appliedChecksum());
        assertEquals(0L, afterRevoke.observedGeneration());
        assertNull(afterRevoke.observedOutcome());
        assertNull(afterRevoke.rejectionCode());
    }

    @Test
    void databaseConstraintsRejectInvalidControlRows() {
        sensors.enrollSensor("sensor-db", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        UUID tenantId = tenantA.tenantId();
        String goodJson = SensorControlDesiredConfigJson.toJson(sampleConfig(1, 1));
        String checksum = "a".repeat(64);
        long over = ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION + 1L;

        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_control_state (
                  tenant_id, sensor_id, desired_schema_version, desired_generation, desired_checksum,
                  desired_config_json, desired_by, desired_at, applied_generation, observed_generation, updated_at)
                VALUES (?, 'sensor-db', 1, ?, ?, CAST(? AS jsonb), 'x', NOW(), 0, 0, NOW())
                """, tenantId, over, checksum, goodJson));

        jdbc.update("""
                INSERT INTO platform.sensor_control_state (
                  tenant_id, sensor_id, desired_schema_version, desired_generation, desired_checksum,
                  desired_config_json, desired_by, desired_at, applied_generation, observed_generation, updated_at)
                VALUES (?, 'sensor-db', 1, 1, ?, CAST(? AS jsonb), 'x', NOW(), 0, 0, NOW())
                """, tenantId, checksum, goodJson);

        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_control_state SET applied_generation = ? WHERE sensor_id='sensor-db'
                """, over));
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_control_state SET observed_generation = ? WHERE sensor_id='sensor-db'
                """, over));
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_control_state SET applied_generation = 2 WHERE sensor_id='sensor-db'
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_control_state SET observed_generation = 2 WHERE sensor_id='sensor-db'
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_control_state SET desired_checksum = 'ZZ' WHERE sensor_id='sensor-db'
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_control_state
                   SET applied_generation = 1, applied_checksum = 'GG'
                 WHERE sensor_id='sensor-db'
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_control_state
                   SET observed_generation = 1, observed_checksum = 'hh', observed_outcome = 'NOPE'
                 WHERE sensor_id='sensor-db'
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_control_state
                   SET observed_generation = 1, observed_checksum = ?, observed_outcome = 'REJECTED',
                       rejection_code = NULL
                 WHERE sensor_id='sensor-db'
                """, checksum));
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_control_state
                   SET observed_generation = 1, observed_checksum = ?, observed_outcome = 'APPLIED',
                       rejection_code = 'X'
                 WHERE sensor_id='sensor-db'
                """, checksum));
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.sensor_control_state
                   SET desired_config_json = CAST('"not-object"' AS jsonb)
                 WHERE sensor_id='sensor-db'
                """));
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_control_state (
                  tenant_id, sensor_id, desired_schema_version, desired_generation, desired_checksum,
                  desired_config_json, desired_by, desired_at, applied_generation, observed_generation, updated_at)
                VALUES (?, 'sensor-db', 1, 1, ?, CAST(? AS jsonb), 'x', NOW(), 0, 0, NOW())
                """, tenantB.tenantId(), checksum, goodJson));
    }

    @Test
    void appliedThenRestartRequiredRejectedThenAppliedRecovery() {
        sensors.enrollSensor("sensor-restart", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        SensorRuntimeDesiredConfig cfg = sampleConfig(2, 8);
        SensorControlState desired = service.setDesiredConfiguration(
                tenantA.tenantId(), "sensor-restart", 0L, cfg);
        long generation = desired.desiredGeneration();
        String checksum = desired.desiredChecksum();

        SensorControlState applied = service.acknowledgeAppliedState(
                tenantA.tenantId(), "sensor-restart", 1, generation, checksum,
                SensorControlObservationOutcome.APPLIED, null, null);
        assertEquals(SensorControlReconciliationState.CURRENT, applied.reconciliationState());
        assertEquals(generation, applied.appliedGeneration());

        SensorControlState rejected = service.acknowledgeAppliedState(
                tenantA.tenantId(), "sensor-restart", 1, generation, checksum,
                SensorControlObservationOutcome.REJECTED, "SENSOR_CONTROL_RESTART_REQUIRED",
                "runtime scheduler drift");
        assertEquals(generation, rejected.appliedGeneration());
        assertEquals(checksum, rejected.appliedChecksum());
        assertEquals(generation, rejected.observedGeneration());
        assertEquals(checksum, rejected.observedChecksum());
        assertEquals(SensorControlObservationOutcome.REJECTED, rejected.observedOutcome());
        assertEquals("SENSOR_CONTROL_RESTART_REQUIRED", rejected.rejectionCode());
        assertEquals(SensorControlReconciliationState.REJECTED, rejected.reconciliationState());

        SensorControlState dup = service.acknowledgeAppliedState(
                tenantA.tenantId(), "sensor-restart", 1, generation, checksum,
                SensorControlObservationOutcome.REJECTED, "SENSOR_CONTROL_RESTART_REQUIRED",
                "runtime scheduler drift");
        assertEquals(SensorControlReconciliationState.REJECTED, dup.reconciliationState());

        DomainException conflict = assertThrows(DomainException.class,
                () -> service.acknowledgeAppliedState(
                        tenantA.tenantId(), "sensor-restart", 1, generation, checksum,
                        SensorControlObservationOutcome.REJECTED, "OTHER", "different"));
        assertEquals(SensorControlDesiredStateService.ERR_OUTCOME_CONFLICT, conflict.getCode());

        SensorControlState recovered = service.acknowledgeAppliedState(
                tenantA.tenantId(), "sensor-restart", 1, generation, checksum,
                SensorControlObservationOutcome.APPLIED, null, null);
        assertEquals(SensorControlObservationOutcome.APPLIED, recovered.observedOutcome());
        assertEquals(SensorControlReconciliationState.CURRENT, recovered.reconciliationState());
        assertEquals(generation, recovered.appliedGeneration());
    }

    private static SensorRuntimeDesiredConfig sampleConfig(int workers, int queueDepth) {
        EnumMap<SensorSchedulerPoolName, SensorSchedulerPoolConfig> pools =
                new EnumMap<>(SensorSchedulerPoolName.class);
        pools.put(SensorSchedulerPoolName.NETWORK_POLL, new SensorSchedulerPoolConfig(workers, queueDepth));
        return new SensorRuntimeDesiredConfig(
                SensorLogLevel.INFO,
                new SensorSchedulerDesiredConfig(
                        Math.max(workers, 4), Math.max(queueDepth, 16), 32, 64, 1_048_576,
                        5_000, 10_000, 3, 100, 1_000, 0, pools),
                new SensorTransportDesiredConfig(15_000, 1_048_576, 1_048_576, 100, 524_288)
        );
    }

    private void authenticateAs(String subjectId) {
        Instant now = Instant.parse("2026-06-01T00:00:00Z");
        PrincipalContext principal = new PrincipalContext(
                subjectId, AuthenticationState.AUTHENTICATED, Set.of(), Set.of());
        UUID subjectUuid = UUID.fromString("018f0e0a-0000-7000-8000-0000000000aa");
        SecurityContextHolder.getContext().setAuthentication(
                new PlatformAuthenticationToken(
                        ResolvedAuthenticatedIdentity.current(
                                new PlatformSubject(subjectUuid),
                                new AuthenticatedExternalIdentity(
                                        IdentityProviderType.OIDC,
                                        new ProviderInstanceId("platform-oidc"),
                                        new ExternalSubject(subjectId),
                                        now,
                                        AuthenticationAssurance.withoutVerifiedMfa(),
                                        Instant.parse("2099-01-01T00:00:00Z")
                                ),
                                now
                        ),
                        principal
                )
        );
    }
}
