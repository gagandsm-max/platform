package com.platform.core.telemetry.read;

import java.util.Locale;
import java.util.Map;
import java.util.Optional;

/**
 * Static Phase-1 allowlist for per-entity metric observation reads.
 * Free-form metric identifiers are rejected fail-closed.
 */
public enum AllowedMetricSelector {
    CPU_UTIL("cpu.util", MetricValueKind.NUMERIC),
    HOST_MEM_USED_PCT("host.mem.used_pct", MetricValueKind.NUMERIC),
    IF_OPER_STATUS("if.oper_status", MetricValueKind.NUMERIC),
    IF_HC_IN_OCTETS("if.hc_in_octets", MetricValueKind.COUNTER64),
    IF_HC_OUT_OCTETS("if.hc_out_octets", MetricValueKind.COUNTER64);

    private static final Map<String, AllowedMetricSelector> BY_ID;

    static {
        BY_ID = Map.of(
                CPU_UTIL.metricId, CPU_UTIL,
                HOST_MEM_USED_PCT.metricId, HOST_MEM_USED_PCT,
                IF_OPER_STATUS.metricId, IF_OPER_STATUS,
                IF_HC_IN_OCTETS.metricId, IF_HC_IN_OCTETS,
                IF_HC_OUT_OCTETS.metricId, IF_HC_OUT_OCTETS
        );
    }

    private final String metricId;
    private final MetricValueKind valueKind;

    AllowedMetricSelector(String metricId, MetricValueKind valueKind) {
        this.metricId = metricId;
        this.valueKind = valueKind;
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
        AllowedMetricSelector selector = BY_ID.get(metricId.trim().toLowerCase(Locale.ROOT));
        if (selector == null || selector.valueKind != valueKind) {
            return Optional.empty();
        }
        return Optional.of(selector);
    }
}
