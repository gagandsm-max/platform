package com.platform.core.sensorcontrol;

import org.springframework.dao.DuplicateKeyException;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;

import java.sql.Timestamp;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

@Repository
public class SensorEnrollmentAuthorityRepository {

    private static final RowMapper<SensorEnrollmentAuthority> MAPPER = (rs, rowNum) -> {
        String roleRaw = rs.getString("role");
        return new SensorEnrollmentAuthority(
                rs.getObject("authority_id", UUID.class),
                rs.getString("token_hash_sha256"),
                rs.getObject("tenant_id", UUID.class),
                rs.getString("deployment_id"),
                rs.getObject("site_id", UUID.class),
                roleRaw == null ? null : SensorRole.valueOf(roleRaw),
                toInstant(rs.getTimestamp("issued_at")),
                toInstant(rs.getTimestamp("expires_at")),
                toInstant(rs.getTimestamp("consumed_at")),
                toInstant(rs.getTimestamp("revoked_at")),
                toInstant(rs.getTimestamp("created_at")),
                toInstant(rs.getTimestamp("updated_at"))
        );
    };

    private final NamedParameterJdbcTemplate jdbc;

    public SensorEnrollmentAuthorityRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public void insert(SensorEnrollmentAuthority authority) {
        try {
            jdbc.update("""
                    INSERT INTO platform.sensor_enrollment_authority
                        (authority_id, token_hash_sha256, tenant_id, deployment_id, site_id, role,
                         issued_at, expires_at, consumed_at, revoked_at, created_at, updated_at)
                    VALUES
                        (:authorityId, :tokenHash, :tenantId, :deploymentId, :siteId, :role,
                         :issuedAt, :expiresAt, NULL, NULL, :createdAt, :updatedAt)
                    """, params(authority));
        } catch (DuplicateKeyException ex) {
            throw ex;
        }
    }

    public Optional<SensorEnrollmentAuthority> findById(UUID authorityId) {
        List<SensorEnrollmentAuthority> rows = jdbc.query(
                "SELECT * FROM platform.sensor_enrollment_authority WHERE authority_id = :authorityId",
                Map.of("authorityId", authorityId),
                MAPPER);
        return rows.stream().findFirst();
    }

    public Optional<SensorEnrollmentAuthority> findByTokenHash(String tokenHashSha256) {
        List<SensorEnrollmentAuthority> rows = jdbc.query(
                "SELECT * FROM platform.sensor_enrollment_authority WHERE token_hash_sha256 = :tokenHash",
                Map.of("tokenHash", tokenHashSha256),
                MAPPER);
        return rows.stream().findFirst();
    }

    /**
     * Atomic one-time consumption. Returns true only when exactly one row transitioned.
     */
    public boolean tryConsume(UUID authorityId, Instant now) {
        int updated = jdbc.update("""
                UPDATE platform.sensor_enrollment_authority
                   SET consumed_at = :now,
                       updated_at = :now
                 WHERE authority_id = :authorityId
                   AND consumed_at IS NULL
                   AND revoked_at IS NULL
                   AND expires_at > :now
                """,
                new MapSqlParameterSource()
                        .addValue("authorityId", authorityId)
                        .addValue("now", Timestamp.from(now)));
        return updated == 1;
    }

    /**
     * Atomic revoke of an active (not consumed, not already revoked) authority.
     * Returns true when exactly one row was revoked.
     */
    public boolean tryRevoke(UUID authorityId, Instant now) {
        int updated = jdbc.update("""
                UPDATE platform.sensor_enrollment_authority
                   SET revoked_at = :now,
                       updated_at = :now
                 WHERE authority_id = :authorityId
                   AND consumed_at IS NULL
                   AND revoked_at IS NULL
                """,
                new MapSqlParameterSource()
                        .addValue("authorityId", authorityId)
                        .addValue("now", Timestamp.from(now)));
        return updated == 1;
    }

    public boolean tokenHashExists(String tokenHashSha256) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_enrollment_authority WHERE token_hash_sha256 = :tokenHash",
                Map.of("tokenHash", tokenHashSha256),
                Long.class);
        return count != null && count > 0;
    }

    private static MapSqlParameterSource params(SensorEnrollmentAuthority authority) {
        return new MapSqlParameterSource()
                .addValue("authorityId", authority.authorityId())
                .addValue("tokenHash", authority.tokenHashSha256())
                .addValue("tenantId", authority.tenantId())
                .addValue("deploymentId", authority.deploymentId())
                .addValue("siteId", authority.siteId())
                .addValue("role", authority.role() == null ? null : authority.role().name())
                .addValue("issuedAt", Timestamp.from(authority.issuedAt()))
                .addValue("expiresAt", Timestamp.from(authority.expiresAt()))
                .addValue("createdAt", Timestamp.from(authority.createdAt()))
                .addValue("updatedAt", Timestamp.from(authority.updatedAt()));
    }

    private static Instant toInstant(Timestamp timestamp) {
        return timestamp == null ? null : timestamp.toInstant();
    }
}
