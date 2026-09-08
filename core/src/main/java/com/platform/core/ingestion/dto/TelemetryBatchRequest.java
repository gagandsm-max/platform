package com.platform.core.ingestion.dto;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public record TelemetryBatchRequest(
        @JsonProperty("schemaVersion") int schemaVersion,
        @JsonProperty("sensorId") String sensorId,
        @JsonProperty("bootEpoch") String bootEpoch,
        @JsonProperty("batchId") String batchId,
        @JsonProperty("firstSequence") long firstSequence,
        @JsonProperty("lastSequence") long lastSequence,
        @JsonProperty("checksum") String checksum,
        @JsonProperty("records") List<JsonNode> records
) {}
