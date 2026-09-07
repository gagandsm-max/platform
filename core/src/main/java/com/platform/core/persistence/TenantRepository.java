package com.platform.core.persistence;

import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantStatus;
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
public class TenantRepository {

    private static final RowMapper<Tenant> MAPPER = (rs, rowNum) -> new Tenant(
            rs.getObject("tenant_id", UUID.class),
            rs.getString("name"),
            TenantMode.valueOf(rs.getString("mode")),
            TenantStatus.valueOf(rs.getString("status")),
            rs.getString("timezone"),
            JdbcTime.instant(rs.getTimestamp("created_at")),
            JdbcTime.instant(rs.getTimestamp("updated_at"))
    );

    private final NamedParameterJdbcTemplate jdbc;

    public TenantRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public void insert(Tenant tenant) {
        jdbc.update("""
                INSERT INTO platform.tenant
                    (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES
                    (:tenantId, :name, :mode, :status, :timezone, :createdAt, :updatedAt)
                """, params(tenant));
    }

    public void update(Tenant tenant) {
        jdbc.update("""
                UPDATE platform.tenant
                   SET name = :name,
                       mode = :mode,
                       status = :status,
                       timezone = :timezone,
                       updated_at = :updatedAt
                 WHERE tenant_id = :tenantId
                """, params(tenant));
    }

    public Optional<Tenant> findById(UUID tenantId) {
        List<Tenant> rows = jdbc.query(
                "SELECT * FROM platform.tenant WHERE tenant_id = :tenantId",
                Map.of("tenantId", tenantId),
                MAPPER
        );
        return rows.stream().findFirst();
    }

    public List<Tenant> list(int limit, int offset) {
        return jdbc.query("""
                SELECT * FROM platform.tenant
                 ORDER BY created_at ASC, tenant_id ASC
                 LIMIT :limit OFFSET :offset
                """,
                Map.of("limit", limit, "offset", offset),
                MAPPER);
    }

    public long count() {
        Long count = jdbc.queryForObject("SELECT COUNT(*) FROM platform.tenant", Map.of(), Long.class);
        return count == null ? 0L : count;
    }

    public boolean exists(UUID tenantId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.tenant WHERE tenant_id = :tenantId",
                Map.of("tenantId", tenantId),
                Long.class
        );
        return count != null && count > 0;
    }

    private static MapSqlParameterSource params(Tenant tenant) {
        return new MapSqlParameterSource()
                .addValue("tenantId", tenant.tenantId())
                .addValue("name", tenant.name())
                .addValue("mode", tenant.mode().name())
                .addValue("status", tenant.status().name())
                .addValue("timezone", tenant.timezone())
                .addValue("createdAt", JdbcTime.ts(tenant.createdAt()))
                .addValue("updatedAt", JdbcTime.ts(tenant.updatedAt()));
    }
}
