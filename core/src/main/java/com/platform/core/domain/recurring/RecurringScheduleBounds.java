package com.platform.core.domain.recurring;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.platform.core.domain.DomainException;

import java.util.ArrayList;
import java.util.Iterator;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;
import java.util.Set;

/**
 * Bounds and secret-rejection helpers for recurring schedule assignments (Slice A).
 * Mirrors Sensor {@code recurring} package constants where applicable.
 */
public final class RecurringScheduleBounds {

    public static final int MAX_ASSIGNMENTS = 512;
    public static final int MAX_STRING_LEN = 256;
    public static final int MAX_ID_LEN = 128;
    public static final int MAX_CLASS_LEN = 64;
    public static final int MAX_CREDENTIAL_REF_LEN = 256;
    public static final int MAX_PROTOCOL_LEN = 32;
    public static final int MAX_OIDS = 128;
    public static final int MAX_PROTOCOL_PARAMS_BYTES = 65_536;

    public static final long MIN_CADENCE_MS = 1_000L;
    public static final long MAX_CADENCE_MS = 24L * 60L * 60L * 1_000L;
    public static final long MIN_FRESHNESS_MS = 1L;
    public static final long MAX_FRESHNESS_MS = 24L * 60L * 60L * 1_000L;
    public static final long MIN_EXECUTION_TIMEOUT_MS = 1L;
    public static final long MAX_EXECUTION_TIMEOUT_MS = 60L * 60L * 1_000L;
    public static final long MIN_SNMP_TIMEOUT_MS = 1L;
    public static final long MAX_SNMP_TIMEOUT_MS = 60L * 1_000L;
    public static final int MIN_PORT = 1;
    public static final int MAX_PORT = 65_535;
    public static final int MIN_PHASE_VERSION = 1;

    public static final String PROTOCOL_SNMP = "SNMP";

    public static final Set<String> SCHEDULING_CLASSES = Set.of(
            "LIVENESS",
            "OPERATIONAL_CRITICAL",
            "OPERATIONAL_DETAIL",
            "STATE_TOPOLOGY",
            "INVENTORY",
            "LIFECYCLE",
            "EVENT_STREAM",
            "INVESTIGATION"
    );

    public static final Set<String> EXECUTION_CLASSES = Set.of(
            "NETWORK_POLL",
            "STREAMING",
            "INFRA",
            "DISCOVERY",
            "NCM",
            "SYNTHETIC"
    );

    /** Forbidden protocol-params JSON keys (normalized: lower-case, no _/-). */
    private static final Set<String> FORBIDDEN_SECRET_KEYS = Set.of(
            "community",
            "authsecret",
            "privacysecret",
            "privatekey",
            "password",
            "bearer",
            "communitystring",
            "bearertoken"
    );

    private static final ObjectMapper MAPPER = new ObjectMapper();

    private RecurringScheduleBounds() {
    }

    public static String requireId(String field, String value) {
        String v = requireNonBlank(field, value);
        if (v.length() > MAX_ID_LEN) {
            throw new DomainException("VALIDATION_FAILED", field + " exceeds max length " + MAX_ID_LEN);
        }
        return v;
    }

    public static String requireClass(String field, String value, Set<String> allowed) {
        String v = requireNonBlank(field, value);
        if (v.length() > MAX_CLASS_LEN) {
            throw new DomainException("VALIDATION_FAILED", field + " exceeds max length " + MAX_CLASS_LEN);
        }
        if (!allowed.contains(v)) {
            throw new DomainException("VALIDATION_FAILED", "invalid " + field);
        }
        return v;
    }

    public static String requireCredentialRef(String value) {
        String v = requireNonBlank("credentialProfileRef", value);
        if (v.length() > MAX_CREDENTIAL_REF_LEN) {
            throw new DomainException(
                    "VALIDATION_FAILED",
                    "credentialProfileRef exceeds max length " + MAX_CREDENTIAL_REF_LEN);
        }
        return v;
    }

    public static String requireNonBlank(String field, String value) {
        if (value == null || value.isBlank()) {
            throw new DomainException("VALIDATION_FAILED", field + " is required");
        }
        if (value.length() > MAX_STRING_LEN) {
            throw new DomainException("VALIDATION_FAILED", field + " exceeds max length " + MAX_STRING_LEN);
        }
        return value.trim();
    }

    public static void requireCadence(long cadenceMs) {
        if (cadenceMs < MIN_CADENCE_MS || cadenceMs > MAX_CADENCE_MS) {
            throw new DomainException("VALIDATION_FAILED", "cadenceMs out of bounds");
        }
    }

    public static void requireFreshness(long freshnessMs) {
        if (freshnessMs < MIN_FRESHNESS_MS || freshnessMs > MAX_FRESHNESS_MS) {
            throw new DomainException("VALIDATION_FAILED", "freshnessMs out of bounds");
        }
    }

    public static void requireExecutionTimeout(long executionTimeoutMs) {
        if (executionTimeoutMs < MIN_EXECUTION_TIMEOUT_MS
                || executionTimeoutMs > MAX_EXECUTION_TIMEOUT_MS) {
            throw new DomainException("VALIDATION_FAILED", "executionTimeoutMs out of bounds");
        }
    }

    public static void requirePhaseVersion(int phaseVersion) {
        if (phaseVersion < MIN_PHASE_VERSION) {
            throw new DomainException("VALIDATION_FAILED", "phaseVersion must be >= 1");
        }
    }

    public static void requireProtocol(String protocol) {
        String v = requireNonBlank("protocol", protocol);
        if (v.length() > MAX_PROTOCOL_LEN) {
            throw new DomainException("VALIDATION_FAILED", "protocol exceeds max length");
        }
        if (!PROTOCOL_SNMP.equals(v)) {
            throw new DomainException("VALIDATION_FAILED", "unsupported protocol");
        }
    }

    public static SnmpParams parseAndValidateSnmpParams(String protocolParamsJson) {
        Objects.requireNonNull(protocolParamsJson, "protocolParamsJson");
        byte[] raw = protocolParamsJson.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        if (raw.length > MAX_PROTOCOL_PARAMS_BYTES) {
            throw new DomainException("VALIDATION_FAILED", "protocol_params_json exceeds max size");
        }
        final JsonNode root;
        try {
            root = MAPPER.readTree(raw);
        } catch (java.io.IOException ex) {
            throw new DomainException("VALIDATION_FAILED", "protocol_params_json is not valid JSON");
        }
        if (root == null || !root.isObject()) {
            throw new DomainException("VALIDATION_FAILED", "protocol_params_json must be a JSON object");
        }
        rejectForbiddenKeys(root, "$");
        return parseSnmpObject(root);
    }

    public static String canonicalizeSnmpJson(SnmpParams snmp) {
        Objects.requireNonNull(snmp, "snmp");
        ObjectNode root = MAPPER.createObjectNode();
        root.put("target", snmp.target());
        root.put("port", snmp.port());
        root.put("version", snmp.version());
        root.put("operation", snmp.operation());
        ArrayNode oids = root.putArray("oids");
        for (String oid : snmp.oids()) {
            oids.add(oid);
        }
        root.put("timeoutMs", snmp.timeoutMs());
        ObjectNode limits = root.putObject("limits");
        limits.put("maxOids", snmp.limits().maxOids());
        limits.put("maxWalkEntries", snmp.limits().maxWalkEntries());
        limits.put("maxBulkRepetitions", snmp.limits().maxBulkRepetitions());
        limits.put("maxResults", snmp.limits().maxResults());
        limits.put("maxResultBytes", snmp.limits().maxResultBytes());
        try {
            return MAPPER.writeValueAsString(root);
        } catch (JsonProcessingException ex) {
            throw new DomainException("VALIDATION_FAILED", "failed to serialize snmp params");
        }
    }

    public static SnmpParams requireSnmp(SnmpParams snmp) {
        if (snmp == null) {
            throw new DomainException("VALIDATION_FAILED", "snmp params required for SNMP protocol");
        }
        String target = requireNonBlank("snmp.target", snmp.target());
        if (snmp.port() < MIN_PORT || snmp.port() > MAX_PORT) {
            throw new DomainException("VALIDATION_FAILED", "snmp.port out of bounds");
        }
        String version = requireNonBlank("snmp.version", snmp.version());
        if (!Set.of("1", "2c", "3").contains(version)) {
            throw new DomainException("VALIDATION_FAILED", "unsupported snmp.version");
        }
        String operation = requireNonBlank("snmp.operation", snmp.operation());
        if (snmp.oids() == null || snmp.oids().isEmpty()) {
            throw new DomainException("VALIDATION_FAILED", "snmp.oids is required");
        }
        if (snmp.oids().size() > MAX_OIDS) {
            throw new DomainException("VALIDATION_FAILED", "snmp.oids exceeds max " + MAX_OIDS);
        }
        for (String oid : snmp.oids()) {
            requireNonBlank("snmp.oids[]", oid);
        }
        if (snmp.timeoutMs() < MIN_SNMP_TIMEOUT_MS || snmp.timeoutMs() > MAX_SNMP_TIMEOUT_MS) {
            throw new DomainException("VALIDATION_FAILED", "snmp.timeoutMs out of bounds");
        }
        SnmpParams.SnmpLimits limits = snmp.limits();
        if (limits == null) {
            throw new DomainException("VALIDATION_FAILED", "snmp.limits is required");
        }
        if (limits.maxOids() <= 0
                || limits.maxWalkEntries() <= 0
                || limits.maxBulkRepetitions() <= 0
                || limits.maxResults() <= 0
                || limits.maxResultBytes() <= 0) {
            throw new DomainException("VALIDATION_FAILED", "snmp.limits values must be positive");
        }
        return new SnmpParams(
                target,
                snmp.port(),
                version,
                operation,
                List.copyOf(snmp.oids()),
                snmp.timeoutMs(),
                limits
        );
    }

    static void rejectForbiddenKeys(JsonNode node, String path) {
        if (node == null) {
            return;
        }
        if (node.isObject()) {
            Iterator<Map.Entry<String, JsonNode>> fields = node.fields();
            while (fields.hasNext()) {
                Map.Entry<String, JsonNode> entry = fields.next();
                String key = entry.getKey();
                if (isForbiddenSecretKey(key)) {
                    throw new DomainException(
                            "VALIDATION_FAILED",
                            "protocol_params_json contains forbidden secret key: " + key,
                            Map.of("path", path + "." + key)
                    );
                }
                rejectForbiddenKeys(entry.getValue(), path + "." + key);
            }
        } else if (node.isArray()) {
            int i = 0;
            for (JsonNode child : node) {
                rejectForbiddenKeys(child, path + "[" + i + "]");
                i++;
            }
        }
    }

    static boolean isForbiddenSecretKey(String key) {
        if (key == null || key.isBlank()) {
            return false;
        }
        String normalized = key.toLowerCase(Locale.ROOT).replace("_", "").replace("-", "");
        return FORBIDDEN_SECRET_KEYS.contains(normalized);
    }

    private static SnmpParams parseSnmpObject(JsonNode root) {
        String target = textRequired(root, "target");
        if (!root.has("port") || !root.get("port").canConvertToInt()) {
            throw new DomainException("VALIDATION_FAILED", "snmp.port is required");
        }
        int port = root.get("port").asInt();
        String version = textRequired(root, "version");
        String operation = textRequired(root, "operation");
        JsonNode oidsNode = root.get("oids");
        if (oidsNode == null || !oidsNode.isArray() || oidsNode.isEmpty()) {
            throw new DomainException("VALIDATION_FAILED", "snmp.oids is required");
        }
        List<String> oids = new ArrayList<>();
        for (JsonNode oid : oidsNode) {
            if (!oid.isTextual() || oid.asText().isBlank()) {
                throw new DomainException("VALIDATION_FAILED", "snmp.oids entries must be non-empty strings");
            }
            oids.add(oid.asText());
        }
        if (!root.has("timeoutMs") || !root.get("timeoutMs").canConvertToLong()) {
            throw new DomainException("VALIDATION_FAILED", "snmp.timeoutMs is required");
        }
        long timeoutMs = root.get("timeoutMs").asLong();
        JsonNode limitsNode = root.get("limits");
        if (limitsNode == null || !limitsNode.isObject()) {
            throw new DomainException("VALIDATION_FAILED", "snmp.limits is required");
        }
        SnmpParams.SnmpLimits limits = new SnmpParams.SnmpLimits(
                intRequired(limitsNode, "maxOids"),
                intRequired(limitsNode, "maxWalkEntries"),
                longRequired(limitsNode, "maxBulkRepetitions"),
                intRequired(limitsNode, "maxResults"),
                intRequired(limitsNode, "maxResultBytes")
        );
        return requireSnmp(new SnmpParams(target, port, version, operation, oids, timeoutMs, limits));
    }

    private static String textRequired(JsonNode node, String field) {
        JsonNode v = node.get(field);
        if (v == null || !v.isTextual() || v.asText().isBlank()) {
            throw new DomainException("VALIDATION_FAILED", "snmp." + field + " is required");
        }
        return v.asText();
    }

    private static int intRequired(JsonNode node, String field) {
        JsonNode v = node.get(field);
        if (v == null || !v.canConvertToInt()) {
            throw new DomainException("VALIDATION_FAILED", "snmp.limits." + field + " is required");
        }
        return v.asInt();
    }

    private static long longRequired(JsonNode node, String field) {
        JsonNode v = node.get(field);
        if (v == null || !v.canConvertToLong()) {
            throw new DomainException("VALIDATION_FAILED", "snmp.limits." + field + " is required");
        }
        return v.asLong();
    }
}
