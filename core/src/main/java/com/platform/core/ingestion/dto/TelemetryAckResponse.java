package com.platform.core.ingestion.dto;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;

@JsonInclude(JsonInclude.Include.NON_NULL)
public record TelemetryAckResponse(
        @JsonProperty("status") String status,
        @JsonProperty("accepted_through_sequence") Long acceptedThroughSequence,
        @JsonProperty("sensor_identity") String sensorIdentity
) {}
