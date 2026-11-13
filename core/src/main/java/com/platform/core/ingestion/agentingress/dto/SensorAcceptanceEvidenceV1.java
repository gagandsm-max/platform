package com.platform.core.ingestion.agentingress.dto;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.JsonNode;

/** Immutable Sensor-stamped clock and durable spool provenance for one ingress record. */
@JsonIgnoreProperties(ignoreUnknown = false)
public record SensorAcceptanceEvidenceV1(
        @JsonProperty("evidenceVersion") JsonNode evidenceVersion,
        @JsonProperty("acceptedAtUtc") String acceptedAtUtc,
        @JsonProperty("clockQuality") String clockQuality,
        @JsonProperty("clockMaxErrorMs") JsonNode clockMaxErrorMs,
        @JsonProperty("clockSyncAgeMs") JsonNode clockSyncAgeMs,
        @JsonProperty("clockSource") String clockSource,
        @JsonProperty("sensorServiceInstanceId") String sensorServiceInstanceId,
        @JsonProperty("durableSpoolSequence") JsonNode durableSpoolSequence
) {}
