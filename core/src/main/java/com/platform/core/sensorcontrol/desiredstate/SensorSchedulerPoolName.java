package com.platform.core.sensorcontrol.desiredstate;

import java.util.Locale;

/**
 * Canonical Sensor scheduler execution classes (mirrors sensor/internal/scheduler/job.go).
 */
public enum SensorSchedulerPoolName {
    NETWORK_POLL,
    STREAMING,
    INFRA,
    DISCOVERY,
    NCM,
    SYNTHETIC;

    public static SensorSchedulerPoolName parse(String raw) {
        if (raw == null || raw.isBlank()) {
            throw new IllegalArgumentException("scheduler pool name is required");
        }
        try {
            return SensorSchedulerPoolName.valueOf(raw.trim().toUpperCase(Locale.ROOT));
        } catch (IllegalArgumentException ex) {
            throw new IllegalArgumentException("unknown scheduler pool name: " + raw);
        }
    }
}
