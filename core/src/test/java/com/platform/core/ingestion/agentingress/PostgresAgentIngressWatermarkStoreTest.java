package com.platform.core.ingestion.agentingress;

import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;

import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;

@Tag("postgres")
class PostgresAgentIngressWatermarkStoreTest {

    private PostgresTestSupport pg;
    private PostgresAgentIngressWatermarkStore store;
    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        store = new PostgresAgentIngressWatermarkStore(pg.jdbc());
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
    void getAndUpdateWatermarkCycle() {
        String sensorId = "sensor-infra-01";
        assertNull(store.getWatermarkForUpdate(tenantId, sensorId));

        store.updateWatermark(tenantId, sensorId, 100L, 90L, 100L, "checksum-100", "batch-100");
        var updated = store.getWatermarkForUpdate(tenantId, sensorId);
        assertNotNull(updated);
        assertEquals(100L, updated.ackedThroughSequence());
        assertEquals("checksum-100", updated.lastChecksum());
        assertEquals("batch-100", updated.lastBatchId());

        store.updateWatermark(tenantId, sensorId, 110L, 101L, 110L, "checksum-110", "batch-110");
        assertEquals(110L, store.getWatermarkForUpdate(tenantId, sensorId).ackedThroughSequence());
    }

    @Test
    void independentFromTelemetryWatermarkTable() {
        String sensorId = "sensor-shared-01";
        store.updateWatermark(tenantId, sensorId, 50L, 40L, 50L, "agent-chk", "agent-batch");

        Integer telemetryRows = pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_telemetry_watermark WHERE tenant_id=? AND sensor_id=?",
                Integer.class, tenantId, sensorId
        );
        assertEquals(0, telemetryRows);

        Integer agentRows = pg.jdbc().queryForObject(
                "SELECT COUNT(*) FROM platform.sensor_agent_ingress_watermark WHERE tenant_id=? AND sensor_id=?",
                Integer.class, tenantId, sensorId
        );
        assertEquals(1, agentRows);
    }

    @Test
    void concurrentUpdatesSerializeCleanly() throws Exception {
        String sensorId = "sensor-concurrent-agent-01";
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
        assertNotNull(store.getWatermarkForUpdate(tenantId, sensorId));
    }
}
