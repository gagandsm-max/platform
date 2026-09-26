package com.platform.core.domain.capabilitypack;

import com.platform.core.control.ControlGenerationLimits;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.CapabilityPackRepository;
import com.platform.core.persistence.SensorCapabilityPackAssignmentRepository;
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
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Assignment cases A–J, activation fence, machine desired/artifact/ACK, and bounded E2E.
 */
@Tag("postgres")
class SensorCapabilityPackAssignmentIntegrationTest {

    private static final String PACK = "fixture.fictional.assign";

    private PostgresTestSupport pg;
    private JdbcTemplate jdbc;
    private TenantService tenants;
    private CapabilityPackCatalogService catalog;
    private SensorCapabilityPackAssignmentService assignments;
    private SensorControlService sensors;
    private Tenant tenantA;
    private Tenant tenantB;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        NamedParameterJdbcTemplate named = pg.namedJdbc();
        jdbc = pg.jdbc();
        TenantRepository tenantRepo = new TenantRepository(named);
        CapabilityPackRepository packRepo = new CapabilityPackRepository(named);
        SensorCapabilityPackAssignmentRepository assignRepo =
                new SensorCapabilityPackAssignmentRepository(named);
        SensorControlRepository sensorRepo = new SensorControlRepository(named);
        SecurityAuditEmitter audit = new SecurityAuditEmitter();
        tenants = new TenantService(tenantRepo);
        catalog = new CapabilityPackCatalogService(packRepo, assignRepo);
        assignments = new SensorCapabilityPackAssignmentService(assignRepo, packRepo, audit);
        sensors = new SensorControlService(sensorRepo, audit);
        tenantA = tenants.create("Assign Tenant A", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        tenantB = tenants.create("Assign Tenant B", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        authenticateAs("assign-actor");
    }

    @AfterEach
    void tearDown() {
        SecurityContextHolder.clearContext();
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void assignmentCasesAThroughJ() {
        sensors.enrollSensor("sensor-a", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        byte[] v1 = CapabilityPackTestSupport.minimalManifestBytes(PACK, "1.0.0", "AssignV1");
        byte[] v2 = CapabilityPackTestSupport.minimalManifestBytes(PACK, "2.0.0", "AssignV2");
        catalog.register(tenantA.tenantId(), PACK, "1.0.0", v1);
        catalog.register(tenantA.tenantId(), PACK, "2.0.0", v2);
        catalog.activate(tenantA.tenantId(), PACK, "1.0.0", 0L);

        // A: assign ACTIVE sensor + ACTIVE pack → gen 1
        var a = assignments.assign(tenantA.tenantId(), "sensor-a", PACK, "1.0.0", 0L);
        assertEquals(1L, a.desiredGeneration());
        assertEquals(1, a.assignments().size());
        assertEquals(64, a.desiredChecksum().length());

        // B: idempotent same assignment — no increment
        var b = assignments.assign(tenantA.tenantId(), "sensor-a", PACK, "1.0.0", 1L);
        assertEquals(1L, b.desiredGeneration());
        assertEquals(a.desiredChecksum(), b.desiredChecksum());

        // C: version change requires unassign first (activation fence blocks replace while assigned)
        DomainException fence = assertThrows(DomainException.class,
                () -> catalog.activate(tenantA.tenantId(), PACK, "2.0.0", 1L));
        assertEquals(SensorCapabilityPackAssignmentService.ERR_SENSOR_ASSIGNMENTS_EXIST, fence.getCode());

        // Unassign then activate v2 then assign new version
        assignments.unassign(tenantA.tenantId(), "sensor-a", PACK, 1L);
        catalog.activate(tenantA.tenantId(), PACK, "2.0.0", 1L);
        var c = assignments.assign(tenantA.tenantId(), "sensor-a", PACK, "2.0.0", 2L);
        assertEquals(3L, c.desiredGeneration());
        assertEquals("2.0.0", c.assignments().getFirst().packVersion());
        assertNotEquals(a.desiredChecksum(), c.desiredChecksum());

        // D: DELETE assignment increments
        var d = assignments.unassign(tenantA.tenantId(), "sensor-a", PACK, 3L);
        assertEquals(4L, d.desiredGeneration());
        assertTrue(d.assignments().isEmpty());
        assertNull(d.desiredChecksum());

        // E: DELETE absent idempotent no increment
        var e = assignments.unassign(tenantA.tenantId(), "sensor-a", PACK, 4L);
        assertEquals(4L, e.desiredGeneration());

        // F: expectedGeneration mismatch → 409
        DomainException conflict = assertThrows(DomainException.class,
                () -> assignments.assign(tenantA.tenantId(), "sensor-a", PACK, "2.0.0", 99L));
        assertEquals(SensorCapabilityPackAssignmentService.ERR_GENERATION_CONFLICT, conflict.getCode());

        // G: omitted expectedGeneration → VALIDATION_FAILED
        DomainException missing = assertThrows(DomainException.class,
                () -> assignments.assign(tenantA.tenantId(), "sensor-a", PACK, "2.0.0", null));
        assertEquals("VALIDATION_FAILED", missing.getCode());

        // H: non-ACTIVE sensor
        sensors.enrollSensor("sensor-disabled", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        sensors.disableSensor("sensor-disabled");
        DomainException inactive = assertThrows(DomainException.class,
                () -> assignments.assign(tenantA.tenantId(), "sensor-disabled", PACK, "2.0.0", 0L));
        assertEquals(SensorCapabilityPackAssignmentService.ERR_SENSOR_NOT_ACTIVE, inactive.getCode());

        // I: non-ACTIVE pack version
        catalog.register(tenantA.tenantId(), PACK, "3.0.0",
                CapabilityPackTestSupport.minimalManifestBytes(PACK, "3.0.0", "AssignV3"));
        DomainException notActive = assertThrows(DomainException.class,
                () -> assignments.assign(tenantA.tenantId(), "sensor-a", PACK, "3.0.0", 4L));
        assertEquals(SensorCapabilityPackAssignmentService.ERR_VERSION_NOT_ACTIVE, notActive.getCode());

        // J: cross-tenant impossible (FK + service)
        sensors.enrollSensor("sensor-b", tenantB.tenantId(), null, SensorRole.NETWORK, true);
        DomainException cross = assertThrows(DomainException.class,
                () -> assignments.assign(tenantB.tenantId(), "sensor-a", PACK, "2.0.0", 0L));
        assertEquals(SensorCapabilityPackAssignmentService.ERR_SENSOR_NOT_FOUND, cross.getCode());
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_capability_pack_assignment
                  (tenant_id, sensor_id, pack_id, pack_version, assigned_by, assigned_at)
                VALUES (?, 'sensor-b', ?, '2.0.0', 'evil', NOW())
                """, tenantB.tenantId(), PACK));
    }

    @Test
    void activationFenceBlocksReplaceAndDeactivateButAllowsIdempotentSameVersion() {
        sensors.enrollSensor("sensor-fence", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        catalog.register(tenantA.tenantId(), PACK, "1.0.0",
                CapabilityPackTestSupport.minimalManifestBytes(PACK, "1.0.0", "Fence"));
        catalog.register(tenantA.tenantId(), PACK, "2.0.0",
                CapabilityPackTestSupport.minimalManifestBytes(PACK, "2.0.0", "Fence2"));
        CapabilityPackActivation active = catalog.activate(tenantA.tenantId(), PACK, "1.0.0", 0L);
        assignments.assign(tenantA.tenantId(), "sensor-fence", PACK, "1.0.0", 0L);

        CapabilityPackActivation same = catalog.activate(tenantA.tenantId(), PACK, "1.0.0", active.generation());
        assertEquals(active.generation(), same.generation());

        DomainException replace = assertThrows(DomainException.class,
                () -> catalog.activate(tenantA.tenantId(), PACK, "2.0.0", active.generation()));
        assertEquals(CapabilityPackCatalogService.ERR_SENSOR_ASSIGNMENTS_EXIST, replace.getCode());

        DomainException deactivate = assertThrows(DomainException.class,
                () -> catalog.deactivate(tenantA.tenantId(), PACK, active.generation()));
        assertEquals(CapabilityPackCatalogService.ERR_SENSOR_ASSIGNMENTS_EXIST, deactivate.getCode());
    }

    @Test
    void machineDesiredArtifactAndAckExactBytes() {
        sensors.enrollSensor("sensor-pull", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        byte[] bytes = CapabilityPackTestSupport.minimalManifestBytes(PACK, "1.0.0", "Pull");
        catalog.register(tenantA.tenantId(), PACK, "1.0.0", bytes);
        catalog.activate(tenantA.tenantId(), PACK, "1.0.0", 0L);
        var assigned = assignments.assign(tenantA.tenantId(), "sensor-pull", PACK, "1.0.0", 0L);

        SensorPackDesiredState desired = assignments.getDesiredState(tenantA.tenantId(), "sensor-pull");
        assertEquals(1L, desired.generation());
        assertEquals(assigned.desiredChecksum(), desired.checksum());
        assertEquals(0L, desired.appliedGeneration());
        assertNull(desired.appliedChecksum());
        assertEquals(1, desired.packs().size());

        StoredCapabilityPack artifact = assignments.requireAssignedArtifact(
                tenantA.tenantId(), "sensor-pull", PACK, "1.0.0");
        assertArrayEquals(bytes, artifact.manifestBytes());

        assertThrows(DomainException.class, () ->
                assignments.requireAssignedArtifact(tenantA.tenantId(), "sensor-pull", PACK, "9.9.9"));

        assignments.ackAppliedState(tenantA.tenantId(), "sensor-pull",
                desired.generation(), desired.checksum());
        assignments.ackAppliedState(tenantA.tenantId(), "sensor-pull",
                desired.generation(), desired.checksum()); // idempotent

        SensorPackDesiredState afterAck = assignments.getDesiredState(tenantA.tenantId(), "sensor-pull");
        assertEquals(desired.generation(), afterAck.appliedGeneration());
        assertEquals(desired.checksum(), afterAck.appliedChecksum());

        DomainException stale = assertThrows(DomainException.class, () ->
                assignments.ackAppliedState(tenantA.tenantId(), "sensor-pull", 0L, desired.checksum()));
        assertEquals(SensorCapabilityPackAssignmentService.ERR_PACK_STATE_STALE, stale.getCode());

        DomainException mismatch = assertThrows(DomainException.class, () ->
                assignments.ackAppliedState(tenantA.tenantId(), "sensor-pull",
                        desired.generation(), "0".repeat(64)));
        assertEquals(SensorCapabilityPackAssignmentService.ERR_PACK_CHECKSUM_MISMATCH, mismatch.getCode());
    }

    @Test
    void boundedE2EAssignStageAckReassign() {
        sensors.enrollSensor("sensor-e2e", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        byte[] v1 = CapabilityPackTestSupport.minimalManifestBytes(PACK, "1.0.0", "E2EV1");
        byte[] v2 = CapabilityPackTestSupport.minimalManifestBytes(PACK, "2.0.0", "E2EV2");
        catalog.register(tenantA.tenantId(), PACK, "1.0.0", v1);
        catalog.register(tenantA.tenantId(), PACK, "2.0.0", v2);
        catalog.activate(tenantA.tenantId(), PACK, "1.0.0", 0L);

        var g1 = assignments.assign(tenantA.tenantId(), "sensor-e2e", PACK, "1.0.0", 0L);
        SensorPackDesiredState d1 = assignments.getDesiredState(tenantA.tenantId(), "sensor-e2e");
        assertEquals(g1.desiredGeneration(), d1.generation());
        assertArrayEquals(v1, assignments.requireAssignedArtifact(
                tenantA.tenantId(), "sensor-e2e", PACK, "1.0.0").manifestBytes());
        assignments.ackAppliedState(tenantA.tenantId(), "sensor-e2e", d1.generation(), d1.checksum());
        SensorPackDesiredState d1Applied = assignments.getDesiredState(tenantA.tenantId(), "sensor-e2e");
        assertEquals(d1.generation(), d1Applied.appliedGeneration());
        assertEquals(d1.checksum(), d1Applied.appliedChecksum());

        assignments.unassign(tenantA.tenantId(), "sensor-e2e", PACK, g1.desiredGeneration());
        catalog.activate(tenantA.tenantId(), PACK, "2.0.0", 1L);
        var g2 = assignments.assign(tenantA.tenantId(), "sensor-e2e", PACK, "2.0.0",
                g1.desiredGeneration() + 1);
        SensorPackDesiredState d2 = assignments.getDesiredState(tenantA.tenantId(), "sensor-e2e");
        assertEquals(g2.desiredGeneration(), d2.generation());
        assertArrayEquals(v2, assignments.requireAssignedArtifact(
                tenantA.tenantId(), "sensor-e2e", PACK, "2.0.0").manifestBytes());
        assignments.ackAppliedState(tenantA.tenantId(), "sensor-e2e", d2.generation(), d2.checksum());
        SensorPackDesiredState d2Applied = assignments.getDesiredState(tenantA.tenantId(), "sensor-e2e");
        assertEquals(d2.generation(), d2Applied.appliedGeneration());
        assertEquals(d2.checksum(), d2Applied.appliedChecksum());

        var view = assignments.getAssignments(tenantA.tenantId(), "sensor-e2e");
        assertEquals(d2.generation(), view.appliedGeneration());
        assertEquals(d2.checksum(), view.appliedChecksum());
    }

    @Test
    void migrationFksRejectCrossTenantAssignmentAndState() {
        sensors.enrollSensor("sensor-a", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        sensors.enrollSensor("sensor-b", tenantB.tenantId(), null, SensorRole.NETWORK, true);
        catalog.register(tenantA.tenantId(), PACK, "1.0.0",
                CapabilityPackTestSupport.minimalManifestBytes(PACK, "1.0.0", "Fk"));
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_capability_pack_assignment
                  (tenant_id, sensor_id, pack_id, pack_version, assigned_by, assigned_at)
                VALUES (?, 'sensor-b', ?, '1.0.0', 'x', NOW())
                """, tenantA.tenantId(), PACK));
        assertThrows(Exception.class, () -> jdbc.update("""
                INSERT INTO platform.sensor_capability_pack_state
                  (tenant_id, sensor_id, desired_generation, desired_checksum,
                   applied_generation, applied_checksum, applied_at, updated_by, updated_at)
                VALUES (?, 'sensor-b', 1, NULL, 0, NULL, NULL, 'x', NOW())
                """, tenantA.tenantId()));
    }

    @Test
    void controlGenerationBoundary_desiredExhaustionPreservesAssignmentSet() {
        String packB = "fixture.fictional.assign.b";
        sensors.enrollSensor("sensor-gen", tenantA.tenantId(), null, SensorRole.NETWORK, true);
        catalog.register(tenantA.tenantId(), PACK, "1.0.0",
                CapabilityPackTestSupport.minimalManifestBytes(PACK, "1.0.0", "GenA"));
        catalog.register(tenantA.tenantId(), packB, "1.0.0",
                CapabilityPackTestSupport.minimalManifestBytes(packB, "1.0.0", "GenB"));
        catalog.activate(tenantA.tenantId(), PACK, "1.0.0", 0L);
        catalog.activate(tenantA.tenantId(), packB, "1.0.0", 0L);

        var first = assignments.assign(tenantA.tenantId(), "sensor-gen", PACK, "1.0.0", 0L);
        assertEquals(1L, first.desiredGeneration());

        long maxSafe = ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION;
        jdbc.update("""
                UPDATE platform.sensor_capability_pack_state
                   SET desired_generation = ?
                 WHERE tenant_id = ? AND sensor_id = 'sensor-gen'
                """, maxSafe - 1L, tenantA.tenantId());

        var atMax = assignments.assign(tenantA.tenantId(), "sensor-gen", packB, "1.0.0", maxSafe - 1L);
        assertEquals(maxSafe, atMax.desiredGeneration());
        assertEquals(2, atMax.assignments().size());

        // Idempotent same assignment at MAX_SAFE: no increment required.
        var idempotent = assignments.assign(tenantA.tenantId(), "sensor-gen", PACK, "1.0.0", maxSafe);
        assertEquals(maxSafe, idempotent.desiredGeneration());
        assertEquals(2, idempotent.assignments().size());

        // Unassign of unknown pack is no-op even at MAX_SAFE.
        var noopUnassign = assignments.unassign(tenantA.tenantId(), "sensor-gen", "missing.pack", maxSafe);
        assertEquals(maxSafe, noopUnassign.desiredGeneration());
        assertEquals(2, noopUnassign.assignments().size());

        DomainException exhausted = assertThrows(DomainException.class,
                () -> assignments.unassign(tenantA.tenantId(), "sensor-gen", PACK, maxSafe));
        assertEquals(ControlGenerationLimits.ERR_CONTROL_GENERATION_EXHAUSTED, exhausted.getCode());

        var preserved = assignments.getAssignments(tenantA.tenantId(), "sensor-gen");
        assertEquals(maxSafe, preserved.desiredGeneration());
        assertEquals(2, preserved.assignments().size());

        DomainException overMax = assertThrows(DomainException.class,
                () -> assignments.assign(tenantA.tenantId(), "sensor-gen", PACK, "1.0.0", maxSafe + 1L));
        assertEquals("VALIDATION_FAILED", overMax.getCode());

        DomainException ackOverMax = assertThrows(DomainException.class,
                () -> assignments.ackAppliedState(
                        tenantA.tenantId(), "sensor-gen", maxSafe + 1L, preserved.desiredChecksum()));
        assertEquals("VALIDATION_FAILED", ackOverMax.getCode());

        // MAX_SAFE itself remains a valid applied generation input (may still fail stale/checksum).
        DomainException ackStale = assertThrows(DomainException.class,
                () -> assignments.ackAppliedState(
                        tenantA.tenantId(), "sensor-gen", maxSafe, "0".repeat(64)));
        assertEquals(SensorCapabilityPackAssignmentService.ERR_PACK_CHECKSUM_MISMATCH, ackStale.getCode());
    }

    private void authenticateAs(String subjectId) {
        Instant now = Instant.parse("2026-06-01T00:00:00Z");
        PrincipalContext principal = new PrincipalContext(
                subjectId,
                AuthenticationState.AUTHENTICATED,
                Set.of(),
                Set.of()
        );
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
