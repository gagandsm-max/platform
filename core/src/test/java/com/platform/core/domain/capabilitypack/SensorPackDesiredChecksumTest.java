package com.platform.core.domain.capabilitypack;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;

class SensorPackDesiredChecksumTest {

    @Test
    void emptyAssignmentsYieldNullChecksum() {
        assertNull(SensorPackDesiredChecksum.compute(List.of()));
    }

    @Test
    void sortingByPackIdIsDeterministicAndOrderIndependent() {
        var a = new SensorPackDesiredChecksum.AssignmentDigest("fixture.a", "1.0.0", "a".repeat(64));
        var b = new SensorPackDesiredChecksum.AssignmentDigest("fixture.b", "2.0.0", "b".repeat(64));
        String forward = SensorPackDesiredChecksum.compute(List.of(a, b));
        String reverse = SensorPackDesiredChecksum.compute(List.of(b, a));
        assertEquals(forward, reverse);
        assertEquals(64, forward.length());
    }

    @Test
    void lengthPrefixPreventsDelimiterAmbiguity() {
        var ambiguous = new SensorPackDesiredChecksum.AssignmentDigest("ab", "c", "d".repeat(64));
        var split = new SensorPackDesiredChecksum.AssignmentDigest("a", "bc", "d".repeat(64));
        assertNotEquals(
                SensorPackDesiredChecksum.compute(List.of(ambiguous)),
                SensorPackDesiredChecksum.compute(List.of(split)));
    }

    @Test
    void versionChangeAltersChecksum() {
        var v1 = new SensorPackDesiredChecksum.AssignmentDigest("fixture.pack", "1.0.0", "a".repeat(64));
        var v2 = new SensorPackDesiredChecksum.AssignmentDigest("fixture.pack", "1.0.1", "a".repeat(64));
        assertNotEquals(
                SensorPackDesiredChecksum.compute(List.of(v1)),
                SensorPackDesiredChecksum.compute(List.of(v2)));
    }
}
