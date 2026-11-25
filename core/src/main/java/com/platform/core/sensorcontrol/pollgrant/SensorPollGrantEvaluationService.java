package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.ingestion.authority.ResolvedSensorAuthority;
import com.platform.core.sensorcontrol.SensorControlException;
import org.springframework.transaction.annotation.Transactional;

import java.util.Objects;
import java.util.UUID;

/**
 * Spring-managed PostgreSQL read-transaction seam for Issue #229 G1b new-execution
 * eligibility evaluation.
 *
 * <p><b>What this is:</b> an internal Core invocation boundary that opens one
 * {@code transactionManager} PostgreSQL transaction and delegates to
 * {@link SensorPollGrantAccess#evaluateForNewExecution} under that caller-held
 * transaction. Callers supply only a server-derived {@link ResolvedSensorAuthority}
 * and the immutable persisted {@code grantId}. Evaluation performs no grant
 * writes; the transaction is intentionally <em>not</em> marked Spring
 * {@code readOnly} because the accessor requires {@code FOR SHARE} locks, which
 * PostgreSQL rejects under {@code READ ONLY}.
 *
 * <p><b>What this is not:</b> a public API, companion mTLS route, Sensor job
 * scheduler, grant delivery/cursor path, typed G2 admission surface, alternate
 * authn/authz channel, second database, or G1B_RUNTIME activation. It never
 * accepts raw client-claimed tenant/Sensor identity or unchecked
 * {@link SensorPollGrantAccess.ExpectedBinding} values as issuance proof.
 *
 * <p>Lock order, scope∩live-allowlist intersection, current credential fencing,
 * persisted status/window checks and denial-without-grant-snapshot semantics remain
 * owned by {@link SensorPollGrantAccess}; this class does not replace or nest a
 * second transaction around that accessor.
 */
public class SensorPollGrantEvaluationService {

    private final SensorPollGrantAccess access;

    public SensorPollGrantEvaluationService(SensorPollGrantAccess access) {
        this.access = Objects.requireNonNull(access, "access");
    }

    /**
     * Evaluate whether the named persisted grant may authorize <em>new</em> execution
     * work for the authenticated Sensor at Core evaluation time.
     *
     * <p>{@code authority} must already be server-derived (for example from
     * {@code PostgresSensorAuthorityResolver}). {@code grantId} selects the V015
     * ledger row under that authenticated scope. Optional Core-side expected-binding
     * consistency checks are intentionally not exposed on this seam.
     */
    @Transactional("transactionManager")
    public SensorPollGrantEligibility evaluateForNewExecution(
            ResolvedSensorAuthority authority,
            UUID grantId
    ) {
        if (authority == null || grantId == null) {
            throw new SensorControlException(
                    SensorControlException.ERR_POLL_GRANT_REQUEST_INVALID,
                    "Poll grant lookup request is invalid");
        }
        return access.evaluateForNewExecution(
                new SensorPollGrantAccess.LookupRequest(authority, grantId));
    }
}
