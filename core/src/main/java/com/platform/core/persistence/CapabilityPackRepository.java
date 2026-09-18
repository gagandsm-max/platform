package com.platform.core.persistence;

import com.platform.core.domain.capabilitypack.CapabilityPackActivation;
import com.platform.core.domain.capabilitypack.StoredCapabilityPack;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;

import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

/**
 * Tenant-scoped Capability Pack artifact and activation persistence.
 * Every query predicates {@code tenant_id}; no cross-tenant lookup helpers.
 */
@Repository
public class CapabilityPackRepository {

    private static final RowMapper<StoredCapabilityPack> PACK_MAPPER = (rs, rowNum) -> new StoredCapabilityPack(
            rs.getObject("tenant_id", UUID.class),
            rs.getString("pack_id"),
            rs.getString("pack_version"),
            rs.getString("schema_version"),
            rs.getString("pack_class"),
            rs.getString("name"),
            rs.getString("publisher_type"),
            rs.getString("publisher_name"),
            rs.getString("certification"),
            rs.getString("manifest_sha256"),
            rs.getBytes("manifest_bytes"),
            rs.getString("created_by"),
            JdbcTime.instant(rs.getTimestamp("created_at"))
    );

    private static final RowMapper<CapabilityPackActivation> ACTIVATION_MAPPER = (rs, rowNum) -> new CapabilityPackActivation(
            rs.getObject("activation_id", UUID.class),
            rs.getObject("tenant_id", UUID.class),
            rs.getString("pack_id"),
            rs.getString("pack_version"),
            rs.getLong("generation"),
            rs.getString("status"),
            rs.getString("activated_by"),
            JdbcTime.instant(rs.getTimestamp("activated_at")),
            rs.getString("revoked_by"),
            JdbcTime.instant(rs.getTimestamp("revoked_at")),
            rs.getString("correlation_id")
    );

    private final NamedParameterJdbcTemplate jdbc;

    public CapabilityPackRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public boolean tenantExists(UUID tenantId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.tenant WHERE tenant_id = :tenantId",
                Map.of("tenantId", tenantId),
                Long.class
        );
        return count != null && count > 0;
    }

    /**
     * Serializes pack activation mutations for one tenant for the life of the caller's transaction.
     */
    public boolean lockTenantForPackMutation(UUID tenantId) {
        List<UUID> rows = jdbc.query(
                "SELECT tenant_id FROM platform.tenant WHERE tenant_id = :tenantId FOR UPDATE",
                Map.of("tenantId", tenantId),
                (rs, rowNum) -> rs.getObject("tenant_id", UUID.class)
        );
        return !rows.isEmpty();
    }

    /**
     * Inserts a pack artifact if absent. Returns 1 when inserted, 0 when the identity already exists.
     */
    public int insertPackIfAbsent(StoredCapabilityPack pack) {
        return jdbc.update("""
                INSERT INTO platform.tenant_capability_pack
                    (tenant_id, pack_id, pack_version, schema_version, pack_class, name,
                     publisher_type, publisher_name, certification, manifest_sha256,
                     manifest_bytes, created_by, created_at)
                VALUES
                    (:tenantId, :packId, :packVersion, :schemaVersion, :packClass, :name,
                     :publisherType, :publisherName, :certification, :manifestSha256,
                     :manifestBytes, :createdBy, :createdAt)
                ON CONFLICT (tenant_id, pack_id, pack_version) DO NOTHING
                """, packParams(pack));
    }

    /**
     * Max historical activation generation for tenant+pack (includes REVOKED). 0 when none exist.
     * Callers must hold the tenant FOR UPDATE lock before relying on this for CAS allocation.
     */
    public long maxActivationGeneration(UUID tenantId, String packId) {
        Long max = jdbc.queryForObject("""
                SELECT COALESCE(MAX(generation), 0)
                  FROM platform.tenant_capability_pack_activation
                 WHERE tenant_id = :tenantId
                   AND pack_id = :packId
                """,
                Map.of("tenantId", tenantId, "packId", packId),
                Long.class);
        return max == null ? 0L : max;
    }

    public Optional<StoredCapabilityPack> findPack(UUID tenantId, String packId, String packVersion) {
        List<StoredCapabilityPack> rows = jdbc.query("""
                SELECT * FROM platform.tenant_capability_pack
                 WHERE tenant_id = :tenantId
                   AND pack_id = :packId
                   AND pack_version = :packVersion
                """,
                Map.of("tenantId", tenantId, "packId", packId, "packVersion", packVersion),
                PACK_MAPPER);
        return rows.stream().findFirst();
    }

    public List<StoredCapabilityPack> listPacks(UUID tenantId, int limit, long offset) {
        return jdbc.query("""
                SELECT * FROM platform.tenant_capability_pack
                 WHERE tenant_id = :tenantId
                 ORDER BY pack_id ASC, pack_version ASC
                 LIMIT :limit OFFSET :offset
                """,
                Map.of("tenantId", tenantId, "limit", limit, "offset", offset),
                PACK_MAPPER);
    }

    public long countPacks(UUID tenantId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.tenant_capability_pack WHERE tenant_id = :tenantId",
                Map.of("tenantId", tenantId),
                Long.class
        );
        return count == null ? 0L : count;
    }

    public List<CapabilityPackActivation> listActiveActivations(UUID tenantId) {
        return jdbc.query("""
                SELECT * FROM platform.tenant_capability_pack_activation
                 WHERE tenant_id = :tenantId
                   AND status = 'ACTIVE'
                 ORDER BY pack_id ASC
                """,
                Map.of("tenantId", tenantId),
                ACTIVATION_MAPPER);
    }

    public Optional<CapabilityPackActivation> findActiveActivation(UUID tenantId, String packId) {
        List<CapabilityPackActivation> rows = jdbc.query("""
                SELECT * FROM platform.tenant_capability_pack_activation
                 WHERE tenant_id = :tenantId
                   AND pack_id = :packId
                   AND status = 'ACTIVE'
                """,
                Map.of("tenantId", tenantId, "packId", packId),
                ACTIVATION_MAPPER);
        return rows.stream().findFirst();
    }

    public void insertActivation(CapabilityPackActivation activation) {
        jdbc.update("""
                INSERT INTO platform.tenant_capability_pack_activation
                    (activation_id, tenant_id, pack_id, pack_version, generation, status,
                     activated_by, activated_at, revoked_by, revoked_at, correlation_id)
                VALUES
                    (:activationId, :tenantId, :packId, :packVersion, :generation, :status,
                     :activatedBy, :activatedAt, :revokedBy, :revokedAt, :correlationId)
                """, activationParams(activation));
    }

    public int revokeActivation(
            UUID tenantId,
            UUID activationId,
            String revokedBy,
            java.time.Instant revokedAt
    ) {
        return jdbc.update("""
                UPDATE platform.tenant_capability_pack_activation
                   SET status = 'REVOKED',
                       revoked_by = :revokedBy,
                       revoked_at = :revokedAt
                 WHERE tenant_id = :tenantId
                   AND activation_id = :activationId
                   AND status = 'ACTIVE'
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("activationId", activationId)
                        .addValue("revokedBy", revokedBy)
                        .addValue("revokedAt", JdbcTime.ts(revokedAt)));
    }

    private static MapSqlParameterSource packParams(StoredCapabilityPack pack) {
        return new MapSqlParameterSource()
                .addValue("tenantId", pack.tenantId())
                .addValue("packId", pack.packId())
                .addValue("packVersion", pack.packVersion())
                .addValue("schemaVersion", pack.schemaVersion())
                .addValue("packClass", pack.packClass())
                .addValue("name", pack.name())
                .addValue("publisherType", pack.publisherType())
                .addValue("publisherName", pack.publisherName())
                .addValue("certification", pack.certification())
                .addValue("manifestSha256", pack.manifestSha256())
                .addValue("manifestBytes", pack.manifestBytes())
                .addValue("createdBy", pack.createdBy())
                .addValue("createdAt", JdbcTime.ts(pack.createdAt()));
    }

    private static MapSqlParameterSource activationParams(CapabilityPackActivation activation) {
        return new MapSqlParameterSource()
                .addValue("activationId", activation.activationId())
                .addValue("tenantId", activation.tenantId())
                .addValue("packId", activation.packId())
                .addValue("packVersion", activation.packVersion())
                .addValue("generation", activation.generation())
                .addValue("status", activation.status())
                .addValue("activatedBy", activation.activatedBy())
                .addValue("activatedAt", JdbcTime.ts(activation.activatedAt()))
                .addValue("revokedBy", activation.revokedBy())
                .addValue("revokedAt", activation.revokedAt() == null ? null : JdbcTime.ts(activation.revokedAt()))
                .addValue("correlationId", activation.correlationId());
    }
}
