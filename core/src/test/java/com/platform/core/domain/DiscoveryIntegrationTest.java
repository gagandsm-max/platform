package com.platform.core.domain;

import com.platform.core.api.PageResult;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.discovery.CandidateReviewStatus;
import com.platform.core.domain.discovery.DiscoveryCandidate;
import com.platform.core.domain.discovery.DiscoveryConfidence;
import com.platform.core.domain.discovery.DiscoveryEvidence;
import com.platform.core.domain.discovery.DiscoveryMethod;
import com.platform.core.domain.discovery.DiscoveryRequest;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.discovery.DiscoveryStatus;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.DiscoveryRepository;
import com.platform.core.persistence.TenantRepository;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

import java.util.List;
import java.util.Map;
import java.util.Optional;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

@Tag("postgres")
class DiscoveryIntegrationTest {

    private PostgresTestSupport pg;
    private TenantService tenantService;
    private DiscoveryRepository discoveryRepo;
    private DiscoveryService discoveryService;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();

        NamedParameterJdbcTemplate named = pg.namedJdbc();
        TenantRepository tenantRepo = new TenantRepository(named);
        tenantService = new TenantService(tenantRepo);
        discoveryRepo = new DiscoveryRepository(named);
        discoveryService = new DiscoveryService(discoveryRepo, tenantService);
    }

    @AfterEach
    void tearDown() {
        if (pg != null) pg.stop();
    }

    @Test
    void schemaMigration_V003AppliedSuccessfullyWithFencingAndTenantScopedIdempotency() {
        Integer tableCount = pg.jdbc().queryForObject(
                "SELECT count(*) FROM information_schema.tables " +
                        "WHERE table_schema = 'platform' AND table_name IN (" +
                        "'discovery_request', 'discovery_target', 'discovery_evidence', 'discovery_candidate')",
                Integer.class
        );
        assertEquals(4, tableCount, "All 4 discovery tables must exist in platform schema");

        Integer telemetryMigrationCount = pg.jdbc().queryForObject(
                "SELECT count(*) FROM platform.schema_history WHERE version = 'V002'", Integer.class);
        assertEquals(1, telemetryMigrationCount, "Migration V002 (telemetry watermark) must precede discovery");

        Integer migrationCount = pg.jdbc().queryForObject(
                "SELECT count(*) FROM platform.schema_history WHERE version = 'V003'", Integer.class);
        assertEquals(1, migrationCount, "Migration V003 must be recorded in platform.schema_history");

        Integer assignmentVersionColumns = pg.jdbc().queryForObject(
                "SELECT count(*) FROM information_schema.columns WHERE table_schema='platform' " +
                        "AND table_name='discovery_request' AND column_name='assignment_version'",
                Integer.class);
        assertEquals(1, assignmentVersionColumns);

        Integer idempotencyIndexes = pg.jdbc().queryForObject(
                "SELECT count(*) FROM pg_indexes WHERE schemaname='platform' " +
                        "AND tablename='discovery_request' AND indexname='uq_discovery_tenant_idempotency'",
                Integer.class);
        assertEquals(1, idempotencyIndexes);
    }

    @Test
    void strictTenantIsolation_CustomerReadsAndIdempotencyRemainTenantScoped() {
        Tenant tenantA = tenantService.create("Tenant Alpha", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant tenantB = tenantService.create("Tenant Beta", TenantMode.MSP_CHILD, TenantStatus.ACTIVE, "America/New_York");
        assertNotEquals(tenantA.tenantId(), tenantB.tenantId());

        DiscoveryRequest reqA = discoveryService.createDiscovery(
                tenantA.tenantId(), "Alpha Discovery", List.of("192.168.10.1"), List.of(DiscoveryMethod.ICMP),
                null, null, null, 161, 1500, 2, "same-key", "corr-a", "user-a");
        DiscoveryRequest reqB = discoveryService.createDiscovery(
                tenantB.tenantId(), "Beta Discovery", List.of("10.10.0.1"), List.of(DiscoveryMethod.ICMP),
                null, null, null, 161, 1500, 2, "same-key", "corr-b", "user-b");
        assertNotEquals(reqA.requestId(), reqB.requestId(), "same idempotency key in another tenant must be independent");

        PageResult<DiscoveryRequest> pageA = discoveryService.listRequests(tenantA.tenantId(), 1, 20);
        PageResult<DiscoveryRequest> pageB = discoveryService.listRequests(tenantB.tenantId(), 1, 20);
        assertEquals(1, pageA.total());
        assertEquals(reqA.requestId(), pageA.items().get(0).requestId());
        assertEquals(1, pageB.total());
        assertEquals(reqB.requestId(), pageB.items().get(0).requestId());

        DomainException crossGet = assertThrows(DomainException.class,
                () -> discoveryService.getRequest(tenantB.tenantId(), reqA.requestId()));
        assertEquals("RESOURCE_NOT_FOUND", crossGet.getCode());
        DomainException crossCandidates = assertThrows(DomainException.class,
                () -> discoveryService.listCandidates(tenantB.tenantId(), reqA.requestId(), 1, 20));
        assertEquals("RESOURCE_NOT_FOUND", crossCandidates.getCode());
        DomainException crossEvidence = assertThrows(DomainException.class,
                () -> discoveryService.listEvidence(tenantB.tenantId(), reqA.requestId(), 1, 20));
        assertEquals("RESOURCE_NOT_FOUND", crossEvidence.getCode());
    }

    @Test
    void lifecycle_PendingAssignedRunningCompleted_WithAtomicClaimAndSafeReplay() {
        Tenant tenant = tenantService.create("Lifecycle Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        DiscoveryRequest created = discoveryService.createDiscovery(
                tenant.tenantId(), "Lifecycle Scan", List.of("172.16.5.10"), List.of(DiscoveryMethod.ICMP),
                null, null, null, 161, 2000, 2, "life-1", "corr-life-1", "operator");
        assertEquals(DiscoveryStatus.PENDING, created.status());
        assertEquals(null, created.assignedSensorId());
        assertFalse(discoveryService.fetchTaskForSensor("sensor-a").isPresent(), "unassigned work must not self-assign");

        DiscoveryRequest assigned = discoveryService.assignSensorFromControlPlane(tenant.tenantId(), created.requestId(), "sensor-a");
        assertEquals(DiscoveryStatus.ASSIGNED, assigned.status());
        assertEquals("sensor-a", assigned.assignedSensorId());
        assertFalse(discoveryService.fetchTaskForSensor("sensor-b").isPresent(), "Sensor B cannot claim Sensor A work");

        Optional<DiscoveryService.DiscoveryTaskPayload> task = discoveryService.fetchTaskForSensor("sensor-a");
        assertTrue(task.isPresent());
        assertTrue(task.get().assignmentVersion() > 0);
        assertFalse(discoveryService.fetchTaskForSensor("sensor-a").isPresent(), "atomic claim prevents duplicate RUNNING claim");
        assertEquals(DiscoveryStatus.RUNNING,
                discoveryService.getRequest(tenant.tenantId(), created.requestId()).status());

        DiscoveryService.SensorEvidenceItem evidence = new DiscoveryService.SensorEvidenceItem(
                "172.16.5.10", "ICMP", "ECHO", "SUCCESS", "OBSERVED", "ICMP_ECHO_REPLY",
                Map.of("rtt_micros", 900), Map.of(), 3L);
        DiscoveryService.ResultIngestOutcome first = discoveryService.ingestResultsFromSensor(
                "sensor-a", created.requestId(), task.get().assignmentVersion(), List.of(evidence), "COMPLETED", null);
        assertFalse(first.replay());
        assertEquals(DiscoveryStatus.COMPLETED, first.status());

        DiscoveryService.ResultIngestOutcome replay = discoveryService.ingestResultsFromSensor(
                "sensor-a", created.requestId(), task.get().assignmentVersion(), List.of(evidence), "COMPLETED", null);
        assertTrue(replay.replay());
        assertEquals(1, discoveryService.listEvidence(tenant.tenantId(), created.requestId(), 1, 20).total());

        DomainException cancelTerminal = assertThrows(DomainException.class,
                () -> discoveryService.cancelRequest(tenant.tenantId(), created.requestId()));
        assertEquals("INVALID_STATE_TRANSITION", cancelTerminal.getCode());
    }

    @Test
    void reassignment_IncrementsFenceAndPreviousSensorCannotClaimOrSubmit() {
        Tenant tenant = tenantService.create("Reassign Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        DiscoveryRequest created = discoveryService.createDiscovery(
                tenant.tenantId(), "Reassign Scan", List.of("192.168.50.10"), List.of(DiscoveryMethod.ICMP),
                null, null, null, 161, 2000, 2, "reassign-1", "corr-r", "operator");

        discoveryService.assignSensorFromControlPlane(tenant.tenantId(), created.requestId(), "sensor-a");
        discoveryService.assignSensorFromControlPlane(tenant.tenantId(), created.requestId(), "sensor-b");
        assertFalse(discoveryService.fetchTaskForSensor("sensor-a").isPresent());
        DiscoveryService.DiscoveryTaskPayload task = discoveryService.fetchTaskForSensor("sensor-b").orElseThrow();
        assertEquals(2L, task.assignmentVersion());

        DomainException previousSensor = assertThrows(DomainException.class, () -> discoveryService.ingestResultsFromSensor(
                "sensor-a", created.requestId(), 1L, List.of(), "COMPLETED", null));
        assertEquals("RESOURCE_NOT_FOUND", previousSensor.getCode());

        DomainException staleFence = assertThrows(DomainException.class, () -> discoveryService.ingestResultsFromSensor(
                "sensor-b", created.requestId(), 1L, List.of(), "COMPLETED", null));
        assertEquals("STALE_ASSIGNMENT", staleFence.getCode());
    }

    @Test
    void cancellationIsTerminalAndRejectsLateSensorResult() {
        Tenant tenant = tenantService.create("Cancel Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        DiscoveryRequest created = discoveryService.createDiscovery(
                tenant.tenantId(), "Scan to Cancel", List.of("192.168.1.1"), List.of(DiscoveryMethod.ICMP),
                null, null, null, 161, 2000, 2, "cancel-key-1", "corr-cancel-1", "operator");
        discoveryService.assignSensorFromControlPlane(tenant.tenantId(), created.requestId(), "sensor-a");
        DiscoveryService.DiscoveryTaskPayload task = discoveryService.fetchTaskForSensor("sensor-a").orElseThrow();

        discoveryService.cancelRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.CANCELLED,
                discoveryService.getRequest(tenant.tenantId(), created.requestId()).status());

        DomainException late = assertThrows(DomainException.class, () -> discoveryService.ingestResultsFromSensor(
                "sensor-a", created.requestId(), task.assignmentVersion(), List.of(), "COMPLETED", null));
        assertEquals("INVALID_STATE_TRANSITION", late.getCode());
        assertEquals(DiscoveryStatus.CANCELLED,
                discoveryService.getRequest(tenant.tenantId(), created.requestId()).status());
    }

    @Test
    void idempotency_EquivalentReplayReturnsSameRequestButChangedPayloadConflicts() {
        Tenant tenant = tenantService.create("Idem Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        DiscoveryRequest first = discoveryService.createDiscovery(
                tenant.tenantId(), "Idem Scan", List.of("10.20.30.40"), List.of(DiscoveryMethod.ICMP),
                null, null, null, 161, 2000, 2, "idem-1", "corr-1", "operator");
        DiscoveryRequest replay = discoveryService.createDiscovery(
                tenant.tenantId(), "Idem Scan", List.of("10.20.30.40"), List.of(DiscoveryMethod.ICMP),
                null, null, null, 161, 2000, 2, "idem-1", "corr-2", "operator");
        assertEquals(first.requestId(), replay.requestId());

        DomainException conflict = assertThrows(DomainException.class, () -> discoveryService.createDiscovery(
                tenant.tenantId(), "Idem Scan", List.of("10.20.30.41"), List.of(DiscoveryMethod.ICMP),
                null, null, null, 161, 2000, 2, "idem-1", "corr-3", "operator"));
        assertEquals("IDEMPOTENCY_CONFLICT", conflict.getCode());
    }

    @Test
    void fullRoundtrip_CandidateStaysReviewOnlyAndDoesNotCreateEntity() {
        Tenant tenant = tenantService.create("Discovery Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Integer entityCountBefore = pg.jdbc().queryForObject(
                "SELECT count(*) FROM platform.entity WHERE tenant_id = ?", Integer.class, tenant.tenantId());

        DiscoveryRequest created = discoveryService.createDiscovery(
                tenant.tenantId(), "Branch Subnet Scan", List.of("172.16.5.10", "172.16.5.20"),
                List.of(DiscoveryMethod.ICMP, DiscoveryMethod.SNMP), "prof-cisco", null, null,
                161, 2000, 2, "scan-branch-1", "corr-full-1", "operator");
        assertEquals(DiscoveryStatus.PENDING, created.status());
        discoveryService.assignSensorFromControlPlane(tenant.tenantId(), created.requestId(), "sensor-net-01");
        DiscoveryService.DiscoveryTaskPayload task = discoveryService.fetchTaskForSensor("sensor-net-01").orElseThrow();

        DiscoveryService.SensorEvidenceItem icmp = new DiscoveryService.SensorEvidenceItem(
                "172.16.5.10", "ICMP", "ECHO", "SUCCESS", "OBSERVED", "ICMP_ECHO_REPLY",
                Map.of("rtt_micros", 900), Map.of(), 3L);
        DiscoveryService.SensorEvidenceItem snmp = new DiscoveryService.SensorEvidenceItem(
                "172.16.5.10", "SNMP", "GET_IDENTITY", "SUCCESS", "OBSERVED", "SNMP_GET_RESPONSE",
                Map.of("sys_name", "branch-gw-01", "sys_descr", "Cisco ISR 4331",
                        "sys_object_id", "1.3.6.1.4.1.9.1.2067"), Map.of(), 50L);

        discoveryService.ingestResultsFromSensor(
                "sensor-net-01", created.requestId(), task.assignmentVersion(),
                List.of(icmp, snmp), "COMPLETED", null);

        DiscoveryRequest updated = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.COMPLETED, updated.status());
        PageResult<DiscoveryCandidate> candidates = discoveryService.listCandidates(tenant.tenantId(), created.requestId(), 1, 50);
        assertEquals(1, candidates.total());
        DiscoveryCandidate candidate = candidates.items().get(0);
        assertEquals("172.16.5.10", candidate.targetAddress());
        assertEquals("branch-gw-01", candidate.discoveredName());
        assertEquals(DiscoveryConfidence.HIGH, candidate.confidence());
        assertEquals(CandidateReviewStatus.PENDING_REVIEW, candidate.reviewStatus());

        PageResult<DiscoveryEvidence> evidence = discoveryService.listEvidence(tenant.tenantId(), created.requestId(), 1, 50);
        assertEquals(2, evidence.total());
        Integer entityCountAfter = pg.jdbc().queryForObject(
                "SELECT count(*) FROM platform.entity WHERE tenant_id = ?", Integer.class, tenant.tenantId());
        assertEquals(entityCountBefore, entityCountAfter, "discovery completion must not auto-provision platform.entity");
    }
}
