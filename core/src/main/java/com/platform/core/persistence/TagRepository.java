package com.platform.core.persistence;

import com.platform.core.domain.inventory.Tag;
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
public class TagRepository {

    private static final RowMapper<Tag> MAPPER = (rs, rowNum) -> new Tag(
            rs.getObject("tag_id", UUID.class),
            rs.getObject("tenant_id", UUID.class),
            rs.getString("tag_key"),
            rs.getString("tag_value"),
            JdbcTime.instant(rs.getTimestamp("created_at"))
    );

    private final NamedParameterJdbcTemplate jdbc;

    public TagRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public void insert(Tag tag) {
        jdbc.update("""
                INSERT INTO platform.tag
                    (tag_id, tenant_id, tag_key, tag_value, created_at)
                VALUES
                    (:tagId, :tenantId, :tagKey, :tagValue, :createdAt)
                """, params(tag));
    }

    public Optional<Tag> findByTenantAndId(UUID tenantId, UUID tagId) {
        List<Tag> rows = jdbc.query(
                "SELECT * FROM platform.tag WHERE tenant_id = :tenantId AND tag_id = :tagId",
                Map.of("tenantId", tenantId, "tagId", tagId),
                MAPPER
        );
        return rows.stream().findFirst();
    }

    public List<Tag> listByTenant(UUID tenantId, int limit, int offset) {
        return jdbc.query("""
                SELECT * FROM platform.tag
                 WHERE tenant_id = :tenantId
                 ORDER BY created_at ASC, tag_id ASC
                 LIMIT :limit OFFSET :offset
                """,
                Map.of("tenantId", tenantId, "limit", limit, "offset", offset),
                MAPPER);
    }

    public long countByTenant(UUID tenantId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.tag WHERE tenant_id = :tenantId",
                Map.of("tenantId", tenantId),
                Long.class
        );
        return count == null ? 0L : count;
    }

    private static MapSqlParameterSource params(Tag tag) {
        return new MapSqlParameterSource()
                .addValue("tagId", tag.tagId())
                .addValue("tenantId", tag.tenantId())
                .addValue("tagKey", tag.tagKey())
                .addValue("tagValue", tag.tagValue())
                .addValue("createdAt", JdbcTime.ts(tag.createdAt()));
    }
}
