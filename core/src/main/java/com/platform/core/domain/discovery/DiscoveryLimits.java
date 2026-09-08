package com.platform.core.domain.discovery;

import com.platform.core.domain.DomainException;

import java.net.InetAddress;
import java.net.UnknownHostException;
import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.regex.Pattern;

public final class DiscoveryLimits {

    // Defensive implementation guards only; these are not commercial or certified scale limits.
    public static final int MAX_TARGETS = 256;
    public static final int MIN_CIDR_PREFIX_IPV4 = 24;
    public static final int MAX_TIMEOUT_MS = 60_000;
    public static final int MIN_TIMEOUT_MS = 100;
    public static final int DEFAULT_TIMEOUT_MS = 3000;
    public static final int MAX_RETRIES = 5;
    public static final int DEFAULT_RETRIES = 2;
    public static final int DEFAULT_SNMP_PORT = 161;
    public static final int MAX_EVIDENCE_ITEMS = MAX_TARGETS * 2;
    public static final int MAX_EVIDENCE_MAP_ENTRIES = 32;
    public static final int MAX_EVIDENCE_TEXT = 2_000;
    public static final int MAX_EVIDENCE_NESTING = 4;

    private static final Pattern HOSTNAME_LABEL = Pattern.compile("^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$");
    private static final Pattern FORBIDDEN_SECRET_PATTERN = Pattern.compile(
            "(?i)(community\\s*=|password\\s*=|secret\\s*=|private_key|auth_pass|priv_pass|api_key|token=)"
    );
    private static final Pattern FORBIDDEN_SECRET_KEY = Pattern.compile(
            "(?i).*(password|passwd|secret|community|private[-_]?key|auth[-_]?pass|priv[-_]?pass|api[-_]?key|access[-_]?token|refresh[-_]?token|bearer|cookie|credential).*"
    );

    private DiscoveryLimits() {
    }

    public static void validateNoSecrets(String... inputs) {
        if (inputs == null) return;
        for (String input : inputs) {
            if (input != null && FORBIDDEN_SECRET_PATTERN.matcher(input).find()) {
                throw new DomainException("SECRET_MATERIAL_PROHIBITED",
                        "Raw credential or secret material is strictly prohibited in discovery requests");
            }
        }
    }

    public static List<String> expandAndValidateTargets(List<String> rawTargets) {
        if (rawTargets == null || rawTargets.isEmpty()) {
            throw new DomainException("VALIDATION_FAILED", "At least one discovery target is required");
        }
        if (rawTargets.size() > MAX_TARGETS) {
            throw new DomainException("QUERY_LIMIT_EXCEEDED",
                    "Targets list exceeds defensive implementation limit of " + MAX_TARGETS);
        }

        Set<String> resolved = new LinkedHashSet<>();
        for (String raw : rawTargets) {
            if (raw == null || raw.trim().isEmpty()) {
                throw new DomainException("VALIDATION_FAILED", "Discovery target cannot be blank");
            }
            String target = raw.trim();
            validateNoSecrets(target);

            if (target.contains("/")) {
                for (String ip : expandCidr(target)) {
                    resolved.add(validateAndNormalizeTarget(ip));
                    enforceTargetCount(resolved.size());
                }
            } else {
                resolved.add(validateAndNormalizeTarget(target));
                enforceTargetCount(resolved.size());
            }
        }
        return List.copyOf(resolved);
    }

    public static TargetType classifyTargetType(String address) {
        String normalized = validateAndNormalizeTarget(address);
        if (isStrictIpv4Literal(normalized)) return TargetType.IPV4;
        if (normalized.indexOf(':') >= 0) return TargetType.IPV6;
        return TargetType.HOSTNAME;
    }

    public static void validateTargetAddress(String target) {
        validateAndNormalizeTarget(target);
    }

    public static String validateAndNormalizeTarget(String rawTarget) {
        if (rawTarget == null) {
            throw new DomainException("VALIDATION_FAILED", "Discovery target cannot be null");
        }
        String target = rawTarget.trim();
        if (target.isEmpty() || target.length() > 512) {
            throw new DomainException("VALIDATION_FAILED", "Discovery target is outside allowed text bounds");
        }
        if (target.indexOf('%') >= 0) {
            throw new DomainException("VALIDATION_FAILED", "Scoped IPv6 zone identifiers are not accepted in discovery targets");
        }

        if (isStrictIpv4Literal(target)) {
            String normalized = normalizeIpv4(target);
            validateIpLiteral(normalized);
            return normalized;
        }
        if (looksLikeIpv4Literal(target)) {
            throw new DomainException("VALIDATION_FAILED", "Malformed IPv4 discovery target: " + target);
        }

        if (target.indexOf(':') >= 0) {
            try {
                InetAddress parsed = InetAddress.getByName(target);
                if (parsed.getAddress().length != 16 && parsed.getAddress().length != 4) {
                    throw new DomainException("VALIDATION_FAILED", "Invalid IPv6 target");
                }
                validateInet(parsed, target);
                return parsed.getHostAddress().toLowerCase(Locale.ROOT);
            } catch (UnknownHostException e) {
                throw new DomainException("VALIDATION_FAILED", "Malformed IPv6 target: " + target);
            }
        }

        return normalizeHostname(target);
    }

    public static void validateEvidenceValue(Object value) {
        validateEvidenceValue(value, 0);
    }

    private static void validateEvidenceValue(Object value, int depth) {
        if (value == null || value instanceof Boolean || value instanceof Number) return;
        if (depth > MAX_EVIDENCE_NESTING) {
            throw new DomainException("QUERY_LIMIT_EXCEEDED", "Discovery evidence nesting exceeds defensive limit");
        }
        if (value instanceof String s) {
            if (s.length() > MAX_EVIDENCE_TEXT) {
                throw new DomainException("QUERY_LIMIT_EXCEEDED", "Discovery evidence text exceeds defensive limit");
            }
            validateNoSecrets(s);
            return;
        }
        if (value instanceof Map<?, ?> map) {
            if (map.size() > MAX_EVIDENCE_MAP_ENTRIES) {
                throw new DomainException("QUERY_LIMIT_EXCEEDED", "Discovery evidence map exceeds defensive entry limit");
            }
            for (Map.Entry<?, ?> entry : map.entrySet()) {
                if (!(entry.getKey() instanceof String key) || key.length() > 128) {
                    throw new DomainException("VALIDATION_FAILED", "Discovery evidence keys must be bounded strings");
                }
                if (FORBIDDEN_SECRET_KEY.matcher(key).matches()) {
                    throw new DomainException("SECRET_MATERIAL_PROHIBITED",
                            "Secret-bearing fields are prohibited in discovery evidence");
                }
                validateNoSecrets(key);
                validateEvidenceValue(entry.getValue(), depth + 1);
            }
            return;
        }
        if (value instanceof List<?> list) {
            if (list.size() > MAX_EVIDENCE_MAP_ENTRIES) {
                throw new DomainException("QUERY_LIMIT_EXCEEDED", "Discovery evidence list exceeds defensive entry limit");
            }
            for (Object item : list) validateEvidenceValue(item, depth + 1);
            return;
        }
        throw new DomainException("VALIDATION_FAILED", "Unsupported discovery evidence value type");
    }

    private static void enforceTargetCount(int size) {
        if (size > MAX_TARGETS) {
            throw new DomainException("QUERY_LIMIT_EXCEEDED",
                    "Expanded discovery target count exceeds defensive implementation limit of " + MAX_TARGETS);
        }
    }

    private static String normalizeHostname(String target) {
        String host = target.toLowerCase(Locale.ROOT);
        if (host.endsWith(".")) host = host.substring(0, host.length() - 1);
        if (host.isEmpty() || host.length() > 253 || "localhost".equals(host) || host.endsWith(".localhost")) {
            throw new DomainException("SSRF_TARGET_PROHIBITED", "Localhost targets are prohibited for discovery");
        }
        String[] labels = host.split("\\.", -1);
        if (labels.length < 1) throw new DomainException("VALIDATION_FAILED", "Invalid discovery hostname");
        for (String label : labels) {
            if (!HOSTNAME_LABEL.matcher(label).matches()) {
                throw new DomainException("VALIDATION_FAILED", "Target must be a canonical DNS hostname: " + target);
            }
        }
        // Hostnames are intentionally not resolved by Core. DNS resolution belongs at the assigned Sensor.
        return host;
    }

    private static void validateIpLiteral(String target) {
        try {
            validateInet(InetAddress.getByName(target), target);
        } catch (UnknownHostException e) {
            throw new DomainException("VALIDATION_FAILED", "Invalid IP target: " + target);
        }
    }

    private static void validateInet(InetAddress inet, String original) {
        if (inet.isLoopbackAddress()) {
            throw new DomainException("SSRF_TARGET_PROHIBITED", "Loopback addresses are prohibited for discovery: " + original);
        }
        if (inet.isLinkLocalAddress()) {
            throw new DomainException("SSRF_TARGET_PROHIBITED", "Link-local/cloud-metadata address space is prohibited for discovery: " + original);
        }
        if (inet.isMulticastAddress()) {
            throw new DomainException("SSRF_TARGET_PROHIBITED", "Multicast addresses are prohibited for discovery: " + original);
        }
        if (inet.isAnyLocalAddress()) {
            throw new DomainException("SSRF_TARGET_PROHIBITED", "Unspecified addresses are prohibited for discovery: " + original);
        }
        String hostAddress = inet.getHostAddress();
        if ("255.255.255.255".equals(hostAddress)) {
            throw new DomainException("SSRF_TARGET_PROHIBITED", "Limited broadcast address is prohibited for discovery: " + original);
        }
        // RFC1918/ULA addresses are intentionally permitted: private-network monitoring is a core NMS use case.
    }

    private static boolean isStrictIpv4Literal(String value) {
        String[] parts = value.split("\\.", -1);
        if (parts.length != 4) return false;
        for (String part : parts) {
            if (part.isEmpty() || part.length() > 3) return false;
            if (part.length() > 1 && part.charAt(0) == '0') return false;
            int v = 0;
            for (int i = 0; i < part.length(); i++) {
                char c = part.charAt(i);
                if (c < '0' || c > '9') return false;
                v = v * 10 + (c - '0');
            }
            if (v > 255) return false;
        }
        return true;
    }

    private static boolean looksLikeIpv4Literal(String value) {
        String[] parts = value.split("\\.", -1);
        if (parts.length != 4) return false;
        for (String part : parts) {
            if (part.isEmpty()) return true;
            for (int i = 0; i < part.length(); i++) {
                char c = part.charAt(i);
                if (c < '0' || c > '9') return false;
            }
        }
        return true;
    }

    private static String normalizeIpv4(String value) {
        String[] parts = value.split("\\.");
        return Integer.parseInt(parts[0]) + "." + Integer.parseInt(parts[1]) + "." +
                Integer.parseInt(parts[2]) + "." + Integer.parseInt(parts[3]);
    }

    private static List<String> expandCidr(String cidr) {
        String[] parts = cidr.split("/", -1);
        if (parts.length != 2 || !isStrictIpv4Literal(parts[0].trim())) {
            throw new DomainException("VALIDATION_FAILED", "Only canonical IPv4 CIDR expansion is supported: " + cidr);
        }
        int prefix;
        try {
            prefix = Integer.parseInt(parts[1].trim());
        } catch (NumberFormatException e) {
            throw new DomainException("VALIDATION_FAILED", "Invalid CIDR prefix");
        }
        if (prefix < MIN_CIDR_PREFIX_IPV4 || prefix > 32) {
            throw new DomainException("QUERY_LIMIT_EXCEEDED",
                    "Defensive CIDR expansion guard requires IPv4 prefix /" + MIN_CIDR_PREFIX_IPV4 + " through /32");
        }

        long ipNum = ipv4ToLong(parts[0].trim());
        long mask = prefix == 32 ? 0xFFFFFFFFL : (0xFFFFFFFFL << (32 - prefix)) & 0xFFFFFFFFL;
        long network = ipNum & mask;
        long broadcast = network | (~mask & 0xFFFFFFFFL);
        List<String> ips = new ArrayList<>();
        if (prefix == 32) {
            ips.add(toIpString(network));
        } else if (prefix == 31) {
            ips.add(toIpString(network));
            ips.add(toIpString(broadcast));
        } else {
            for (long cur = network + 1; cur < broadcast; cur++) {
                ips.add(toIpString(cur));
                if (ips.size() > MAX_TARGETS) {
                    throw new DomainException("QUERY_LIMIT_EXCEEDED", "CIDR expansion exceeds defensive target limit");
                }
            }
        }
        return ips;
    }

    private static long ipv4ToLong(String ip) {
        String[] parts = ip.split("\\.");
        return (Long.parseLong(parts[0]) << 24) | (Long.parseLong(parts[1]) << 16) |
                (Long.parseLong(parts[2]) << 8) | Long.parseLong(parts[3]);
    }

    private static String toIpString(long ip) {
        return ((ip >> 24) & 0xFF) + "." + ((ip >> 16) & 0xFF) + "." +
                ((ip >> 8) & 0xFF) + "." + (ip & 0xFF);
    }
}
