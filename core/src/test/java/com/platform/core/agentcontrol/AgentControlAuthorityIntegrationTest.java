package com.platform.core.agentcontrol;

import com.platform.core.config.PersistenceConfig;
import com.platform.core.domain.inventory.Site;
import com.platform.core.domain.inventory.SiteService;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.persistence.SiteRepository;
import com.platform.core.persistence.TenantRepository;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.sensorcontrol.CertificateFingerprints;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.sensorcontrol.SensorControlService;
import com.platform.core.sensorcontrol.SensorRole;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

import javax.sql.DataSource;
import java.math.BigInteger;
import java.security.cert.X509Certificate;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.Date;
import java.util.List;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

@Tag("postgres")
class AgentControlAuthorityIntegrationTest {

    private PostgresTestSupport pg;
    private TenantService tenantService;
    private SiteService siteService;
    private SensorControlService sensorControl;
    private AgentControlRepository agentRepo;
    private AgentControlService agentControl;
    private AgentAuthorityQueryService authorityQuery;
    private SensorControlRepository sensorRepo;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();

        NamedParameterJdbcTemplate named = pg.namedJdbc();
        TenantRepository tenantRepo = new TenantRepository(named);
        SiteRepository siteRepo = new SiteRepository(named);
        tenantService = new TenantService(tenantRepo);
        siteService = new SiteService(siteRepo, tenantService);
        sensorRepo = new SensorControlRepository(named);
        SecurityAuditEmitter audit = new SecurityAuditEmitter();
        sensorControl = new SensorControlService(sensorRepo, audit);
        agentRepo = new AgentControlRepository(named);
        AssignmentMetrics assignmentMetrics = new AssignmentMetrics();
        agentControl = new AgentControlService(agentRepo, sensorRepo, audit, assignmentMetrics);
        authorityQuery = new AgentAuthorityQueryService(agentRepo, new AgentAuthorityMetrics());
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    private X509Certificate mockCert(byte[] der, String serialHex, Instant notBefore, Instant notAfter)
            throws Exception {
        return mockCert(der, serialHex, notBefore, notAfter, "CN=Platform Agent,O=Platform", null);
    }

    private X509Certificate mockCert(
            byte[] der,
            String serialHex,
            Instant notBefore,
            Instant notAfter,
            String subjectDn,
            java.util.Collection<List<?>> subjectAltNames
    ) throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        java.security.PublicKey publicKey = mock(java.security.PublicKey.class);
        byte[] spki = ("spki-" + serialHex).getBytes();
        BigInteger serial = new BigInteger(serialHex, 16);
        when(publicKey.getEncoded()).thenReturn(spki);
        when(cert.getEncoded()).thenReturn(der);
        when(cert.getSerialNumber()).thenReturn(serial);
        when(cert.getIssuerX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal("CN=Platform Test CA"));
        when(cert.getSubjectX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal(subjectDn));
        when(cert.getNotBefore()).thenReturn(Date.from(notBefore));
        when(cert.getNotAfter()).thenReturn(Date.from(notAfter));
        when(cert.getPublicKey()).thenReturn(publicKey);
        if (subjectAltNames != null) {
            when(cert.getSubjectAlternativeNames()).thenReturn(subjectAltNames);
        }
        return cert;
    }

    @Test
    void enrollBindAssignResolveFencingAndNegativeMatrix() throws Exception {
        Tenant tenant = tenantService.create("Agent Control Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant other = tenantService.create("Other Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Site siteA = siteService.create(tenant.tenantId(), null, "SiteA", "A", "UTC");
        Site siteB = siteService.create(tenant.tenantId(), null, "SiteB", "B", "UTC");

        sensorControl.enrollSensor("sensor-infra-01", tenant.tenantId(), siteA.siteId(),
                SensorRole.INFRASTRUCTURE, true);
        sensorControl.enrollSensor("sensor-infra-02", tenant.tenantId(), siteA.siteId(),
                SensorRole.INFRASTRUCTURE, true);
        sensorControl.enrollSensor("sensor-net-01", tenant.tenantId(), siteA.siteId(),
                SensorRole.NETWORK, true);
        sensorControl.enrollSensor("sensor-other-tenant", other.tenantId(), null,
                SensorRole.INFRASTRUCTURE, true);
        sensorControl.enrollSensor("sensor-disabled", tenant.tenantId(), siteA.siteId(),
                SensorRole.INFRASTRUCTURE, true);
        sensorControl.disableSensor("sensor-disabled");

        agentControl.enrollAgent("agent-01", tenant.tenantId(), siteA.siteId());

        Instant notBefore = Instant.now().minus(1, ChronoUnit.HOURS);
        Instant notAfter = Instant.now().plus(24, ChronoUnit.HOURS);
        X509Certificate cert1 = mockCert(new byte[]{10, 20, 30}, "aa01", notBefore, notAfter);
        String fp1 = CertificateFingerprints.sha256DerHex(cert1);
        agentControl.bindCertificate("agent-01", cert1);

        // Duplicate fingerprint bind fails
        assertThrows(AgentControlException.class, () -> agentControl.bindCertificate("agent-01", cert1));

        UUID boundEntity = UUID.randomUUID();
        pg.jdbc().update("""
                INSERT INTO platform.entity (tenant_id, entity_id, entity_type, display_name, lifecycle_state,
                    first_seen_at, last_seen_at, created_at, updated_at)
                VALUES (?, ?, 'HOST', 'Agent host', 'ACTIVE', now(), now(), now(), now())
                """, tenant.tenantId(), boundEntity);
        agentControl.establishEntityBinding(tenant.tenantId(), "agent-01", boundEntity, fp1, "host-a",
                "{\"family\":\"linux\"}", true);
        assertEquals(boundEntity, agentRepo.findActiveEntityBinding(tenant.tenantId(), "agent-01").orElseThrow().entityId());

        // NETWORK sensor lacks AGENT_INGRESS
        AgentControlException noIngress = assertThrows(AgentControlException.class,
                () -> agentControl.assignToSensor(tenant.tenantId(), "agent-01", "sensor-net-01"));
        assertEquals(AgentControlException.ERR_AGENT_SENSOR_INELIGIBLE, noIngress.getCode());

        // Disabled sensor rejected
        AgentControlException disabledSensor = assertThrows(AgentControlException.class,
                () -> agentControl.assignToSensor(tenant.tenantId(), "agent-01", "sensor-disabled"));
        assertEquals(AgentControlException.ERR_AGENT_SENSOR_INELIGIBLE, disabledSensor.getCode());

        // Cross-tenant assignment rejected
        AgentControlException crossTenant = assertThrows(AgentControlException.class,
                () -> agentControl.assignToSensor(tenant.tenantId(), "agent-01", "sensor-other-tenant"));
        assertEquals(AgentControlException.ERR_SENSOR_NOT_FOUND, crossTenant.getCode());

        // Cross-site invalid when both non-null and different
        agentControl.enrollAgent("agent-site-b", tenant.tenantId(), siteB.siteId());
        AgentControlException crossSite = assertThrows(AgentControlException.class,
                () -> agentControl.assignToSensor(tenant.tenantId(), "agent-site-b", "sensor-infra-01"));
        assertEquals(AgentControlException.ERR_AGENT_SITE_INCOMPATIBLE, crossSite.getCode());

        AgentSensorAssignment a1 = agentControl.assignToSensor(
                tenant.tenantId(), "agent-01", "sensor-infra-01");
        assertEquals(1L, a1.generation());

        SensorScope sensor1 = new SensorScope("sensor-infra-01", tenant.tenantId(), siteA.siteId(), Set.of(), true);
        AgentAuthorityView view = authorityQuery.resolveForSensor(sensor1, fp1);
        assertEquals("agent-01", view.agentId());
        assertEquals(1L, view.assignmentGeneration());

        // Wrong sensor rejected
        SensorScope sensor2 = new SensorScope("sensor-infra-02", tenant.tenantId(), siteA.siteId(), Set.of(), true);
        IngestionException wrongSensor = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(sensor2, fp1));
        assertEquals("ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH", wrongSensor.getCode());

        // Exact Sensor ID case mismatch
        SensorScope caseMismatch = new SensorScope("Sensor-Infra-01", tenant.tenantId(), siteA.siteId(), Set.of(), true);
        IngestionException caseEx = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(caseMismatch, fp1));
        assertEquals("ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH", caseEx.getCode());

        // Reassign with fencing
        AgentSensorAssignment a2 = agentControl.reassignToSensor(
                tenant.tenantId(), "agent-01", "sensor-infra-02", 1L);
        assertEquals(2L, a2.generation());
        assertEquals(boundEntity, agentRepo.findActiveEntityBinding(tenant.tenantId(), "agent-01").orElseThrow().entityId());

        // P0 fencing completeness with claimed generation:
        // Sensor1 + gen N rejected after reassign to Sensor2 gen N+1
        IngestionException s1Claim = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(sensor1, fp1, 1L));
        assertEquals("ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH", s1Claim.getCode());
        // Sensor2 + gen N rejected via claimed generation
        IngestionException s2StaleGen = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(sensor2, fp1, 1L));
        assertEquals("ERR_AGENT_ASSIGNMENT_GENERATION_MISMATCH", s2StaleGen.getCode());
        // Sensor2 + gen N+1 accepted
        AgentAuthorityView afterReassign = authorityQuery.resolveForSensor(sensor2, fp1, 2L);
        assertEquals(2L, afterReassign.assignmentGeneration());
        assertEquals("sensor-infra-02", afterReassign.sensorId());
        // Lookup without claim still returns current fence
        assertEquals(2L, authorityQuery.resolveForSensor(sensor2, fp1).assignmentGeneration());

        // Stale CAS generation conflict
        AgentControlException conflict = assertThrows(AgentControlException.class,
                () -> agentControl.reassignToSensor(tenant.tenantId(), "agent-01", "sensor-infra-01", 1L));
        assertEquals(AgentControlException.ERR_AGENT_ASSIGNMENT_CONFLICT, conflict.getCode());

        // Rotate supersedes old
        X509Certificate cert2 = mockCert(new byte[]{40, 50, 60}, "bb02", notBefore, notAfter);
        String fp2 = CertificateFingerprints.sha256DerHex(cert2);
        assertNotEquals(fp1, fp2);
        agentControl.rotateCertificate("agent-01", cert2);
        var rotatedBinding = agentRepo.findActiveEntityBinding(tenant.tenantId(), "agent-01").orElseThrow();
        assertEquals(boundEntity, rotatedBinding.entityId());
        assertEquals(fp2, rotatedBinding.certificateIdentity());
        assertEquals(1, rotatedBinding.reinstallGeneration());
        IngestionException superseded = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(sensor2, fp1));
        assertEquals("ERR_AGENT_CREDENTIAL_REVOKED", superseded.getCode());
        assertEquals("agent-01", authorityQuery.resolveForSensor(sensor2, fp2).agentId());

        // Unknown fingerprint
        X509Certificate unknown = mockCert(new byte[]{99}, "dead", notBefore, notAfter);
        IngestionException unknownEx = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(sensor2, CertificateFingerprints.sha256DerHex(unknown)));
        assertEquals("ERR_AGENT_CREDENTIAL_UNKNOWN", unknownEx.getCode());

        // Revoke credential
        agentControl.revokeCredential(fp2);
        // Already accepted Sensor custody retains canonical entity; new credential
        // admission still fails below. Binding revocation is a separate authority.
        assertEquals(boundEntity, agentRepo.findActiveEntityBinding(tenant.tenantId(), "agent-01").orElseThrow().entityId());
        IngestionException revokedCred = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(sensor2, fp2));
        assertEquals("ERR_AGENT_CREDENTIAL_REVOKED", revokedCred.getCode());

        // Re-bind and disable agent
        X509Certificate cert3 = mockCert(new byte[]{70, 80, 90}, "cc03", notBefore, notAfter);
        String fp3 = CertificateFingerprints.sha256DerHex(cert3);
        agentControl.bindCertificate("agent-01", cert3);
        agentControl.disableAgent("agent-01");
        IngestionException disabled = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(sensor2, fp3));
        assertEquals("ERR_AGENT_DISABLED", disabled.getCode());
    }

    @Test
    void revokeAgentDistinctFromDisableAndBlocksAuthority() throws Exception {
        Tenant tenant = tenantService.create("Revoke Agent Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-infra-r", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        agentControl.enrollAgent("agent-rev", tenant.tenantId(), null);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate cert = mockCert(new byte[]{1, 2, 3}, "a001", notBefore, notAfter);
        String fp = CertificateFingerprints.sha256DerHex(cert);
        agentControl.bindCertificate("agent-rev", cert);
        agentControl.assignToSensor(tenant.tenantId(), "agent-rev", "sensor-infra-r");

        agentControl.revokeAgent("agent-rev");
        assertEquals(AgentLifecycleStatus.REVOKED,
                agentRepo.findPrincipalById("agent-rev").orElseThrow().status());
        assertTrue(agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-rev").isEmpty());
        assertTrue(agentRepo.findActiveAssignmentForAgent(tenant.tenantId(), "agent-rev").isEmpty());

        SensorScope scope = new SensorScope("sensor-infra-r", tenant.tenantId(), null, Set.of(), true);
        // Credential revoke happens first in the resolve chain after agent revoke.
        IngestionException ex = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(scope, fp));
        assertEquals("ERR_AGENT_CREDENTIAL_REVOKED", ex.getCode());
    }

    @Test
    void cnSpoofDifferentDerSameCnOnlyEnrolledFingerprintAuthorizes() throws Exception {
        Tenant tenant = tenantService.create("CN Spoof Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-infra-cn", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        agentControl.enrollAgent("agent-cn", tenant.tenantId(), null);

        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        String sameCn = "CN=agent-cn,O=Platform";
        X509Certificate enrolled = mockCert(new byte[]{10, 11, 12}, "c101", notBefore, notAfter, sameCn, null);
        X509Certificate spoof = mockCert(new byte[]{20, 21, 22}, "c102", notBefore, notAfter, sameCn, null);
        String fpEnrolled = CertificateFingerprints.sha256DerHex(enrolled);
        String fpSpoof = CertificateFingerprints.sha256DerHex(spoof);
        assertNotEquals(fpEnrolled, fpSpoof);

        agentControl.bindCertificate("agent-cn", enrolled);
        agentControl.assignToSensor(tenant.tenantId(), "agent-cn", "sensor-infra-cn");

        SensorScope scope = new SensorScope("sensor-infra-cn", tenant.tenantId(), null, Set.of(), true);
        assertEquals("agent-cn", authorityQuery.resolveForSensor(scope, fpEnrolled).agentId());
        IngestionException spoofEx = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(scope, fpSpoof));
        assertEquals("ERR_AGENT_CREDENTIAL_UNKNOWN", spoofEx.getCode());
    }

    @Test
    void sanSpoofDifferentDerOnlyEnrolledFingerprintAuthorizes() throws Exception {
        Tenant tenant = tenantService.create("SAN Spoof Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-infra-san", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        agentControl.enrollAgent("agent-san", tenant.tenantId(), null);

        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        @SuppressWarnings({"unchecked", "rawtypes"})
        java.util.Collection<List<?>> san = (java.util.Collection) List.of(
                List.of(2, "agent-san.agent.platform.local"));
        X509Certificate enrolled = mockCert(
                new byte[]{30, 31, 32}, "a101", notBefore, notAfter, "CN=agent-san,O=Platform", san);
        X509Certificate spoof = mockCert(
                new byte[]{40, 41, 42}, "a102", notBefore, notAfter, "CN=agent-san,O=Platform", san);
        String fpEnrolled = CertificateFingerprints.sha256DerHex(enrolled);
        String fpSpoof = CertificateFingerprints.sha256DerHex(spoof);
        assertNotEquals(fpEnrolled, fpSpoof);

        agentControl.bindCertificate("agent-san", enrolled);
        agentControl.assignToSensor(tenant.tenantId(), "agent-san", "sensor-infra-san");

        SensorScope scope = new SensorScope("sensor-infra-san", tenant.tenantId(), null, Set.of(), true);
        assertEquals("agent-san", authorityQuery.resolveForSensor(scope, fpEnrolled).agentId());
        IngestionException spoofEx = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(scope, fpSpoof));
        assertEquals("ERR_AGENT_CREDENTIAL_UNKNOWN", spoofEx.getCode());
    }

    @Test
    void duplicateFingerprintCannotBindTwoAgents() throws Exception {
        Tenant tenant = tenantService.create("Dup Agent Fingerprint Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        agentControl.enrollAgent("agent-alpha", tenant.tenantId(), null);
        agentControl.enrollAgent("agent-beta", tenant.tenantId(), null);

        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate shared = mockCert(new byte[]{51, 52, 53}, "f101", notBefore, notAfter);
        agentControl.bindCertificate("agent-alpha", shared);

        AgentControlException ex = assertThrows(AgentControlException.class,
                () -> agentControl.bindCertificate("agent-beta", shared));
        assertEquals(AgentControlException.ERR_AGENT_CREDENTIAL_DUPLICATE, ex.getCode());

        sensorControl.enrollSensor("sensor-infra-dup", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        agentControl.assignToSensor(tenant.tenantId(), "agent-alpha", "sensor-infra-dup");
        SensorScope scope = new SensorScope("sensor-infra-dup", tenant.tenantId(), null, Set.of(), true);
        assertEquals("agent-alpha",
                authorityQuery.resolveForSensor(scope, CertificateFingerprints.sha256DerHex(shared)).agentId());
    }

    @Test
    void assignmentToRevokedSensorRejected() throws Exception {
        Tenant tenant = tenantService.create("Revoked Sensor Assign Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-revoked-tgt", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        sensorControl.revokeSensor("sensor-revoked-tgt");
        assertEquals(com.platform.core.sensorcontrol.SensorLifecycleStatus.REVOKED,
                sensorRepo.findPrincipalById("sensor-revoked-tgt").orElseThrow().status());

        agentControl.enrollAgent("agent-rev-sensor", tenant.tenantId(), null);
        AgentControlException ex = assertThrows(AgentControlException.class,
                () -> agentControl.assignToSensor(tenant.tenantId(), "agent-rev-sensor", "sensor-revoked-tgt"));
        assertEquals(AgentControlException.ERR_AGENT_SENSOR_INELIGIBLE, ex.getCode());
    }

    @Test
    void revokedSensorPrincipalRemainsIneligibleForAssignment() throws Exception {
        Tenant tenant = tenantService.create("Revoked Sensor Principal Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-rev-prin", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        agentControl.enrollAgent("agent-rev-prin", tenant.tenantId(), null);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate cert = mockCert(new byte[]{61, 62, 63}, "b201", notBefore, notAfter);
        agentControl.bindCertificate("agent-rev-prin", cert);
        agentControl.assignToSensor(tenant.tenantId(), "agent-rev-prin", "sensor-rev-prin");

        sensorControl.revokeSensor("sensor-rev-prin");
        assertEquals(com.platform.core.sensorcontrol.SensorLifecycleStatus.REVOKED,
                sensorRepo.findPrincipalById("sensor-rev-prin").orElseThrow().status());

        // Further assignment / reassignment to revoked Sensor principal is rejected.
        AgentControlException reassign = assertThrows(AgentControlException.class,
                () -> agentControl.reassignToSensor(tenant.tenantId(), "agent-rev-prin", "sensor-rev-prin", 1L));
        assertEquals(AgentControlException.ERR_AGENT_SENSOR_INELIGIBLE, reassign.getCode());
    }

    @Test
    void expiredAndNotYetValidCredentialsFailClosed() throws Exception {
        Tenant tenant = tenantService.create("Validity Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-infra-v", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        agentControl.enrollAgent("agent-v", tenant.tenantId(), null);

        Instant pastBefore = Instant.now().minus(48, ChronoUnit.HOURS);
        Instant pastAfter = Instant.now().minus(1, ChronoUnit.HOURS);
        X509Certificate expired = mockCert(new byte[]{11}, "e001", pastBefore, pastAfter);
        agentControl.bindCertificate("agent-v", expired);
        agentControl.assignToSensor(tenant.tenantId(), "agent-v", "sensor-infra-v");

        SensorScope scope = new SensorScope("sensor-infra-v", tenant.tenantId(), null, Set.of(), true);
        IngestionException expiredEx = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(scope, CertificateFingerprints.sha256DerHex(expired)));
        assertEquals("ERR_AGENT_CREDENTIAL_EXPIRED", expiredEx.getCode());

        agentControl.revokeCredential(CertificateFingerprints.sha256DerHex(expired));
        Instant futureBefore = Instant.now().plus(1, ChronoUnit.HOURS);
        Instant futureAfter = Instant.now().plus(48, ChronoUnit.HOURS);
        X509Certificate notYet = mockCert(new byte[]{12}, "e002", futureBefore, futureAfter);
        agentControl.bindCertificate("agent-v", notYet);
        IngestionException notYetEx = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(scope, CertificateFingerprints.sha256DerHex(notYet)));
        assertEquals("ERR_AGENT_CREDENTIAL_EXPIRED", notYetEx.getCode());
    }

    @Test
    void assignAfterRevokeAdvancesGenerationFenceNotReset() throws Exception {
        Tenant tenant = tenantService.create("Gen Fence After Revoke Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-fence-1", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        agentControl.enrollAgent("agent-fence", tenant.tenantId(), null);

        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate cert = mockCert(new byte[]{71, 72, 73}, "f301", notBefore, notAfter);
        String fp = CertificateFingerprints.sha256DerHex(cert);
        agentControl.bindCertificate("agent-fence", cert);

        AgentSensorAssignment gen1 = agentControl.assignToSensor(
                tenant.tenantId(), "agent-fence", "sensor-fence-1");
        assertEquals(1L, gen1.generation());

        agentControl.revokeAssignment(tenant.tenantId(), "agent-fence");
        assertTrue(agentRepo.findActiveAssignmentForAgent(tenant.tenantId(), "agent-fence").isEmpty());
        assertEquals(1L, agentRepo.maxGenerationForAgent(tenant.tenantId(), "agent-fence"));

        AgentSensorAssignment gen2 = agentControl.assignToSensor(
                tenant.tenantId(), "agent-fence", "sensor-fence-1");
        assertEquals(2L, gen2.generation());
        assertEquals(2L, agentRepo.maxGenerationForAgent(tenant.tenantId(), "agent-fence"));

        SensorScope scope = new SensorScope("sensor-fence-1", tenant.tenantId(), null, Set.of(), true);
        IngestionException staleClaim = assertThrows(IngestionException.class,
                () -> authorityQuery.resolveForSensor(scope, fp, 1L));
        assertEquals("ERR_AGENT_ASSIGNMENT_GENERATION_MISMATCH", staleClaim.getCode());

        AgentAuthorityView accepted = authorityQuery.resolveForSensor(scope, fp, 2L);
        assertEquals("agent-fence", accepted.agentId());
        assertEquals(2L, accepted.assignmentGeneration());
    }

    @Test
    void concurrentReassignmentOneWinsGenerationsCorrect() throws Exception {
        Tenant tenant = tenantService.create("Concurrency Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-c1", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        sensorControl.enrollSensor("sensor-c2", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        agentControl.enrollAgent("agent-c", tenant.tenantId(), null);
        AgentSensorAssignment initial = agentControl.assignToSensor(
                tenant.tenantId(), "agent-c", "sensor-c1");
        assertEquals(1L, initial.generation());

        CountDownLatch start = new CountDownLatch(1);
        AtomicInteger successes = new AtomicInteger();
        AtomicInteger conflicts = new AtomicInteger();
        AtomicReference<AgentSensorAssignment> winner = new AtomicReference<>();
        ExecutorService pool = Executors.newFixedThreadPool(2);
        try {
            Future<?> f1 = pool.submit(() -> {
                start.await();
                try {
                    AgentSensorAssignment a = agentControl.reassignToSensor(
                            tenant.tenantId(), "agent-c", "sensor-c2", 1L);
                    successes.incrementAndGet();
                    winner.set(a);
                } catch (AgentControlException e) {
                    if (AgentControlException.ERR_AGENT_ASSIGNMENT_CONFLICT.equals(e.getCode())) {
                        conflicts.incrementAndGet();
                    } else {
                        throw e;
                    }
                }
                return null;
            });
            Future<?> f2 = pool.submit(() -> {
                start.await();
                try {
                    AgentSensorAssignment a = agentControl.reassignToSensor(
                            tenant.tenantId(), "agent-c", "sensor-c2", 1L);
                    successes.incrementAndGet();
                    winner.set(a);
                } catch (AgentControlException e) {
                    if (AgentControlException.ERR_AGENT_ASSIGNMENT_CONFLICT.equals(e.getCode())) {
                        conflicts.incrementAndGet();
                    } else {
                        throw e;
                    }
                }
                return null;
            });
            start.countDown();
            f1.get(30, TimeUnit.SECONDS);
            f2.get(30, TimeUnit.SECONDS);
        } finally {
            pool.shutdownNow();
        }

        assertEquals(1, successes.get());
        assertEquals(1, conflicts.get());
        assertEquals(2L, winner.get().generation());
        AgentSensorAssignment active = agentRepo.findActiveAssignmentForAgent(tenant.tenantId(), "agent-c")
                .orElseThrow();
        assertEquals(2L, active.generation());
        assertEquals("sensor-c2", active.sensorId());
        List<AgentSensorAssignment> forSensor = agentRepo.findActiveAssignmentsForSensor(
                tenant.tenantId(), "sensor-c2");
        assertEquals(1, forSensor.size());
    }

    @Test
    void controlGenerationBoundary_maxSafeMinusOneAdvancesAndExhaustionPreservesActive() throws Exception {
        Tenant tenant = tenantService.create("Gen Fence Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-g1", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        sensorControl.enrollSensor("sensor-g2", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        agentControl.enrollAgent("agent-gen", tenant.tenantId(), null);

        AgentSensorAssignment seeded = agentControl.assignToSensor(
                tenant.tenantId(), "agent-gen", "sensor-g1");
        assertEquals(1L, seeded.generation());

        long maxSafe = com.platform.core.control.ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION;
        pg.jdbc().update("""
                UPDATE platform.agent_sensor_assignment
                   SET generation = ?
                 WHERE assignment_id = ?::uuid
                """, maxSafe - 1L, seeded.assignmentId());

        AgentSensorAssignment atMax = agentControl.reassignToSensor(
                tenant.tenantId(), "agent-gen", "sensor-g2", maxSafe - 1L);
        assertEquals(maxSafe, atMax.generation());
        assertEquals("sensor-g2", atMax.sensorId());
        assertEquals(AgentAssignmentStatus.ACTIVE, atMax.status());

        // Idempotent path is N/A for assignment; mutation requiring increment must exhaust.
        AgentControlException exhausted = assertThrows(AgentControlException.class,
                () -> agentControl.reassignToSensor(tenant.tenantId(), "agent-gen", "sensor-g1", maxSafe));
        assertEquals(AgentControlException.ERR_CONTROL_GENERATION_EXHAUSTED, exhausted.getCode());

        AgentSensorAssignment stillActive = agentRepo.findActiveAssignmentForAgent(tenant.tenantId(), "agent-gen")
                .orElseThrow();
        assertEquals(maxSafe, stillActive.generation());
        assertEquals("sensor-g2", stillActive.sensorId());
        assertEquals(AgentAssignmentStatus.ACTIVE, stillActive.status());

        // Historical max at MAX_SAFE with no ACTIVE: assign must not reset to 1.
        agentControl.revokeAssignment(tenant.tenantId(), "agent-gen");
        assertTrue(agentRepo.findActiveAssignmentForAgent(tenant.tenantId(), "agent-gen").isEmpty());
        AgentControlException histExhausted = assertThrows(AgentControlException.class,
                () -> agentControl.assignToSensor(tenant.tenantId(), "agent-gen", "sensor-g1"));
        assertEquals(AgentControlException.ERR_CONTROL_GENERATION_EXHAUSTED, histExhausted.getCode());
        assertTrue(agentRepo.findActiveAssignmentForAgent(tenant.tenantId(), "agent-gen").isEmpty());

        AgentControlException overMax = assertThrows(AgentControlException.class,
                () -> agentControl.reassignToSensor(
                        tenant.tenantId(), "agent-gen", "sensor-g1", maxSafe + 1L));
        assertEquals(AgentControlException.ERR_AGENT_STATE_INVALID, overMax.getCode());
    }

    @Test
    void reassignStaleMaxSafeClaimAgainstLowerAuthoritativeGenerationIsConflictNotExhaustion()
            throws Exception {
        Tenant tenant = tenantService.create("Stale MaxSafe Claim Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-stale-1", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        sensorControl.enrollSensor("sensor-stale-2", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);
        agentControl.enrollAgent("agent-stale", tenant.tenantId(), null);

        AgentSensorAssignment active = agentControl.assignToSensor(
                tenant.tenantId(), "agent-stale", "sensor-stale-1");
        assertEquals(1L, active.generation());
        assertEquals("sensor-stale-1", active.sensorId());

        long maxSafe = com.platform.core.control.ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION;
        long assignmentCountBefore = pg.jdbc().queryForObject("""
                SELECT COUNT(*) FROM platform.agent_sensor_assignment
                 WHERE tenant_id = ? AND agent_id = 'agent-stale'
                """, Long.class, tenant.tenantId());

        AgentControlException conflict = assertThrows(AgentControlException.class,
                () -> agentControl.reassignToSensor(
                        tenant.tenantId(), "agent-stale", "sensor-stale-2", maxSafe));
        assertEquals(AgentControlException.ERR_AGENT_ASSIGNMENT_CONFLICT, conflict.getCode());

        AgentSensorAssignment preserved = agentRepo.findActiveAssignmentForAgent(tenant.tenantId(), "agent-stale")
                .orElseThrow();
        assertEquals(1L, preserved.generation());
        assertEquals("sensor-stale-1", preserved.sensorId());
        assertEquals(AgentAssignmentStatus.ACTIVE, preserved.status());
        assertEquals(active.assignmentId(), preserved.assignmentId());

        long assignmentCountAfter = pg.jdbc().queryForObject("""
                SELECT COUNT(*) FROM platform.agent_sensor_assignment
                 WHERE tenant_id = ? AND agent_id = 'agent-stale'
                """, Long.class, tenant.tenantId());
        assertEquals(assignmentCountBefore, assignmentCountAfter);
    }

    @Test
    void concurrentRotateSamePrincipal_serializesToSingleActive() throws Exception {
        Tenant tenant = tenantService.create("Rot Concurrency Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        agentControl.enrollAgent("agent-rot-c", tenant.tenantId(), null);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate oldCert = mockCert(new byte[]{1, 2, 3}, "a001", notBefore, notAfter);
        String oldFp = CertificateFingerprints.sha256DerHex(oldCert);
        agentControl.bindCertificate("agent-rot-c", oldCert);

        X509Certificate certA = mockCert(new byte[]{4, 5, 6}, "a002", notBefore, notAfter);
        X509Certificate certB = mockCert(new byte[]{7, 8, 9}, "a003", notBefore, notAfter);
        String fpA = CertificateFingerprints.sha256DerHex(certA);
        String fpB = CertificateFingerprints.sha256DerHex(certB);

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class);
            ctx.registerBean(AgentControlRepository.class, () -> agentRepo);
            ctx.registerBean(SensorControlRepository.class, () -> sensorRepo);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.registerBean(AssignmentMetrics.class, AssignmentMetrics::new);
            ctx.register(AgentControlService.class);
            ctx.refresh();

            AgentControlService txService = ctx.getBean(AgentControlService.class);
            CountDownLatch start = new CountDownLatch(1);
            AtomicInteger successes = new AtomicInteger();
            ExecutorService pool = Executors.newFixedThreadPool(2);
            try {
                Future<?> f1 = pool.submit(() -> {
                    start.await();
                    txService.rotateCertificate("agent-rot-c", certA);
                    successes.incrementAndGet();
                    return null;
                });
                Future<?> f2 = pool.submit(() -> {
                    start.await();
                    txService.rotateCertificate("agent-rot-c", certB);
                    successes.incrementAndGet();
                    return null;
                });
                start.countDown();
                f1.get(30, TimeUnit.SECONDS);
                f2.get(30, TimeUnit.SECONDS);
            } finally {
                pool.shutdownNow();
            }
            assertEquals(2, successes.get());
        }

        List<AgentCertificateCredential> active =
                agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-rot-c");
        assertEquals(1, active.size());
        String winnerFp = active.get(0).fingerprintSha256();
        assertTrue(winnerFp.equals(fpA) || winnerFp.equals(fpB));
        assertEquals(AgentCredentialStatus.SUPERSEDED,
                agentRepo.findCredentialByFingerprint(oldFp).orElseThrow().status());
        String loserFp = winnerFp.equals(fpA) ? fpB : fpA;
        assertEquals(AgentCredentialStatus.SUPERSEDED,
                agentRepo.findCredentialByFingerprint(loserFp).orElseThrow().status());
        assertEquals(AgentCredentialStatus.ACTIVE,
                agentRepo.findCredentialByFingerprint(winnerFp).orElseThrow().status());
    }

    @Test
    void concurrentRotateVersusRevoke_oldFingerprintNotActive() throws Exception {
        Tenant tenant = tenantService.create("Rot Revoke Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        agentControl.enrollAgent("agent-rot-rev", tenant.tenantId(), null);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate oldCert = mockCert(new byte[]{11, 12, 13}, "b101", notBefore, notAfter);
        String oldFp = CertificateFingerprints.sha256DerHex(oldCert);
        agentControl.bindCertificate("agent-rot-rev", oldCert);
        X509Certificate newCert = mockCert(new byte[]{14, 15, 16}, "b102", notBefore, notAfter);
        String newFp = CertificateFingerprints.sha256DerHex(newCert);

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class);
            ctx.registerBean(AgentControlRepository.class, () -> agentRepo);
            ctx.registerBean(SensorControlRepository.class, () -> sensorRepo);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.registerBean(AssignmentMetrics.class, AssignmentMetrics::new);
            ctx.register(AgentControlService.class);
            ctx.refresh();

            AgentControlService txService = ctx.getBean(AgentControlService.class);
            CountDownLatch start = new CountDownLatch(1);
            ExecutorService pool = Executors.newFixedThreadPool(2);
            try {
                Future<?> rotate = pool.submit(() -> {
                    start.await();
                    txService.rotateCertificate("agent-rot-rev", newCert);
                    return null;
                });
                Future<?> revoke = pool.submit(() -> {
                    start.await();
                    txService.revokeCredential(oldFp);
                    return null;
                });
                start.countDown();
                rotate.get(30, TimeUnit.SECONDS);
                revoke.get(30, TimeUnit.SECONDS);
            } finally {
                pool.shutdownNow();
            }
        }

        assertNotEquals(AgentCredentialStatus.ACTIVE,
                agentRepo.findCredentialByFingerprint(oldFp).orElseThrow().status());
        List<AgentCertificateCredential> active =
                agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-rot-rev");
        assertEquals(1, active.size());
        assertEquals(newFp, active.get(0).fingerprintSha256());
    }

    @Test
    void concurrentRotateIndependentPrincipals_remainIndependentlyOperable() throws Exception {
        Tenant tenant = tenantService.create("Rot Indep Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        agentControl.enrollAgent("agent-indep-a", tenant.tenantId(), null);
        agentControl.enrollAgent("agent-indep-b", tenant.tenantId(), null);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        agentControl.bindCertificate("agent-indep-a",
                mockCert(new byte[]{21, 22, 23}, "c101", notBefore, notAfter));
        agentControl.bindCertificate("agent-indep-b",
                mockCert(new byte[]{24, 25, 26}, "c201", notBefore, notAfter));
        X509Certificate nextA = mockCert(new byte[]{27, 28, 29}, "c102", notBefore, notAfter);
        X509Certificate nextB = mockCert(new byte[]{30, 31, 32}, "c202", notBefore, notAfter);
        String fpA = CertificateFingerprints.sha256DerHex(nextA);
        String fpB = CertificateFingerprints.sha256DerHex(nextB);

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class);
            ctx.registerBean(AgentControlRepository.class, () -> agentRepo);
            ctx.registerBean(SensorControlRepository.class, () -> sensorRepo);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.registerBean(AssignmentMetrics.class, AssignmentMetrics::new);
            ctx.register(AgentControlService.class);
            ctx.refresh();

            AgentControlService txService = ctx.getBean(AgentControlService.class);
            CountDownLatch start = new CountDownLatch(1);
            ExecutorService pool = Executors.newFixedThreadPool(2);
            try {
                Future<?> f1 = pool.submit(() -> {
                    start.await();
                    txService.rotateCertificate("agent-indep-a", nextA);
                    return null;
                });
                Future<?> f2 = pool.submit(() -> {
                    start.await();
                    txService.rotateCertificate("agent-indep-b", nextB);
                    return null;
                });
                start.countDown();
                f1.get(30, TimeUnit.SECONDS);
                f2.get(30, TimeUnit.SECONDS);
            } finally {
                pool.shutdownNow();
            }
        }

        List<AgentCertificateCredential> activeA =
                agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-indep-a");
        List<AgentCertificateCredential> activeB =
                agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-indep-b");
        assertEquals(1, activeA.size());
        assertEquals(1, activeB.size());
        assertEquals(fpA, activeA.get(0).fingerprintSha256());
        assertEquals(fpB, activeB.get(0).fingerprintSha256());
    }

    @Test
    void concurrentRotateDifferentTenants_keepsTenantIsolation() throws Exception {
        Tenant tenantA = tenantService.create("Rot Tenant A", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant tenantB = tenantService.create("Rot Tenant B", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        agentControl.enrollAgent("agent-tenant-a", tenantA.tenantId(), null);
        agentControl.enrollAgent("agent-tenant-b", tenantB.tenantId(), null);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        agentControl.bindCertificate("agent-tenant-a",
                mockCert(new byte[]{41, 42, 43}, "d101", notBefore, notAfter));
        agentControl.bindCertificate("agent-tenant-b",
                mockCert(new byte[]{44, 45, 46}, "d201", notBefore, notAfter));
        X509Certificate nextA = mockCert(new byte[]{47, 48, 49}, "d102", notBefore, notAfter);
        X509Certificate nextB = mockCert(new byte[]{50, 51, 52}, "d202", notBefore, notAfter);

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class);
            ctx.registerBean(AgentControlRepository.class, () -> agentRepo);
            ctx.registerBean(SensorControlRepository.class, () -> sensorRepo);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.registerBean(AssignmentMetrics.class, AssignmentMetrics::new);
            ctx.register(AgentControlService.class);
            ctx.refresh();

            AgentControlService txService = ctx.getBean(AgentControlService.class);
            CountDownLatch start = new CountDownLatch(1);
            ExecutorService pool = Executors.newFixedThreadPool(2);
            try {
                Future<?> f1 = pool.submit(() -> {
                    start.await();
                    txService.rotateCertificate("agent-tenant-a", nextA);
                    return null;
                });
                Future<?> f2 = pool.submit(() -> {
                    start.await();
                    txService.rotateCertificate("agent-tenant-b", nextB);
                    return null;
                });
                start.countDown();
                f1.get(30, TimeUnit.SECONDS);
                f2.get(30, TimeUnit.SECONDS);
            } finally {
                pool.shutdownNow();
            }
        }

        AgentCertificateCredential activeA =
                agentRepo.listActiveCredentialsForAgent(tenantA.tenantId(), "agent-tenant-a").get(0);
        AgentCertificateCredential activeB =
                agentRepo.listActiveCredentialsForAgent(tenantB.tenantId(), "agent-tenant-b").get(0);
        assertEquals(tenantA.tenantId(), activeA.tenantId());
        assertEquals(tenantB.tenantId(), activeB.tenantId());
        assertEquals(CertificateFingerprints.sha256DerHex(nextA), activeA.fingerprintSha256());
        assertEquals(CertificateFingerprints.sha256DerHex(nextB), activeB.fingerprintSha256());
        assertTrue(agentRepo.listActiveCredentialsForAgent(tenantA.tenantId(), "agent-tenant-b").isEmpty());
        assertTrue(agentRepo.listActiveCredentialsForAgent(tenantB.tenantId(), "agent-tenant-a").isEmpty());
    }

    @Test
    void springManagedRotationRollbackKeepsOldCredentialActive() throws Exception {
        Tenant tenant = tenantService.create("Tx Rollback Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        agentControl.enrollAgent("agent-tx", tenant.tenantId(), null);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate oldCert = mockCert(new byte[]{21, 22, 23}, "d001", notBefore, notAfter);
        String oldFp = CertificateFingerprints.sha256DerHex(oldCert);
        agentControl.bindCertificate("agent-tx", oldCert);
        assertEquals(1, agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-tx").size());

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            // Do not destroy the shared PostgresTestSupport DataSource when the context closes.
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class, FailingInsertAgentControlConfig.class);
            ctx.registerBean(SensorControlRepository.class, () -> sensorRepo);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.registerBean(AssignmentMetrics.class, AssignmentMetrics::new);
            ctx.register(AgentControlService.class);
            ctx.refresh();

            AgentControlService txService = ctx.getBean(AgentControlService.class);
            X509Certificate newCert = mockCert(new byte[]{31, 32, 33}, "d002", notBefore, notAfter);
            assertThrows(IllegalStateException.class,
                    () -> txService.rotateCertificate("agent-tx", newCert));

            List<AgentCertificateCredential> activeInsideTxContext =
                    agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-tx");
            assertEquals(1, activeInsideTxContext.size());
            assertEquals(oldFp, activeInsideTxContext.get(0).fingerprintSha256());
            assertEquals(AgentCredentialStatus.ACTIVE, activeInsideTxContext.get(0).status());
        }

        List<AgentCertificateCredential> active =
                agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-tx");
        assertEquals(1, active.size());
        assertEquals(oldFp, active.get(0).fingerprintSha256());
        assertEquals(AgentCredentialStatus.ACTIVE, active.get(0).status());
        assertEquals(AgentCredentialStatus.ACTIVE,
                agentRepo.findCredentialByFingerprint(oldFp).orElseThrow().status());
    }

    @Test
    void rotateAndBindAfterPrincipalRevokeOrDisable_denied() throws Exception {
        Tenant tenant = tenantService.create("Lifecycle Deny Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);

        agentControl.enrollAgent("agent-lc-rev", tenant.tenantId(), null);
        agentControl.bindCertificate("agent-lc-rev",
                mockCert(new byte[]{1, 1, 1}, "f101", notBefore, notAfter));
        agentControl.revokeAgent("agent-lc-rev");
        AgentControlException rotateRevoked = assertThrows(AgentControlException.class,
                () -> agentControl.rotateCertificate("agent-lc-rev",
                        mockCert(new byte[]{1, 1, 2}, "f102", notBefore, notAfter)));
        assertEquals(AgentControlException.ERR_AGENT_STATE_INVALID, rotateRevoked.getCode());
        AgentControlException bindRevoked = assertThrows(AgentControlException.class,
                () -> agentControl.bindCertificate("agent-lc-rev",
                        mockCert(new byte[]{1, 1, 3}, "f103", notBefore, notAfter)));
        assertEquals(AgentControlException.ERR_AGENT_STATE_INVALID, bindRevoked.getCode());
        assertTrue(agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-lc-rev").isEmpty());

        agentControl.enrollAgent("agent-lc-dis", tenant.tenantId(), null);
        agentControl.bindCertificate("agent-lc-dis",
                mockCert(new byte[]{2, 2, 1}, "f111", notBefore, notAfter));
        agentControl.disableAgent("agent-lc-dis");
        AgentControlException rotateDisabled = assertThrows(AgentControlException.class,
                () -> agentControl.rotateCertificate("agent-lc-dis",
                        mockCert(new byte[]{2, 2, 2}, "f112", notBefore, notAfter)));
        assertEquals(AgentControlException.ERR_AGENT_STATE_INVALID, rotateDisabled.getCode());
        AgentControlException bindDisabled = assertThrows(AgentControlException.class,
                () -> agentControl.bindCertificate("agent-lc-dis",
                        mockCert(new byte[]{2, 2, 3}, "f113", notBefore, notAfter)));
        assertEquals(AgentControlException.ERR_AGENT_STATE_INVALID, bindDisabled.getCode());
        assertEquals(1, agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-lc-dis").size());
    }

    @Test
    void deliberateCertificateOverlapRemainsValidForActivePrincipal() throws Exception {
        Tenant tenant = tenantService.create("Overlap Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        agentControl.enrollAgent("agent-overlap", tenant.tenantId(), null);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate certA = mockCert(new byte[]{9, 9, 1}, "f201", notBefore, notAfter);
        X509Certificate certB = mockCert(new byte[]{9, 9, 2}, "f202", notBefore, notAfter);
        agentControl.bindCertificate("agent-overlap", certA);
        agentControl.bindCertificate("agent-overlap", certB);
        List<AgentCertificateCredential> active =
                agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-overlap");
        assertEquals(2, active.size());
        assertEquals(AgentLifecycleStatus.ACTIVE,
                agentRepo.findPrincipalById("agent-overlap").orElseThrow().status());
    }

    @Test
    void revokeRacingWithRotate_cannotLeaveActiveCredentialForRevokedPrincipal() throws Exception {
        Tenant tenant = tenantService.create("Rot Principal Revoke Race", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        agentControl.enrollAgent("agent-race-rev", tenant.tenantId(), null);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        agentControl.bindCertificate("agent-race-rev",
                mockCert(new byte[]{3, 3, 1}, "f301", notBefore, notAfter));
        X509Certificate newCert = mockCert(new byte[]{3, 3, 2}, "f302", notBefore, notAfter);

        CountDownLatch rotateReachedLock = new CountDownLatch(1);
        CountDownLatch allowRotateLock = new CountDownLatch(1);
        AtomicReference<Throwable> rotateFailure = new AtomicReference<>();

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class);
            ctx.registerBean(AgentControlRepository.class,
                    () -> new GatedLockAgentControlRepository(pg.namedJdbc(), rotateReachedLock, allowRotateLock));
            ctx.registerBean(SensorControlRepository.class, () -> sensorRepo);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.registerBean(AssignmentMetrics.class, AssignmentMetrics::new);
            ctx.register(AgentControlService.class);
            ctx.refresh();

            AgentControlService txService = ctx.getBean(AgentControlService.class);
            ExecutorService pool = Executors.newSingleThreadExecutor();
            try {
                Future<?> rotate = pool.submit(() -> {
                    try {
                        txService.rotateCertificate("agent-race-rev", newCert);
                    } catch (Throwable t) {
                        rotateFailure.set(t);
                    }
                    return null;
                });
                assertTrue(rotateReachedLock.await(10, TimeUnit.SECONDS));
                // Principal was read as ACTIVE before lock; revoke commits first.
                agentControl.revokeAgent("agent-race-rev");
                allowRotateLock.countDown();
                rotate.get(30, TimeUnit.SECONDS);
            } finally {
                pool.shutdownNow();
            }
        }

        Throwable failure = rotateFailure.get();
        assertTrue(failure instanceof AgentControlException);
        assertEquals(AgentControlException.ERR_AGENT_STATE_INVALID,
                ((AgentControlException) failure).getCode());
        assertEquals(AgentLifecycleStatus.REVOKED,
                agentRepo.findPrincipalById("agent-race-rev").orElseThrow().status());
        assertTrue(agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-race-rev").isEmpty());
        assertTrue(agentRepo.findCredentialByFingerprint(CertificateFingerprints.sha256DerHex(newCert)).isEmpty());
    }

    @Test
    void disableRacingWithRotate_cannotCreateActiveCredentialAfterDisable() throws Exception {
        Tenant tenant = tenantService.create("Rot Principal Disable Race", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        agentControl.enrollAgent("agent-race-dis", tenant.tenantId(), null);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate oldCert = mockCert(new byte[]{4, 4, 1}, "f401", notBefore, notAfter);
        String oldFp = CertificateFingerprints.sha256DerHex(oldCert);
        agentControl.bindCertificate("agent-race-dis", oldCert);
        X509Certificate newCert = mockCert(new byte[]{4, 4, 2}, "f402", notBefore, notAfter);
        String newFp = CertificateFingerprints.sha256DerHex(newCert);

        CountDownLatch rotateReachedLock = new CountDownLatch(1);
        CountDownLatch allowRotateLock = new CountDownLatch(1);
        AtomicReference<Throwable> rotateFailure = new AtomicReference<>();

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class);
            ctx.registerBean(AgentControlRepository.class,
                    () -> new GatedLockAgentControlRepository(pg.namedJdbc(), rotateReachedLock, allowRotateLock));
            ctx.registerBean(SensorControlRepository.class, () -> sensorRepo);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.registerBean(AssignmentMetrics.class, AssignmentMetrics::new);
            ctx.register(AgentControlService.class);
            ctx.refresh();

            AgentControlService txService = ctx.getBean(AgentControlService.class);
            ExecutorService pool = Executors.newSingleThreadExecutor();
            try {
                Future<?> rotate = pool.submit(() -> {
                    try {
                        txService.rotateCertificate("agent-race-dis", newCert);
                    } catch (Throwable t) {
                        rotateFailure.set(t);
                    }
                    return null;
                });
                assertTrue(rotateReachedLock.await(10, TimeUnit.SECONDS));
                agentControl.disableAgent("agent-race-dis");
                allowRotateLock.countDown();
                rotate.get(30, TimeUnit.SECONDS);
            } finally {
                pool.shutdownNow();
            }
        }

        Throwable failure = rotateFailure.get();
        assertTrue(failure instanceof AgentControlException);
        assertEquals(AgentControlException.ERR_AGENT_STATE_INVALID,
                ((AgentControlException) failure).getCode());
        assertEquals(AgentLifecycleStatus.DISABLED,
                agentRepo.findPrincipalById("agent-race-dis").orElseThrow().status());
        List<AgentCertificateCredential> active =
                agentRepo.listActiveCredentialsForAgent(tenant.tenantId(), "agent-race-dis");
        assertEquals(1, active.size());
        assertEquals(oldFp, active.get(0).fingerprintSha256());
        assertTrue(agentRepo.findCredentialByFingerprint(newFp).isEmpty());
    }

    /**
     * Repository stub that performs real supersede updates then fails insert — proves @Transactional rollback.
     */
    static final class FailingInsertAgentControlRepository extends AgentControlRepository {
        FailingInsertAgentControlRepository(NamedParameterJdbcTemplate jdbc) {
            super(jdbc);
        }

        @Override
        public void insertCredential(AgentCertificateCredential credential) {
            throw new IllegalStateException("forced insert failure for rotation rollback test");
        }
    }

    /**
     * Gates the first principal lock so a concurrent revoke/disable can commit after the
     * pre-lock principal read and before lock acquisition — deterministic race ordering.
     */
    public static class GatedLockAgentControlRepository extends AgentControlRepository {
        private final CountDownLatch reachedLock;
        private final CountDownLatch allowLock;
        private final AtomicInteger gateUses = new AtomicInteger();

        public GatedLockAgentControlRepository(
                NamedParameterJdbcTemplate jdbc,
                CountDownLatch reachedLock,
                CountDownLatch allowLock
        ) {
            super(jdbc);
            this.reachedLock = reachedLock;
            this.allowLock = allowLock;
        }

        @Override
        public AgentPrincipal lockPrincipalForUpdate(UUID tenantId, String agentId) {
            if (gateUses.getAndIncrement() == 0) {
                reachedLock.countDown();
                try {
                    if (!allowLock.await(30, TimeUnit.SECONDS)) {
                        throw new IllegalStateException("timed out waiting for lock gate release");
                    }
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    throw new IllegalStateException("interrupted waiting for lock gate release", e);
                }
            }
            return super.lockPrincipalForUpdate(tenantId, agentId);
        }
    }

    @Configuration
    static class FailingInsertAgentControlConfig {
        @Bean
        @Primary
        AgentControlRepository agentControlRepository(NamedParameterJdbcTemplate jdbc) {
            return new FailingInsertAgentControlRepository(jdbc);
        }
    }
}
