package com.platform.core.domain.discovery;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.DiscoveryRepository;
import com.platform.core.persistence.DiscoveryRepository.AssignedRequest;
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
import static org.junit.jupiter.api.Assertions.assertNull;
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
    void validateTargetLimits_Max256EnforcedAsDefensiveGuard() {
        List<String> tooMany = new ArrayList<>();
        for (int i = 0; i < 257; i++) {
            tooMany.add("192.0.2." + (i % 250 + 1));
        }

        DomainException ex = assertThrows(DomainException.class, () ->
                service.createDiscovery(
                        tenantId, "Too Many Targets", tooMany, List.of(DiscoveryMethod.ICMP),
                        null, null, null, null, null, null, null, null, "admin"
                )
        );
        assertEquals("QUERY_LIMIT_EXCEEDED", ex.getCode());
        assertTrue(ex.getMessage().contains("defensive implementation limit"));
    }

    @Test
    void validateTargetLimits_BroadCidrRejectedByDefensiveExpansionGuard() {
        DomainException ex1 = assertThrows(DomainException.class, () ->
                service.createDiscovery(
                        tenantId, "Broad CIDR", List.of("10.0.0.0/16"), List.of(DiscoveryMethod.ICMP),
                        null, null, null, null, null, null, null, null, "admin"
                )
        );
        assertEquals("QUERY_LIMIT_EXCEEDED", ex1.getCode());
        assertTrue(ex1.getMessage().contains("Defensive CIDR expansion guard"));

        DomainException ex2 = assertThrows(DomainException.class, () ->
                service.createDiscovery(
                        tenantId, "Broad CIDR 23", List.of("192.168.0.0/23"), List.of(DiscoveryMethod.ICMP),
                        null, null, null, null, null, null, null, null, "admin"
                )
        );
        assertEquals("QUERY_LIMIT_EXCEEDED", ex2.getCode());
    }

    @Test
    void validateTargetLimits_ValidPrivateCidr24AllowedAndRemainsPending() {
        DiscoveryRequest created = service.createDiscovery(
                tenantId, "Valid Subnet", List.of("192.168.10.0/24"), List.of(DiscoveryMethod.ICMP),
                null, null, null, null, null, null, null, null, "admin"
        );

        assertNotNull(created);
        assertEquals(DiscoveryStatus.PENDING, created.status());
        assertNull(created.assignedSensorId());
        verify(repository).createRequest(any(DiscoveryRequest.class), any());
    }

    @Test
    void validateSSRF_LoopbackUnspecifiedLinkLocalMulticastAndMappedLoopbackRejected() {
        for (String target : List.of("127.0.0.1", "0.0.0.0", "::", "::1", "::ffff:127.0.0.1", "169.254.169.254", "fe80::1", "224.0.0.1", "ff02::1")) {
            DomainException ex = assertThrows(DomainException.class, () -> DiscoveryLimits.validateTargetAddress(target), target);
            assertEquals("SSRF_TARGET_PROHIBITED", ex.getCode(), target);
        }
    }

    @Test
    void validateSSRF_PrivateIpv4AndIpv6UlaPermitted() {
        assertEquals("10.10.10.10", DiscoveryLimits.validateAndNormalizeTarget("10.10.10.10"));
        assertTrue(DiscoveryLimits.validateAndNormalizeTarget("fd00::10").contains(":"));
    }

    @Test
    void validateSSRF_ScopedIpv6AndMalformedAddressRejected() {
        DomainException scoped = assertThrows(DomainException.class,
                () -> DiscoveryLimits.validateTargetAddress("fe80::1%eth0"));
        assertEquals("VALIDATION_FAILED", scoped.getCode());

        DomainException malformed = assertThrows(DomainException.class,
                () -> DiscoveryLimits.validateTargetAddress("300.1.1.1"));
        assertEquals("VALIDATION_FAILED", malformed.getCode());
    }

    @Test
    void validateSecrets_RawCredentialsInPayloadProhibited() {
        DomainException ex = assertThrows(DomainException.class, () ->
                service.createDiscovery(
                        tenantId, "Scan with password=secret123", List.of("192.168.1.1"), List.of(DiscoveryMethod.ICMP),
                        null, null, null, null, null, null, null, null, "admin"
                )
        );
        assertEquals("SECRET_MATERIAL_PROHIBITED", ex.getCode());
    }

    @Test
    void idempotencyReplay_SameTenantEquivalentPayloadReturnsExistingRequest() {
        String key = "idem-scan-42";
        UUID existingId = UUID.fromString("018f0e0a-0000-7000-8000-000000000099");
        DiscoveryRequest existing = new DiscoveryRequest(
                existingId, tenantId, "Existing Scan", DiscoveryStatus.COMPLETED, "corr-1", key,
                null, null, "sensor-01", List.of(DiscoveryMethod.ICMP), "prof-1",
                161, 2000, 2, "admin", Instant.now(), Instant.now(), null, Map.of(), null
        );
        when(repository.findByIdempotencyKey(tenantId, key)).thenReturn(Optional.of(existing));
        when(repository.listTargets(tenantId, existingId)).thenReturn(List.of(
                target(existingId, "192.168.1.1", TargetType.IPV4)
        ));

        DiscoveryRequest result = service.createDiscovery(
                tenantId, "Existing Scan", List.of("192.168.1.1"), List.of(DiscoveryMethod.ICMP),
                "prof-1", null, null, null, 2000, 2, key, "corr-2", "admin"
        );

        assertEquals(existing.requestId(), result.requestId());
        verify(repository, never()).createRequest(any(), any());
    }

    @Test
    void idempotencyReplay_SameKeyChangedPayloadRejected() {
        String key = "idem-conflict";
        UUID existingId = UUID.fromString("018f0e0a-0000-7000-8000-000000000098");
        DiscoveryRequest existing = new DiscoveryRequest(
                existingId, tenantId, "Existing Scan", DiscoveryStatus.PENDING, "corr-1", key,
                null, null, null, List.of(DiscoveryMethod.ICMP), null,
                161, 3000, 2, "admin", Instant.now(), null, null, Map.of(), null
        );
        when(repository.findByIdempotencyKey(tenantId, key)).thenReturn(Optional.of(existing));
        when(repository.listTargets(tenantId, existingId)).thenReturn(List.of(
                target(existingId, "192.168.1.1", TargetType.IPV4)
        ));

        DomainException ex = assertThrows(DomainException.class, () -> service.createDiscovery(
                tenantId, "Existing Scan", List.of("192.168.1.2"), List.of(DiscoveryMethod.ICMP),
                null, null, null, null, null, null, key, "corr-2", "admin"
        ));
        assertEquals("IDEMPOTENCY_CONFLICT", ex.getCode());
        verify(repository, never()).createRequest(any(), any());
    }

    @Test
    void sensorA_CannotSubmitSensorBTask() {
        UUID reqId = UUID.fromString("018f0e0a-0000-7000-8000-000000000081");
        when(repository.findAssignedForUpdate("sensor-a", reqId)).thenReturn(Optional.empty());

        DomainException ex = assertThrows(DomainException.class, () -> service.ingestResultsFromSensor(
                "sensor-a", reqId, 4L, List.of(), "COMPLETED", null));
        assertEquals("RESOURCE_NOT_FOUND", ex.getCode());
        verify(repository, never()).recordEvidence(any(), any(), any());
    }

    @Test
    void staleAssignmentFenceRejectedBeforeEvidencePersistence() {
        UUID reqId = UUID.fromString("018f0e0a-0000-7000-8000-000000000082");
        DiscoveryRequest req = runningRequest(reqId, "sensor-01", List.of(DiscoveryMethod.ICMP));
        when(repository.findAssignedForUpdate("sensor-01", reqId)).thenReturn(Optional.of(new AssignedRequest(req, 7L)));

        DomainException ex = assertThrows(DomainException.class, () -> service.ingestResultsFromSensor(
                "sensor-01", reqId, 6L, List.of(), "COMPLETED", null));
        assertEquals("STALE_ASSIGNMENT", ex.getCode());
        verify(repository, never()).recordEvidence(any(), any(), any());
    }

    @Test
    void cancelledRequestRejectsStaleSensorResult() {
        UUID reqId = UUID.fromString("018f0e0a-0000-7000-8000-000000000083");
        DiscoveryRequest cancelled = request(reqId, DiscoveryStatus.CANCELLED, "sensor-01", List.of(DiscoveryMethod.ICMP));
        when(repository.findAssignedForUpdate("sensor-01", reqId)).thenReturn(Optional.of(new AssignedRequest(cancelled, 3L)));

        DomainException ex = assertThrows(DomainException.class, () -> service.ingestResultsFromSensor(
                "sensor-01", reqId, 3L, List.of(), "COMPLETED", null));
        assertEquals("INVALID_STATE_TRANSITION", ex.getCode());
        verify(repository, never()).recordEvidence(any(), any(), any());
    }

    @Test
    void terminalResultReplayIsSafeNoOp() {
        UUID reqId = UUID.fromString("018f0e0a-0000-7000-8000-000000000084");
        DiscoveryRequest completed = request(reqId, DiscoveryStatus.COMPLETED, "sensor-01", List.of(DiscoveryMethod.ICMP));
        when(repository.findAssignedForUpdate("sensor-01", reqId)).thenReturn(Optional.of(new AssignedRequest(completed, 9L)));

        DiscoveryService.ResultIngestOutcome outcome = service.ingestResultsFromSensor(
                "sensor-01", reqId, 9L, List.of(), "COMPLETED", null);
        assertTrue(outcome.replay());
        assertEquals(DiscoveryStatus.COMPLETED, outcome.status());
        verify(repository, never()).recordEvidence(any(), any(), any());
        verify(repository, never()).upsertCandidate(any());
    }

    @Test
    void evidenceTargetMustBelongToAssignedRequest() {
        UUID reqId = UUID.fromString("018f0e0a-0000-7000-8000-000000000085");
        DiscoveryRequest req = runningRequest(reqId, "sensor-01", List.of(DiscoveryMethod.ICMP));
        when(repository.findAssignedForUpdate("sensor-01", reqId)).thenReturn(Optional.of(new AssignedRequest(req, 2L)));
        when(repository.listTargets(tenantId, reqId)).thenReturn(List.of(target(reqId, "192.168.1.10", TargetType.IPV4)));

        DomainException ex = assertThrows(DomainException.class, () -> service.ingestResultsFromSensor(
                "sensor-01", reqId, 2L,
                List.of(evidence("192.168.1.11", "ICMP", "ECHO", "SUCCESS", "OBSERVED", "ICMP_ECHO_REPLY", Map.of())),
                "COMPLETED", null));
        assertEquals("VALIDATION_FAILED", ex.getCode());
        verify(repository, never()).recordEvidence(any(), any(), any());
    }

    @Test
    void unsupportedProtocolDoesNotFallBackToIcmp() {
        UUID reqId = UUID.fromString("018f0e0a-0000-7000-8000-000000000086");
        DiscoveryRequest req = runningRequest(reqId, "sensor-01", List.of(DiscoveryMethod.ICMP));
        when(repository.findAssignedForUpdate("sensor-01", reqId)).thenReturn(Optional.of(new AssignedRequest(req, 2L)));
        when(repository.listTargets(tenantId, reqId)).thenReturn(List.of(target(reqId, "192.168.1.10", TargetType.IPV4)));

        DomainException ex = assertThrows(DomainException.class, () -> service.ingestResultsFromSensor(
                "sensor-01", reqId, 2L,
                List.of(evidence("192.168.1.10", "SSH", "CONNECT", "SUCCESS", "OBSERVED", "SSH_OK", Map.of())),
                "COMPLETED", null));
        assertEquals("VALIDATION_FAILED", ex.getCode());
        verify(repository, never()).recordEvidence(any(), any(), any());
    }

    @Test
    void timeoutAndProhibitedCausalConclusionsRemainRejected() {
        UUID reqId = UUID.fromString("018f0e0a-0000-7000-8000-000000000087");
        DiscoveryRequest req = runningRequest(reqId, "sensor-01", List.of(DiscoveryMethod.ICMP));
        when(repository.findAssignedForUpdate("sensor-01", reqId)).thenReturn(Optional.of(new AssignedRequest(req, 2L)));
        when(repository.listTargets(tenantId, reqId)).thenReturn(List.of(target(reqId, "192.168.1.10", TargetType.IPV4)));

        for (String code : List.of("DEVICE_DOWN", "FIREWALL_BLOCKED", "WRONG_CREDENTIAL")) {
            DomainException ex = assertThrows(DomainException.class, () -> service.ingestResultsFromSensor(
                    "sensor-01", reqId, 2L,
                    List.of(evidence("192.168.1.10", "ICMP", "ECHO", "INCONCLUSIVE", "UNKNOWN", code, Map.of())),
                    "COMPLETED", null), code);
            assertEquals("VALIDATION_FAILED", ex.getCode());
        }
    }

    @Test
    void secretBearingEvidenceKeyRejected() {
        UUID reqId = UUID.fromString("018f0e0a-0000-7000-8000-000000000089");
        DiscoveryRequest req = runningRequest(reqId, "sensor-01", List.of(DiscoveryMethod.ICMP));
        when(repository.findAssignedForUpdate("sensor-01", reqId)).thenReturn(Optional.of(new AssignedRequest(req, 2L)));
        when(repository.listTargets(tenantId, reqId)).thenReturn(List.of(target(reqId, "192.168.1.10", TargetType.IPV4)));

        DomainException ex = assertThrows(DomainException.class, () -> service.ingestResultsFromSensor(
                "sensor-01", reqId, 2L,
                List.of(evidence("192.168.1.10", "ICMP", "ECHO", "SUCCESS", "OBSERVED", "ICMP_ECHO_REPLY", Map.of("community", "redacted"))),
                "COMPLETED", null));
        assertEquals("SECRET_MATERIAL_PROHIBITED", ex.getCode());
        verify(repository, never()).recordEvidence(any(), any(), any());
    }

    @Test
    void nonCausalEvidenceAggregation_IcmpTimeoutPersistsInconclusiveUnknown() {
        UUID reqId = UUID.fromString("018f0e0a-0000-7000-8000-000000000088");
        DiscoveryRequest req = runningRequest(reqId, "sensor-01", List.of(DiscoveryMethod.ICMP));
        when(repository.findAssignedForUpdate("sensor-01", reqId)).thenReturn(Optional.of(new AssignedRequest(req, 3L)));
        when(repository.listTargets(tenantId, reqId)).thenReturn(List.of(target(reqId, "192.168.1.200", TargetType.IPV4)));

        DiscoveryService.SensorEvidenceItem icmpTimeout = evidence(
                "192.168.1.200", "ICMP", "ECHO", "INCONCLUSIVE", "UNKNOWN", "ICMP_TIMEOUT",
                Map.of("error", "timeout"));

        service.ingestResultsFromSensor("sensor-01", reqId, 3L, List.of(icmpTimeout), "COMPLETED", null);

        verify(repository).recordEvidence(eq(tenantId), eq(reqId), any());
        verify(repository).updateStatus(eq(tenantId), eq(reqId), eq(DiscoveryStatus.COMPLETED), any(), any(), any(), any());
    }

    @Test
    void candidateCreation_AlwaysPendingReview_ZeroAutoProvisioning() {
        UUID reqId = UUID.fromString("018f0e0a-0000-7000-8000-000000000077");
        DiscoveryRequest req = runningRequest(reqId, "sensor-01", List.of(DiscoveryMethod.ICMP, DiscoveryMethod.SNMP));
        when(repository.findAssignedForUpdate("sensor-01", reqId)).thenReturn(Optional.of(new AssignedRequest(req, 5L)));
        when(repository.listTargets(tenantId, reqId)).thenReturn(List.of(target(reqId, "192.168.1.55", TargetType.IPV4)));

        DiscoveryService.SensorEvidenceItem icmpSuccess = evidence(
                "192.168.1.55", "ICMP", "ECHO", "SUCCESS", "OBSERVED", "ICMP_ECHO_REPLY",
                Map.of("rtt_micros", 1200));
        DiscoveryService.SensorEvidenceItem snmpSuccess = evidence(
                "192.168.1.55", "SNMP", "GET_IDENTITY", "SUCCESS", "OBSERVED", "SNMP_GET_RESPONSE",
                Map.of("sys_name", "edge-router-01", "sys_descr", "Cisco IOS-XE 17.3",
                        "sys_object_id", "1.3.6.1.4.1.9.1.2000"));

        service.ingestResultsFromSensor("sensor-01", reqId, 5L, List.of(icmpSuccess, snmpSuccess), "COMPLETED", null);

        ArgumentCaptor<DiscoveryCandidate> captor = ArgumentCaptor.forClass(DiscoveryCandidate.class);
        verify(repository).upsertCandidate(captor.capture());
        DiscoveryCandidate candidate = captor.getValue();
        assertEquals(CandidateReviewStatus.PENDING_REVIEW, candidate.reviewStatus());
        assertEquals("192.168.1.55", candidate.targetAddress());
        assertEquals("edge-router-01", candidate.discoveredName());
        assertEquals(DiscoveryConfidence.HIGH, candidate.confidence());
        assertTrue(candidate.evidencedProtocols().contains(DiscoveryMethod.ICMP));
        assertTrue(candidate.evidencedProtocols().contains(DiscoveryMethod.SNMP));
    }

    private DiscoveryRequest runningRequest(UUID reqId, String sensorId, List<DiscoveryMethod> methods) {
        return request(reqId, DiscoveryStatus.RUNNING, sensorId, methods);
    }

    private DiscoveryRequest request(UUID reqId, DiscoveryStatus status, String sensorId, List<DiscoveryMethod> methods) {
        return new DiscoveryRequest(
                reqId, tenantId, "Test Scan", status, "corr-1", null,
                null, null, sensorId, methods, null, 161, 2000, 2,
                "admin", Instant.now(), Instant.now(), null, Map.of(), null
        );
    }

    private DiscoveryTarget target(UUID requestId, String address, TargetType type) {
        return new DiscoveryTarget(UUID.randomUUID(), tenantId, requestId, address, type, "RUNNING", Instant.now(), Instant.now());
    }

    private DiscoveryService.SensorEvidenceItem evidence(
            String target, String protocol, String method, String status, String reachability,
            String technicalCode, Map<String, Object> attributes) {
        return new DiscoveryService.SensorEvidenceItem(
                target, protocol, method, status, reachability, technicalCode,
                attributes, Map.of(), 5L
        );
    }
}
