package com.platform.core.sensorcontrol.desiredstate;

import com.platform.core.control.ControlGenerationLimits;
import com.platform.core.domain.DomainException;
import com.platform.core.persistence.SensorControlDesiredStateRepository;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.http.PlatformSecurityContext;
import com.platform.core.security.http.SecurityAuditEmitter;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;
import java.util.regex.Pattern;

/**
 * Core Sensor control desired-state mutation and machine ACK authority (BUILD-004 Slice 2).
 *
 * <p>Lock order: tenant FOR UPDATE → Sensor FOR UPDATE → desired-state row FOR UPDATE.
 */
@Service
public class SensorControlDesiredStateService {

    public static final String ERR_SENSOR_NOT_FOUND = "SENSOR_NOT_FOUND";
    public static final String ERR_SENSOR_NOT_ACTIVE = "SENSOR_NOT_ACTIVE";
    public static final String ERR_GENERATION_CONFLICT = "SENSOR_CONTROL_GENERATION_CONFLICT";
    public static final String ERR_ACK_STALE = "ERR_SENSOR_CONTROL_ACK_STALE";
    public static final String ERR_ACK_FUTURE = "ERR_SENSOR_CONTROL_ACK_FUTURE";
    public static final String ERR_CHECKSUM_MISMATCH = "ERR_SENSOR_CONTROL_CHECKSUM_MISMATCH";
    public static final String ERR_OUTCOME_CONFLICT = "ERR_SENSOR_CONTROL_OUTCOME_CONFLICT";
    public static final String ERR_SCHEMA_UNSUPPORTED = "ERR_SENSOR_CONTROL_SCHEMA_UNSUPPORTED";

    private static final Pattern SHA256_LOWER = Pattern.compile("^[a-f0-9]{64}$");
    private static final Pattern REJECTION_CODE = Pattern.compile("^[A-Za-z0-9._:-]{1,128}$");

    private final SensorControlDesiredStateRepository repository;
    private final SecurityAuditEmitter auditEmitter;

    public SensorControlDesiredStateService(
            SensorControlDesiredStateRepository repository,
            SecurityAuditEmitter auditEmitter
    ) {
        this.repository = Objects.requireNonNull(repository, "repository");
        this.auditEmitter = Objects.requireNonNull(auditEmitter, "auditEmitter");
    }

    @Transactional(value = "transactionManager", readOnly = true)
    public Optional<SensorControlState> findDesiredState(UUID tenantId, String sensorId) {
        requireTenantSensorIds(tenantId, sensorId);
        return repository.findState(tenantId, sensorId);
    }

    @Transactional("transactionManager")
    public SensorControlState setDesiredConfiguration(
            UUID tenantId,
            String sensorId,
            long expectedGeneration,
            SensorRuntimeDesiredConfig configuration
    ) {
        requireTenantSensorIds(tenantId, sensorId);
        if (!ControlGenerationLimits.isJsonSafe(expectedGeneration)) {
            throw new DomainException("VALIDATION_FAILED",
                    "expectedGeneration exceeds MAX_SAFE_CONTROL_GENERATION");
        }
        if (expectedGeneration < 0) {
            throw new DomainException("VALIDATION_FAILED", "expectedGeneration must be >= 0");
        }
        SensorControlDesiredConfigValidator.validate(configuration);
        String checksum = SensorControlDesiredChecksum.compute(configuration);

        if (!repository.lockTenantForUpdate(tenantId)) {
            throw new DomainException("TENANT_NOT_FOUND", "Tenant not found");
        }
        Optional<String> statusBefore = repository.findSensorStatus(tenantId, sensorId);
        if (statusBefore.isEmpty()) {
            throw new DomainException(ERR_SENSOR_NOT_FOUND, "Sensor not found");
        }
        if (!"ACTIVE".equals(statusBefore.get())) {
            throw new DomainException(ERR_SENSOR_NOT_ACTIVE, "Sensor is not ACTIVE");
        }
        Optional<String> lockedStatus = repository.lockSensorForUpdate(tenantId, sensorId);
        if (lockedStatus.isEmpty() || !"ACTIVE".equals(lockedStatus.get())) {
            throw new DomainException(ERR_SENSOR_NOT_ACTIVE, "Sensor is not ACTIVE");
        }

        Optional<SensorControlState> existing = repository.findStateForUpdate(tenantId, sensorId);
        long authoritativeGeneration = existing.map(SensorControlState::desiredGeneration).orElse(0L);
        if (expectedGeneration != authoritativeGeneration) {
            throw new DomainException(ERR_GENERATION_CONFLICT, "Sensor control generation conflict");
        }

        if (existing.isPresent()
                && existing.get().desiredChecksum().equals(checksum)
                && configsEqual(existing.get().desiredConfig(), configuration)) {
            return existing.get();
        }

        long nextGeneration = ControlGenerationLimits.nextOrThrow(
                authoritativeGeneration,
                new DomainException(
                        ControlGenerationLimits.ERR_CONTROL_GENERATION_EXHAUSTED,
                        ControlGenerationLimits.EXHAUSTED_MESSAGE));

        String actor = PlatformSecurityContext.requirePrincipal().subjectId();
        if (actor == null || actor.isBlank() || actor.length() > SensorControlDesiredLimits.MAX_DESIRED_BY_LEN) {
            throw new DomainException("VALIDATION_FAILED", "authenticated subject identity is invalid");
        }
        Instant now = Instant.now();
        SensorControlState next;
        if (existing.isEmpty()) {
            next = new SensorControlState(
                    tenantId, sensorId,
                    SensorControlDesiredLimits.SCHEMA_VERSION,
                    nextGeneration, checksum, configuration, actor, now,
                    0L, null, null,
                    0L, null, null, null, null, null,
                    now
            );
            repository.insertDesiredState(next);
        } else {
            SensorControlState prior = existing.get();
            next = new SensorControlState(
                    tenantId, sensorId,
                    SensorControlDesiredLimits.SCHEMA_VERSION,
                    nextGeneration, checksum, configuration, actor, now,
                    prior.appliedGeneration(), prior.appliedChecksum(), prior.appliedAt(),
                    prior.observedGeneration(), prior.observedChecksum(), prior.observedOutcome(),
                    prior.rejectionCode(), prior.rejectionMessage(), prior.observedAt(),
                    now
            );
            repository.updateDesiredStatePreservingAppliedObserved(next);
        }

        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.SENSOR_CONTROL_DESIRED_SET,
                AuditOutcome.SUCCESS,
                tenantId,
                "SENSOR_CONTROL_DESIRED_SET",
                Map.of(
                        "sensor_id", sensorId,
                        "desired_generation", String.valueOf(nextGeneration),
                        "desired_checksum", checksum
                )
        );
        return next;
    }

    @Transactional("transactionManager")
    public SensorControlState acknowledgeAppliedState(
            UUID tenantId,
            String sensorId,
            int schemaVersion,
            long generation,
            String checksum,
            SensorControlObservationOutcome outcome,
            String rejectionCode,
            String rejectionMessage
    ) {
        requireTenantSensorIds(tenantId, sensorId);
        if (schemaVersion != SensorControlDesiredLimits.SCHEMA_VERSION) {
            throw new DomainException(ERR_SCHEMA_UNSUPPORTED, "Unsupported sensor control schema version");
        }
        if (!ControlGenerationLimits.isPositiveJsonSafe(generation)) {
            throw new DomainException("VALIDATION_FAILED",
                    "generation must be within 1..MAX_SAFE_CONTROL_GENERATION");
        }
        if (checksum == null || !SHA256_LOWER.matcher(checksum).matches()) {
            throw new DomainException("VALIDATION_FAILED", "checksum must be lowercase 64-hex");
        }
        Objects.requireNonNull(outcome, "outcome");
        validateRejectionFields(outcome, rejectionCode, rejectionMessage);

        if (!repository.lockTenantForUpdate(tenantId)) {
            throw new DomainException("TENANT_NOT_FOUND", "Tenant not found");
        }
        Optional<String> lockedStatus = repository.lockSensorForUpdate(tenantId, sensorId);
        if (lockedStatus.isEmpty()) {
            throw new DomainException(ERR_SENSOR_NOT_FOUND, "Sensor not found");
        }
        if (!"ACTIVE".equals(lockedStatus.get())) {
            throw new DomainException(ERR_SENSOR_NOT_ACTIVE, "Sensor is not ACTIVE");
        }

        SensorControlState state = repository.findStateForUpdate(tenantId, sensorId)
                .orElseThrow(() -> new DomainException(ERR_ACK_FUTURE,
                        "No desired control state exists for ACK"));

        if (generation < state.desiredGeneration()) {
            throw new DomainException(ERR_ACK_STALE, "ACK generation is stale relative to desired");
        }
        if (generation > state.desiredGeneration()) {
            throw new DomainException(ERR_ACK_FUTURE, "ACK generation is ahead of desired");
        }
        if (!checksum.equals(state.desiredChecksum())) {
            throw new DomainException(ERR_CHECKSUM_MISMATCH, "ACK checksum does not match desired");
        }

        Instant now = Instant.now();
        if (outcome == SensorControlObservationOutcome.APPLIED) {
            return applyAppliedAck(state, generation, checksum, now);
        }
        return applyRejectedAck(state, generation, checksum, rejectionCode, rejectionMessage, now);
    }

    private SensorControlState applyAppliedAck(
            SensorControlState state,
            long generation,
            String checksum,
            Instant now
    ) {
        if (state.observedGeneration() == generation
                && checksum.equals(state.observedChecksum())
                && state.observedOutcome() == SensorControlObservationOutcome.APPLIED
                && state.appliedGeneration() == generation
                && checksum.equals(state.appliedChecksum())) {
            return state; // exact duplicate APPLIED
        }
        if (state.observedGeneration() == generation
                && checksum.equals(state.observedChecksum())
                && state.observedOutcome() == SensorControlObservationOutcome.REJECTED) {
            // REJECTED -> APPLIED recovery allowed
        } else if (state.appliedGeneration() == generation
                && checksum.equals(state.appliedChecksum())
                && state.observedOutcome() == SensorControlObservationOutcome.APPLIED) {
            return state;
        }

        SensorControlState next = new SensorControlState(
                state.tenantId(), state.sensorId(),
                state.desiredSchemaVersion(), state.desiredGeneration(), state.desiredChecksum(),
                state.desiredConfig(), state.desiredBy(), state.desiredAt(),
                generation, checksum, now,
                generation, checksum, SensorControlObservationOutcome.APPLIED,
                null, null, now,
                now
        );
        if (repository.updateObservation(next) != 1) {
            throw new DomainException(ERR_OUTCOME_CONFLICT, "Applied observation could not be recorded");
        }
        return next;
    }

    private SensorControlState applyRejectedAck(
            SensorControlState state,
            long generation,
            String checksum,
            String rejectionCode,
            String rejectionMessage,
            Instant now
    ) {
        if (state.observedGeneration() == generation
                && checksum.equals(state.observedChecksum())
                && state.observedOutcome() == SensorControlObservationOutcome.REJECTED) {
            if (Objects.equals(state.rejectionCode(), rejectionCode)
                    && Objects.equals(state.rejectionMessage(), rejectionMessage)) {
                return state; // exact duplicate REJECTED
            }
            throw new DomainException(ERR_OUTCOME_CONFLICT,
                    "Conflicting REJECTED observation evidence for the same generation");
        }
        if (state.observedGeneration() > generation) {
            throw new DomainException(ERR_OUTCOME_CONFLICT, "Observation regression is prohibited");
        }

        // APPLIED -> REJECTED for the same desired generation/checksum is permitted:
        // applied* remains historical last-successfully-applied evidence; observed*
        // records the latest Sensor truth (e.g. SENSOR_CONTROL_RESTART_REQUIRED).
        SensorControlState next = new SensorControlState(
                state.tenantId(), state.sensorId(),
                state.desiredSchemaVersion(), state.desiredGeneration(), state.desiredChecksum(),
                state.desiredConfig(), state.desiredBy(), state.desiredAt(),
                state.appliedGeneration(), state.appliedChecksum(), state.appliedAt(),
                generation, checksum, SensorControlObservationOutcome.REJECTED,
                rejectionCode, rejectionMessage, now,
                now
        );
        if (repository.updateObservation(next) != 1) {
            throw new DomainException(ERR_OUTCOME_CONFLICT, "Rejected observation could not be recorded");
        }
        return next;
    }

    private static void validateRejectionFields(
            SensorControlObservationOutcome outcome,
            String rejectionCode,
            String rejectionMessage
    ) {
        if (outcome == SensorControlObservationOutcome.APPLIED) {
            if (rejectionCode != null || rejectionMessage != null) {
                throw new DomainException("VALIDATION_FAILED",
                        "APPLIED outcome must not include rejection fields");
            }
            return;
        }
        if (rejectionCode == null || rejectionCode.isBlank()) {
            throw new DomainException("VALIDATION_FAILED", "REJECTED outcome requires rejectionCode");
        }
        if (rejectionCode.length() > SensorControlDesiredLimits.MAX_REJECTION_CODE_LEN
                || !REJECTION_CODE.matcher(rejectionCode).matches()) {
            throw new DomainException("VALIDATION_FAILED", "rejectionCode is invalid");
        }
        if (rejectionMessage != null
                && rejectionMessage.length() > SensorControlDesiredLimits.MAX_REJECTION_MESSAGE_LEN) {
            throw new DomainException("VALIDATION_FAILED", "rejectionMessage exceeds bound");
        }
    }

    private static boolean configsEqual(SensorRuntimeDesiredConfig a, SensorRuntimeDesiredConfig b) {
        return SensorControlDesiredChecksum.compute(a).equals(SensorControlDesiredChecksum.compute(b));
    }

    private static void requireTenantSensorIds(UUID tenantId, String sensorId) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(sensorId, "sensorId");
        if (sensorId.isBlank() || sensorId.length() > 128) {
            throw new DomainException("VALIDATION_FAILED", "sensorId is invalid");
        }
    }
}
