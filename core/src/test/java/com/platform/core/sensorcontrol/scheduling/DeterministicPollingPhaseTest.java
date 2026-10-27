package com.platform.core.sensorcontrol.scheduling;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;
import org.junit.jupiter.params.provider.ValueSource;

import java.io.InputStream;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.Arrays;
import java.util.HexFormat;
import java.util.List;
import java.util.Objects;
import java.util.UUID;
import java.util.stream.Stream;
import java.util.stream.StreamSupport;

import static org.junit.jupiter.api.Assertions.*;

class DeterministicPollingPhaseTest {
    private static final UUID TENANT = UUID.fromString("00010203-0405-0607-0809-0a0b0c0d0e0f");
    private static final UUID ENTITY = UUID.fromString("10111213-1415-1617-1819-1a1b1c1d1e1f");
    private static final String RESOURCE = "/scheduling/polling-phase-v1-golden-vectors.json";

    static Stream<Arguments> goldenVectors() throws Exception {
        try (InputStream input = Objects.requireNonNull(DeterministicPollingPhaseTest.class.getResourceAsStream(RESOURCE))) {
            JsonNode root = new ObjectMapper().readTree(input);
            assertEquals(1, root.get("algorithmVersion").asInt());
            return StreamSupport.stream(root.get("vectors").spliterator(), false)
                    .map(v -> Arguments.of(v.get("name").asText(), v)).toList().stream();
        }
    }

    @ParameterizedTest(name = "{0}")
    @MethodSource("goldenVectors")
    void independentlyDerivedEncodingHashSeedAndPhase(String name, JsonNode vector) throws Exception {
        UUID tenant = UUID.fromString(vector.get("tenant").asText());
        UUID entity = UUID.fromString(vector.get("entity").asText());
        String key = vector.get("collectionKey").asText();
        byte[] encoded = DeterministicPollingPhase.encodeIdentity(tenant, entity, key, 1);
        assertEquals(vector.get("encodingHex").asText(), HexFormat.of().formatHex(encoded));
        assertEquals(vector.get("sha256Hex").asText(),
                HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(encoded)));
        long seed = DeterministicPollingPhase.seed(tenant, entity, key, 1);
        assertEquals(vector.get("seedHex").asText(), HexFormat.of().toHexDigits(seed));
        assertEquals(vector.get("seedUnsignedDecimal").asText(), Long.toUnsignedString(seed));
        assertEquals(vector.get("phaseMillis").asLong(), DeterministicPollingPhase.phaseMillis(
                tenant, entity, key, 1, vector.get("cadenceMillis").asLong()));
    }

    @Test
    void highBitSeedUsesUnsignedRemainder() {
        long seed = DeterministicPollingPhase.seed(TENANT, ENTITY, "core.collection.cpu", 1);
        assertTrue(seed < 0);
        assertEquals("12061834632549209665", Long.toUnsignedString(seed));
        assertEquals(209665, DeterministicPollingPhase.phaseMillis(TENANT, ENTITY, "core.collection.cpu", 1, 300000));
    }

    @Test
    void reconstructionRetainsExactSeedAndPhase() {
        long expected = DeterministicPollingPhase.seed(TENANT, ENTITY, "core.collection.cpu", 1);
        UUID restoredTenant = UUID.fromString(TENANT.toString());
        UUID restoredEntity = UUID.fromString(ENTITY.toString());
        String restoredKey = new String("core.collection.cpu".getBytes(StandardCharsets.UTF_8), StandardCharsets.UTF_8);
        assertEquals(expected, DeterministicPollingPhase.seed(restoredTenant, restoredEntity, restoredKey, 1));
        assertEquals(209665, DeterministicPollingPhase.phaseMillis(restoredTenant, restoredEntity, restoredKey, 1, 300000));
    }

    @Test
    void lengthIsUtf8BytesAndEmbeddedZeroRemainsUnambiguous() {
        byte[] encoded = DeterministicPollingPhase.encodeIdentity(TENANT, ENTITY, "é😀", 1);
        int lengthOffset = "platform.poll.phase.v1".length() + 1 + 16 + 16;
        assertEquals(6, ByteBuffer.wrap(encoded, lengthOffset, 4).getInt());
        assertArrayEquals("é😀".getBytes(StandardCharsets.UTF_8), Arrays.copyOfRange(encoded, lengthOffset + 4, encoded.length));
        List<String> keys = List.of("a", "a\0", "ab", "a\0b", "a\0b\0");
        // Encoding must be injective; phases are explicitly allowed to collide.
        assertEquals(keys.size(), keys.stream().map(k -> HexFormat.of().formatHex(
                DeterministicPollingPhase.encodeIdentity(TENANT, ENTITY, k, 1))).distinct().count());
    }

    @Test
    void preservesCaseSpacesAndUnicodeNormalizationForm() {
        List<String> keys = List.of("a", "A", " a ", "é", "e\u0301");
        assertEquals(keys.size(), keys.stream().map(k -> HexFormat.of().formatHex(
                DeterministicPollingPhase.encodeIdentity(TENANT, ENTITY, k, 1))).distinct().count());
        assertFalse(Arrays.equals(DeterministicPollingPhase.encodeIdentity(TENANT, ENTITY, "a", 1),
                DeterministicPollingPhase.encodeIdentity(ENTITY, TENANT, "a", 1)));
    }

    @Test
    void returnedEncodingCannotChangeLaterCalculations() {
        byte[] encoded = DeterministicPollingPhase.encodeIdentity(TENANT, ENTITY, "core.collection.cpu", 1);
        Arrays.fill(encoded, (byte) 0);
        assertEquals("a7643eb124999641", HexFormat.of().toHexDigits(
                DeterministicPollingPhase.seed(TENANT, ENTITY, "core.collection.cpu", 1)));
    }

    @ParameterizedTest
    @ValueSource(longs = {1, 2, 3, 300000, Long.MAX_VALUE})
    void explicitPositiveCadencesProduceInRangeOffsets(long cadence) {
        long phase = DeterministicPollingPhase.phaseMillis(TENANT, ENTITY, "core.collection.cpu", 1, cadence);
        assertTrue(phase >= 0 && phase < cadence);
        if (cadence == 1) assertEquals(0, phase);
    }

    @ParameterizedTest
    @ValueSource(longs = {0, -1, Long.MIN_VALUE})
    void rejectsNonpositiveCadence(long cadence) {
        assertThrows(IllegalArgumentException.class,
                () -> DeterministicPollingPhase.phaseMillis(TENANT, ENTITY, "key", 1, cadence));
    }

    @ParameterizedTest
    @ValueSource(ints = {0, -1, 2, Integer.MIN_VALUE, Integer.MAX_VALUE})
    void rejectsUnknownVersion(int version) {
        assertThrows(IllegalArgumentException.class,
                () -> DeterministicPollingPhase.seed(TENANT, ENTITY, "key", version));
        assertThrows(IllegalArgumentException.class,
                () -> DeterministicPollingPhase.phaseMillis(TENANT, ENTITY, "key", version, 1));
    }

    @ParameterizedTest
    @ValueSource(strings = {"", " ", "\t\r\n", "\u2003\u3000"})
    void rejectsBlankIdentity(String key) {
        assertThrows(IllegalArgumentException.class, () -> DeterministicPollingPhase.seed(TENANT, ENTITY, key, 1));
    }

    @ParameterizedTest(name = "malformed Unicode case {index}")
    @ValueSource(strings = {"\uD800", "\uDC00", "x\uD800y", "\uDC00\uD800"})
    void rejectsMalformedUnicodeInsteadOfReplacingIt(String key) {
        assertThrows(IllegalArgumentException.class, () -> DeterministicPollingPhase.seed(TENANT, ENTITY, key, 1));
    }

    @Test
    void rejectsNullIdentifiersAndKey() {
        assertThrows(NullPointerException.class, () -> DeterministicPollingPhase.seed(null, ENTITY, "key", 1));
        assertThrows(NullPointerException.class, () -> DeterministicPollingPhase.seed(TENANT, null, "key", 1));
        assertThrows(NullPointerException.class, () -> DeterministicPollingPhase.seed(TENANT, ENTITY, null, 1));
    }
}
