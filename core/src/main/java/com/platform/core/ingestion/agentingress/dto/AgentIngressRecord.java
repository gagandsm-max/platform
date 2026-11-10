package com.platform.core.ingestion.agentingress.dto;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.JsonNode;

import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = false)
public record AgentIngressRecord(
        @JsonProperty("sensorIngressSequence") long sensorIngressSequence,
        @JsonProperty("sensorIngressRecordId") String sensorIngressRecordId,
        @JsonProperty("tenantId") String tenantId,
        @JsonProperty("agentId") String agentId,
        @JsonProperty("acceptingSensorId") String acceptingSensorId,
        @JsonProperty("assignmentGeneration") long assignmentGeneration,
        @JsonProperty("agentFirstSequence") long agentFirstSequence,
        @JsonProperty("agentLastSequence") long agentLastSequence,
        @JsonProperty("agentBatchId") String agentBatchId,
        @JsonProperty("correlationId") String correlationId,
        @JsonProperty("acceptedAt") String acceptedAt,
        @JsonProperty("acceptanceEvidence") SensorAcceptanceEvidenceV1 acceptanceEvidence,
        @JsonProperty("records") List<JsonNode> records
) {
    /** Transitional constructor for pre-evidence persisted fixtures and legacy exact requests. */
    public AgentIngressRecord(long sensorIngressSequence, String sensorIngressRecordId,
            String tenantId, String agentId, String acceptingSensorId, long assignmentGeneration,
            long agentFirstSequence, long agentLastSequence, String agentBatchId,
            String correlationId, String acceptedAt, List<JsonNode> records) {
        this(sensorIngressSequence, sensorIngressRecordId, tenantId, agentId,
                acceptingSensorId, assignmentGeneration, agentFirstSequence, agentLastSequence,
                agentBatchId, correlationId, acceptedAt, null, records);
    }
}
