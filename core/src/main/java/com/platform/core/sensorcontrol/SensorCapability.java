package com.platform.core.sensorcontrol;

import java.util.Collections;
import java.util.EnumMap;
import java.util.EnumSet;
import java.util.Map;
import java.util.Set;

/**
 * Sensor capability tokens and role→capability matrix.
 * Mirrors {@code sensor/internal/runtime/role.go} RoleAllows / roleCapabilities.
 */
public enum SensorCapability {
    ICMP,
    SNMP,
    SSH,
    NETCONF,
    GNMI,
    REST,
    DISCOVERY,
    SNMP_TRAP,
    SYSLOG,
    FLOW,
    INFRA,
    WINRM,
    REDFISH,
    VMWARE,
    KUBERNETES,
    DATABASE,
    AGENT_INGRESS,
    SYNTHETIC,
    CLOUD_API,
    OTEL;

    private static final Map<SensorRole, Set<SensorCapability>> ROLE_CAPABILITIES;

    static {
        Map<SensorRole, Set<SensorCapability>> map = new EnumMap<>(SensorRole.class);
        map.put(SensorRole.NETWORK, setOf(
                ICMP, SNMP, SSH, NETCONF, GNMI, REST, DISCOVERY, SNMP_TRAP, SYSLOG, FLOW
        ));
        map.put(SensorRole.INFRASTRUCTURE, setOf(
                SSH, REST, INFRA, WINRM, REDFISH, VMWARE, KUBERNETES, DATABASE, AGENT_INGRESS, OTEL
        ));
        map.put(SensorRole.STREAMING_EVENT, setOf(SNMP_TRAP, SYSLOG, FLOW, OTEL));
        map.put(SensorRole.SYNTHETIC, setOf(SYNTHETIC));
        map.put(SensorRole.CLOUD_API, setOf(REST, CLOUD_API, KUBERNETES));

        EnumSet<SensorCapability> combined = EnumSet.noneOf(SensorCapability.class);
        for (SensorRole role : new SensorRole[]{
                SensorRole.NETWORK, SensorRole.INFRASTRUCTURE, SensorRole.STREAMING_EVENT,
                SensorRole.SYNTHETIC, SensorRole.CLOUD_API
        }) {
            combined.addAll(map.get(role));
        }
        map.put(SensorRole.COMBINED, Collections.unmodifiableSet(combined));
        ROLE_CAPABILITIES = Collections.unmodifiableMap(map);
    }

    private static Set<SensorCapability> setOf(SensorCapability... values) {
        return Collections.unmodifiableSet(EnumSet.copyOf(Set.of(values)));
    }

    /** Mirrors Go {@code RoleAllows(role, capability)}. */
    public static boolean roleAllows(SensorRole role, SensorCapability capability) {
        if (role == null || capability == null) {
            return false;
        }
        Set<SensorCapability> allowed = ROLE_CAPABILITIES.get(role);
        return allowed != null && allowed.contains(capability);
    }

    public static Set<SensorCapability> capabilitiesForRole(SensorRole role) {
        Set<SensorCapability> allowed = ROLE_CAPABILITIES.get(role);
        return allowed == null ? Set.of() : allowed;
    }
}
