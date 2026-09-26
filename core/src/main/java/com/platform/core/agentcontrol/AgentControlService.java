package com.platform.core.agentcontrol;

import com.platform.core.control.ControlGenerationLimits;
import com.platform.core.id.UuidV7;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.sensorcontrol.CertificateFingerprints;
import com.platform.core.sensorcontrol.SensorCapability;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.sensorcontrol.SensorLifecycleStatus;
import com.platform.core.sensorcontrol.SensorPrincipal;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;

/**
 * Internal Agent Control operations (enrollment, credential bind/rotate/revoke, disable/revoke, Sensor assignment).
 * No public customer REST controllers — callers are Core control-plane components only.
 *
 * <p>Site compatibility for assignment: if both {@code agent.siteId} and {@code sensor.siteId} are non-null,
 * they must be equal; if either is null, assignment is allowed.
 */
@Service
public class AgentControlService {

    private final AgentControlRepository repository;
    private final SensorControlRepository sensorRepository;
    private final SecurityAuditEmitter auditEmitter;
    private final AssignmentMetrics assignmentMetrics;

    public AgentControlService(
            AgentControlRepository repository,
            SensorControlRepository sensorRepository,
            SecurityAuditEmitter auditEmitter,
            AssignmentMetrics assignmentMetrics
    ) {
        this.repository = Objects.requireNonNull(repository, "repository");
        this.sensorRepository = Objects.requireNonNull(sensorRepository, "sensorRepository");
        this.auditEmitter = Objects.requireNonNull(auditEmitter, "auditEmitter");
        this.assignmentMetrics = Objects.requireNonNull(assignmentMetrics, "assignmentMetrics");
    }

    @Transactional("transactionManager")
    public AgentPrincipal enrollAgent(String agentId, UUID tenantId, UUID siteId) {
        String id = requireAgentId(agentId);
        Objects.requireNonNull(tenantId, "tenantId");

        if (!repository.tenantExists(tenantId)) {
            throw new AgentControlException("TENANT_NOT_FOUND", "Tenant not found");
        }
        if (siteId != null && !repository.siteExists(tenantId, siteId)) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_SITE_INVALID,
                    "Site does not exist for tenant");
        }
        if (repository.findPrincipalById(id).isPresent()) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_ALREADY_EXISTS,
                    "Agent already enrolled");
        }

        Instant now = Instant.now();
        AgentPrincipal principal = new AgentPrincipal(
                id, tenantId, siteId, AgentLifecycleStatus.ACTIVE, now, now
        );
        try {
            repository.insertPrincipal(principal);
        } catch (DuplicateKeyException e) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_ALREADY_EXISTS,
                    "Agent already enrolled");
        }

        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.AGENT_ENROLL,
                AuditOutcome.SUCCESS,
                tenantId,
                "AGENT_ENROLLED",
                Map.of("agent_id", id)
        );
        return principal;
    }

    @Transactional("transactionManager")
    public AgentCertificateCredential bindCertificate(String agentId, X509Certificate certificate) {
        Objects.requireNonNull(certificate, "certificate");
        AgentPrincipal principal = requirePrincipal(agentId);
        Instant now = Instant.now();
        Instant notBefore = certificate.getNotBefore().toInstant();
        Instant notAfter = certificate.getNotAfter().toInstant();
        if (!notAfter.isAfter(notBefore)) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_CREDENTIAL_INVALID,
                    "Certificate validity window is invalid");
        }

        String fingerprint = CertificateFingerprints.sha256DerHex(certificate);
        AgentCertificateCredential credential = buildCredential(
                principal,
                fingerprint,
                CertificateFingerprints.spkiSha256Hex(certificate),
                certificate.getSerialNumber().toString(16),
                certificate.getIssuerX500Principal().getName(),
                null,
                notBefore,
                notAfter,
                now
        );
        repository.insertCredential(credential);
        auditEmitter.emitControlEvent(
                AuditCategory.CREDENTIAL,
                AuditActions.AGENT_CREDENTIAL_BIND,
                AuditOutcome.SUCCESS,
                principal.tenantId(),
                "AGENT_CREDENTIAL_BOUND",
                Map.of("agent_id", principal.agentId(), "fingerprint_sha256", fingerprint)
        );
        return credential;
    }

    /**
     * Rotate certificate for the same agent. Supersedes prior ACTIVE credentials then inserts the new binding.
     * On insert failure the whole transaction rolls back so prior credentials remain ACTIVE.
     * Tenant and site of the principal are never changed by rotation.
     */
    @Transactional("transactionManager")
    public AgentCertificateCredential rotateCertificate(String agentId, X509Certificate newCertificate) {
        AgentPrincipal principal = requirePrincipal(agentId);
        Objects.requireNonNull(newCertificate, "newCertificate");
        Instant now = Instant.now();
        Instant notBefore = newCertificate.getNotBefore().toInstant();
        Instant notAfter = newCertificate.getNotAfter().toInstant();
        if (!notAfter.isAfter(notBefore)) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_CREDENTIAL_INVALID,
                    "Certificate validity window is invalid");
        }

        List<AgentCertificateCredential> active =
                repository.listActiveCredentialsForAgent(principal.tenantId(), principal.agentId());
        for (AgentCertificateCredential existing : active) {
            repository.updateCredentialStatus(
                    existing.credentialId(), AgentCredentialStatus.SUPERSEDED, now, now);
        }

        String fingerprint = CertificateFingerprints.sha256DerHex(newCertificate);
        AgentCertificateCredential bound = buildCredential(
                principal,
                fingerprint,
                CertificateFingerprints.spkiSha256Hex(newCertificate),
                newCertificate.getSerialNumber().toString(16),
                newCertificate.getIssuerX500Principal().getName(),
                null,
                notBefore,
                notAfter,
                now
        );
        repository.insertCredential(bound);
        auditEmitter.emitControlEvent(
                AuditCategory.CREDENTIAL,
                AuditActions.AGENT_CREDENTIAL_ROTATE,
                AuditOutcome.SUCCESS,
                principal.tenantId(),
                "AGENT_CREDENTIAL_ROTATED",
                Map.of(
                        "agent_id", principal.agentId(),
                        "fingerprint_sha256", fingerprint,
                        "superseded_count", String.valueOf(active.size())
                )
        );
        return bound;
    }

    @Transactional("transactionManager")
    public void revokeCredential(String fingerprintSha256) {
        Objects.requireNonNull(fingerprintSha256, "fingerprintSha256");
        AgentCertificateCredential credential = repository.findCredentialByFingerprint(fingerprintSha256)
                .orElseThrow(() -> new AgentControlException(
                        AgentControlException.ERR_AGENT_CREDENTIAL_NOT_FOUND,
                        "Credential fingerprint not found"));
        Instant now = Instant.now();
        repository.updateCredentialStatus(
                credential.credentialId(), AgentCredentialStatus.REVOKED, now, now);
        auditEmitter.emitControlEvent(
                AuditCategory.CREDENTIAL,
                AuditActions.AGENT_CREDENTIAL_REVOKE,
                AuditOutcome.SUCCESS,
                credential.tenantId(),
                "AGENT_CREDENTIAL_REVOKED",
                Map.of("agent_id", credential.agentId(), "fingerprint_sha256", fingerprintSha256)
        );
    }

    @Transactional("transactionManager")
    public AgentPrincipal disableAgent(String agentId) {
        AgentPrincipal principal = requirePrincipal(agentId);
        Instant now = Instant.now();
        repository.updatePrincipalStatus(principal.agentId(), AgentLifecycleStatus.DISABLED, now);
        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.AGENT_DISABLE,
                AuditOutcome.SUCCESS,
                principal.tenantId(),
                "AGENT_DISABLED",
                Map.of("agent_id", principal.agentId())
        );
        return repository.findPrincipalById(principal.agentId()).orElseThrow();
    }

    @Transactional("transactionManager")
    public AgentPrincipal revokeAgent(String agentId) {
        AgentPrincipal principal = requirePrincipal(agentId);
        Instant now = Instant.now();
        repository.updatePrincipalStatus(principal.agentId(), AgentLifecycleStatus.REVOKED, now);
        List<AgentCertificateCredential> active =
                repository.listActiveCredentialsForAgent(principal.tenantId(), principal.agentId());
        for (AgentCertificateCredential existing : active) {
            repository.updateCredentialStatus(
                    existing.credentialId(), AgentCredentialStatus.REVOKED, now, now);
        }
        Optional<AgentSensorAssignment> assignment =
                repository.findActiveAssignmentForAgent(principal.tenantId(), principal.agentId());
        assignment.ifPresent(a ->
                repository.revokeAssignmentById(a.assignmentId(), now, now));
        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                AuditActions.AGENT_REVOKE,
                AuditOutcome.SUCCESS,
                principal.tenantId(),
                "AGENT_REVOKED",
                Map.of("agent_id", principal.agentId())
        );
        return repository.findPrincipalById(principal.agentId()).orElseThrow();
    }

    /**
     * Assign Agent to Sensor. Revokes any prior ACTIVE assignment under row lock and inserts
     * generation = prior+1 when ACTIVE exists, else max(historical generation)+1 (never resets to 1
     * while historical rows exist; starts at 1 only when the agent has never been assigned).
     *
     * <p>Site compatibility: if agent.siteId and sensor.siteId are both non-null they must equal;
     * if either is null, allow.
     */
    @Transactional("transactionManager")
    public AgentSensorAssignment assignToSensor(UUID tenantId, String agentId, String sensorId) {
        long started = System.currentTimeMillis();
        try {
            AgentPrincipal agent = requireActiveAgent(tenantId, agentId);
            SensorPrincipal sensor = requireEligibleSensor(tenantId, sensorId);
            assertSiteCompatible(agent, sensor);

            Instant now = Instant.now();
            Optional<AgentSensorAssignment> prior =
                    repository.findActiveAssignmentForAgentForUpdate(tenantId, agent.agentId());
            long nextGeneration;
            if (prior.isPresent()) {
                // Fail closed before revoke so an exhausted ACTIVE assignment remains ACTIVE.
                nextGeneration = ControlGenerationLimits.nextOrThrow(
                        prior.get().generation(),
                        new AgentControlException(
                                AgentControlException.ERR_CONTROL_GENERATION_EXHAUSTED,
                                ControlGenerationLimits.EXHAUSTED_MESSAGE));
                repository.revokeAssignmentById(prior.get().assignmentId(), now, now);
            } else {
                // No ACTIVE row: serialize on agent principal, then advance past any historical fence.
                repository.lockPrincipalForUpdate(tenantId, agent.agentId());
                long historicalMax = repository.maxGenerationForAgent(tenantId, agent.agentId());
                nextGeneration = ControlGenerationLimits.nextOrThrow(
                        historicalMax,
                        new AgentControlException(
                                AgentControlException.ERR_CONTROL_GENERATION_EXHAUSTED,
                                ControlGenerationLimits.EXHAUSTED_MESSAGE));
            }

            AgentSensorAssignment created = new AgentSensorAssignment(
                    UuidV7.generate(),
                    tenantId,
                    agent.agentId(),
                    sensor.sensorId(),
                    nextGeneration,
                    AgentAssignmentStatus.ACTIVE,
                    now,
                    now,
                    null
            );
            repository.insertAssignment(created);
            auditEmitter.emitControlEvent(
                    AuditCategory.SECURITY_ADMINISTRATION,
                    AuditActions.AGENT_ASSIGN,
                    AuditOutcome.SUCCESS,
                    tenantId,
                    "AGENT_ASSIGNED",
                    Map.of(
                            "agent_id", agent.agentId(),
                            "sensor_id", sensor.sensorId(),
                            "generation", String.valueOf(nextGeneration)
                    )
            );
            assignmentMetrics.recordSuccess(AssignmentMetrics.REASON_ASSIGNED, System.currentTimeMillis() - started);
            return created;
        } catch (AgentControlException e) {
            assignmentMetrics.recordFailure(mapAssignmentFailure(e), System.currentTimeMillis() - started);
            throw e;
        }
    }

    /**
     * Reassign using optimistic CAS on {@code expectedGeneration}. Stale generation →
     * {@link AgentControlException#ERR_AGENT_ASSIGNMENT_CONFLICT}.
     */
    @Transactional("transactionManager")
    public AgentSensorAssignment reassignToSensor(
            UUID tenantId,
            String agentId,
            String sensorId,
            long expectedGeneration
    ) {
        long started = System.currentTimeMillis();
        try {
            if (expectedGeneration < 1) {
                throw new AgentControlException(AgentControlException.ERR_AGENT_STATE_INVALID,
                        "expectedGeneration must be >= 1");
            }
            if (!ControlGenerationLimits.isPositiveJsonSafe(expectedGeneration)) {
                throw new AgentControlException(AgentControlException.ERR_AGENT_STATE_INVALID,
                        "expectedGeneration exceeds MAX_SAFE_CONTROL_GENERATION");
            }
            AgentPrincipal agent = requireActiveAgent(tenantId, agentId);
            SensorPrincipal sensor = requireEligibleSensor(tenantId, sensorId);
            assertSiteCompatible(agent, sensor);

            Instant now = Instant.now();
            // Authoritative ACTIVE generation must match CAS claim before exhaustion is evaluated.
            Optional<AgentSensorAssignment> active =
                    repository.findActiveAssignmentForAgentForUpdate(tenantId, agent.agentId());
            if (active.isEmpty() || active.get().generation() != expectedGeneration) {
                throw new AgentControlException(
                        AgentControlException.ERR_AGENT_ASSIGNMENT_CONFLICT,
                        "Active assignment generation conflict");
            }
            long authoritativeGeneration = active.get().generation();
            long nextGeneration = ControlGenerationLimits.nextOrThrow(
                    authoritativeGeneration,
                    new AgentControlException(
                            AgentControlException.ERR_CONTROL_GENERATION_EXHAUSTED,
                            ControlGenerationLimits.EXHAUSTED_MESSAGE));
            int revoked = repository.revokeActiveAssignmentCas(
                    tenantId, agent.agentId(), expectedGeneration, now, now);
            if (revoked != 1) {
                throw new AgentControlException(
                        AgentControlException.ERR_AGENT_ASSIGNMENT_CONFLICT,
                        "Active assignment generation conflict");
            }

            AgentSensorAssignment created = new AgentSensorAssignment(
                    UuidV7.generate(),
                    tenantId,
                    agent.agentId(),
                    sensor.sensorId(),
                    nextGeneration,
                    AgentAssignmentStatus.ACTIVE,
                    now,
                    now,
                    null
            );
            repository.insertAssignment(created);
            auditEmitter.emitControlEvent(
                    AuditCategory.SECURITY_ADMINISTRATION,
                    AuditActions.AGENT_REASSIGN,
                    AuditOutcome.SUCCESS,
                    tenantId,
                    "AGENT_REASSIGNED",
                    Map.of(
                            "agent_id", agent.agentId(),
                            "sensor_id", sensor.sensorId(),
                            "generation", String.valueOf(nextGeneration),
                            "expected_generation", String.valueOf(expectedGeneration)
                    )
            );
            assignmentMetrics.recordSuccess(AssignmentMetrics.REASON_REASSIGNED, System.currentTimeMillis() - started);
            return created;
        } catch (AgentControlException e) {
            assignmentMetrics.recordFailure(mapAssignmentFailure(e), System.currentTimeMillis() - started);
            throw e;
        }
    }

    @Transactional("transactionManager")
    public void revokeAssignment(UUID tenantId, String agentId) {
        long started = System.currentTimeMillis();
        try {
            AgentPrincipal agent = requirePrincipal(agentId);
            if (!agent.tenantId().equals(tenantId)) {
                throw new AgentControlException(AgentControlException.ERR_AGENT_TENANT_MISMATCH,
                        "Agent tenant does not match");
            }
            AgentSensorAssignment active = repository.findActiveAssignmentForAgent(tenantId, agent.agentId())
                    .orElseThrow(() -> new AgentControlException(
                            AgentControlException.ERR_AGENT_ASSIGNMENT_NOT_FOUND,
                            "No active assignment"));
            Instant now = Instant.now();
            repository.revokeAssignmentById(active.assignmentId(), now, now);
            auditEmitter.emitControlEvent(
                    AuditCategory.SECURITY_ADMINISTRATION,
                    AuditActions.AGENT_ASSIGNMENT_REVOKE,
                    AuditOutcome.SUCCESS,
                    tenantId,
                    "AGENT_ASSIGNMENT_REVOKED",
                    Map.of(
                            "agent_id", agent.agentId(),
                            "sensor_id", active.sensorId(),
                            "generation", String.valueOf(active.generation())
                    )
            );
            assignmentMetrics.recordSuccess(AssignmentMetrics.REASON_REVOKED, System.currentTimeMillis() - started);
        } catch (AgentControlException e) {
            assignmentMetrics.recordFailure(mapAssignmentFailure(e), System.currentTimeMillis() - started);
            throw e;
        }
    }

    public AgentPrincipal requirePrincipal(String agentId) {
        return repository.findPrincipalById(requireAgentId(agentId))
                .orElseThrow(() -> new AgentControlException(
                        AgentControlException.ERR_AGENT_NOT_FOUND, "Agent not found"));
    }

    private AgentPrincipal requireActiveAgent(UUID tenantId, String agentId) {
        AgentPrincipal agent = requirePrincipal(agentId);
        if (!agent.tenantId().equals(tenantId)) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_TENANT_MISMATCH,
                    "Agent tenant does not match");
        }
        if (agent.status() != AgentLifecycleStatus.ACTIVE) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_STATE_INVALID,
                    "Agent is not ACTIVE");
        }
        return agent;
    }

    private SensorPrincipal requireEligibleSensor(UUID tenantId, String sensorId) {
        String id = requireSensorId(sensorId);
        SensorPrincipal sensor = sensorRepository.findPrincipalByTenantAndId(tenantId, id)
                .orElseThrow(() -> new AgentControlException(
                        AgentControlException.ERR_SENSOR_NOT_FOUND, "Sensor not found for tenant"));
        if (!sensor.tenantId().equals(tenantId)) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_TENANT_MISMATCH,
                    "Sensor tenant does not match");
        }
        if (sensor.status() != SensorLifecycleStatus.ACTIVE) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_SENSOR_INELIGIBLE,
                    "Sensor is not ACTIVE");
        }
        if (!SensorCapability.roleAllows(sensor.role(), SensorCapability.AGENT_INGRESS)) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_SENSOR_INELIGIBLE,
                    "Sensor role does not allow AGENT_INGRESS");
        }
        return sensor;
    }

    /**
     * Site compatibility: both non-null ⇒ must equal; either null ⇒ allow.
     */
    static void assertSiteCompatible(AgentPrincipal agent, SensorPrincipal sensor) {
        if (agent.siteId() != null && sensor.siteId() != null && !agent.siteId().equals(sensor.siteId())) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_SITE_INCOMPATIBLE,
                    "Agent and Sensor site IDs are incompatible");
        }
    }

    private static String mapAssignmentFailure(AgentControlException e) {
        return switch (e.getCode()) {
            case AgentControlException.ERR_AGENT_ASSIGNMENT_CONFLICT -> AssignmentMetrics.REASON_CONFLICT;
            case AgentControlException.ERR_AGENT_SENSOR_INELIGIBLE -> AssignmentMetrics.REASON_SENSOR_INELIGIBLE;
            case AgentControlException.ERR_AGENT_SITE_INCOMPATIBLE -> AssignmentMetrics.REASON_SITE_INCOMPATIBLE;
            case AgentControlException.ERR_AGENT_TENANT_MISMATCH -> AssignmentMetrics.REASON_TENANT_MISMATCH;
            case AgentControlException.ERR_AGENT_STATE_INVALID -> AssignmentMetrics.REASON_AGENT_NOT_ACTIVE;
            default -> AssignmentMetrics.REASON_ASSIGNMENT_ERROR;
        };
    }

    private static AgentCertificateCredential buildCredential(
            AgentPrincipal principal,
            String fingerprint,
            String spkiSha256,
            String serialNumber,
            String issuerDn,
            String issuerFingerprintSha256,
            Instant notBefore,
            Instant notAfter,
            Instant now
    ) {
        return new AgentCertificateCredential(
                UuidV7.generate(),
                principal.tenantId(),
                principal.agentId(),
                fingerprint,
                spkiSha256,
                serialNumber,
                issuerDn,
                issuerFingerprintSha256,
                notBefore,
                notAfter,
                AgentCredentialStatus.ACTIVE,
                now,
                null,
                now,
                now
        );
    }

    private static String requireAgentId(String agentId) {
        if (agentId == null || agentId.isBlank() || agentId.length() > 128) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_STATE_INVALID,
                    "agentId must be 1..128 characters");
        }
        return agentId.trim();
    }

    private static String requireSensorId(String sensorId) {
        if (sensorId == null || sensorId.isBlank() || sensorId.length() > 128) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_STATE_INVALID,
                    "sensorId must be 1..128 characters");
        }
        return sensorId.trim();
    }
}
