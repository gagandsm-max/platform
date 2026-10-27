package com.platform.core.sensorcontrol.scheduling;

import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.CharBuffer;
import java.nio.charset.CharacterCodingException;
import java.nio.charset.CodingErrorAction;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.Objects;
import java.util.UUID;

/**
 * Pure SCHED-PHASE-v1 calculation. The collection key must come from a future
 * authoritative Core caller and remain stable across Sensor reassignment/grant
 * renewal. This utility neither allocates keys nor establishes plan authority.
 */
public final class DeterministicPollingPhase {
    public static final int VERSION_1 = 1;
    private static final byte[] DOMAIN = "platform.poll.phase.v1".getBytes(StandardCharsets.US_ASCII);

    private DeterministicPollingPhase() { }

    /** Returns the first eight SHA-256 bytes as an unsigned64 bit pattern in a Java long. */
    public static long seed(UUID tenantId, UUID entityId, String collectionKey, int algorithmVersion) {
        byte[] identity = encodeIdentity(tenantId, entityId, collectionKey, algorithmVersion);
        try {
            byte[] digest = MessageDigest.getInstance("SHA-256").digest(identity);
            return ByteBuffer.wrap(digest).order(ByteOrder.BIG_ENDIAN).getLong();
        } catch (NoSuchAlgorithmException unavailable) {
            // SHA-256 is mandatory in the JDK; never substitute a different algorithm.
            throw new IllegalStateException("Required JDK SHA-256 unavailable", unavailable);
        }
    }

    /** Returns an offset in milliseconds in [0, cadenceMillis); no cadence default. */
    public static long phaseMillis(UUID tenantId, UUID entityId, String collectionKey,
                                   int algorithmVersion, long cadenceMillis) {
        if (cadenceMillis <= 0) {
            throw new IllegalArgumentException("cadenceMillis must be positive");
        }
        return Long.remainderUnsigned(seed(tenantId, entityId, collectionKey, algorithmVersion), cadenceMillis);
    }

    // Package-visible so fixed vectors can pin the byte-level interoperability contract.
    static byte[] encodeIdentity(UUID tenantId, UUID entityId, String collectionKey, int algorithmVersion) {
        if (algorithmVersion != VERSION_1) {
            throw new IllegalArgumentException("Unsupported polling phase algorithm version");
        }
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(entityId, "entityId");
        Objects.requireNonNull(collectionKey, "collectionKey");
        if (collectionKey.isBlank()) {
            throw new IllegalArgumentException("collectionKey must not be blank");
        }
        final ByteBuffer key;
        try {
            key = StandardCharsets.UTF_8.newEncoder()
                    .onMalformedInput(CodingErrorAction.REPORT)
                    .onUnmappableCharacter(CodingErrorAction.REPORT)
                    .encode(CharBuffer.wrap(collectionKey));
        } catch (CharacterCodingException malformed) {
            throw new IllegalArgumentException("collectionKey must contain well-formed Unicode", malformed);
        }
        // Java byte-buffer lengths fit unsigned32. Do not substitute UTF-16 character count.
        int size = Math.addExact(DOMAIN.length + 1 + 16 + 16 + Integer.BYTES, key.remaining());
        return ByteBuffer.allocate(size).order(ByteOrder.BIG_ENDIAN)
                .put(DOMAIN).put((byte) 0)
                .putLong(tenantId.getMostSignificantBits()).putLong(tenantId.getLeastSignificantBits())
                .putLong(entityId.getMostSignificantBits()).putLong(entityId.getLeastSignificantBits())
                .putInt(key.remaining()).put(key).array();
    }
}
