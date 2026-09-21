package com.platform.core.domain.recurring;

import com.platform.core.domain.DomainException;
import com.platform.core.persistence.SensorRecurringAssignmentRepository;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.http.PlatformSecurityContext;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.sensorcontrol.SensorLifecycleStatus;
import com.platform.core.sensorcontrol.SensorPrincipal;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;

/**
 * Admin replace of Sensor recurring schedule assignments and Sensor machine pull/ACK (Slice A).
 *
 * <p>Lock order (non-negotiable): tenant FOR UPDATE → resolve Sensor → require ACTIVE →
 * Sensor FOR UPDATE → CAS expectedGeneration → mutate → recompute checksum →
 * increment desired generation if changed.
 *
 * <p>Entity authority reuses canonical Sensor Control ({@code allow_all_tenant_entities} /
 * {@code sensor_allowed_entity}). Entity identity is canonical UUID. Assignments that lose
 * entity scope are stripped and generation is bumped before Sensor pull (fail closed).
 *
 * <p>SNMP {@code target} remains unresolved ({@code BLOCKED_CANONICAL_TARGET_RESOLUTION}).
 * Credential Profile materialization remains unresolved
 * ({@code BLOCKED_CREDENTIAL_PROFILE_RUNTIME}).
 */
@Service
public class SensorRecurringAssignmentService {

    public static final String ERR_SENSOR_NOT_FOUND = "SENSOR_NOT_FOUND";
    public static final String ERR_SENSOR_NOT_ACTIVE = "SENSOR_NOT_ACTIVE";
    public static final String ERR_GENERATION_CONFLICT = "RECURRING_SCHEDULE_SENSOR_GENERATION_CONFLICT";
    public static final String ERR_STATE_STALE = "ERR_SENSOR_RECURRING_STATE_STALE";
    public static final String ERR_CHECKSUM_MISMATCH = "ERR_SENSOR_RECURRING_CHECKSUM_MISMATCH";
    public static final String ERR_SENSOR_ENTITY_INVALID = "ERR_SENSOR_ENTITY_INVALID";
    public static final String ERR_ENTITY_SCOPE_UNAUTHORIZED = "ERR_ENTITY_SCOPE_UNAUTHORIZED";

    private final SensorRecurringAssignmentRepository assignmentRepository;
    private final SensorControlRepository sensorControlRepository;
    private final SecurityAuditEmitter auditEmitter;

    public SensorRecurringAssignmentService(
            SensorRecurringAssignmentRepository assignmentRepository,
            SensorControlRepository sensorControlRepository,
            SecurityAuditEmitter auditEmitter
    ) {
        this.assignmentRepository = Objects.requireNonNull(assignmentRepository, "assignmentRepository");
        this.sensorControlRepository = Objects.requireNonNull(sensorControlRepository, "sensorControlRepository");
        this.auditEmitter = Objects.requireNonNull(auditEmitter, "auditEmitter");
    }

    public record AssignmentInput(
            String assignmentId,
            String entityId,
            String monitorDefinitionId,
            String schedulingClass,
            String executionClass,
            long cadenceMs,
            long freshnessMs,
            int phaseVersion,
            long executionTimeoutMs,
            String credentialProfileRef,
            String protocol,
            SnmpParams snmp,
            String protocolParamsJson
    ) {
    }

    public record AssignmentView(
            List<RecurringAssignment> assignments,
            long desiredGeneration,
            String desiredChecksum,
            long appliedGeneration,
            String appliedChecksum,
            Instant appliedAt
    ) {
    }

    @Transactional(value = "transactionManager", readOnly = true)
    public AssignmentView getAssignments(UUID tenantId, String sensorId) {
        requireTenant(tenantId);
        requireSensorExists(tenantId, sensorId);
        List<RecurringAssignment> assignments =
                assignmentRepository.listAssignments(tenantId, sensorId);
        RecurringState state = assignmentRepository.findState(tenantId, sensorId)
                .orElse(RecurringState.neverMutated(tenantId, sensorId));
        return new AssignmentView(
                assignments,
                state.desiredGeneration(),
                state.desiredChecksum(),
                state.appliedGeneration(),
                state.appliedChecksum(),
                state.appliedAt()
        );
    }

    @Transactional("transactionManager")
    public AssignmentView replaceAssignments(
            UUID tenantId,
            String sensorId,
            Long expectedGeneration,
            List<AssignmentInput> inputs
    ) {
        requireExpectedGeneration(expectedGeneration);
        Objects.requireNonNull(inputs, "assignments");
        if (inputs.size() > RecurringScheduleBounds.MAX_ASSIGNMENTS) {
            throw new DomainException(
                    "VALIDATION_FAILED",
                    "Assignment count exceeds max " + RecurringScheduleBounds.MAX_ASSIGNMENTS);
        }

        MutationContext ctx = beginMutation(tenantId, sensorId, expectedGeneration);
        EntityAuthorityScope scope = requireEntityAuthorityScope(tenantId, sensorId);

        String actor = PlatformSecurityContext.requirePrincipal().subjectId();
        Instant now = Instant.now();
        List<RecurringAssignment> nextRows = validateAndBuildRows(tenantId, sensorId, inputs, scope, actor, now);
        List<SensorRecurringDesiredChecksum.AssignmentDigest> digests = nextRows.stream()
                .map(SensorRecurringAssignmentRepository::toDigest)
                .toList();
        String nextChecksum = SensorRecurringDesiredChecksum.compute(digests);

        List<SensorRecurringDesiredChecksum.AssignmentDigest> currentDigests =
                assignmentRepository.listAssignmentDigests(tenantId, sensorId);
        String currentChecksum = SensorRecurringDesiredChecksum.compute(currentDigests);
        if (Objects.equals(currentChecksum, nextChecksum)
                && currentDigests.size() == digests.size()) {
            return snapshot(tenantId, sensorId, ctx.state());
        }

        assignmentRepository.deleteAllAssignments(tenantId, sensorId);
        for (RecurringAssignment row : nextRows) {
            assignmentRepository.insertAssignment(row);
        }

        RecurringState next = persistDesired(tenantId, sensorId, ctx, nextChecksum, actor, now, true);
        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.RECURRING_SCHEDULE_SENSOR_REPLACE,
                AuditOutcome.SUCCESS,
                tenantId,
                "RECURRING_SCHEDULE_SENSOR_REPLACE",
                Map.of(
                        "sensor_id", sensorId,
                        "assignment_count", Integer.toString(nextRows.size()),
                        "desired_generation", Long.toString(next.desiredGeneration())
                )
        );
        return snapshot(tenantId, sensorId, next);
    }

    /**
     * Sensor pull. Strips assignments that lost entity scope and bumps desired generation
     * so revoked allowlists cannot continue to authorize new desired state.
     */
    @Transactional("transactionManager")
    public SensorRecurringDesiredState getDesiredState(UUID tenantId, String sensorId) {
        if (!assignmentRepository.lockTenantForUpdate(tenantId)) {
            throw new DomainException("TENANT_NOT_FOUND", "Tenant not found");
        }
        Optional<String> sensorStatus = assignmentRepository.lockSensorForUpdate(tenantId, sensorId);
        if (sensorStatus.isEmpty()) {
            throw new DomainException(ERR_SENSOR_NOT_FOUND, "Sensor not found");
        }
        if (!"ACTIVE".equals(sensorStatus.get())) {
            throw new DomainException(ERR_SENSOR_NOT_ACTIVE, "Sensor is not ACTIVE");
        }

        RecurringState state = assignmentRepository.findStateForUpdate(tenantId, sensorId)
                .orElse(RecurringState.neverMutated(tenantId, sensorId));
        if (state.desiredGeneration() == 0L) {
            return new SensorRecurringDesiredState(
                    0L, null, state.appliedGeneration(), state.appliedChecksum(), List.of());
        }

        EntityAuthorityScope scope = requireEntityAuthorityScope(tenantId, sensorId);
        List<RecurringAssignment> rows = assignmentRepository.listAssignments(tenantId, sensorId);
        List<RecurringAssignment> authorized = new ArrayList<>(rows.size());
        List<String> revokedAssignmentIds = new ArrayList<>();
        for (RecurringAssignment row : rows) {
            if (isEntityAuthorized(scope, row.entityId())) {
                authorized.add(row);
            } else {
                revokedAssignmentIds.add(row.assignmentId());
            }
        }

        if (!revokedAssignmentIds.isEmpty()) {
            for (String assignmentId : revokedAssignmentIds) {
                assignmentRepository.deleteAssignment(tenantId, sensorId, assignmentId);
            }
            List<SensorRecurringDesiredChecksum.AssignmentDigest> digests = authorized.stream()
                    .map(SensorRecurringAssignmentRepository::toDigest)
                    .toList();
            String nextChecksum = SensorRecurringDesiredChecksum.compute(digests);
            Instant now = Instant.now();
            String actor = "system:entity-scope-reconcile";
            MutationContext ctx = new MutationContext(state);
            state = persistDesired(tenantId, sensorId, ctx, nextChecksum, actor, now, true);
            auditEmitter.emitControlEvent(
                    AuditCategory.SECURITY_ADMINISTRATION,
                    AuditActions.RECURRING_SCHEDULE_SENSOR_REPLACE,
                    AuditOutcome.SUCCESS,
                    tenantId,
                    "RECURRING_SCHEDULE_ENTITY_SCOPE_RECONCILE",
                    Map.of(
                            "sensor_id", sensorId,
                            "revoked_count", Integer.toString(revokedAssignmentIds.size()),
                            "desired_generation", Long.toString(state.desiredGeneration())
                    )
            );
            rows = authorized;
        }

        if (rows.size() > RecurringScheduleBounds.MAX_ASSIGNMENTS) {
            throw new DomainException(
                    "RECURRING_SCHEDULE_SNAPSHOT_INVALID",
                    "Desired assignment count exceeds max " + RecurringScheduleBounds.MAX_ASSIGNMENTS);
        }
        List<DesiredRecurringDefinition> definitions = new ArrayList<>(rows.size());
        for (RecurringAssignment row : rows) {
            SnmpParams snmp = RecurringScheduleBounds.parseAndValidateSnmpParams(row.protocolParamsJson());
            definitions.add(new DesiredRecurringDefinition(
                    row.assignmentId(),
                    row.tenantId().toString(),
                    row.entityId(),
                    row.monitorDefinitionId(),
                    row.schedulingClass(),
                    row.executionClass(),
                    row.cadenceMs(),
                    row.freshnessMs(),
                    row.phaseVersion(),
                    row.executionTimeoutMs(),
                    row.credentialProfileRef(),
                    row.protocol(),
                    snmp
            ));
        }
        return new SensorRecurringDesiredState(
                state.desiredGeneration(),
                state.desiredChecksum(),
                state.appliedGeneration(),
                state.appliedChecksum(),
                definitions
        );
    }

    @Transactional("transactionManager")
    public void ackAppliedState(
            UUID tenantId,
            String sensorId,
            long generation,
            String checksum
    ) {
        if (generation < 0) {
            throw new DomainException("VALIDATION_FAILED", "generation must be >= 0");
        }
        if (checksum != null && !checksum.matches("^[a-f0-9]{64}$")) {
            throw new DomainException("VALIDATION_FAILED", "checksum must be lowercase 64-hex when present");
        }

        if (!assignmentRepository.lockTenantForUpdate(tenantId)) {
            throw new DomainException("TENANT_NOT_FOUND", "Tenant not found");
        }
        Optional<String> sensorStatus = assignmentRepository.lockSensorForUpdate(tenantId, sensorId);
        if (sensorStatus.isEmpty()) {
            throw new DomainException(ERR_SENSOR_NOT_FOUND, "Sensor not found");
        }

        RecurringState state = assignmentRepository.findStateForUpdate(tenantId, sensorId)
                .orElse(RecurringState.neverMutated(tenantId, sensorId));

        if (generation > state.desiredGeneration()) {
            throw new DomainException(ERR_STATE_STALE, "Applied generation is ahead of desired");
        }
        if (generation < state.desiredGeneration()) {
            throw new DomainException(ERR_STATE_STALE, "Applied generation is stale relative to desired");
        }
        String desiredChecksum = state.desiredChecksum();
        if (!Objects.equals(desiredChecksum, checksum)) {
            throw new DomainException(ERR_CHECKSUM_MISMATCH, "Applied checksum does not match desired");
        }
        if (state.desiredGeneration() == 0L) {
            return;
        }
        if (state.appliedGeneration() == generation && Objects.equals(state.appliedChecksum(), checksum)) {
            return;
        }

        Instant now = Instant.now();
        int updated = assignmentRepository.updateAppliedState(
                tenantId, sensorId, generation, checksum, now, "sensor:" + sensorId, now);
        if (updated != 1) {
            throw new DomainException(ERR_STATE_STALE, "Applied state could not be recorded");
        }
    }

    private List<RecurringAssignment> validateAndBuildRows(
            UUID tenantId,
            String sensorId,
            List<AssignmentInput> inputs,
            EntityAuthorityScope scope,
            String actor,
            Instant now
    ) {
        Set<String> seen = new HashSet<>();
        List<RecurringAssignment> rows = new ArrayList<>(inputs.size());
        for (AssignmentInput input : inputs) {
            if (input == null) {
                throw new DomainException("VALIDATION_FAILED", "assignment entry is required");
            }
            String assignmentId = RecurringScheduleBounds.requireId("assignmentId", input.assignmentId());
            if (!seen.add(assignmentId)) {
                throw new DomainException("VALIDATION_FAILED", "duplicate assignmentId");
            }
            UUID entityUuid = requireAuthorizedEntity(scope, input.entityId());
            String entityId = entityUuid.toString();
            String monitorDefinitionId = RecurringScheduleBounds.requireId(
                    "monitorDefinitionId", input.monitorDefinitionId());
            String schedulingClass = RecurringScheduleBounds.requireClass(
                    "schedulingClass", input.schedulingClass(), RecurringScheduleBounds.SCHEDULING_CLASSES);
            String executionClass = RecurringScheduleBounds.requireClass(
                    "executionClass", input.executionClass(), RecurringScheduleBounds.EXECUTION_CLASSES);
            RecurringScheduleBounds.requireCadence(input.cadenceMs());
            RecurringScheduleBounds.requireFreshness(input.freshnessMs());
            RecurringScheduleBounds.requirePhaseVersion(input.phaseVersion());
            RecurringScheduleBounds.requireExecutionTimeout(input.executionTimeoutMs());
            RecurringScheduleBounds.requireProtocol(input.protocol());
            String credentialProfileRef = RecurringScheduleBounds.requireCredentialRef(
                    input.credentialProfileRef());

            SnmpParams snmp;
            if (input.snmp() != null) {
                snmp = RecurringScheduleBounds.requireSnmp(input.snmp());
            } else if (input.protocolParamsJson() != null && !input.protocolParamsJson().isBlank()) {
                snmp = RecurringScheduleBounds.parseAndValidateSnmpParams(input.protocolParamsJson());
            } else {
                throw new DomainException("VALIDATION_FAILED", "snmp params required for SNMP protocol");
            }
            String protocolParamsJson = RecurringScheduleBounds.canonicalizeSnmpJson(snmp);

            rows.add(new RecurringAssignment(
                    tenantId,
                    sensorId,
                    assignmentId,
                    entityId,
                    monitorDefinitionId,
                    schedulingClass,
                    executionClass,
                    input.cadenceMs(),
                    input.freshnessMs(),
                    input.phaseVersion(),
                    input.executionTimeoutMs(),
                    credentialProfileRef,
                    RecurringScheduleBounds.PROTOCOL_SNMP,
                    protocolParamsJson,
                    actor,
                    now
            ));
        }
        return rows;
    }

    private EntityAuthorityScope requireEntityAuthorityScope(UUID tenantId, String sensorId) {
        SensorPrincipal principal = sensorControlRepository.findPrincipalByTenantAndId(tenantId, sensorId)
                .orElseThrow(() -> new DomainException(ERR_SENSOR_NOT_FOUND, "Sensor not found"));
        if (principal.status() != SensorLifecycleStatus.ACTIVE) {
            throw new DomainException(ERR_SENSOR_NOT_ACTIVE, "Sensor is not ACTIVE");
        }
        if (!principal.tenantId().equals(tenantId)) {
            throw new DomainException(ERR_SENSOR_NOT_FOUND, "Sensor not found");
        }
        Set<UUID> allowed = principal.allowAllTenantEntities()
                ? Set.of()
                : Set.copyOf(sensorControlRepository.listAllowedEntityIds(tenantId, sensorId));
        return new EntityAuthorityScope(tenantId, principal.allowAllTenantEntities(), allowed);
    }

    private UUID requireAuthorizedEntity(EntityAuthorityScope scope, String rawEntityId) {
        UUID entityId = requireEntityUuid(rawEntityId);
        if (!sensorControlRepository.entityExists(scope.tenantId(), entityId)) {
            throw new DomainException(ERR_SENSOR_ENTITY_INVALID, "Entity not found in tenant: " + entityId);
        }
        if (scope.allowAllTenantEntities()) {
            return entityId;
        }
        if (!scope.allowedEntityIds().contains(entityId)) {
            throw new DomainException(
                    ERR_ENTITY_SCOPE_UNAUTHORIZED,
                    "Entity is not authorized for this Sensor: " + entityId);
        }
        return entityId;
    }

    private boolean isEntityAuthorized(EntityAuthorityScope scope, String entityIdText) {
        final UUID entityId;
        try {
            entityId = UUID.fromString(entityIdText);
        } catch (IllegalArgumentException | NullPointerException ex) {
            return false;
        }
        if (!sensorControlRepository.entityExists(scope.tenantId(), entityId)) {
            return false;
        }
        if (scope.allowAllTenantEntities()) {
            return true;
        }
        return scope.allowedEntityIds().contains(entityId);
    }

    private static UUID requireEntityUuid(String rawEntityId) {
        String id = RecurringScheduleBounds.requireId("entityId", rawEntityId);
        try {
            return UUID.fromString(id);
        } catch (IllegalArgumentException ex) {
            throw new DomainException(ERR_SENSOR_ENTITY_INVALID, "entityId must be a canonical UUID");
        }
    }

    private MutationContext beginMutation(UUID tenantId, String sensorId, long expectedGeneration) {
        if (!assignmentRepository.lockTenantForUpdate(tenantId)) {
            throw new DomainException("TENANT_NOT_FOUND", "Tenant not found");
        }
        Optional<String> statusBeforeLock = assignmentRepository.findSensorStatus(tenantId, sensorId);
        if (statusBeforeLock.isEmpty()) {
            throw new DomainException(ERR_SENSOR_NOT_FOUND, "Sensor not found");
        }
        if (!"ACTIVE".equals(statusBeforeLock.get())) {
            throw new DomainException(ERR_SENSOR_NOT_ACTIVE, "Sensor is not ACTIVE");
        }
        Optional<String> lockedStatus = assignmentRepository.lockSensorForUpdate(tenantId, sensorId);
        if (lockedStatus.isEmpty() || !"ACTIVE".equals(lockedStatus.get())) {
            throw new DomainException(ERR_SENSOR_NOT_ACTIVE, "Sensor is not ACTIVE");
        }

        RecurringState state = assignmentRepository.findStateForUpdate(tenantId, sensorId)
                .orElse(RecurringState.neverMutated(tenantId, sensorId));
        if (state.desiredGeneration() != expectedGeneration) {
            throw new DomainException(ERR_GENERATION_CONFLICT, "Sensor recurring schedule generation conflict");
        }
        return new MutationContext(state);
    }

    private RecurringState persistDesired(
            UUID tenantId,
            String sensorId,
            MutationContext ctx,
            String checksum,
            String actor,
            Instant now,
            boolean increment
    ) {
        long nextGen = increment ? ctx.state().desiredGeneration() + 1L : ctx.state().desiredGeneration();
        assignmentRepository.upsertDesiredState(tenantId, sensorId, nextGen, checksum, actor, now);
        return new RecurringState(
                tenantId,
                sensorId,
                nextGen,
                checksum,
                ctx.state().appliedGeneration(),
                ctx.state().appliedChecksum(),
                ctx.state().appliedAt(),
                actor,
                now
        );
    }

    private AssignmentView snapshot(UUID tenantId, String sensorId, RecurringState state) {
        return new AssignmentView(
                assignmentRepository.listAssignments(tenantId, sensorId),
                state.desiredGeneration(),
                state.desiredChecksum(),
                state.appliedGeneration(),
                state.appliedChecksum(),
                state.appliedAt()
        );
    }

    private void requireTenant(UUID tenantId) {
        Objects.requireNonNull(tenantId, "tenantId");
        if (!assignmentRepository.tenantExists(tenantId)) {
            throw new DomainException("TENANT_NOT_FOUND", "Tenant not found");
        }
    }

    private void requireSensorExists(UUID tenantId, String sensorId) {
        Objects.requireNonNull(sensorId, "sensorId");
        if (assignmentRepository.findSensorStatus(tenantId, sensorId).isEmpty()) {
            throw new DomainException(ERR_SENSOR_NOT_FOUND, "Sensor not found");
        }
    }

    private static void requireExpectedGeneration(Long expectedGeneration) {
        if (expectedGeneration == null) {
            throw new DomainException("VALIDATION_FAILED", "expectedGeneration is required");
        }
        if (expectedGeneration < 0) {
            throw new DomainException("VALIDATION_FAILED", "expectedGeneration must be >= 0");
        }
    }

    private record MutationContext(RecurringState state) {
    }

    private record EntityAuthorityScope(
            UUID tenantId,
            boolean allowAllTenantEntities,
            Set<UUID> allowedEntityIds
    ) {
        EntityAuthorityScope {
            Objects.requireNonNull(tenantId, "tenantId");
            Objects.requireNonNull(allowedEntityIds, "allowedEntityIds");
        }
    }
}
