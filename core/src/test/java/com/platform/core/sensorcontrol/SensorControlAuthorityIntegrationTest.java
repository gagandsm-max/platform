package com.platform.core.sensorcontrol;

import com.platform.core.config.PersistenceConfig;
import com.platform.core.domain.discovery.DiscoveryMethod;
import com.platform.core.domain.discovery.DiscoveryRequest;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.discovery.DiscoveryStatus;
import com.platform.core.domain.inventory.Site;
import com.platform.core.domain.inventory.SiteService;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.persistence.DiscoveryRepository;
import com.platform.core.persistence.SiteRepository;
import com.platform.core.persistence.TenantRepository;
import com.platform.core.security.http.SecurityAuditEmitter;
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
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

@Tag("postgres")
class SensorControlAuthorityIntegrationTest {

    private PostgresTestSupport pg;
    private TenantService tenantService;
    private SiteService siteService;
    private SensorControlRepository sensorRepo;
    private SensorControlService sensorControl;
    private PostgresSensorAuthorityResolver resolver;
    private DiscoveryService discoveryService;
    private DiscoveryPlacementService placement;

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
        SensorAuthorityMetrics metrics = new SensorAuthorityMetrics();
        resolver = new PostgresSensorAuthorityResolver(sensorRepo, metrics);
        DiscoveryRepository discoveryRepo = new DiscoveryRepository(named);
        discoveryService = new DiscoveryService(discoveryRepo, tenantService);
        placement = new DiscoveryPlacementService(
                sensorRepo, discoveryService, audit, new DiscoveryPlacementMetrics());
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    private X509Certificate mockCert(byte[] der, String serialHex, Instant notBefore, Instant notAfter) throws Exception {
        X509Certificate cert = mock(X509Certificate.class);
        java.security.PublicKey publicKey = mock(java.security.PublicKey.class);
        when(publicKey.getEncoded()).thenReturn(("spki-" + serialHex).getBytes());
        when(cert.getEncoded()).thenReturn(der);
        when(cert.getSerialNumber()).thenReturn(new BigInteger(serialHex, 16));
        when(cert.getIssuerX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal("CN=Platform Test CA"));
        when(cert.getNotBefore()).thenReturn(Date.from(notBefore));
        when(cert.getNotAfter()).thenReturn(Date.from(notAfter));
        when(cert.getPublicKey()).thenReturn(publicKey);
        return cert;
    }

    @Test
    void enrollBindResolveRotateAndNegativeMatrix() throws Exception {
        Tenant tenant = tenantService.create("Sensor Control Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Site site = siteService.create(tenant.tenantId(), null, "HQ", "HQ site", "UTC");

        sensorControl.enrollSensor("sensor-net-01", tenant.tenantId(), site.siteId(), SensorRole.NETWORK, true);

        Instant notBefore = Instant.now().minus(1, ChronoUnit.HOURS);
        Instant notAfter = Instant.now().plus(24, ChronoUnit.HOURS);
        X509Certificate cert1 = mockCert(new byte[]{10, 20, 30}, "aa01", notBefore, notAfter);
        String fp1 = CertificateFingerprints.sha256DerHex(cert1);
        sensorControl.bindCertificate("sensor-net-01", cert1);

        SensorScope scope = resolver.resolve(cert1);
        assertEquals("sensor-net-01", scope.sensorId());
        assertEquals(tenant.tenantId(), scope.tenantId());
        assertEquals(site.siteId(), scope.siteId());
        assertTrue(scope.allowAllTenantEntities());

        // Unknown fingerprint
        X509Certificate unknown = mockCert(new byte[]{99}, "dead", notBefore, notAfter);
        IngestionException unknownEx = assertThrows(IngestionException.class, () -> resolver.resolve(unknown));
        assertEquals("ERR_SENSOR_CREDENTIAL_UNKNOWN", unknownEx.getCode());

        // Duplicate fingerprint bind fails
        assertThrows(SensorControlException.class,
                () -> sensorControl.bindCertificate("sensor-net-01", cert1));

        // Rotate supersedes old and keeps tenant/site
        X509Certificate cert2 = mockCert(new byte[]{40, 50, 60}, "bb02", notBefore, notAfter);
        String fp2 = CertificateFingerprints.sha256DerHex(cert2);
        assertNotEquals(fp1, fp2);
        sensorControl.rotateCertificate("sensor-net-01", cert2);

        IngestionException superseded = assertThrows(IngestionException.class, () -> resolver.resolve(cert1));
        assertEquals("ERR_SENSOR_CREDENTIAL_REVOKED", superseded.getCode());

        SensorScope afterRotate = resolver.resolve(cert2);
        assertEquals("sensor-net-01", afterRotate.sensorId());
        assertEquals(tenant.tenantId(), afterRotate.tenantId());
        assertEquals(site.siteId(), afterRotate.siteId());

        SensorPrincipal principal = sensorRepo.findPrincipalById("sensor-net-01").orElseThrow();
        assertEquals(tenant.tenantId(), principal.tenantId());
        assertEquals(site.siteId(), principal.siteId());

        // Disable fails closed
        sensorControl.disableSensor("sensor-net-01");
        IngestionException disabled = assertThrows(IngestionException.class, () -> resolver.resolve(cert2));
        assertEquals("ERR_SENSOR_DISABLED", disabled.getCode());
    }

    @Test
    void revokeCredentialFailsClosed() throws Exception {
        Tenant tenant = tenantService.create("Revoke Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-net-02", tenant.tenantId(), null, SensorRole.COMBINED, true);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate cert = mockCert(new byte[]{7, 8, 9}, "cc03", notBefore, notAfter);
        sensorControl.bindCertificate("sensor-net-02", cert);
        String fp = CertificateFingerprints.sha256DerHex(cert);
        sensorControl.revokeCredential(fp);
        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_CREDENTIAL_REVOKED", ex.getCode());
    }

    @Test
    void revokeSensorFailsClosed() throws Exception {
        Tenant tenant = tenantService.create("Revoke Sensor Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-revoked", tenant.tenantId(), null, SensorRole.NETWORK, true);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate cert = mockCert(new byte[]{11, 12, 13}, "dd04", notBefore, notAfter);
        sensorControl.bindCertificate("sensor-revoked", cert);

        sensorControl.revokeSensor("sensor-revoked");

        SensorPrincipal principal = sensorRepo.findPrincipalById("sensor-revoked").orElseThrow();
        assertEquals(SensorLifecycleStatus.REVOKED, principal.status());

        // revokeSensor also revokes active credentials; resolver fails closed on credential status first.
        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_CREDENTIAL_REVOKED", ex.getCode());
    }

    @Test
    void duplicateFingerprintCannotBindTwoSensors() throws Exception {
        Tenant tenant = tenantService.create("Dup Fingerprint Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-alpha", tenant.tenantId(), null, SensorRole.NETWORK, true);
        sensorControl.enrollSensor("sensor-beta", tenant.tenantId(), null, SensorRole.NETWORK, true);

        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate shared = mockCert(new byte[]{21, 22, 23}, "ee05", notBefore, notAfter);
        sensorControl.bindCertificate("sensor-alpha", shared);

        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> sensorControl.bindCertificate("sensor-beta", shared));
        assertEquals(SensorControlException.ERR_SENSOR_CREDENTIAL_DUPLICATE, ex.getCode());

        SensorScope scope = resolver.resolve(shared);
        assertEquals("sensor-alpha", scope.sensorId());
    }

    @Test
    void siteTenantMismatchRejectedOnEnroll() {
        Tenant tenantA = tenantService.create("Site Tenant A", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant tenantB = tenantService.create("Site Tenant B", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Site siteB = siteService.create(tenantB.tenantId(), null, "B-HQ", null, "UTC");

        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> sensorControl.enrollSensor(
                        "sensor-cross-site", tenantA.tenantId(), siteB.siteId(), SensorRole.NETWORK, true));
        assertEquals(SensorControlException.ERR_SENSOR_SITE_INVALID, ex.getCode());
    }

    @Test
    void discoveryPlacementSelectsDiscoveryCapableSensorDeterministically() {
        Tenant tenant = tenantService.create("Placement Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Site site = siteService.create(tenant.tenantId(), null, "Edge", null, "UTC");

        sensorControl.enrollSensor("sensor-infra", tenant.tenantId(), site.siteId(), SensorRole.INFRASTRUCTURE, true);
        sensorControl.enrollSensor("sensor-z", tenant.tenantId(), site.siteId(), SensorRole.NETWORK, true);
        sensorControl.enrollSensor("sensor-a", tenant.tenantId(), site.siteId(), SensorRole.NETWORK, true);

        DiscoveryRequest created = discoveryService.createDiscovery(
                tenant.tenantId(),
                "edge-scan",
                List.of("10.0.0.1"),
                List.of(DiscoveryMethod.ICMP),
                null,
                site.siteId(),
                null,
                161,
                1000,
                1,
                "idem-placement-1",
                null,
                "tester"
        );
        assertEquals(DiscoveryStatus.PENDING, created.status());

        DiscoveryRequest assigned = placement.placePendingRequest(tenant.tenantId(), created.requestId());
        assertEquals("sensor-a", assigned.assignedSensorId());
        assertEquals(DiscoveryStatus.ASSIGNED, assigned.status());
    }

    @Test
    void discoveryPlacementFailsClosedWhenNoEligibleSensor() {
        Tenant tenant = tenantService.create("No Sensor Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-infra-only", tenant.tenantId(), null, SensorRole.INFRASTRUCTURE, true);

        DiscoveryRequest created = discoveryService.createDiscovery(
                tenant.tenantId(),
                "scan",
                List.of("10.0.0.2"),
                List.of(DiscoveryMethod.ICMP),
                null,
                null,
                null,
                161,
                1000,
                1,
                "idem-placement-2",
                null,
                "tester"
        );

        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> placement.placePendingRequest(tenant.tenantId(), created.requestId()));
        assertEquals(SensorControlException.ERR_NO_ELIGIBLE_SENSOR, ex.getCode());

        DiscoveryRequest stillPending = discoveryService.getRequest(tenant.tenantId(), created.requestId());
        assertEquals(DiscoveryStatus.PENDING, stillPending.status());
        assertEquals(null, stillPending.assignedSensorId());
    }

    @Test
    void expiredCredentialFailsClosed() throws Exception {
        Tenant tenant = tenantService.create("Expiry Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-exp", tenant.tenantId(), null, SensorRole.NETWORK, true);
        Instant notBefore = Instant.now().minus(48, ChronoUnit.HOURS);
        Instant notAfter = Instant.now().minus(1, ChronoUnit.HOURS);
        String fp = CertificateFingerprints.sha256Hex(new byte[]{1, 1, 1, 1});
        sensorControl.bindCertificateFromMetadata(
                "sensor-exp", fp, null, "ee01", "CN=CA", null, notBefore, notAfter);

        X509Certificate cert = mockCert(new byte[]{1, 1, 1, 1}, "ee01", notBefore, notAfter);
        IngestionException ex = assertThrows(IngestionException.class, () -> resolver.resolve(cert));
        assertEquals("ERR_SENSOR_CREDENTIAL_EXPIRED", ex.getCode());
    }

    @Test
    void concurrentRotateSamePrincipal_serializesToSingleActive() throws Exception {
        Tenant tenant = tenantService.create("Sensor Rot Concurrency Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-rot-c", tenant.tenantId(), null, SensorRole.NETWORK, true);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate oldCert = mockCert(new byte[]{61, 62, 63}, "e101", notBefore, notAfter);
        String oldFp = CertificateFingerprints.sha256DerHex(oldCert);
        sensorControl.bindCertificate("sensor-rot-c", oldCert);

        X509Certificate certA = mockCert(new byte[]{64, 65, 66}, "e102", notBefore, notAfter);
        X509Certificate certB = mockCert(new byte[]{67, 68, 69}, "e103", notBefore, notAfter);
        String fpA = CertificateFingerprints.sha256DerHex(certA);
        String fpB = CertificateFingerprints.sha256DerHex(certB);

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class);
            ctx.registerBean(SensorControlRepository.class, () -> sensorRepo);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.register(SensorControlService.class);
            ctx.refresh();

            SensorControlService txService = ctx.getBean(SensorControlService.class);
            CountDownLatch start = new CountDownLatch(1);
            AtomicInteger successes = new AtomicInteger();
            ExecutorService pool = Executors.newFixedThreadPool(2);
            try {
                Future<?> f1 = pool.submit(() -> {
                    start.await();
                    txService.rotateCertificate("sensor-rot-c", certA);
                    successes.incrementAndGet();
                    return null;
                });
                Future<?> f2 = pool.submit(() -> {
                    start.await();
                    txService.rotateCertificate("sensor-rot-c", certB);
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

        List<SensorCertificateCredential> active =
                sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-rot-c");
        assertEquals(1, active.size());
        String winnerFp = active.get(0).fingerprintSha256();
        assertTrue(winnerFp.equals(fpA) || winnerFp.equals(fpB));
        assertEquals(SensorCredentialStatus.SUPERSEDED,
                sensorRepo.findCredentialByFingerprint(oldFp).orElseThrow().status());
        String loserFp = winnerFp.equals(fpA) ? fpB : fpA;
        assertEquals(SensorCredentialStatus.SUPERSEDED,
                sensorRepo.findCredentialByFingerprint(loserFp).orElseThrow().status());
    }

    @Test
    void concurrentRotateVersusRevoke_oldFingerprintNotActive() throws Exception {
        Tenant tenant = tenantService.create("Sensor Rot Revoke Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-rot-rev", tenant.tenantId(), null, SensorRole.NETWORK, true);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate oldCert = mockCert(new byte[]{71, 72, 73}, "f101", notBefore, notAfter);
        String oldFp = CertificateFingerprints.sha256DerHex(oldCert);
        sensorControl.bindCertificate("sensor-rot-rev", oldCert);
        X509Certificate newCert = mockCert(new byte[]{74, 75, 76}, "f102", notBefore, notAfter);
        String newFp = CertificateFingerprints.sha256DerHex(newCert);

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class);
            ctx.registerBean(SensorControlRepository.class, () -> sensorRepo);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.register(SensorControlService.class);
            ctx.refresh();

            SensorControlService txService = ctx.getBean(SensorControlService.class);
            CountDownLatch start = new CountDownLatch(1);
            ExecutorService pool = Executors.newFixedThreadPool(2);
            try {
                Future<?> rotate = pool.submit(() -> {
                    start.await();
                    txService.rotateCertificate("sensor-rot-rev", newCert);
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

        assertNotEquals(SensorCredentialStatus.ACTIVE,
                sensorRepo.findCredentialByFingerprint(oldFp).orElseThrow().status());
        List<SensorCertificateCredential> active =
                sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-rot-rev");
        assertEquals(1, active.size());
        assertEquals(newFp, active.get(0).fingerprintSha256());
    }

    @Test
    void concurrentRotateIndependentPrincipals_remainIndependentlyOperable() throws Exception {
        Tenant tenant = tenantService.create("Sensor Rot Indep Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-indep-a", tenant.tenantId(), null, SensorRole.NETWORK, true);
        sensorControl.enrollSensor("sensor-indep-b", tenant.tenantId(), null, SensorRole.NETWORK, true);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        sensorControl.bindCertificate("sensor-indep-a",
                mockCert(new byte[]{81, 82, 83}, "a201", notBefore, notAfter));
        sensorControl.bindCertificate("sensor-indep-b",
                mockCert(new byte[]{84, 85, 86}, "a301", notBefore, notAfter));
        X509Certificate nextA = mockCert(new byte[]{87, 88, 89}, "a202", notBefore, notAfter);
        X509Certificate nextB = mockCert(new byte[]{90, 91, 92}, "a302", notBefore, notAfter);
        String fpA = CertificateFingerprints.sha256DerHex(nextA);
        String fpB = CertificateFingerprints.sha256DerHex(nextB);

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class);
            ctx.registerBean(SensorControlRepository.class, () -> sensorRepo);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.register(SensorControlService.class);
            ctx.refresh();

            SensorControlService txService = ctx.getBean(SensorControlService.class);
            CountDownLatch start = new CountDownLatch(1);
            ExecutorService pool = Executors.newFixedThreadPool(2);
            try {
                Future<?> f1 = pool.submit(() -> {
                    start.await();
                    txService.rotateCertificate("sensor-indep-a", nextA);
                    return null;
                });
                Future<?> f2 = pool.submit(() -> {
                    start.await();
                    txService.rotateCertificate("sensor-indep-b", nextB);
                    return null;
                });
                start.countDown();
                f1.get(30, TimeUnit.SECONDS);
                f2.get(30, TimeUnit.SECONDS);
            } finally {
                pool.shutdownNow();
            }
        }

        assertEquals(fpA, sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-indep-a")
                .get(0).fingerprintSha256());
        assertEquals(fpB, sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-indep-b")
                .get(0).fingerprintSha256());
        assertEquals(1, sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-indep-a").size());
        assertEquals(1, sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-indep-b").size());
    }

    @Test
    void springManagedRotationRollbackKeepsOldCredentialActive() throws Exception {
        Tenant tenant = tenantService.create("Sensor Tx Rollback Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-tx", tenant.tenantId(), null, SensorRole.NETWORK, true);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate oldCert = mockCert(new byte[]{93, 94, 95}, "b301", notBefore, notAfter);
        String oldFp = CertificateFingerprints.sha256DerHex(oldCert);
        sensorControl.bindCertificate("sensor-tx", oldCert);
        assertEquals(1, sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-tx").size());

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class, FailingInsertSensorControlConfig.class);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.register(SensorControlService.class);
            ctx.refresh();

            SensorControlService txService = ctx.getBean(SensorControlService.class);
            X509Certificate newCert = mockCert(new byte[]{96, 97, 98}, "b302", notBefore, notAfter);
            assertThrows(IllegalStateException.class,
                    () -> txService.rotateCertificate("sensor-tx", newCert));

            List<SensorCertificateCredential> activeInside =
                    sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-tx");
            assertEquals(1, activeInside.size());
            assertEquals(oldFp, activeInside.get(0).fingerprintSha256());
            assertEquals(SensorCredentialStatus.ACTIVE, activeInside.get(0).status());
        }

        List<SensorCertificateCredential> active =
                sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-tx");
        assertEquals(1, active.size());
        assertEquals(oldFp, active.get(0).fingerprintSha256());
        assertEquals(SensorCredentialStatus.ACTIVE, active.get(0).status());
    }

    @Test
    void rotateAndBindAfterPrincipalRevokeOrDisable_denied() throws Exception {
        Tenant tenant = tenantService.create("Sensor Lifecycle Deny Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);

        sensorControl.enrollSensor("sensor-lc-rev", tenant.tenantId(), null, SensorRole.NETWORK, true);
        sensorControl.bindCertificate("sensor-lc-rev",
                mockCert(new byte[]{11, 11, 1}, "a101", notBefore, notAfter));
        sensorControl.revokeSensor("sensor-lc-rev");
        SensorControlException rotateRevoked = assertThrows(SensorControlException.class,
                () -> sensorControl.rotateCertificate("sensor-lc-rev",
                        mockCert(new byte[]{11, 11, 2}, "a102", notBefore, notAfter)));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, rotateRevoked.getCode());
        SensorControlException bindRevoked = assertThrows(SensorControlException.class,
                () -> sensorControl.bindCertificate("sensor-lc-rev",
                        mockCert(new byte[]{11, 11, 3}, "a103", notBefore, notAfter)));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, bindRevoked.getCode());
        String metaFp = CertificateFingerprints.sha256Hex(new byte[]{11, 11, 4});
        SensorControlException metaRevoked = assertThrows(SensorControlException.class,
                () -> sensorControl.bindCertificateFromMetadata(
                        "sensor-lc-rev", metaFp, null, "a104", "CN=CA", null, notBefore, notAfter));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, metaRevoked.getCode());
        assertTrue(sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-lc-rev").isEmpty());

        sensorControl.enrollSensor("sensor-lc-dis", tenant.tenantId(), null, SensorRole.NETWORK, true);
        sensorControl.bindCertificate("sensor-lc-dis",
                mockCert(new byte[]{12, 12, 1}, "a111", notBefore, notAfter));
        sensorControl.disableSensor("sensor-lc-dis");
        SensorControlException rotateDisabled = assertThrows(SensorControlException.class,
                () -> sensorControl.rotateCertificate("sensor-lc-dis",
                        mockCert(new byte[]{12, 12, 2}, "a112", notBefore, notAfter)));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, rotateDisabled.getCode());
        SensorControlException bindDisabled = assertThrows(SensorControlException.class,
                () -> sensorControl.bindCertificate("sensor-lc-dis",
                        mockCert(new byte[]{12, 12, 3}, "a113", notBefore, notAfter)));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, bindDisabled.getCode());
        String metaDisFp = CertificateFingerprints.sha256Hex(new byte[]{12, 12, 4});
        SensorControlException metaDisabled = assertThrows(SensorControlException.class,
                () -> sensorControl.bindCertificateFromMetadata(
                        "sensor-lc-dis", metaDisFp, null, "a114", "CN=CA", null, notBefore, notAfter));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, metaDisabled.getCode());
        assertEquals(1, sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-lc-dis").size());
    }

    @Test
    void deliberateCertificateOverlapRemainsValidForActivePrincipal() throws Exception {
        Tenant tenant = tenantService.create("Sensor Overlap Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-overlap", tenant.tenantId(), null, SensorRole.NETWORK, true);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        sensorControl.bindCertificate("sensor-overlap",
                mockCert(new byte[]{13, 13, 1}, "a201", notBefore, notAfter));
        sensorControl.bindCertificate("sensor-overlap",
                mockCert(new byte[]{13, 13, 2}, "a202", notBefore, notAfter));
        assertEquals(2, sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-overlap").size());
        assertEquals(SensorLifecycleStatus.ACTIVE,
                sensorRepo.findPrincipalById("sensor-overlap").orElseThrow().status());
    }

    @Test
    void revokeRacingWithRotate_cannotLeaveActiveCredentialForRevokedPrincipal() throws Exception {
        Tenant tenant = tenantService.create("Sensor Rot Principal Revoke Race", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-race-rev", tenant.tenantId(), null, SensorRole.NETWORK, true);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        sensorControl.bindCertificate("sensor-race-rev",
                mockCert(new byte[]{14, 14, 1}, "a301", notBefore, notAfter));
        X509Certificate newCert = mockCert(new byte[]{14, 14, 2}, "a302", notBefore, notAfter);

        CountDownLatch rotateReachedLock = new CountDownLatch(1);
        CountDownLatch allowRotateLock = new CountDownLatch(1);
        AtomicReference<Throwable> rotateFailure = new AtomicReference<>();

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class);
            ctx.registerBean(SensorControlRepository.class,
                    () -> new GatedLockSensorControlRepository(pg.namedJdbc(), rotateReachedLock, allowRotateLock));
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.register(SensorControlService.class);
            ctx.refresh();

            SensorControlService txService = ctx.getBean(SensorControlService.class);
            ExecutorService pool = Executors.newSingleThreadExecutor();
            try {
                Future<?> rotate = pool.submit(() -> {
                    try {
                        txService.rotateCertificate("sensor-race-rev", newCert);
                    } catch (Throwable t) {
                        rotateFailure.set(t);
                    }
                    return null;
                });
                assertTrue(rotateReachedLock.await(10, TimeUnit.SECONDS));
                sensorControl.revokeSensor("sensor-race-rev");
                allowRotateLock.countDown();
                rotate.get(30, TimeUnit.SECONDS);
            } finally {
                pool.shutdownNow();
            }
        }

        Throwable failure = rotateFailure.get();
        assertTrue(failure instanceof SensorControlException);
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID,
                ((SensorControlException) failure).getCode());
        assertEquals(SensorLifecycleStatus.REVOKED,
                sensorRepo.findPrincipalById("sensor-race-rev").orElseThrow().status());
        assertTrue(sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-race-rev").isEmpty());
        assertTrue(sensorRepo.findCredentialByFingerprint(CertificateFingerprints.sha256DerHex(newCert)).isEmpty());
    }

    @Test
    void disableRacingWithRotate_cannotCreateActiveCredentialAfterDisable() throws Exception {
        Tenant tenant = tenantService.create("Sensor Rot Principal Disable Race", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-race-dis", tenant.tenantId(), null, SensorRole.NETWORK, true);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate oldCert = mockCert(new byte[]{15, 15, 1}, "a401", notBefore, notAfter);
        String oldFp = CertificateFingerprints.sha256DerHex(oldCert);
        sensorControl.bindCertificate("sensor-race-dis", oldCert);
        X509Certificate newCert = mockCert(new byte[]{15, 15, 2}, "a402", notBefore, notAfter);
        String newFp = CertificateFingerprints.sha256DerHex(newCert);

        CountDownLatch rotateReachedLock = new CountDownLatch(1);
        CountDownLatch allowRotateLock = new CountDownLatch(1);
        AtomicReference<Throwable> rotateFailure = new AtomicReference<>();

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class);
            ctx.registerBean(SensorControlRepository.class,
                    () -> new GatedLockSensorControlRepository(pg.namedJdbc(), rotateReachedLock, allowRotateLock));
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.register(SensorControlService.class);
            ctx.refresh();

            SensorControlService txService = ctx.getBean(SensorControlService.class);
            ExecutorService pool = Executors.newSingleThreadExecutor();
            try {
                Future<?> rotate = pool.submit(() -> {
                    try {
                        txService.rotateCertificate("sensor-race-dis", newCert);
                    } catch (Throwable t) {
                        rotateFailure.set(t);
                    }
                    return null;
                });
                assertTrue(rotateReachedLock.await(10, TimeUnit.SECONDS));
                sensorControl.disableSensor("sensor-race-dis");
                allowRotateLock.countDown();
                rotate.get(30, TimeUnit.SECONDS);
            } finally {
                pool.shutdownNow();
            }
        }

        Throwable failure = rotateFailure.get();
        assertTrue(failure instanceof SensorControlException);
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID,
                ((SensorControlException) failure).getCode());
        assertEquals(SensorLifecycleStatus.DISABLED,
                sensorRepo.findPrincipalById("sensor-race-dis").orElseThrow().status());
        List<SensorCertificateCredential> active =
                sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-race-dis");
        assertEquals(1, active.size());
        assertEquals(oldFp, active.get(0).fingerprintSha256());
        assertTrue(sensorRepo.findCredentialByFingerprint(newFp).isEmpty());
    }

    public static class FailingInsertSensorControlRepository extends SensorControlRepository {
        public FailingInsertSensorControlRepository(NamedParameterJdbcTemplate jdbc) {
            super(jdbc);
        }

        @Override
        public void insertCredential(SensorCertificateCredential credential) {
            throw new IllegalStateException("forced insert failure for rotation rollback test");
        }
    }

    public static class GatedLockSensorControlRepository extends SensorControlRepository {
        private final CountDownLatch reachedLock;
        private final CountDownLatch allowLock;
        private final AtomicInteger gateUses = new AtomicInteger();

        public GatedLockSensorControlRepository(
                NamedParameterJdbcTemplate jdbc,
                CountDownLatch reachedLock,
                CountDownLatch allowLock
        ) {
            super(jdbc);
            this.reachedLock = reachedLock;
            this.allowLock = allowLock;
        }

        @Override
        public SensorPrincipal lockPrincipalForUpdate(UUID tenantId, String sensorId) {
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
            return super.lockPrincipalForUpdate(tenantId, sensorId);
        }
    }

    @Configuration
    public static class FailingInsertSensorControlConfig {
        @Bean
        @Primary
        public SensorControlRepository sensorControlRepository(NamedParameterJdbcTemplate jdbc) {
            return new FailingInsertSensorControlRepository(jdbc);
        }
    }
}
