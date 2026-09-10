package com.platform.core.sensorcontrol;

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
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

import java.math.BigInteger;
import java.security.cert.X509Certificate;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.Date;
import java.util.List;

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
}
