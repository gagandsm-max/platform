package com.platform.core.persistence;

import com.platform.core.domain.inventory.EntityIdentifier;
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
public class EntityIdentifierRepository {

    /** Persisted write provenance; never inferred from the current identifier row. */
    public record HistoricalBinding(UUID identifierId, UUID entityId, long historyVersion,
            Instant validFrom, Instant validTo, String provenance) {}

    private static final RowMapper<EntityIdentifier> MAPPER = (rs, rowNum) -> new EntityIdentifier(
            rs.getObject("identifier_id", UUID.class),
            rs.getObject("tenant_id", UUID.class),
            rs.getObject("entity_id", UUID.class),
            rs.getString("identifier_type"),
            rs.getString("namespace"),
            rs.getString("identifier_value"),
            rs.getBigDecimal("confidence"),
            JdbcTime.instant(rs.getTimestamp("first_seen_at")),
            JdbcTime.instant(rs.getTimestamp("last_seen_at"))
    );

    private final NamedParameterJdbcTemplate jdbc;

    public EntityIdentifierRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public void insert(EntityIdentifier identifier) {
        jdbc.update("""
                INSERT INTO platform.entity_identifier (
                    identifier_id, tenant_id, entity_id, identifier_type, namespace,
                    identifier_value, confidence, first_seen_at, last_seen_at
                ) VALUES (
                    :identifierId, :tenantId, :entityId, :identifierType, :namespace,
                    :identifierValue, :confidence, :firstSeenAt, :lastSeenAt
                )
                """, params(identifier));
    }

    public Optional<EntityIdentifier> findByTenantAndId(UUID tenantId, UUID identifierId) {
        List<EntityIdentifier> rows = jdbc.query(
                "SELECT * FROM platform.entity_identifier WHERE tenant_id = :tenantId AND identifier_id = :identifierId",
                Map.of("tenantId", tenantId, "identifierId", identifierId),
                MAPPER
        );
        return rows.stream().findFirst();
    }

    public Optional<EntityIdentifier> findByResolutionKey(
            UUID tenantId, String namespace, String identifierType, String identifierValue
    ) {
        List<EntityIdentifier> rows = jdbc.query("""
                SELECT * FROM platform.entity_identifier
                 WHERE tenant_id = :tenantId
                   AND namespace = :namespace
                   AND identifier_type = :identifierType
                   AND identifier_value = :identifierValue
                """,
                Map.of(
                        "tenantId", tenantId,
                        "namespace", namespace,
                        "identifierType", identifierType,
                        "identifierValue", identifierValue
                ),
                MAPPER);
        return rows.stream().findFirst();
    }

    public Optional<HistoricalBinding> findHistoricalBinding(UUID tenantId, String namespace,
            String identifierType, String identifierValue, Instant lower, Instant upper) {
        if (lower == null || upper == null || upper.isBefore(lower)) return Optional.empty();
        List<HistoricalBinding> rows = jdbc.query("""
                SELECT entity_identifier_id, entity_id, history_version, valid_from, valid_to, provenance
                  FROM platform.entity_identifier_binding_history
                 WHERE tenant_id = :tenantId AND namespace = :namespace
                   AND identifier_type = :identifierType AND identifier_value = :identifierValue
                   AND valid_from <= :lower AND (valid_to IS NULL OR valid_to > :upper)
                   AND confidence = 1.0
                   AND provenance = 'ENTITY_IDENTIFIER_WRITE_V021'
                 ORDER BY history_version
                 LIMIT 2
                """, new MapSqlParameterSource()
                    .addValue("tenantId", tenantId).addValue("namespace", namespace)
                    .addValue("identifierType", identifierType).addValue("identifierValue", identifierValue)
                    .addValue("lower", JdbcTime.ts(lower)).addValue("upper", JdbcTime.ts(upper)),
                (rs, n) -> new HistoricalBinding(
                        rs.getObject("entity_identifier_id", UUID.class),
                        rs.getObject("entity_id", UUID.class), rs.getLong("history_version"),
                        JdbcTime.instant(rs.getTimestamp("valid_from")),
                        JdbcTime.instant(rs.getTimestamp("valid_to")), rs.getString("provenance")));
        return rows.size() == 1 ? Optional.of(rows.get(0)) : Optional.empty();
    }

    /** One tenant-scoped query for the bounded host evidence in one ingress record. */
    public boolean hasConflictingHostEvidence(UUID tenantId, UUID entityId, List<String> hostIds) {
        if (hostIds.isEmpty()) return false;
        return Boolean.TRUE.equals(jdbc.queryForObject("""
                SELECT EXISTS (SELECT 1 FROM platform.entity_identifier
                 WHERE tenant_id=:tenantId AND namespace='platform.agent' AND identifier_type='HOST_ID'
                   AND identifier_value IN (:hostIds) AND entity_id<>:entityId)
                """, Map.of("tenantId", tenantId, "entityId", entityId, "hostIds", hostIds), Boolean.class));
    }

    public List<EntityIdentifier> listByEntity(UUID tenantId, UUID entityId, int limit, long offset) {
        return jdbc.query("""
                SELECT * FROM platform.entity_identifier
                 WHERE tenant_id = :tenantId AND entity_id = :entityId
                 ORDER BY first_seen_at ASC, identifier_id ASC
                 LIMIT :limit OFFSET :offset
                """,
                Map.of("tenantId", tenantId, "entityId", entityId, "limit", limit, "offset", offset),
                MAPPER);
    }

    public long countByEntity(UUID tenantId, UUID entityId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.entity_identifier WHERE tenant_id = :tenantId AND entity_id = :entityId",
                Map.of("tenantId", tenantId, "entityId", entityId),
                Long.class
        );
        return count == null ? 0L : count;
    }

    public int delete(UUID tenantId, UUID entityId, UUID identifierId) {
        return jdbc.update("""
                DELETE FROM platform.entity_identifier
                 WHERE tenant_id = :tenantId AND entity_id = :entityId AND identifier_id = :identifierId
                """,
                Map.of("tenantId", tenantId, "entityId", entityId, "identifierId", identifierId));
    }

    private static MapSqlParameterSource params(EntityIdentifier identifier) {
        return new MapSqlParameterSource()
                .addValue("identifierId", identifier.identifierId())
                .addValue("tenantId", identifier.tenantId())
                .addValue("entityId", identifier.entityId())
                .addValue("identifierType", identifier.identifierType())
                .addValue("namespace", identifier.namespace())
                .addValue("identifierValue", identifier.identifierValue())
                .addValue("confidence", identifier.confidence())
                .addValue("firstSeenAt", JdbcTime.ts(identifier.firstSeenAt()))
                .addValue("lastSeenAt", JdbcTime.ts(identifier.lastSeenAt()));
    }
}
