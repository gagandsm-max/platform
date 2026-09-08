package com.platform.core.domain.discovery;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.DiscoveryRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class DiscoveryServiceTest {

    private DiscoveryRepository repository;
    private TenantService tenantService;
    private DiscoveryService service;

    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private final Tenant activeTenant = new Tenant(
            tenantId,
            "Acme Corp",
            TenantMode.SINGLE,
            TenantStatus.ACTIVE,
            "UTC",
            Instant.now(),
            Instant.now()
    );

    @BeforeEach
    void setUp() {
        repository = mock(DiscoveryRepository.class);
        tenantService = mock(TenantService.class);
        when(tenantService.get(tenantId)).thenReturn(activeTenant);
        service = new DiscoveryService(repository, tenantService);
    }

    @Test
    void validateTargetLimits_Max256Enforced() {
        List<String> tooMany = new ArrayList<>();
        for (int i = 0; i < 257; i++) {
            tooMany.add("192.168.1." + (i % 250 + 1));
        }

        DomainException ex = assertThrows(DomainException.class, () ->
                service.createDiscovery(
                        tenantId,
                        "Too Many Targets",
                        tooMany,
                        List.of(DiscoveryMethod.ICMP),
                        null, null, null, null, null, null, null, null, "admin"
                )
        );
        assertEquals("QUERY_LIMIT_EXCEEDED", ex.getCode());
        assertTrue(ex.getMessage().contains("exceeds maximum bounded limit"));
    }

    @Test
    void validateTargetLimits_BroadCidrRejected() {
        // /16 exceeds /24 limit
        DomainException ex1 = assertThrows(DomainException.class, () ->
                service.createDiscovery(
                        tenantId,
                        "Broad CIDR",
                        List.of("10.0.0.0/16"),
                        List.of(DiscoveryMethod.ICMP),
                        null, null, null, null, null, null, null, null, "admin"
                )
        );
        assertEquals("QUERY_LIMIT_EXCEEDED", ex1.getCode());
        assertTrue(ex1.getMessage().contains("prefix must be between /24 and /32"));

        // /23 also exceeds /24 limit
        DomainException ex2 = assertThrows(DomainException.class, () ->
                service.createDiscovery(
                        tenantId,
                        "Broad CIDR 23",
                        List.of("192.168.0.0/23"),
                        List.of(DiscoveryMethod.ICMP),
                        null, null, null, null, null, null, null, null, "admin"
                )
        );
        assertEquals("QUERY_LIMIT_EXCEEDED", ex2.getCode());
    }

    @Test
    void validateTargetLimits_ValidCidr24Allowed() {
        // /24 expands to 254 host IPs and fits within the 256 bound
        DiscoveryRequest created = service.createDiscovery(
                tenantId,
                "Valid Subnet",
                List.of("192.168.10.0/24"),
                List.of(DiscoveryMethod.ICMP),
                null, null, null, null, null, null, null, null, "admin"
        );

        assertNotNull(created);
        assertEquals("Valid Subnet", created.name());
        verify(repository).createRequest(any(DiscoveryRequest.class), any());
    }

    @Test
    void validateSSRF_LoopbackRejected() {
        DomainException ex = assertThrows(DomainException.class, () ->
                service.createDiscovery(
                        tenantId,
                        "SSRF Loopback",
                        List.of("127.0.0.1"),
                        List.of(DiscoveryMethod.ICMP),
                        null, null, null, null, null, null, null, null, "admin"
                )
        );
        assertEquals("SSRF_TARGET_PROHIBITED", ex.getCode());
        assertTrue(ex.getMessage().contains("Loopback addresses"));
    }

    @Test
    void validateSSRF_MetadataAddressRejected() {
        DomainException ex = assertThrows(DomainException.class, () ->
                service.createDiscovery(
                        tenantId,
                        "SSRF Cloud Metadata",
                        List.of("169.254.169.254"),
                        List.of(DiscoveryMethod.ICMP),
                        null, null, null, null, null, null, null, null, "admin"
                )
        );
        assertEquals("SSRF_TARGET_PROHIBITED", ex.getCode());
        assertTrue(ex.getMessage().contains("strictly prohibited"));
    }

    @Test
    void validateSSRF_MulticastAndBroadcastRejected() {
        DomainException ex1 = assertThrows(DomainException.class, () ->
                service.createDiscovery(
                        tenantId,
                        "Multicast target",
                        List.of("224.0.0.1"),
                        List.of(DiscoveryMethod.ICMP),
                        null, null, null, null, null, null, null, null, "admin"
                )
        );
        assertEquals("SSRF_TARGET_PROHIBITED", ex1.getCode());

        DomainException ex2 = assertThrows(DomainException.class, () ->
                service.createDiscovery(
                        tenantId,
                        "Broadcast target",
                        List.of("255.255.255.255"),
                        List.of(DiscoveryMethod.ICMP),
                        null, null, null, null, null, null, null, null, "admin"
                )
        );
        assertEquals("SSRF_TARGET_PROHIBITED", ex2.getCode());
    }

    @Test
    void validateSecrets_RawCredentialsInPayloadProhibited() {
        DomainException ex = assertThrows(DomainException.class, () ->
                service.createDiscovery(
                        tenantId,
                        "Scan with password=secret123",
                        List.of("192.168.1.1"),
                        List.of(DiscoveryMethod.ICMP),
                        null, null, null, null, null, null, null, null, "admin"
                )
        );
        assertEquals("SECRET_MATERIAL_PROHIBITED", ex.getCode());
        assertTrue(ex.getMessage().contains("Raw credential or secret material is strictly prohibited"));
    }

    @Test
    void idempotencyReplay_SameTenantReturnsExistingRequest() {
        String key = "idem-scan-42";
        DiscoveryRequest existing = new DiscoveryRequest(
                UUID.fromString("018f0e0a-0000-7000-8000-000000000099"),
                tenantId,
                "Existing Scan",
                DiscoveryStatus.COMPLETED,
                "corr-1",
                key,
                null,
                null,
                "sensor-01",
                List.of(DiscoveryMethod.ICMP),
                "prof-1",
                161,
                2000,
                2,
                "admin",
                Instant.now(),
                Instant.now(),
                null,
                Map.of(),
                null
        );
        when(repository.findByIdempotencyKey(tenantId, key)).thenReturn(Optional.of(existing));

        DiscoveryRequest result = service.createDiscovery(
                tenantId,
                "Existing Scan",
                List.of("192.168.1.1"),
                List.of(DiscoveryMethod.ICMP),
                "prof-1", null, null, null, null, null, key, "corr-1", "admin"
        );

        assertEquals(existing.requestId(), result.requestId());
        verify(repository, never()).createRequest(any(), any());
    }

    @Test
    void nonCausalEvidenceAggregation_IcmpTimeoutDoesNotDeclareDeviceDown() {
        UUID reqId = UUID.fromString("018f0e0a-0000-7000-8000-000000000088");
        DiscoveryRequest req = new DiscoveryRequest(
                reqId, tenantId, "Test Scan", DiscoveryStatus.RUNNING, "corr-88",
                null, null, null, "sensor-01", List.of(DiscoveryMethod.ICMP),
                null, 161, 2000, 2, "admin", Instant.now(), null, null, Map.of(), null
        );
        when(repository.findById(tenantId, reqId)).thenReturn(Optional.of(req));
        when(repository.listTargets(tenantId, reqId)).thenReturn(List.of(
                new DiscoveryTarget(UUID.randomUUID(), tenantId, reqId, "192.168.1.200", TargetType.IPV4, "RUNNING", Instant.now(), Instant.now())
        ));

        DiscoveryService.SensorEvidenceItem icmpTimeout = new DiscoveryService.SensorEvidenceItem(
                "192.168.1.200",
                "ICMP",
                "ECHO",
                "INCONCLUSIVE",
                "UNKNOWN",
                "ICMP_TIMEOUT",
                Map.of("error", "timeout"),
                Map.of(),
                2000L
        );

        service.ingestResultsFromSensor("sensor-01", tenantId, reqId, List.of(icmpTimeout), "COMPLETED", null);

        // Verify evidence is saved and request completed without declaring device down
        verify(repository).recordEvidence(eq(tenantId), eq(reqId), any());
        verify(repository).updateStatus(eq(tenantId), eq(reqId), eq(DiscoveryStatus.COMPLETED), any(), any(), any(), any());
    }

    @Test
    void candidateCreation_AlwaysPendingReview_ZeroAutoProvisioning() {
        UUID reqId = UUID.fromString("018f0e0a-0000-7000-8000-000000000077");
        DiscoveryRequest req = new DiscoveryRequest(
                reqId, tenantId, "Subnet Scan", DiscoveryStatus.RUNNING, "corr-77",
                null, null, null, "sensor-01", List.of(DiscoveryMethod.ICMP, DiscoveryMethod.SNMP),
                null, 161, 2000, 2, "admin", Instant.now(), null, null, Map.of(), null
        );
        when(repository.findById(tenantId, reqId)).thenReturn(Optional.of(req));
        when(repository.listTargets(tenantId, reqId)).thenReturn(List.of(
                new DiscoveryTarget(UUID.randomUUID(), tenantId, reqId, "192.168.1.55", TargetType.IPV4, "RUNNING", Instant.now(), Instant.now())
        ));

        DiscoveryService.SensorEvidenceItem icmpSuccess = new DiscoveryService.SensorEvidenceItem(
                "192.168.1.55", "ICMP", "ECHO", "SUCCESS", "OBSERVED", "ICMP_ECHO_REPLY",
                Map.of("rtt_micros", 1200), Map.of(), 2L
        );
        DiscoveryService.SensorEvidenceItem snmpSuccess = new DiscoveryService.SensorEvidenceItem(
                "192.168.1.55", "SNMP", "GET_IDENTITY", "SUCCESS", "OBSERVED", "SNMP_V2C_SUCCESS",
                Map.of(
                        "sys_name", "edge-router-01",
                        "sys_descr", "Cisco IOS-XE 17.3",
                        "sys_object_id", "1.3.6.1.4.1.9.1.2000"
                ), Map.of(), 45L
        );

        service.ingestResultsFromSensor("sensor-01", tenantId, reqId, List.of(icmpSuccess, snmpSuccess), "COMPLETED", null);

        ArgumentCaptor<DiscoveryCandidate> captor = ArgumentCaptor.forClass(DiscoveryCandidate.class);
        verify(repository).upsertCandidate(captor.capture());

        DiscoveryCandidate c = captor.getValue();
        assertNotNull(c);

        // Non-negotiable: Candidate status MUST be strictly PENDING_REVIEW! Zero auto-provisioning!
        assertEquals(CandidateReviewStatus.PENDING_REVIEW, c.reviewStatus());
        assertEquals("192.168.1.55", c.targetAddress());
        assertEquals("edge-router-01", c.discoveredName());
        assertEquals(DiscoveryConfidence.HIGH, c.confidence());
        assertTrue(c.evidencedProtocols().contains(DiscoveryMethod.ICMP));
        assertTrue(c.evidencedProtocols().contains(DiscoveryMethod.SNMP));
    }
}
