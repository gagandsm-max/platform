package com.platform.core.domain.recurring;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;

class SensorRecurringDesiredChecksumTest {

    private static final String TENANT = "018f0e0a-0000-7000-8000-0000000000a1";

    @Test
    void emptyAssignmentsYieldNullChecksum() {
        assertNull(SensorRecurringDesiredChecksum.compute(List.of()));
    }

    @Test
    void sortingByAssignmentIdIsDeterministicAndOrderIndependent() {
        var a = digest("assign-a", "entity-1");
        var b = digest("assign-b", "entity-2");
        String forward = SensorRecurringDesiredChecksum.compute(List.of(a, b));
        String reverse = SensorRecurringDesiredChecksum.compute(List.of(b, a));
        assertEquals(forward, reverse);
        assertEquals(64, forward.length());
    }

    @Test
    void lengthPrefixPreventsDelimiterAmbiguity() {
        var ambiguous = digest("ab", "c");
        var split = digest("a", "bc");
        assertNotEquals(
                SensorRecurringDesiredChecksum.compute(List.of(ambiguous)),
                SensorRecurringDesiredChecksum.compute(List.of(split)));
    }

    @Test
    void cadenceChangeAltersChecksum() {
        var v1 = digest("assign-1", "entity-1", 60_000L);
        var v2 = digest("assign-1", "entity-1", 120_000L);
        assertNotEquals(
                SensorRecurringDesiredChecksum.compute(List.of(v1)),
                SensorRecurringDesiredChecksum.compute(List.of(v2)));
    }

    @Test
    void oidOrderIsMaterialToChecksum() {
        SnmpParams.SnmpLimits limits = new SnmpParams.SnmpLimits(16, 100, 10, 100, 65_536);
        var forward = baseDigest(
                new SnmpParams("127.0.0.1", 161, "2c", "GET",
                        List.of(".1.3.6.1.2.1.1.1.0", ".1.3.6.1.2.1.1.3.0"), 1_000L, limits));
        var reverse = baseDigest(
                new SnmpParams("127.0.0.1", 161, "2c", "GET",
                        List.of(".1.3.6.1.2.1.1.3.0", ".1.3.6.1.2.1.1.1.0"), 1_000L, limits));
        assertNotEquals(
                SensorRecurringDesiredChecksum.compute(List.of(forward)),
                SensorRecurringDesiredChecksum.compute(List.of(reverse)));
    }

    /** Must match Go recurring.TestGoldenChecksumParity. */
    private static final String GOLDEN_CHECKSUM_SLICE_A =
            "d246391352ee9131e00ee07bdd2cf1b81a7c1246a9e5ee4a4b578bc655fa4235";

    @Test
    void goldenChecksumMatchesGoSensorVector() {
        SnmpParams.SnmpLimits limits = new SnmpParams.SnmpLimits(16, 100, 10, 100, 65_536);
        var digest = new SensorRecurringDesiredChecksum.AssignmentDigest(
                "assign-golden-1",
                TENANT,
                "entity-golden",
                "mon-golden",
                "OPERATIONAL_DETAIL",
                "NETWORK_POLL",
                300_000L,
                600_000L,
                1,
                30_000L,
                "cred-golden-1",
                "SNMP",
                new SnmpParams("192.0.2.50", 161, "2c", "GET",
                        List.of(".1.3.6.1.2.1.1.3.0"), 2_000L, limits)
        );
        assertEquals(GOLDEN_CHECKSUM_SLICE_A, SensorRecurringDesiredChecksum.compute(List.of(digest)));
    }

    private static SensorRecurringDesiredChecksum.AssignmentDigest digest(String assignmentId, String entityId) {
        return digest(assignmentId, entityId, 60_000L);
    }

    private static SensorRecurringDesiredChecksum.AssignmentDigest digest(
            String assignmentId, String entityId, long cadenceMs) {
        SnmpParams.SnmpLimits limits = new SnmpParams.SnmpLimits(16, 100, 10, 100, 65_536);
        return new SensorRecurringDesiredChecksum.AssignmentDigest(
                assignmentId,
                TENANT,
                entityId,
                "mon-1",
                "LIVENESS",
                "NETWORK_POLL",
                cadenceMs,
                90_000L,
                1,
                5_000L,
                "cred-profile-1",
                "SNMP",
                new SnmpParams("127.0.0.1", 161, "2c", "GET", List.of(".1.3.6.1.2.1.1.3.0"), 1_000L, limits)
        );
    }

    private static SensorRecurringDesiredChecksum.AssignmentDigest baseDigest(SnmpParams snmp) {
        return new SensorRecurringDesiredChecksum.AssignmentDigest(
                "assign-1",
                TENANT,
                "entity-1",
                "mon-1",
                "LIVENESS",
                "NETWORK_POLL",
                60_000L,
                90_000L,
                1,
                5_000L,
                "cred-profile-1",
                "SNMP",
                snmp
        );
    }
}
