package com.platform.core.agentcontrol;

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
public class AgentControlRepository {

    private final NamedParameterJdbcTemplate jdbc;

    private static final RowMapper<AgentPrincipal> PRINCIPAL_MAPPER = (rs, rowNum) -> new AgentPrincipal(
            rs.getString("agent_id"),
            rs.getObject("tenant_id", UUID.class),
            rs.getObject("site_id", UUID.class),
            AgentLifecycleStatus.valueOf(rs.getString("status")),
            toInstant(rs.getTimestamp("created_at")),
            toInstant(rs.getTimestamp("updated_at"))
    );

    private static final RowMapper<AgentCertificateCredential> CREDENTIAL_MAPPER = (rs, rowNum) ->
            new AgentCertificateCredential(
                    rs.getObject("credential_id", UUID.class),
                    rs.getObject("tenant_id", UUID.class),
                    rs.getString("agent_id"),
                    rs.getString("fingerprint_sha256"),
                    rs.getString("spki_sha256"),
                    rs.getString("serial_number"),
                    rs.getString("issuer_dn"),
                    rs.getString("issuer_fingerprint_sha256"),
                    toInstant(rs.getTimestamp("not_before")),
                    toInstant(rs.getTimestamp("not_after")),
                    AgentCredentialStatus.valueOf(rs.getString("status")),
                    toInstant(rs.getTimestamp("activated_at")),
                    toInstant(rs.getTimestamp("revoked_at")),
                    toInstant(rs.getTimestamp("created_at")),
                    toInstant(rs.getTimestamp("updated_at"))
            );

    private static final RowMapper<AgentSensorAssignment> ASSIGNMENT_MAPPER = (rs, rowNum) ->
            new AgentSensorAssignment(
                    rs.getObject("assignment_id", UUID.class),
                    rs.getObject("tenant_id", UUID.class),
                    rs.getString("agent_id"),
                    rs.getString("sensor_id"),
                    rs.getLong("generation"),
                    AgentAssignmentStatus.valueOf(rs.getString("status")),
                    toInstant(rs.getTimestamp("created_at")),
                    toInstant(rs.getTimestamp("updated_at")),
                    toInstant(rs.getTimestamp("revoked_at"))
            );

    public AgentControlRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    public void insertPrincipal(AgentPrincipal principal) {
        jdbc.update("""
                INSERT INTO platform.agent
                    (agent_id, tenant_id, site_id, status, created_at, updated_at)
                VALUES
                    (:agentId, :tenantId, :siteId, :status, :createdAt, :updatedAt)
                """, principalParams(principal));
    }

    public Optional<AgentPrincipal> findPrincipalById(String agentId) {
        List<AgentPrincipal> rows = jdbc.query(
                "SELECT * FROM platform.agent WHERE agent_id = :agentId",
                Map.of("agentId", agentId),
                PRINCIPAL_MAPPER
        );
        return rows.stream().findFirst();
    }

    public Optional<AgentPrincipal> findPrincipalByTenantAndId(UUID tenantId, String agentId) {
        List<AgentPrincipal> rows = jdbc.query(
                "SELECT * FROM platform.agent WHERE tenant_id = :tenantId AND agent_id = :agentId",
                Map.of("tenantId", tenantId, "agentId", agentId),
                PRINCIPAL_MAPPER
        );
        return rows.stream().findFirst();
    }

    public void updatePrincipalStatus(String agentId, AgentLifecycleStatus status, Instant updatedAt) {
        int updated = jdbc.update("""
                UPDATE platform.agent
                   SET status = :status, updated_at = :updatedAt
                 WHERE agent_id = :agentId
                """,
                new MapSqlParameterSource()
                        .addValue("agentId", agentId)
                        .addValue("status", status.name())
                        .addValue("updatedAt", Timestamp.from(updatedAt)));
        if (updated == 0) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_NOT_FOUND, "Agent not found");
        }
    }

    public void insertCredential(AgentCertificateCredential credential) {
        try {
            jdbc.update("""
                    INSERT INTO platform.agent_certificate_credential (
                        credential_id, tenant_id, agent_id, fingerprint_sha256, spki_sha256,
                        serial_number, issuer_dn, issuer_fingerprint_sha256,
                        not_before, not_after, status, activated_at, revoked_at, created_at, updated_at
                    ) VALUES (
                        :credentialId, :tenantId, :agentId, :fingerprint, :spki,
                        :serial, :issuerDn, :issuerFingerprint,
                        :notBefore, :notAfter, :status, :activatedAt, :revokedAt, :createdAt, :updatedAt
                    )
                    """, credentialParams(credential));
        } catch (DuplicateKeyException e) {
            throw new AgentControlException(
                    AgentControlException.ERR_AGENT_CREDENTIAL_DUPLICATE,
                    "Certificate fingerprint already bound",
                    Map.of("fingerprint_sha256", credential.fingerprintSha256())
            );
        }
    }

    public Optional<AgentCertificateCredential> findCredentialByFingerprint(String fingerprintSha256) {
        List<AgentCertificateCredential> rows = jdbc.query(
                "SELECT * FROM platform.agent_certificate_credential WHERE fingerprint_sha256 = :fp",
                Map.of("fp", fingerprintSha256),
                CREDENTIAL_MAPPER
        );
        return rows.stream().findFirst();
    }

    public List<AgentCertificateCredential> listActiveCredentialsForAgent(UUID tenantId, String agentId) {
        return jdbc.query("""
                SELECT * FROM platform.agent_certificate_credential
                 WHERE tenant_id = :tenantId AND agent_id = :agentId AND status = 'ACTIVE'
                 ORDER BY activated_at ASC, credential_id ASC
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("agentId", agentId),
                CREDENTIAL_MAPPER);
    }

    public void updateCredentialStatus(
            UUID credentialId,
            AgentCredentialStatus status,
            Instant revokedAt,
            Instant updatedAt
    ) {
        jdbc.update("""
                UPDATE platform.agent_certificate_credential
                   SET status = :status, revoked_at = :revokedAt, updated_at = :updatedAt
                 WHERE credential_id = :credentialId
                """,
                new MapSqlParameterSource()
                        .addValue("credentialId", credentialId)
                        .addValue("status", status.name())
                        .addValue("revokedAt", revokedAt == null ? null : Timestamp.from(revokedAt))
                        .addValue("updatedAt", Timestamp.from(updatedAt)));
    }

    /**
     * Locks the ACTIVE assignment row for the agent (if any) within the current transaction.
     */
    public Optional<AgentSensorAssignment> findActiveAssignmentForAgentForUpdate(UUID tenantId, String agentId) {
        List<AgentSensorAssignment> rows = jdbc.query("""
                SELECT * FROM platform.agent_sensor_assignment
                 WHERE tenant_id = :tenantId AND agent_id = :agentId AND status = 'ACTIVE'
                 FOR UPDATE
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("agentId", agentId),
                ASSIGNMENT_MAPPER);
        return rows.stream().findFirst();
    }

    /**
     * Highest assignment generation ever recorded for the agent (including REVOKED), or 0 if none.
     */
    public long maxGenerationForAgent(UUID tenantId, String agentId) {
        Long max = jdbc.queryForObject("""
                SELECT COALESCE(MAX(generation), 0)
                  FROM platform.agent_sensor_assignment
                 WHERE tenant_id = :tenantId AND agent_id = :agentId
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("agentId", agentId),
                Long.class);
        return max == null ? 0L : max;
    }

    /**
     * Row-lock the agent principal so assign-after-revoke serializes when no ACTIVE assignment exists.
     */
    public void lockPrincipalForUpdate(UUID tenantId, String agentId) {
        List<String> rows = jdbc.query("""
                SELECT agent_id FROM platform.agent
                 WHERE tenant_id = :tenantId AND agent_id = :agentId
                 FOR UPDATE
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("agentId", agentId),
                (rs, rowNum) -> rs.getString("agent_id"));
        if (rows.isEmpty()) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_NOT_FOUND, "Agent not found");
        }
    }

    public Optional<AgentSensorAssignment> findActiveAssignmentForAgent(UUID tenantId, String agentId) {
        List<AgentSensorAssignment> rows = jdbc.query("""
                SELECT * FROM platform.agent_sensor_assignment
                 WHERE tenant_id = :tenantId AND agent_id = :agentId AND status = 'ACTIVE'
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("agentId", agentId),
                ASSIGNMENT_MAPPER);
        return rows.stream().findFirst();
    }

    public List<AgentSensorAssignment> findActiveAssignmentsForSensor(UUID tenantId, String sensorId) {
        return jdbc.query("""
                SELECT * FROM platform.agent_sensor_assignment
                 WHERE tenant_id = :tenantId AND sensor_id = :sensorId AND status = 'ACTIVE'
                 ORDER BY agent_id ASC, generation ASC
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("sensorId", sensorId),
                ASSIGNMENT_MAPPER);
    }

    public void insertAssignment(AgentSensorAssignment assignment) {
        try {
            jdbc.update("""
                    INSERT INTO platform.agent_sensor_assignment (
                        assignment_id, tenant_id, agent_id, sensor_id, generation,
                        status, created_at, updated_at, revoked_at
                    ) VALUES (
                        :assignmentId, :tenantId, :agentId, :sensorId, :generation,
                        :status, :createdAt, :updatedAt, :revokedAt
                    )
                    """, assignmentParams(assignment));
        } catch (DuplicateKeyException e) {
            throw new AgentControlException(
                    AgentControlException.ERR_AGENT_ASSIGNMENT_CONFLICT,
                    "Active Agent assignment conflict"
            );
        }
    }

    /**
     * Optimistic CAS revoke of the ACTIVE assignment at {@code expectedGeneration}.
     *
     * @return number of rows updated (1 = success, 0 = conflict/stale)
     */
    public int revokeActiveAssignmentCas(
            UUID tenantId,
            String agentId,
            long expectedGeneration,
            Instant revokedAt,
            Instant updatedAt
    ) {
        return jdbc.update("""
                UPDATE platform.agent_sensor_assignment
                   SET status = 'REVOKED', revoked_at = :revokedAt, updated_at = :updatedAt
                 WHERE tenant_id = :tenantId
                   AND agent_id = :agentId
                   AND status = 'ACTIVE'
                   AND generation = :expectedGeneration
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("agentId", agentId)
                        .addValue("expectedGeneration", expectedGeneration)
                        .addValue("revokedAt", Timestamp.from(revokedAt))
                        .addValue("updatedAt", Timestamp.from(updatedAt)));
    }

    public void revokeAssignmentById(UUID assignmentId, Instant revokedAt, Instant updatedAt) {
        int updated = jdbc.update("""
                UPDATE platform.agent_sensor_assignment
                   SET status = 'REVOKED', revoked_at = :revokedAt, updated_at = :updatedAt
                 WHERE assignment_id = :assignmentId AND status = 'ACTIVE'
                """,
                new MapSqlParameterSource()
                        .addValue("assignmentId", assignmentId)
                        .addValue("revokedAt", Timestamp.from(revokedAt))
                        .addValue("updatedAt", Timestamp.from(updatedAt)));
        if (updated == 0) {
            throw new AgentControlException(
                    AgentControlException.ERR_AGENT_ASSIGNMENT_NOT_FOUND,
                    "Active assignment not found");
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

    private static MapSqlParameterSource principalParams(AgentPrincipal principal) {
        return new MapSqlParameterSource()
                .addValue("agentId", principal.agentId())
                .addValue("tenantId", principal.tenantId())
                .addValue("siteId", principal.siteId())
                .addValue("status", principal.status().name())
                .addValue("createdAt", Timestamp.from(principal.createdAt()))
                .addValue("updatedAt", Timestamp.from(principal.updatedAt()));
    }

    private static MapSqlParameterSource credentialParams(AgentCertificateCredential credential) {
        return new MapSqlParameterSource()
                .addValue("credentialId", credential.credentialId())
                .addValue("tenantId", credential.tenantId())
                .addValue("agentId", credential.agentId())
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

    private static MapSqlParameterSource assignmentParams(AgentSensorAssignment assignment) {
        return new MapSqlParameterSource()
                .addValue("assignmentId", assignment.assignmentId())
                .addValue("tenantId", assignment.tenantId())
                .addValue("agentId", assignment.agentId())
                .addValue("sensorId", assignment.sensorId())
                .addValue("generation", assignment.generation())
                .addValue("status", assignment.status().name())
                .addValue("createdAt", Timestamp.from(assignment.createdAt()))
                .addValue("updatedAt", Timestamp.from(assignment.updatedAt()))
                .addValue("revokedAt",
                        assignment.revokedAt() == null ? null : Timestamp.from(assignment.revokedAt()));
    }

    private static Instant toInstant(Timestamp timestamp) {
        return timestamp == null ? null : timestamp.toInstant();
    }
}
