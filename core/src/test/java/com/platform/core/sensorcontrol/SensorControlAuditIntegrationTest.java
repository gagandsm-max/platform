package com.platform.core.sensorcontrol;

import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.core.read.ListAppender;
import com.platform.core.config.PersistenceConfig;
import com.platform.core.domain.inventory.Site;
import com.platform.core.domain.inventory.SiteService;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.SiteRepository;
import com.platform.core.persistence.TenantRepository;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.support.TransactionTemplate;

import javax.sql.DataSource;
import java.math.BigInteger;
import java.security.cert.X509Certificate;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.Date;
import java.util.List;
import java.util.stream.Collectors;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

/**
 * PostgreSQL coverage for Sensor disable vs revoke audit identity and principal/credential mutation.
 * Audit emission is log-based today; this does not invent a transactional-audit guarantee.
 */
@Tag("postgres")
class SensorControlAuditIntegrationTest {

    private static final String PRIVATE_MATERIAL =
            "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n-----END PRIVATE KEY-----";

    private PostgresTestSupport pg;
    private TenantService tenantService;
    private SiteService siteService;
    private SensorControlRepository sensorRepo;
    private SensorControlService sensorControl;
    private Logger auditLogger;
    private ListAppender<ILoggingEvent> auditAppender;

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

        auditLogger = (Logger) LoggerFactory.getLogger(SecurityAuditEmitter.class);
        auditAppender = new ListAppender<>();
        auditAppender.start();
        auditLogger.addAppender(auditAppender);
    }

    @AfterEach
    void tearDown() {
        if (auditLogger != null && auditAppender != null) {
            auditLogger.detachAppender(auditAppender);
            auditAppender.stop();
        }
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void disableAndRevokeEmitDistinctActionsAndMutatePrincipalCredentials() throws Exception {
        Tenant tenant = tenantService.create("Sensor Audit Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Site site = siteService.create(tenant.tenantId(), null, "HQ", "HQ site", "UTC");
        Instant notBefore = Instant.now().minus(1, ChronoUnit.HOURS);
        Instant notAfter = Instant.now().plus(24, ChronoUnit.HOURS);

        sensorControl.enrollSensor("sensor-audit-dis", tenant.tenantId(), site.siteId(), SensorRole.NETWORK, true);
        X509Certificate disableCert = mockCert(new byte[]{1, 2, 3}, "d101", notBefore, notAfter);
        String disableFp = CertificateFingerprints.sha256DerHex(disableCert);
        sensorControl.bindCertificate("sensor-audit-dis", disableCert);

        sensorControl.enrollSensor("sensor-audit-rev", tenant.tenantId(), site.siteId(), SensorRole.NETWORK, true);
        X509Certificate revokeCert = mockCert(new byte[]{4, 5, 6}, "d102", notBefore, notAfter);
        String revokeFp = CertificateFingerprints.sha256DerHex(revokeCert);
        sensorControl.bindCertificate("sensor-audit-rev", revokeCert);

        clearAuditLines();

        sensorControl.disableSensor("sensor-audit-dis");
        sensorControl.revokeSensor("sensor-audit-rev");

        String lines = securityAuditLines();
        assertTrue(lines.contains("action=sensor.disable"), lines);
        assertTrue(lines.contains("action=sensor.revoke"), lines);
        assertTrue(lines.contains("reason=SENSOR_DISABLED"), lines);
        assertTrue(lines.contains("reason=SENSOR_REVOKED"), lines);
        assertTrue(lines.contains("tenant_id=" + tenant.tenantId()), lines);
        assertTrue(lines.contains("sensor_id=sensor-audit-dis"), lines);
        assertTrue(lines.contains("sensor_id=sensor-audit-rev"), lines);
        assertFalse(lines.contains(PRIVATE_MATERIAL), lines);
        assertFalse(lines.contains(disableFp), lines);
        assertFalse(lines.contains(revokeFp), lines);
        assertFalse(lines.toLowerCase().contains("begin private key"), lines);

        assertEquals(SensorLifecycleStatus.DISABLED,
                sensorRepo.findPrincipalById("sensor-audit-dis").orElseThrow().status());
        assertEquals(SensorLifecycleStatus.REVOKED,
                sensorRepo.findPrincipalById("sensor-audit-rev").orElseThrow().status());
        assertEquals(1, sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-audit-dis").size());
        assertTrue(sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-audit-rev").isEmpty());
    }

    @Test
    void missingPrincipalProducesNoSuccessAuditEvent() {
        clearAuditLines();
        SensorControlException ex = assertThrows(SensorControlException.class,
                () -> sensorControl.revokeSensor("sensor-missing-audit"));
        assertEquals(SensorControlException.ERR_SENSOR_NOT_FOUND, ex.getCode());
        String lines = securityAuditLines();
        assertFalse(lines.contains("action=sensor.revoke"), lines);
        assertFalse(lines.contains("reason=SENSOR_REVOKED"), lines);
        assertFalse(lines.contains("action=sensor.disable"), lines);
    }

    @Test
    void springManagedRevokeRollbackPreservesPrincipalAndCredentials() throws Exception {
        Tenant tenant = tenantService.create("Sensor Audit Rollback Corp", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        sensorControl.enrollSensor("sensor-audit-tx", tenant.tenantId(), null, SensorRole.NETWORK, true);
        Instant notBefore = Instant.now().minusSeconds(60);
        Instant notAfter = Instant.now().plusSeconds(3600);
        X509Certificate cert = mockCert(new byte[]{7, 8, 9}, "d201", notBefore, notAfter);
        String fp = CertificateFingerprints.sha256DerHex(cert);
        sensorControl.bindCertificate("sensor-audit-tx", cert);
        assertEquals(1, sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-audit-tx").size());
        assertEquals(SensorLifecycleStatus.ACTIVE,
                sensorRepo.findPrincipalById("sensor-audit-tx").orElseThrow().status());

        clearAuditLines();

        try (AnnotationConfigApplicationContext ctx = new AnnotationConfigApplicationContext()) {
            ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
            ctx.registerBean(NamedParameterJdbcTemplate.class, pg::namedJdbc);
            ctx.register(PersistenceConfig.class);
            // Same explicit repository registration as SensorControlAuthorityIntegrationTest Spring TX cases.
            ctx.registerBean(SensorControlRepository.class, () -> sensorRepo);
            ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
            ctx.register(SensorControlService.class);
            ctx.refresh();

            SensorControlService txService = ctx.getBean(SensorControlService.class);
            TransactionTemplate tx = new TransactionTemplate(
                    ctx.getBean("transactionManager", PlatformTransactionManager.class));

            assertThrows(IllegalStateException.class, () -> tx.executeWithoutResult(status -> {
                txService.revokeSensor("sensor-audit-tx");
                // Force rollback after service mutations + in-tx audit emit (log-only; not DB-backed).
                throw new IllegalStateException("forced rollback after revoke");
            }));
        }

        assertEquals(SensorLifecycleStatus.ACTIVE,
                sensorRepo.findPrincipalById("sensor-audit-tx").orElseThrow().status());
        List<SensorCertificateCredential> active =
                sensorRepo.listActiveCredentialsForSensor(tenant.tenantId(), "sensor-audit-tx");
        assertEquals(1, active.size());
        assertEquals(fp, active.get(0).fingerprintSha256());
        assertEquals(SensorCredentialStatus.ACTIVE, active.get(0).status());
        // Log-based audit may still have been emitted before rollback; do not assert absence as a DB guarantee.
        String lines = securityAuditLines();
        assertFalse(lines.contains(PRIVATE_MATERIAL), lines);
        assertFalse(lines.contains(fp), lines);
    }

    private X509Certificate mockCert(byte[] der, String serialHex, Instant notBefore, Instant notAfter)
            throws Exception {
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

    private void clearAuditLines() {
        auditAppender.list.clear();
    }

    private String securityAuditLines() {
        return auditAppender.list.stream()
                .map(ILoggingEvent::getFormattedMessage)
                .filter(msg -> msg.startsWith("security_audit "))
                .collect(Collectors.joining("\n"));
    }
}
