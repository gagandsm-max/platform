package com.platform.core.domain.capabilitypack;

import com.platform.core.domain.discovery.DiscoveryMethod;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.CapabilityPackRepository;
import com.platform.core.persistence.DiscoveryRepository;
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
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.common.serialization.StringSerializer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.NoUniqueBeanDefinitionException;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;
import org.springframework.context.annotation.Configuration;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;
import org.springframework.kafka.core.DefaultKafkaProducerFactory;
import org.springframework.kafka.transaction.KafkaTransactionManager;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.annotation.EnableTransactionManagement;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.transaction.support.TransactionTemplate;

import javax.sql.DataSource;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Proves DB-backed Capability Pack / Discovery services bind explicitly to the
 * PostgreSQL {@code transactionManager} bean when Kafka's
 * {@code kafkaTransactionManager} is also present — the production dual-TM
 * ambiguity that broke Sensor desired-state reconciliation in CI.
 */
@Tag("postgres")
class DualTransactionManagerCapabilityPackIntegrationTest {

    private static final String PACK = "fixture.fictional.dualtm";
    private static final String SENSOR_ID = "sensor-dual-tm";

    private PostgresTestSupport pg;
    private AnnotationConfigApplicationContext ctx;
    private Tenant tenant;
    private SensorCapabilityPackAssignmentService assignments;
    private CapabilityPackCatalogService catalog;
    private DiscoveryService discovery;
    private KafkaTransactionManager<String, String> kafkaTransactionManager;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        NamedParameterJdbcTemplate named = pg.namedJdbc();

        DefaultKafkaProducerFactory<String, String> producerFactory = new DefaultKafkaProducerFactory<>(Map.of(
                ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, "127.0.0.1:1",
                ProducerConfig.KEY_SERIALIZER_CLASS_CONFIG, StringSerializer.class,
                ProducerConfig.VALUE_SERIALIZER_CLASS_CONFIG, StringSerializer.class
        ));
        producerFactory.setTransactionIdPrefix("tx-telemetry-test-");
        kafkaTransactionManager = new KafkaTransactionManager<>(producerFactory);
        assertTrue(producerFactory.transactionCapable(), "Kafka producer factory must remain transactional");

        ctx = new AnnotationConfigApplicationContext();
        ctx.registerBean(DataSource.class, pg::dataSource, def -> def.setDestroyMethodName(""));
        ctx.registerBean(NamedParameterJdbcTemplate.class, () -> named);
        ctx.registerBean(
                "transactionManager",
                PlatformTransactionManager.class,
                () -> new DataSourceTransactionManager(pg.dataSource())
        );
        ctx.registerBean(
                "kafkaTransactionManager",
                PlatformTransactionManager.class,
                () -> kafkaTransactionManager
        );
        ctx.registerBean(TenantRepository.class, () -> new TenantRepository(named));
        ctx.registerBean(CapabilityPackRepository.class, () -> new CapabilityPackRepository(named));
        ctx.registerBean(
                SensorCapabilityPackAssignmentRepository.class,
                () -> new SensorCapabilityPackAssignmentRepository(named)
        );
        ctx.registerBean(DiscoveryRepository.class, () -> new DiscoveryRepository(named));
        ctx.registerBean(SensorControlRepository.class, () -> new SensorControlRepository(named));
        ctx.registerBean(SecurityAuditEmitter.class, SecurityAuditEmitter::new);
        ctx.register(
                TxManagementConfig.class,
                AmbiguousTransactionalProbe.class,
                TenantService.class,
                CapabilityPackCatalogService.class,
                SensorCapabilityPackAssignmentService.class,
                SensorControlService.class,
                DiscoveryService.class
        );
        ctx.refresh();

        assertEquals(2, ctx.getBeansOfType(PlatformTransactionManager.class).size());
        assertNotNull(ctx.getBean("transactionManager", PlatformTransactionManager.class));
        assertInstanceOf(
                KafkaTransactionManager.class,
                ctx.getBean("kafkaTransactionManager", PlatformTransactionManager.class)
        );
        assertTrue(
                ctx.getBean("kafkaTransactionManager", KafkaTransactionManager.class)
                        .getProducerFactory()
                        .transactionCapable(),
                "kafkaTransactionManager must remain operational/transactional"
        );

        authenticateAs("dual-tm-actor");

        // TenantService.create still uses unqualified @Transactional; create via explicit JDBC TM.
        TransactionTemplate pgTx = new TransactionTemplate(
                ctx.getBean("transactionManager", PlatformTransactionManager.class));
        TenantRepository tenantRepo = ctx.getBean(TenantRepository.class);
        tenant = pgTx.execute(status -> {
            TenantService bare = new TenantService(tenantRepo);
            return bare.create("Dual TM Tenant", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        });

        SensorControlService sensors = ctx.getBean(SensorControlService.class);
        sensors.enrollSensor(SENSOR_ID, tenant.tenantId(), null, SensorRole.NETWORK, true);

        catalog = ctx.getBean(CapabilityPackCatalogService.class);
        assignments = ctx.getBean(SensorCapabilityPackAssignmentService.class);
        discovery = ctx.getBean(DiscoveryService.class);
    }

    @AfterEach
    void tearDown() {
        SecurityContextHolder.clearContext();
        if (ctx != null) {
            ctx.close();
        }
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void qualifiedServicesExecuteWithBothTransactionManagersPresent() {
        // Negative control: unqualified @Transactional remains ambiguous (no @Primary concealment).
        AmbiguousTransactionalProbe probe = ctx.getBean(AmbiguousTransactionalProbe.class);
        Throwable ambiguous = assertThrows(Throwable.class, probe::touch);
        assertTrue(
                containsNoUniqueBeanDefinition(ambiguous),
                "unqualified @Transactional must still fail with dual TMs; got: " + ambiguous
        );

        byte[] manifest = CapabilityPackTestSupport.minimalManifestBytes(PACK, "1.0.0", "DualTM");
        assertDoesNotThrow(() -> catalog.register(tenant.tenantId(), PACK, "1.0.0", manifest));
        assertDoesNotThrow(() -> catalog.activate(tenant.tenantId(), PACK, "1.0.0", 0L));

        var desiredBefore = assertDoesNotThrow(
                () -> assignments.getDesiredState(tenant.tenantId(), SENSOR_ID));
        assertEquals(0L, desiredBefore.generation());

        var assigned = assertDoesNotThrow(
                () -> assignments.assign(tenant.tenantId(), SENSOR_ID, PACK, "1.0.0", 0L));
        assertEquals(1L, assigned.desiredGeneration());

        var desired = assertDoesNotThrow(
                () -> assignments.getDesiredState(tenant.tenantId(), SENSOR_ID));
        assertEquals(1L, desired.generation());
        assertEquals(1, desired.packs().size());

        assertDoesNotThrow(() -> discovery.createDiscovery(
                tenant.tenantId(),
                "dual-tm-discovery",
                List.of("10.255.0.1"),
                List.of(DiscoveryMethod.ICMP),
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                "corr-dual-tm",
                "dual-tm-actor"
        ));
        assertDoesNotThrow(() -> discovery.fetchTaskForSensor(SENSOR_ID));

        // Kafka TM still registered and transactional after DB paths succeed.
        assertInstanceOf(
                KafkaTransactionManager.class,
                ctx.getBean("kafkaTransactionManager", PlatformTransactionManager.class)
        );
        assertTrue(
                ctx.getBean("kafkaTransactionManager", KafkaTransactionManager.class)
                        .getProducerFactory()
                        .transactionCapable()
        );
    }

    private static boolean containsNoUniqueBeanDefinition(Throwable error) {
        for (Throwable cur = error; cur != null; cur = cur.getCause()) {
            if (cur instanceof NoUniqueBeanDefinitionException) {
                return true;
            }
            if (cur.getMessage() != null && cur.getMessage().contains("NoUniqueBeanDefinitionException")) {
                return true;
            }
            if (cur.getMessage() != null
                    && cur.getMessage().contains("expected single matching bean but found 2")) {
                return true;
            }
        }
        return false;
    }

    private static void authenticateAs(String subjectId) {
        Instant now = Instant.parse("2026-06-01T00:00:00Z");
        PrincipalContext principal = new PrincipalContext(
                subjectId,
                AuthenticationState.AUTHENTICATED,
                Set.of(),
                Set.of()
        );
        UUID subjectUuid = UUID.fromString("018f0e0a-0000-7000-8000-00000000d001");
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

    @Configuration
    @EnableTransactionManagement
    static class TxManagementConfig {
    }

    @Service
    static class AmbiguousTransactionalProbe {
        @Transactional
        public void touch() {
            // Intentionally unqualified — must remain ambiguous with dual TMs.
        }
    }
}
