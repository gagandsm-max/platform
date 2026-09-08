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
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void schemaMigration_V002AppliedSuccessfully() {
        Integer tableCount = pg.jdbc().queryForObject(
                "SELECT count(*) FROM information_schema.tables " +
                "WHERE table_schema = 'platform' AND table_name IN (" +
                "'discovery_request', 'discovery_target', 'discovery_evidence', 'discovery_candidate')",
                Integer.class
        );
        assertEquals(4, tableCount, "All 4 discovery tables must exist in platform schema");

        Integer migrationCount = pg.jdbc().queryForObject(
                "SELECT count(*) FROM platform.schema_history WHERE version = 'V002'",
                Integer.class
        );
        assertEquals(1, migrationCount, "Migration V002 must be recorded in platform.schema_history");
    }

    @Test
    void strictTenantIsolation_MatrixEnforced() {
        Tenant tenantA = tenantService.create("Tenant Alpha", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant tenantB = tenantService.create("Tenant Beta", TenantMode.MSP_CHILD, TenantStatus.ACTIVE, "America/New_York");
        assertNotEquals(tenantA.tenantId(), tenantB.tenantId());

        // Create Discovery in Tenant A
        DiscoveryRequest reqA = discoveryService.createDiscovery(
                tenantA.tenantId(),
                "Alpha Discovery",
                List.of("192.168.10.1", "192.168.10.2"),
                List.of(DiscoveryMethod.ICMP),
                null, null, null, 161, 1500, 2, "idem-a", "corr-a", "user-a"
        );

        // Create Discovery in Tenant B
        DiscoveryRequest reqB = discoveryService.createDiscovery(
                tenantB.tenantId(),
                "Beta Discovery",
                List.of("10.10.0.1"),
                List.of(DiscoveryMethod.ICMP, DiscoveryMethod.SNMP),
                null, null, null, 161, 2000, 1, "idem-b", "corr-b", "user-b"
        );

        // Verify Tenant A can only see reqA
        PageResult<DiscoveryRequest> pageA = discoveryService.listRequests(tenantA.tenantId(), 1, 20);
        assertEquals(1, pageA.total());
        assertEquals(reqA.requestId(), pageA.items().get(0).requestId());

        // Verify Tenant B can only see reqB
        PageResult<DiscoveryRequest> pageB = discoveryService.listRequests(tenantB.tenantId(), 1, 20);
        assertEquals(1, pageB.total());
        assertEquals(reqB.requestId(), pageB.items().get(0).requestId());

        // Cross-tenant access verification: Tenant B attempting to access reqA must fail
        DomainException exCrossGet = assertThrows(DomainException.class, () ->
                discoveryService.getRequest(tenantB.tenantId(), reqA.requestId())
        );
        assertEquals("RESOURCE_NOT_FOUND", exCrossGet.getCode());

        // Ingest evidence into Request A
        DiscoveryService.SensorEvidenceItem evItem = new DiscoveryService.SensorEvidenceItem(
                "192.168.10.1", "ICMP", "ECHO", "SUCCESS", "OBSERVED",
                "ICMP_ECHO_REPLY", Map.of("rtt_micros", 1000), Map.of(), 5L
        );
        discoveryService.ingestResultsFromSensor("sensor-net-01", tenantA.tenantId(), reqA.requestId(), List.of(evItem), "COMPLETED", null);

        // Tenant B attempting to list candidates or evidence for reqA must be blocked
        DomainException exCrossCand = assertThrows(DomainException.class, () ->
                discoveryService.listCandidates(tenantB.tenantId(), reqA.requestId(), 1, 20)
        );
        assertEquals("RESOURCE_NOT_FOUND", exCrossCand.getCode());

        DomainException exCrossEv = assertThrows(DomainException.class, () ->
                discoveryService.listEvidence(tenantB.tenantId(), reqA.requestId(), 1, 20)
        );
        assertEquals("RESOURCE_NOT_FOUND", exCrossEv.getCode());
    }

    @Test
    void fullRoundtrip_ExecutionAndCandidateAggregation_ZeroAutoProvisioning() {
        Tenant tenant = tenantService.create("Discovery Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");

        // 1. Create request
        DiscoveryRequest created = discoveryService.createDiscovery(
                tenant.tenantId(),
                "Branch Subnet Scan",
                List.of("172.16.5.10", "172.16.5.20"),
                List.of(DiscoveryMethod.ICMP, DiscoveryMethod.SNMP),
                "prof-cisco", null, null, 161, 2000, 2, "scan-branch-1", "corr-full-1", "operator"
        );
        assertEquals(DiscoveryStatus.ASSIGNED, created.status());

        // 2. Sensor polls for task
        Optional<DiscoveryService.DiscoveryTaskPayload> assignedTask = discoveryService.fetchTaskForSensor("sensor-net-01");
        assertTrue(assignedTask.isPresent());
        assertEquals(created.requestId().toString(), assignedTask.get().requestId());

        // 3. Sensor executes and submits results
        DiscoveryService.SensorEvidenceItem icmpEv = new DiscoveryService.SensorEvidenceItem(
                "172.16.5.10", "ICMP", "ECHO", "SUCCESS", "OBSERVED", "ICMP_ECHO_REPLY",
                Map.of("rtt_micros", 900), Map.of(), 3L
        );
        DiscoveryService.SensorEvidenceItem snmpEv = new DiscoveryService.SensorEvidenceItem(
                "172.16.5.10", "SNMP", "GET_IDENTITY", "SUCCESS", "OBSERVED", "SNMP_V2C_SUCCESS",
                Map.of(
                        "sys_name", "branch-gw-01",
                        "sys_descr", "Cisco ISR 4331",
                        "sys_object_id", "1.3.6.1.4.1.9.1.2067"
                ), Map.of(), 50L
        );

        discoveryService.ingestResultsFromSensor(
                "sensor-net-01", tenant.tenantId(), created.requestId(),
                List.of(icmpEv, snmpEv), "COMPLETED", null
        );

        // 4. Verify request status is COMPLETED
        DiscoveryRequest updated = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.COMPLETED, updated.status());

        // 5. Verify candidates in review queue
        PageResult<DiscoveryCandidate> candidatePage = discoveryService.listCandidates(tenant.tenantId(), created.requestId(), 1, 50);
        assertEquals(1, candidatePage.total());

        DiscoveryCandidate cand = candidatePage.items().get(0);
        assertEquals("172.16.5.10", cand.targetAddress());
        assertEquals("branch-gw-01", cand.discoveredName());
        assertEquals(DiscoveryConfidence.HIGH, cand.confidence());

        // MANDATORY ARCHITECTURAL REQUIREMENT:
        // Candidate review status MUST be PENDING_REVIEW. Zero auto-provisioning!
        assertEquals(CandidateReviewStatus.PENDING_REVIEW, cand.reviewStatus());

        // 6. Verify evidence persistence
        PageResult<DiscoveryEvidence> evidencePage = discoveryService.listEvidence(tenant.tenantId(), created.requestId(), 1, 50);
        assertEquals(2, evidencePage.total());
    }

    @Test
    void cancellationFlow() {
        Tenant tenant = tenantService.create("Cancel Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");

        DiscoveryRequest created = discoveryService.createDiscovery(
                tenant.tenantId(),
                "Scan to Cancel",
                List.of("192.168.1.1"),
                List.of(DiscoveryMethod.ICMP),
                null, null, null, 161, 2000, 2, "cancel-key-1", "corr-cancel-1", "operator"
        );

        discoveryService.cancelRequest(tenant.tenantId(), created.requestId());

        DiscoveryRequest updated = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.CANCELLED, updated.status());
    }
}
