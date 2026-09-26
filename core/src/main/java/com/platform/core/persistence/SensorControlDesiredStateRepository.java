package com.platform.core.persistence;

import com.platform.core.sensorcontrol.desiredstate.SensorControlDesiredConfigJson;
import com.platform.core.sensorcontrol.desiredstate.SensorControlObservationOutcome;
import com.platform.core.sensorcontrol.desiredstate.SensorControlState;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;

import java.sql.Timestamp;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

@Repository
public class SensorControlDesiredStateRepository {

    private final NamedParameterJdbcTemplate jdbc;

    private final RowMapper<SensorControlState> STATE_MAPPER = (rs, rowNum) -> {
        String outcomeRaw = rs.getString("observed_outcome");
        SensorControlObservationOutcome outcome = outcomeRaw == null
                ? null
                : SensorControlObservationOutcome.valueOf(outcomeRaw);
        return new SensorControlState(
                rs.getObject("tenant_id", UUID.class),
                rs.getString("sensor_id"),
                rs.getInt("desired_schema_version"),
                rs.getLong("desired_generation"),
                rs.getString("desired_checksum"),
                SensorControlDesiredConfigJson.fromJson(rs.getString("desired_config_json")),
                rs.getString("desired_by"),
                JdbcTime.instant(rs.getTimestamp("desired_at")),
                rs.getLong("applied_generation"),
                rs.getString("applied_checksum"),
                JdbcTime.instant(rs.getTimestamp("applied_at")),
                rs.getLong("observed_generation"),
                rs.getString("observed_checksum"),
                outcome,
                rs.getString("rejection_code"),
                rs.getString("rejection_message"),
                JdbcTime.instant(rs.getTimestamp("observed_at")),
                JdbcTime.instant(rs.getTimestamp("updated_at"))
        );
    };

    public SensorControlDesiredStateRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public boolean lockTenantForUpdate(UUID tenantId) {
        List<UUID> rows = jdbc.query(
                "SELECT tenant_id FROM platform.tenant WHERE tenant_id = :tenantId FOR UPDATE",
                Map.of("tenantId", tenantId),
                (rs, rowNum) -> rs.getObject("tenant_id", UUID.class));
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

    public Optional<SensorControlState> findState(UUID tenantId, String sensorId) {
        return jdbc.query("""
                SELECT * FROM platform.sensor_control_state
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId),
                STATE_MAPPER).stream().findFirst();
    }

    public Optional<SensorControlState> findStateForUpdate(UUID tenantId, String sensorId) {
        return jdbc.query("""
                SELECT * FROM platform.sensor_control_state
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                 FOR UPDATE
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId),
                STATE_MAPPER).stream().findFirst();
    }

    public void insertDesiredState(SensorControlState state) {
        jdbc.update("""
                INSERT INTO platform.sensor_control_state (
                    tenant_id, sensor_id,
                    desired_schema_version, desired_generation, desired_checksum, desired_config_json,
                    desired_by, desired_at,
                    applied_generation, applied_checksum, applied_at,
                    observed_generation, observed_checksum, observed_outcome,
                    rejection_code, rejection_message, observed_at,
                    updated_at
                ) VALUES (
                    :tenantId, :sensorId,
                    :desiredSchemaVersion, :desiredGeneration, :desiredChecksum, CAST(:desiredConfigJson AS jsonb),
                    :desiredBy, :desiredAt,
                    :appliedGeneration, :appliedChecksum, :appliedAt,
                    :observedGeneration, :observedChecksum, :observedOutcome,
                    :rejectionCode, :rejectionMessage, :observedAt,
                    :updatedAt
                )
                """, params(state));
    }

    public void updateDesiredStatePreservingAppliedObserved(SensorControlState state) {
        jdbc.update("""
                UPDATE platform.sensor_control_state
                   SET desired_schema_version = :desiredSchemaVersion,
                       desired_generation = :desiredGeneration,
                       desired_checksum = :desiredChecksum,
                       desired_config_json = CAST(:desiredConfigJson AS jsonb),
                       desired_by = :desiredBy,
                       desired_at = :desiredAt,
                       updated_at = :updatedAt
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                """, params(state));
    }

    public int updateObservation(SensorControlState state) {
        return jdbc.update("""
                UPDATE platform.sensor_control_state
                   SET applied_generation = :appliedGeneration,
                       applied_checksum = :appliedChecksum,
                       applied_at = :appliedAt,
                       observed_generation = :observedGeneration,
                       observed_checksum = :observedChecksum,
                       observed_outcome = :observedOutcome,
                       rejection_code = :rejectionCode,
                       rejection_message = :rejectionMessage,
                       observed_at = :observedAt,
                       updated_at = :updatedAt
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                """, params(state));
    }

    private static MapSqlParameterSource params(SensorControlState state) {
        return new MapSqlParameterSource()
                .addValue("tenantId", state.tenantId())
                .addValue("sensorId", state.sensorId())
                .addValue("desiredSchemaVersion", state.desiredSchemaVersion())
                .addValue("desiredGeneration", state.desiredGeneration())
                .addValue("desiredChecksum", state.desiredChecksum())
                .addValue("desiredConfigJson", SensorControlDesiredConfigJson.toJson(state.desiredConfig()))
                .addValue("desiredBy", state.desiredBy())
                .addValue("desiredAt", Timestamp.from(state.desiredAt()))
                .addValue("appliedGeneration", state.appliedGeneration())
                .addValue("appliedChecksum", state.appliedChecksum())
                .addValue("appliedAt", state.appliedAt() == null ? null : Timestamp.from(state.appliedAt()))
                .addValue("observedGeneration", state.observedGeneration())
                .addValue("observedChecksum", state.observedChecksum())
                .addValue("observedOutcome",
                        state.observedOutcome() == null ? null : state.observedOutcome().name())
                .addValue("rejectionCode", state.rejectionCode())
                .addValue("rejectionMessage", state.rejectionMessage())
                .addValue("observedAt", state.observedAt() == null ? null : Timestamp.from(state.observedAt()))
                .addValue("updatedAt", Timestamp.from(state.updatedAt()));
    }
}
