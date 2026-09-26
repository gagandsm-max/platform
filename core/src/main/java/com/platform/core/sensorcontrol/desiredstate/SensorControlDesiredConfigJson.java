package com.platform.core.sensorcontrol.desiredstate;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.platform.core.domain.DomainException;

import java.util.EnumMap;
import java.util.Iterator;
import java.util.Map;

/**
 * Strict JSON codec for desired-config v1 persistence and machine wire.
 * Rejects unknown properties. Does not accept identity/secret fields.
 */
public final class SensorControlDesiredConfigJson {

    private static final ObjectMapper MAPPER = new ObjectMapper()
            .configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, true)
            .configure(SerializationFeature.ORDER_MAP_ENTRIES_BY_KEYS, true);

    private SensorControlDesiredConfigJson() {}

    public static String toJson(SensorRuntimeDesiredConfig config) {
        SensorControlDesiredConfigValidator.validate(config);
        try {
            ObjectNode root = MAPPER.createObjectNode();
            root.put("logLevel", config.logLevel().name());
            ObjectNode scheduler = root.putObject("scheduler");
            SensorSchedulerDesiredConfig s = config.scheduler();
            scheduler.put("maxWorkers", s.maxWorkers());
            scheduler.put("maxQueueDepth", s.maxQueueDepth());
            scheduler.put("maxTrackedAssignments", s.maxTrackedAssignments());
            scheduler.put("maxTrackedJobs", s.maxTrackedJobs());
            scheduler.put("maxPayloadBytes", s.maxPayloadBytes());
            scheduler.put("defaultExecutionTimeoutMs", s.defaultExecutionTimeoutMs());
            scheduler.put("shutdownGracePeriodMs", s.shutdownGracePeriodMs());
            scheduler.put("retryMaxAttempts", s.retryMaxAttempts());
            scheduler.put("retryInitialBackoffMs", s.retryInitialBackoffMs());
            scheduler.put("retryMaxBackoffMs", s.retryMaxBackoffMs());
            scheduler.put("retryJitterBasisPoints", s.retryJitterBasisPoints());
            ObjectNode pools = scheduler.putObject("pools");
            for (Map.Entry<String, SensorSchedulerPoolConfig> e : s.poolsSortedByName().entrySet()) {
                ObjectNode pool = pools.putObject(e.getKey());
                pool.put("workers", e.getValue().workers());
                pool.put("queueDepth", e.getValue().queueDepth());
            }
            ObjectNode transport = root.putObject("transport");
            SensorTransportDesiredConfig t = config.transport();
            transport.put("requestTimeoutMs", t.requestTimeoutMs());
            transport.put("maxRequestBytes", t.maxRequestBytes());
            transport.put("maxResponseBytes", t.maxResponseBytes());
            transport.put("uploadBatchRecords", t.uploadBatchRecords());
            transport.put("uploadBatchBytes", t.uploadBatchBytes());
            String json = MAPPER.writeValueAsString(root);
            if (json.getBytes().length > SensorControlDesiredLimits.MAX_CONFIG_JSON_BYTES) {
                throw new DomainException("VALIDATION_FAILED", "configuration JSON exceeds size bound");
            }
            return json;
        } catch (JsonProcessingException ex) {
            throw new DomainException("VALIDATION_FAILED", "configuration JSON serialization failed");
        }
    }

    public static SensorRuntimeDesiredConfig fromJson(String json) {
        if (json == null || json.isBlank()) {
            throw new DomainException("VALIDATION_FAILED", "configuration is required");
        }
        if (json.getBytes().length > SensorControlDesiredLimits.MAX_CONFIG_JSON_BYTES) {
            throw new DomainException("VALIDATION_FAILED", "configuration JSON exceeds size bound");
        }
        try {
            JsonNode root = MAPPER.readTree(json);
            if (root == null || !root.isObject()) {
                throw new DomainException("VALIDATION_FAILED", "configuration must be a JSON object");
            }
            rejectUnknown(root, "logLevel", "scheduler", "transport");
            SensorLogLevel logLevel = SensorLogLevel.parse(requireText(root, "logLevel"));
            JsonNode schedulerNode = requireObject(root, "scheduler");
            rejectUnknown(schedulerNode,
                    "maxWorkers", "maxQueueDepth", "maxTrackedAssignments", "maxTrackedJobs",
                    "maxPayloadBytes", "defaultExecutionTimeoutMs", "shutdownGracePeriodMs",
                    "retryMaxAttempts", "retryInitialBackoffMs", "retryMaxBackoffMs",
                    "retryJitterBasisPoints", "pools");
            JsonNode poolsNode = requireObject(schedulerNode, "pools");
            EnumMap<SensorSchedulerPoolName, SensorSchedulerPoolConfig> pools =
                    new EnumMap<>(SensorSchedulerPoolName.class);
            Iterator<String> names = poolsNode.fieldNames();
            while (names.hasNext()) {
                String name = names.next();
                SensorSchedulerPoolName poolName = SensorSchedulerPoolName.parse(name);
                JsonNode poolNode = poolsNode.get(name);
                if (poolNode == null || !poolNode.isObject()) {
                    throw new DomainException("VALIDATION_FAILED", "scheduler.pools." + name + " must be object");
                }
                rejectUnknown(poolNode, "workers", "queueDepth");
                pools.put(poolName, new SensorSchedulerPoolConfig(
                        requireInt(poolNode, "workers"),
                        requireInt(poolNode, "queueDepth")));
            }
            SensorSchedulerDesiredConfig scheduler = new SensorSchedulerDesiredConfig(
                    requireInt(schedulerNode, "maxWorkers"),
                    requireInt(schedulerNode, "maxQueueDepth"),
                    requireInt(schedulerNode, "maxTrackedAssignments"),
                    requireInt(schedulerNode, "maxTrackedJobs"),
                    requireInt(schedulerNode, "maxPayloadBytes"),
                    requireInt(schedulerNode, "defaultExecutionTimeoutMs"),
                    requireInt(schedulerNode, "shutdownGracePeriodMs"),
                    requireInt(schedulerNode, "retryMaxAttempts"),
                    requireInt(schedulerNode, "retryInitialBackoffMs"),
                    requireInt(schedulerNode, "retryMaxBackoffMs"),
                    requireInt(schedulerNode, "retryJitterBasisPoints"),
                    pools
            );
            JsonNode transportNode = requireObject(root, "transport");
            rejectUnknown(transportNode,
                    "requestTimeoutMs", "maxRequestBytes", "maxResponseBytes",
                    "uploadBatchRecords", "uploadBatchBytes");
            SensorTransportDesiredConfig transport = new SensorTransportDesiredConfig(
                    requireInt(transportNode, "requestTimeoutMs"),
                    requireInt(transportNode, "maxRequestBytes"),
                    requireInt(transportNode, "maxResponseBytes"),
                    requireInt(transportNode, "uploadBatchRecords"),
                    requireInt(transportNode, "uploadBatchBytes")
            );
            SensorRuntimeDesiredConfig config = new SensorRuntimeDesiredConfig(logLevel, scheduler, transport);
            SensorControlDesiredConfigValidator.validate(config);
            return config;
        } catch (DomainException ex) {
            throw ex;
        } catch (IllegalArgumentException ex) {
            throw new DomainException("VALIDATION_FAILED", ex.getMessage());
        } catch (JsonProcessingException ex) {
            throw new DomainException("VALIDATION_FAILED", "configuration JSON is malformed");
        }
    }

    private static void rejectUnknown(JsonNode node, String... allowed) {
        java.util.Set<String> ok = java.util.Set.of(allowed);
        Iterator<String> it = node.fieldNames();
        while (it.hasNext()) {
            String name = it.next();
            if (!ok.contains(name)) {
                throw new DomainException("VALIDATION_FAILED", "Unknown configuration field: " + name);
            }
        }
    }

    private static JsonNode requireObject(JsonNode parent, String field) {
        JsonNode node = parent.get(field);
        if (node == null || !node.isObject()) {
            throw new DomainException("VALIDATION_FAILED", field + " must be an object");
        }
        return node;
    }

    private static String requireText(JsonNode parent, String field) {
        JsonNode node = parent.get(field);
        if (node == null || !node.isTextual()) {
            throw new DomainException("VALIDATION_FAILED", field + " must be a string");
        }
        return node.asText();
    }

    private static int requireInt(JsonNode parent, String field) {
        JsonNode node = parent.get(field);
        if (node == null || !node.isIntegralNumber() || !node.canConvertToInt()) {
            throw new DomainException("VALIDATION_FAILED", field + " must be an integer");
        }
        return node.intValue();
    }
}
