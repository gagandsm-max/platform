package com.platform.core.persistence;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.inventory.EntityManagementEndpoint;
import com.platform.core.domain.inventory.ManagementEndpointAddressType;
import com.platform.core.domain.inventory.ManagementEndpointSource;
import com.platform.core.domain.inventory.ManagementEndpointState;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.jdbc.datasource.ConnectionHolder;
import org.springframework.stereotype.Repository;
import org.springframework.transaction.support.TransactionSynchronizationManager;

import javax.sql.DataSource;
import java.sql.Connection;
import java.sql.SQLException;
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

    /**
     * Read and hold the exact same-tenant/same-entity endpoint with PostgreSQL
     * {@code FOR SHARE} until the caller commits or rolls back. Missing and
     * wrong-scope rows are equally empty. Never opens a replacement transaction.
     * Intended for Core poll-grant issuance snapshotting (Issue #229 G1a).
     */
    public Optional<EntityManagementEndpoint> findByTenantEntityAndIdForShare(
            UUID tenantId, UUID entityId, UUID endpointId) {
        requireActiveCallerTransaction("findByTenantEntityAndIdForShare");
        List<EntityManagementEndpoint> rows = jdbc.query("""
                SELECT * FROM platform.entity_management_endpoint
                 WHERE tenant_id = :tenantId
                   AND entity_id = :entityId
                   AND endpoint_id = :endpointId
                 FOR SHARE
                """,
                Map.of("tenantId", tenantId, "entityId", entityId, "endpointId", endpointId),
                MAPPER);
        return rows.stream().findFirst();
    }

    /**
     * Fail closed unless this repository's DataSource connection participates in the
     * caller's active PostgreSQL transaction and is non-autocommit.
     */
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

    private static DomainException transactionRequired(String helperName, String reason) {
        // Same stable code family as Sensor held-authority helpers; DomainException avoids
        // a persistence→sensorcontrol package dependency.
        return new DomainException(
                "ERR_SENSOR_STATE_INVALID",
                "Active PostgreSQL transaction required for " + helperName + "; " + reason);
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
