package com.platform.core.security.http;

import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.core.read.ListAppender;
import com.platform.core.logging.CorrelationIdFilter;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;
import java.util.stream.Collectors;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class SecurityAuditEmitterTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final String CORRELATION = "corr-enrollment-audit-01";

    private SecurityAuditEmitter emitter;
    private Logger logger;
    private ListAppender<ILoggingEvent> appender;

    @BeforeEach
    void setUp() {
        emitter = new SecurityAuditEmitter();
        logger = (Logger) LoggerFactory.getLogger(SecurityAuditEmitter.class);
        appender = new ListAppender<>();
        appender.start();
        logger.addAppender(appender);
        MDC.put(CorrelationIdFilter.MDC_CORRELATION_ID_KEY, CORRELATION);
    }

    @AfterEach
    void tearDown() {
        MDC.remove(CorrelationIdFilter.MDC_CORRELATION_ID_KEY);
        if (logger != null && appender != null) {
            logger.detachAppender(appender);
            appender.stop();
        }
    }

    @Test
    void emitControlEventIncludesValidatedIdentityAndMetadata() {
        Map<String, String> metadata = new LinkedHashMap<>();
        metadata.put("authority_id", "018f0e0a-0000-7000-8000-0000000000e1");
        metadata.put("deployment_id", "deploy-primary");
        metadata.put("role", "NETWORK");

        emitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.SENSOR_ENROLLMENT_AUTHORITY_ISSUE,
                AuditOutcome.SUCCESS,
                TENANT,
                "SENSOR_ENROLLMENT_AUTHORITY_ISSUED",
                metadata
        );

        String line = securityAuditLines();
        assertTrue(line.contains("action=sensor.enrollment_authority.issue"), line);
        assertTrue(line.contains("tenant_id=" + TENANT), line);
        assertTrue(line.contains("correlation_id=" + CORRELATION), line);
        assertTrue(line.contains("authority_id=018f0e0a-0000-7000-8000-0000000000e1"), line);
        assertTrue(line.contains("deployment_id=deploy-primary"), line);
        assertTrue(line.contains("role=NETWORK"), line);
        assertTrue(line.contains("metadata="), line);
    }

    @Test
    void enrollmentAuditLogDoesNotContainTokenOrDigest() {
        String plaintextToken = "PlaintextEnrollmentToken_DO_NOT_LOG_abcdefghijklmnopqrstuvwxyz012345";
        String digest = "token_hash_sha256_deadbeefcafebabe0123456789abcdef0123456789abcdef0123456789ab";

        Map<String, String> metadata = new LinkedHashMap<>();
        metadata.put("authority_id", "018f0e0a-0000-7000-8000-0000000000e2");
        metadata.put("deployment_id", "deploy-safe");
        metadata.put("role", "NETWORK");

        emitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.SENSOR_ENROLLMENT_AUTHORITY_REVOKE,
                AuditOutcome.SUCCESS,
                TENANT,
                "SENSOR_ENROLLMENT_AUTHORITY_REVOKED",
                metadata
        );

        String line = securityAuditLines();
        assertTrue(line.contains("authority_id=018f0e0a-0000-7000-8000-0000000000e2"), line);
        assertFalse(line.contains(plaintextToken), line);
        assertFalse(line.contains(digest), line);
        assertFalse(line.contains("token_hash_sha256"), line);
    }

    @Test
    void secretBearingMetadataIsRejectedAndNotEmitted() {
        Map<String, String> metadata = Map.of("token", "should-never-appear-in-audit");

        emitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.SENSOR_ENROLLMENT_AUTHORITY_ISSUE,
                AuditOutcome.SUCCESS,
                TENANT,
                "SENSOR_ENROLLMENT_AUTHORITY_ISSUED",
                metadata
        );

        String all = appender.list.stream()
                .map(ILoggingEvent::getFormattedMessage)
                .collect(Collectors.joining("\n"));
        assertTrue(all.contains("security_audit_emit_failed"), all);
        assertFalse(all.contains("should-never-appear-in-audit"), all);
        assertFalse(all.contains("action=sensor.enrollment_authority.issue"), all);
    }

    private String securityAuditLines() {
        return appender.list.stream()
                .map(ILoggingEvent::getFormattedMessage)
                .filter(msg -> msg.startsWith("security_audit "))
                .collect(Collectors.joining("\n"));
    }
}
