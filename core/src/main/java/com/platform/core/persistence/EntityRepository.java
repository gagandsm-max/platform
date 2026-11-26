package com.platform.core.persistence;

import com.platform.core.domain.inventory.Entity;
import com.platform.core.domain.inventory.LifecycleState;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;

import java.sql.Types;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

@Repository
public class EntityRepository {

    private static final RowMapper<Entity> MAPPER = (rs, rowNum) -> new Entity(
            rs.getObject("entity_id", UUID.class),
            rs.getObject("tenant_id", UUID.class),
            rs.getString("entity_type"),
            rs.getString("display_name"),
            rs.getString("description"),
            rs.getString("discovered_name"),
            LifecycleState.valueOf(rs.getString("lifecycle_state")),
            rs.getObject("site_id", UUID.class),
            rs.getString("criticality"),
            rs.getObject("owner_team_id", UUID.class),
            rs.getString("device_pack_id"),
            rs.getString("device_pack_version"),
            JdbcTime.instant(rs.getTimestamp("first_seen_at")),
            JdbcTime.instant(rs.getTimestamp("last_seen_at")),
            rs.getString("created_by"),
            JdbcTime.instant(rs.getTimestamp("created_at")),
            JdbcTime.instant(rs.getTimestamp("updated_at"))
    );

    private final NamedParameterJdbcTemplate jdbc;

    public EntityRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public void insert(Entity entity) {
        jdbc.update("""
                INSERT INTO platform.entity (
                    entity_id, tenant_id, entity_type, display_name, description, discovered_name,
                    lifecycle_state, site_id, criticality, owner_team_id, device_pack_id, device_pack_version,
                    first_seen_at, last_seen_at, created_by, created_at, updated_at
                ) VALUES (
                    :entityId, :tenantId, :entityType, :displayName, :description, :discoveredName,
                    :lifecycleState, :siteId, :criticality, :ownerTeamId, :devicePackId, :devicePackVersion,
                    :firstSeenAt, :lastSeenAt, :createdBy, :createdAt, :updatedAt
                )
                """, params(entity));
    }

    public void update(Entity entity) {
        jdbc.update("""
                UPDATE platform.entity
                   SET entity_type = :entityType,
                       display_name = :displayName,
                       description = :description,
                       discovered_name = :discoveredName,
                       lifecycle_state = :lifecycleState,
                       site_id = :siteId,
                       criticality = :criticality,
                       owner_team_id = :ownerTeamId,
                       device_pack_id = :devicePackId,
                       device_pack_version = :devicePackVersion,
                       first_seen_at = :firstSeenAt,
                       last_seen_at = :lastSeenAt,
                       created_by = :createdBy,
                       updated_at = :updatedAt
                 WHERE tenant_id = :tenantId AND entity_id = :entityId
                """, params(entity));
    }

    public Optional<Entity> findByTenantAndId(UUID tenantId, UUID entityId) {
        List<Entity> rows = jdbc.query(
                "SELECT * FROM platform.entity WHERE tenant_id = :tenantId AND entity_id = :entityId",
                Map.of("tenantId", tenantId, "entityId", entityId),
                MAPPER
        );
        return rows.stream().findFirst();
    }

    public List<Entity> listByTenant(UUID tenantId, int limit, long offset) {
        return jdbc.query("""
                SELECT * FROM platform.entity
                 WHERE tenant_id = :tenantId
                 ORDER BY created_at ASC, entity_id ASC
                 LIMIT :limit OFFSET :offset
                """,
                Map.of("tenantId", tenantId, "limit", limit, "offset", offset),
                MAPPER);
    }

    public List<Entity> listByTenantAfter(UUID tenantId, int fetchLimit, Instant afterCreatedAt, UUID afterEntityId) {
        if (tenantId == null) {
            throw new IllegalArgumentException("tenantId is required");
        }
        if (fetchLimit < 1 || fetchLimit > 201) {
            throw new IllegalArgumentException("fetchLimit must be between 1 and 201");
        }
        if ((afterCreatedAt == null) != (afterEntityId == null)) {
            throw new IllegalArgumentException("afterCreatedAt and afterEntityId must be supplied together");
        }

        MapSqlParameterSource parameters = new MapSqlParameterSource()
                .addValue("tenantId", tenantId, Types.OTHER)
                .addValue("fetchLimit", fetchLimit, Types.INTEGER);
        if (afterCreatedAt == null) {
            return jdbc.query("""
                    SELECT * FROM platform.entity
                     WHERE tenant_id = :tenantId
                     ORDER BY created_at ASC, entity_id ASC
                     LIMIT :fetchLimit
                    """, parameters, MAPPER);
        }

        return jdbc.query("""
                SELECT * FROM platform.entity
                 WHERE tenant_id = :tenantId
                   AND (created_at, entity_id) > (:afterCreatedAt, :afterEntityId)
                 ORDER BY created_at ASC, entity_id ASC
                 LIMIT :fetchLimit
                """, parameters
                .addValue("afterCreatedAt", JdbcTime.ts(afterCreatedAt), Types.TIMESTAMP)
                .addValue("afterEntityId", afterEntityId, Types.OTHER), MAPPER);
    }

    public long countByTenant(UUID tenantId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.entity WHERE tenant_id = :tenantId",
                Map.of("tenantId", tenantId),
                Long.class
        );
        return count == null ? 0L : count;
    }

    private static MapSqlParameterSource params(Entity entity) {
        return new MapSqlParameterSource()
                .addValue("entityId", entity.entityId())
                .addValue("tenantId", entity.tenantId())
                .addValue("entityType", entity.entityType())
                .addValue("displayName", entity.displayName())
                .addValue("description", entity.description())
                .addValue("discoveredName", entity.discoveredName())
                .addValue("lifecycleState", entity.lifecycleState().name())
                .addValue("siteId", entity.siteId())
                .addValue("criticality", entity.criticality())
                .addValue("ownerTeamId", entity.ownerTeamId())
                .addValue("devicePackId", entity.devicePackId())
                .addValue("devicePackVersion", entity.devicePackVersion())
                .addValue("firstSeenAt", JdbcTime.ts(entity.firstSeenAt()))
                .addValue("lastSeenAt", JdbcTime.ts(entity.lastSeenAt()))
                .addValue("createdBy", entity.createdBy())
                .addValue("createdAt", JdbcTime.ts(entity.createdAt()))
                .addValue("updatedAt", JdbcTime.ts(entity.updatedAt()));
    }
}
