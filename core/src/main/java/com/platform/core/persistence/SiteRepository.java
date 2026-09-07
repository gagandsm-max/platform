package com.platform.core.persistence;

import com.platform.core.domain.inventory.Site;
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
public class SiteRepository {

    private static final RowMapper<Site> MAPPER = (rs, rowNum) -> new Site(
            rs.getObject("site_id", UUID.class),
            rs.getObject("tenant_id", UUID.class),
            rs.getObject("parent_site_id", UUID.class),
            rs.getString("name"),
            rs.getString("description"),
            rs.getString("timezone"),
            JdbcTime.instant(rs.getTimestamp("created_at")),
            JdbcTime.instant(rs.getTimestamp("updated_at"))
    );

    private final NamedParameterJdbcTemplate jdbc;

    public SiteRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public void insert(Site site) {
        jdbc.update("""
                INSERT INTO platform.site
                    (site_id, tenant_id, parent_site_id, name, description, timezone, created_at, updated_at)
                VALUES
                    (:siteId, :tenantId, :parentSiteId, :name, :description, :timezone, :createdAt, :updatedAt)
                """, params(site));
    }

    public void update(Site site) {
        jdbc.update("""
                UPDATE platform.site
                   SET parent_site_id = :parentSiteId,
                       name = :name,
                       description = :description,
                       timezone = :timezone,
                       updated_at = :updatedAt
                 WHERE tenant_id = :tenantId AND site_id = :siteId
                """, params(site));
    }

    public Optional<Site> findByTenantAndId(UUID tenantId, UUID siteId) {
        List<Site> rows = jdbc.query(
                "SELECT * FROM platform.site WHERE tenant_id = :tenantId AND site_id = :siteId",
                Map.of("tenantId", tenantId, "siteId", siteId),
                MAPPER
        );
        return rows.stream().findFirst();
    }

    public List<Site> listByTenant(UUID tenantId, int limit, int offset) {
        return jdbc.query("""
                SELECT * FROM platform.site
                 WHERE tenant_id = :tenantId
                 ORDER BY created_at ASC, site_id ASC
                 LIMIT :limit OFFSET :offset
                """,
                Map.of("tenantId", tenantId, "limit", limit, "offset", offset),
                MAPPER);
    }

    public long countByTenant(UUID tenantId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.site WHERE tenant_id = :tenantId",
                Map.of("tenantId", tenantId),
                Long.class
        );
        return count == null ? 0L : count;
    }

    private static MapSqlParameterSource params(Site site) {
        return new MapSqlParameterSource()
                .addValue("siteId", site.siteId())
                .addValue("tenantId", site.tenantId())
                .addValue("parentSiteId", site.parentSiteId())
                .addValue("name", site.name())
                .addValue("description", site.description())
                .addValue("timezone", site.timezone())
                .addValue("createdAt", JdbcTime.ts(site.createdAt()))
                .addValue("updatedAt", JdbcTime.ts(site.updatedAt()));
    }
}
