package com.platform.core.persistence;

import com.platform.core.domain.monitoring.MonitoringProfileSelection.AssignmentMode;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;

/**
 * Tenant-scoped, read-only monitoring-profile projection (BUILD-070).
 *
 * <p>No write, activation, criticality inference, or policy validation authority is exposed.
 * Callers must enforce tenant/resource authorization before invoking this persistence adapter.
 * All lookups use the caller's tenant UUID as part of the SQL predicate.
 */
@Repository
public final class MonitoringProfileRepository {

    public static final int MAX_LIST_LIMIT = 200;

    public record Profile(
            UUID tenantId, UUID profileId, String name, UUID createdBy,
            Instant createdAt, Instant updatedAt, UUID auditRef
    ) { }

    /** Content hashes are immutable lowercase hex, avoiding mutable byte-array exposure. */
    public record Version(
            UUID tenantId, UUID profileId, int versionNo, String definitionJson,
            String contentHashHex, String capacityEstimateJson, UUID createdBy,
            Instant createdAt, UUID auditRef
    ) { }

    public record Assignment(
            UUID tenantId, UUID entityId, UUID profileId, int profileVersionNo,
            AssignmentMode mode, UUID assignedBy, Instant effectiveFrom,
            long rowVersion, UUID auditRef, Instant updatedAt
    ) { }

    private static final RowMapper<Profile> PROFILE_MAPPER = (rs, row) -> new Profile(
            rs.getObject("tenant_id", UUID.class),
            rs.getObject("profile_id", UUID.class),
            rs.getString("name"),
            rs.getObject("created_by", UUID.class),
            JdbcTime.instant(rs.getTimestamp("created_at")),
            JdbcTime.instant(rs.getTimestamp("updated_at")),
            rs.getObject("audit_ref", UUID.class)
    );

    private static final RowMapper<Version> VERSION_MAPPER = (rs, row) -> new Version(
            rs.getObject("tenant_id", UUID.class),
            rs.getObject("profile_id", UUID.class),
            rs.getInt("version_no"),
            rs.getString("definition"),
            rs.getString("content_hash_hex"),
            rs.getString("capacity_estimate"),
            rs.getObject("created_by", UUID.class),
            JdbcTime.instant(rs.getTimestamp("created_at")),
            rs.getObject("audit_ref", UUID.class)
    );

    private static final RowMapper<Assignment> ASSIGNMENT_MAPPER = (rs, row) -> new Assignment(
            rs.getObject("tenant_id", UUID.class),
            rs.getObject("entity_id", UUID.class),
            rs.getObject("profile_id", UUID.class),
            rs.getInt("profile_version_no"),
            AssignmentMode.valueOf(rs.getString("assignment_mode")),
            rs.getObject("assigned_by", UUID.class),
            JdbcTime.instant(rs.getTimestamp("effective_from")),
            rs.getLong("row_version"),
            rs.getObject("audit_ref", UUID.class),
            JdbcTime.instant(rs.getTimestamp("updated_at"))
    );

    private final NamedParameterJdbcTemplate jdbc;

    public MonitoringProfileRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = Objects.requireNonNull(jdbc, "jdbc");
    }

    public Optional<Profile> findProfile(UUID tenantId, UUID profileId) {
        requireIds(tenantId, profileId);
        return jdbc.query("""
                SELECT tenant_id, profile_id, name, created_by, created_at, updated_at, audit_ref
                  FROM platform.monitoring_profile
                 WHERE tenant_id = :tenantId AND profile_id = :profileId
                """, Map.of("tenantId", tenantId, "profileId", profileId), PROFILE_MAPPER)
                .stream().findFirst();
    }

    public Optional<Version> findVersion(UUID tenantId, UUID profileId, int versionNo) {
        requireIds(tenantId, profileId);
        if (versionNo < 1) {
            throw new IllegalArgumentException("versionNo must be positive");
        }
        return jdbc.query("""
                SELECT tenant_id, profile_id, version_no, definition,
                       encode(content_hash, 'hex') AS content_hash_hex,
                       capacity_estimate, created_by, created_at, audit_ref
                  FROM platform.monitoring_profile_version
                 WHERE tenant_id = :tenantId AND profile_id = :profileId
                   AND version_no = :versionNo
                """, Map.of("tenantId", tenantId, "profileId", profileId, "versionNo", versionNo),
                VERSION_MAPPER).stream().findFirst();
    }

    public Optional<Version> latestVersion(UUID tenantId, UUID profileId) {
        requireIds(tenantId, profileId);
        return jdbc.query("""
                SELECT tenant_id, profile_id, version_no, definition,
                       encode(content_hash, 'hex') AS content_hash_hex,
                       capacity_estimate, created_by, created_at, audit_ref
                  FROM platform.monitoring_profile_version
                 WHERE tenant_id = :tenantId AND profile_id = :profileId
                 ORDER BY version_no DESC
                 LIMIT 1
                """, Map.of("tenantId", tenantId, "profileId", profileId),
                VERSION_MAPPER).stream().findFirst();
    }

    public Optional<Assignment> findAssignment(UUID tenantId, UUID entityId) {
        requireIds(tenantId, entityId);
        return jdbc.query("""
                SELECT tenant_id, entity_id, profile_id, profile_version_no,
                       assignment_mode, assigned_by, effective_from, row_version,
                       audit_ref, updated_at
                  FROM platform.monitoring_profile_assignment
                 WHERE tenant_id = :tenantId AND entity_id = :entityId
                """, Map.of("tenantId", tenantId, "entityId", entityId),
                ASSIGNMENT_MAPPER).stream().findFirst();
    }

    /** Stable tenant-scoped paging; deliberately no unbounded list variant. */
    public List<Profile> listProfiles(UUID tenantId, int limit, long offset) {
        Objects.requireNonNull(tenantId, "tenantId");
        if (limit < 1 || limit > MAX_LIST_LIMIT || offset < 0) {
            throw new IllegalArgumentException("invalid bounded profile page");
        }
        return jdbc.query("""
                SELECT tenant_id, profile_id, name, created_by, created_at, updated_at, audit_ref
                  FROM platform.monitoring_profile
                 WHERE tenant_id = :tenantId
                 ORDER BY name ASC, profile_id ASC
                 LIMIT :limit OFFSET :offset
                """, Map.of("tenantId", tenantId, "limit", limit, "offset", offset),
                PROFILE_MAPPER);
    }

    private static void requireIds(UUID tenantId, UUID recordId) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(recordId, "recordId");
    }
}
