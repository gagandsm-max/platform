package com.platform.core.persistence;

import com.platform.core.domain.inventory.EntityManagementEndpoint;
import com.platform.core.domain.inventory.ManagementEndpointAddressType;
import com.platform.core.domain.inventory.ManagementEndpointSource;
import com.platform.core.domain.inventory.ManagementEndpointState;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;

import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

@Repository
public class EntityManagementEndpointRepository {

    private static final RowMapper<EntityManagementEndpoint> MAPPER = (rs, rowNum) ->
            new EntityManagementEndpoint(
                    rs.getObject("endpoint_id", UUID.class),
                    rs.getObject("tenant_id", UUID.class),
                    rs.getObject("entity_id", UUID.class),
                    ManagementEndpointAddressType.valueOf(rs.getString("address_type")),
                    rs.getString("address"),
                    ManagementEndpointState.valueOf(rs.getString("state")),
                    ManagementEndpointSource.valueOf(rs.getString("source")),
                    rs.getBoolean("is_primary"),
                    rs.getString("created_by"),
                    JdbcTime.instant(rs.getTimestamp("created_at")),
                    rs.getString("updated_by"),
                    JdbcTime.instant(rs.getTimestamp("updated_at"))
            );

    private final NamedParameterJdbcTemplate jdbc;

    public EntityManagementEndpointRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public void insert(EntityManagementEndpoint endpoint) {
        jdbc.update("""
                INSERT INTO platform.entity_management_endpoint
                    (endpoint_id, tenant_id, entity_id, address_type, address, state, source,
                     is_primary, created_by, created_at, updated_by, updated_at)
                VALUES
                    (:endpointId, :tenantId, :entityId, :addressType, :address, :state, :source,
                     :isPrimary, :createdBy, :createdAt, :updatedBy, :updatedAt)
                """,
                params(endpoint));
    }

    public void update(EntityManagementEndpoint endpoint) {
        jdbc.update("""
                UPDATE platform.entity_management_endpoint
                   SET address_type = :addressType,
                       address = :address,
                       state = :state,
                       source = :source,
                       is_primary = :isPrimary,
                       updated_by = :updatedBy,
                       updated_at = :updatedAt
                 WHERE tenant_id = :tenantId
                   AND entity_id = :entityId
                   AND endpoint_id = :endpointId
                """,
                params(endpoint));
    }

    public int clearPrimaryFlags(
            UUID tenantId,
            UUID entityId,
            UUID exceptEndpointId,
            String updatedBy,
            java.time.Instant updatedAt
    ) {
        MapSqlParameterSource params = new MapSqlParameterSource()
                .addValue("tenantId", tenantId)
                .addValue("entityId", entityId)
                .addValue("exceptEndpointId", exceptEndpointId)
                .addValue("updatedBy", updatedBy)
                .addValue("updatedAt", JdbcTime.ts(updatedAt));
        String sql = """
                UPDATE platform.entity_management_endpoint
                   SET is_primary = FALSE,
                       updated_by = :updatedBy,
                       updated_at = :updatedAt
                 WHERE tenant_id = :tenantId
                   AND entity_id = :entityId
                   AND is_primary = TRUE
                """;
        if (exceptEndpointId != null) {
            sql += " AND endpoint_id <> :exceptEndpointId";
        }
        return jdbc.update(sql, params);
    }

    public List<EntityManagementEndpoint> listByEntity(UUID tenantId, UUID entityId) {
        return jdbc.query("""
                SELECT * FROM platform.entity_management_endpoint
                 WHERE tenant_id = :tenantId AND entity_id = :entityId
                 ORDER BY is_primary DESC, created_at ASC, endpoint_id ASC
                """,
                Map.of("tenantId", tenantId, "entityId", entityId),
                MAPPER);
    }

    public Optional<EntityManagementEndpoint> findByTenantEntityAndId(
            UUID tenantId, UUID entityId, UUID endpointId) {
        List<EntityManagementEndpoint> rows = jdbc.query("""
                SELECT * FROM platform.entity_management_endpoint
                 WHERE tenant_id = :tenantId
                   AND entity_id = :entityId
                   AND endpoint_id = :endpointId
                """,
                Map.of("tenantId", tenantId, "entityId", entityId, "endpointId", endpointId),
                MAPPER);
        return rows.stream().findFirst();
    }

    public Optional<EntityManagementEndpoint> findActivePrimary(UUID tenantId, UUID entityId) {
        List<EntityManagementEndpoint> rows = jdbc.query("""
                SELECT * FROM platform.entity_management_endpoint
                 WHERE tenant_id = :tenantId
                   AND entity_id = :entityId
                   AND is_primary = TRUE
                   AND state = 'ACTIVE'
                """,
                Map.of("tenantId", tenantId, "entityId", entityId),
                MAPPER);
        return rows.stream().findFirst();
    }

    private static MapSqlParameterSource params(EntityManagementEndpoint endpoint) {
        return new MapSqlParameterSource()
                .addValue("endpointId", endpoint.endpointId())
                .addValue("tenantId", endpoint.tenantId())
                .addValue("entityId", endpoint.entityId())
                .addValue("addressType", endpoint.addressType().name())
                .addValue("address", endpoint.address())
                .addValue("state", endpoint.state().name())
                .addValue("source", endpoint.source().name())
                .addValue("isPrimary", endpoint.primary())
                .addValue("createdBy", endpoint.createdBy())
                .addValue("createdAt", JdbcTime.ts(endpoint.createdAt()))
                .addValue("updatedBy", endpoint.updatedBy())
                .addValue("updatedAt", JdbcTime.ts(endpoint.updatedAt()));
    }
}
