package com.platform.core.sensorcontrol;

import org.springframework.dao.DuplicateKeyException;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;
import org.springframework.transaction.annotation.Transactional;

import java.sql.Timestamp;
import java.time.Instant;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;

@Repository
public class SensorControlRepository {

    private final NamedParameterJdbcTemplate jdbc;

    private static final RowMapper<SensorPrincipal> PRINCIPAL_MAPPER = (rs, rowNum) -> new SensorPrincipal(
            rs.getString("sensor_id"),
            rs.getObject("tenant_id", UUID.class),
            rs.getObject("site_id", UUID.class),
            SensorRole.valueOf(rs.getString("role")),
            SensorLifecycleStatus.valueOf(rs.getString("status")),
            rs.getBoolean("allow_all_tenant_entities"),
            toInstant(rs.getTimestamp("created_at")),
            toInstant(rs.getTimestamp("updated_at"))
    );

    private static final RowMapper<SensorCertificateCredential> CREDENTIAL_MAPPER = (rs, rowNum) ->
            new SensorCertificateCredential(
                    rs.getObject("credential_id", UUID.class),
                    rs.getObject("tenant_id", UUID.class),
                    rs.getString("sensor_id"),
                    rs.getString("fingerprint_sha256"),
                    rs.getString("spki_sha256"),
                    rs.getString("serial_number"),
                    rs.getString("issuer_dn"),
                    rs.getString("issuer_fingerprint_sha256"),
                    toInstant(rs.getTimestamp("not_before")),
                    toInstant(rs.getTimestamp("not_after")),
                    SensorCredentialStatus.valueOf(rs.getString("status")),
                    toInstant(rs.getTimestamp("activated_at")),
                    toInstant(rs.getTimestamp("revoked_at")),
                    toInstant(rs.getTimestamp("created_at")),
                    toInstant(rs.getTimestamp("updated_at"))
            );

    public SensorControlRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public void insertPrincipal(SensorPrincipal principal) {
        jdbc.update("""
                INSERT INTO platform.sensor
                    (sensor_id, tenant_id, site_id, role, status, allow_all_tenant_entities, created_at, updated_at)
                VALUES
                    (:sensorId, :tenantId, :siteId, :role, :status, :allowAll, :createdAt, :updatedAt)
                """, principalParams(principal));
    }

    public Optional<SensorPrincipal> findPrincipalById(String sensorId) {
        List<SensorPrincipal> rows = jdbc.query(
                "SELECT * FROM platform.sensor WHERE sensor_id = :sensorId",
                Map.of("sensorId", sensorId),
                PRINCIPAL_MAPPER
        );
        return rows.stream().findFirst();
    }

    public Optional<SensorPrincipal> findPrincipalByTenantAndId(UUID tenantId, String sensorId) {
        List<SensorPrincipal> rows = jdbc.query(
                "SELECT * FROM platform.sensor WHERE tenant_id = :tenantId AND sensor_id = :sensorId",
                Map.of("tenantId", tenantId, "sensorId", sensorId),
                PRINCIPAL_MAPPER
        );
        return rows.stream().findFirst();
    }

    public List<SensorPrincipal> listActivePrincipalsByTenant(UUID tenantId) {
        return jdbc.query("""
                SELECT * FROM platform.sensor
                 WHERE tenant_id = :tenantId AND status = 'ACTIVE'
                 ORDER BY sensor_id ASC
                """,
                Map.of("tenantId", tenantId),
                PRINCIPAL_MAPPER);
    }

    public void updatePrincipalStatus(String sensorId, SensorLifecycleStatus status, Instant updatedAt) {
        int updated = jdbc.update("""
                UPDATE platform.sensor
                   SET status = :status, updated_at = :updatedAt
                 WHERE sensor_id = :sensorId
                """,
                new MapSqlParameterSource()
                        .addValue("sensorId", sensorId)
                        .addValue("status", status.name())
                        .addValue("updatedAt", Timestamp.from(updatedAt)));
        if (updated == 0) {
            throw new SensorControlException(SensorControlException.ERR_SENSOR_NOT_FOUND, "Sensor not found");
        }
    }

    public void updateAllowAllTenantEntities(String sensorId, boolean allowAll, Instant updatedAt) {
        jdbc.update("""
                UPDATE platform.sensor
                   SET allow_all_tenant_entities = :allowAll, updated_at = :updatedAt
                 WHERE sensor_id = :sensorId
                """,
                new MapSqlParameterSource()
                        .addValue("sensorId", sensorId)
                        .addValue("allowAll", allowAll)
                        .addValue("updatedAt", Timestamp.from(updatedAt)));
    }

    public void insertCredential(SensorCertificateCredential credential) {
        try {
            jdbc.update("""
                    INSERT INTO platform.sensor_certificate_credential (
                        credential_id, tenant_id, sensor_id, fingerprint_sha256, spki_sha256,
                        serial_number, issuer_dn, issuer_fingerprint_sha256,
                        not_before, not_after, status, activated_at, revoked_at, created_at, updated_at
                    ) VALUES (
                        :credentialId, :tenantId, :sensorId, :fingerprint, :spki,
                        :serial, :issuerDn, :issuerFingerprint,
                        :notBefore, :notAfter, :status, :activatedAt, :revokedAt, :createdAt, :updatedAt
                    )
                    """, credentialParams(credential));
        } catch (DuplicateKeyException e) {
            throw new SensorControlException(
                    SensorControlException.ERR_SENSOR_CREDENTIAL_DUPLICATE,
                    "Certificate fingerprint already bound",
                    Map.of("fingerprint_sha256", credential.fingerprintSha256())
            );
        }
    }

    public Optional<SensorCertificateCredential> findCredentialByFingerprint(String fingerprintSha256) {
        List<SensorCertificateCredential> rows = jdbc.query(
                "SELECT * FROM platform.sensor_certificate_credential WHERE fingerprint_sha256 = :fp",
                Map.of("fp", fingerprintSha256),
                CREDENTIAL_MAPPER
        );
        return rows.stream().findFirst();
    }

    public List<SensorCertificateCredential> listActiveCredentialsForSensor(UUID tenantId, String sensorId) {
        return jdbc.query("""
                SELECT * FROM platform.sensor_certificate_credential
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId AND status = 'ACTIVE'
                 ORDER BY activated_at ASC, credential_id ASC
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("sensorId", sensorId),
                CREDENTIAL_MAPPER);
    }

    public void updateCredentialStatus(
            UUID credentialId,
            SensorCredentialStatus status,
            Instant revokedAt,
            Instant updatedAt
    ) {
        jdbc.update("""
                UPDATE platform.sensor_certificate_credential
                   SET status = :status, revoked_at = :revokedAt, updated_at = :updatedAt
                 WHERE credential_id = :credentialId
                """,
                new MapSqlParameterSource()
                        .addValue("credentialId", credentialId)
                        .addValue("status", status.name())
                        .addValue("revokedAt", revokedAt == null ? null : Timestamp.from(revokedAt))
                        .addValue("updatedAt", Timestamp.from(updatedAt)));
    }

    public Set<UUID> listAllowedEntityIds(UUID tenantId, String sensorId) {
        List<UUID> rows = jdbc.query("""
                SELECT entity_id FROM platform.sensor_allowed_entity
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                 ORDER BY entity_id ASC
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("sensorId", sensorId),
                (rs, rowNum) -> rs.getObject("entity_id", UUID.class));
        return new HashSet<>(rows);
    }

    @Transactional("transactionManager")
    public void replaceAllowedEntities(UUID tenantId, String sensorId, Set<UUID> entityIds, Instant createdAt) {
        jdbc.update("""
                DELETE FROM platform.sensor_allowed_entity
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId
                """,
                Map.of("tenantId", tenantId, "sensorId", sensorId));
        if (entityIds == null || entityIds.isEmpty()) {
            return;
        }
        for (UUID entityId : entityIds) {
            jdbc.update("""
                    INSERT INTO platform.sensor_allowed_entity (tenant_id, sensor_id, entity_id, created_at)
                    VALUES (:tenantId, :sensorId, :entityId, :createdAt)
                    """,
                    new MapSqlParameterSource()
                            .addValue("tenantId", tenantId)
                            .addValue("sensorId", sensorId)
                            .addValue("entityId", entityId)
                            .addValue("createdAt", Timestamp.from(createdAt)));
        }
    }

    public boolean tenantExists(UUID tenantId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.tenant WHERE tenant_id = :tenantId",
                Map.of("tenantId", tenantId),
                Long.class
        );
        return count != null && count > 0;
    }

    public boolean siteExists(UUID tenantId, UUID siteId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.site WHERE tenant_id = :tenantId AND site_id = :siteId",
                Map.of("tenantId", tenantId, "siteId", siteId),
                Long.class
        );
        return count != null && count > 0;
    }

    public boolean entityExists(UUID tenantId, UUID entityId) {
        Long count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.entity WHERE tenant_id = :tenantId AND entity_id = :entityId",
                Map.of("tenantId", tenantId, "entityId", entityId),
                Long.class
        );
        return count != null && count > 0;
    }

    private static MapSqlParameterSource principalParams(SensorPrincipal principal) {
        return new MapSqlParameterSource()
                .addValue("sensorId", principal.sensorId())
                .addValue("tenantId", principal.tenantId())
                .addValue("siteId", principal.siteId())
                .addValue("role", principal.role().name())
                .addValue("status", principal.status().name())
                .addValue("allowAll", principal.allowAllTenantEntities())
                .addValue("createdAt", Timestamp.from(principal.createdAt()))
                .addValue("updatedAt", Timestamp.from(principal.updatedAt()));
    }

    private static MapSqlParameterSource credentialParams(SensorCertificateCredential credential) {
        return new MapSqlParameterSource()
                .addValue("credentialId", credential.credentialId())
                .addValue("tenantId", credential.tenantId())
                .addValue("sensorId", credential.sensorId())
                .addValue("fingerprint", credential.fingerprintSha256())
                .addValue("spki", credential.spkiSha256())
                .addValue("serial", credential.serialNumber())
                .addValue("issuerDn", credential.issuerDn())
                .addValue("issuerFingerprint", credential.issuerFingerprintSha256())
                .addValue("notBefore", Timestamp.from(credential.notBefore()))
                .addValue("notAfter", Timestamp.from(credential.notAfter()))
                .addValue("status", credential.status().name())
                .addValue("activatedAt", Timestamp.from(credential.activatedAt()))
                .addValue("revokedAt", credential.revokedAt() == null ? null : Timestamp.from(credential.revokedAt()))
                .addValue("createdAt", Timestamp.from(credential.createdAt()))
                .addValue("updatedAt", Timestamp.from(credential.updatedAt()));
    }

    private static Instant toInstant(Timestamp timestamp) {
        return timestamp == null ? null : timestamp.toInstant();
    }
}
