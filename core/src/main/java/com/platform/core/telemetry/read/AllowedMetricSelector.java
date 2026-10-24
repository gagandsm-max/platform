package com.platform.core.telemetry.read;

import java.util.List;
import java.util.Locale;
import java.util.Objects;
import java.util.Optional;
import java.util.regex.Pattern;

/**
 * Phase-1 allowlist for per-entity metric observation reads.
 * Bindings are exact persisted producer keys (Agent) or approved SNMP numeric OID
 * families with full-arc + bounded instance validation. Free-form aliases are rejected.
 */
public final class AllowedMetricSelector {

    public static final String AGENT_CPU_UTILIZATION = "system.cpu.utilization";
    public static final String AGENT_MEMORY_UTILIZATION = "system.memory.utilization";

    /** SNMP ifIndex / instance arc: positive Integer32 upper bound. */
    public static final long MAX_SNMP_INSTANCE = 2_147_483_647L;

    private static final Pattern DECIMAL_INSTANCE = Pattern.compile("^[1-9][0-9]*$");

    private static final List<SnmpOidFamily> SNMP_FAMILIES = List.of(
            // IF-MIB ifOperStatus
            new SnmpOidFamily(".1.3.6.1.2.1.2.2.1.8", MetricValueKind.NUMERIC),
            // IF-MIB ifHCInOctets
            new SnmpOidFamily(".1.3.6.1.2.1.31.1.1.1.6", MetricValueKind.COUNTER64),
            // IF-MIB ifHCOutOctets
            new SnmpOidFamily(".1.3.6.1.2.1.31.1.1.1.10", MetricValueKind.COUNTER64)
    );

    private final String metricId;
    private final MetricValueKind valueKind;

    private AllowedMetricSelector(String metricId, MetricValueKind valueKind) {
        this.metricId = Objects.requireNonNull(metricId, "metricId");
        this.valueKind = Objects.requireNonNull(valueKind, "valueKind");
    }

    public String metricId() {
        return metricId;
    }

    public MetricValueKind valueKind() {
        return valueKind;
    }

    public static Optional<AllowedMetricSelector> find(String metricId, MetricValueKind valueKind) {
        if (metricId == null || valueKind == null) {
            return Optional.empty();
        }
        String trimmed = metricId.trim();
        if (trimmed.isEmpty()) {
            return Optional.empty();
        }

        if (AGENT_CPU_UTILIZATION.equals(trimmed) && valueKind == MetricValueKind.NUMERIC) {
            return Optional.of(new AllowedMetricSelector(AGENT_CPU_UTILIZATION, MetricValueKind.NUMERIC));
        }
        if (AGENT_MEMORY_UTILIZATION.equals(trimmed) && valueKind == MetricValueKind.NUMERIC) {
            return Optional.of(new AllowedMetricSelector(AGENT_MEMORY_UTILIZATION, MetricValueKind.NUMERIC));
        }

        // SNMP metric ids are case-sensitive numeric OID arcs as persisted by Core ingestion.
        // Do not lowercase — OID identity must match the stored key exactly.
        for (SnmpOidFamily family : SNMP_FAMILIES) {
            Optional<AllowedMetricSelector> matched = family.match(trimmed, valueKind);
            if (matched.isPresent()) {
                return matched;
            }
        }

        // Reject known human aliases / lookalike free-form keys fail-closed.
        String lower = trimmed.toLowerCase(Locale.ROOT);
        if (lower.equals("cpu.util")
                || lower.equals("host.mem.used_pct")
                || lower.equals("if.oper_status")
                || lower.equals("if.hc_in_octets")
                || lower.equals("if.hc_out_octets")
                || lower.equals("ifhcincoctets")
                || lower.equals("ifhcoutoctets")
                || lower.equals("ifoperstatus")) {
            return Optional.empty();
        }
        return Optional.empty();
    }

    @Override
    public boolean equals(Object other) {
        if (this == other) {
            return true;
        }
        if (!(other instanceof AllowedMetricSelector that)) {
            return false;
        }
        return metricId.equals(that.metricId) && valueKind == that.valueKind;
    }

    @Override
    public int hashCode() {
        return Objects.hash(metricId, valueKind);
    }

    @Override
    public String toString() {
        return "AllowedMetricSelector[metricId=" + metricId + ", valueKind=" + valueKind + "]";
    }

    /**
     * Approved SNMP object family. Matching requires the exact object arc plus a single
     * bounded decimal instance suffix. Prefix lookalikes (e.g. ...1.8 vs ...1.81) are rejected
     * because the instance remainder must be pure decimal with no extra dots.
     */
    private record SnmpOidFamily(String objectArc, MetricValueKind valueKind) {
        Optional<AllowedMetricSelector> match(String metricId, MetricValueKind requestedKind) {
            if (requestedKind != valueKind) {
                return Optional.empty();
            }
            String prefix = objectArc + ".";
            if (!metricId.startsWith(prefix)) {
                return Optional.empty();
            }
            String instance = metricId.substring(prefix.length());
            if (!DECIMAL_INSTANCE.matcher(instance).matches()) {
                return Optional.empty();
            }
            // Guard against absurdly long digit strings before BigInteger/long parse.
            if (instance.length() > 10) {
                return Optional.empty();
            }
            long value;
            try {
                value = Long.parseLong(instance);
            } catch (NumberFormatException ex) {
                return Optional.empty();
            }
            if (value < 1L || value > MAX_SNMP_INSTANCE) {
                return Optional.empty();
            }
            // Preserve the exact caller/persisted metric identity (including leading dot).
            return Optional.of(new AllowedMetricSelector(metricId, valueKind));
        }
    }
}
