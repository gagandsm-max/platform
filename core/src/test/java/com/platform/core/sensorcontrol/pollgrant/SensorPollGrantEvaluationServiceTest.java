package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.config.PersistenceConfig;
import com.platform.core.ingestion.authority.ResolvedSensorAuthority;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.sensorcontrol.SensorAuthorityMetrics;
import com.platform.core.sensorcontrol.SensorControlConfiguration;
import com.platform.core.sensorcontrol.SensorControlException;
import com.platform.core.sensorcontrol.SensorControlRepository;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.aop.support.AopUtils;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;
import org.springframework.core.annotation.AnnotationUtils;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Component;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.transaction.support.TransactionSynchronizationManager;
import org.springframework.web.bind.annotation.RestController;

import javax.sql.DataSource;
import java.lang.reflect.Method;
import java.sql.Connection;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicBoolean;

import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrantEligibility.Decision;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

/**
 * Unit coverage for the Issue #229 G1b Spring evaluation seam.
 * No public API, companion wire, scheduler or typed admission wiring.
 */
class SensorPollGrantEvaluationServiceTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000501");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-000000000502");
    private static final UUID GRANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000504");
    private static final UUID CREDENTIAL = UUID.fromString("018f0e0a-0000-7000-8000-000000000505");
    private static final String SENSOR = "sensor-eval-unit";
    private static final String FINGERPRINT =
            "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";

    private AnnotationConfigApplicationContext context;

    @AfterEach
    void closeContext() {
        if (context != null) {
            context.close();
            context = null;
        }
    }

    @Test
    void evaluateMethodDeclaresSingleTransactionManagerBoundaryAllowingShareLocks() throws Exception {
        Method method = SensorPollGrantEvaluationService.class.getMethod(
                "evaluateForNewExecution", ResolvedSensorAuthority.class, UUID.class);
        Transactional transactional = AnnotationUtils.findAnnotation(method, Transactional.class);
        assertNotNull(transactional);
        assertEquals("transactionManager", transactional.transactionManager());
        // Must not be Spring readOnly: PostgreSQL rejects FOR SHARE under READ ONLY.
        assertFalse(transactional.readOnly());
        assertEquals(2, method.getParameterCount(),
                "Seam accepts only ResolvedSensorAuthority + grantId");
        assertFalse(method.getParameterTypes()[0].getName().contains("ExpectedBinding"));
    }

    @Test
    void classIsInternalSeamNotPublicSurface() {
        assertFalse(SensorPollGrantEvaluationService.class.isAnnotationPresent(Service.class));
        assertFalse(SensorPollGrantEvaluationService.class.isAnnotationPresent(Component.class));
        assertFalse(SensorPollGrantEvaluationService.class.isAnnotationPresent(RestController.class));
        assertFalse(SensorPollGrantAccess.class.isAnnotationPresent(Service.class));
        assertFalse(SensorPollGrantAccess.class.isAnnotationPresent(
                org.springframework.transaction.annotation.Transactional.class));
    }

    @Test
    void delegatesWithServerAuthorityAndGrantIdWithoutExpectedBinding() {
        SensorPollGrantAccess access = mock(SensorPollGrantAccess.class);
        when(access.evaluateForNewExecution(any())).thenReturn(
                SensorPollGrantEligibility.denied(Decision.NOT_VISIBLE));
        SensorPollGrantEvaluationService service = new SensorPollGrantEvaluationService(access);
        ResolvedSensorAuthority resolved = authority();

        assertEquals(Decision.NOT_VISIBLE,
                service.evaluateForNewExecution(resolved, GRANT).decision());

        ArgumentCaptor<SensorPollGrantAccess.LookupRequest> captor =
                ArgumentCaptor.forClass(SensorPollGrantAccess.LookupRequest.class);
        verify(access).evaluateForNewExecution(captor.capture());
        SensorPollGrantAccess.LookupRequest request = captor.getValue();
        assertSame(resolved, request.authority());
        assertEquals(GRANT, request.grantId());
        assertNull(request.expected(), "ExpectedBinding must not be supplied by this seam");
    }

    @Test
    void nullAuthorityOrGrantIdFailClosedWithoutTouchingAccess() {
        SensorPollGrantAccess access = mock(SensorPollGrantAccess.class);
        SensorPollGrantEvaluationService service = new SensorPollGrantEvaluationService(access);

        SensorControlException missingAuthority = assertThrows(SensorControlException.class,
                () -> service.evaluateForNewExecution(null, GRANT));
        assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID, missingAuthority.getCode());

        SensorControlException missingGrant = assertThrows(SensorControlException.class,
                () -> service.evaluateForNewExecution(authority(), null));
        assertEquals(SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID, missingGrant.getCode());
        verifyNoInteractions(access);
    }

    @Test
    void springTransactionalProxyOpensReadTransactionBeforeAccess() throws Exception {
        SensorPollGrantAccess access = mock(SensorPollGrantAccess.class);
        AtomicBoolean sawActiveTx = new AtomicBoolean(false);
        AtomicBoolean sawReadOnlyFlag = new AtomicBoolean(true);
        when(access.evaluateForNewExecution(any())).thenAnswer(inv -> {
            sawActiveTx.set(TransactionSynchronizationManager.isActualTransactionActive());
            sawReadOnlyFlag.set(TransactionSynchronizationManager.isCurrentTransactionReadOnly());
            return SensorPollGrantEligibility.denied(Decision.NOT_VISIBLE);
        });

        DataSource dataSource = mock(DataSource.class);
        Connection connection = mock(Connection.class);
        when(dataSource.getConnection()).thenReturn(connection);
        when(connection.getAutoCommit()).thenReturn(true);

        context = new AnnotationConfigApplicationContext();
        context.getDefaultListableBeanFactory().registerSingleton("dataSource", dataSource);
        context.getDefaultListableBeanFactory().registerSingleton("sensorPollGrantAccess", access);
        context.register(PersistenceConfig.class);
        context.registerBean(SensorPollGrantEvaluationService.class);
        context.refresh();

        SensorPollGrantEvaluationService service =
                context.getBean(SensorPollGrantEvaluationService.class);
        assertTrue(AopUtils.isAopProxy(service), "@Transactional proxy must wrap the seam");
        assertFalse(TransactionSynchronizationManager.isActualTransactionActive());

        SensorPollGrantEligibility outcome = service.evaluateForNewExecution(authority(), GRANT);

        assertEquals(Decision.NOT_VISIBLE, outcome.decision());
        assertNull(outcome.grant(), "Denials must not carry a grant snapshot");
        assertTrue(sawActiveTx.get(), "Access must observe the Spring-held transaction");
        assertFalse(sawReadOnlyFlag.get(),
                "TX must allow FOR SHARE (not PostgreSQL READ ONLY)");
        verify(access, times(1)).evaluateForNewExecution(any());
        verify(dataSource, atLeastOnce()).getConnection();
    }

    @Test
    void unproxiedInvocationDoesNotInventALockHoldingTransaction() {
        DataSource dataSource = mock(DataSource.class);
        SensorControlRepository sensors = mock(SensorControlRepository.class);
        SensorPollGrantRepository grants = new SensorPollGrantRepository(
                new NamedParameterJdbcTemplate(dataSource));
        SensorPollGrantAccess access = new SensorPollGrantAccess(sensors, grants);
        SensorPollGrantEvaluationService unproxied = new SensorPollGrantEvaluationService(access);

        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> unproxied.evaluateForNewExecution(authority(), GRANT));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, failure.getCode());
        verifyNoInteractions(sensors);
    }

    @Test
    void sensorControlConfigurationWiresEvaluationBeansWithoutPublicEndpoint() {
        DataSource dataSource = mock(DataSource.class);
        NamedParameterJdbcTemplate jdbc = mock(NamedParameterJdbcTemplate.class);

        context = new AnnotationConfigApplicationContext();
        context.getDefaultListableBeanFactory().registerSingleton("dataSource", dataSource);
        context.getDefaultListableBeanFactory().registerSingleton("namedParameterJdbcTemplate", jdbc);
        context.register(SensorControlRepository.class);
        context.register(SensorAuthorityMetrics.class);
        context.register(SensorControlConfiguration.class);
        context.register(PersistenceConfig.class);
        context.refresh();

        SensorPollGrantEvaluationService service =
                context.getBean(SensorPollGrantEvaluationService.class);
        assertNotNull(context.getBean(SensorPollGrantAccess.class));
        assertNotNull(context.getBean(SensorPollGrantRepository.class));
        assertTrue(AopUtils.isAopProxy(service));
        assertSame(service, context.getBean("sensorPollGrantEvaluationService"));
    }

    private static ResolvedSensorAuthority authority() {
        return new ResolvedSensorAuthority(
                new SensorScope(SENSOR, TENANT, null, Set.of(ENTITY), false),
                CREDENTIAL,
                FINGERPRINT);
    }
}
