package com.platform.core.domain.inventory;

import com.platform.core.domain.DomainException;

import java.util.Locale;
import java.util.regex.Pattern;

/**
 * Host-identity validation for management endpoints. No DNS lookup. Rejects URL/userinfo/path/query.
 */
public final class ManagementEndpointAddressValidator {

    private static final int MAX_ADDRESS_LEN = 255;
    // Allow 1–3 digit octets (including leading zeros); value range checked after parse.
    private static final Pattern IPV4 = Pattern.compile("^(\\d{1,3})\\.(\\d{1,3})\\.(\\d{1,3})\\.(\\d{1,3})$");
    private static final Pattern FQDN_LABEL = Pattern.compile("^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$");

    private ManagementEndpointAddressValidator() {
    }

    public record NormalizedAddress(ManagementEndpointAddressType type, String address) {
    }

    public static NormalizedAddress normalizeAndValidate(ManagementEndpointAddressType type, String raw) {
        if (type == null) {
            throw new DomainException("VALIDATION_FAILED", "addressType is required");
        }
        if (raw == null || raw.isBlank()) {
            throw new DomainException("VALIDATION_FAILED", "address is required");
        }
        String trimmed = raw.trim();
        if (trimmed.length() > MAX_ADDRESS_LEN) {
            throw new DomainException("VALIDATION_FAILED", "address exceeds maximum length");
        }
        rejectUrlPivotSemantics(trimmed);

        return switch (type) {
            case IPV4 -> new NormalizedAddress(type, normalizeIpv4(trimmed));
            case IPV6 -> new NormalizedAddress(type, normalizeIpv6(trimmed));
            case FQDN -> new NormalizedAddress(type, normalizeFqdn(trimmed));
        };
    }

    private static void rejectUrlPivotSemantics(String value) {
        String lower = value.toLowerCase(Locale.ROOT);
        if (lower.startsWith("http://") || lower.startsWith("https://")
                || lower.contains("://")
                || value.contains("@")
                || value.contains("/")
                || value.contains("?")
                || value.contains("#")) {
            throw new DomainException(
                    "VALIDATION_FAILED",
                    "address must be a host identity (IPv4/IPv6/FQDN), not a URL or path");
        }
    }

    private static String normalizeIpv4(String value) {
        var matcher = IPV4.matcher(value);
        if (!matcher.matches()) {
            throw new DomainException("VALIDATION_FAILED", "malformed IPv4 address");
        }
        // Strip leading zeros per octet; reject out-of-range octets.
        StringBuilder out = new StringBuilder(15);
        for (int i = 1; i <= 4; i++) {
            int octet = Integer.parseInt(matcher.group(i));
            if (octet > 255) {
                throw new DomainException("VALIDATION_FAILED", "malformed IPv4 address");
            }
            if (i > 1) {
                out.append('.');
            }
            out.append(octet);
        }
        return out.toString();
    }

    private static String normalizeIpv6(String value) {
        String candidate = value;
        if (candidate.startsWith("[") && candidate.endsWith("]")) {
            candidate = candidate.substring(1, candidate.length() - 1);
        }
        if (candidate.isBlank() || !candidate.contains(":")) {
            throw new DomainException("VALIDATION_FAILED", "malformed IPv6 address");
        }
        // Reject IPv4-mapped mixed forms with URL/path already handled; reject zone ids.
        if (candidate.contains("%")) {
            throw new DomainException("VALIDATION_FAILED", "malformed IPv6 address");
        }
        try {
            // Literal-only: java.net parses IPv6 literals without DNS when the string is a literal.
            byte[] bytes = java.net.InetAddress.getByName(candidate).getAddress();
            if (bytes.length != 16) {
                throw new DomainException("VALIDATION_FAILED", "malformed IPv6 address");
            }
            return java.net.InetAddress.getByAddress(bytes).getHostAddress().toLowerCase(Locale.ROOT);
        } catch (DomainException ex) {
            throw ex;
        } catch (Exception ex) {
            throw new DomainException("VALIDATION_FAILED", "malformed IPv6 address");
        }
    }

    private static String normalizeFqdn(String value) {
        String fqdn = value;
        if (fqdn.endsWith(".")) {
            fqdn = fqdn.substring(0, fqdn.length() - 1);
        }
        fqdn = fqdn.toLowerCase(Locale.ROOT);
        if (fqdn.isBlank() || fqdn.length() > 253) {
            throw new DomainException("VALIDATION_FAILED", "malformed FQDN");
        }
        if (fqdn.contains("..") || fqdn.startsWith("-") || fqdn.endsWith("-")) {
            throw new DomainException("VALIDATION_FAILED", "malformed FQDN");
        }
        // Must not look like an IP literal pretending to be FQDN without type.
        if (IPV4.matcher(fqdn).matches() || fqdn.contains(":")) {
            throw new DomainException("VALIDATION_FAILED", "malformed FQDN");
        }
        String[] labels = fqdn.split("\\.");
        // Canonical management authority requires a real multi-label FQDN (not search-domain dependent).
        if (labels.length < 2) {
            throw new DomainException("VALIDATION_FAILED", "malformed FQDN");
        }
        for (String label : labels) {
            if (!FQDN_LABEL.matcher(label).matches()) {
                throw new DomainException("VALIDATION_FAILED", "malformed FQDN");
            }
        }
        return fqdn;
    }
}
