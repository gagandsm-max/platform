package com.platform.core.sensorcontrol.pollgrant;

import org.junit.jupiter.api.Test;
import java.time.Instant;
import java.util.UUID;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrant.*;
import static com.platform.core.sensorcontrol.pollgrant.SensorPollGrantRepository.*;
import static org.junit.jupiter.api.Assertions.*;

class SensorPollGrantTest {
    private static final Instant AT = Instant.parse("2026-10-27T01:02:03.123456Z");

    @Test
    void completeImmutableValueRetainsNullableTerminalFields() {
        UUID id = UUID.randomUUID();
        SensorPollGrant row = new SensorPollGrant(id, id, "sensor", id, id,
                9007199254740991L, 0, Protocol.SNMP, AddressType.IPV6, "2001:db8::1",
                "test-only-plan", "v1", AT, AT, AT.plusSeconds(1), "fixture",
                Status.ISSUED, null, null, null, null, AT);
        assertTrue(SensorPollGrant.class.isRecord());
        assertEquals(22, SensorPollGrant.class.getRecordComponents().length);
        assertEquals(9007199254740991L, row.grantGeneration());
        assertEquals(0, row.assignmentGeneration());
        assertEquals(AT, row.issuedAt());
        assertNull(row.revokedAt());
        assertNull(row.revokedBy());
        assertNull(row.revocationClass());
        assertNull(row.revocationReason());
    }

    @Test
    void terminalRequestAcceptsNullableActorAndReason() {
        Termination terminal = new Termination(Status.SUPERSEDED, AT, null, RevocationClass.SUPERSEDED, null);
        assertEquals(Status.SUPERSEDED, terminal.status());
        assertNull(terminal.revokedBy());
        assertNull(terminal.revocationReason());
    }

    @Test
    void issuedIsNotATerminalRequest() {
        assertThrows(IllegalArgumentException.class,
                () -> new Termination(Status.ISSUED, AT, null, RevocationClass.ROUTINE, null));
    }

    @Test
    void terminalRequestRequiresStatusTimestampAndClass() {
        assertThrows(NullPointerException.class,
                () -> new Termination(null, AT, null, RevocationClass.ROUTINE, null));
        assertThrows(NullPointerException.class,
                () -> new Termination(Status.REVOKED, null, null, RevocationClass.ROUTINE, null));
        assertThrows(NullPointerException.class,
                () -> new Termination(Status.REVOKED, AT, null, null, null));
    }
}
