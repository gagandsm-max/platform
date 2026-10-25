package com.platform.core.ingestion.icmp;

/**
 * First-class ICMP evidence quality from Sensor {@code observation.Envelope}.
 * Distinct from numeric {@code qualityCode} collapse used by SNMP ingestion.
 * Never invents DEVICE_DOWN.
 */
public enum IcmpQuality {
    GOOD,
    PARTIAL,
    UNKNOWN;

    public static IcmpQuality parseRequired(String raw) {
        if (raw == null || raw.isBlank()) {
            throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_QUALITY,
                    "ICMP quality is required and must be GOOD, PARTIAL, or UNKNOWN"
            );
        }
        String normalized = raw.trim().toUpperCase();
        return switch (normalized) {
            case "GOOD" -> GOOD;
            case "PARTIAL" -> PARTIAL;
            case "UNKNOWN" -> UNKNOWN;
            default -> throw new IcmpDecodeException(
                    IcmpDecodeException.Code.ERR_ICMP_INVALID_QUALITY,
                    "Unsupported ICMP quality '" + raw + "'; only GOOD, PARTIAL, UNKNOWN are permitted"
            );
        };
    }
}
