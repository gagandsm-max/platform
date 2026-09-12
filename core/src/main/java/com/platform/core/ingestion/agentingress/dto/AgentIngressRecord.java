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
        @JsonProperty("records") List<JsonNode> records
) {}
