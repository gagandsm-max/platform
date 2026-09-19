package com.platform.core.domain.capabilitypack;

import com.platform.core.domain.DomainException;
import com.platform.core.persistence.CapabilityPackRepository;
import com.platform.core.persistence.SensorCapabilityPackAssignmentRepository;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.http.PlatformSecurityContext;
import com.platform.core.security.http.SecurityAuditEmitter;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;

/**
 * Admin assignment of Capability Packs to Sensors and Sensor machine pull/ACK (EMC1.3).
 *
 * <p>Lock order (non-negotiable): tenant FOR UPDATE → resolve Sensor → require ACTIVE →
 * Sensor FOR UPDATE → CAS expectedGeneration → validate ACTIVE pack version → mutate →
 * recompute checksum → increment desired generation if changed.
 */
@Service
public class SensorCapabilityPackAssignmentService {

    public static final String ERR_SENSOR_NOT_FOUND = "SENSOR_NOT_FOUND";
    public static final String ERR_SENSOR_NOT_ACTIVE = "SENSOR_NOT_ACTIVE";
    public static final String ERR_GENERATION_CONFLICT = "CAPABILITY_PACK_SENSOR_GENERATION_CONFLICT";
    public static final String ERR_VERSION_NOT_ACTIVE = "CAPABILITY_PACK_VERSION_NOT_ACTIVE";
    public static final String ERR_SENSOR_ASSIGNMENTS_EXIST = "CAPABILITY_PACK_SENSOR_ASSIGNMENTS_EXIST";
    public static final String ERR_PACK_STATE_STALE = "ERR_SENSOR_PACK_STATE_STALE";
    public static final String ERR_PACK_CHECKSUM_MISMATCH = "ERR_SENSOR_PACK_CHECKSUM_MISMATCH";
    public static final String ERR_ARTIFACT_NOT_FOUND = "CAPABILITY_PACK_ARTIFACT_NOT_FOUND";

    private final SensorCapabilityPackAssignmentRepository assignmentRepository;
    private final CapabilityPackRepository packRepository;
    private final SecurityAuditEmitter auditEmitter;

    public SensorCapabilityPackAssignmentService(
            SensorCapabilityPackAssignmentRepository assignmentRepository,
            CapabilityPackRepository packRepository,
            SecurityAuditEmitter auditEmitter
    ) {
        this.assignmentRepository = Objects.requireNonNull(assignmentRepository, "assignmentRepository");
        this.packRepository = Objects.requireNonNull(packRepository, "packRepository");
        this.auditEmitter = Objects.requireNonNull(auditEmitter, "auditEmitter");
    }

    public record AssignmentView(
            List<SensorCapabilityPackAssignment> assignments,
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
        List<SensorCapabilityPackAssignment> assignments =
                assignmentRepository.listAssignments(tenantId, sensorId);
        SensorCapabilityPackState state = assignmentRepository.findState(tenantId, sensorId)
                .orElse(SensorCapabilityPackState.neverMutated(tenantId, sensorId));
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
    public AssignmentView assign(
            UUID tenantId,
            String sensorId,
            String packId,
            String packVersion,
            Long expectedGeneration
    ) {
        Objects.requireNonNull(packId, "packId");
        Objects.requireNonNull(packVersion, "packVersion");
        requireExpectedGeneration(expectedGeneration);

        MutationContext ctx = beginMutation(tenantId, sensorId, expectedGeneration);
        Optional<SensorCapabilityPackAssignment> existing =
                assignmentRepository.findAssignment(tenantId, sensorId, packId);
        if (existing.isPresent() && existing.get().packVersion().equals(packVersion)) {
            return snapshot(tenantId, sensorId, ctx.state());
        }

        requireActivePackVersion(tenantId, packId, packVersion);

        String actor = PlatformSecurityContext.requirePrincipal().subjectId();
        Instant now = Instant.now();
        assignmentRepository.upsertAssignment(new SensorCapabilityPackAssignment(
                tenantId, sensorId, packId, packVersion, actor, now));

        SensorCapabilityPackState next = recomputeAndPersistDesired(tenantId, sensorId, ctx, actor, now, true);
        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.CAPABILITY_PACK_SENSOR_ASSIGN,
                AuditOutcome.SUCCESS,
                tenantId,
                "CAPABILITY_PACK_SENSOR_ASSIGN",
                Map.of("sensor_id", sensorId, "pack_id", packId, "pack_version", packVersion)
        );
        return snapshot(tenantId, sensorId, next);
    }

    @Transactional("transactionManager")
    public AssignmentView unassign(
            UUID tenantId,
            String sensorId,
            String packId,
            Long expectedGeneration
    ) {
        Objects.requireNonNull(packId, "packId");
        requireExpectedGeneration(expectedGeneration);

        MutationContext ctx = beginMutation(tenantId, sensorId, expectedGeneration);
        int deleted = assignmentRepository.deleteAssignment(tenantId, sensorId, packId);
        if (deleted == 0) {
            return snapshot(tenantId, sensorId, ctx.state());
        }

        String actor = PlatformSecurityContext.requirePrincipal().subjectId();
        Instant now = Instant.now();
        SensorCapabilityPackState next = recomputeAndPersistDesired(tenantId, sensorId, ctx, actor, now, true);
        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.CAPABILITY_PACK_SENSOR_UNASSIGN,
                AuditOutcome.SUCCESS,
                tenantId,
                "CAPABILITY_PACK_SENSOR_UNASSIGN",
                Map.of("sensor_id", sensorId, "pack_id", packId)
        );
        return snapshot(tenantId, sensorId, next);
    }

    @Transactional(value = "transactionManager", readOnly = true)
    public SensorPackDesiredState getDesiredState(UUID tenantId, String sensorId) {
        SensorCapabilityPackState state = assignmentRepository.findState(tenantId, sensorId)
                .orElse(SensorCapabilityPackState.neverMutated(tenantId, sensorId));
        if (state.desiredGeneration() == 0L) {
            return new SensorPackDesiredState(
                    0L, null, state.appliedGeneration(), state.appliedChecksum(), List.of());
        }
        List<DesiredPackEntry> packs = assignmentRepository.listDesiredPackEntries(tenantId, sensorId);
        if (packs.size() > CapabilityPackRegistry.MAX_PACK_COUNT) {
            throw new DomainException(
                    "CAPABILITY_PACK_SNAPSHOT_INVALID",
                    "Desired pack count exceeds max " + CapabilityPackRegistry.MAX_PACK_COUNT);
        }
        return new SensorPackDesiredState(
                state.desiredGeneration(),
                state.desiredChecksum(),
                state.appliedGeneration(),
                state.appliedChecksum(),
                packs
        );
    }

    @Transactional(value = "transactionManager", readOnly = true)
    public StoredCapabilityPack requireAssignedArtifact(
            UUID tenantId, String sensorId, String packId, String packVersion) {
        return assignmentRepository.findAssignedArtifact(tenantId, sensorId, packId, packVersion)
                .orElseThrow(() -> new DomainException(ERR_ARTIFACT_NOT_FOUND, "Pack artifact not found"));
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

        SensorCapabilityPackState state = assignmentRepository.findStateForUpdate(tenantId, sensorId)
                .orElse(SensorCapabilityPackState.neverMutated(tenantId, sensorId));

        if (generation > state.desiredGeneration()) {
            throw new DomainException(ERR_PACK_STATE_STALE, "Applied generation is ahead of desired");
        }
        if (generation < state.desiredGeneration()) {
            throw new DomainException(ERR_PACK_STATE_STALE, "Applied generation is stale relative to desired");
        }
        String desiredChecksum = state.desiredChecksum();
        if (!Objects.equals(desiredChecksum, checksum)) {
            throw new DomainException(ERR_PACK_CHECKSUM_MISMATCH, "Applied checksum does not match desired");
        }
        if (state.desiredGeneration() == 0L) {
            return; // never mutated — nothing durable to record
        }
        if (state.appliedGeneration() == generation && Objects.equals(state.appliedChecksum(), checksum)) {
            return; // idempotent repeat
        }

        Instant now = Instant.now();
        int updated = assignmentRepository.updateAppliedState(
                tenantId, sensorId, generation, checksum, now, "sensor:" + sensorId, now);
        if (updated != 1) {
            throw new DomainException(ERR_PACK_STATE_STALE, "Applied state could not be recorded");
        }
    }

    /** Used by activation fence — true when any Sensor still references this pack_id. */
    public boolean hasSensorAssignments(UUID tenantId, String packId) {
        return assignmentRepository.hasAssignmentsForPack(tenantId, packId);
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

        SensorCapabilityPackState state = assignmentRepository.findStateForUpdate(tenantId, sensorId)
                .orElse(SensorCapabilityPackState.neverMutated(tenantId, sensorId));
        // No state row ⇒ generation 0 for CAS.
        if (state.desiredGeneration() != expectedGeneration) {
            throw new DomainException(ERR_GENERATION_CONFLICT, "Sensor capability pack generation conflict");
        }
        return new MutationContext(state);
    }

    private SensorCapabilityPackState recomputeAndPersistDesired(
            UUID tenantId,
            String sensorId,
            MutationContext ctx,
            String actor,
            Instant now,
            boolean increment
    ) {
        List<SensorPackDesiredChecksum.AssignmentDigest> digests =
                assignmentRepository.listAssignmentDigests(tenantId, sensorId);
        if (digests.size() > CapabilityPackRegistry.MAX_PACK_COUNT) {
            throw new DomainException(
                    "CAPABILITY_PACK_SNAPSHOT_INVALID",
                    "Assignment count exceeds max " + CapabilityPackRegistry.MAX_PACK_COUNT);
        }
        String checksum = SensorPackDesiredChecksum.compute(digests);
        long nextGen = increment ? ctx.state().desiredGeneration() + 1L : ctx.state().desiredGeneration();
        assignmentRepository.upsertDesiredState(tenantId, sensorId, nextGen, checksum, actor, now);
        return new SensorCapabilityPackState(
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

    private void requireActivePackVersion(UUID tenantId, String packId, String packVersion) {
        Optional<CapabilityPackActivation> active = packRepository.findActiveActivation(tenantId, packId);
        if (active.isEmpty() || !active.get().packVersion().equals(packVersion)) {
            throw new DomainException(ERR_VERSION_NOT_ACTIVE, "Pack version is not the ACTIVE activation");
        }
        // Ensure artifact exists (FK would also catch, but fail with clear code).
        packRepository.findPack(tenantId, packId, packVersion)
                .orElseThrow(() -> new DomainException(
                        CapabilityPackCatalogService.ERR_NOT_FOUND, "Capability pack version not found"));
    }

    private AssignmentView snapshot(UUID tenantId, String sensorId, SensorCapabilityPackState state) {
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

    private record MutationContext(SensorCapabilityPackState state) {
    }
}
