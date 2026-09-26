package com.platform.core.sensorcontrol.desiredstate;

import org.junit.jupiter.api.Test;

import java.util.EnumMap;
import java.util.LinkedHashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

public class SensorControlDesiredChecksumTest {

    /**
     * Golden vector for BUILD-004 Slice 2 / Slice 3 Go mirror.
     * Fixture: INFO logLevel; scheduler maxWorkers=4 maxQueueDepth=16 maxTrackedAssignments=32
     * maxTrackedJobs=64 maxPayloadBytes=1048576 defaultExecutionTimeoutMs=5000
     * shutdownGracePeriodMs=10000 retryMaxAttempts=3 retryInitialBackoffMs=100
     * retryMaxBackoffMs=1000 retryJitterBasisPoints=500;
     * pools inserted as SYNTHETIC then NETWORK_POLL (sorted for digest);
     * transport 15000/1048576/1048576/100/524288.
     */
    static final String GOLDEN_SHA256 =
            "ec1e58221b8f3bb05ce0ddc4d8104b2e6bfce709e0de4cc07062e80919346f1d";

    public static SensorRuntimeDesiredConfig goldenConfig(Map<SensorSchedulerPoolName, SensorSchedulerPoolConfig> pools) {
        return new SensorRuntimeDesiredConfig(
                SensorLogLevel.INFO,
                new SensorSchedulerDesiredConfig(
                        4, 16, 32, 64, 1_048_576, 5_000, 10_000, 3, 100, 1_000, 500, pools),
                new SensorTransportDesiredConfig(15_000, 1_048_576, 1_048_576, 100, 524_288)
        );
    }

    @Test
    void goldenVectorExactSha256() {
        EnumMap<SensorSchedulerPoolName, SensorSchedulerPoolConfig> pools =
                new EnumMap<>(SensorSchedulerPoolName.class);
        pools.put(SensorSchedulerPoolName.SYNTHETIC, new SensorSchedulerPoolConfig(1, 4));
        pools.put(SensorSchedulerPoolName.NETWORK_POLL, new SensorSchedulerPoolConfig(2, 8));
        SensorRuntimeDesiredConfig cfg = goldenConfig(pools);
        SensorControlDesiredConfigValidator.validate(cfg);
        assertEquals(GOLDEN_SHA256, SensorControlDesiredChecksum.compute(cfg));
    }

    @Test
    void poolInsertionOrderDoesNotChangeChecksum() {
        LinkedHashMap<SensorSchedulerPoolName, SensorSchedulerPoolConfig> orderA = new LinkedHashMap<>();
        orderA.put(SensorSchedulerPoolName.SYNTHETIC, new SensorSchedulerPoolConfig(1, 4));
        orderA.put(SensorSchedulerPoolName.NETWORK_POLL, new SensorSchedulerPoolConfig(2, 8));

        LinkedHashMap<SensorSchedulerPoolName, SensorSchedulerPoolConfig> orderB = new LinkedHashMap<>();
        orderB.put(SensorSchedulerPoolName.NETWORK_POLL, new SensorSchedulerPoolConfig(2, 8));
        orderB.put(SensorSchedulerPoolName.SYNTHETIC, new SensorSchedulerPoolConfig(1, 4));

        String a = SensorControlDesiredChecksum.compute(goldenConfig(orderA));
        String b = SensorControlDesiredChecksum.compute(goldenConfig(orderB));
        assertEquals(GOLDEN_SHA256, a);
        assertEquals(a, b);
    }

    @Test
    void validatorRejectsWorkerSumOverflow() {
        EnumMap<SensorSchedulerPoolName, SensorSchedulerPoolConfig> pools =
                new EnumMap<>(SensorSchedulerPoolName.class);
        pools.put(SensorSchedulerPoolName.NETWORK_POLL, new SensorSchedulerPoolConfig(3, 1));
        pools.put(SensorSchedulerPoolName.INFRA, new SensorSchedulerPoolConfig(2, 1));
        SensorRuntimeDesiredConfig cfg = new SensorRuntimeDesiredConfig(
                SensorLogLevel.WARN,
                new SensorSchedulerDesiredConfig(
                        4, 16, 32, 64, 1024, 1000, 1000, 1, 10, 20, 0, pools),
                new SensorTransportDesiredConfig(1000, 1024, 1024, 1, 512)
        );
        assertThrows(com.platform.core.domain.DomainException.class,
                () -> SensorControlDesiredConfigValidator.validate(cfg));
    }
}
