package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import com.platform.core.ingestion.dto.CanonicalCounterObservation;
import org.junit.jupiter.api.Test;

import java.math.BigInteger;
import java.time.Instant;

import static org.junit.jupiter.api.Assertions.*;

class CanonicalCounterObservationTest {

    private final ObjectMapper objectMapper = new ObjectMapper().registerModule(new JavaTimeModule());

    @Test
    void kafkaJsonRoundTripPreservesUint64MaxAsQuotedDecimal() throws Exception {
        CanonicalCounterObservation original = new CanonicalCounterObservation(
                "tenant", "entity", "ifHCInOctets", "sensor-1",
                Instant.parse("2026-09-21T00:00:00Z"),
                new BigInteger("18446744073709551615"),
                1, 0, "seq-1", "snmp"
        );
        String json = objectMapper.writeValueAsString(original);
        assertTrue(json.contains("\"counterValue\":\"18446744073709551615\""), json);
        assertFalse(json.contains("1.8446744073709552E19"), json);
        assertFalse(json.contains("\"counterValue\":18446744073709551615"), json);

        CanonicalCounterObservation roundTrip = objectMapper.readValue(json, CanonicalCounterObservation.class);
        assertEquals("18446744073709551615", roundTrip.counterValue());
        assertEquals(new BigInteger("18446744073709551615"), roundTrip.exactValue());
        assertEquals(CanonicalCounterObservation.KIND, roundTrip.observationKind());
    }

    @Test
    void rejectsOutOfRangeAndNonDecimal() {
        assertThrows(IllegalArgumentException.class,
                () -> CanonicalCounterObservation.parseExactUnsigned64("-1"));
        assertThrows(IllegalArgumentException.class,
                () -> CanonicalCounterObservation.parseExactUnsigned64("18446744073709551616"));
        assertThrows(IllegalArgumentException.class,
                () -> CanonicalCounterObservation.parseExactUnsigned64("+1"));
        assertThrows(IllegalArgumentException.class,
                () -> CanonicalCounterObservation.parseExactUnsigned64(" 1"));
        assertThrows(IllegalArgumentException.class,
                () -> CanonicalCounterObservation.parseExactUnsigned64("1 "));
        assertThrows(IllegalArgumentException.class,
                () -> CanonicalCounterObservation.parseExactUnsigned64("1.5"));
        assertThrows(IllegalArgumentException.class,
                () -> CanonicalCounterObservation.parseExactUnsigned64("1e10"));
    }
}
