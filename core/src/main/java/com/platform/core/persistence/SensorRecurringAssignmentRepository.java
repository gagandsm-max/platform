package com.platform.core.persistence;

import com.platform.core.domain.recurring.RecurringAssignment;
import com.platform.core.domain.recurring.RecurringScheduleBounds;
import com.platform.core.domain.recurring.RecurringState;
import com.platform.core.domain.recurring.SensorRecurringDesiredChecksum;
import com.platform.core.domain.recurring.SnmpParams;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;

import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

/**
 * Sensor recurring schedule assignment and desired/applied state persistence (Slice A).
 * Every query predicates {@code tenant_id}; no cross-tenant helpers.
 */
@Repository
public class SensorRecurringAssignmentRepository {

    private static final RowMapper<RecurringAssignment> ASSIGNMENT_MAPPER = (rs, rowNum) ->
            new RecurringAssignment(
                    rs.getObject("tenant_id", UUID.class),
                    rs.getString("sensor_id"),
                    rs.getString("assignment_id"),
                    rs.getString("entity_id"),
                    rs.getString("monitor_definition_id"),
                    rs.getString("scheduling_class"),
                    rs.getString("execution_class"),
                    rs.getLong("cadence_ms"),
                    rs.getLong("freshness_ms"),
                    rs.getInt("phase_version"),
                    rs.getLong("execution_timeout_ms"),
                    rs.getString("credential_profile_ref"),
                    rs.getString("protocol"),
                    rs.getString("protocol_params_json"),
                    rs.getString("assigned_by"),
                    JdbcTime.instant(rs.getTimestamp("assigned_at"))
            );

    private static final RowMapper<RecurringState> STATE_MAPPER = (rs, rowNum) ->
            new RecurringState(
                    rs.getObject("tenant_id", UUID.class),
                    rs.getString("sensor_id"),
                    rs.getLong("desired_generation"),
                    rs.getString("desired_checksum"),
                    rs.getLong("applied_generation"),
                    rs.getString("applied_checksum"),
                    JdbcTime.instant(rs.getTimestamp("applied_at")),
                    rs.getString("updated_by"),
                    JdbcTime.instant(rs.getTimestamp("updated_at"))
            );

    private final NamedParameterJdbcTemplate jdbc;

    public SensorRecurringAssignmentRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public boolean tenantExists(UUID tenantId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.tenant WHERE tenant_id = :tenantId",
                Map.of("tenantId", tenantId),
                Long.class
        );
        return count != null && count > 0;
    }

    /** Serializes assignment mutations for one tenant. Must precede Sensor lock. */
    public boolean lockTenantForUpdate(UUID tenantId) {
        List<UUID> rows = jdbc.query(
                "SELECT tenant_id FROM platform.tenant WHERE tenant_id = :tenantId FOR UPDATE",
                Map.of("tenantId", tenantId),
                (rs, rowNum) -> rs.getObject("tenant_id", UUID.class)
        );
        return !rows.isEmpty();
    }

    public Optional<String> findSensorStatus(UUID tenantId, String sensorId) {
        List<String> rows = jdbc.query("""
                SELECT status FROM platform.sensor
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId),
                (rs, rowNum) -> rs.getString("status"));
        return rows.stream().findFirst();
    }

    /** Locks the Sensor row after tenant lock. Returns status when found. */
    public Optional<String> lockSensorForUpdate(UUID tenantId, String sensorId) {
        List<String> rows = jdbc.query("""
                SELECT status FROM platform.sensor
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                 FOR UPDATE
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId),
                (rs, rowNum) -> rs.getString("status"));
        return rows.stream().findFirst();
    }

    public Optional<RecurringState> findState(UUID tenantId, String sensorId) {
        List<RecurringState> rows = jdbc.query("""
                SELECT * FROM platform.sensor_recurring_state
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId),
                STATE_MAPPER);
        return rows.stream().findFirst();
    }

    public Optional<RecurringState> findStateForUpdate(UUID tenantId, String sensorId) {
        List<RecurringState> rows = jdbc.query("""
                SELECT * FROM platform.sensor_recurring_state
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                 FOR UPDATE
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId),
                STATE_MAPPER);
        return rows.stream().findFirst();
    }

    public void upsertDesiredState(
            UUID tenantId,
            String sensorId,
            long desiredGeneration,
            String desiredChecksum,
            String updatedBy,
            java.time.Instant updatedAt
    ) {
        jdbc.update("""
                INSERT INTO platform.sensor_recurring_state
                    (tenant_id, sensor_id, desired_generation, desired_checksum,
                     applied_generation, applied_checksum, applied_at, updated_by, updated_at)
                VALUES
                    (:tenantId, :sensorId, :desiredGeneration, :desiredChecksum,
                     0, NULL, NULL, :updatedBy, :updatedAt)
                ON CONFLICT (tenant_id, sensor_id) DO UPDATE
                   SET desired_generation = EXCLUDED.desired_generation,
                       desired_checksum = EXCLUDED.desired_checksum,
                       updated_by = EXCLUDED.updated_by,
                       updated_at = EXCLUDED.updated_at
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("sensorId", sensorId)
                        .addValue("desiredGeneration", desiredGeneration)
                        .addValue("desiredChecksum", desiredChecksum)
                        .addValue("updatedBy", updatedBy)
                        .addValue("updatedAt", JdbcTime.ts(updatedAt)));
    }

    public int updateAppliedState(
            UUID tenantId,
            String sensorId,
            long appliedGeneration,
            String appliedChecksum,
            java.time.Instant appliedAt,
            String updatedBy,
            java.time.Instant updatedAt
    ) {
        return jdbc.update("""
                UPDATE platform.sensor_recurring_state
                   SET applied_generation = :appliedGeneration,
                       applied_checksum = :appliedChecksum,
                       applied_at = :appliedAt,
                       updated_by = :updatedBy,
                       updated_at = :updatedAt
                 WHERE tenant_id = :tenantId
                   AND sensor_id = :sensorId
                   AND desired_generation = :appliedGeneration
                   AND desired_checksum IS NOT DISTINCT FROM :appliedChecksum
                   AND applied_generation <= :appliedGeneration
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("sensorId", sensorId)
                        .addValue("appliedGeneration", appliedGeneration)
                        .addValue("appliedChecksum", appliedChecksum)
                        .addValue("appliedAt", JdbcTime.ts(appliedAt))
                        .addValue("updatedBy", updatedBy)
                        .addValue("updatedAt", JdbcTime.ts(updatedAt)));
    }

    public List<RecurringAssignment> listAssignments(UUID tenantId, String sensorId) {
        return jdbc.query("""
                SELECT * FROM platform.sensor_recurring_assignment
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                 ORDER BY assignment_id ASC
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId),
                ASSIGNMENT_MAPPER);
    }

    public int deleteAllAssignments(UUID tenantId, String sensorId) {
        return jdbc.update("""
                DELETE FROM platform.sensor_recurring_assignment
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId));
    }

    public int deleteAssignment(UUID tenantId, String sensorId, String assignmentId) {
        return jdbc.update("""
                DELETE FROM platform.sensor_recurring_assignment
                 WHERE tenant_id = :tenantId
                   AND sensor_id = :sensorId
                   AND assignment_id = :assignmentId
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId, "assignmentId", assignmentId));
    }

    public void insertAssignment(RecurringAssignment assignment) {
        jdbc.update("""
                INSERT INTO platform.sensor_recurring_assignment
                    (tenant_id, sensor_id, assignment_id, entity_id, monitor_definition_id,
                     scheduling_class, execution_class, cadence_ms, freshness_ms, phase_version,
                     execution_timeout_ms, credential_profile_ref, protocol, protocol_params_json,
                     assigned_by, assigned_at)
                VALUES
                    (:tenantId, :sensorId, :assignmentId, :entityId, :monitorDefinitionId,
                     :schedulingClass, :executionClass, :cadenceMs, :freshnessMs, :phaseVersion,
                     :executionTimeoutMs, :credentialProfileRef, :protocol, :protocolParamsJson,
                     :assignedBy, :assignedAt)
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", assignment.tenantId())
                        .addValue("sensorId", assignment.sensorId())
                        .addValue("assignmentId", assignment.assignmentId())
                        .addValue("entityId", assignment.entityId())
                        .addValue("monitorDefinitionId", assignment.monitorDefinitionId())
                        .addValue("schedulingClass", assignment.schedulingClass())
                        .addValue("executionClass", assignment.executionClass())
                        .addValue("cadenceMs", assignment.cadenceMs())
                        .addValue("freshnessMs", assignment.freshnessMs())
                        .addValue("phaseVersion", assignment.phaseVersion())
                        .addValue("executionTimeoutMs", assignment.executionTimeoutMs())
                        .addValue("credentialProfileRef", assignment.credentialProfileRef())
                        .addValue("protocol", assignment.protocol())
                        .addValue("protocolParamsJson", assignment.protocolParamsJson())
                        .addValue("assignedBy", assignment.assignedBy())
                        .addValue("assignedAt", JdbcTime.ts(assignment.assignedAt())));
    }

    public List<SensorRecurringDesiredChecksum.AssignmentDigest> listAssignmentDigests(
            UUID tenantId, String sensorId) {
        return listAssignments(tenantId, sensorId).stream()
                .map(SensorRecurringAssignmentRepository::toDigest)
                .toList();
    }

    public static SensorRecurringDesiredChecksum.AssignmentDigest toDigest(RecurringAssignment row) {
        SnmpParams snmp = RecurringScheduleBounds.parseAndValidateSnmpParams(row.protocolParamsJson());
        return new SensorRecurringDesiredChecksum.AssignmentDigest(
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
        );
    }
}
