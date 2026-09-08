package com.platform.core.ingestion;

import com.platform.core.ingestion.dto.TelemetryAckResponse;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;

import static org.junit.jupiter.api.Assertions.*;

class IdempotencyTrackerTest {

    private IdempotencyTracker tracker;

    @BeforeEach
    void setUp() {
        tracker = new IdempotencyTracker();
    }

    @Test
    void testNewKeyReturnsNull() {
        assertNull(tracker.checkDuplicate("key-1", "checksum-abc"));
    }

    @Test
    void testDuplicateReplayReturnsCachedAck() {
        TelemetryAckResponse ack = new TelemetryAckResponse("ACCEPTED", 42L, "sensor-net-01");
        tracker.recordSuccess("key-1", "sensor-net-01", 42L, "checksum-abc", ack);

        TelemetryAckResponse cached = tracker.checkDuplicate("key-1", "checksum-abc");
        assertNotNull(cached);
        assertEquals("ACCEPTED", cached.status());
        assertEquals(42L, cached.acceptedThroughSequence());
        assertEquals("sensor-net-01", cached.sensorIdentity());
    }

    @Test
    void testChecksumConflictThrowsException() {
        TelemetryAckResponse ack = new TelemetryAckResponse("ACCEPTED", 42L, "sensor-net-01");
        tracker.recordSuccess("key-1", "sensor-net-01", 42L, "checksum-abc", ack);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                tracker.checkDuplicate("key-1", "checksum-different"));
        assertEquals(HttpStatus.CONFLICT, ex.getStatus());
        assertEquals("ERR_IDEMPOTENCY_CONFLICT", ex.getCode());
    }

    @Test
    void testCacheBoundedEviction() {
        for (int i = 0; i < 10_005; i++) {
            TelemetryAckResponse ack = new TelemetryAckResponse("ACCEPTED", (long) i, "sensor-net-01");
            tracker.recordSuccess("key-" + i, "sensor-net-01", (long) i, "checksum-" + i, ack);
        }
        assertTrue(tracker.size() <= 10_000);
        // Older entries should have been evicted
        assertNull(tracker.checkDuplicate("key-0", "checksum-0"));
        // Recent entry exists
        assertNotNull(tracker.checkDuplicate("key-10004", "checksum-10004"));
    }
}
