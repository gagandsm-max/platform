package com.platform.core.agentcontrol;

import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.SensorScope;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;

import java.time.Clock;
import java.time.Instant;
import java.util.Objects;

/**
 * Sensor-facing Agent authority resolution.
 * Calling Sensor must already be authenticated via {@link com.platform.core.ingestion.authority.SensorAuthorityResolver}.
 * Returns only bounded Agent facts for the Sensor that holds the ACTIVE assignment.
 */
@Service
public class AgentAuthorityQueryService {

    private static final Logger log = LoggerFactory.getLogger(AgentAuthorityQueryService.class);

    private final AgentControlRepository repository;
    private final AgentAuthorityMetrics metrics;
    private final Clock clock;

    /**
     * Production constructor. Clock is fixed to UTC — do not inject a Clock bean here
     * (multiple constructors confuse Spring unless this path is explicitly selected).
     */
    @Autowired
    public AgentAuthorityQueryService(AgentControlRepository repository, AgentAuthorityMetrics metrics) {
        this(repository, metrics, Clock.systemUTC());
    }

    /** Test-visible constructor for fixed-clock authority window checks. */
    public AgentAuthorityQueryService(
            AgentControlRepository repository,
            AgentAuthorityMetrics metrics,
            Clock clock
    ) {
        this.repository = Objects.requireNonNull(repository, "repository");
        this.metrics = Objects.requireNonNull(metrics, "metrics");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    /**
     * Resolve Agent authority for a calling Sensor by Agent certificate fingerprint.
     * Sensor ID and tenant ID comparisons use exact equality (case-sensitive for IDs).
     * Omits assignment-generation claim check (lookup returns current fence for Sensor to store).
     */
    public AgentAuthorityView resolveForSensor(SensorScope callingSensor, String agentFingerprintSha256) {
        return resolveForSensor(callingSensor, agentFingerprintSha256, null);
    }

    /**
     * Resolve Agent authority for a calling Sensor by Agent certificate fingerprint.
     * When {@code claimedGeneration} is non-null it must exactly equal the ACTIVE assignment
     * generation; otherwise resolution fails with {@code ERR_AGENT_ASSIGNMENT_GENERATION_MISMATCH}.
     * When null, returns the current fence without a claim check (Sensor bootstrap / refresh lookup).
     */
    public AgentAuthorityView resolveForSensor(
            SensorScope callingSensor,
            String agentFingerprintSha256,
            Long claimedGeneration
    ) {
        Objects.requireNonNull(callingSensor, "callingSensor");
        long started = System.currentTimeMillis();
        if (agentFingerprintSha256 == null || !agentFingerprintSha256.matches("^[a-f0-9]{64}$")) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_UNKNOWN_FINGERPRINT,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_CREDENTIAL_UNKNOWN",
                    "Agent credential fingerprint is missing or malformed");
        }

        try {
            return resolveInternal(callingSensor, agentFingerprintSha256, claimedGeneration, started);
        } catch (IngestionException e) {
            throw e;
        } catch (RuntimeException e) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_DB_ERROR, System.currentTimeMillis() - started);
            log.warn("Agent authority resolution failed closed due to persistence error");
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_AUTHORITY_UNAVAILABLE",
                    "Authoritative Agent Control resolution failed; Sensor Agent lookup is fail-closed", e);
        }
    }

    private AgentAuthorityView resolveInternal(
            SensorScope callingSensor,
            String fingerprint,
            Long claimedGeneration,
            long started
    ) {
        Instant now = clock.instant();
        var credentialOpt = repository.findCredentialByFingerprint(fingerprint);
        if (credentialOpt.isEmpty()) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_UNKNOWN_FINGERPRINT,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_CREDENTIAL_UNKNOWN",
                    "No enrolled Agent credential matches the fingerprint");
        }

        AgentCertificateCredential credential = credentialOpt.get();
        if (credential.status() == AgentCredentialStatus.REVOKED) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_CREDENTIAL_REVOKED,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_CREDENTIAL_REVOKED",
                    "Agent credential is revoked");
        }
        if (credential.status() == AgentCredentialStatus.SUPERSEDED) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_CREDENTIAL_SUPERSEDED,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_CREDENTIAL_REVOKED",
                    "Agent credential has been superseded");
        }
        if (credential.status() != AgentCredentialStatus.ACTIVE) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_CREDENTIAL_REVOKED,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_CREDENTIAL_REVOKED",
                    "Agent credential is not active");
        }
        if (now.isBefore(credential.notBefore())) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_CREDENTIAL_NOT_YET_VALID,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_CREDENTIAL_EXPIRED",
                    "Agent credential is not yet valid");
        }
        if (now.isAfter(credential.notAfter())) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_CREDENTIAL_EXPIRED,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_CREDENTIAL_EXPIRED",
                    "Agent credential is outside its validity window");
        }

        AgentPrincipal principal = repository.findPrincipalByTenantAndId(credential.tenantId(), credential.agentId())
                .orElse(null);
        if (principal == null) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_AUTHORITY_UNAVAILABLE,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_AUTHORITY_UNAVAILABLE",
                    "Agent principal is missing for enrolled credential");
        }
        if (principal.status() == AgentLifecycleStatus.DISABLED) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_AGENT_DISABLED,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_DISABLED",
                    "Agent is disabled");
        }
        if (principal.status() == AgentLifecycleStatus.REVOKED) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_AGENT_REVOKED,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_DISABLED",
                    "Agent is revoked");
        }
        if (principal.status() != AgentLifecycleStatus.ACTIVE) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_AGENT_DISABLED,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_DISABLED",
                    "Agent is not active");
        }

        AgentSensorAssignment assignment = repository
                .findActiveAssignmentForAgent(principal.tenantId(), principal.agentId())
                .orElse(null);
        if (assignment == null) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_ASSIGNMENT_MISSING,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_ASSIGNMENT_MISSING",
                    "Agent has no active Sensor assignment");
        }

        // Exact equals — case-sensitive Sensor ID claim check.
        if (!assignment.sensorId().equals(callingSensor.sensorId())) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_SENSOR_MISMATCH,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH",
                    "Agent is not assigned to the calling Sensor");
        }
        if (!assignment.tenantId().equals(callingSensor.tenantId())) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_TENANT_MISMATCH,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH",
                    "Agent assignment tenant does not match calling Sensor");
        }

        // Optional Sensor-cached generation claim: exact match against ACTIVE fence only.
        if (claimedGeneration != null && claimedGeneration.longValue() != assignment.generation()) {
            metrics.recordRejection(AgentAuthorityMetrics.REASON_STALE_GENERATION,
                    System.currentTimeMillis() - started);
            throw new IngestionException(HttpStatus.FORBIDDEN, "ERR_AGENT_ASSIGNMENT_GENERATION_MISMATCH",
                    "Sensor-claimed assignment generation does not match ACTIVE fence");
        }

        metrics.recordResolved(System.currentTimeMillis() - started);
        return new AgentAuthorityView(
                principal.agentId(),
                principal.tenantId(),
                principal.siteId(),
                assignment.generation(),
                assignment.sensorId()
        );
    }
}
