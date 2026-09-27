package com.platform.core.sensorcontrol.desiredstate;

import java.util.Objects;

public record SensorSchedulerPoolConfig(int workers, int queueDepth) {
    public SensorSchedulerPoolConfig {
        if (workers <= 0 || workers > SensorControlDesiredLimits.MAX_WORKERS) {
            throw new IllegalArgumentException("pool.workers out of bounds");
        }
        if (queueDepth <= 0 || queueDepth > SensorControlDesiredLimits.MAX_QUEUE_DEPTH) {
            throw new IllegalArgumentException("pool.queueDepth out of bounds");
        }
    }

    public static SensorSchedulerPoolConfig of(int workers, int queueDepth) {
        return new SensorSchedulerPoolConfig(workers, queueDepth);
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof SensorSchedulerPoolConfig that)) return false;
        return workers == that.workers && queueDepth == that.queueDepth;
    }

    @Override
    public int hashCode() {
        return Objects.hash(workers, queueDepth);
    }
}
