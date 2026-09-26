package com.platform.core.persistence;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.api.PageResult;
import com.platform.core.domain.discovery.CandidateReviewStatus;
import com.platform.core.domain.discovery.DiscoveryCandidate;
import com.platform.core.domain.discovery.DiscoveryConfidence;
import com.platform.core.domain.discovery.DiscoveryEvidence;
import com.platform.core.domain.discovery.DiscoveryMethod;
import com.platform.core.domain.discovery.DiscoveryRequest;
import com.platform.core.domain.discovery.DiscoveryStatus;
import com.platform.core.domain.discovery.DiscoveryTarget;
import com.platform.core.domain.discovery.EvidenceStatus;
import com.platform.core.domain.discovery.ReachabilityKind;
import com.platform.core.domain.discovery.TargetType;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.jdbc.core.namedparam.SqlParameterSource;
import org.springframework.stereotype.Repository;
import org.springframework.transaction.annotation.Transactional;

import java.sql.Timestamp;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

@Repository
public class DiscoveryRepository {

    private final NamedParameterJdbcTemplate jdbc;
    private final ObjectMapper mapper;

    public record AssignedRequest(DiscoveryRequest request, long assignmentVersion) {}

    public DiscoveryRepository(NamedParameterJdbcTemplate jdbc) {
        this.jdbc = jdbc;
        this.mapper = new ObjectMapper();
    }

    private final RowMapper<DiscoveryRequest> REQUEST_MAPPER = (rs, rowNum) -> {
        List<DiscoveryMethod> methods = parseJsonList(rs.getString("requested_methods"), DiscoveryMethod.class);
        Map<String, Object> summary = parseJsonMap(rs.getString("summary_json"));
        return new DiscoveryRequest(
                rs.getObject("request_id", UUID.class),
                rs.getObject("tenant_id", UUID.class),
                rs.getString("name"),
                DiscoveryStatus.valueOf(rs.getString("status")),
                rs.getString("correlation_id"),
                rs.getString("idempotency_key"),
                rs.getObject("site_id", UUID.class),
                rs.getObject("sensor_pool_id", UUID.class),
                rs.getString("assigned_sensor_id"),
                methods,
                rs.getString("credential_profile_ref"),
                rs.getInt("snmp_port"),
                rs.getInt("timeout_ms"),
                rs.getInt("max_retries"),
                rs.getString("created_by"),
                JdbcTime.instant(rs.getTimestamp("created_at")),
                JdbcTime.instant(rs.getTimestamp("started_at")),
                JdbcTime.instant(rs.getTimestamp("completed_at")),
                summary,
                rs.getString("failure_reason")
        );
    };

    private final RowMapper<AssignedRequest> ASSIGNED_REQUEST_MAPPER = (rs, rowNum) ->
            new AssignedRequest(REQUEST_MAPPER.mapRow(rs, rowNum), rs.getLong("assignment_version"));

    private final RowMapper<DiscoveryTarget> TARGET_MAPPER = (rs, rowNum) -> new DiscoveryTarget(
            rs.getObject("target_id", UUID.class), rs.getObject("tenant_id", UUID.class),
            rs.getObject("request_id", UUID.class), rs.getString("target_address"),
            TargetType.valueOf(rs.getString("target_type")), rs.getString("status"),
            JdbcTime.instant(rs.getTimestamp("created_at")), JdbcTime.instant(rs.getTimestamp("updated_at"))
    );

    private final RowMapper<DiscoveryEvidence> EVIDENCE_MAPPER = (rs, rowNum) -> new DiscoveryEvidence(
            rs.getObject("evidence_id", UUID.class), rs.getObject("tenant_id", UUID.class),
            rs.getObject("request_id", UUID.class), rs.getObject("target_id", UUID.class),
            rs.getString("sensor_id"), DiscoveryMethod.valueOf(rs.getString("protocol")), rs.getString("method"),
            EvidenceStatus.valueOf(rs.getString("status")), ReachabilityKind.valueOf(rs.getString("reachability_kind")),
            rs.getString("technical_code"), parseJsonMap(rs.getString("attributes_json")),
            parseJsonMap(rs.getString("raw_evidence_json")), rs.getLong("duration_ms"),
            JdbcTime.instant(rs.getTimestamp("observed_at"))
    );

    private final RowMapper<DiscoveryCandidate> CANDIDATE_MAPPER = (rs, rowNum) -> new DiscoveryCandidate(
            rs.getObject("candidate_id", UUID.class), rs.getObject("tenant_id", UUID.class),
            rs.getObject("request_id", UUID.class), rs.getString("target_address"), rs.getString("discovered_name"),
            rs.getString("sys_object_id"), rs.getString("sys_descr"),
            parseJsonList(rs.getString("evidenced_protocols"), DiscoveryMethod.class),
            parseJsonMap(rs.getString("attributes_json")), DiscoveryConfidence.valueOf(rs.getString("confidence")),
            CandidateReviewStatus.valueOf(rs.getString("review_status")), rs.getBoolean("conflict_detected"),
            rs.getString("conflict_details"), rs.getObject("site_id", UUID.class),
            JdbcTime.instant(rs.getTimestamp("first_seen_at")), JdbcTime.instant(rs.getTimestamp("last_seen_at")),
            JdbcTime.instant(rs.getTimestamp("created_at")), JdbcTime.instant(rs.getTimestamp("updated_at"))
    );

    @Transactional("transactionManager")
    public void createRequest(DiscoveryRequest request, List<DiscoveryTarget> targets) {
        String methodsJson = toJson(request.requestedMethods().stream().map(Enum::name).toList());
        String summaryJson = toJson(request.summary() == null ? Map.of() : request.summary());
        MapSqlParameterSource reqSource = new MapSqlParameterSource()
                .addValue("requestId", request.requestId()).addValue("tenantId", request.tenantId())
                .addValue("name", request.name()).addValue("status", request.status().name())
                .addValue("correlationId", request.correlationId()).addValue("idempotencyKey", request.idempotencyKey())
                .addValue("siteId", request.siteId()).addValue("sensorPoolId", request.sensorPoolId())
                .addValue("assignedSensorId", request.assignedSensorId()).addValue("requestedMethods", methodsJson)
                .addValue("credentialProfileRef", request.credentialProfileRef()).addValue("snmpPort", request.snmpPort())
                .addValue("timeoutMs", request.timeoutMs()).addValue("maxRetries", request.maxRetries())
                .addValue("createdBy", request.createdBy()).addValue("createdAt", Timestamp.from(request.createdAt()))
                .addValue("startedAt", request.startedAt() == null ? null : Timestamp.from(request.startedAt()))
                .addValue("completedAt", request.completedAt() == null ? null : Timestamp.from(request.completedAt()))
                .addValue("summaryJson", summaryJson).addValue("failureReason", request.failureReason());
        jdbc.update("""
                INSERT INTO platform.discovery_request (
                    request_id, tenant_id, name, status, correlation_id, idempotency_key,
                    site_id, sensor_pool_id, assigned_sensor_id, assignment_version, requested_methods, credential_profile_ref,
                    snmp_port, timeout_ms, max_retries, created_by, created_at, started_at, completed_at,
                    summary_json, failure_reason
                ) VALUES (
                    :requestId, :tenantId, :name, :status, :correlationId, :idempotencyKey,
                    :siteId, :sensorPoolId, :assignedSensorId, 0, CAST(:requestedMethods AS jsonb), :credentialProfileRef,
                    :snmpPort, :timeoutMs, :maxRetries, :createdBy, :createdAt, :startedAt, :completedAt,
                    CAST(:summaryJson AS jsonb), :failureReason
                )
                """, reqSource);

        if (targets != null && !targets.isEmpty()) {
            SqlParameterSource[] batchSources = new SqlParameterSource[targets.size()];
            for (int i = 0; i < targets.size(); i++) {
                DiscoveryTarget t = targets.get(i);
                batchSources[i] = new MapSqlParameterSource()
                        .addValue("targetId", t.targetId()).addValue("tenantId", t.tenantId())
                        .addValue("requestId", t.requestId()).addValue("targetAddress", t.targetAddress())
                        .addValue("targetType", t.targetType().name()).addValue("status", t.status())
                        .addValue("createdAt", Timestamp.from(t.createdAt())).addValue("updatedAt", Timestamp.from(t.updatedAt()));
            }
            jdbc.batchUpdate("""
                    INSERT INTO platform.discovery_target (
                        target_id, tenant_id, request_id, target_address, target_type, status, created_at, updated_at
                    ) VALUES (:targetId, :tenantId, :requestId, :targetAddress, :targetType, :status, :createdAt, :updatedAt)
                    """, batchSources);
        }
    }

    public Optional<DiscoveryRequest> findById(UUID tenantId, UUID requestId) {
        return jdbc.query("SELECT * FROM platform.discovery_request WHERE tenant_id=:tenantId AND request_id=:requestId",
                Map.of("tenantId", tenantId, "requestId", requestId), REQUEST_MAPPER).stream().findFirst();
    }

    public Optional<DiscoveryRequest> findByIdForUpdate(UUID tenantId, UUID requestId) {
        return jdbc.query("SELECT * FROM platform.discovery_request WHERE tenant_id=:tenantId AND request_id=:requestId FOR UPDATE",
                Map.of("tenantId", tenantId, "requestId", requestId), REQUEST_MAPPER).stream().findFirst();
    }

    /**
     * Read {@code assignment_version} for a row already locked in the current transaction.
     * Returns empty when the request does not exist.
     */
    public Optional<Long> findAssignmentVersion(UUID tenantId, UUID requestId) {
        List<Long> rows = jdbc.query("""
                SELECT assignment_version FROM platform.discovery_request
                 WHERE tenant_id=:tenantId AND request_id=:requestId
                """,
                Map.of("tenantId", tenantId, "requestId", requestId),
                (rs, rowNum) -> rs.getLong("assignment_version"));
        return rows.stream().findFirst();
    }

    public Optional<AssignedRequest> findAssignedForUpdate(String sensorId, UUID requestId) {
        return jdbc.query("""
                SELECT * FROM platform.discovery_request
                 WHERE request_id=:requestId AND assigned_sensor_id=:sensorId
                 FOR UPDATE
                """, Map.of("requestId", requestId, "sensorId", sensorId), ASSIGNED_REQUEST_MAPPER).stream().findFirst();
    }

    public Optional<DiscoveryRequest> findByIdempotencyKey(UUID tenantId, String idempotencyKey) {
        if (idempotencyKey == null) return Optional.empty();
        return jdbc.query("""
                SELECT * FROM platform.discovery_request
                 WHERE tenant_id=:tenantId AND idempotency_key=:idempotencyKey
                """, Map.of("tenantId", tenantId, "idempotencyKey", idempotencyKey), REQUEST_MAPPER).stream().findFirst();
    }

    public PageResult<DiscoveryRequest> listRequests(UUID tenantId, int page, int limit) {
        int offset = (page - 1) * limit;
        int total = jdbc.queryForObject("SELECT COUNT(*) FROM platform.discovery_request WHERE tenant_id=:tenantId",
                Map.of("tenantId", tenantId), Integer.class);
        List<DiscoveryRequest> items = jdbc.query("""
                SELECT * FROM platform.discovery_request WHERE tenant_id=:tenantId
                 ORDER BY created_at DESC LIMIT :limit OFFSET :offset
                """, Map.of("tenantId", tenantId, "limit", limit, "offset", offset), REQUEST_MAPPER);
        return new PageResult<>(items, page, limit, total, total > offset + items.size());
    }

    public List<DiscoveryTarget> listTargets(UUID tenantId, UUID requestId) {
        return jdbc.query("""
                SELECT * FROM platform.discovery_target
                 WHERE tenant_id=:tenantId AND request_id=:requestId ORDER BY target_address ASC
                """, Map.of("tenantId", tenantId, "requestId", requestId), TARGET_MAPPER);
    }

    /**
     * First placement only: PENDING with no assigned sensor. Concurrent losers get {@code false}
     * (no last-write-wins overwrite of an existing assignment).
     */
    public boolean assignSensorFirstPlacement(UUID tenantId, UUID requestId, String sensorId) {
        int changed = jdbc.update("""
                UPDATE platform.discovery_request
                   SET assigned_sensor_id=:sensorId,
                       assignment_version=assignment_version+1,
                       status='ASSIGNED', started_at=NULL, completed_at=NULL, failure_reason=NULL
                 WHERE tenant_id=:tenantId AND request_id=:requestId
                   AND status='PENDING'
                   AND assigned_sensor_id IS NULL
                """, Map.of("tenantId", tenantId, "requestId", requestId, "sensorId", sensorId));
        return changed == 1;
    }

    /**
     * Intentional reassignment (PENDING→ASSIGNED or ASSIGNED→ASSIGNED). Not used for first placement races.
     */
    public boolean assignSensor(UUID tenantId, UUID requestId, String sensorId) {
        int changed = jdbc.update("""
                UPDATE platform.discovery_request
                   SET assigned_sensor_id=:sensorId,
                       assignment_version=assignment_version+1,
                       status='ASSIGNED', started_at=NULL, completed_at=NULL, failure_reason=NULL
                 WHERE tenant_id=:tenantId AND request_id=:requestId
                   AND status IN ('PENDING','ASSIGNED')
                """, Map.of("tenantId", tenantId, "requestId", requestId, "sensorId", sensorId));
        return changed == 1;
    }

    public Optional<AssignedRequest> claimAssignedRequestForSensor(String sensorId, Instant startedAt) {
        MapSqlParameterSource params = new MapSqlParameterSource()
                .addValue("sensorId", sensorId).addValue("startedAt", Timestamp.from(startedAt));
        return jdbc.query("""
                WITH candidate AS (
                    SELECT request_id FROM platform.discovery_request
                     WHERE assigned_sensor_id=:sensorId AND status='ASSIGNED'
                     ORDER BY created_at ASC
                     FOR UPDATE SKIP LOCKED
                     LIMIT 1
                )
                UPDATE platform.discovery_request r
                   SET status='RUNNING', started_at=COALESCE(r.started_at,:startedAt)
                  FROM candidate c
                 WHERE r.request_id=c.request_id
                RETURNING r.*
                """, params, ASSIGNED_REQUEST_MAPPER).stream().findFirst();
    }

    public void updateStatus(UUID tenantId, UUID requestId, DiscoveryStatus status,
                             Instant startedAt, Instant completedAt, String failureReason, Map<String, Object> summary) {
        MapSqlParameterSource params = new MapSqlParameterSource()
                .addValue("tenantId", tenantId).addValue("requestId", requestId).addValue("status", status.name())
                .addValue("startedAt", startedAt == null ? null : Timestamp.from(startedAt))
                .addValue("completedAt", completedAt == null ? null : Timestamp.from(completedAt))
                .addValue("failureReason", failureReason)
                .addValue("summaryJson", summary == null ? null : toJson(summary));
        jdbc.update("""
                UPDATE platform.discovery_request
                   SET status=:status, started_at=COALESCE(:startedAt,started_at),
                       completed_at=COALESCE(:completedAt,completed_at), failure_reason=:failureReason,
                       summary_json=COALESCE(CAST(:summaryJson AS jsonb),summary_json)
                 WHERE tenant_id=:tenantId AND request_id=:requestId
                """, params);
    }

    @Transactional("transactionManager")
    public void recordEvidence(UUID tenantId, UUID requestId, List<DiscoveryEvidence> evidenceList) {
        if (evidenceList == null || evidenceList.isEmpty()) return;
        SqlParameterSource[] batchSources = new SqlParameterSource[evidenceList.size()];
        for (int i = 0; i < evidenceList.size(); i++) {
            DiscoveryEvidence e = evidenceList.get(i);
            batchSources[i] = new MapSqlParameterSource()
                    .addValue("evidenceId", e.evidenceId()).addValue("tenantId", tenantId).addValue("requestId", requestId)
                    .addValue("targetId", e.targetId()).addValue("sensorId", e.sensorId())
                    .addValue("protocol", e.protocol().name()).addValue("method", e.method())
                    .addValue("status", e.status().name()).addValue("reachabilityKind", e.reachabilityKind().name())
                    .addValue("technicalCode", e.technicalCode()).addValue("attributesJson", toJson(e.attributes()))
                    .addValue("rawEvidenceJson", toJson(e.rawEvidence())).addValue("durationMs", e.durationMs())
                    .addValue("observedAt", Timestamp.from(e.observedAt()));
        }
        jdbc.batchUpdate("""
                INSERT INTO platform.discovery_evidence (
                    evidence_id,tenant_id,request_id,target_id,sensor_id,protocol,method,status,
                    reachability_kind,technical_code,attributes_json,raw_evidence_json,duration_ms,observed_at
                ) VALUES (
                    :evidenceId,:tenantId,:requestId,:targetId,:sensorId,:protocol,:method,:status,
                    :reachabilityKind,:technicalCode,CAST(:attributesJson AS jsonb),CAST(:rawEvidenceJson AS jsonb),:durationMs,:observedAt
                )
                ON CONFLICT (tenant_id,request_id,target_id,sensor_id,protocol,method) DO NOTHING
                """, batchSources);
    }

    public void upsertCandidate(DiscoveryCandidate candidate) {
        String protocolsJson = toJson(candidate.evidencedProtocols().stream().map(Enum::name).toList());
        String attrsJson = toJson(candidate.attributes() == null ? Map.of() : candidate.attributes());
        MapSqlParameterSource params = new MapSqlParameterSource()
                .addValue("candidateId", candidate.candidateId()).addValue("tenantId", candidate.tenantId())
                .addValue("requestId", candidate.requestId()).addValue("targetAddress", candidate.targetAddress())
                .addValue("discoveredName", candidate.discoveredName()).addValue("sysObjectId", candidate.sysObjectId())
                .addValue("sysDescr", candidate.sysDescr()).addValue("evidencedProtocols", protocolsJson)
                .addValue("attributesJson", attrsJson).addValue("confidence", candidate.confidence().name())
                .addValue("reviewStatus", candidate.reviewStatus().name()).addValue("conflictDetected", candidate.conflictDetected())
                .addValue("conflictDetails", candidate.conflictDetails()).addValue("siteId", candidate.siteId())
                .addValue("firstSeenAt", Timestamp.from(candidate.firstSeenAt())).addValue("lastSeenAt", Timestamp.from(candidate.lastSeenAt()))
                .addValue("createdAt", Timestamp.from(candidate.createdAt())).addValue("updatedAt", Timestamp.from(candidate.updatedAt()));
        jdbc.update("""
                INSERT INTO platform.discovery_candidate (
                    candidate_id,tenant_id,request_id,target_address,discovered_name,sys_object_id,sys_descr,
                    evidenced_protocols,attributes_json,confidence,review_status,conflict_detected,conflict_details,
                    site_id,first_seen_at,last_seen_at,created_at,updated_at
                ) VALUES (
                    :candidateId,:tenantId,:requestId,:targetAddress,:discoveredName,:sysObjectId,:sysDescr,
                    CAST(:evidencedProtocols AS jsonb),CAST(:attributesJson AS jsonb),:confidence,:reviewStatus,
                    :conflictDetected,:conflictDetails,:siteId,:firstSeenAt,:lastSeenAt,:createdAt,:updatedAt
                )
                ON CONFLICT (tenant_id,request_id,target_address) DO UPDATE
                   SET discovered_name=COALESCE(EXCLUDED.discovered_name,platform.discovery_candidate.discovered_name),
                       sys_object_id=COALESCE(EXCLUDED.sys_object_id,platform.discovery_candidate.sys_object_id),
                       sys_descr=COALESCE(EXCLUDED.sys_descr,platform.discovery_candidate.sys_descr),
                       evidenced_protocols=EXCLUDED.evidenced_protocols, attributes_json=EXCLUDED.attributes_json,
                       confidence=EXCLUDED.confidence, conflict_detected=EXCLUDED.conflict_detected,
                       conflict_details=EXCLUDED.conflict_details, last_seen_at=EXCLUDED.last_seen_at,
                       updated_at=EXCLUDED.updated_at
                """, params);
    }

    public PageResult<DiscoveryCandidate> listCandidates(UUID tenantId, UUID requestId, int page, int limit) {
        int offset = (page - 1) * limit;
        int total = jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.discovery_candidate WHERE tenant_id=:tenantId AND request_id=:requestId
                """, Map.of("tenantId", tenantId, "requestId", requestId), Integer.class);
        List<DiscoveryCandidate> items = jdbc.query("""
                SELECT * FROM platform.discovery_candidate WHERE tenant_id=:tenantId AND request_id=:requestId
                 ORDER BY target_address ASC LIMIT :limit OFFSET :offset
                """, Map.of("tenantId", tenantId, "requestId", requestId, "limit", limit, "offset", offset), CANDIDATE_MAPPER);
        return new PageResult<>(items, page, limit, total, total > offset + items.size());
    }

    public PageResult<DiscoveryEvidence> listEvidence(UUID tenantId, UUID requestId, int page, int limit) {
        int offset = (page - 1) * limit;
        int total = jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.discovery_evidence WHERE tenant_id=:tenantId AND request_id=:requestId
                """, Map.of("tenantId", tenantId, "requestId", requestId), Integer.class);
        List<DiscoveryEvidence> items = jdbc.query("""
                SELECT * FROM platform.discovery_evidence WHERE tenant_id=:tenantId AND request_id=:requestId
                 ORDER BY observed_at ASC,evidence_id ASC LIMIT :limit OFFSET :offset
                """, Map.of("tenantId", tenantId, "requestId", requestId, "limit", limit, "offset", offset), EVIDENCE_MAPPER);
        return new PageResult<>(items, page, limit, total, total > offset + items.size());
    }

    private String toJson(Object obj) {
        try {
            return mapper.writeValueAsString(obj == null ? Map.of() : obj);
        } catch (JsonProcessingException e) {
            throw new IllegalStateException("DISCOVERY_JSON_SERIALIZATION_FAILED", e);
        }
    }

    private Map<String, Object> parseJsonMap(String json) {
        if (json == null || json.trim().isEmpty()) return Collections.emptyMap();
        try {
            return mapper.readValue(json, new TypeReference<>() {});
        } catch (Exception e) {
            throw new IllegalStateException("DISCOVERY_JSON_CORRUPT", e);
        }
    }

    private <T extends Enum<T>> List<T> parseJsonList(String json, Class<T> enumType) {
        if (json == null || json.trim().isEmpty()) return Collections.emptyList();
        try {
            List<String> raw = mapper.readValue(json, new TypeReference<>() {});
            List<T> list = new ArrayList<>(raw.size());
            for (String value : raw) list.add(Enum.valueOf(enumType, value));
            return List.copyOf(list);
        } catch (Exception e) {
            throw new IllegalStateException("DISCOVERY_JSON_CORRUPT", e);
        }
    }
}