package com.platform.core.id;

import java.security.SecureRandom;
import java.util.UUID;
import java.util.concurrent.ThreadLocalRandom;

/**
 * Platform-owned UUIDv7 generator (RFC 9562 layout) using Java 21 standard library only.
 * Timestamp-based sortable identity; remaining bits from SecureRandom.
 */
public final class UuidV7 {

    private static final SecureRandom SECURE_RANDOM = new SecureRandom();

    private UuidV7() {
    }

    public static UUID generate() {
        return generate(System.currentTimeMillis());
    }

    static UUID generate(long epochMillis) {
        if (epochMillis < 0) {
            throw new IllegalArgumentException("epochMillis must be non-negative");
        }
        byte[] bytes = new byte[16];
        bytes[0] = (byte) (epochMillis >>> 40);
        bytes[1] = (byte) (epochMillis >>> 32);
        bytes[2] = (byte) (epochMillis >>> 24);
        bytes[3] = (byte) (epochMillis >>> 16);
        bytes[4] = (byte) (epochMillis >>> 8);
        bytes[5] = (byte) epochMillis;

        byte[] random = new byte[10];
        SECURE_RANDOM.nextBytes(random);
        System.arraycopy(random, 0, bytes, 6, 10);

        // version 7
        bytes[6] = (byte) ((bytes[6] & 0x0f) | 0x70);
        // RFC variant 10xx
        bytes[8] = (byte) ((bytes[8] & 0x3f) | 0x80);

        long msb = 0;
        long lsb = 0;
        for (int i = 0; i < 8; i++) {
            msb = (msb << 8) | (bytes[i] & 0xff);
        }
        for (int i = 8; i < 16; i++) {
            lsb = (lsb << 8) | (bytes[i] & 0xff);
        }
        return new UUID(msb, lsb);
    }

    /** Test helper: generate with injected entropy for deterministic assertions. */
    static UUID generateForTest(long epochMillis, byte[] entropy10) {
        if (entropy10 == null || entropy10.length != 10) {
            throw new IllegalArgumentException("entropy10 must be 10 bytes");
        }
        byte[] bytes = new byte[16];
        bytes[0] = (byte) (epochMillis >>> 40);
        bytes[1] = (byte) (epochMillis >>> 32);
        bytes[2] = (byte) (epochMillis >>> 24);
        bytes[3] = (byte) (epochMillis >>> 16);
        bytes[4] = (byte) (epochMillis >>> 8);
        bytes[5] = (byte) epochMillis;
        System.arraycopy(entropy10, 0, bytes, 6, 10);
        bytes[6] = (byte) ((bytes[6] & 0x0f) | 0x70);
        bytes[8] = (byte) ((bytes[8] & 0x3f) | 0x80);
        long msb = 0;
        long lsb = 0;
        for (int i = 0; i < 8; i++) {
            msb = (msb << 8) | (bytes[i] & 0xff);
        }
        for (int i = 8; i < 16; i++) {
            lsb = (lsb << 8) | (bytes[i] & 0xff);
        }
        return new UUID(msb, lsb);
    }

    public static int versionOf(UUID id) {
        return id.version();
    }

    public static long timestampMillis(UUID id) {
        long msb = id.getMostSignificantBits();
        return (msb >>> 16) & 0xFFFFFFFFFFFFL;
    }

    /** Uniqueness helper used by concurrency tests. */
    public static UUID generateConcurrentSafe() {
        // SecureRandom is thread-safe; ThreadLocalRandom unused for UUID bits by design.
        ThreadLocalRandom.current().nextInt();
        return generate();
    }
}
