package com.platform.core.persistence;

import com.platform.core.domain.capabilitypack.DesiredPackEntry;
import com.platform.core.domain.capabilitypack.SensorCapabilityPackAssignment;
import com.platform.core.domain.capabilitypack.SensorCapabilityPackState;
import com.platform.core.domain.capabilitypack.SensorPackDesiredChecksum;
import com.platform.core.domain.capabilitypack.StoredCapabilityPack;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;

import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

/**
 * Sensor Capability Pack assignment and desired/applied state persistence (EMC1.3).
 * Every query predicates {@code tenant_id}; no cross-tenant helpers.
 */
@Repository
public class SensorCapabilityPackAssignmentRepository {

    private static final RowMapper<SensorCapabilityPackAssignment> ASSIGNMENT_MAPPER = (rs, rowNum) ->
            new SensorCapabilityPackAssignment(
                    rs.getObject("tenant_id", UUID.class),
                    rs.getString("sensor_id"),
                    rs.getString("pack_id"),
                    rs.getString("pack_version"),
                    rs.getString("assigned_by"),
                    JdbcTime.instant(rs.getTimestamp("assigned_at"))
            );

    private static final RowMapper<SensorCapabilityPackState> STATE_MAPPER = (rs, rowNum) ->
            new SensorCapabilityPackState(
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

    private static final RowMapper<DesiredPackEntry> DESIRED_ENTRY_MAPPER = (rs, rowNum) ->
            new DesiredPackEntry(
                    rs.getString("pack_id"),
                    rs.getString("pack_version"),
                    rs.getString("manifest_sha256"),
                    rs.getInt("artifact_bytes")
            );

    private final NamedParameterJdbcTemplate jdbc;

    public SensorCapabilityPackAssignmentRepository(NamedParameterJdbcTemplate jdbc) {
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

    public Optional<SensorCapabilityPackState> findState(UUID tenantId, String sensorId) {
        List<SensorCapabilityPackState> rows = jdbc.query("""
                SELECT * FROM platform.sensor_capability_pack_state
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId),
                STATE_MAPPER);
        return rows.stream().findFirst();
    }

    public Optional<SensorCapabilityPackState> findStateForUpdate(UUID tenantId, String sensorId) {
        List<SensorCapabilityPackState> rows = jdbc.query("""
                SELECT * FROM platform.sensor_capability_pack_state
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
                INSERT INTO platform.sensor_capability_pack_state
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
                UPDATE platform.sensor_capability_pack_state
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

    public List<SensorCapabilityPackAssignment> listAssignments(UUID tenantId, String sensorId) {
        return jdbc.query("""
                SELECT * FROM platform.sensor_capability_pack_assignment
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                 ORDER BY pack_id ASC
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId),
                ASSIGNMENT_MAPPER);
    }

    public Optional<SensorCapabilityPackAssignment> findAssignment(
            UUID tenantId, String sensorId, String packId) {
        List<SensorCapabilityPackAssignment> rows = jdbc.query("""
                SELECT * FROM platform.sensor_capability_pack_assignment
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId AND pack_id = :packId
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId, "packId", packId),
                ASSIGNMENT_MAPPER);
        return rows.stream().findFirst();
    }

    public void upsertAssignment(SensorCapabilityPackAssignment assignment) {
        jdbc.update("""
                INSERT INTO platform.sensor_capability_pack_assignment
                    (tenant_id, sensor_id, pack_id, pack_version, assigned_by, assigned_at)
                VALUES
                    (:tenantId, :sensorId, :packId, :packVersion, :assignedBy, :assignedAt)
                ON CONFLICT (tenant_id, sensor_id, pack_id) DO UPDATE
                   SET pack_version = EXCLUDED.pack_version,
                       assigned_by = EXCLUDED.assigned_by,
                       assigned_at = EXCLUDED.assigned_at
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", assignment.tenantId())
                        .addValue("sensorId", assignment.sensorId())
                        .addValue("packId", assignment.packId())
                        .addValue("packVersion", assignment.packVersion())
                        .addValue("assignedBy", assignment.assignedBy())
                        .addValue("assignedAt", JdbcTime.ts(assignment.assignedAt())));
    }

    public int deleteAssignment(UUID tenantId, String sensorId, String packId) {
        return jdbc.update("""
                DELETE FROM platform.sensor_capability_pack_assignment
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId AND pack_id = :packId
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId, "packId", packId));
    }

    public boolean hasAssignmentsForPack(UUID tenantId, String packId) {
        Long count = jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.sensor_capability_pack_assignment
                 WHERE tenant_id = :tenantId AND pack_id = :packId
                """,
                Map.of("tenantId", tenantId, "packId", packId),
                Long.class);
        return count != null && count > 0;
    }

    public List<DesiredPackEntry> listDesiredPackEntries(UUID tenantId, String sensorId) {
        return jdbc.query("""
                SELECT a.pack_id, a.pack_version, p.manifest_sha256,
                       octet_length(p.manifest_bytes) AS artifact_bytes
                  FROM platform.sensor_capability_pack_assignment a
                  JOIN platform.tenant_capability_pack p
                    ON p.tenant_id = a.tenant_id
                   AND p.pack_id = a.pack_id
                   AND p.pack_version = a.pack_version
                 WHERE a.tenant_id = :tenantId AND a.sensor_id = :sensorId
                 ORDER BY a.pack_id ASC
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId),
                DESIRED_ENTRY_MAPPER);
    }

    public List<SensorPackDesiredChecksum.AssignmentDigest> listAssignmentDigests(
            UUID tenantId, String sensorId) {
        return jdbc.query("""
                SELECT a.pack_id, a.pack_version, p.manifest_sha256
                  FROM platform.sensor_capability_pack_assignment a
                  JOIN platform.tenant_capability_pack p
                    ON p.tenant_id = a.tenant_id
                   AND p.pack_id = a.pack_id
                   AND p.pack_version = a.pack_version
                 WHERE a.tenant_id = :tenantId AND a.sensor_id = :sensorId
                 ORDER BY a.pack_id ASC
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId),
                (rs, rowNum) -> new SensorPackDesiredChecksum.AssignmentDigest(
                        rs.getString("pack_id"),
                        rs.getString("pack_version"),
                        rs.getString("manifest_sha256")));
    }

    public Optional<StoredCapabilityPack> findAssignedArtifact(
            UUID tenantId, String sensorId, String packId, String packVersion) {
        List<StoredCapabilityPack> rows = jdbc.query("""
                SELECT p.*
                  FROM platform.sensor_capability_pack_assignment a
                  JOIN platform.tenant_capability_pack p
                    ON p.tenant_id = a.tenant_id
                   AND p.pack_id = a.pack_id
                   AND p.pack_version = a.pack_version
                 WHERE a.tenant_id = :tenantId
                   AND a.sensor_id = :sensorId
                   AND a.pack_id = :packId
                   AND a.pack_version = :packVersion
                """,
                Map.of(
                        "tenantId", tenantId,
                        "sensorId", sensorId,
                        "packId", packId,
                        "packVersion", packVersion),
                (rs, rowNum) -> new StoredCapabilityPack(
                        rs.getObject("tenant_id", UUID.class),
                        rs.getString("pack_id"),
                        rs.getString("pack_version"),
                        rs.getString("schema_version"),
                        rs.getString("pack_class"),
                        rs.getString("name"),
                        rs.getString("publisher_type"),
                        rs.getString("publisher_name"),
                        rs.getString("certification"),
                        rs.getString("manifest_sha256"),
                        rs.getBytes("manifest_bytes"),
                        rs.getString("created_by"),
                        JdbcTime.instant(rs.getTimestamp("created_at"))
                ));
        return rows.stream().findFirst();
    }
}
