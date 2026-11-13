package com.platform.core.persistence;

import com.platform.core.domain.inventory.EntityIdentifier;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Repository;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.TransactionDefinition;
import org.springframework.transaction.support.TransactionSynchronization;
import org.springframework.transaction.support.TransactionSynchronizationManager;
import org.springframework.transaction.support.TransactionTemplate;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

@Repository
public class EntityIdentifierRepository {

    private static final Logger log = LoggerFactory.getLogger(EntityIdentifierRepository.class);

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
    private final PlatformTransactionManager transactionManager;

    @Autowired
    public EntityIdentifierRepository(NamedParameterJdbcTemplate jdbc,
            PlatformTransactionManager transactionManager) {
        this.jdbc = jdbc;
        this.transactionManager = transactionManager;
    }

    /** Standalone repository queries in migration tests activate explicitly. */
    public EntityIdentifierRepository(NamedParameterJdbcTemplate jdbc) {
        this(jdbc, null);
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
        if (transactionManager != null) {
            Long version = jdbc.queryForObject("""
                    SELECT history_version FROM platform.entity_identifier_binding_history
                     WHERE tenant_id=:tenantId AND entity_identifier_id=:identifierId
                       AND entity_id=:entityId AND namespace=:namespace
                       AND identifier_type=:identifierType AND identifier_value=:identifierValue
                       AND activation_state='PENDING_COMMIT_VISIBILITY'
                    """, params(identifier), Long.class);
            if (version == null) throw new IllegalStateException("IDENTIFIER_HISTORY_PENDING_MISSING");
            Runnable activate = () -> {
                try {
                    TransactionTemplate transaction = new TransactionTemplate(transactionManager);
                    transaction.setPropagationBehavior(TransactionDefinition.PROPAGATION_REQUIRES_NEW);
                    transaction.setTimeout(5);
                    Boolean activated = transaction.execute(status -> jdbc.queryForObject("""
                            SELECT platform.activate_entity_identifier_binding_history(
                                :version, :tenantId, :identifierId, :entityId,
                                :namespace, :identifierType, :identifierValue)
                            """, params(identifier).addValue("version", version), Boolean.class));
                    if (!Boolean.TRUE.equals(activated)) {
                        log.warn("IDENTIFIER_HISTORY_ACTIVATION_UNPROVEN tenant_id={} identifier_id={} history_version={}",
                                identifier.tenantId(), identifier.identifierId(), version);
                    }
                } catch (RuntimeException ex) {
                    log.error("IDENTIFIER_HISTORY_ACTIVATION_FAILED tenant_id={} identifier_id={} history_version={} failure_class={}",
                            identifier.tenantId(), identifier.identifierId(), version,
                            ex.getClass().getSimpleName());
                }
            };
            if (TransactionSynchronizationManager.isSynchronizationActive()) {
                TransactionSynchronizationManager.registerSynchronization(new TransactionSynchronization() {
                    @Override public void afterCommit() { activate.run(); }
                });
            } else {
                activate.run();
            }
        }
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
                   AND activation_state = 'ACTIVE' AND valid_from IS NOT NULL
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
