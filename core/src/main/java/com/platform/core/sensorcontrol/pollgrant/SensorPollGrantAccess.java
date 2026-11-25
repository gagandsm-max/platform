package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.ingestion.authority.ResolvedSensorAuthority;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.sensorcontrol.SensorCertificateCredential;
import com.platform.core.sensorcontrol.SensorControlException;
import com.platform.core.sensorcontrol.SensorControlRepository;
import com.platform.core.sensorcontrol.SensorCredentialStatus;
import com.platform.core.sensorcontrol.SensorLifecycleStatus;
import com.platform.core.sensorcontrol.SensorPrincipal;

import java.time.Clock;
import java.time.Instant;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;

import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.AddressType;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Protocol;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Status;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrantEligibility.Decision;

/**
 * Read-only validated lookup / new-execution eligibility accessor for Core-issued
 * poll grants (Issue #229 G1b producer/lookup slice).
 *
 * <p><b>What this is:</b> an internal, separately testable component that loads a
 * persisted V015 grant under a caller-held PostgreSQL transaction, re-holds
 * authenticated Sensor principal + matched credential fences, and evaluates
 * tenant/Sensor/entity/target/generation/window/revocation eligibility using
 * Core time and the immutable issued snapshot.
 *
 * <p><b>What this is not:</b> a public API, companion mTLS route, scheduler, second
 * issuer, queue, typed-admission path, cryptographic signer, or G1B_RUNTIME
 * activation. It does not trust request-supplied issuance proof fields as
 * authority. It does not invent slot/cadence/attempt, incarnation binding,
 * delivery cursor/receipt, offline grace, or historical capture proof — those
 * remain explicit remaining authoritative linkages outside this allowlist.
 *
 * <p><b>Lock order</b> (caller-held Spring JDBC transaction; no replacement TX):
 * <ol>
 *   <li>{@code platform.sensor} principal {@code FOR SHARE}</li>
 *   <li>{@code platform.sensor_certificate_credential} matched credential
 *       {@code FOR SHARE}</li>
 *   <li>{@code platform.sensor_poll_grant} row {@code FOR SHARE}</li>
 * </ol>
 * Non-{@code ACTIVE} credential status is rejected after principal + credential
 * identity match and <em>before</em> any grant lookup, so grant existence is not
 * disclosed by decision branching. When status is {@code ACTIVE}, Core evaluation
 * time is sampled after {@code findForShare} returns (present or absent) and
 * credential inclusive time bounds are revalidated before the empty-branch
 * decision, so a credential that expires while waiting for the grant lock is
 * denied uniformly without a grant-existence oracle. This is logical denial
 * nondisclosure only — not a claim of constant-time or complete timing-side-channel
 * elimination. Incarnation SHARE is intentionally omitted here: V015 grants carry
 * no {@code incarnation_id}, so locking an unrelated ACTIVE incarnation would fake
 * grant↔incarnation authority.
 */
public final class SensorPollGrantAccess {

    private final SensorControlRepository sensors;
    private final SensorPollGrantRepository grants;
    private final Clock clock;

    public SensorPollGrantAccess(
            SensorControlRepository sensors,
            SensorPollGrantRepository grants
    ) {
        this(sensors, grants, Clock.systemUTC());
    }

    /**
     * Package-local Clock seam for deterministic tests only. Does not introduce
     * grant TTL, offline horizon or issuance policy.
     */
    SensorPollGrantAccess(
            SensorControlRepository sensors,
            SensorPollGrantRepository grants,
            Clock clock
    ) {
        this.sensors = Objects.requireNonNull(sensors, "sensors");
        this.grants = Objects.requireNonNull(grants, "grants");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    /**
     * Evaluate whether a persisted grant may authorize <em>new</em> execution work
     * for the authenticated Sensor at Core evaluation time.
     *
     * <p>{@code authority} must already be server-derived (for example from
     * {@code PostgresSensorAuthorityResolver}); this method never accepts a raw
     * client certificate or client-claimed tenant/Sensor identity as proof.
     * Optional {@code expected} is a Core-side consistency filter against the
     * persisted immutable snapshot — never treat Sensor-supplied values as
     * issuance proof by passing them here.
     */
    public SensorPollGrantEligibility evaluateForNewExecution(LookupRequest request) {
        if (request == null) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                    "Poll grant lookup request is invalid");
        }
        ResolvedSensorAuthority authority = requireAuthority(request.authority());
        UUID grantId = requireGrantId(request.grantId());
        ExpectedBinding expected = request.expected();

        grants.requireHeldCallerTransaction("evaluateForNewExecution");

        SensorScope scope = authority.scope();
        SensorPrincipal principal = sensors.lockPrincipalForShare(scope.tenantId(), scope.sensorId());
        if (!samePrincipal(principal, scope)) {
            return SensorPollGrantEligibility.denied(Decision.NOT_VISIBLE);
        }
        if (principal.status() != SensorLifecycleStatus.ACTIVE) {
            return SensorPollGrantEligibility.denied(Decision.PRINCIPAL_INACTIVE);
        }

        SensorCertificateCredential credential =
                sensors.lockCredentialForShare(authority.credentialId());
        if (!credentialMatchesAuthority(credential, authority, principal)) {
            return SensorPollGrantEligibility.denied(Decision.CREDENTIAL_INACTIVE);
        }
        // Status fence before grant lookup: REVOKED/SUPERSEDED must not oracle existence.
        if (credential.status() != SensorCredentialStatus.ACTIVE) {
            return SensorPollGrantEligibility.denied(Decision.CREDENTIAL_INACTIVE);
        }

        Optional<SensorPollGrant> locked =
                grants.findForShare(scope.tenantId(), scope.sensorId(), grantId);

        Instant evaluatedAt = sampleEvaluationInstant();
        // Post-lock credential revalidation (status + inclusive notBefore/notAfter)
        // before empty branching — no grant-existence decision on invalid credential.
        if (!credential.isValidAt(evaluatedAt)) {
            return SensorPollGrantEligibility.denied(Decision.CREDENTIAL_INACTIVE);
        }

        if (locked.isEmpty()) {
            return SensorPollGrantEligibility.denied(Decision.NOT_VISIBLE);
        }
        SensorPollGrant grant = locked.get();

        if (!entityAuthorized(principal, scope, grant.entityId())) {
            return SensorPollGrantEligibility.denied(Decision.ENTITY_OUT_OF_SCOPE);
        }
        if (grant.status() != Status.ISSUED) {
            return SensorPollGrantEligibility.denied(Decision.NOT_ISSUED);
        }
        if (!windowActive(grant, evaluatedAt)) {
            return SensorPollGrantEligibility.denied(Decision.WINDOW_INACTIVE);
        }
        if (expected != null && !expected.matches(grant)) {
            return SensorPollGrantEligibility.denied(Decision.BINDING_MISMATCH);
        }
        return SensorPollGrantEligibility.eligible(grant, evaluatedAt);
    }

    private Instant sampleEvaluationInstant() {
        final Instant now;
        try {
            now = clock.instant();
        } catch (RuntimeException ex) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_STATE_INVALID,
                    "Poll grant evaluation time is unavailable");
        }
        if (now == null) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_STATE_INVALID,
                    "Poll grant evaluation time is unavailable");
        }
        return now;
    }

    /**
     * New-execution entity eligibility is the intersection of (a) the
     * server-derived {@link SensorScope} snapshot and (b) live principal
     * grant authority under the held caller transaction. A stale scope hit
     * alone is not authority; live additions beyond the snapshot do not widen
     * (reauthentication may be required). Cache/scope is never sole authority.
     */
    private boolean entityAuthorized(SensorPrincipal principal, SensorScope scope, UUID entityId) {
        if (entityId == null || !scope.isEntityAllowed(entityId)) {
            // Fail closed on missing/out-of-snapshot resource; do not widen from live grants.
            return false;
        }
        if (principal.allowAllTenantEntities()) {
            return true;
        }
        Set<UUID> liveAllowed = sensors.listAllowedEntityIds(principal.tenantId(), principal.sensorId());
        return liveAllowed.contains(entityId);
    }

    private static boolean windowActive(SensorPollGrant grant, Instant now) {
        // Inclusive bounds match V015 chk_spg_window_contains_issued; no invented grace.
        return !now.isBefore(grant.windowStart()) && !now.isAfter(grant.windowEnd());
    }

    private static boolean samePrincipal(SensorPrincipal principal, SensorScope scope) {
        return principal.tenantId().equals(scope.tenantId())
                && principal.sensorId().equals(scope.sensorId());
    }

    private static boolean credentialMatchesAuthority(
            SensorCertificateCredential credential,
            ResolvedSensorAuthority authority,
            SensorPrincipal principal
    ) {
        return credential.credentialId().equals(authority.credentialId())
                && credential.fingerprintSha256().equals(authority.fingerprintSha256())
                && credential.tenantId().equals(principal.tenantId())
                && credential.sensorId().equals(principal.sensorId());
    }

    private static ResolvedSensorAuthority requireAuthority(ResolvedSensorAuthority authority) {
        if (authority == null) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                    "Poll grant lookup request is invalid");
        }
        return authority;
    }

    private static UUID requireGrantId(UUID grantId) {
        if (grantId == null) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                    "Poll grant lookup request is invalid");
        }
        return grantId;
    }

    /**
     * Authenticated lookup binding. {@code authority} is server-derived Sensor
     * identity; {@code grantId} selects the persisted ledger row under that scope.
     * {@code expected} is optional Core-side snapshot consistency only.
     */
    public record LookupRequest(
            ResolvedSensorAuthority authority,
            UUID grantId,
            ExpectedBinding expected
    ) {
        public LookupRequest(ResolvedSensorAuthority authority, UUID grantId) {
            this(authority, grantId, null);
        }
    }

    /**
     * Optional Core-owned expected immutable issuance snapshot. Any non-null field
     * must equal the persisted grant; null fields are not checked. Passing
     * Sensor-claimed values here does not create issuance authority.
     */
    public record ExpectedBinding(
            UUID entityId,
            UUID endpointId,
            Protocol protocol,
            Long grantGeneration,
            Long assignmentGeneration,
            String profilePlanId,
            String profilePlanVersion,
            AddressType issuedAddressType,
            String issuedAddress
    ) {
        boolean matches(SensorPollGrant grant) {
            if (entityId != null && !entityId.equals(grant.entityId())) {
                return false;
            }
            if (endpointId != null && !endpointId.equals(grant.endpointId())) {
                return false;
            }
            if (protocol != null && protocol != grant.protocol()) {
                return false;
            }
            if (grantGeneration != null && grantGeneration != grant.grantGeneration()) {
                return false;
            }
            if (assignmentGeneration != null
                    && assignmentGeneration != grant.assignmentGeneration()) {
                return false;
            }
            if (profilePlanId != null && !profilePlanId.equals(grant.profilePlanId())) {
                return false;
            }
            if (profilePlanVersion != null
                    && !profilePlanVersion.equals(grant.profilePlanVersion())) {
                return false;
            }
            if (issuedAddressType != null && issuedAddressType != grant.issuedAddressType()) {
                return false;
            }
            if (issuedAddress != null && !issuedAddress.equals(grant.issuedAddress())) {
                return false;
            }
            return true;
        }
    }
}
