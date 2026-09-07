package com.platform.core.persistence;

import com.platform.core.domain.inventory.EntityAttribute;
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
public class EntityAttributeRepository {

    private static final RowMapper<EntityAttribute> MAPPER = (rs, rowNum) -> {
        Object raw = rs.getObject("value_json");
        String json = raw == null ? null : String.valueOf(raw);
        return new EntityAttribute(
                rs.getObject("attribute_id", UUID.class),
                rs.getObject("tenant_id", UUID.class),
                rs.getObject("entity_id", UUID.class),
                rs.getString("schema_key"),
                json,
                rs.getString("source"),
                JdbcTime.instant(rs.getTimestamp("observed_at"))
        );
    };

    private final NamedParameterJdbcTemplate jdbc;

    public EntityAttributeRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public void upsert(EntityAttribute attribute) {
        jdbc.update("""
                INSERT INTO platform.entity_attribute (
                    attribute_id, tenant_id, entity_id, schema_key, value_json, source, observed_at
                ) VALUES (
                    :attributeId, :tenantId, :entityId, :schemaKey, CAST(:valueJson AS jsonb), :source, :observedAt
                )
                ON CONFLICT (tenant_id, entity_id, schema_key) DO UPDATE
                   SET value_json = EXCLUDED.value_json,
                       source = EXCLUDED.source,
                       observed_at = EXCLUDED.observed_at,
                       attribute_id = EXCLUDED.attribute_id
                """, params(attribute));
    }

    public Optional<EntityAttribute> findByTenantEntityKey(UUID tenantId, UUID entityId, String schemaKey) {
        List<EntityAttribute> rows = jdbc.query("""
                SELECT * FROM platform.entity_attribute
                 WHERE tenant_id = :tenantId AND entity_id = :entityId AND schema_key = :schemaKey
                """,
                Map.of("tenantId", tenantId, "entityId", entityId, "schemaKey", schemaKey),
                MAPPER);
        return rows.stream().findFirst();
    }

    public List<EntityAttribute> listByEntity(UUID tenantId, UUID entityId, int limit, long offset) {
        return jdbc.query("""
                SELECT * FROM platform.entity_attribute
                 WHERE tenant_id = :tenantId AND entity_id = :entityId
                 ORDER BY observed_at ASC, attribute_id ASC
                 LIMIT :limit OFFSET :offset
                """,
                Map.of("tenantId", tenantId, "entityId", entityId, "limit", limit, "offset", offset),
                MAPPER);
    }

    public long countByEntity(UUID tenantId, UUID entityId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.entity_attribute WHERE tenant_id = :tenantId AND entity_id = :entityId",
                Map.of("tenantId", tenantId, "entityId", entityId),
                Long.class
        );
        return count == null ? 0L : count;
    }

    private static MapSqlParameterSource params(EntityAttribute attribute) {
        return new MapSqlParameterSource()
                .addValue("attributeId", attribute.attributeId())
                .addValue("tenantId", attribute.tenantId())
                .addValue("entityId", attribute.entityId())
                .addValue("schemaKey", attribute.schemaKey())
                .addValue("valueJson", attribute.valueJson())
                .addValue("source", attribute.source())
                .addValue("observedAt", JdbcTime.ts(attribute.observedAt()));
    }
}
