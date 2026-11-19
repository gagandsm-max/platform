package com.platform.core.persistence;

import com.platform.core.domain.monitoring.MonitoringProfileSelection.AssignmentMode;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;

import java.util.Map;
import java.util.Objects;
import java.util.UUID;

/** Inert tenant-scoped persistence primitives for BUILD-070 assignment mutations. */
@Repository
public final class MonitoringProfileAssignmentMutationRepository {

    private static final long MAX_ROW_VERSION = Long.MAX_VALUE;

    private final NamedParameterJdbcTemplate jdbc;

    public MonitoringProfileAssignmentMutationRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = Objects.requireNonNull(jdbc, "jdbc");
    }

    /** Inserts only when this tenant/entity has no assignment; returns 1 on insert, otherwise 0. */
    public int insertIfAbsent(UUID tenantId, UUID entityId, UUID profileId, int profileVersionNo,
                              AssignmentMode mode, UUID assignedBy, UUID auditRef) {
        validate(tenantId, entityId, profileId, profileVersionNo, mode, assignedBy, auditRef);
        return jdbc.update("""
                INSERT INTO platform.monitoring_profile_assignment
                    (tenant_id, entity_id, profile_id, profile_version_no, assignment_mode,
                     assigned_by, effective_from, row_version, audit_ref)
                VALUES (:tenantId, :entityId, :profileId, :profileVersionNo, :mode,
                        :assignedBy, CURRENT_TIMESTAMP, 1, :auditRef)
                ON CONFLICT (tenant_id, entity_id) DO NOTHING
                """, params(tenantId, entityId, profileId, profileVersionNo, mode, assignedBy,
                auditRef, null));
    }

    /**
     * Replaces an assignment only at the caller's exact positive version. AUTO changes cannot
     * replace EXPLICIT_PROFILE. Returns 1 on update, or 0 for stale, absent, protected, or overflow.
     */
    public int updateIfVersion(UUID tenantId, UUID entityId, UUID profileId, int profileVersionNo,
                               AssignmentMode mode, UUID assignedBy, UUID auditRef,
                               long expectedRowVersion) {
        validate(tenantId, entityId, profileId, profileVersionNo, mode, assignedBy, auditRef);
        if (expectedRowVersion < 1) {
            throw new IllegalArgumentException("expectedRowVersion must be positive");
        }
        return jdbc.update("""
                UPDATE platform.monitoring_profile_assignment
                   SET profile_id = :profileId,
                       profile_version_no = :profileVersionNo,
                       assignment_mode = :mode,
                       assigned_by = :assignedBy,
                       audit_ref = :auditRef,
                       effective_from = CURRENT_TIMESTAMP,
                       updated_at = CURRENT_TIMESTAMP,
                       row_version = row_version + 1
                 WHERE tenant_id = :tenantId
                   AND entity_id = :entityId
                   AND row_version = :expectedRowVersion
                   AND row_version < :maxRowVersion
                   AND (:mode <> 'AUTO_FROM_CRITICALITY'
                        OR assignment_mode <> 'EXPLICIT_PROFILE')
                """, params(tenantId, entityId, profileId, profileVersionNo, mode, assignedBy,
                auditRef, expectedRowVersion));
    }

    private static Map<String, Object> params(UUID tenantId, UUID entityId, UUID profileId,
                                               int profileVersionNo, AssignmentMode mode,
                                               UUID assignedBy, UUID auditRef, Long expectedRowVersion) {
        return Map.of(
                "tenantId", tenantId,
                "entityId", entityId,
                "profileId", profileId,
                "profileVersionNo", profileVersionNo,
                "mode", mode.name(),
                "assignedBy", assignedBy,
                "auditRef", auditRef,
                "expectedRowVersion", expectedRowVersion == null ? 0L : expectedRowVersion,
                "maxRowVersion", MAX_ROW_VERSION);
    }

    private static void validate(UUID tenantId, UUID entityId, UUID profileId, int profileVersionNo,
                                 AssignmentMode mode, UUID assignedBy, UUID auditRef) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(entityId, "entityId");
        Objects.requireNonNull(profileId, "profileId");
        Objects.requireNonNull(mode, "mode");
        Objects.requireNonNull(assignedBy, "assignedBy");
        Objects.requireNonNull(auditRef, "auditRef");
        if (profileVersionNo < 1) {
            throw new IllegalArgumentException("profileVersionNo must be positive");
        }
    }
}
