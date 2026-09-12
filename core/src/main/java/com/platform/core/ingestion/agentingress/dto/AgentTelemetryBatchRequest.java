package com.platform.core.ingestion.agentingress.dto;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = false)
public record AgentTelemetryBatchRequest(
        @JsonProperty("schemaVersion") int schemaVersion,
        @JsonProperty("sensorId") String sensorId,
        @JsonProperty("batchId") String batchId,
        @JsonProperty("firstIngressSequence") long firstIngressSequence,
        @JsonProperty("lastIngressSequence") long lastIngressSequence,
        @JsonProperty("recordCount") int recordCount,
        @JsonProperty("checksum") String checksum,
        @JsonProperty("correlationId") String correlationId,
        @JsonProperty("records") List<AgentIngressRecord> records
) {}
