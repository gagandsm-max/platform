package com.platform.core.agentcontrol;

import com.platform.core.sensorcontrol.SensorLifecycleStatus;
import com.platform.core.sensorcontrol.SensorPrincipal;
import com.platform.core.sensorcontrol.SensorRole;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class AgentControlSiteCompatibilityTest {

    private static final Instant NOW = Instant.parse("2026-09-10T06:00:00Z");
    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private static final UUID SITE_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID SITE_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a2");

    private AgentPrincipal agent(UUID siteId) {
        return new AgentPrincipal("agent-01", TENANT, siteId, AgentLifecycleStatus.ACTIVE, NOW, NOW);
    }

    private SensorPrincipal sensor(UUID siteId) {
        return new SensorPrincipal(
                "sensor-infra-01", TENANT, siteId, SensorRole.INFRASTRUCTURE,
                SensorLifecycleStatus.ACTIVE, true, NOW, NOW);
    }

    @Test
    void bothSitesEqualAllowed() {
        assertDoesNotThrow(() -> AgentControlService.assertSiteCompatible(agent(SITE_A), sensor(SITE_A)));
    }

    @Test
    void bothSitesDifferentRejected() {
        AgentControlException ex = assertThrows(AgentControlException.class,
                () -> AgentControlService.assertSiteCompatible(agent(SITE_A), sensor(SITE_B)));
        assertEquals(AgentControlException.ERR_AGENT_SITE_INCOMPATIBLE, ex.getCode());
    }

    @Test
    void agentSiteNullAllowed() {
        assertDoesNotThrow(() -> AgentControlService.assertSiteCompatible(agent(null), sensor(SITE_A)));
    }

    @Test
    void sensorSiteNullAllowed() {
        assertDoesNotThrow(() -> AgentControlService.assertSiteCompatible(agent(SITE_A), sensor(null)));
    }

    @Test
    void bothNullAllowed() {
        assertDoesNotThrow(() -> AgentControlService.assertSiteCompatible(agent(null), sensor(null)));
    }
}
