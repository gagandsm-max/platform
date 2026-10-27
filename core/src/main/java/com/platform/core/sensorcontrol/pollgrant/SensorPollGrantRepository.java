package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.sensorcontrol.SensorControlException;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.jdbc.datasource.ConnectionHolder;
import org.springframework.transaction.support.TransactionSynchronizationManager;

import javax.sql.DataSource;
import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.time.Instant;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;

import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.*;

/**
 * Unwired persistence adapter over V015; no issuance or admission authority.
 * Callers must supply trusted, already-authorized tenant/Sensor identifiers.
 * Uses the caller's Spring JDBC transaction when present (no independent commit).
 * The conditional UPDATE protects transitions through this adapter; V015 does
 * not prevent every possible direct SQL rewrite of terminal metadata.
 */
public final class SensorPollGrantRepository {
    private final NamedParameterJdbcTemplate jdbc;

    public SensorPollGrantRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = Objects.requireNonNull(jdbc, "jdbc");
    }

    public Optional<SensorPollGrant> find(UUID tenantId, String sensorId, UUID grantId) {
        return jdbc.query("""
                SELECT grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                       grant_generation, assignment_generation, protocol,
                       issued_address_type, issued_address, profile_plan_id, profile_plan_version,
                       issued_at, window_start, window_end, issued_by, status,
                       revoked_at, revoked_by, revocation_class, revocation_reason, updated_at
                  FROM platform.sensor_poll_grant
                 WHERE tenant_id = :tenant AND sensor_id = :sensor AND grant_id = :grant
                """, scope(tenantId, sensorId, grantId), (rs, row) -> map(rs)).stream().findFirst();
    }

    /**
     * Read and hold the exact grant row with PostgreSQL {@code FOR SHARE} until the caller
     * commits or rolls back. Existing principal and matched-credential authority locks must
     * precede grant locking. Future multi-grant admission must lock grants in a deterministic
     * order; this single-row helper does not enforce the complete admission lock order.
     * Returns terminal rows too: a row lock is neither a grant-authorization decision nor
     * proof of historical execution time. Missing and wrong-scope rows are equally empty.
     * Never opens a replacement transaction or releases caller-owned locks.
     */
    public Optional<SensorPollGrant> findForShare(UUID tenantId, String sensorId, UUID grantId) {
        requireActiveCallerTransaction("findForShare");
        return jdbc.query("""
                SELECT grant_id, tenant_id, sensor_id, entity_id, endpoint_id,
                       grant_generation, assignment_generation, protocol,
                       issued_address_type, issued_address, profile_plan_id, profile_plan_version,
                       issued_at, window_start, window_end, issued_by, status,
                       revoked_at, revoked_by, revocation_class, revocation_reason, updated_at
                  FROM platform.sensor_poll_grant
                 WHERE tenant_id = :tenant AND sensor_id = :sensor AND grant_id = :grant
                 FOR SHARE
                """, scope(tenantId, sensorId, grantId), (rs, row) -> map(rs)).stream().findFirst();
    }

    /** Same connection-participation guard as SensorControlRepository's held-authority helpers. */
    private void requireActiveCallerTransaction(String helperName) {
        if (!TransactionSynchronizationManager.isActualTransactionActive()) {
            throw transactionRequired(helperName, "unlocked lookup is not held authority");
        }
        DataSource dataSource = jdbc.getJdbcTemplate().getDataSource();
        if (dataSource == null) {
            throw transactionRequired(helperName, "repository DataSource is unavailable");
        }
        Object resource = TransactionSynchronizationManager.getResource(dataSource);
        if (!(resource instanceof ConnectionHolder holder)) {
            throw transactionRequired(helperName, "repository connection is not bound to the caller transaction");
        }
        if (!holder.isSynchronizedWithTransaction()) {
            throw transactionRequired(helperName, "repository connection is not synchronized with the caller transaction");
        }
        final Connection connection;
        try {
            connection = holder.getConnection();
        } catch (IllegalStateException e) {
            throw transactionRequired(helperName, "repository connection is not bound to the caller transaction");
        }
        final boolean autoCommit;
        try {
            autoCommit = connection.getAutoCommit();
        } catch (SQLException e) {
            throw transactionRequired(helperName, "unable to verify caller transaction participation");
        }
        if (autoCommit) {
            throw transactionRequired(helperName, "bound repository connection remains autocommit");
        }
    }

    private static SensorControlException transactionRequired(String helperName, String reason) {
        return new SensorControlException(SensorControlException.ERR_SENSOR_STATE_INVALID,
                "Active PostgreSQL transaction required for " + helperName + "; " + reason);
    }

    /** First committed terminal status and complete tuple win. A rolled-back attempt does not win. */
    public TransitionOutcome terminate(UUID tenantId, String sensorId, UUID grantId, Termination terminal) {
        Objects.requireNonNull(terminal, "terminal");
        MapSqlParameterSource parameters = scope(tenantId, sensorId, grantId)
                .addValue("status", terminal.status().name())
                .addValue("at", Timestamp.from(terminal.revokedAt()))
                .addValue("by", terminal.revokedBy())
                .addValue("class", terminal.revocationClass().name())
                .addValue("reason", terminal.revocationReason());
        int changed = jdbc.update("""
                UPDATE platform.sensor_poll_grant
                   SET status = :status, revoked_at = :at, revoked_by = :by,
                       revocation_class = :class, revocation_reason = :reason,
                       updated_at = clock_timestamp()
                 WHERE tenant_id = :tenant AND sensor_id = :sensor AND grant_id = :grant
                   AND status = 'ISSUED'
                """, parameters);
        return changed == 1 ? TransitionOutcome.TRANSITIONED : TransitionOutcome.NO_PERMITTED_TRANSITION;
    }

    /** Missing, wrong-scope and already-terminal rows deliberately share one outcome. */
    public enum TransitionOutcome { TRANSITIONED, NO_PERMITTED_TRANSITION }

    public record Termination(Status status, Instant revokedAt, String revokedBy,
                              RevocationClass revocationClass, String revocationReason) {
        public Termination {
            Objects.requireNonNull(status, "status");
            if (status == Status.ISSUED) {
                throw new IllegalArgumentException("Terminal status required");
            }
            Objects.requireNonNull(revokedAt, "revokedAt");
            Objects.requireNonNull(revocationClass, "revocationClass");
            // Remaining V015 coherence/length constraints are enforced by PostgreSQL.
        }
    }

    private static MapSqlParameterSource scope(UUID tenantId, String sensorId, UUID grantId) {
        return new MapSqlParameterSource()
                .addValue("tenant", Objects.requireNonNull(tenantId, "tenantId"))
                .addValue("sensor", Objects.requireNonNull(sensorId, "sensorId"))
                .addValue("grant", Objects.requireNonNull(grantId, "grantId"));
    }

    private static SensorPollGrant map(ResultSet rs) throws SQLException {
        String revocation = rs.getString("revocation_class");
        Timestamp revoked = rs.getTimestamp("revoked_at");
        return new SensorPollGrant(
                rs.getObject("grant_id", UUID.class), rs.getObject("tenant_id", UUID.class),
                rs.getString("sensor_id"), rs.getObject("entity_id", UUID.class),
                rs.getObject("endpoint_id", UUID.class), rs.getLong("grant_generation"),
                rs.getLong("assignment_generation"), Protocol.valueOf(rs.getString("protocol")),
                AddressType.valueOf(rs.getString("issued_address_type")), rs.getString("issued_address"),
                rs.getString("profile_plan_id"), rs.getString("profile_plan_version"),
                rs.getTimestamp("issued_at").toInstant(), rs.getTimestamp("window_start").toInstant(),
                rs.getTimestamp("window_end").toInstant(), rs.getString("issued_by"),
                Status.valueOf(rs.getString("status")), revoked == null ? null : revoked.toInstant(),
                rs.getString("revoked_by"), revocation == null ? null : RevocationClass.valueOf(revocation),
                rs.getString("revocation_reason"), rs.getTimestamp("updated_at").toInstant());
    }
}
