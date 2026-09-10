package com.platform.core.sensorcontrol;

import org.junit.jupiter.api.Test;

import java.util.EnumSet;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Parity with sensor/internal/runtime/role.go RoleAllows / roleCapabilities.
 */
class SensorCapabilityRoleMatrixTest {

    @Test
    void networkAllowsDiscoveryButNotAgentIngress() {
        assertTrue(SensorCapability.roleAllows(SensorRole.NETWORK, SensorCapability.DISCOVERY));
        assertTrue(SensorCapability.roleAllows(SensorRole.NETWORK, SensorCapability.SNMP));
        assertFalse(SensorCapability.roleAllows(SensorRole.NETWORK, SensorCapability.AGENT_INGRESS));
    }

    @Test
    void infrastructureAllowsAgentIngressButNotDiscovery() {
        assertTrue(SensorCapability.roleAllows(SensorRole.INFRASTRUCTURE, SensorCapability.AGENT_INGRESS));
        assertFalse(SensorCapability.roleAllows(SensorRole.INFRASTRUCTURE, SensorCapability.DISCOVERY));
    }

    @Test
    void combinedIsUnionOfBaseRoles() {
        Set<SensorCapability> combined = SensorCapability.capabilitiesForRole(SensorRole.COMBINED);
        assertTrue(combined.contains(SensorCapability.DISCOVERY));
        assertTrue(combined.contains(SensorCapability.AGENT_INGRESS));
        assertTrue(combined.contains(SensorCapability.SYNTHETIC));
        assertTrue(combined.contains(SensorCapability.CLOUD_API));
        assertEquals(EnumSet.allOf(SensorCapability.class), EnumSet.copyOf(combined));
    }

    @Test
    void streamingSyntheticCloudMatricesMatchGo() {
        assertTrue(SensorCapability.roleAllows(SensorRole.STREAMING_EVENT, SensorCapability.SYSLOG));
        assertFalse(SensorCapability.roleAllows(SensorRole.STREAMING_EVENT, SensorCapability.DISCOVERY));
        assertTrue(SensorCapability.roleAllows(SensorRole.SYNTHETIC, SensorCapability.SYNTHETIC));
        assertFalse(SensorCapability.roleAllows(SensorRole.SYNTHETIC, SensorCapability.SNMP));
        assertTrue(SensorCapability.roleAllows(SensorRole.CLOUD_API, SensorCapability.KUBERNETES));
        assertFalse(SensorCapability.roleAllows(SensorRole.CLOUD_API, SensorCapability.AGENT_INGRESS));
    }

    @Test
    void nullRoleOrCapabilityFailsClosed() {
        assertFalse(SensorCapability.roleAllows(null, SensorCapability.DISCOVERY));
        assertFalse(SensorCapability.roleAllows(SensorRole.NETWORK, null));
    }

    @Test
    void roleEnumValuesMatchGoConstants() {
        assertEquals(
                Set.of("NETWORK", "INFRASTRUCTURE", "STREAMING_EVENT", "SYNTHETIC", "CLOUD_API", "COMBINED"),
                EnumSet.allOf(SensorRole.class).stream().map(Enum::name).collect(java.util.stream.Collectors.toSet())
        );
    }
}
