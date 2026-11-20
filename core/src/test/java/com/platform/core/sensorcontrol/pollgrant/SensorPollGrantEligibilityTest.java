package com.platform.core.sensorcontrol.pollgrant;

import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.UUID;

import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.AddressType;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Protocol;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.Status;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrantEligibility.Decision;
import static org.junit.jupiter.api.Assertions.*;

class SensorPollGrantEligibilityTest {

    private static final Instant AT = Instant.parse("2026-11-20T12:00:00Z");

    @Test
    void eligibleRequiresIssuedSnapshotAndEvaluationTime() {
        UUID id = UUID.randomUUID();
        SensorPollGrant issued = new SensorPollGrant(
                id, id, "sensor", id, id, 1L, 0L, Protocol.ICMP, AddressType.IPV4, "192.0.2.1",
                "plan", "v1", AT, AT.minusSeconds(1), AT.plusSeconds(1), "fixture",
                Status.ISSUED, null, null, null, null, AT);
        SensorPollGrantEligibility outcome = SensorPollGrantEligibility.eligible(issued, AT);
        assertTrue(outcome.eligible());
        assertEquals(Decision.ELIGIBLE, outcome.decision());
        assertSame(issued, outcome.grant());
        assertEquals(AT, outcome.evaluatedAt());
    }

    @Test
    void deniedOutcomesCarryNoGrantSnapshot() {
        SensorPollGrantEligibility denied = SensorPollGrantEligibility.denied(Decision.NOT_VISIBLE);
        assertFalse(denied.eligible());
        assertNull(denied.grant());
        assertNull(denied.evaluatedAt());
        assertThrows(IllegalArgumentException.class,
                () -> SensorPollGrantEligibility.denied(Decision.ELIGIBLE));
    }
}
