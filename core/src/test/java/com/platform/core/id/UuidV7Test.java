package com.platform.core.id;

import org.junit.jupiter.api.Test;

import java.util.HashSet;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class UuidV7Test {

    @Test
    void versionAndVariantAreRfcCompliant() {
        UUID id = UuidV7.generate();
        assertEquals(7, id.version());
        assertEquals(2, id.variant());
    }

    @Test
    void timestampOrderingAcrossMillisecondBoundaries() {
        UUID earlier = UuidV7.generate(1_700_000_000_000L);
        UUID later = UuidV7.generate(1_700_000_000_001L);
        assertTrue(UuidV7.timestampMillis(earlier) < UuidV7.timestampMillis(later));
        assertTrue(earlier.toString().compareTo(later.toString()) < 0);
    }

    @Test
    void uniquenessUnderConcurrency() throws Exception {
        int n = 500;
        Set<UUID> ids = ConcurrentHashMap.newKeySet();
        ExecutorService pool = Executors.newFixedThreadPool(8);
        CountDownLatch start = new CountDownLatch(1);
        CountDownLatch done = new CountDownLatch(n);
        for (int i = 0; i < n; i++) {
            pool.submit(() -> {
                try {
                    start.await();
                    ids.add(UuidV7.generateConcurrentSafe());
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                } finally {
                    done.countDown();
                }
            });
        }
        start.countDown();
        assertTrue(done.await(10, TimeUnit.SECONDS));
        pool.shutdownNow();
        assertEquals(n, ids.size());
    }

    @Test
    void deterministicEntropyPreservesVersionVariant() {
        byte[] entropy = new byte[10];
        for (int i = 0; i < entropy.length; i++) {
            entropy[i] = (byte) i;
        }
        UUID id = UuidV7.generateForTest(123456789L, entropy);
        assertEquals(7, id.version());
        assertEquals(2, id.variant());
        assertEquals(123456789L, UuidV7.timestampMillis(id));
        Set<UUID> once = new HashSet<>();
        once.add(id);
        assertEquals(1, once.size());
    }
}
