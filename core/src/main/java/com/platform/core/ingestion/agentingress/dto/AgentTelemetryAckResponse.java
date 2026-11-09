package com.platform.core.ingestion.agentingress.dto;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;

@JsonInclude(JsonInclude.Include.NON_NULL)
public record AgentTelemetryAckResponse(
        @JsonProperty("status") String status,
        @JsonProperty("sensorId") String sensorId,
        @JsonProperty("batchId") String batchId,
        @JsonProperty("acceptedThroughIngressSequence") Long acceptedThroughIngressSequence,
        @JsonProperty("acceptedLastIngressSequence") Long acceptedLastIngressSequence,
        @JsonProperty("correlationId") String correlationId,
        @JsonProperty("firstIngressSequence") Long firstIngressSequence,
        @JsonProperty("lastIngressSequence") Long lastIngressSequence,
        @JsonProperty("checksum") String checksum,
        @JsonProperty("settledThroughIngressSequence") Long settledThroughIngressSequence,
        @JsonProperty("dispositionId") java.util.UUID dispositionId,
        @JsonProperty("reasonCode") String reasonCode
) {
    public AgentTelemetryAckResponse(String status, String sensorId, String batchId,
            Long acceptedThroughIngressSequence, String correlationId) {
        this(status, sensorId, batchId, acceptedThroughIngressSequence,
                acceptedThroughIngressSequence, correlationId,
                null, null, null, null, null, null);
    }
}
