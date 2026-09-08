package com.platform.core.domain.discovery;

import com.platform.core.domain.DomainException;

import java.net.InetAddress;
import java.net.UnknownHostException;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.regex.Pattern;

public final class DiscoveryLimits {

    public static final int MAX_TARGETS = 256;
    public static final int MIN_CIDR_PREFIX_IPV4 = 24; // Defensive safety: no scans broader than /24
    public static final int MAX_TIMEOUT_MS = 60_000;
    public static final int MIN_TIMEOUT_MS = 100;
    public static final int DEFAULT_TIMEOUT_MS = 3000;
    public static final int MAX_RETRIES = 5;
    public static final int DEFAULT_RETRIES = 2;
    public static final int DEFAULT_SNMP_PORT = 161;

    private static final Pattern HOSTNAME_PATTERN = Pattern.compile(
            "^(?=.{1,253}$)(?!-)[A-Za-z0-9-_]{1,63}(?<!-)(\\.(?!-)[A-Za-z0-9-_]{1,63}(?<!-))*$"
    );

    private static final Pattern FORBIDDEN_SECRET_PATTERN = Pattern.compile(
            "(?i)(community\\s*=|password\\s*=|secret\\s*=|private_key|auth_pass|priv_pass|api_key|token=)"
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
                    "Targets list exceeds maximum bounded limit of " + MAX_TARGETS);
        }

        List<String> resolved = new ArrayList<>();
        for (String raw : rawTargets) {
            if (raw == null || raw.trim().isEmpty()) {
                throw new DomainException("VALIDATION_FAILED", "Discovery target cannot be blank");
            }
            String target = raw.trim();
            validateNoSecrets(target);

            if (target.contains("/")) {
                // CIDR notation
                List<String> expanded = expandCidr(target);
                for (String ip : expanded) {
                    validateTargetAddress(ip);
                    if (!resolved.contains(ip)) {
                        resolved.add(ip);
                    }
                }
            } else {
                validateTargetAddress(target);
                if (!resolved.contains(target)) {
                    resolved.add(target);
                }
            }

            if (resolved.size() > MAX_TARGETS) {
                throw new DomainException("QUERY_LIMIT_EXCEEDED",
                        "Expanded discovery target count exceeds maximum defensive limit of " + MAX_TARGETS);
            }
        }

        return List.copyOf(resolved);
    }

    public static TargetType classifyTargetType(String address) {
        try {
            InetAddress inet = InetAddress.getByName(address);
            if (inet instanceof java.net.Inet4Address) {
                return TargetType.IPV4;
            } else if (inet instanceof java.net.Inet6Address) {
                return TargetType.IPV6;
            }
        } catch (UnknownHostException ignored) {
        }
        return TargetType.HOSTNAME;
    }

    public static void validateTargetAddress(String target) {
        try {
            InetAddress inet = InetAddress.getByName(target);
            if (inet.isLoopbackAddress()) {
                throw new DomainException("SSRF_TARGET_PROHIBITED",
                        "Loopback addresses (127.0.0.0/8, ::1) are strictly prohibited for discovery: " + target);
            }
            if (inet.isLinkLocalAddress()) {
                throw new DomainException("SSRF_TARGET_PROHIBITED",
                        "Link-local addresses are strictly prohibited for discovery: " + target);
            }
            if (inet.isMulticastAddress()) {
                throw new DomainException("SSRF_TARGET_PROHIBITED",
                        "Multicast addresses are prohibited for discovery: " + target);
            }
            if (inet.isAnyLocalAddress()) {
                throw new DomainException("SSRF_TARGET_PROHIBITED",
                        "Wildcard address 0.0.0.0/:: is prohibited for discovery: " + target);
            }

            String hostAddress = inet.getHostAddress();
            if ("255.255.255.255".equals(hostAddress)) {
                throw new DomainException("SSRF_TARGET_PROHIBITED",
                        "Broadcast address is prohibited for discovery: " + target);
            }
            if (hostAddress.startsWith("169.254.")) {
                throw new DomainException("SSRF_TARGET_PROHIBITED",
                        "Cloud metadata address (169.254.x.x) is strictly prohibited: " + target);
            }
        } catch (UnknownHostException e) {
            // Check hostname format
            if (!HOSTNAME_PATTERN.matcher(target).matches() || target.toLowerCase(Locale.ROOT).contains("localhost")) {
                throw new DomainException("VALIDATION_FAILED",
                        "Target must be a valid IP address or canonical hostname: " + target);
            }
        }
    }

    private static List<String> expandCidr(String cidr) {
        String[] parts = cidr.split("/");
        if (parts.length != 2) {
            throw new DomainException("VALIDATION_FAILED", "Malformed CIDR notation: " + cidr);
        }
        String ipPart = parts[0].trim();
        int prefix;
        try {
            prefix = Integer.parseInt(parts[1].trim());
        } catch (NumberFormatException e) {
            throw new DomainException("VALIDATION_FAILED", "Invalid CIDR prefix: " + parts[1]);
        }

        if (prefix < MIN_CIDR_PREFIX_IPV4 || prefix > 32) {
            throw new DomainException("QUERY_LIMIT_EXCEEDED",
                    "Defensive scan boundary: CIDR prefix must be between /" + MIN_CIDR_PREFIX_IPV4
                            + " and /32 to prevent unbounded network scanning: " + cidr);
        }

        long ipNum;
        try {
            byte[] octets = InetAddress.getByName(ipPart).getAddress();
            if (octets.length != 4) {
                throw new DomainException("VALIDATION_FAILED", "Only IPv4 CIDR expansion is supported: " + cidr);
            }
            ipNum = ((octets[0] & 0xFFL) << 24)
                    | ((octets[1] & 0xFFL) << 16)
                    | ((octets[2] & 0xFFL) << 8)
                    | (octets[3] & 0xFFL);
        } catch (UnknownHostException e) {
            throw new DomainException("VALIDATION_FAILED", "Invalid IP in CIDR: " + ipPart);
        }

        long mask = prefix == 0 ? 0 : (~0L << (32 - prefix)) & 0xFFFFFFFFL;
        long network = ipNum & mask;
        long broadcast = network | ~mask & 0xFFFFFFFFL;

        List<String> ips = new ArrayList<>();
        if (prefix == 32) {
            ips.add(toIpString(network));
        } else if (prefix == 31) {
            ips.add(toIpString(network));
            ips.add(toIpString(broadcast));
        } else {
            // For /24 to /30, expand host range (skip network and broadcast)
            for (long cur = network + 1; cur < broadcast; cur++) {
                ips.add(toIpString(cur));
            }
        }
        return ips;
    }

    private static String toIpString(long ip) {
        return ((ip >> 24) & 0xFF) + "."
                + ((ip >> 16) & 0xFF) + "."
                + ((ip >> 8) & 0xFF) + "."
                + (ip & 0xFF);
    }
}
