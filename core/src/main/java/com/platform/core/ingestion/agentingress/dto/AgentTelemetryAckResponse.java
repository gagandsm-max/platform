package com.platform.core.ingestion.agentingress.dto;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;

@JsonInclude(JsonInclude.Include.NON_NULL)
public record AgentTelemetryAckResponse(
        @JsonProperty("status") String status,
        @JsonProperty("sensorId") String sensorId,
        @JsonProperty("batchId") String batchId,
        @JsonProperty("acceptedThroughIngressSequence") Long acceptedThroughIngressSequence,
        @JsonProperty("correlationId") String correlationId
) {}
