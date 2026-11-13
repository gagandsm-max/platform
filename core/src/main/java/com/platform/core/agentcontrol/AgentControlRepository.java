package com.platform.core.agentcontrol;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.stereotype.Repository;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.TransactionDefinition;
import org.springframework.transaction.support.TransactionSynchronization;
import org.springframework.transaction.support.TransactionSynchronizationManager;
import org.springframework.transaction.support.TransactionTemplate;

import java.sql.Timestamp;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

@Repository
public class AgentControlRepository {

    private static final Logger log = LoggerFactory.getLogger(AgentControlRepository.class);

    /** Persisted Agent↔entity write provenance; never inferred from the current binding row. */
    public record HistoricalAgentBinding(UUID entityId, String certificateIdentity, long historyVersion,
            String bindingState, long reinstallGeneration, Instant validFrom, Instant validTo,
            String provenance) {}

    private final NamedParameterJdbcTemplate jdbc;
    private final PlatformTransactionManager transactionManager;

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

    @Autowired
    public AgentControlRepository(NamedParameterJdbcTemplate jdbc,
            PlatformTransactionManager transactionManager) {
        this.jdbc = jdbc;
        this.transactionManager = transactionManager;
    }

    /** Standalone repository queries in migration/integration tests activate explicitly when needed. */
    public AgentControlRepository(NamedParameterJdbcTemplate jdbc) {
        this(jdbc, null);
    }

    /**
     * Core custody uses explicit active identity authority, never host evidence.
     * The Sensor authenticated the credential at admission. Do not re-evaluate its
     * current expiry/status here: accepted historical spool records must remain
     * drainable after certificate expiry/rotation. New admissions use the existing
     * AgentAuthorityQueryService credential checks.
     */
    public Optional<EntityAgentBinding> findActiveEntityBinding(UUID tenantId, String agentId) {
        return jdbc.query("""
                SELECT b.* FROM platform.entity_agent_binding b
                JOIN platform.agent a ON a.tenant_id=b.tenant_id AND a.agent_id=b.agent_id
                JOIN platform.agent_certificate_credential c
                  ON c.tenant_id=b.tenant_id AND c.agent_id=b.agent_id
                 AND c.fingerprint_sha256=b.certificate_identity
                WHERE b.tenant_id=:tenantId AND b.agent_id=:agentId
                  AND b.binding_state='ACTIVE' AND a.status='ACTIVE'
                """, Map.of("tenantId", tenantId, "agentId", agentId), (rs, rowNum) ->
                new EntityAgentBinding(rs.getObject("tenant_id", UUID.class),
                        rs.getObject("entity_id", UUID.class), rs.getString("agent_id"),
                        rs.getString("certificate_identity"), rs.getString("system_name"),
                        rs.getString("os_identity"), toInstant(rs.getTimestamp("first_enrolled_at")),
                        toInstant(rs.getTimestamp("last_heartbeat_at")), rs.getString("binding_state"),
                        rs.getLong("reinstall_generation"))).stream().findFirst();
    }

    public void insertEntityBinding(EntityAgentBinding binding) {
        try {
            jdbc.update("""
                    INSERT INTO platform.entity_agent_binding
                        (tenant_id, entity_id, agent_id, certificate_identity, system_name, os_identity,
                         first_enrolled_at, last_heartbeat_at, binding_state, reinstall_generation)
                    VALUES (:tenant, :entity, :agent, :certificate, :name, CAST(:os AS jsonb),
                            :first, :heartbeat, :state, :generation)
                    """, bindingParams(binding));
        } catch (DuplicateKeyException e) {
            throw new AgentControlException(
                    AgentControlException.ERR_AGENT_BINDING_IDENTITY_CONFLICT,
                    "Existing Agent, entity or certificate binding requires reconciliation");
        } catch (DataIntegrityViolationException e) {
            throw new AgentControlException(
                    AgentControlException.ERR_AGENT_BINDING_IDENTITY_CONFLICT,
                    "Existing Agent, entity or certificate binding requires reconciliation");
        }
        scheduleAgentBindingHistoryActivation(binding.tenantId(), binding.agentId(), binding.entityId(),
                binding.certificateIdentity(), binding.bindingState(), binding.reinstallGeneration());
    }

    public int rotateEntityBindingCertificate(UUID tenantId, String agentId, String oldFingerprint,
                                              String newFingerprint) {
        // Called under the same principal lock/transaction as credential rotation.
        // No entity, generation or approval-state change is permitted here.
        int updated = jdbc.update("""
                UPDATE platform.entity_agent_binding SET certificate_identity=:next
                 WHERE tenant_id=:tenant AND agent_id=:agent AND certificate_identity=:previous
                """, Map.of("tenant", tenantId, "agent", agentId,
                "previous", oldFingerprint, "next", newFingerprint));
        if (updated == 1) {
            List<EntityAgentBinding> current = jdbc.query("""
                    SELECT * FROM platform.entity_agent_binding
                     WHERE tenant_id=:tenant AND agent_id=:agent AND certificate_identity=:next
                    """, Map.of("tenant", tenantId, "agent", agentId, "next", newFingerprint),
                    (rs, rowNum) -> new EntityAgentBinding(
                            rs.getObject("tenant_id", UUID.class),
                            rs.getObject("entity_id", UUID.class),
                            rs.getString("agent_id"),
                            rs.getString("certificate_identity"),
                            rs.getString("system_name"),
                            rs.getString("os_identity"),
                            toInstant(rs.getTimestamp("first_enrolled_at")),
                            toInstant(rs.getTimestamp("last_heartbeat_at")),
                            rs.getString("binding_state"),
                            rs.getLong("reinstall_generation")));
            if (current.size() == 1) {
                EntityAgentBinding row = current.get(0);
                scheduleAgentBindingHistoryActivation(tenantId, agentId, row.entityId(),
                        newFingerprint, row.bindingState(), row.reinstallGeneration());
            }
        }
        return updated;
    }

    /**
     * Historical Agent↔entity authority covering the qualified acceptance interval.
     * HOST_ID history alone is insufficient; callers must still corroborate host evidence.
     */
    public Optional<HistoricalAgentBinding> findHistoricalAgentEntityBinding(
            UUID tenantId, String agentId, Instant lower, Instant upper) {
        if (tenantId == null || agentId == null || agentId.isBlank()
                || lower == null || upper == null || upper.isBefore(lower)) {
            return Optional.empty();
        }
        List<HistoricalAgentBinding> rows = jdbc.query("""
                SELECT entity_id, certificate_identity, history_version, binding_state,
                       reinstall_generation, valid_from, valid_to, provenance
                  FROM platform.entity_agent_binding_history
                 WHERE tenant_id = :tenantId AND agent_id = :agentId
                   AND activation_state = 'ACTIVE' AND valid_from IS NOT NULL
                   AND valid_from <= :lower AND (valid_to IS NULL OR valid_to > :upper)
                   AND binding_state = 'ACTIVE'
                   AND provenance = 'ENTITY_AGENT_BINDING_WRITE_V022'
                 ORDER BY history_version
                 LIMIT 2
                """, new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("agentId", agentId)
                        .addValue("lower", Timestamp.from(lower))
                        .addValue("upper", Timestamp.from(upper)),
                (rs, n) -> new HistoricalAgentBinding(
                        rs.getObject("entity_id", UUID.class),
                        rs.getString("certificate_identity"),
                        rs.getLong("history_version"),
                        rs.getString("binding_state"),
                        rs.getLong("reinstall_generation"),
                        toInstant(rs.getTimestamp("valid_from")),
                        toInstant(rs.getTimestamp("valid_to")),
                        rs.getString("provenance")));
        return rows.size() == 1 ? Optional.of(rows.get(0)) : Optional.empty();
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
     * Row-lock the agent principal so assign-after-revoke and credential rotate/revoke/bind
     * serialize when operating on the same principal. Returns the locked row so callers can
     * re-evaluate lifecycle state after acquiring the lock.
     */
    public AgentPrincipal lockPrincipalForUpdate(UUID tenantId, String agentId) {
        List<AgentPrincipal> rows = jdbc.query("""
                SELECT * FROM platform.agent
                 WHERE tenant_id = :tenantId AND agent_id = :agentId
                 FOR UPDATE
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("agentId", agentId),
                PRINCIPAL_MAPPER);
        if (rows.isEmpty()) {
            throw new AgentControlException(AgentControlException.ERR_AGENT_NOT_FOUND, "Agent not found");
        }
        return rows.get(0);
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

    /**
     * Historical assignment lookup for Agent-ingress replay: returns ACTIVE or REVOKED row
     * for the exact generation. Unknown generation → empty (fail-closed at caller).
     */
    public Optional<AgentSensorAssignment> findAssignmentByGeneration(
            UUID tenantId,
            String agentId,
            long generation
    ) {
        List<AgentSensorAssignment> rows = jdbc.query("""
                SELECT * FROM platform.agent_sensor_assignment
                 WHERE tenant_id = :tenantId
                   AND agent_id = :agentId
                   AND generation = :generation
                   AND status IN ('ACTIVE', 'REVOKED')
                """,
                new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("agentId", agentId)
                        .addValue("generation", generation),
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

    private static MapSqlParameterSource bindingParams(EntityAgentBinding binding) {
        return new MapSqlParameterSource()
                .addValue("tenant", binding.tenantId())
                .addValue("entity", binding.entityId())
                .addValue("agent", binding.agentId())
                .addValue("certificate", binding.certificateIdentity())
                .addValue("name", binding.systemName())
                .addValue("os", binding.osIdentity())
                .addValue("first", Timestamp.from(binding.firstEnrolledAt()))
                .addValue("heartbeat", Timestamp.from(binding.lastHeartbeatAt()))
                .addValue("state", binding.bindingState())
                .addValue("generation", binding.reinstallGeneration());
    }

    private void scheduleAgentBindingHistoryActivation(
            UUID tenantId, String agentId, UUID entityId, String certificateIdentity,
            String bindingState, long reinstallGeneration) {
        if (transactionManager == null) {
            return;
        }
        Long version = jdbc.queryForObject("""
                SELECT history_version FROM platform.entity_agent_binding_history
                 WHERE tenant_id=:tenantId AND agent_id=:agentId AND entity_id=:entityId
                   AND certificate_identity=:certificate AND binding_state=:state
                   AND reinstall_generation=:generation
                   AND activation_state='PENDING_COMMIT_VISIBILITY'
                """, new MapSqlParameterSource()
                        .addValue("tenantId", tenantId)
                        .addValue("agentId", agentId)
                        .addValue("entityId", entityId)
                        .addValue("certificate", certificateIdentity)
                        .addValue("state", bindingState)
                        .addValue("generation", reinstallGeneration), Long.class);
        if (version == null) {
            throw new IllegalStateException("AGENT_BINDING_HISTORY_PENDING_MISSING");
        }
        Runnable activate = () -> {
            try {
                TransactionTemplate transaction = new TransactionTemplate(transactionManager);
                transaction.setPropagationBehavior(TransactionDefinition.PROPAGATION_REQUIRES_NEW);
                transaction.setTimeout(5);
                Boolean activated = transaction.execute(status -> jdbc.queryForObject("""
                        SELECT platform.activate_entity_agent_binding_history(
                            :version, :tenantId, CAST(:agentId AS varchar), :entityId,
                            CAST(:certificate AS varchar), CAST(:state AS varchar), :generation)
                        """, new MapSqlParameterSource()
                                .addValue("version", version)
                                .addValue("tenantId", tenantId)
                                .addValue("agentId", agentId)
                                .addValue("entityId", entityId)
                                .addValue("certificate", certificateIdentity)
                                .addValue("state", bindingState)
                                .addValue("generation", reinstallGeneration), Boolean.class));
                if (!Boolean.TRUE.equals(activated)) {
                    log.warn("AGENT_BINDING_HISTORY_ACTIVATION_UNPROVEN tenant_id={} agent_id={} history_version={}",
                            tenantId, agentId, version);
                }
            } catch (RuntimeException ex) {
                log.error("AGENT_BINDING_HISTORY_ACTIVATION_FAILED tenant_id={} agent_id={} history_version={} failure_class={}",
                        tenantId, agentId, version, ex.getClass().getSimpleName());
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

    private static Instant toInstant(Timestamp timestamp) {
        return timestamp == null ? null : timestamp.toInstant();
    }
}
