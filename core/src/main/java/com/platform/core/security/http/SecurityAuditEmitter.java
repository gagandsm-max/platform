package com.platform.core.security.http;

import com.platform.core.security.audit.ActorType;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.audit.AuditReasonCode;
import com.platform.core.security.audit.CorrelationId;
import com.platform.core.security.audit.SecurityAuditEvent;
import com.platform.core.logging.CorrelationIdFilter;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.Map;
import java.util.UUID;

/**
 * Emits structured security audit facts without tokens, secrets, or claim dumps.
 * Persistence is out of scope; failures are logged for operations.
 */
@Component
public class SecurityAuditEmitter {

    private static final Logger log = LoggerFactory.getLogger(SecurityAuditEmitter.class);

    public void authenticationFailure(String reasonCode) {
        emit(AuditCategory.AUTHENTICATION, AuditActions.AUTHENTICATION_LOGIN, AuditOutcome.FAILURE,
                ActorType.SYSTEM, null, null, reasonCode);
    }

    public void authorizationDeny(UUID actorSubjectId, UUID tenantId, String reasonCode) {
        emit(AuditCategory.AUTHORIZATION, AuditActions.AUTHORIZATION_DENY, AuditOutcome.DENIED,
                ActorType.USER, actorSubjectId, tenantId, reasonCode);
    }

    public void authorizationAllow(UUID actorSubjectId, UUID tenantId, String reasonCode) {
        emit(AuditCategory.AUTHORIZATION, AuditActions.AUTHORIZATION_ALLOW, AuditOutcome.SUCCESS,
                ActorType.USER, actorSubjectId, tenantId, reasonCode);
    }

    /**
     * Emits a control-plane Sensor Control / security-administration audit fact.
     * Metadata must never contain secrets, private keys, or raw certificate material.
     */
    public void emitControlEvent(
            AuditCategory category,
            com.platform.core.security.audit.AuditAction action,
            AuditOutcome outcome,
            UUID tenantId,
            String reasonCode,
            Map<String, String> metadata
    ) {
        emit(category, action, outcome, ActorType.SYSTEM, null, tenantId, reasonCode,
                metadata == null ? Map.of() : metadata);
    }

    private void emit(
            AuditCategory category,
            com.platform.core.security.audit.AuditAction action,
            AuditOutcome outcome,
            ActorType actorType,
            UUID actorSubjectId,
            UUID tenantId,
            String reasonCode
    ) {
        emit(category, action, outcome, actorType, actorSubjectId, tenantId, reasonCode, Map.of());
    }

    private void emit(
            AuditCategory category,
            com.platform.core.security.audit.AuditAction action,
            AuditOutcome outcome,
            ActorType actorType,
            UUID actorSubjectId,
            UUID tenantId,
            String reasonCode,
            Map<String, String> metadata
    ) {
        try {
            String corr = MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY);
            CorrelationId correlationId = (corr == null || corr.isBlank())
                    ? new CorrelationId(UUID.randomUUID().toString())
                    : new CorrelationId(corr);
            SecurityAuditEvent event = SecurityAuditEvent.create(
                    Instant.now(),
                    correlationId,
                    category,
                    action,
                    outcome,
                    actorType,
                    actorSubjectId,
                    tenantId,
                    null,
                    null,
                    null,
                    reasonCode == null ? null : new AuditReasonCode(reasonCode),
                    metadata
            );
            log.info("security_audit event_id={} category={} action={} outcome={} reason={}",
                    event.eventId(), category, action.value(), outcome, reasonCode == null ? "" : reasonCode);
        } catch (RuntimeException ex) {
            log.warn("security_audit_emit_failed");
        }
    }
}
