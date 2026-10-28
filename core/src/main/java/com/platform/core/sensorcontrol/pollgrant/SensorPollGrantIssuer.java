package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.control.ControlGenerationLimits;
import com.platform.core.domain.inventory.EntityManagementEndpoint;
import com.platform.core.domain.inventory.ManagementEndpointState;
import com.platform.core.id.UuidV7;
import com.platform.core.persistence.EntityManagementEndpointRepository;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.sensorcontrol.SensorControlException;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.sensorcontrol.SensorLifecycleStatus;
import com.platform.core.sensorcontrol.SensorPrincipal;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.transaction.support.TransactionSynchronization;
import org.springframework.transaction.support.TransactionSynchronizationManager;

import java.time.Clock;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.UUID;
import java.util.function.Supplier;

import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.AddressType;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Protocol;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Status;

/**
 * Internal Core issuer for one already-planned bounded poll grant (Issue #229 G1a).
 *
 * <p>Not a public API, scheduler, admission path, generation allocator or plan catalogue.
 * Core alone creates grant ID, issued timestamp, endpoint address snapshot, initial
 * {@code ISSUED} status and updated timestamp. Callers must supply every binding field
 * explicitly; this class invents no cadence, TTL, offline horizon, rate/attempt default
 * or uniqueness/monotonicity contract beyond V015 column checks and max-safe bounds.
 *
 * <p><b>Deterministic lock order</b> (caller-held Spring JDBC / PostgreSQL transaction):
 * <ol>
 *   <li>{@code platform.sensor} principal {@code FOR SHARE} (tenant_id, sensor_id)</li>
 *   <li>{@code platform.entity_management_endpoint} {@code FOR SHARE}
 *       (tenant_id, entity_id, endpoint_id)</li>
 *   <li>{@code INSERT} into {@code platform.sensor_poll_grant}</li>
 * </ol>
 * Principal {@code FOR SHARE} blocks disable/revoke/scope replacement while permitting
 * independent valid issuances. Endpoint share lock retains the exact address snapshot
 * until commit/rollback. Core time is sampled only after both locks are held so a window
 * that expires while waiting is rejected. This issuer never opens a replacement or
 * {@code REQUIRES_NEW} transaction.
 *
 * <p><b>Post-commit audit</b> is best-effort logging only: crash after commit can still
 * lose the audit; audit failure cannot roll back an already committed grant; this slice
 * does not implement durable transactional audit. Issuance success is never emitted
 * before owning-transaction commit. Active Spring transaction synchronization is required
 * before insert or audit registration; missing synchronization fails closed.
 *
 * <p>Remaining caller responsibility: plan existence/authority, generation allocation and
 * uniqueness across grants, delivery, renewal and typed admission (G1b/G2).
 */
public final class SensorPollGrantIssuer {

    private final SensorControlRepository sensors;
    private final EntityManagementEndpointRepository endpoints;
    private final SensorPollGrantRepository grants;
    private final SecurityAuditEmitter audit;
    private final Clock clock;
    private final Supplier<UUID> grantIds;

    public SensorPollGrantIssuer(
            SensorControlRepository sensors,
            EntityManagementEndpointRepository endpoints,
            SensorPollGrantRepository grants,
            SecurityAuditEmitter audit
    ) {
        this(sensors, endpoints, grants, audit, Clock.systemUTC(), UuidV7::generate);
    }

    /**
     * Package-local Clock/ID seam for deterministic tests only. Does not introduce
     * issuance policy, cadence, TTL or generation defaults.
     */
    SensorPollGrantIssuer(
            SensorControlRepository sensors,
            EntityManagementEndpointRepository endpoints,
            SensorPollGrantRepository grants,
            SecurityAuditEmitter audit,
            Clock clock,
            Supplier<UUID> grantIds
    ) {
        this.sensors = Objects.requireNonNull(sensors, "sensors");
        this.endpoints = Objects.requireNonNull(endpoints, "endpoints");
        this.grants = Objects.requireNonNull(grants, "grants");
        this.audit = Objects.requireNonNull(audit, "audit");
        this.clock = Objects.requireNonNull(clock, "clock");
        this.grantIds = Objects.requireNonNull(grantIds, "grantIds");
    }

    /**
     * Issue exactly one {@code ISSUED} grant inside the caller's open transaction.
     * Returns the exact PostgreSQL-persisted row from {@code INSERT ... RETURNING}.
     * Rolls back with the caller on any failure; registers success audit only for
     * after-commit emission.
     */
    public SensorPollGrant issue(IssueRequest request) {
        if (request == null) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                    "Poll grant request is invalid");
        }
        ValidatedIssue validated = validate(request);

        requireHeldTransactionAndSynchronization();

        SensorPrincipal principal = sensors.lockPrincipalForShare(
                validated.tenantId(), validated.sensorId());
        if (principal.status() != SensorLifecycleStatus.ACTIVE) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_STATE_INVALID,
                    "Sensor is not ACTIVE");
        }
        requireEntityAuthorized(principal, validated.entityId());

        EntityManagementEndpoint endpoint = endpoints
                .findByTenantEntityAndIdForShare(
                        validated.tenantId(), validated.entityId(), validated.endpointId())
                .orElseThrow(() -> new SensorControlException(
                        SensorControlException.ERR_POLL_GRANT_ENDPOINT_UNAVAILABLE,
                        "Management endpoint unavailable for issuance"));
        if (endpoint.state() != ManagementEndpointState.ACTIVE) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_ENDPOINT_UNAVAILABLE,
                    "Management endpoint unavailable for issuance");
        }

        Instant issuedAt = clock.instant();
        if (issuedAt.isBefore(validated.windowStart()) || issuedAt.isAfter(validated.windowEnd())) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_WINDOW_INVALID,
                    "Issued time must fall within the grant window");
        }

        UUID grantId = Objects.requireNonNull(grantIds.get(), "grantId");
        SensorPollGrant candidate = new SensorPollGrant(
                grantId,
                validated.tenantId(),
                validated.sensorId(),
                validated.entityId(),
                validated.endpointId(),
                validated.grantGeneration(),
                validated.assignmentGeneration(),
                validated.protocol(),
                AddressType.valueOf(endpoint.addressType().name()),
                endpoint.address(),
                validated.profilePlanId(),
                validated.profilePlanVersion(),
                issuedAt,
                validated.windowStart(),
                validated.windowEnd(),
                validated.issuedBy(),
                Status.ISSUED,
                null,
                null,
                null,
                null,
                issuedAt
        );

        final SensorPollGrant persisted;
        try {
            persisted = grants.insert(candidate);
        } catch (DuplicateKeyException ex) {
            throw conflict();
        } catch (DataIntegrityViolationException ex) {
            throw conflict();
        }

        registerIssuedAuditAfterCommit(persisted);
        return persisted;
    }

    private void requireHeldTransactionAndSynchronization() {
        grants.requireHeldCallerTransaction("issue");
        if (!TransactionSynchronizationManager.isSynchronizationActive()) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_STATE_INVALID,
                    "Active Spring transaction synchronization required for issue; "
                            + "issuance success must not emit before owning commit");
        }
    }

    private void requireEntityAuthorized(SensorPrincipal principal, UUID entityId) {
        if (principal.allowAllTenantEntities()) {
            return;
        }
        Set<UUID> allowed = sensors.listAllowedEntityIds(principal.tenantId(), principal.sensorId());
        if (!allowed.contains(entityId)) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_ENTITY_OUT_OF_SCOPE,
                    "Entity is outside Sensor scope");
        }
    }

    private void registerIssuedAuditAfterCommit(SensorPollGrant row) {
        // Synchronization already proven before insert; register exactly one afterCommit.
        Map<String, String> metadata = new LinkedHashMap<>();
        metadata.put("grant_id", row.grantId().toString());
        metadata.put("sensor_id", row.sensorId());
        metadata.put("entity_id", row.entityId().toString());
        metadata.put("endpoint_id", row.endpointId().toString());
        metadata.put("protocol", row.protocol().name());
        metadata.put("grant_generation", Long.toString(row.grantGeneration()));
        metadata.put("assignment_generation", Long.toString(row.assignmentGeneration()));
        TransactionSynchronizationManager.registerSynchronization(new TransactionSynchronization() {
            @Override
            public void afterCommit() {
                // Best-effort only: crash after commit can lose this; failure cannot roll
                // back the committed grant; durable transactional audit is out of scope.
                audit.emitControlEvent(
                        AuditCategory.SECURITY_ADMINISTRATION,
                        AuditActions.SENSOR_POLL_GRANT_ISSUE,
                        AuditOutcome.SUCCESS,
                        row.tenantId(),
                        "SENSOR_POLL_GRANT_ISSUED",
                        metadata
                );
            }
        });
    }

    private static SensorControlException conflict() {
        return new SensorControlException(
                SensorControlException.ERR_POLL_GRANT_CONFLICT,
                "Poll grant could not be issued");
    }

    private static ValidatedIssue validate(IssueRequest request) {
        UUID tenantId = requireUuid(request.tenantId(), "tenantId");
        String sensorId = requireSensorId(request.sensorId());
        UUID entityId = requireUuid(request.entityId(), "entityId");
        UUID endpointId = requireUuid(request.endpointId(), "endpointId");
        Protocol protocol = requireProtocol(request.protocol());
        Instant windowStart = requireInstant(request.windowStart(), "windowStart");
        Instant windowEnd = requireInstant(request.windowEnd(), "windowEnd");
        String issuedBy = requireIssuedBy(request.issuedBy());
        String planId = requirePlanField(request.profilePlanId(), "profilePlanId");
        String planVersion = requirePlanField(request.profilePlanVersion(), "profilePlanVersion");

        if (!ControlGenerationLimits.isPositiveJsonSafe(request.grantGeneration())
                || !ControlGenerationLimits.isJsonSafe(request.assignmentGeneration())) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_GENERATION_INVALID,
                    "Grant or assignment generation is outside max-safe bounds");
        }
        if (!windowEnd.isAfter(windowStart)) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_WINDOW_INVALID,
                    "Grant window end must be after window start");
        }

        return new ValidatedIssue(
                tenantId, sensorId, entityId, endpointId, protocol,
                request.grantGeneration(), request.assignmentGeneration(),
                planId, planVersion, windowStart, windowEnd, issuedBy);
    }

    private static UUID requireUuid(UUID value, String field) {
        if (value == null) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                    "Poll grant request is invalid");
        }
        return value;
    }

    private static Instant requireInstant(Instant value, String field) {
        if (value == null) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                    "Poll grant request is invalid");
        }
        return value;
    }

    private static Protocol requireProtocol(Protocol protocol) {
        if (protocol == null) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                    "Poll grant request is invalid");
        }
        return protocol;
    }

    private static String requireSensorId(String sensorId) {
        if (sensorId == null || sensorId.isBlank() || sensorId.length() > 128) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_NOT_FOUND,
                    "Sensor not found");
        }
        return sensorId;
    }

    private static String requireIssuedBy(String issuedBy) {
        if (issuedBy == null || issuedBy.isEmpty() || issuedBy.length() > 256) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                    "Poll grant request is invalid");
        }
        return issuedBy;
    }

    private static String requirePlanField(String value, String field) {
        if (value == null || value.isBlank() || !value.equals(value.trim())) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_PLAN_INVALID,
                    "Profile-plan binding is invalid");
        }
        if ("profilePlanId".equals(field) && value.length() > 128) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_PLAN_INVALID,
                    "Profile-plan binding is invalid");
        }
        if ("profilePlanVersion".equals(field) && value.length() > 64) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_PLAN_INVALID,
                    "Profile-plan binding is invalid");
        }
        return value;
    }

    /**
     * Caller-supplied issuance binding. Core creates grant ID, issued/updated timestamps,
     * endpoint address snapshot and initial {@code ISSUED} status — those fields are absent.
     */
    public record IssueRequest(
            UUID tenantId,
            String sensorId,
            UUID entityId,
            UUID endpointId,
            Protocol protocol,
            long grantGeneration,
            long assignmentGeneration,
            String profilePlanId,
            String profilePlanVersion,
            Instant windowStart,
            Instant windowEnd,
            String issuedBy
    ) {
    }

    private record ValidatedIssue(
            UUID tenantId,
            String sensorId,
            UUID entityId,
            UUID endpointId,
            Protocol protocol,
            long grantGeneration,
            long assignmentGeneration,
            String profilePlanId,
            String profilePlanVersion,
            Instant windowStart,
            Instant windowEnd,
            String issuedBy
    ) {
    }
}
