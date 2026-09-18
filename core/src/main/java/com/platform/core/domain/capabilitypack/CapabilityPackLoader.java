package com.platform.core.domain.capabilitypack;

import com.fasterxml.jackson.core.JsonFactory;
import com.fasterxml.jackson.core.JsonParser;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.StreamReadFeature;
import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;

import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.HexFormat;
import java.util.Iterator;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.regex.Pattern;

/**
 * Parses Capability Pack / Device Pack manifest bytes into an immutable descriptor.
 *
 * <p>Mirrors {@code device-packs/tools/validate.mjs} runtime-safety semantics without a
 * JSON Schema dependency. Input is raw manifest bytes only — no network I/O.
 */
public final class CapabilityPackLoader {

    /**
     * Maximum accepted manifest byte length. Same bound as {@code MAX_MANIFEST_BYTES}
     * in {@code device-packs/tools/validate.mjs}.
     */
    public static final int MAX_MANIFEST_BYTES = 256 * 1024;

    public static final String CONTRACT_SCHEMA_VERSION = "1.0.0";

    private static final Set<String> TOP_KEYS = Set.of(
            "schemaVersion", "packId", "packVersion", "name", "publisher", "certification", "metadata",
            "matchRules", "capabilities", "protocols", "references", "normalization", "dependencies", "provenance"
    );
    private static final Set<String> REQUIRED_TOP = Set.of(
            "schemaVersion", "packId", "packVersion", "name", "publisher", "certification",
            "matchRules", "capabilities", "protocols", "references", "normalization", "dependencies", "provenance"
    );
    private static final Set<String> PROTOCOLS = Set.of(
            "ICMP", "SNMP", "SNMP_TRAP", "SYSLOG", "SSH_CLI", "TELNET", "NETCONF", "REST",
            "RESTCONF", "GNMI", "GRPC", "FLOW", "DNS", "HTTP", "HTTPS", "TLS", "REDFISH",
            "WINRM", "CLOUD_API", "OTLP", "SCP", "SFTP", "TFTP", "FTP"
    );
    private static final Set<String> LEGACY_PROTOCOLS = Set.of("TELNET", "TFTP", "FTP");
    private static final Set<String> TRANSPORT_MODES = Set.of("IP", "UDP", "TCP", "TLS", "HTTPS", "SSH", "GRPC", "MTLS");
    private static final Set<String> SECURITY_MODES = Set.of(
            "TLS", "MTLS", "SSHV2", "SNMPV3", "KERBEROS", "NEGOTIATE", "NTLM", "BASIC",
            "BEARER", "API_KEY", "OAUTH2", "PUBLIC_KEY", "PASSWORD", "KEYBOARD_INTERACTIVE"
    );
    private static final Set<String> CREDENTIAL_TYPES = Set.of("SNMP", "SSH", "WINRM", "REST", "GNMI", "DB", "CLOUD", "REDFISH");
    private static final Set<String> CERTIFICATIONS = Set.of("OFFICIAL_CERTIFIED", "PARTNER_VERIFIED", "CUSTOMER_CUSTOM", "EXPERIMENTAL");
    private static final Set<String> PUBLISHER_TYPES = Set.of("PLATFORM", "PARTNER", "CUSTOMER");
    private static final Set<String> SOURCE_TYPES = Set.of("PLATFORM_SUPPLIED", "PARTNER_SUPPLIED", "CUSTOMER_SUPPLIED");
    private static final Set<String> REDISTRIBUTION = Set.of("ALLOWED", "RESTRICTED", "UNKNOWN");
    private static final Set<String> MATCH_FIELDS = Set.of("vendor", "family", "product", "model", "platform", "software", "firmware", "api", "mib");
    private static final Set<String> MATCH_OPERATORS = Set.of("EXACT", "GLOB");
    private static final Set<String> REFERENCE_KINDS = Set.of("MIB", "YANG", "OPENAPI", "MODEL", "OTHER");
    private static final Set<String> VALUE_TYPES = Set.of("BOOLEAN", "INTEGER", "NUMBER", "STRING", "STATE");
    private static final Set<String> TRANSFORM_TYPES = Set.of("IDENTITY", "SCALE", "OFFSET");

    private static final Set<String> FORBIDDEN_SECRET_KEYS = Set.of(
            "password", "passwd", "community", "secret", "apitoken", "accesstoken", "refreshtoken",
            "bearertoken", "privatekey", "clientsecret", "cookie", "cookies", "credentialvalue",
            "credentialprofileid", "authorization"
    );
    private static final Set<String> FORBIDDEN_EXECUTABLE_KEYS = Set.of(
            "script", "scripts", "shell", "command", "commands", "exec", "executable", "javascript",
            "python", "code", "inlinecode"
    );

    private static final Pattern PACK_ID_RE = Pattern.compile("^[a-z0-9]+(?:[.-][a-z0-9]+)*$");
    private static final Pattern SHORT_ID_RE = Pattern.compile("^[a-z][a-z0-9-]*$");
    private static final Pattern SEMANTIC_KEY_RE = Pattern.compile("^[a-z][a-z0-9]*(?:\\.[a-z0-9][a-z0-9_-]*)+$");
    private static final Pattern ENTITY_TYPE_RE = Pattern.compile("^[a-z][a-z0-9_.-]*$");
    private static final Pattern SEMVER_RE = Pattern.compile(
            "^(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*)?(?:\\+[0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*)?$"
    );
    private static final Pattern CONTROL_CHARS = Pattern.compile("[\\p{Cc}]");
    private static final Pattern PRIVATE_KEY = Pattern.compile("-----BEGIN [A-Z ]*PRIVATE KEY-----");
    private static final Pattern URL_CREDENTIAL = Pattern.compile("://[^/\\s:@]+:[^/\\s@]+@");
    private static final Pattern BEARER_OR_QUERY_SECRET = Pattern.compile(
            "(?i)^Bearer\\s+\\S+|[?&](?:token|api[_-]?key|password|secret)="
    );
    private static final Pattern UNSAFE_GLOB = Pattern.compile("[\\[\\]{}()|^$\\\\]|\\*\\*");
    private static final Pattern WIN_ABSOLUTE = Pattern.compile("(?i)^[a-z]:[/\\\\]");

    private final ObjectMapper mapper;

    public CapabilityPackLoader() {
        this.mapper = createStrictMapper();
    }

    static ObjectMapper createStrictMapper() {
        JsonFactory factory = JsonFactory.builder()
                .configure(StreamReadFeature.STRICT_DUPLICATE_DETECTION, true)
                .build();
        return new ObjectMapper(factory)
                .enable(DeserializationFeature.FAIL_ON_TRAILING_TOKENS);
    }

    public CapabilityPackDescriptor load(byte[] manifestBytes) {
        if (manifestBytes == null) {
            throw new CapabilityPackValidationException("INVALID_MANIFEST", "Manifest bytes are required.");
        }
        if (manifestBytes.length > MAX_MANIFEST_BYTES) {
            throw fail("MANIFEST_TOO_LARGE", "$",
                    "Manifest exceeds " + MAX_MANIFEST_BYTES + " bytes.");
        }

        final JsonNode root;
        try (JsonParser parser = mapper.createParser(manifestBytes)) {
            root = mapper.readTree(parser);
            if (parser.nextToken() != null) {
                throw fail("TRAILING_TOKENS", "$", "Trailing JSON tokens are prohibited.");
            }
        } catch (CapabilityPackValidationException ex) {
            throw ex;
        } catch (JsonProcessingException ex) {
            String code = messageIndicatesDuplicate(ex) ? "DUPLICATE_KEY" : "JSON_PARSE_ERROR";
            String safe = fixedParseMessage(code);
            throw new CapabilityPackValidationException(
                    code,
                    safe,
                    List.of(new CapabilityPackValidationException.Issue(code, "$", safe))
            );
        } catch (Exception ex) {
            String safe = fixedParseMessage("JSON_PARSE_ERROR");
            throw new CapabilityPackValidationException(
                    "JSON_PARSE_ERROR",
                    safe,
                    List.of(new CapabilityPackValidationException.Issue("JSON_PARSE_ERROR", "$", safe))
            );
        }

        List<CapabilityPackValidationException.Issue> issues = new ArrayList<>();
        if (root == null || !root.isObject()) {
            issues.add(issue("INVALID_MANIFEST", "$", "Manifest must be a JSON object."));
            throwFail(issues);
        }

        ObjectNode object = (ObjectNode) root;
        scanForbidden(object, issues, "$");
        checkKeys(object, TOP_KEYS, REQUIRED_TOP, issues, "$");
        if (!issues.isEmpty() && issues.stream().anyMatch(i -> "INVALID_TYPE".equals(i.code()))) {
            throwFail(issues);
        }

        if (object.has("schemaVersion")) {
            String schemaVersion = textOrNull(object.get("schemaVersion"));
            if (!CONTRACT_SCHEMA_VERSION.equals(schemaVersion)) {
                issues.add(issue("UNSUPPORTED_SCHEMA_VERSION", "$.schemaVersion",
                        "Expected " + CONTRACT_SCHEMA_VERSION + "."));
            }
        }

        String packId = textOrNull(object.get("packId"));
        if (object.has("packId") && !validPackId(packId)) {
            issues.add(issue("INVALID_PACK_ID", "$.packId",
                    "packId must be stable lower-case dotted/hyphenated identifier."));
        }
        String packVersion = textOrNull(object.get("packVersion"));
        if (object.has("packVersion") && !validSemver(packVersion)) {
            issues.add(issue("MALFORMED_VERSION", "$.packVersion",
                    "packVersion must use SemVer 2.0 shape."));
        }
        String name = textOrNull(object.get("name"));
        if (object.has("name")) {
            stringValue(name, 1, 160, issues, "$.name");
        }

        PackPublisher publisher = null;
        if (object.has("publisher")) {
            publisher = validatePublisher(object.get("publisher"), issues);
        }
        String certification = textOrNull(object.get("certification"));
        if (object.has("certification") && !CERTIFICATIONS.contains(certification)) {
            issues.add(issue("UNKNOWN_CERTIFICATION", "$.certification", "Unsupported enum value."));
        }

        PackMetadata metadata = PackMetadata.empty();
        if (object.has("metadata")) {
            metadata = validateMetadata(object.get("metadata"), issues);
        }

        List<MatchRule> matchRules = List.of();
        if (object.has("matchRules")) {
            matchRules = validateMatchRules(object.get("matchRules"), issues);
        }
        List<PackCapability> capabilities = List.of();
        if (object.has("capabilities")) {
            capabilities = validateCapabilities(object.get("capabilities"), issues);
        }
        List<PackProtocol> protocols = List.of();
        if (object.has("protocols")) {
            protocols = validateProtocols(object.get("protocols"), issues);
        }
        List<PackReference> references = List.of();
        if (object.has("references")) {
            references = validateReferences(object.get("references"), issues);
        }
        List<NormalizationMapping> normalization = List.of();
        if (object.has("normalization")) {
            normalization = validateNormalization(object.get("normalization"), issues);
        }
        List<PackDependency> dependencies = List.of();
        if (object.has("dependencies")) {
            dependencies = validateDependencies(object.get("dependencies"), issues);
        }
        List<PackProvenance> provenance = List.of();
        if (object.has("provenance")) {
            provenance = validateProvenance(object.get("provenance"), issues);
        }

        validateInternalReferences(object, issues);

        if (!issues.isEmpty()) {
            throwFail(issues);
        }

        // schemaVersion 1.0.0 Device Pack profile maps only to DEVICE_NETWORK (fail-closed).
        CapabilityPackClass packClass = CapabilityPackClass.DEVICE_NETWORK;

        return new CapabilityPackDescriptor(
                packId,
                packVersion,
                packClass,
                CONTRACT_SCHEMA_VERSION,
                name,
                publisher,
                certification,
                metadata,
                matchRules,
                capabilities,
                protocols,
                references,
                normalization,
                dependencies,
                provenance,
                sha256Hex(manifestBytes)
        );
    }

    static String sha256Hex(byte[] bytes) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            return HexFormat.of().formatHex(digest.digest(bytes));
        } catch (NoSuchAlgorithmException ex) {
            throw new IllegalStateException("SHA-256 not available", ex);
        }
    }

    private static PackPublisher validatePublisher(JsonNode value, List<CapabilityPackValidationException.Issue> issues) {
        String p = "$.publisher";
        if (!checkKeys(value, Set.of("type", "name"), Set.of("type", "name"), issues, p)) {
            return null;
        }
        String type = textOrNull(value.get("type"));
        if (value.has("type") && !PUBLISHER_TYPES.contains(type)) {
            issues.add(issue("UNKNOWN_ENUM", p + ".type", "Unsupported enum value."));
        }
        String name = textOrNull(value.get("name"));
        if (value.has("name")) {
            stringValue(name, 1, 160, issues, p + ".name");
        }
        if (type == null || name == null) {
            return null;
        }
        return new PackPublisher(type, name);
    }

    private static PackMetadata validateMetadata(JsonNode value, List<CapabilityPackValidationException.Issue> issues) {
        String p = "$.metadata";
        if (!checkKeys(value, Set.of("vendor", "family", "product", "description"), Set.of(), issues, p)) {
            return PackMetadata.empty();
        }
        Optional<String> vendor = optionalString(value, "vendor", 1, 160, issues, p);
        Optional<String> family = optionalString(value, "family", 1, 160, issues, p);
        Optional<String> product = optionalString(value, "product", 1, 160, issues, p);
        Optional<String> description = optionalString(value, "description", 1, 512, issues, p);
        return new PackMetadata(vendor, family, product, description);
    }

    private static List<MatchRule> validateMatchRules(JsonNode value, List<CapabilityPackValidationException.Issue> issues) {
        String p = "$.matchRules";
        if (!arrayValue(value, 1, 64, issues, p)) {
            return List.of();
        }
        ArrayNode array = (ArrayNode) value;
        unique(array, node -> textOrNull(node.get("ruleId")), "DUPLICATE_MATCH_RULE_ID", issues, p);
        unique(array, node -> node.has("priority") && node.get("priority").isIntegralNumber()
                ? node.get("priority").intValue() : null, "AMBIGUOUS_MATCH_PRIORITY", issues, p);

        Set<String> signatures = new HashSet<>();
        List<MatchRule> rules = new ArrayList<>();
        for (int i = 0; i < array.size(); i++) {
            JsonNode rule = array.get(i);
            String rp = p + "[" + i + "]";
            if (!checkKeys(rule, Set.of("ruleId", "priority", "conditions"),
                    Set.of("ruleId", "priority", "conditions"), issues, rp)) {
                continue;
            }
            String ruleId = textOrNull(rule.get("ruleId"));
            if (rule.has("ruleId") && !validShortId(ruleId)) {
                issues.add(issue("INVALID_MATCH_RULE_ID", rp + ".ruleId", "Invalid match rule ID."));
            }
            Integer priority = null;
            if (rule.has("priority")) {
                JsonNode pri = rule.get("priority");
                if (!pri.isIntegralNumber() || pri.intValue() < 0 || pri.intValue() > 1000 || !pri.canConvertToInt()) {
                    issues.add(issue("INVALID_MATCH_PRIORITY", rp + ".priority",
                            "Priority must be an integer from 0 to 1000."));
                } else {
                    priority = pri.intValue();
                }
            }
            List<MatchCondition> conditions = new ArrayList<>();
            if (!arrayValue(rule.get("conditions"), 1, 16, issues, rp + ".conditions")) {
                continue;
            }
            ArrayNode conds = (ArrayNode) rule.get("conditions");
            Set<String> seenFields = new HashSet<>();
            for (int j = 0; j < conds.size(); j++) {
                JsonNode condition = conds.get(j);
                String cp = rp + ".conditions[" + j + "]";
                if (!checkKeys(condition, Set.of("field", "operator", "value", "caseSensitive"),
                        Set.of("field", "operator", "value", "caseSensitive"), issues, cp)) {
                    continue;
                }
                String field = textOrNull(condition.get("field"));
                MatchField matchField = null;
                if (condition.has("field")) {
                    if (!MATCH_FIELDS.contains(field)) {
                        issues.add(issue("UNKNOWN_MATCH_FIELD", cp + ".field", "Unsupported enum value."));
                    } else if (!seenFields.add(field)) {
                        issues.add(issue("DUPLICATE_MATCH_FIELD", cp + ".field",
                                "A match rule may declare a selector field only once."));
                    } else {
                        matchField = MatchField.valueOf(field);
                    }
                }
                String operator = textOrNull(condition.get("operator"));
                MatchOperator matchOperator = null;
                if (condition.has("operator")) {
                    if (!MATCH_OPERATORS.contains(operator)) {
                        issues.add(issue("UNKNOWN_MATCH_OPERATOR", cp + ".operator", "Unsupported enum value."));
                    } else {
                        matchOperator = MatchOperator.valueOf(operator);
                    }
                }
                String condValue = textOrNull(condition.get("value"));
                if (condition.has("value") && stringValue(condValue, 1, 160, issues, cp + ".value")) {
                    if ("EXACT".equals(operator) && (condValue.contains("*") || condValue.contains("?"))) {
                        issues.add(issue("EXACT_MATCH_HAS_WILDCARD", cp + ".value", "EXACT values must be literal."));
                    }
                    if ("GLOB".equals(operator) && UNSAFE_GLOB.matcher(condValue).find()) {
                        issues.add(issue("UNSAFE_GLOB", cp + ".value",
                                "GLOB supports only bounded * and ? wildcards; regex syntax and ** are rejected."));
                    }
                }
                Boolean caseSensitive = null;
                if (condition.has("caseSensitive")) {
                    if (!condition.get("caseSensitive").isBoolean()) {
                        issues.add(issue("INVALID_TYPE", cp + ".caseSensitive", "Expected boolean."));
                    } else {
                        caseSensitive = condition.get("caseSensitive").booleanValue();
                    }
                }
                if (matchField != null && matchOperator != null && condValue != null && caseSensitive != null) {
                    conditions.add(new MatchCondition(matchField, matchOperator, condValue, caseSensitive));
                }
            }

            String signature = ruleSignature(priority, conditions, rule);
            if (!signatures.add(signature)) {
                issues.add(issue("DUPLICATE_MATCH_RULE", rp, "Equivalent match rule already exists in this pack."));
            }
            if (ruleId != null && priority != null && !conditions.isEmpty()) {
                rules.add(new MatchRule(ruleId, priority, conditions));
            }
        }
        return List.copyOf(rules);
    }

    private static String ruleSignature(Integer priority, List<MatchCondition> conditions, JsonNode rule) {
        if (priority == null || conditions.isEmpty()) {
            // Fall back to raw JSON-ish signature for duplicate detection when partial.
            List<String> parts = new ArrayList<>();
            JsonNode conds = rule.get("conditions");
            if (conds != null && conds.isArray()) {
                for (JsonNode c : conds) {
                    parts.add(textOrNull(c.get("field")) + "|" + textOrNull(c.get("operator"))
                            + "|" + diagnosticText(c.get("caseSensitive")) + "|" + textOrNull(c.get("value")));
                }
            }
            parts.sort(String::compareTo);
            return String.valueOf(priority) + "|" + String.join("::", parts);
        }
        List<String> parts = new ArrayList<>();
        for (MatchCondition c : conditions) {
            parts.add(c.field().name() + "|" + c.operator().name() + "|" + c.caseSensitive() + "|" + c.value());
        }
        parts.sort(String::compareTo);
        return priority + "|" + String.join("::", parts);
    }

    private static List<PackCapability> validateCapabilities(JsonNode value, List<CapabilityPackValidationException.Issue> issues) {
        String p = "$.capabilities";
        if (!arrayValue(value, 0, 128, issues, p)) {
            return List.of();
        }
        ArrayNode array = (ArrayNode) value;
        unique(array, node -> textOrNull(node.get("id")), "DUPLICATE_CAPABILITY", issues, p);
        List<PackCapability> out = new ArrayList<>();
        for (int i = 0; i < array.size(); i++) {
            JsonNode capability = array.get(i);
            String cp = p + "[" + i + "]";
            if (!checkKeys(capability, Set.of("id", "description", "requiresProtocols"), Set.of("id"), issues, cp)) {
                continue;
            }
            String id = textOrNull(capability.get("id"));
            if (capability.has("id") && !validSemanticKey(id)) {
                issues.add(issue("INVALID_CAPABILITY_ID", cp + ".id",
                        "Capability ID must use dotted lower-case form."));
            }
            Optional<String> description = Optional.empty();
            if (capability.has("description")) {
                String d = textOrNull(capability.get("description"));
                if (stringValue(d, 1, 512, issues, cp + ".description")) {
                    description = Optional.of(d);
                }
            }
            List<String> requires = new ArrayList<>();
            if (capability.has("requiresProtocols")) {
                if (arrayValue(capability.get("requiresProtocols"), 0, 16, issues, cp + ".requiresProtocols")) {
                    ArrayNode req = (ArrayNode) capability.get("requiresProtocols");
                    uniqueStrings(req, "DUPLICATE_PROTOCOL_REFERENCE", issues, cp + ".requiresProtocols");
                    for (int j = 0; j < req.size(); j++) {
                        String family = textOrNull(req.get(j));
                        if (!PROTOCOLS.contains(family)) {
                            issues.add(issue("UNKNOWN_PROTOCOL", cp + ".requiresProtocols[" + j + "]",
                                    "Unsupported enum value."));
                        } else {
                            requires.add(family);
                        }
                    }
                }
            }
            if (id != null && validSemanticKey(id)) {
                out.add(new PackCapability(id, description, requires));
            }
        }
        return List.copyOf(out);
    }

    private static List<PackProtocol> validateProtocols(JsonNode value, List<CapabilityPackValidationException.Issue> issues) {
        String p = "$.protocols";
        if (!arrayValue(value, 0, 32, issues, p)) {
            return List.of();
        }
        ArrayNode array = (ArrayNode) value;
        unique(array, node -> textOrNull(node.get("family")), "DUPLICATE_PROTOCOL", issues, p);
        Set<String> allowed = Set.of(
                "family", "requirement", "legacy", "suggestedPort", "transportModes",
                "securityModes", "credentialProfileType", "prerequisiteCapabilities"
        );
        List<PackProtocol> out = new ArrayList<>();
        for (int i = 0; i < array.size(); i++) {
            JsonNode protocol = array.get(i);
            String pp = p + "[" + i + "]";
            if (!checkKeys(protocol, allowed, Set.of("family", "requirement", "legacy"), issues, pp)) {
                continue;
            }
            String family = textOrNull(protocol.get("family"));
            if (protocol.has("family") && !PROTOCOLS.contains(family)) {
                issues.add(issue("UNKNOWN_PROTOCOL", pp + ".family", "Unsupported enum value."));
            }
            String requirement = textOrNull(protocol.get("requirement"));
            if (protocol.has("requirement") && !"REQUIRED".equals(requirement) && !"OPTIONAL".equals(requirement)) {
                issues.add(issue("UNKNOWN_REQUIREMENT", pp + ".requirement",
                        "Protocol requirement must be REQUIRED or OPTIONAL."));
            }
            Boolean legacy = null;
            if (protocol.has("legacy")) {
                if (!protocol.get("legacy").isBoolean()) {
                    issues.add(issue("INVALID_TYPE", pp + ".legacy", "Expected boolean."));
                } else {
                    legacy = protocol.get("legacy").booleanValue();
                }
            }
            if (family != null && LEGACY_PROTOCOLS.contains(family)) {
                if (!Boolean.TRUE.equals(legacy)) {
                    issues.add(issue("LEGACY_PROTOCOL_NOT_FLAGGED", pp + ".legacy",
                            "Legacy protocol must be explicitly marked legacy."));
                }
                if ("REQUIRED".equals(requirement)) {
                    issues.add(issue("LEGACY_PROTOCOL_CANNOT_BE_REQUIRED", pp + ".requirement",
                            "Legacy protocol cannot be required by a Device Pack."));
                }
            }
            Optional<Integer> suggestedPort = Optional.empty();
            if (protocol.has("suggestedPort")) {
                JsonNode port = protocol.get("suggestedPort");
                if (!port.isIntegralNumber() || port.intValue() < 1 || port.intValue() > 65535) {
                    issues.add(issue("INVALID_PORT", pp + ".suggestedPort", "Suggested port must be 1-65535."));
                } else {
                    suggestedPort = Optional.of(port.intValue());
                }
            }
            List<String> transportModes = readEnumArray(protocol, "transportModes", 8, TRANSPORT_MODES,
                    "DUPLICATE_TRANSPORT_MODE", "UNKNOWN_TRANSPORT_MODE", issues, pp);
            List<String> securityModes = readEnumArray(protocol, "securityModes", 12, SECURITY_MODES,
                    "DUPLICATE_SECURITY_MODE", "UNKNOWN_SECURITY_MODE", issues, pp);
            Optional<String> credentialProfileType = Optional.empty();
            if (protocol.has("credentialProfileType")) {
                String cpt = textOrNull(protocol.get("credentialProfileType"));
                if (!CREDENTIAL_TYPES.contains(cpt)) {
                    issues.add(issue("UNKNOWN_CREDENTIAL_PROFILE_TYPE", pp + ".credentialProfileType",
                            "Unsupported enum value."));
                } else {
                    credentialProfileType = Optional.of(cpt);
                }
            }
            List<String> prereqs = new ArrayList<>();
            if (protocol.has("prerequisiteCapabilities")) {
                if (arrayValue(protocol.get("prerequisiteCapabilities"), 0, 16, issues, pp + ".prerequisiteCapabilities")) {
                    ArrayNode arr = (ArrayNode) protocol.get("prerequisiteCapabilities");
                    uniqueStrings(arr, "DUPLICATE_CAPABILITY_REFERENCE", issues, pp + ".prerequisiteCapabilities");
                    for (int j = 0; j < arr.size(); j++) {
                        String id = textOrNull(arr.get(j));
                        if (!validSemanticKey(id)) {
                            issues.add(issue("INVALID_CAPABILITY_ID", pp + ".prerequisiteCapabilities[" + j + "]",
                                    "Invalid capability reference."));
                        } else {
                            prereqs.add(id);
                        }
                    }
                }
            }
            if (family != null && requirement != null && legacy != null && PROTOCOLS.contains(family)
                    && ("REQUIRED".equals(requirement) || "OPTIONAL".equals(requirement))) {
                out.add(new PackProtocol(family, requirement, legacy, suggestedPort, transportModes,
                        securityModes, credentialProfileType, prereqs));
            }
        }
        return List.copyOf(out);
    }

    private static List<PackReference> validateReferences(JsonNode value, List<CapabilityPackValidationException.Issue> issues) {
        String p = "$.references";
        if (!arrayValue(value, 0, 256, issues, p)) {
            return List.of();
        }
        ArrayNode array = (ArrayNode) value;
        unique(array, node -> textOrNull(node.get("id")), "DUPLICATE_REFERENCE", issues, p);
        Set<String> allowed = Set.of("id", "kind", "version", "packagePath", "sourceReference", "provenanceId");
        List<PackReference> out = new ArrayList<>();
        for (int i = 0; i < array.size(); i++) {
            JsonNode reference = array.get(i);
            String rp = p + "[" + i + "]";
            if (!checkKeys(reference, allowed, Set.of("id", "kind", "provenanceId"), issues, rp)) {
                continue;
            }
            String id = textOrNull(reference.get("id"));
            if (reference.has("id") && !validShortId(id)) {
                issues.add(issue("INVALID_REFERENCE_ID", rp + ".id", "Invalid reference ID."));
            }
            String kind = textOrNull(reference.get("kind"));
            if (reference.has("kind") && !REFERENCE_KINDS.contains(kind)) {
                issues.add(issue("UNKNOWN_REFERENCE_KIND", rp + ".kind", "Unsupported enum value."));
            }
            Optional<String> version = optionalString(reference, "version", 1, 80, issues, rp);
            Optional<String> packagePath = Optional.empty();
            if (reference.has("packagePath")) {
                String path = textOrNull(reference.get("packagePath"));
                if (!safePackagePath(path)) {
                    issues.add(issue("PATH_TRAVERSAL", rp + ".packagePath",
                            "Package path must be safe and relative."));
                } else {
                    packagePath = Optional.of(path);
                }
            }
            Optional<String> sourceReference = Optional.empty();
            if (reference.has("sourceReference")) {
                String src = textOrNull(reference.get("sourceReference"));
                if (stringValue(src, 1, 512, issues, rp + ".sourceReference")) {
                    if (absoluteFilesystemReference(src)) {
                        issues.add(issue("ABSOLUTE_FILESYSTEM_REFERENCE", rp + ".sourceReference",
                                "Absolute filesystem references are prohibited."));
                    } else {
                        sourceReference = Optional.of(src);
                    }
                }
            }
            String provenanceId = textOrNull(reference.get("provenanceId"));
            if (reference.has("provenanceId") && !validShortId(provenanceId)) {
                issues.add(issue("INVALID_PROVENANCE_ID", rp + ".provenanceId", "Invalid provenance reference."));
            }
            if (!reference.has("packagePath") && !reference.has("sourceReference")) {
                issues.add(issue("REFERENCE_LOCATION_REQUIRED", rp,
                        "Reference requires packagePath or sourceReference metadata."));
            }
            if (id != null && kind != null && provenanceId != null && validShortId(id)
                    && REFERENCE_KINDS.contains(kind) && validShortId(provenanceId)
                    && (packagePath.isPresent() || sourceReference.isPresent())) {
                out.add(new PackReference(id, kind, version, packagePath, sourceReference, provenanceId));
            }
        }
        return List.copyOf(out);
    }

    private static List<NormalizationMapping> validateNormalization(JsonNode value, List<CapabilityPackValidationException.Issue> issues) {
        String p = "$.normalization";
        if (!arrayValue(value, 0, 512, issues, p)) {
            return List.of();
        }
        ArrayNode array = (ArrayNode) value;
        unique(array, node -> textOrNull(node.get("key")), "NORMALIZATION_COLLISION", issues, p);
        Set<String> allowed = Set.of("key", "source", "valueType", "unit", "target", "qualityFallback", "provenanceId", "transform");
        List<NormalizationMapping> out = new ArrayList<>();
        for (int i = 0; i < array.size(); i++) {
            JsonNode mapping = array.get(i);
            String mp = p + "[" + i + "]";
            if (!checkKeys(mapping, allowed,
                    Set.of("key", "source", "valueType", "target", "qualityFallback", "provenanceId"), issues, mp)) {
                continue;
            }
            String key = textOrNull(mapping.get("key"));
            if (mapping.has("key") && !validSemanticKey(key)) {
                issues.add(issue("INVALID_NORMALIZATION_KEY", mp + ".key",
                        "Normalization key must use dotted lower-case form."));
            }
            String valueType = textOrNull(mapping.get("valueType"));
            if (mapping.has("valueType") && !VALUE_TYPES.contains(valueType)) {
                issues.add(issue("UNKNOWN_VALUE_TYPE", mp + ".valueType", "Unsupported enum value."));
            }
            Optional<String> unit = optionalString(mapping, "unit", 1, 32, issues, mp);
            String qualityFallback = textOrNull(mapping.get("qualityFallback"));
            if (mapping.has("qualityFallback") && !"UNKNOWN".equals(qualityFallback)) {
                issues.add(issue("UNSAFE_QUALITY_FALLBACK", mp + ".qualityFallback",
                        "Fallback quality may only be UNKNOWN; packs must not fabricate healthy data."));
            }
            String provenanceId = textOrNull(mapping.get("provenanceId"));
            if (mapping.has("provenanceId") && !validShortId(provenanceId)) {
                issues.add(issue("INVALID_PROVENANCE_ID", mp + ".provenanceId", "Invalid provenance reference."));
            }

            NormalizationSource source = null;
            if (mapping.has("source")) {
                JsonNode spNode = mapping.get("source");
                String sp = mp + ".source";
                if (checkKeys(spNode, Set.of("protocol", "field", "referenceId"), Set.of("protocol", "field"), issues, sp)) {
                    String protocol = textOrNull(spNode.get("protocol"));
                    if (spNode.has("protocol") && !PROTOCOLS.contains(protocol)) {
                        issues.add(issue("UNKNOWN_PROTOCOL", sp + ".protocol", "Unsupported enum value."));
                    }
                    String field = textOrNull(spNode.get("field"));
                    boolean fieldOk = spNode.has("field") && stringValue(field, 1, 256, issues, sp + ".field");
                    Optional<String> referenceId = Optional.empty();
                    if (spNode.has("referenceId")) {
                        String rid = textOrNull(spNode.get("referenceId"));
                        if (!validShortId(rid)) {
                            issues.add(issue("INVALID_REFERENCE_ID", sp + ".referenceId", "Invalid reference ID."));
                        } else {
                            referenceId = Optional.of(rid);
                        }
                    }
                    if (protocol != null && PROTOCOLS.contains(protocol) && fieldOk) {
                        source = new NormalizationSource(protocol, field, referenceId);
                    }
                }
            }

            NormalizationTarget target = null;
            if (mapping.has("target")) {
                JsonNode tpNode = mapping.get("target");
                String tp = mp + ".target";
                if (checkKeys(tpNode, Set.of("entityType", "component"), Set.of("entityType"), issues, tp)) {
                    String entityType = textOrNull(tpNode.get("entityType"));
                    boolean ok = tpNode.has("entityType") && stringValue(entityType, 1, 80, issues, tp + ".entityType");
                    if (ok && !ENTITY_TYPE_RE.matcher(entityType).matches()) {
                        issues.add(issue("INVALID_ENTITY_TYPE", tp + ".entityType", "Invalid entity type identifier."));
                        ok = false;
                    }
                    Optional<String> component = optionalString(tpNode, "component", 1, 160, issues, tp);
                    if (ok) {
                        target = new NormalizationTarget(entityType, component);
                    }
                }
            }

            Optional<NormalizationTransform> transform = Optional.empty();
            if (mapping.has("transform")) {
                transform = Optional.ofNullable(validateTransform(mapping.get("transform"), issues, mp + ".transform"));
            }

            if (key != null && validSemanticKey(key) && source != null && valueType != null && VALUE_TYPES.contains(valueType)
                    && target != null && "UNKNOWN".equals(qualityFallback) && provenanceId != null && validShortId(provenanceId)) {
                out.add(new NormalizationMapping(key, source, valueType, unit, target, qualityFallback, provenanceId, transform));
            }
        }
        return List.copyOf(out);
    }

    private static NormalizationTransform validateTransform(
            JsonNode value, List<CapabilityPackValidationException.Issue> issues, String p) {
        if (!checkKeys(value, Set.of("type", "factor", "offset"), Set.of("type"), issues, p)) {
            return null;
        }
        String type = textOrNull(value.get("type"));
        if (!value.has("type") || !TRANSFORM_TYPES.contains(type)) {
            if (value.has("type")) {
                issues.add(issue("UNKNOWN_TRANSFORM", p + ".type", "Unsupported enum value."));
            }
            return null;
        }
        if ("IDENTITY".equals(type) && (value.has("factor") || value.has("offset"))) {
            issues.add(issue("INVALID_TRANSFORM_PARAMETERS", p, "IDENTITY accepts no parameters."));
        }
        Optional<Double> factor = Optional.empty();
        Optional<Double> offset = Optional.empty();
        if ("SCALE".equals(type)) {
            if (!value.has("factor") || !value.get("factor").isNumber() || !Double.isFinite(value.get("factor").asDouble())) {
                issues.add(issue("INVALID_TRANSFORM_PARAMETERS", p + ".factor", "SCALE requires a finite factor."));
            } else {
                factor = Optional.of(value.get("factor").asDouble());
            }
            if (value.has("offset")) {
                issues.add(issue("INVALID_TRANSFORM_PARAMETERS", p + ".offset", "SCALE does not accept offset."));
            }
        }
        if ("OFFSET".equals(type)) {
            if (!value.has("offset") || !value.get("offset").isNumber() || !Double.isFinite(value.get("offset").asDouble())) {
                issues.add(issue("INVALID_TRANSFORM_PARAMETERS", p + ".offset", "OFFSET requires a finite offset."));
            } else {
                offset = Optional.of(value.get("offset").asDouble());
            }
            if (value.has("factor")) {
                issues.add(issue("INVALID_TRANSFORM_PARAMETERS", p + ".factor", "OFFSET does not accept factor."));
            }
        }
        return new NormalizationTransform(type, factor, offset);
    }

    private static List<PackDependency> validateDependencies(JsonNode value, List<CapabilityPackValidationException.Issue> issues) {
        String p = "$.dependencies";
        if (!arrayValue(value, 0, 64, issues, p)) {
            return List.of();
        }
        ArrayNode array = (ArrayNode) value;
        Set<String> seen = new HashSet<>();
        List<PackDependency> out = new ArrayList<>();
        for (int i = 0; i < array.size(); i++) {
            JsonNode dependency = array.get(i);
            String dp = p + "[" + i + "]";
            if (!checkKeys(dependency, Set.of("packId", "packVersion"), Set.of("packId", "packVersion"), issues, dp)) {
                continue;
            }
            String depPackId = textOrNull(dependency.get("packId"));
            if (dependency.has("packId") && !validPackId(depPackId)) {
                issues.add(issue("INVALID_PACK_ID", dp + ".packId", "Invalid dependency packId."));
            }
            String depVersion = textOrNull(dependency.get("packVersion"));
            if (dependency.has("packVersion") && !validSemver(depVersion)) {
                issues.add(issue("MALFORMED_VERSION", dp + ".packVersion",
                        "Dependency version must use SemVer shape."));
            }
            if (depPackId != null && depVersion != null) {
                String identity = depPackId + "@" + depVersion;
                if (!seen.add(identity)) {
                    issues.add(issue("DUPLICATE_DEPENDENCY", dp, "Duplicate value."));
                } else if (validPackId(depPackId) && validSemver(depVersion)) {
                    out.add(new PackDependency(depPackId, depVersion));
                }
            }
        }
        return List.copyOf(out);
    }

    private static List<PackProvenance> validateProvenance(JsonNode value, List<CapabilityPackValidationException.Issue> issues) {
        String p = "$.provenance";
        if (!arrayValue(value, 1, 128, issues, p)) {
            return List.of();
        }
        ArrayNode array = (ArrayNode) value;
        unique(array, node -> textOrNull(node.get("id")), "DUPLICATE_PROVENANCE", issues, p);
        Set<String> allowed = Set.of("id", "sourceType", "sourceReference", "license", "redistribution");
        List<PackProvenance> out = new ArrayList<>();
        for (int i = 0; i < array.size(); i++) {
            JsonNode entry = array.get(i);
            String ep = p + "[" + i + "]";
            if (!checkKeys(entry, allowed, Set.of("id", "sourceType", "sourceReference", "redistribution"), issues, ep)) {
                continue;
            }
            String id = textOrNull(entry.get("id"));
            if (entry.has("id") && !validShortId(id)) {
                issues.add(issue("INVALID_PROVENANCE_ID", ep + ".id", "Invalid provenance ID."));
            }
            String sourceType = textOrNull(entry.get("sourceType"));
            if (entry.has("sourceType") && !SOURCE_TYPES.contains(sourceType)) {
                issues.add(issue("UNKNOWN_SOURCE_TYPE", ep + ".sourceType", "Unsupported enum value."));
            }
            String sourceReference = textOrNull(entry.get("sourceReference"));
            if (entry.has("sourceReference") && stringValue(sourceReference, 1, 512, issues, ep + ".sourceReference")) {
                if (absoluteFilesystemReference(sourceReference)) {
                    issues.add(issue("ABSOLUTE_FILESYSTEM_REFERENCE", ep + ".sourceReference",
                            "Absolute filesystem references are prohibited."));
                }
            }
            Optional<String> license = optionalString(entry, "license", 1, 160, issues, ep);
            String redistribution = textOrNull(entry.get("redistribution"));
            if (entry.has("redistribution") && !REDISTRIBUTION.contains(redistribution)) {
                issues.add(issue("UNKNOWN_REDISTRIBUTION", ep + ".redistribution",
                        "Unsupported enum value."));
            }
            if (id != null && validShortId(id) && sourceType != null && SOURCE_TYPES.contains(sourceType)
                    && sourceReference != null && redistribution != null && REDISTRIBUTION.contains(redistribution)
                    && !absoluteFilesystemReference(sourceReference)) {
                out.add(new PackProvenance(id, sourceType, sourceReference, license, redistribution));
            }
        }
        return List.copyOf(out);
    }

    private static void validateInternalReferences(ObjectNode manifest, List<CapabilityPackValidationException.Issue> issues) {
        Set<String> protocols = new LinkedHashSet<>();
        if (manifest.path("protocols").isArray()) {
            for (JsonNode protocol : manifest.get("protocols")) {
                String family = textOrNull(protocol.get("family"));
                if (PROTOCOLS.contains(family)) {
                    protocols.add(family);
                }
            }
        }
        Set<String> capabilities = new LinkedHashSet<>();
        if (manifest.path("capabilities").isArray()) {
            for (JsonNode capability : manifest.get("capabilities")) {
                String id = textOrNull(capability.get("id"));
                if (validSemanticKey(id)) {
                    capabilities.add(id);
                }
            }
        }
        Set<String> references = new LinkedHashSet<>();
        if (manifest.path("references").isArray()) {
            for (JsonNode reference : manifest.get("references")) {
                String id = textOrNull(reference.get("id"));
                if (validShortId(id)) {
                    references.add(id);
                }
            }
        }
        Set<String> provenance = new LinkedHashSet<>();
        if (manifest.path("provenance").isArray()) {
            for (JsonNode entry : manifest.get("provenance")) {
                String id = textOrNull(entry.get("id"));
                if (validShortId(id)) {
                    provenance.add(id);
                }
            }
        }

        if (manifest.path("capabilities").isArray()) {
            ArrayNode caps = (ArrayNode) manifest.get("capabilities");
            for (int i = 0; i < caps.size(); i++) {
                JsonNode capability = caps.get(i);
                if (!capability.path("requiresProtocols").isArray()) {
                    continue;
                }
                ArrayNode req = (ArrayNode) capability.get("requiresProtocols");
                for (int j = 0; j < req.size(); j++) {
                    String family = textOrNull(req.get(j));
                    if (PROTOCOLS.contains(family) && !protocols.contains(family)) {
                        issues.add(issue("CAPABILITY_PROTOCOL_NOT_DECLARED",
                                "$.capabilities[" + i + "].requiresProtocols[" + j + "]",
                                "Referenced protocol is not declared."));
                    }
                }
            }
        }

        if (manifest.path("protocols").isArray()) {
            ArrayNode prots = (ArrayNode) manifest.get("protocols");
            for (int i = 0; i < prots.size(); i++) {
                JsonNode protocol = prots.get(i);
                if (!protocol.path("prerequisiteCapabilities").isArray()) {
                    continue;
                }
                ArrayNode prereqs = (ArrayNode) protocol.get("prerequisiteCapabilities");
                for (int j = 0; j < prereqs.size(); j++) {
                    String id = textOrNull(prereqs.get(j));
                    if (validSemanticKey(id) && !capabilities.contains(id)) {
                        issues.add(issue("CAPABILITY_REFERENCE_NOT_FOUND",
                                "$.protocols[" + i + "].prerequisiteCapabilities[" + j + "]",
                                "Referenced capability is not declared."));
                    }
                }
            }
        }

        if (manifest.path("references").isArray()) {
            ArrayNode refs = (ArrayNode) manifest.get("references");
            for (int i = 0; i < refs.size(); i++) {
                String provenanceId = textOrNull(refs.get(i).get("provenanceId"));
                if (validShortId(provenanceId) && !provenance.contains(provenanceId)) {
                    issues.add(issue("PROVENANCE_REFERENCE_NOT_FOUND",
                            "$.references[" + i + "].provenanceId",
                            "Referenced provenance is not declared."));
                }
            }
        }

        if (manifest.path("normalization").isArray()) {
            ArrayNode norms = (ArrayNode) manifest.get("normalization");
            for (int i = 0; i < norms.size(); i++) {
                JsonNode mapping = norms.get(i);
                String protocol = textOrNull(mapping.path("source").get("protocol"));
                if (PROTOCOLS.contains(protocol) && !protocols.contains(protocol)) {
                    issues.add(issue("NORMALIZATION_PROTOCOL_NOT_DECLARED",
                            "$.normalization[" + i + "].source.protocol",
                            "Referenced protocol is not declared."));
                }
                String referenceId = textOrNull(mapping.path("source").get("referenceId"));
                if (validShortId(referenceId) && !references.contains(referenceId)) {
                    issues.add(issue("REFERENCE_NOT_FOUND",
                            "$.normalization[" + i + "].source.referenceId",
                            "Referenced reference is not declared."));
                }
                String provenanceId = textOrNull(mapping.get("provenanceId"));
                if (validShortId(provenanceId) && !provenance.contains(provenanceId)) {
                    issues.add(issue("PROVENANCE_REFERENCE_NOT_FOUND",
                            "$.normalization[" + i + "].provenanceId",
                            "Referenced provenance is not declared."));
                }
            }
        }
    }

    private static void scanForbidden(JsonNode value, List<CapabilityPackValidationException.Issue> issues, String jsonPath) {
        if (value == null || value.isNull()) {
            return;
        }
        if (value.isArray()) {
            for (int i = 0; i < value.size(); i++) {
                scanForbidden(value.get(i), issues, jsonPath + "[" + i + "]");
            }
            return;
        }
        if (!value.isObject()) {
            return;
        }
        Iterator<Map.Entry<String, JsonNode>> fields = value.fields();
        while (fields.hasNext()) {
            Map.Entry<String, JsonNode> entry = fields.next();
            String key = entry.getKey();
            JsonNode child = entry.getValue();
            String childPath = jsonPath + "." + key;
            String token = token(key);
            if (FORBIDDEN_SECRET_KEYS.contains(token)) {
                issues.add(issue("FORBIDDEN_SECRET_FIELD", childPath,
                        "Credential secret values and Credential Profile IDs are prohibited."));
            }
            if (FORBIDDEN_EXECUTABLE_KEYS.contains(token)) {
                issues.add(issue("FORBIDDEN_EXECUTABLE_FIELD", childPath,
                        "Executable/script/command content is prohibited."));
            }
            if ("packagepath".equals(token) && child.isTextual() && !safePackagePath(child.asText())) {
                issues.add(issue("PATH_TRAVERSAL", childPath, "Package path must be a safe relative POSIX path."));
            }
            if ("sourcereference".equals(token) && child.isTextual() && absoluteFilesystemReference(child.asText())) {
                issues.add(issue("ABSOLUTE_FILESYSTEM_REFERENCE", childPath,
                        "Absolute filesystem references are prohibited."));
            }
            if (child.isTextual()) {
                String text = child.asText();
                if (PRIVATE_KEY.matcher(text).find()) {
                    issues.add(issue("FORBIDDEN_SECRET_VALUE", childPath, "Private-key material is prohibited."));
                }
                if (URL_CREDENTIAL.matcher(text).find()) {
                    issues.add(issue("EMBEDDED_URL_CREDENTIAL", childPath,
                            "Credentials embedded in URLs are prohibited."));
                }
                if (BEARER_OR_QUERY_SECRET.matcher(text).find()) {
                    issues.add(issue("FORBIDDEN_SECRET_VALUE", childPath, "Token/secret material is prohibited."));
                }
            }
            scanForbidden(child, issues, childPath);
        }
    }

    private static List<String> readEnumArray(
            JsonNode parent,
            String field,
            int max,
            Set<String> allowed,
            String duplicateCode,
            String unknownCode,
            List<CapabilityPackValidationException.Issue> issues,
            String parentPath
    ) {
        if (!parent.has(field)) {
            return List.of();
        }
        String path = parentPath + "." + field;
        if (!arrayValue(parent.get(field), 0, max, issues, path)) {
            return List.of();
        }
        ArrayNode array = (ArrayNode) parent.get(field);
        uniqueStrings(array, duplicateCode, issues, path);
        List<String> out = new ArrayList<>();
        for (int j = 0; j < array.size(); j++) {
            String v = textOrNull(array.get(j));
            if (!allowed.contains(v)) {
                issues.add(issue(unknownCode, path + "[" + j + "]", "Unsupported enum value."));
            } else {
                out.add(v);
            }
        }
        return List.copyOf(out);
    }

    private static boolean checkKeys(
            JsonNode object,
            Set<String> allowed,
            Set<String> required,
            List<CapabilityPackValidationException.Issue> issues,
            String jsonPath
    ) {
        if (object == null || !object.isObject()) {
            issues.add(issue("INVALID_TYPE", jsonPath, "Expected object."));
            return false;
        }
        Iterator<String> names = object.fieldNames();
        while (names.hasNext()) {
            String key = names.next();
            if (!allowed.contains(key)) {
                issues.add(issue("UNKNOWN_FIELD", jsonPath + "." + key,
                        "Field is not defined by the Device Pack contract."));
            }
        }
        for (String key : required) {
            if (!object.has(key)) {
                issues.add(issue("MISSING_REQUIRED_FIELD", jsonPath + "." + key, "Required field is missing."));
            }
        }
        return true;
    }

    private static boolean stringValue(
            String value, int min, int max, List<CapabilityPackValidationException.Issue> issues, String jsonPath) {
        if (value == null) {
            issues.add(issue("INVALID_TYPE", jsonPath, "Expected string."));
            return false;
        }
        if (value.length() < min || value.length() > max) {
            issues.add(issue("STRING_BOUNDS", jsonPath, "String length must be " + min + "-" + max + "."));
            return false;
        }
        if (CONTROL_CHARS.matcher(value).find()) {
            issues.add(issue("CONTROL_CHARACTER", jsonPath, "Control characters are prohibited."));
        }
        return true;
    }

    private static Optional<String> optionalString(
            JsonNode object, String field, int min, int max,
            List<CapabilityPackValidationException.Issue> issues, String parentPath) {
        if (!object.has(field)) {
            return Optional.empty();
        }
        String value = textOrNull(object.get(field));
        if (stringValue(value, min, max, issues, parentPath + "." + field)) {
            return Optional.of(value);
        }
        return Optional.empty();
    }

    private static boolean arrayValue(
            JsonNode value, int min, int max, List<CapabilityPackValidationException.Issue> issues, String jsonPath) {
        if (value == null || !value.isArray()) {
            issues.add(issue("INVALID_TYPE", jsonPath, "Expected array."));
            return false;
        }
        if (value.size() < min || value.size() > max) {
            issues.add(issue("ARRAY_BOUNDS", jsonPath, "Array length must be " + min + "-" + max + "."));
        }
        return true;
    }

    private static void unique(
            ArrayNode items,
            java.util.function.Function<JsonNode, Object> selector,
            String code,
            List<CapabilityPackValidationException.Issue> issues,
            String jsonPath
    ) {
        Set<Object> seen = new HashSet<>();
        for (int i = 0; i < items.size(); i++) {
            Object value = selector.apply(items.get(i));
            if (!(value instanceof String) && !(value instanceof Number)) {
                continue;
            }
            if (!seen.add(value)) {
                issues.add(issue(code, jsonPath + "[" + i + "]", "Duplicate value."));
            }
        }
    }

    private static void uniqueStrings(
            ArrayNode items, String code, List<CapabilityPackValidationException.Issue> issues, String jsonPath) {
        unique(items, CapabilityPackLoader::textOrNull, code, issues, jsonPath);
    }

    private static boolean validPackId(String v) {
        return v != null && v.length() >= 3 && v.length() <= 128 && PACK_ID_RE.matcher(v).matches();
    }

    private static boolean validShortId(String v) {
        return v != null && v.length() >= 1 && v.length() <= 64 && SHORT_ID_RE.matcher(v).matches();
    }

    private static boolean validSemver(String v) {
        return v != null && v.length() >= 5 && v.length() <= 64 && SEMVER_RE.matcher(v).matches();
    }

    private static boolean validSemanticKey(String v) {
        return v != null && v.length() >= 3 && v.length() <= 128 && SEMANTIC_KEY_RE.matcher(v).matches();
    }

    static boolean safePackagePath(String value) {
        if (value == null || value.length() < 1 || value.length() > 240) {
            return false;
        }
        String lowered = value.toLowerCase(Locale.ROOT);
        if (value.startsWith("/") || value.contains("\\") || WIN_ABSOLUTE.matcher(value).find()) {
            return false;
        }
        if (lowered.contains("%2e") || lowered.contains("%2f") || lowered.contains("%5c")) {
            return false;
        }
        String[] parts = value.split("/", -1);
        for (String part : parts) {
            if (part.isEmpty() || ".".equals(part) || "..".equals(part)) {
                return false;
            }
        }
        return true;
    }

    static boolean absoluteFilesystemReference(String value) {
        if (value == null) {
            return false;
        }
        return value.startsWith("/")
                || WIN_ABSOLUTE.matcher(value).find()
                || value.regionMatches(true, 0, "file:", 0, 5);
    }

    private static String token(String key) {
        return key.toLowerCase(Locale.ROOT).replace("_", "").replace("-", "").replace(".", "");
    }

    /**
     * STRING contract fields accept JSON text nodes only. Boolean/number coercion is rejected
     * (callers treat null as an invalid-type / validation failure).
     */
    private static String textOrNull(JsonNode node) {
        if (node == null || node.isNull() || !node.isTextual()) {
            return null;
        }
        return node.asText();
    }

    /** Diagnostic-only stringification; must not be used for STRING contract acceptance. */
    private static String diagnosticText(JsonNode node) {
        if (node == null || node.isNull()) {
            return null;
        }
        if (node.isTextual() || node.isNumber() || node.isBoolean()) {
            return node.asText();
        }
        return null;
    }

    private static CapabilityPackValidationException.Issue issue(String code, String path, String message) {
        return new CapabilityPackValidationException.Issue(code, path, message);
    }

    private static CapabilityPackValidationException fail(String code, String path, String message) {
        return new CapabilityPackValidationException(code, message, List.of(issue(code, path, message)));
    }

    private static void throwFail(List<CapabilityPackValidationException.Issue> issues) {
        CapabilityPackValidationException.Issue first = issues.get(0);
        throw new CapabilityPackValidationException(
                first.code(),
                first.message(),
                issues,
                Map.of("path", first.path())
        );
    }

    private static String fixedParseMessage(String code) {
        if ("DUPLICATE_KEY".equals(code)) {
            return "Manifest JSON contains duplicate object keys.";
        }
        return "Manifest JSON is malformed.";
    }

    private static boolean messageIndicatesDuplicate(Exception ex) {
        String message = ex.getMessage();
        if (message == null) {
            return false;
        }
        String lower = message.toLowerCase(Locale.ROOT);
        return lower.contains("duplicate field") || lower.contains("duplicate key");
    }
}
