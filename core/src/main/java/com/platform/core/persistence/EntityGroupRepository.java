package com.platform.core.persistence;

import com.platform.core.domain.inventory.EntityGroup;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

@Repository
public class EntityGroupRepository {

    private static final RowMapper<EntityGroup> MAPPER = (rs, rowNum) -> new EntityGroup(
            rs.getObject("group_id", UUID.class),
            rs.getObject("tenant_id", UUID.class),
            rs.getObject("parent_group_id", UUID.class),
            rs.getString("name"),
            rs.getString("description"),
            JdbcTime.instant(rs.getTimestamp("created_at")),
            JdbcTime.instant(rs.getTimestamp("updated_at"))
    );

    private final NamedParameterJdbcTemplate jdbc;

    public EntityGroupRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public void insert(EntityGroup group) {
        jdbc.update("""
                INSERT INTO platform.entity_group
                    (group_id, tenant_id, parent_group_id, name, description, created_at, updated_at)
                VALUES
                    (:groupId, :tenantId, :parentGroupId, :name, :description, :createdAt, :updatedAt)
                """, params(group));
    }

    public void update(EntityGroup group) {
        jdbc.update("""
                UPDATE platform.entity_group
                   SET parent_group_id = :parentGroupId,
                       name = :name,
                       description = :description,
                       updated_at = :updatedAt
                 WHERE tenant_id = :tenantId AND group_id = :groupId
                """, params(group));
    }

    public Optional<EntityGroup> findByTenantAndId(UUID tenantId, UUID groupId) {
        List<EntityGroup> rows = jdbc.query(
                "SELECT * FROM platform.entity_group WHERE tenant_id = :tenantId AND group_id = :groupId",
                Map.of("tenantId", tenantId, "groupId", groupId),
                MAPPER
        );
        return rows.stream().findFirst();
    }

    public List<EntityGroup> listByTenant(UUID tenantId, int limit, int offset) {
        return jdbc.query("""
                SELECT * FROM platform.entity_group
                 WHERE tenant_id = :tenantId
                 ORDER BY created_at ASC, group_id ASC
                 LIMIT :limit OFFSET :offset
                """,
                Map.of("tenantId", tenantId, "limit", limit, "offset", offset),
                MAPPER);
    }

    public long countByTenant(UUID tenantId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.entity_group WHERE tenant_id = :tenantId",
                Map.of("tenantId", tenantId),
                Long.class
        );
        return count == null ? 0L : count;
    }

    private static MapSqlParameterSource params(EntityGroup group) {
        return new MapSqlParameterSource()
                .addValue("groupId", group.groupId())
                .addValue("tenantId", group.tenantId())
                .addValue("parentGroupId", group.parentGroupId())
                .addValue("name", group.name())
                .addValue("description", group.description())
                .addValue("createdAt", JdbcTime.ts(group.createdAt()))
                .addValue("updatedAt", JdbcTime.ts(group.updatedAt()));
    }
}
