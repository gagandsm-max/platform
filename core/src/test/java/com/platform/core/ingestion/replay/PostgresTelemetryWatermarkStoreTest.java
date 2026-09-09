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

        WatermarkState initial = store.getWatermarkForUpdate(tenantId, sensorId);
        assertNull(initial);

        store.updateWatermark(tenantId, sensorId, 100L, 90L, 100L, "checksum-100", "batch-100");

        WatermarkState updated = store.getWatermarkForUpdate(tenantId, sensorId);
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
        store.updateWatermark(tenantId, sensorId, 110L, 101L, 110L, "checksum-110", "batch-110");
        WatermarkState advanced = store.getWatermarkForUpdate(tenantId, sensorId);
        assertEquals(110L, advanced.ackedThroughSequence());
        assertEquals("checksum-110", advanced.lastChecksum());
    }

    @Test
    void testTenantIsolationSameSensorIdAcrossTenants() {
        UUID tenantId2 = UUID.fromString("018f0e0a-0000-7000-8000-000000000002");
        pg.jdbc().update("""
                INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                VALUES (?, 'Tenant 2', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                ON CONFLICT (tenant_id) DO NOTHING
                """, tenantId2);

        String sensorId = "sensor-shared-name-01";

        // Update watermark for tenant 1
        store.updateWatermark(tenantId, sensorId, 100L, 90L, 100L, "chk-t1", "batch-t1");

        // Tenant 2 has no watermark yet for the same sensor ID
        assertNull(store.getWatermarkForUpdate(tenantId2, sensorId));

        // Update watermark for tenant 2 with completely different sequence
        store.updateWatermark(tenantId2, sensorId, 500L, 490L, 500L, "chk-t2", "batch-t2");

        // Verify Tenant 1 state is intact and unaffected
        WatermarkState t1State = store.getWatermarkForUpdate(tenantId, sensorId);
        assertNotNull(t1State);
        assertEquals(100L, t1State.ackedThroughSequence());
        assertEquals(tenantId, t1State.tenantId());
        assertEquals("chk-t1", t1State.lastChecksum());

        // Verify Tenant 2 state is separate
        WatermarkState t2State = store.getWatermarkForUpdate(tenantId2, sensorId);
        assertNotNull(t2State);
        assertEquals(500L, t2State.ackedThroughSequence());
        assertEquals(tenantId2, t2State.tenantId());
        assertEquals("chk-t2", t2State.lastChecksum());
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
                        store.getWatermarkForUpdate(tenantId, sensorId);
                        store.updateWatermark(tenantId, sensorId, seq, seq, seq, "sum-" + seq, "b-" + seq);
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
        WatermarkState finalState = store.getWatermarkForUpdate(tenantId, sensorId);
        assertNotNull(finalState);
        assertTrue(finalState.ackedThroughSequence() > 0);
    }
}
