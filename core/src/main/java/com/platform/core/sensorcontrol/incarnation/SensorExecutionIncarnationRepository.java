package com.platform.core.sensorcontrol.incarnation;

import com.platform.core.control.ControlGenerationLimits;
import com.platform.core.id.UuidV7;
import com.platform.core.sensorcontrol.SensorControlException;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.jdbc.datasource.ConnectionHolder;
import org.springframework.transaction.support.TransactionSynchronizationManager;

import javax.sql.DataSource;
import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.time.Instant;
import java.time.OffsetDateTime;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;
import java.util.function.Supplier;

import static com.platform.core.sensorcontrol.incarnation.SensorExecutionIncarnation.Status;

/**
 * Unwired persistence adapter over V018 Sensor execution incarnation authority.
 *
 * <p>Core alone mints opaque {@code incarnationId} and allocates monotonic
 * {@code incarnationGeneration} under the Sensor principal {@code FOR UPDATE} lock.
 * Callers must supply trusted tenant/Sensor identifiers. Uses the caller's Spring JDBC
 * transaction when present (no independent commit). Does not enroll Sensors, rotate
 * credentials, issue grants, deliver companion pages or authorize telemetry.
 */
public final class SensorExecutionIncarnationRepository {

    private static final String SELECT_COLUMNS = """
            incarnation_id, tenant_id, sensor_id, incarnation_generation, status,
            activated_at, superseded_at, created_at, updated_at
            """;

    private final NamedParameterJdbcTemplate jdbc;
    private final Supplier<UUID> incarnationIds;

    public SensorExecutionIncarnationRepository(NamedParameterJdbcTemplate jdbc) {
        this(jdbc, UuidV7::generate);
    }

    SensorExecutionIncarnationRepository(NamedParameterJdbcTemplate jdbc, Supplier<UUID> incarnationIds) {
        this.jdbc = Objects.requireNonNull(jdbc, "jdbc");
        this.incarnationIds = Objects.requireNonNull(incarnationIds, "incarnationIds");
    }

    /**
     * Fail closed unless this repository's DataSource connection participates in the
     * caller's active PostgreSQL transaction. Used before acquiring principal locks.
     */
    public void requireHeldCallerTransaction(String operation) {
        requireActiveCallerTransaction(operation);
    }

    /**
     * Issue the first ACTIVE incarnation for a Sensor principal that has none.
     * Locks {@code platform.sensor} {@code FOR UPDATE}, Core-mints identity, generation=1.
     * Does not silently backfill: callers must invoke explicitly. Absent principal fails closed.
     */
    public SensorExecutionIncarnation issueInitial(UUID tenantId, String sensorId) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(sensorId, "sensorId");
        requireActiveCallerTransaction("issueInitial");
        lockPrincipalForUpdate(tenantId, sensorId);
        if (countForPrincipal(tenantId, sensorId) > 0) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_STATE_INVALID,
                    "Sensor already has an execution incarnation");
        }
        UUID incarnationId = Objects.requireNonNull(incarnationIds.get(), "incarnationId");
        return insertActive(tenantId, sensorId, 1L, incarnationId);
    }

    /**
     * Supersede the current ACTIVE incarnation and issue the next monotonic generation.
     * Locks the principal {@code FOR UPDATE}. Intended for governed future re-enrollment
     * transitions; not wired to credential rotation or production enrollment in this slice.
     */
    public SensorExecutionIncarnation rollover(UUID tenantId, String sensorId) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(sensorId, "sensorId");
        requireActiveCallerTransaction("rollover");
        lockPrincipalForUpdate(tenantId, sensorId);
        SensorExecutionIncarnation active = findActiveHoldingLock(tenantId, sensorId)
                .orElseThrow(() -> new SensorControlException(
                        SensorControlException.ERR_SENSOR_STATE_INVALID,
                        "Sensor has no active execution incarnation"));
        long next = ControlGenerationLimits.nextOrThrow(
                active.incarnationGeneration(),
                new SensorControlException(
                        SensorControlException.ERR_SENSOR_STATE_INVALID,
                        ControlGenerationLimits.EXHAUSTED_MESSAGE));
        // Mint/null-validate replacement identity before any state-mutating SQL so a
        // caller that catches UUID failure and still commits cannot leave SUPERSEDED
        // without a replacement ACTIVE row (Issue #288 G1b-C4-A).
        UUID replacementId = Objects.requireNonNull(incarnationIds.get(), "incarnationId");
        supersedeActive(tenantId, sensorId, active.incarnationId());
        return insertActive(tenantId, sensorId, next, replacementId);
    }

    public Optional<SensorExecutionIncarnation> findActive(UUID tenantId, String sensorId) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(sensorId, "sensorId");
        return jdbc.query("""
                SELECT %s
                  FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = :tenant AND sensor_id = :sensor AND status = 'ACTIVE'
                """.formatted(SELECT_COLUMNS),
                principal(tenantId, sensorId),
                (rs, row) -> map(rs)).stream().findFirst();
    }

    public Optional<SensorExecutionIncarnation> find(UUID tenantId, String sensorId, UUID incarnationId) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(sensorId, "sensorId");
        Objects.requireNonNull(incarnationId, "incarnationId");
        return jdbc.query("""
                SELECT %s
                  FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = :tenant AND sensor_id = :sensor AND incarnation_id = :incarnation
                """.formatted(SELECT_COLUMNS),
                principal(tenantId, sensorId).addValue("incarnation", incarnationId),
                (rs, row) -> map(rs)).stream().findFirst();
    }

    /**
     * Hold the ACTIVE incarnation with {@code FOR SHARE} until the caller commits or rolls back.
     * Callers that also mutate principal lifecycle must lock the principal first.
     */
    public Optional<SensorExecutionIncarnation> findActiveForShare(UUID tenantId, String sensorId) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(sensorId, "sensorId");
        requireActiveCallerTransaction("findActiveForShare");
        return jdbc.query("""
                SELECT %s
                  FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = :tenant AND sensor_id = :sensor AND status = 'ACTIVE'
                 FOR SHARE
                """.formatted(SELECT_COLUMNS),
                principal(tenantId, sensorId),
                (rs, row) -> map(rs)).stream().findFirst();
    }

    private void lockPrincipalForUpdate(UUID tenantId, String sensorId) {
        Integer locked = jdbc.query("""
                SELECT 1 FROM platform.sensor
                 WHERE tenant_id = :tenant AND sensor_id = :sensor
                 FOR UPDATE
                """, principal(tenantId, sensorId), (rs, row) -> 1).stream().findFirst().orElse(null);
        if (locked == null) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_NOT_FOUND, "Sensor not found");
        }
    }

    private long countForPrincipal(UUID tenantId, String sensorId) {
        Long count = jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = :tenant AND sensor_id = :sensor
                """, principal(tenantId, sensorId), Long.class);
        return count == null ? 0L : count;
    }

    private Optional<SensorExecutionIncarnation> findActiveHoldingLock(UUID tenantId, String sensorId) {
        return jdbc.query("""
                SELECT %s
                  FROM platform.sensor_execution_incarnation
                 WHERE tenant_id = :tenant AND sensor_id = :sensor AND status = 'ACTIVE'
                 FOR UPDATE
                """.formatted(SELECT_COLUMNS),
                principal(tenantId, sensorId),
                (rs, row) -> map(rs)).stream().findFirst();
    }

    private void supersedeActive(UUID tenantId, String sensorId, UUID incarnationId) {
        int changed = jdbc.update("""
                UPDATE platform.sensor_execution_incarnation
                   SET status = 'SUPERSEDED',
                       superseded_at = clock_timestamp(),
                       updated_at = clock_timestamp()
                 WHERE tenant_id = :tenant
                   AND sensor_id = :sensor
                   AND incarnation_id = :incarnation
                   AND status = 'ACTIVE'
                """, principal(tenantId, sensorId).addValue("incarnation", incarnationId));
        if (changed != 1) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_STATE_INVALID,
                    "Active execution incarnation could not be superseded");
        }
    }

    private SensorExecutionIncarnation insertActive(
            UUID tenantId, String sensorId, long generation, UUID incarnationId) {
        MapSqlParameterSource parameters = principal(tenantId, sensorId)
                .addValue("incarnation", Objects.requireNonNull(incarnationId, "incarnationId"))
                .addValue("generation", generation)
                .addValue("status", Status.ACTIVE.name());
        return jdbc.query("""
                INSERT INTO platform.sensor_execution_incarnation (
                    incarnation_id, tenant_id, sensor_id, incarnation_generation, status,
                    activated_at, superseded_at, created_at, updated_at
                ) VALUES (
                    :incarnation, :tenant, :sensor, :generation, :status,
                    clock_timestamp(), NULL, clock_timestamp(), clock_timestamp()
                )
                RETURNING %s
                """.formatted(SELECT_COLUMNS), parameters, (rs, row) -> map(rs)).stream().findFirst()
                .orElseThrow(() -> new SensorControlException(
                        SensorControlException.ERR_SENSOR_STATE_INVALID,
                        "Execution incarnation insert did not return a persisted row"));
    }

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
            throw transactionRequired(helperName,
                    "repository connection is not synchronized with the caller transaction");
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
        return new SensorControlException(
                SensorControlException.ERR_SENSOR_STATE_INVALID,
                "Active PostgreSQL transaction required for " + helperName + "; " + reason);
    }

    private static MapSqlParameterSource principal(UUID tenantId, String sensorId) {
        return new MapSqlParameterSource()
                .addValue("tenant", tenantId)
                .addValue("sensor", sensorId);
    }

    private static SensorExecutionIncarnation map(ResultSet rs) throws SQLException {
        return new SensorExecutionIncarnation(
                rs.getObject("incarnation_id", UUID.class),
                rs.getObject("tenant_id", UUID.class),
                rs.getString("sensor_id"),
                rs.getLong("incarnation_generation"),
                Status.valueOf(rs.getString("status")),
                toInstant(rs.getObject("activated_at", OffsetDateTime.class)),
                toInstant(rs.getObject("superseded_at", OffsetDateTime.class)),
                toInstant(rs.getObject("created_at", OffsetDateTime.class)),
                toInstant(rs.getObject("updated_at", OffsetDateTime.class)));
    }

    private static Instant toInstant(OffsetDateTime value) {
        return value == null ? null : value.toInstant();
    }
}
