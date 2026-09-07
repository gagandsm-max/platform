package com.platform.core.persistence;

import com.platform.core.domain.inventory.EntityTagAssignment;
import com.platform.core.domain.inventory.GroupMembership;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.UUID;

@Repository
public class MembershipRepository {

    private static final RowMapper<GroupMembership> GROUP_MAPPER = (rs, rowNum) -> new GroupMembership(
            rs.getObject("tenant_id", UUID.class),
            rs.getObject("entity_id", UUID.class),
            rs.getObject("group_id", UUID.class),
            JdbcTime.instant(rs.getTimestamp("created_at"))
    );

    private static final RowMapper<EntityTagAssignment> TAG_MAPPER = (rs, rowNum) -> new EntityTagAssignment(
            rs.getObject("tenant_id", UUID.class),
            rs.getObject("entity_id", UUID.class),
            rs.getObject("tag_id", UUID.class),
            JdbcTime.instant(rs.getTimestamp("created_at"))
    );

    private final NamedParameterJdbcTemplate jdbc;

    public MembershipRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public void assignGroup(GroupMembership membership) {
        jdbc.update("""
                INSERT INTO platform.entity_group_membership (tenant_id, entity_id, group_id, created_at)
                VALUES (:tenantId, :entityId, :groupId, :createdAt)
                ON CONFLICT (tenant_id, entity_id, group_id) DO NOTHING
                """,
                Map.of(
                        "tenantId", membership.tenantId(),
                        "entityId", membership.entityId(),
                        "groupId", membership.groupId(),
                        "createdAt", JdbcTime.ts(membership.createdAt())
                ));
    }

    public int unassignGroup(UUID tenantId, UUID entityId, UUID groupId) {
        return jdbc.update("""
                DELETE FROM platform.entity_group_membership
                 WHERE tenant_id = :tenantId AND entity_id = :entityId AND group_id = :groupId
                """,
                Map.of("tenantId", tenantId, "entityId", entityId, "groupId", groupId));
    }

    public List<GroupMembership> listGroupsForEntity(UUID tenantId, UUID entityId, int limit, long offset) {
        return jdbc.query("""
                SELECT * FROM platform.entity_group_membership
                 WHERE tenant_id = :tenantId AND entity_id = :entityId
                 ORDER BY created_at ASC, group_id ASC
                 LIMIT :limit OFFSET :offset
                """,
                Map.of("tenantId", tenantId, "entityId", entityId, "limit", limit, "offset", offset),
                GROUP_MAPPER);
    }

    public long countGroupsForEntity(UUID tenantId, UUID entityId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.entity_group_membership WHERE tenant_id = :tenantId AND entity_id = :entityId",
                Map.of("tenantId", tenantId, "entityId", entityId),
                Long.class
        );
        return count == null ? 0L : count;
    }

    public void assignTag(EntityTagAssignment assignment) {
        jdbc.update("""
                INSERT INTO platform.entity_tag (tenant_id, entity_id, tag_id, created_at)
                VALUES (:tenantId, :entityId, :tagId, :createdAt)
                ON CONFLICT (tenant_id, entity_id, tag_id) DO NOTHING
                """,
                Map.of(
                        "tenantId", assignment.tenantId(),
                        "entityId", assignment.entityId(),
                        "tagId", assignment.tagId(),
                        "createdAt", JdbcTime.ts(assignment.createdAt())
                ));
    }

    public int unassignTag(UUID tenantId, UUID entityId, UUID tagId) {
        return jdbc.update("""
                DELETE FROM platform.entity_tag
                 WHERE tenant_id = :tenantId AND entity_id = :entityId AND tag_id = :tagId
                """,
                Map.of("tenantId", tenantId, "entityId", entityId, "tagId", tagId));
    }

    public List<EntityTagAssignment> listTagsForEntity(UUID tenantId, UUID entityId, int limit, long offset) {
        return jdbc.query("""
                SELECT * FROM platform.entity_tag
                 WHERE tenant_id = :tenantId AND entity_id = :entityId
                 ORDER BY created_at ASC, tag_id ASC
                 LIMIT :limit OFFSET :offset
                """,
                Map.of("tenantId", tenantId, "entityId", entityId, "limit", limit, "offset", offset),
                TAG_MAPPER);
    }

    public long countTagsForEntity(UUID tenantId, UUID entityId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.entity_tag WHERE tenant_id = :tenantId AND entity_id = :entityId",
                Map.of("tenantId", tenantId, "entityId", entityId),
                Long.class
        );
        return count == null ? 0L : count;
    }
}
