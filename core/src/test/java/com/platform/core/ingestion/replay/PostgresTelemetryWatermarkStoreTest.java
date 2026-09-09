package com.platform.core.ingestion.replay;

import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.transaction.support.TransactionTemplate;

import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;

@Tag("postgres")
class PostgresTelemetryWatermarkStoreTest {

    private PostgresTestSupport pg;
    private PostgresTelemetryWatermarkStore store;
    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        store = new PostgresTelemetryWatermarkStore(pg.jdbc());

        // Ensure default tenant exists
        pg.jdbc().update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?, 'Test Tenant', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                ON CONFLICT (tenant_id) DO NOTHING
                """, tenantId);
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void testGetAndUpdateWatermarkCycle() {
        String sensorId = "sensor-net-01";

        WatermarkState initial = store.getWatermarkForUpdate(sensorId);
        assertNull(initial);

        store.updateWatermark(sensorId, tenantId, 100L, 90L, 100L, "checksum-100", "batch-100");

        WatermarkState updated = store.getWatermarkForUpdate(sensorId);
        assertNotNull(updated);
        assertEquals(sensorId, updated.sensorId());
        assertEquals(tenantId, updated.tenantId());
        assertEquals(100L, updated.ackedThroughSequence());
        assertEquals(90L, updated.lastFirstSequence());
        assertEquals(100L, updated.lastLastSequence());
        assertEquals("checksum-100", updated.lastChecksum());
        assertEquals("batch-100", updated.lastBatchId());
        assertNotNull(updated.updatedAt());

        // Advance watermark
        store.updateWatermark(sensorId, tenantId, 110L, 101L, 110L, "checksum-110", "batch-110");
        WatermarkState advanced = store.getWatermarkForUpdate(sensorId);
        assertEquals(110L, advanced.ackedThroughSequence());
        assertEquals("checksum-110", advanced.lastChecksum());
    }

    @Test
    void testConcurrentUpdatesForSameSensorSerializeCleanly() throws Exception {
        String sensorId = "sensor-concurrent-01";
        ExecutorService pool = Executors.newFixedThreadPool(4);
        int iterations = 20;
        CountDownLatch startLatch = new CountDownLatch(1);
        CountDownLatch doneLatch = new CountDownLatch(4);
        AtomicInteger successfulUpdates = new AtomicInteger(0);

        for (int i = 0; i < 4; i++) {
            final int workerId = i;
            pool.submit(() -> {
                try {
                    startLatch.await();
                    for (int j = 0; j < iterations; j++) {
                        long seq = (long) (workerId * iterations + j + 1);
                        store.getWatermarkForUpdate(sensorId);
                        store.updateWatermark(sensorId, tenantId, seq, seq, seq, "sum-" + seq, "b-" + seq);
                        successfulUpdates.incrementAndGet();
                    }
                } catch (Exception e) {
                    e.printStackTrace();
                } finally {
                    doneLatch.countDown();
                }
            });
        }

        startLatch.countDown();
        assertTrue(doneLatch.await(30, TimeUnit.SECONDS));
        pool.shutdown();

        assertEquals(4 * iterations, successfulUpdates.get());
        WatermarkState finalState = store.getWatermarkForUpdate(sensorId);
        assertNotNull(finalState);
        assertTrue(finalState.ackedThroughSequence() > 0);
    }
}
